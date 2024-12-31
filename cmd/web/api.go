package main

import (
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func setupApi(r *gin.RouterGroup, db *sqlx.DB, sender sender.Sender, mediaStorage server.MediaStorage) {
	r.GET("/posts", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiGetPosts(c, db, userData.DBUser.ID))
	})

	r.POST("/posts", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiNewPost(c, db, sender, userData.DBUser, links.MediaReplacer))
	})

	r.POST("/posts/:id", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiEditPost(c, db, sender, userData.DBUser, links.MediaReplacer, c.Param("id")))
	})

	r.DELETE("/posts/:id", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiDeletePost(c, db, userData.DBUser, c.Param("id")))
	})

	r.PUT("/image", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiUploadImage(c, db, userData.DBUser, mediaStorage))
	})
}
