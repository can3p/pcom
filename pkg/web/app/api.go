package app

import (
	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

func setupApi(r *gin.RouterGroup, d *Deps) {
	posts := d.Services.Posts

	r.GET("/posts", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiGetPosts(c, posts, userData.DBUser))
	})

	r.POST("/posts", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiNewPost(c, posts, userData.DBUser))
	})

	r.POST("/posts/:id", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiEditPost(c, posts, userData.DBUser, c.Param("id")))
	})

	r.DELETE("/posts/:id", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiDeletePost(c, posts, userData.DBUser, c.Param("id")))
	})

	r.PUT("/image", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		ginhelpers.API(c, web.ApiUploadImageWith(c, posts, userData.DBUser))
	})
}
