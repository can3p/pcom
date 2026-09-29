package app

import (
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// mountPromptActions registers the prompt actions.
func mountPromptActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/dismiss_prompt", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			PromptID string `json:"promptId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		prompt, err := core.PostPrompts(
			core.PostPromptWhere.RecipientID.EQ(dbUser.ID),
			core.PostPromptWhere.ID.EQ(input.PromptID),
		).One(c, db)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		prompt.DismissedAt = null.TimeFrom(time.Now())

		if _, err := prompt.Update(c, db, boil.Infer()); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})
}
