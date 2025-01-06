package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // help go learn about timezones

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/gogo/sender/console"
	"github.com/can3p/gogo/sender/mailjet"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/media/server/storage/local"
	"github.com/can3p/pcom/pkg/media/server/storage/s3"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/postops/rss"
	"github.com/can3p/pcom/pkg/types"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csrf"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	_ "github.com/lib/pq" // postgres db driver
	"github.com/mileusna/useragent"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

var staticRoute = "/static"

var requiredVars = []string{
	"DATABASE_URL",
	"SESSION_SALT",
	"SITE_ROOT",
}

func enforceEnvVars(requiredVars []string) {
	for _, v := range requiredVars {
		if _, ok := os.LookupEnv(v); !ok {
			panic(fmt.Sprintf("var %s is not set", v))
		}
	}
}

func main() {
	var forceOpenRegistation bool
	var forceRealSender bool

	flag.BoolVar(&forceOpenRegistation, "force-signup", false, "allow new signups even if it's disabled in system settings")
	flag.BoolVar(&forceRealSender, "force-real-sender", false, "force real sender outside of cluster")

	flag.Parse()

	shouldUseRealSender := util.InCluster() || forceRealSender
	shouldUseS3 := util.InCluster()

	enforceEnvVars(requiredVars)
	if shouldUseRealSender {
		enforceEnvVars(mailjet.RequiredEnv)
		enforceEnvVars([]string{"SENDER_ADDRESS"})
	}

	if shouldUseS3 {
		enforceEnvVars(s3.RequiredEnv)
	}

	// fly.io does not have sslmode enabled
	db := sqlx.MustConnect("postgres", os.Getenv("DATABASE_URL"))
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sender sender.Sender
	var mediaStorage server.MediaStorage

	if shouldUseRealSender {
		sender = mailjet.NewSender()
	} else {
		sender = console.NewSender()
	}

	dbSender := dbsender.NewSender(db, sender)
	sender = dbSender

	go dbSender.RunPoller(ctx)

	if shouldUseS3 {
		var err error
		mediaStorage, err = s3.NewS3Server()

		if err != nil {
			panic(err)
		}
	} else {
		var err error
		mediaStorage, err = local.NewLocalServer("user_media")

		if err != nil {
			panic(err)
		}
	}

	mediaServer := server.New(mediaStorage,
		server.WithClass("thumb", server.ClassParams{Width: 720}),
		server.WithPermaCache(util.InCluster()),
	)

	if !util.InCluster() {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	sessionSalt := os.Getenv("SESSION_SALT")

	store := pgsession.NewStore(db, []byte(sessionSalt))
	store.Options(sessions.Options{
		Path: "/",
		// safari wouldn't allow to save secure cookie
		// if server works on localhost
		Secure:   util.InCluster(),
		HttpOnly: true,
		MaxAge:   24 * 3600 * 30, // make every session one month long
		SameSite: http.SameSiteLaxMode,
	})

	// developer timezone only messes things up
	time.Local = time.UTC

	router := gin.Default()

	staticAsset := loadStaticManifest()

	router.MaxMultipartMemory = 8 << 20 // 8 MiB

	if util.InCluster() {
		router.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
			userData := auth.GetUserData(c)
			user := userData.DBUser

			admin.NotifyPageFailure(c, db, sender, err, user)
		}))
	} else {
		log.Println("Custom error reporter skipped")
	}

	router.Use(csp.Csp)

	html := flag.String("html", "client/html", "path to html templates")

	flag.Parse()

	router.SetFuncMap(funcmap(staticAsset))
	router.LoadHTMLGlob(fmt.Sprintf("%s/*.html", *html))

	apiGroup := router.Group("/api/v1", func(c *gin.Context) { auth.AuthAPI(c, db) })

	setupApi(apiGroup, db, sender, mediaStorage)

	r := router.Group("/", sessions.Sessions("sess", store), func(c *gin.Context) { auth.Auth(c, db) })

	r.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		c.HTML(http.StatusOK, "index.html", web.Index(c, db, &userData))
	})

	//cache static forever
	if util.InCluster() {
		r.Group("/static", func(c *gin.Context) {
			c.Header("cache-control", "max-age=31536000, public")
			c.Next()
		}).Static("/", "dist")
	} else {
		r.Group("/static").Static("/", "dist")
	}

	r.GET("user-media/:fname", func(c *gin.Context) {
		fname := c.Param("fname")

		if fname == "" {
			c.Status(http.StatusNotFound)
			return
		}

		if fname == "robots.txt" {
			c.String(http.StatusOK, "OK")
			return
		}

		if fname == "favicon.ico" {
			c.Redirect(http.StatusMovedPermanently, staticAsset("static/favicon.ico"))
			return
		}

		err := mediaServer.ServeImage(c, c.Request, c.Writer, fname)

		if err != nil {
			panic(err)
		}
	})

	r.GET("/invite/:id", func(c *gin.Context) {
		invitationID := c.Param("id")

		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		invite, err := core.UserInvitations(
			core.UserInvitationWhere.ID.EQ(invitationID),
			core.UserInvitationWhere.CreatedUserID.IsNull(),
		).One(c, db)

		if err == sql.ErrNoRows {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			panic(err)
		}

		c.HTML(http.StatusOK, "invite.html", web.Invite(c, db, invite, &userData))
	})

	r.GET("/articles/:id", func(c *gin.Context) {
		articleName := c.Param("id")

		fname := fmt.Sprintf("client/articles/%s.md", articleName)

		body, err := os.ReadFile((fname))

		if err != nil {
			panic(err)
		}

		lines := util.SplitLines(string(body))

		title := lines[0]
		signupAttribution := lines[1]
		sbody := strings.TrimSpace(strings.Join(lines[2:], "\n"))

		userData := auth.GetUserData(c)
		c.HTML(http.StatusOK, "article.html", gin.H{
			"Name":        title,
			"Body":        sbody,
			"User":        userData,
			"Attribution": signupAttribution,
		})
	})

	r.GET("/login", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		returnUrl := c.Query("return_url")
		sign := c.Query("sign")

		if auth.HashValue(returnUrl) != sign {
			returnUrl = ""
			sign = ""
		}

		c.HTML(http.StatusOK, "login.html", web.Login(c, db, &userData, returnUrl, sign))
	})

	r.GET("/signup", func(c *gin.Context) {
		attribution := c.Query("attribution")
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		systemSettings := core.SystemSettings().OneP(c, db)

		registrationOpen := systemSettings.RegistrationOpen || forceOpenRegistation

		c.HTML(http.StatusOK, "signup.html", gin.H{
			"Name":             "Signup to Webhks",
			"User":             userData,
			"RegistrationOpen": registrationOpen,
			"Attribution":      attribution,
		})
	})

	r.GET("/confirm_waiting_list/:id", func(c *gin.Context) {
		id := c.Param("id")
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
		}

		waitingList, err := core.UserSignupRequests(
			core.UserSignupRequestWhere.ID.EQ(id),
		).One(c, db)

		if err != nil {
			if err == sql.ErrNoRows {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}

			panic(err)
		}

		if !waitingList.EmailConfirmedAt.Valid {
			waitingList.EmailConfirmedAt = null.TimeFrom(time.Now())
			waitingList.UpdateP(c, db, boil.Infer())
		}

		c.HTML(http.StatusOK, "waiting_list_confirmed.html", map[string]interface{}{
			"User": userData,
		})
	})

	r.GET("/users/:username", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		username := c.Param("username")

		ginhelpers.HTML(c, "user_home.html", web.UserHome(c, db, &userData, username))
	})

	r.GET("/rss/public/:username", func(c *gin.Context) {
		username := c.Param("username")

		author, err := core.Users(
			core.UserWhere.Username.EQ(username),
		).One(ctx, db)

		if err == sql.ErrNoRows {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			_ = c.AbortWithError(http.StatusNotFound, err)
			return
		} else if author.ProfileVisibility != core.ProfileVisibilityPublic {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		userHome := web.UserHome(c, db, &auth.UserData{}, username)

		if userHome.IsError() {
			_ = c.AbortWithError(http.StatusNotFound, userHome.Error())
			return
		}

		feed := rss.ToFeed(
			"New posts from @"+username,
			links.AbsLink("user", username),
			author,
			userHome.MustGet().Posts,
		)

		c.Header("Content-Type", "text/xml")

		rss, err := feed.ToRss()
		if err != nil {
			log.Fatal(err)
		}

		c.String(http.StatusOK, rss)
	})

	r.GET("/users/:username/user_styles", auth.EnforceReferer, func(c *gin.Context) {
		username := c.Param("username")

		user, err := core.Users(
			core.UserWhere.Username.EQ(username),
			qm.Load(core.UserRels.UserStyle),
		).One(c, db)

		if err != nil && err != sql.ErrNoRows {
			panic(err)
		}

		if user == nil || user.R.UserStyle == nil || strings.TrimSpace(user.R.UserStyle.Styles) == "" {
			c.Header("Content-Type", "text/css; charset=utf-8")
			c.String(http.StatusOK, "")
			return
		}

		headerUA := c.Request.Header.Get("User-Agent")
		ua := useragent.Parse(headerUA)
		addScope := !ua.IsFirefox()

		css := strings.TrimSpace(user.R.UserStyle.Styles)

		// no scope sucks, we need to parse and and change all the selectors
		// not doing that for now since we trust our users. We trust them, right?
		if addScope {
			css = fmt.Sprintf("@scope (.user-styles-applied) {\n\n%s\n\n}\n", css)
		}

		c.Header("Content-Type", "text/css; charset=utf-8")
		c.String(http.StatusOK, css)
	})

	r.GET("/shared/:id", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		shareID := c.Param("id")

		ginhelpers.HTML(c, "shared_post.html", web.SharedPost(c, db, &userData, shareID))
	})

	r.GET("/posts/:id", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")
		editPreview := c.Query("edit_preview") == "true"

		ginhelpers.HTML(c, "single_post.html", web.SinglePost(c, db, &userData, postID, editPreview))
	})

	r.GET("/posts/:id/md", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		post := web.SinglePost(c, db, &userData, postID, false)

		if post.IsError() {
			ginhelpers.HTML(c, "single_post.html", post)
			return
		}

		c.Header("Content-Type", "text/plain")

		dbPost := post.MustGet().Post.Post

		dbPost.Body = markdown.ReplaceImageUrls(dbPost.Body, links.MediaReplacer)

		serialized := postops.SerializePost(dbPost)
		c.String(http.StatusOK, string(serialized))
	})

	r.GET("/posts/:id/zip", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		user := userData.DBUser
		postID := c.Param("id")

		post := web.SinglePost(c, db, &userData, postID, false)

		if post.IsError() {
			ginhelpers.HTML(c, "single_post.html", post)
			return
		}

		b, err := postops.SerializeBlog(c, db, mediaStorage, user.ID, core.PostWhere.ID.EQ(postID))

		if err != nil {
			panic(err)
		}

		fname := fmt.Sprintf("export_%s_%s.zip", user.Username, time.Now().Format(time.RFC3339))
		contentLength := int64(len(b))
		contentType := "application/zip"

		reader := bytes.NewReader(b)

		extraHeaders := map[string]string{
			"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, fname),
		}

		c.DataFromReader(http.StatusOK, contentLength, contentType, reader, extraHeaders)
	})

	r.GET("/posts/:id/edit", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		ginhelpers.HTML(c, "edit_post.html", web.EditPost(c, db, &userData, postID))
	})

	r.GET("/write", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "write.html", web.Write(c, db, &userData))
	})

	r.GET("/feed", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "feed.html", web.Feed(c, db, &userData, false))
	})

	r.GET("/rss/private/:key", func(c *gin.Context) {
		api, err := core.UserAPIKeys(
			core.UserAPIKeyWhere.APIKey.EQ(c.Param("key")),
			qm.Load(core.UserAPIKeyRels.User),
		).One(c, db)

		if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		user := api.R.User

		userData := &auth.UserData{
			DBUser: user,
		}

		// just a hack to avoid extracting the logic to get the posts
		userFeed := web.Feed(c, db, userData, true)

		if userFeed.IsError() {
			_ = c.AbortWithError(http.StatusInternalServerError, userFeed.Error())
			return
		}

		posts := lo.Map(userFeed.MustGet().Items, func(item *web.FeedItem, index int) *postops.Post {
			return item.Post
		})

		feed := rss.ToFeed(
			"User feed @"+user.Username,
			links.AbsLink("feed", user.Username),
			user,
			posts,
		)

		c.Header("Content-Type", "text/xml")

		rss, err := feed.ToRss()
		if err != nil {
			log.Fatal(err)
		}

		c.String(http.StatusOK, rss)
	})

	controls := r.Group("/controls", auth.EnforceAuth)
	actions := controls.Group("/action", csrf.CheckCSRF)

	actions.POST("/logout", auth.Logout)

	setupActions(actions, db, mediaStorage)

	controls.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "controls.html", web.Controls(c, db, &userData))
	})

	controls.GET("/settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "settings.html", web.Settings(c, db, &userData))
	})

	r.GET("/confirm_signup/:id", func(c *gin.Context) {
		id := c.Param("id")
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
		}

		user, err := core.Users(
			core.UserWhere.EmailConfirmSeed.EQ(null.StringFrom(id)),
		).One(c, db)

		if err != nil {
			if err == sql.ErrNoRows {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}

			panic(err)
		}

		if !user.EmailConfirmedAt.Valid {
			user.EmailConfirmedAt = null.TimeFrom(time.Now())
			user.UpdateP(c, db, boil.Infer())

			admin.NotifySignupConfirmed(c, db, sender, user)
		}

		c.HTML(http.StatusOK, "signup_confirmed.html", map[string]interface{}{
			"User": userData,
		})
	})

	nonControlsForms := r.Group("/form", csrf.CheckCSRF)

	nonControlsForms.POST("/login", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
		}

		form := forms.LoginFormNew()

		gogoForms.DefaultHandler(c, db, form)
	})

	nonControlsForms.POST("/accept_invite/:id", func(c *gin.Context) {
		invitationID := c.Param("id")

		invite, err := core.UserInvitations(
			core.UserInvitationWhere.ID.EQ(invitationID),
			core.UserInvitationWhere.CreatedUserID.IsNull(),
		).One(c, db)

		if err == sql.ErrNoRows {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			panic(err)
		}

		form := forms.AcceptInviteFormNew(sender, invite)
		gogoForms.DefaultHandler(c, db, form)
	})

	nonControlsForms.POST("/signup", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
		}

		systemSettings := core.SystemSettings().OneP(c, db)

		registrationOpen := systemSettings.RegistrationOpen || forceOpenRegistation

		if !registrationOpen {
			c.Status(http.StatusForbidden)
			return
		}
		form := forms.SignupFormNew(sender)

		gogoForms.DefaultHandler(c, db, form)
	})

	nonControlsForms.POST("/signup_waiting_list", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
		}

		systemSettings := core.SystemSettings().OneP(c, db)

		registrationOpen := systemSettings.RegistrationOpen || forceOpenRegistation

		if registrationOpen {
			c.Status(http.StatusForbidden)
			return
		}

		form := forms.SignupWaitingListFormNew(sender)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms := controls.Group("/form", csrf.CheckCSRF)

	controlsForms.POST("/whitelist_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.WhitelistConnectionNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/send_invite", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.SendInviteFormNew(sender, dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/edit_post", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		postID := c.PostForm("post_id")

		var form *forms.PostForm
		var err error

		if postID == "" {
			form, err = forms.NewPostFormNew(c, db, sender, dbUser, links.MediaReplacer, c.PostForm("prompt_id"))

			if err != nil {
				panic(err)
			}
		} else {
			form, err = forms.EditPostFormNew(c, db, sender, dbUser, links.MediaReplacer, postID)

			if err != nil {
				if err == ginhelpers.ErrNotFound {
					c.Status(http.StatusNotFound)
					return
				}

				panic(err)
			}
		}

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/new_comment", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.NewCommentFormNew(sender, dbUser, c.PostForm("post_id"), links.MediaReplacer)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/save_settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.SettingsGeneralFormNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/save_user_styles", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.SettingsUserStylesNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/change_password", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.ChangePasswordFormNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/prompt_post", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		userIDs, err := userops.GetDirectUserIDs(c, db, dbUser.ID)

		if err != nil {
			panic(err)
		}

		directConnections, err := core.Users(
			core.UserWhere.ID.IN(userIDs),
		).All(ctx, db)

		if err != nil {
			panic(err)
		}

		form := forms.PostPromptFormNew(sender, dbUser, directConnections)

		gogoForms.DefaultHandler(c, db, form)
	})

	controlsForms.POST("/add_user_feed", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.NewAddFeedForm(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	if err := router.Run(); err != nil {
		panic(err)
	}
}

func funcmap(staticAsset staticAssetFunc) template.FuncMap {
	markdown := func(view types.HTMLView) func(s string, add ...string) template.HTML {
		return func(s string, add ...string) template.HTML {
			return markdown.ToEnrichedTemplate(s, view, links.MediaReplacer, func(in string, add2 ...string) string {
				// ugly hack to handle cut links
				if in == "single_post_special" {
					args := []string{}
					args = append(args, add...)
					args = append(args, add2...)

					return links.Link("post", args...)
				}

				return links.Link(in, add2...)
			})
		}
	}

	return template.FuncMap{
		"static_asset": staticAsset,

		"link": links.Link,

		"abslink": links.AbsLink,

		"renderTimestamp": func(t time.Time, user *core.User) string {
			if user != nil {
				t = util.LocalizeTime(user, t)
			}

			return t.Format("Mon, 02 Jan 2006 15:04")
		},

		"toMap": func(args ...interface{}) map[string]interface{} {
			if len(args)%2 != 0 {
				panic("toMap got uneven number of arguments")
			}

			out := map[string]interface{}{}

			idx := 0

			for idx+1 < len(args) {
				k := args[idx].(string)

				out[k] = args[idx+1]
				idx += 2
			}

			return out
		},

		// we could do a parameter, but this way we get a free type check
		"markdown_single_post":  markdown(types.ViewSinglePost),
		"markdown_feed":         markdown(types.ViewFeed),
		"markdown_edit_preview": markdown(types.ViewEditPreview),
		"markdown_comment":      markdown(types.ViewComment),
		"markdown_article":      markdown(types.ViewArticle),

		"tzlist": func() []string {
			return util.TimeZones
		},
	}
}

type staticAssetFunc func(n string) string

func loadStaticManifest() staticAssetFunc {
	manifest, err := os.ReadFile("dist/manifest.json")

	if err != nil {
		panic(err)
	}

	files := map[string]string{}

	err = json.Unmarshal(manifest, &files)

	if err != nil {
		panic(err)
	}

	return func(n string) string {
		path, ok := files[n]

		if !ok {
			panic(fmt.Sprintf("asset [%s] is not defined", n))
		}

		prefix := staticRoute

		if pr, ok := os.LookupEnv("STATIC_CDN"); ok && util.InCluster() {
			prefix = pr
		}

		return fmt.Sprintf("%s/%s", prefix, path)
	}
}
