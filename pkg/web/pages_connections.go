package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/gin-gonic/gin"
)

type (
	MediationRequest  = connections.MediationRequest
	MediationResult   = connections.MediationResult
	ConnectionRequest = connections.ConnectionRequest
	Draft             = connections.Draft
)

type ControlsPage struct {
	*BasePage
	DirectConnections       []*model.User
	SecondDegreeConnections []*model.User
	WhitelistedConnections  []*model.User
	MediationRequests       []*MediationRequest
	ConnectionRequests      []*ConnectionRequest
	Drafts                  []*Draft
}

// Controls builds the controls page from what the connections service gathered.
func Controls(ctx *gin.Context, userData *auth.UserData, c *connections.Controls) *ControlsPage {
	return &ControlsPage{
		BasePage:                getBasePage(ctx, "Controls", userData),
		DirectConnections:       c.DirectConnections,
		SecondDegreeConnections: c.SecondDegreeConnections,
		WhitelistedConnections:  c.WhitelistedConnections,
		ConnectionRequests:      c.ConnectionRequests,
		MediationRequests:       c.MediationRequests,
		Drafts:                  c.Drafts,
	}
}
