package app

import (
	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/gin-gonic/gin"
)

// mountFeedRoutes registers the RSS subscription form.
func mountFeedRoutes(d *Deps, controlsForms *gin.RouterGroup) {
	db := d.DB

	controlsForms.POST("/add_user_feed", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		form := forms.NewAddFeedForm(dbUser)

		gogoForms.DefaultHandler(c, db, form)
	})
}
