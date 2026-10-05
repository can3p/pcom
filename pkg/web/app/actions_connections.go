package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/model"
	"github.com/gin-gonic/gin"
)

// failedAction shows a service error the way the connection actions always
// did: the reason behind a prefix that names the action.
func failedAction(prefix string, err error) error {
	if err == nil {
		return nil
	}

	return userError(fmt.Sprintf("%s: %s", prefix, actionMessage(err)))
}

// mountConnectionActions registers the connection actions.
func mountConnectionActions(d *Deps, r *gin.RouterGroup) {
	conns := d.Services.Connections

	r.POST("/remove_from_whitelist", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		UserID string `json:"userId"`
	}) error {
		return failedAction("Failed operation", conns.RemoveFromWhitelist(c, u, in.UserID))
	}))

	r.POST("/create_connection", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		TargetUserID string `json:"userId"`
	}) error {
		return failedAction("Failed operation", conns.Connect(c, u, in.TargetUserID))
	}))

	r.POST("/drop_connection", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		TargetUserID string `json:"userId"`
	}) error {
		return failedAction("Failed operation", conns.Drop(c, u, in.TargetUserID))
	}))

	type decision struct {
		RequestID string `json:"requestId"`
		Note      string `json:"note"`
	}

	r.POST("/reject_connection", jsonAction(d, func(c *gin.Context, u *model.User, in decision) error {
		return failedAction("Operation Failed", conns.DecideRequest(c, u, in.RequestID, model.ConnectionRequestDecisionDismissed, in.Note))
	}))

	r.POST("/accept_connection", jsonAction(d, func(c *gin.Context, u *model.User, in decision) error {
		return failedAction("Operation Failed", conns.DecideRequest(c, u, in.RequestID, model.ConnectionRequestDecisionApproved, in.Note))
	}))
}
