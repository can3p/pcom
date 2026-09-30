package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
)

// mountPostActions registers the post actions.
func mountPostActions(d *Deps, r *gin.RouterGroup) {
	posts := d.Services.Posts

	r.POST("/delete_draft", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		PostID string `json:"postId"`
	}) error {
		if err := posts.DeleteDraft(c, dbUser, input.PostID); err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
