package app

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountPostRoutes registers the pages and forms that write posts, comments and prompts, and the post export.
func mountPostRoutes(d *Deps, r, controlsForms *gin.RouterGroup) {
	db := d.DB
	sender := d.Sender
	mediaStorage := d.MediaStorage

	r.GET("/posts/:id/zip", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		// anyone who may see the post may export it, so the visibility
		// check is the one /posts/:id uses. The viewer may be anonymous.
		post := web.SinglePost(c, db, &userData, postID, false)

		if post.IsError() {
			ginhelpers.HTML(c, "single_post.html", post)
			return
		}

		author := post.MustGet().Post.Author

		b, err := postops.SerializeBlog(c, db, mediaStorage, author.ID, core.PostWhere.ID.EQ(postID))

		if err != nil {
			panic(err)
		}

		fname := fmt.Sprintf("export_%s_%s.zip", author.Username, time.Now().Format(time.RFC3339))
		contentLength := int64(len(b))
		contentType := "application/zip"

		reader := bytes.NewReader(b)

		extraHeaders := map[string]string{
			"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, fname),
		}

		c.DataFromReader(http.StatusOK, contentLength, contentType, reader, extraHeaders)
	})

	r.GET("/posts/:id/edit", auth.EnforceAuth, requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		ginhelpers.HTML(c, "edit_post.html", web.EditPost(c, db, &userData, postID))
	})

	r.GET("/write", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "write.html", web.Write(c, db, &userData))
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
}
