package app

import (
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
)

// mountMediationActions registers the mediation actions.
func mountMediationActions(d *Deps, r *gin.RouterGroup) {
	conns := d.Services.Connections

	r.POST("/request_mediation", jsonAction(d, func(c *gin.Context, u *core.User, in struct {
		TargetUserID  string `json:"userId"`
		MediationNote string `json:"mediation_note"`
	}) error {
		return failedAction("Failed operation", conns.RequestMediation(c, u, in.TargetUserID, in.MediationNote))
	}))

	r.POST("/revoke_mediation_request", jsonAction(d, func(c *gin.Context, u *core.User, in struct {
		TargetUserID string `json:"userId"`
	}) error {
		return failedAction("Failed operation", conns.RevokeMediation(c, u, in.TargetUserID))
	}))

	type decision struct {
		RequestID     string `json:"requestId"`
		MediationNote string `json:"mediation_note"`
	}

	r.POST("/dismiss_mediation", jsonAction(d, func(c *gin.Context, u *core.User, in decision) error {
		return failedAction("Operation Failed", conns.DecideMediation(c, u, in.RequestID, core.ConnectionMediationDecisionDismissed, in.MediationNote))
	}))

	r.POST("/sign_mediation", jsonAction(d, func(c *gin.Context, u *core.User, in decision) error {
		return failedAction("Operation Failed", conns.DecideMediation(c, u, in.RequestID, core.ConnectionMediationDecisionSigned, in.MediationNote))
	}))
}
