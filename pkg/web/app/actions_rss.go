package app

import (
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
)

// mountRSSActions registers the RSS subscription actions.
func mountRSSActions(d *Deps, r *gin.RouterGroup) {
	feeds := d.Services.Feeds

	r.POST("/remove_rss_subscription", jsonAction(d, func(c *gin.Context, u *core.User, in struct {
		SubscriptionID string `json:"id"`
	}) error {
		return feeds.Unsubscribe(c, u, in.SubscriptionID)
	}))

	r.POST("/dissmiss_rss_item", jsonAction(d, func(c *gin.Context, u *core.User, in struct {
		SubscriptionItemID string `json:"id"`
	}) error {
		return feeds.Dismiss(c, u, in.SubscriptionItemID)
	}))
}
