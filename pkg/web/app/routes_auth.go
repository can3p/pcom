package app

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// mountAuthRoutes registers login, signup, invitations and logout.
func mountAuthRoutes(d *Deps, r, actions, nonControlsForms *gin.RouterGroup) {
	db := d.DB
	sender := d.Sender
	forceOpenRegistation := d.Config.ForceOpenRegistration

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

	actions.POST("/logout", auth.Logout)

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
}
