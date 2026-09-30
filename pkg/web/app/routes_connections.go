package app

import (
	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountConnectionRoutes registers the controls page (connections, requests and mediation) and the whitelist form.
func mountConnectionRoutes(d *Deps, controls, controlsForms *gin.RouterGroup) {
	db := d.DB

	controls.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "controls.html", web.Controls(c, db, &userData))
	})

	controlsForms.POST("/whitelist_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.WhitelistConnectionNew(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})

}
