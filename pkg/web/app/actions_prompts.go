package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/model"
	"github.com/gin-gonic/gin"
)

// mountPromptActions registers the prompt actions.
func mountPromptActions(d *Deps, r *gin.RouterGroup) {
	posts := d.Services.Posts

	r.POST("/dismiss_prompt", jsonAction(d, func(c *gin.Context, dbUser *model.User, input struct {
		PromptID string `json:"promptId"`
	}) error {
		if err := posts.DismissPrompt(c, dbUser, input.PromptID); err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
