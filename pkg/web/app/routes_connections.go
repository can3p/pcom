package app

import (
	"net/http"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountConnectionRoutes registers the controls page (connections, requests and mediation) and the whitelist form.
func mountConnectionRoutes(d *Deps, controls, controlsForms *gin.RouterGroup) {
	conns := d.Services.Connections

	controls.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		view, err := conns.Controls(c, userData.DBUser)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "controls.html", web.Controls(c, &userData, view))
	})

	controlsForms.POST("/whitelist_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.WhitelistConnectionNew(dbUser, conns)

		gogoForms.DefaultHandler(c, form)
	})

}
