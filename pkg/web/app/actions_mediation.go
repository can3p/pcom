package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/gin-gonic/gin"
)

// mountMediationActions registers the mediation actions.
func mountMediationActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/request_mediation", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID  string `json:"userId"`
			MediationNote string `json:"mediation_note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.RequestMediation(c, db, dbUser.ID, input.TargetUserID, input.MediationNote); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/revoke_mediation_request", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.RevokeMediationRequest(c, db, dbUser.ID, input.TargetUserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/dismiss_mediation", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			RequestID     string `json:"requestId"`
			MediationNote string `json:"mediation_note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DecideForwardMediationRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionMediationDecisionDismissed, input.MediationNote); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/sign_mediation", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			RequestID     string `json:"requestId"`
			MediationNote string `json:"mediation_note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DecideForwardMediationRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionMediationDecisionSigned, input.MediationNote); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})
}
