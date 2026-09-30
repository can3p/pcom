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

// mountPostRoutes registers the pages and forms that write posts, comments and prompts, and the post export.
func mountPostRoutes(d *Deps, r, controlsForms *gin.RouterGroup) {
	posts := d.Services.Posts

	r.GET("/posts/:id/zip", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		// anyone who may see the post may export it, so the visibility
		// check is the one /posts/:id uses. The viewer may be anonymous.
		post, err := d.Services.Reading.Post(c, userData.DBUser, postID, false)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		author := post.Post.Author

		b, err := posts.ExportPost(c, author.ID, postID)

		if err != nil {
			panic(err)
		}

		sendZip(c, author.Username, b)
	})

	r.GET("/posts/:id/edit", auth.EnforceAuth, requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "edit_post.html", web.EditPost(c, posts, &userData, c.Param("id")))
	})

	r.GET("/write", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "write.html", web.Write(c, posts, &userData))
	})

	controlsForms.POST("/edit_post", func(c *gin.Context) {
		dbUser := auth.GetUserData(c).DBUser

		postID := c.PostForm("post_id")

		var form *forms.PostForm
		var err error

		if postID == "" {
			form, err = forms.NewPostFormNew(c, posts, dbUser, c.PostForm("prompt_id"))

			if err != nil {
				panic(err)
			}
		} else {
			form, err = forms.EditPostFormNew(c, posts, dbUser, postID)

			if err != nil {
				if err == ginhelpers.ErrNotFound {
					c.Status(http.StatusNotFound)
					return
				}

				panic(err)
			}
		}

		gogoForms.DefaultHandler(c, form)
	})

	controlsForms.POST("/new_comment", func(c *gin.Context) {
		dbUser := auth.GetUserData(c).DBUser

		form := forms.NewCommentFormNew(posts, dbUser, c.PostForm("post_id"))

		gogoForms.DefaultHandler(c, form)
	})

	controlsForms.POST("/prompt_post", func(c *gin.Context) {
		dbUser := auth.GetUserData(c).DBUser

		directConnections, err := posts.DirectConnections(c, dbUser)

		if err != nil {
			panic(err)
		}

		gogoForms.DefaultHandler(c, forms.PostPromptFormNew(posts, dbUser, directConnections))
	})
}
