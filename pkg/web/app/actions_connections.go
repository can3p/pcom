package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/gin-gonic/gin"
)

// mountConnectionActions registers the connection actions.
func mountConnectionActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/remove_from_whitelist", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		UserID string `json:"userId"`
	}) error {
		if err := userops.DropConnectionGrant(c, db, dbUser.ID, input.UserID); err != nil {
			return userError(fmt.Sprintf("Failed operation: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/create_connection", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		TargetUserID string `json:"userId"`
	}) error {
		if err := userops.EstablishConnection(c, db, dbUser.ID, input.TargetUserID); err != nil {
			return userError(fmt.Sprintf("Failed operation: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/drop_connection", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		TargetUserID string `json:"userId"`
	}) error {
		if err := userops.DropConnection(c, db, dbUser.ID, input.TargetUserID); err != nil {
			return userError(fmt.Sprintf("Failed operation: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/reject_connection", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		RequestID string `json:"requestId"`
		Note      string `json:"note"`
	}) error {
		if err := userops.DecideConnectionRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionRequestDecisionDismissed, input.Note); err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/accept_connection", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		RequestID string `json:"requestId"`
		Note      string `json:"note"`
	}) error {
		if err := userops.DecideConnectionRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionRequestDecisionApproved, input.Note); err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
