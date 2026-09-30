package app

import (
	"fmt"
	"net/http"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/mileusna/useragent"
)

// mountSettingsRoutes registers the settings page, its forms, invitations and the user styles.
func mountSettingsRoutes(d *Deps, r, controls, controlsForms *gin.RouterGroup) {
	accounts := d.Services.Accounts

	r.GET("/users/:username/user_styles", auth.EnforceReferer(d.Config.SiteRoot), func(c *gin.Context) {
		css, err := accounts.UserStyles(c, c.Param("username"))
		if err != nil {
			panic(err)
		}

		c.Header("Content-Type", "text/css; charset=utf-8")

		if css == "" {
			c.String(http.StatusOK, "")
			return
		}

		// no scope sucks, we need to parse and and change all the selectors
		// not doing that for now since we trust our users. We trust them, right?
		if !useragent.Parse(c.Request.Header.Get("User-Agent")).IsFirefox() {
			css = fmt.Sprintf("@scope (.user-styles-applied) {\n\n%s\n\n}\n", css)
		}

		c.String(http.StatusOK, css)
	})

	controls.GET("/settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		view, err := accounts.Settings(c, userData.DBUser)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "settings.html", web.Settings(c, accounts, &userData, view))
	})

	// form maps a settings form to its route: the form is built for the
	// logged in user and handed to gogo.
	form := func(newForm func(c *gin.Context) gogoForms.Form) gin.HandlerFunc {
		return func(c *gin.Context) {
			gogoForms.DefaultHandler(c, newForm(c))
		}
	}

	controlsForms.POST("/send_invite", form(func(c *gin.Context) gogoForms.Form {
		return forms.SendInviteFormNew(accounts, auth.GetUserData(c).DBUser)
	}))

	controlsForms.POST("/save_settings", form(func(c *gin.Context) gogoForms.Form {
		return forms.SettingsGeneralFormNew(accounts, auth.GetUserData(c).DBUser)
	}))

	controlsForms.POST("/save_user_styles", form(func(c *gin.Context) gogoForms.Form {
		return forms.SettingsUserStylesNew(accounts, auth.GetUserData(c).DBUser)
	}))

	controlsForms.POST("/change_password", form(func(c *gin.Context) gogoForms.Form {
		return forms.ChangePasswordFormNew(accounts, auth.GetUserData(c).DBUser)
	}))
}
