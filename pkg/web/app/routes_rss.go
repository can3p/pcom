package app

import (
	"errors"
	"net/http"

	"github.com/can3p/pcom/pkg/postops/rss"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/feeds"
)

// mountRSSRoutes registers the public and private RSS feeds.
func mountRSSRoutes(d *Deps, r *gin.RouterGroup) {
	site := siteOf(d)

	reading := d.Services.Reading

	r.GET("/rss/public", func(c *gin.Context) {
		posts, err := reading.PublicPostsRSS(c.Request.Context())
		if err != nil {
			rssError(c, err)
			return
		}

		writeRSS(c, rss.ToFeed(site, "Public posts on "+web.ProjectName, site.Root+"/", posts))
	})

	r.GET("/rss/public/:username", func(c *gin.Context) {
		username := c.Param("username")

		journal, err := reading.PublicFeed(c.Request.Context(), username)
		if err != nil {
			rssError(c, err)
			return
		}

		writeRSS(c, rss.ToFeed(
			site,
			"New posts from @"+username,
			site.Abs("user", username),
			journal.Posts,
		))
	})

	r.GET("/rss/private/:token", requireUUIDParam("token"), func(c *gin.Context) {
		feed, err := reading.PrivateFeed(c.Request.Context(), c.Param("token"))
		if err != nil {
			rssError(c, err)
			return
		}

		writeRSS(c, rss.ToFeed(
			site,
			"User feed @"+feed.Owner.Username,
			site.Abs("feed", feed.Owner.Username),
			feed.Posts,
		))
	})
}

// rssError answers a feed that can't be served: 404 for a feed that doesn't
// exist, 500 otherwise.
func rssError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrNotFound) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	_ = c.AbortWithError(http.StatusInternalServerError, err)
}

func writeRSS(c *gin.Context, feed *feeds.Feed) {
	c.Header("Content-Type", "text/xml")

	out, err := feed.ToRss()
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	c.String(http.StatusOK, out)
}
