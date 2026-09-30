package app

import (
	"errors"
	"net/http"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// pageError answers a page whose service call failed: an unknown link is a 404,
// anything else is a bug, so it panics and the recovery middleware answers 500
// and mails the admin.
func pageError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrNotFound) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	panic(err)
}

// mountAuthRoutes registers login, signup, invitations and logout.
func mountAuthRoutes(d *Deps, r, actions, nonControlsForms *gin.RouterGroup) {
	accounts := d.Services.Accounts
	forceOpenRegistation := d.Config.ForceOpenRegistration

	registrationOpen := func(c *gin.Context) (bool, error) {
		open, err := accounts.RegistrationOpen(c)

		return open || forceOpenRegistation, err
	}

	// signedOutPage runs page for a visitor who is not logged in; a logged in
	// one is sent home.
	signedOutPage := func(page func(c *gin.Context, userData auth.UserData) error) gin.HandlerFunc {
		return func(c *gin.Context) {
			userData := auth.GetUserData(c)

			if userData.IsLoggedIn {
				c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
				return
			}

			if err := page(c, userData); err != nil {
				pageError(c, err)
			}
		}
	}

	r.GET("/invite/:id", requireUUIDParam("id"), signedOutPage(func(c *gin.Context, userData auth.UserData) error {
		invite, err := accounts.Invitation(c, c.Param("id"))
		if err != nil {
			return err
		}

		c.HTML(http.StatusOK, "invite.html", web.Invite(c, invite, &userData))

		return nil
	}))

	r.GET("/login", signedOutPage(func(c *gin.Context, userData auth.UserData) error {
		returnUrl := c.Query("return_url")
		sign := c.Query("sign")

		if auth.HashValue(returnUrl) != sign {
			returnUrl = ""
			sign = ""
		}

		c.HTML(http.StatusOK, "login.html", web.Login(c, &userData, returnUrl, sign))

		return nil
	}))

	r.GET("/signup", signedOutPage(func(c *gin.Context, userData auth.UserData) error {
		open, err := registrationOpen(c)
		if err != nil {
			return err
		}

		c.HTML(http.StatusOK, "signup.html", gin.H{
			"Name":             "Signup to Webhks",
			"User":             userData,
			"RegistrationOpen": open,
			"Attribution":      c.Query("attribution"),
			"StyleNonce":       csp.GetStyleNonce(c),
			"ScriptNonce":      csp.GetScriptNonce(c),
		})

		return nil
	}))

	r.GET("/confirm_waiting_list/:id", requireUUIDParam("id"), signedOutPage(func(c *gin.Context, userData auth.UserData) error {
		if err := accounts.ConfirmWaitingList(c, c.Param("id")); err != nil {
			return err
		}

		c.HTML(http.StatusOK, "waiting_list_confirmed.html", map[string]any{
			"User":        userData,
			"StyleNonce":  csp.GetStyleNonce(c),
			"ScriptNonce": csp.GetScriptNonce(c),
		})

		return nil
	}))

	r.GET("/confirm_signup/:id", requireUUIDParam("id"), signedOutPage(func(c *gin.Context, userData auth.UserData) error {
		if err := accounts.ConfirmSignup(c, c.Param("id")); err != nil {
			return err
		}

		c.HTML(http.StatusOK, "signup_confirmed.html", map[string]any{
			"User":        userData,
			"StyleNonce":  csp.GetStyleNonce(c),
			"ScriptNonce": csp.GetScriptNonce(c),
		})

		return nil
	}))

	actions.POST("/logout", auth.Logout)

	nonControlsForms.POST("/login", signedOutPage(func(c *gin.Context, _ auth.UserData) error {
		gogoForms.DefaultHandler(c, forms.LoginFormNew(accounts))

		return nil
	}))

	nonControlsForms.POST("/accept_invite/:id", requireUUIDParam("id"), func(c *gin.Context) {
		invite, err := accounts.Invitation(c, c.Param("id"))
		if err != nil {
			pageError(c, err)
			return
		}

		gogoForms.DefaultHandler(c, forms.AcceptInviteFormNew(accounts, invite))
	})

	nonControlsForms.POST("/signup", signedOutPage(func(c *gin.Context, _ auth.UserData) error {
		open, err := registrationOpen(c)
		if err != nil {
			return err
		}

		if !open {
			c.Status(http.StatusForbidden)
			return nil
		}

		gogoForms.DefaultHandler(c, forms.SignupFormNew(accounts))

		return nil
	}))

	nonControlsForms.POST("/signup_waiting_list", signedOutPage(func(c *gin.Context, _ auth.UserData) error {
		// bots are destroying the endpoint
		const waitingListClosed = true
		if waitingListClosed {
			c.AbortWithStatus(http.StatusNotFound)
			return nil
		}

		open, err := registrationOpen(c)
		if err != nil {
			return err
		}

		if open {
			c.Status(http.StatusForbidden)
			return nil
		}

		gogoForms.DefaultHandler(c, forms.SignupWaitingListFormNew(accounts))

		return nil
	}))
}
