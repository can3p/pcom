package app

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/media"
	mediaerrors "github.com/can3p/pcom/pkg/media/errors"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/postops/rss"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mileusna/useragent"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

var articlesRE = regexp.MustCompile("^[a-z0-9]+(_[a-z0-9]+)*$")

// mountRoutes registers every route on the groups New creates, in the order
// the binary has always registered them. Step 2 of R1 splits it by area.
func mountRoutes(d *Deps, router *gin.Engine, apiGroup, r, controls, actions, nonControlsForms, controlsForms *gin.RouterGroup) {
	db := d.DB
	sender := d.Sender
	mediaStorage := d.MediaStorage
	mediaServer := d.MediaServer
	staticAsset := d.Config.StaticAsset
	forceOpenRegistation := d.Config.ForceOpenRegistration

	//cache static forever
	if d.Config.InCluster {
		router.Group("/static", func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=604800, immutable, stale-while-revalidate=86400")
			c.Next()
		}).Static("/", "dist")
	} else {
		router.Group("/static").Static("/", "dist")
	}

	router.GET("user-media/:fname", func(c *gin.Context) {
		switch c.Param("fname") {
		case "robots.txt":
			c.String(http.StatusOK, "OK")
		case "favicon.ico":
			c.Redirect(http.StatusMovedPermanently, staticAsset("static/favicon.ico"))
		default:
			c.Status(http.StatusNotFound)
		}
	})

	router.GET("user-media/:fname/:class", func(c *gin.Context) {
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

		err := mediaServer.ServeImage(c, mediaServer, c.Request, c.Writer, fname)

		if err != nil {
			if errors.Is(err, mediaerrors.ErrNotFound) || errors.Is(err, media.ErrNotFound) {
				c.Status(http.StatusNotFound)
				return
			}

			panic(err)
		}
	})

	setupApi(apiGroup, db, sender, mediaStorage)

	r.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		c.HTML(http.StatusOK, "index.html", web.Index(c, db, &userData))
	})

	r.GET("/invite/:id", requireUUIDParam("id"), func(c *gin.Context) {
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

		if !articlesRE.MatchString(articleName) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		fname := fmt.Sprintf("client/articles/%s.md", articleName)

		if _, err := os.Stat(fname); errors.Is(err, fs.ErrNotExist) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			panic(err)
		}

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
			"StyleNonce":  csp.GetStyleNonce(c),
			"ScriptNonce": csp.GetScriptNonce(c),
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
			"StyleNonce":       csp.GetStyleNonce(c),
			"ScriptNonce":      csp.GetScriptNonce(c),
		})
	})

	r.GET("/confirm_waiting_list/:id", requireUUIDParam("id"), func(c *gin.Context) {
		id := c.Param("id")

		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
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

		c.HTML(http.StatusOK, "waiting_list_confirmed.html", map[string]any{
			"User":        userData,
			"StyleNonce":  csp.GetStyleNonce(c),
			"ScriptNonce": csp.GetScriptNonce(c),
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
		).One(c.Request.Context(), db)

		if err == sql.ErrNoRows {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
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
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
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

	r.GET("/shared/:id", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		shareID := c.Param("id")

		ginhelpers.HTML(c, "shared_post.html", web.SharedPost(c, db, &userData, shareID))
	})

	r.GET("/posts/:id", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")
		editPreview := c.Query("edit_preview") == "true"

		ginhelpers.HTML(c, "single_post.html", web.SinglePost(c, db, &userData, postID, editPreview))
	})

	r.GET("/posts/:id/md", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		post := web.SinglePost(c, db, &userData, postID, false)

		if post.IsError() {
			ginhelpers.HTML(c, "single_post.html", post)
			return
		}

		c.Header("Content-Type", "text/plain")

		dbPost := post.MustGet().Post.Post

		body, err := markdown.ReplaceImageUrls(dbPost.Body, links.MediaReplacer)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err) //nolint:errcheck
			return
		}
		dbPost.Body = body

		serialized := postops.SerializePost(dbPost)
		c.String(http.StatusOK, string(serialized))
	})

	r.GET("/posts/:id/zip", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		// anyone who may see the post may export it, so the visibility
		// check is the one /posts/:id uses. The viewer may be anonymous.
		post := web.SinglePost(c, db, &userData, postID, false)

		if post.IsError() {
			ginhelpers.HTML(c, "single_post.html", post)
			return
		}

		author := post.MustGet().Post.Author

		b, err := postops.SerializeBlog(c, db, mediaStorage, author.ID, core.PostWhere.ID.EQ(postID))

		if err != nil {
			panic(err)
		}

		fname := fmt.Sprintf("export_%s_%s.zip", author.Username, time.Now().Format(time.RFC3339))
		contentLength := int64(len(b))
		contentType := "application/zip"

		reader := bytes.NewReader(b)

		extraHeaders := map[string]string{
			"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, fname),
		}

		c.DataFromReader(http.StatusOK, contentLength, contentType, reader, extraHeaders)
	})

	r.GET("/posts/:id/edit", auth.EnforceAuth, requireUUIDParam("id"), func(c *gin.Context) {
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

	r.GET("/explore", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "feed.html", web.Explore(c, db, &userData))
	})

	r.GET("/rss/private/:token", requireUUIDParam("token"), func(c *gin.Context) {
		token := c.Param("token")

		user, err := repo.FeedTokenOwner(c.Request.Context(), db, token)

		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

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
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		c.String(http.StatusOK, rss)
	})

	actions.POST("/logout", auth.Logout)

	setupActions(actions, db, mediaStorage)

	// creates the private feed token or replaces it; the old feed URL stops working
	actions.POST("/regenerate_feed_token", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if _, err := repo.RegenerateFeedToken(c.Request.Context(), db, userData.DBUser.ID); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	controls.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "controls.html", web.Controls(c, db, &userData))
	})

	controls.GET("/settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "settings.html", web.Settings(c, db, &userData))
	})

	r.GET("/confirm_signup/:id", requireUUIDParam("id"), func(c *gin.Context) {
		id := c.Param("id")

		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
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

			if nerr := admin.NotifySignupConfirmed(c, db, sender, user); nerr != nil {
				log.Printf("failed to queue the signup confirmed notification: %v", nerr)
			}
		}

		c.HTML(http.StatusOK, "signup_confirmed.html", map[string]any{
			"User":        userData,
			"StyleNonce":  csp.GetStyleNonce(c),
			"ScriptNonce": csp.GetScriptNonce(c),
		})
	})

	nonControlsForms.POST("/login", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		form := forms.LoginFormNew()

		gogoForms.DefaultHandler(c, db, form)
	})

	nonControlsForms.POST("/accept_invite/:id", requireUUIDParam("id"), func(c *gin.Context) {
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
			return
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
			return
		}

		// bots are destroying the endpoint
		if true {
			c.AbortWithStatus(http.StatusNotFound)
			return
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
		).All(c, db)

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
}

// requireUUIDParam answers 404 when the path parameter is not a valid UUID,
// which would otherwise reach a UUID column and fail with a 500.
func requireUUIDParam(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := uuid.Parse(c.Param(name)); err != nil {
			c.AbortWithStatus(http.StatusNotFound)
		}
	}
}
