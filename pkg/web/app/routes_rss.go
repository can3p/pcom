package app

import (
	"errors"
	"net/http"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/postops/rss"
	"github.com/can3p/pcom/pkg/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/feeds"
)

// mountRSSRoutes registers the public and private RSS feeds.
func mountRSSRoutes(d *Deps, r *gin.RouterGroup) {
	reading := d.Services.Reading

	r.GET("/rss/public/:username", func(c *gin.Context) {
		username := c.Param("username")

		journal, err := reading.PublicFeed(c.Request.Context(), username)
		if err != nil {
			rssError(c, err)
			return
		}

		writeRSS(c, rss.ToFeed(
			"New posts from @"+username,
			links.AbsLink("user", username),
			journal.Author,
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
			"User feed @"+feed.Owner.Username,
			links.AbsLink("feed", feed.Owner.Username),
			feed.Owner,
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
