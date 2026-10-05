package app

import (
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/gin-gonic/gin"
)

// mountRSSActions registers the RSS subscription actions.
func mountRSSActions(d *Deps, r *gin.RouterGroup) {
	feeds := d.Services.Feeds

	// answers with the settings' feeds section, which the page swaps in place
	r.POST("/remove_rss_subscription", func(c *gin.Context) {
		var in struct {
			SubscriptionID string `json:"id"`
		}

		if err := c.BindJSON(&in); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		user := auth.GetUserData(c).DBUser

		if err := feeds.Unsubscribe(c, user, in.SubscriptionID); err != nil {
			reportError(c, actionMessage(err))
			return
		}

		subs, err := feeds.Subscriptions(c, user)
		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		// the add form is only rendered here, so it needs no service
		addForm := forms.NewAddFeedForm(nil, user).TemplateData()
		c.HTML(http.StatusOK, forms.FeedsSectionTemplate, forms.FeedsSection(subs, user, addForm))
	})

	r.POST("/dissmiss_rss_item", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		SubscriptionItemID string `json:"id"`
	}) error {
		return feeds.Dismiss(c, u, in.SubscriptionItemID)
	}))
}
