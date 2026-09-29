package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/gin-gonic/gin"
)

// mountMediationActions registers the mediation actions.
func mountMediationActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/request_mediation", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		TargetUserID  string `json:"userId"`
		MediationNote string `json:"mediation_note"`
	}) error {
		if err := userops.RequestMediation(c, db, dbUser.ID, input.TargetUserID, input.MediationNote); err != nil {
			return userError(fmt.Sprintf("Failed operation: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/revoke_mediation_request", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		TargetUserID string `json:"userId"`
	}) error {
		if err := userops.RevokeMediationRequest(c, db, dbUser.ID, input.TargetUserID); err != nil {
			return userError(fmt.Sprintf("Failed operation: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/dismiss_mediation", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		RequestID     string `json:"requestId"`
		MediationNote string `json:"mediation_note"`
	}) error {
		if err := userops.DecideForwardMediationRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionMediationDecisionDismissed, input.MediationNote); err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/sign_mediation", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		RequestID     string `json:"requestId"`
		MediationNote string `json:"mediation_note"`
	}) error {
		if err := userops.DecideForwardMediationRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionMediationDecisionSigned, input.MediationNote); err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
