package app

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountPublicRoutes registers the pages anyone may read.
func mountPublicRoutes(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		c.HTML(http.StatusOK, "index.html", web.Index(c, db, &userData))
	})

	r.GET("/articles/:id", func(c *gin.Context) {
		articleName := c.Param("id")

		if !articlesRE.MatchString(articleName) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		fname := fmt.Sprintf("client/articles/%s.md", articleName)

		if _, err := os.Stat(fname); errors.Is(err, fs.ErrNotExist) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			panic(err)
		}

		body, err := os.ReadFile((fname))

		if err != nil {
			panic(err)
		}

		lines := util.SplitLines(string(body))

		title := lines[0]
		signupAttribution := lines[1]
		sbody := strings.TrimSpace(strings.Join(lines[2:], "\n"))

		userData := auth.GetUserData(c)
		c.HTML(http.StatusOK, "article.html", gin.H{
			"Name":        title,
			"Body":        sbody,
			"User":        userData,
			"Attribution": signupAttribution,
			"StyleNonce":  csp.GetStyleNonce(c),
			"ScriptNonce": csp.GetScriptNonce(c),
		})
	})

	r.GET("/users/:username", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		username := c.Param("username")

		ginhelpers.HTML(c, "user_home.html", web.UserHome(c, db, &userData, username))
	})

	r.GET("/shared/:id", requireUUIDParam("id"), func(c *gin.Context) {
		shared, err := d.Services.Shares.Get(c, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		userData := auth.GetUserData(c)
		c.HTML(http.StatusOK, "shared_post.html", web.SharedPost(c, &userData, shared))
	})

	r.GET("/posts/:id", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")
		editPreview := c.Query("edit_preview") == "true"

		ginhelpers.HTML(c, "single_post.html", web.SinglePost(c, db, &userData, postID, editPreview))
	})

	r.GET("/posts/:id/md", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		post := web.SinglePost(c, db, &userData, postID, false)

		if post.IsError() {
			ginhelpers.HTML(c, "single_post.html", post)
			return
		}

		c.Header("Content-Type", "text/plain")

		dbPost := post.MustGet().Post.Post

		body, err := markdown.ReplaceImageUrls(dbPost.Body, links.MediaReplacer)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err) //nolint:errcheck
			return
		}
		dbPost.Body = body

		serialized := postops.SerializePost(dbPost)
		c.String(http.StatusOK, string(serialized))
	})

	r.GET("/explore", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "feed.html", web.Explore(c, db, &userData))
	})

	r.GET("/feed", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "feed.html", web.Feed(c, db, &userData, false))
	})
}
