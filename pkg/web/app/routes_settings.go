package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/mileusna/useragent"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// mountSettingsRoutes registers the settings page, its forms, invitations and the user styles.
func mountSettingsRoutes(d *Deps, r, controls, controlsForms *gin.RouterGroup) {
	db := d.DB
	sender := d.Sender

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

	controls.GET("/settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "settings.html", web.Settings(c, db, &userData))
	})

	controlsForms.POST("/send_invite", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.SendInviteFormNew(sender, dbUser)

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
}
