package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/gin-gonic/gin"
)

// mountSettingsActions registers the account settings actions.
func mountSettingsActions(d *Deps, r *gin.RouterGroup) {
	accounts := d.Services.Accounts

	// no key rotation for now
	// feel free to implement/change
	r.POST("/generate_api_key", func(c *gin.Context) {
		if err := accounts.GenerateAPIKey(c, auth.GetUserData(c).DBUser); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	// creates the private feed token or replaces it; the old feed URL stops working
	r.POST("/regenerate_feed_token", func(c *gin.Context) {
		if err := accounts.RegenerateFeedToken(c, auth.GetUserData(c).DBUser); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})
}
