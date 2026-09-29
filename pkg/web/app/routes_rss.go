package app

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/postops/rss"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// mountRSSRoutes registers the public and private RSS feeds.
func mountRSSRoutes(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.GET("/rss/public/:username", func(c *gin.Context) {
		username := c.Param("username")

		author, err := core.Users(
			core.UserWhere.Username.EQ(username),
		).One(c.Request.Context(), db)

		if err == sql.ErrNoRows {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		} else if author.ProfileVisibility != core.ProfileVisibilityPublic {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		userHome := web.UserHome(c, db, &auth.UserData{}, username)

		if userHome.IsError() {
			_ = c.AbortWithError(http.StatusNotFound, userHome.Error())
			return
		}

		feed := rss.ToFeed(
			"New posts from @"+username,
			links.AbsLink("user", username),
			author,
			userHome.MustGet().Posts,
		)

		c.Header("Content-Type", "text/xml")

		rss, err := feed.ToRss()
		if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		c.String(http.StatusOK, rss)
	})

	r.GET("/rss/private/:token", requireUUIDParam("token"), func(c *gin.Context) {
		token := c.Param("token")

		user, err := repo.FeedTokenOwner(c.Request.Context(), db, token)

		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		} else if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		userData := &auth.UserData{
			DBUser: user,
		}

		// just a hack to avoid extracting the logic to get the posts
		userFeed := web.Feed(c, db, userData, true)

		if userFeed.IsError() {
			_ = c.AbortWithError(http.StatusInternalServerError, userFeed.Error())
			return
		}

		posts := lo.Map(userFeed.MustGet().Items, func(item *web.FeedItem, index int) *postops.Post {
			return item.Post
		})

		feed := rss.ToFeed(
			"User feed @"+user.Username,
			links.AbsLink("feed", user.Username),
			user,
			posts,
		)

		c.Header("Content-Type", "text/xml")

		rss, err := feed.ToRss()
		if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		c.String(http.StatusOK, rss)
	})
}
