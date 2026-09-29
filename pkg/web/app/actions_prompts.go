package app

import (
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// mountPromptActions registers the prompt actions.
func mountPromptActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/dismiss_prompt", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		PromptID string `json:"promptId"`
	}) error {
		prompt, err := core.PostPrompts(
			core.PostPromptWhere.RecipientID.EQ(dbUser.ID),
			core.PostPromptWhere.ID.EQ(input.PromptID),
		).One(c, db)

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		prompt.DismissedAt = null.TimeFrom(time.Now())

		if _, err := prompt.Update(c, db, boil.Infer()); err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
