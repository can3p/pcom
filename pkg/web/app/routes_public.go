package app

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/util"
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

		page, err := reading.PublicPosts(c, c.Query("cursor"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		indexPage := web.Index(c, &userData, page)
		if isLoadMore(c) {
			c.HTML(http.StatusOK, "partial--feed-items.html", map[string]any{
				"Items": indexPage.Items, "User": indexPage.User, "Next": indexPage.Next, "URL": indexPage.LoadMoreURL,
			})
			return
		}

		c.HTML(http.StatusOK, "index.html", indexPage)
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

		journal, err := reading.Journal(c, userData.DBUser, c.Param("username"), c.Query("cursor"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		home := web.UserHome(c, &userData, journal)
		if isLoadMore(c) {
			c.HTML(http.StatusOK, "partial--post-list.html", map[string]any{
				"Posts": home.Posts, "User": home.User, "Next": home.Next, "URL": links.Link("user", home.Author.Username),
			})
			return
		}

		c.HTML(http.StatusOK, "user_home.html", home)
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

		body, err := markdown.ReplaceImageUrls(dbPost.Body, siteOf(d).MediaReplacer)
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

		if !userData.IsLoggedIn {
			c.Redirect(http.StatusFound, "/")
			return
		}

		page, err := reading.Explore(c, userData.DBUser, c.Query("cursor"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		feedPage := web.Explore(c, &userData, page.Posts)
		feedPage.Next = page.Next
		renderFeed(c, feedPage)
	})

	r.GET("/feed", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		feed, err := reading.Feed(c, userData.DBUser, c.Query("cursor"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		page := web.Feed(c, &userData, feed)
		// the prompts and connections come with the first page only
		page.Capabilities.ShowPromptForm = c.Query("cursor") == ""
		renderFeed(c, page)
	})
}

// isLoadMore reports a "Load more" request from htmx: it has a cursor and is
// not a boosted navigation.
func isLoadMore(c *gin.Context) bool {
	return c.GetHeader("HX-Request") == "true" && c.GetHeader("HX-Boosted") != "true" && c.Query("cursor") != ""
}

// renderFeed renders a page of a list: only the items and the next button
// for an htmx "Load more" request, the whole page otherwise.
func renderFeed(c *gin.Context, page *web.FeedPage) {
	if isLoadMore(c) {
		c.HTML(http.StatusOK, "partial--feed-items.html", map[string]any{
			"Items": page.Items, "User": page.User, "Next": page.Next, "URL": page.LoadMoreURL,
		})
		return
	}

	c.HTML(http.StatusOK, "feed.html", page)
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
