package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/gin-gonic/gin"
)

// mountSettingsActions registers the account settings actions. Each answers
// with its settings section, which the page swaps in place.
func mountSettingsActions(d *Deps, r *gin.RouterGroup) {
	accounts := d.Services.Accounts

	// creates a key with a name and a scope; the key is shown in this
	// response only
	r.POST("/generate_api_key", func(c *gin.Context) {
		var in struct {
			Name  string            `json:"name"`
			Scope model.APIKeyScope `json:"scope"`
		}

		if err := c.BindJSON(&in); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		user := auth.GetUserData(c).DBUser

		newKey, err := accounts.CreateAPIKey(c, user, in.Name, in.Scope)

		if invalid, ok := errors.AsType[*service.ValidationError](err); ok {
			renderAPIKeys(c, d, user, nil, invalid.Message)
			return
		}

		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		renderAPIKeys(c, d, user, newKey, "")
	})

	// deletes a key of the user's; it stops working at once
	r.POST("/revoke_api_key", func(c *gin.Context) {
		var in struct {
			ID string `json:"id"`
		}

		if err := c.BindJSON(&in); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		user := auth.GetUserData(c).DBUser

		if err := accounts.RevokeAPIKey(c, user, in.ID); err != nil {
			reportError(c, actionMessage(err))
			return
		}

		renderAPIKeys(c, d, user, nil, "")
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

// renderAPIKeys answers with the API keys section: the user's keys, the key
// just created (newKey, shown this once) and why a key was not (msg).
func renderAPIKeys(c *gin.Context, d *Deps, user *model.User, newKey *model.UserAPIKey, msg string) {
	keys, err := d.Services.Accounts.APIKeys(c, user)
	if err != nil {
		reportError(c, actionMessage(err))
		return
	}

	c.HTML(http.StatusOK, "partial--settings_api.html", gin.H{"APIKeys": keys, "NewKey": newKey, "Error": msg, "DBUser": user})
}
