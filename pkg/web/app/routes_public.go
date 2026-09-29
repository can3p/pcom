package app

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/mileusna/useragent"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// mountPublicRoutes registers the pages anyone may read.
func mountPublicRoutes(d *Deps, r *gin.RouterGroup) {
	db := d.DB
	mediaStorage := d.MediaStorage

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

	r.GET("/users/:username/user_styles", auth.EnforceReferer, func(c *gin.Context) {
		username := c.Param("username")

		user, err := core.Users(
			core.UserWhere.Username.EQ(username),
			qm.Load(core.UserRels.UserStyle),
		).One(c, db)

		if err != nil && err != sql.ErrNoRows {
			panic(err)
		}

		if user == nil || user.R.UserStyle == nil || strings.TrimSpace(user.R.UserStyle.Styles) == "" {
			c.Header("Content-Type", "text/css; charset=utf-8")
			c.String(http.StatusOK, "")
			return
		}

		headerUA := c.Request.Header.Get("User-Agent")
		ua := useragent.Parse(headerUA)
		addScope := !ua.IsFirefox()

		css := strings.TrimSpace(user.R.UserStyle.Styles)

		// no scope sucks, we need to parse and and change all the selectors
		// not doing that for now since we trust our users. We trust them, right?
		if addScope {
			css = fmt.Sprintf("@scope (.user-styles-applied) {\n\n%s\n\n}\n", css)
		}

		c.Header("Content-Type", "text/css; charset=utf-8")
		c.String(http.StatusOK, css)
	})

	r.GET("/shared/:id", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		shareID := c.Param("id")

		ginhelpers.HTML(c, "shared_post.html", web.SharedPost(c, db, &userData, shareID))
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

	r.GET("/explore", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "feed.html", web.Explore(c, db, &userData))
	})
}
