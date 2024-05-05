package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
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
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	_ "github.com/lib/pq" // postgres db driver
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
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

	enforceEnvVars(requiredVars)
	if shouldUseRealSender {
		enforceEnvVars(mailjet.RequiredEnv)
	}

	// fly.io does not have sslmode enabled
	db := sqlx.MustConnect("postgres", os.Getenv("DATABASE_URL"))
	defer db.Close()

	var sender sender.Sender

	if shouldUseRealSender {
		sender = mailjet.NewSender()
	} else {
		sender = console.NewSender()
	}

	sessionSalt := os.Getenv("SESSION_SALT")

	store := pgsession.NewStore(db, []byte(sessionSalt))

	// developer timezone only messes things up
	time.Local = time.UTC

	r := gin.Default()

	r.Use(sessions.Sessions("sess", store))
	r.Use(func(c *gin.Context) { auth.Auth(c, db) })

	if util.InCluster() {
		r.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
			userData := auth.GetUserData(c)
			user := userData.DBUser

			admin.NotifyPageFailure(c, sender, err, user)
		}))
	} else {
		log.Println("Custom error reporter skipped")
	}

	html := flag.String("html", "client/html", "path to html templates")

	flag.Parse()

	r.SetFuncMap(funcmap())
	r.LoadHTMLGlob(fmt.Sprintf("%s/*.html", *html))

	r.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
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

	r.GET("/invite/:id", func(c *gin.Context) {
		invitationID := c.Param("id")

		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
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
			c.Redirect(http.StatusFound, "/controls")
			return
		}

		c.HTML(http.StatusOK, "login.html", gin.H{
			"Name": "Login to Webhks",
			"User": userData,
		})
	})

	r.GET("/signup", func(c *gin.Context) {
		attribution := c.Query("attribution")
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
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

	r.GET("/logout", auth.Logout)

	r.GET("/confirm_waiting_list/:id", func(c *gin.Context) {
		id := c.Param("id")
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
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

	r.GET("/users/:username", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)
		username := c.Param("username")

		if username != userData.DBUser.Username {
			c.String(http.StatusNotImplemented, "TODO: looking at other users journals is not yet implemented")
			return
		}

		author := userData.DBUser

		c.HTML(http.StatusOK, "user_home.html", web.UserHome(c, db, &userData, author))
	})

	r.GET("/posts/:id", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		post, err := core.Posts(
			core.PostWhere.ID.EQ(postID),
			// proper access control should go there in order to read friends posts
			core.PostWhere.UserID.EQ(userData.DBUser.ID),
		).One(c, db)

		if err == sql.ErrNoRows {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			panic(err)
		}

		c.HTML(http.StatusOK, "single_post.html", web.SinglePost(c, db, &userData, post))
	})

	controls := r.Group("/controls", auth.EnforceAuth)
	actions := controls.Group("/action")

	setupActions(actions, db)

	controls.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		c.HTML(http.StatusOK, "controls.html", web.Controls(c, db, &userData))
	})

	controls.GET("/write", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		c.HTML(http.StatusOK, "write.html", web.Write(c, db, &userData))
	})

	controls.GET("/feed/direct", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		c.HTML(http.StatusOK, "feed.html", web.DirectFeed(c, db, &userData))
	})

	controls.GET("/feed/explore", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		c.HTML(http.StatusOK, "feed.html", web.ExploreFeed(c, db, &userData))
	})

	controls.GET("/settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		c.HTML(http.StatusOK, "settings.html", web.Settings(c, db, &userData))
	})

	r.GET("/confirm_signup/:id", func(c *gin.Context) {
		id := c.Param("id")
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
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

			go admin.NotifySignupConfirmed(c, sender, user)
		}

		c.HTML(http.StatusOK, "signup_confirmed.html", map[string]interface{}{
			"User": userData,
		})
	})

	r.POST("/form/login", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
		}

		form := forms.LoginFormNew()

		gogoForms.DefaultHandler(c, db, form)
	})

	r.POST("/form/accept_invite/:id", func(c *gin.Context) {
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

	r.POST("/form/signup", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
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

	r.POST("/form/signup_waiting_list", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/controls")
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

	controls.POST("/form/send_invite", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.SendInviteFormNew(sender, dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controls.POST("/form/new_post", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.NewPostFormNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controls.POST("/form/save_settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.SettingsGeneralFormNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	controls.POST("/form/change_password", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.ChangePasswordFormNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

	if err := r.Run(); err != nil {
		panic(err)
	}
}

func funcmap() template.FuncMap {
	return template.FuncMap{
		"static_asset": func() func(n string) string {
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

				//if util.InCluster() {
				//prefix = staticRouteCluster
				//}

				return fmt.Sprintf("%s/%s", prefix, path)
			}
		}(),

		"link": links.Link,

		"abslink": links.AbsLink,

		"renderTimestamp": func(t time.Time, user *core.User) string {
			if user != nil {
				t = localizeTime(user, t)
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

		"markdown": func(s string) template.HTML {
			return markdown.ToTemplate(s)
		},

		"tzlist": func() []string {
			return util.TimeZones
		},
	}
}

func localizeTime(user *core.User, t time.Time) time.Time {
	l, err := time.LoadLocation(user.Timezone)

	if err != nil {
		log.Printf("failed to parse timezone setting: [%s] - %v", user.Timezone, err)
		return t
	}

	return t.In(l)
}
