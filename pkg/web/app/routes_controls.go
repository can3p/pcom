package app

import (
	"net/http"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountControlsRoutes registers the signed-in pages and their forms.
func mountControlsRoutes(d *Deps, r, controls, controlsForms *gin.RouterGroup) {
	db := d.DB
	sender := d.Sender

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

	controls.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "controls.html", web.Controls(c, db, &userData))
	})

	controls.GET("/settings", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "settings.html", web.Settings(c, db, &userData))
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
