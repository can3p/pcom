package app

import (
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/gin-gonic/gin"
)

// mountSettingsActions registers the account settings actions. Each answers
// with its settings section, which the page swaps in place.
func mountSettingsActions(d *Deps, r *gin.RouterGroup) {
	accounts := d.Services.Accounts

	// no key rotation for now
	// feel free to implement/change
	r.POST("/generate_api_key", func(c *gin.Context) {
		user := auth.GetUserData(c).DBUser

		if err := accounts.GenerateAPIKey(c, user); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		view, err := accounts.Settings(c, user)
		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		c.HTML(http.StatusOK, "partial--settings_api.html", gin.H{"APIKey": view.APIKey})
	})

	// creates the private feed token or replaces it; the old feed URL stops working
	r.POST("/regenerate_feed_token", func(c *gin.Context) {
		user := auth.GetUserData(c).DBUser

		if err := accounts.RegenerateFeedToken(c, user); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		view, err := accounts.Settings(c, user)
		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		c.HTML(http.StatusOK, "partial--settings_private_feed.html", gin.H{"FeedURL": view.FeedURL})
	})
}
