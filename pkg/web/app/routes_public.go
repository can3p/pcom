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
	reading := d.Services.Reading

	r.GET("/", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if userData.IsLoggedIn {
			c.Redirect(http.StatusFound, links.DefaultAuthorizedHome())
			return
		}

		c.HTML(http.StatusOK, "index.html", web.Index(c, &userData))
	})

	r.GET("/articles/:id", func(c *gin.Context) {
		articleName := c.Param("id")

		if !articlesRE.MatchString(articleName) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		title, signupAttribution, sbody, err := loadArticle(articleName)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}

			panic(err)
		}

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

		journal, err := reading.Journal(c, userData.DBUser, c.Param("username"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "user_home.html", web.UserHome(c, &userData, journal))
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
		editPreview := c.Query("edit_preview") == "true"

		post, err := reading.Post(c, userData.DBUser, c.Param("id"), editPreview)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "single_post.html", web.PostPage(c, &userData, post))
	})

	r.GET("/posts/:id/md", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)

		post, err := reading.Post(c, userData.DBUser, c.Param("id"), false)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.Header("Content-Type", "text/plain")

		dbPost := post.Post.Post

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

		posts, err := reading.Explore(c, userData.DBUser)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "feed.html", web.Explore(c, &userData, posts))
	})

	r.GET("/feed", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		feed, err := reading.Feed(c, userData.DBUser, false)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "feed.html", web.Feed(c, &userData, feed))
	})
}

// loadArticle reads a static article: its title, the signup attribution and
// the markdown body. It returns fs.ErrNotExist for an unknown article.
func loadArticle(name string) (title, attribution, body string, err error) {
	raw, err := os.ReadFile(fmt.Sprintf("client/articles/%s.md", name))
	if err != nil {
		return "", "", "", err
	}

	lines := util.SplitLines(string(raw))

	return lines[0], lines[1], strings.TrimSpace(strings.Join(lines[2:], "\n")), nil
}
