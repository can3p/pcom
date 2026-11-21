package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/gin-gonic/gin"
)

type (
	MediationRequest  = connections.MediationRequest
	MediationResult   = connections.MediationResult
	ConnectionRequest = connections.ConnectionRequest
	Draft             = connections.Draft
	NewPost           = connections.NewPost
)

type ControlsPage struct {
	*BasePage
	DirectConnections       []*model.User
	SecondDegreeConnections []*model.User
	WhitelistedConnections  []*model.User
	MediationRequests       []*MediationRequest
	ConnectionRequests      []*ConnectionRequest
	Drafts                  []*Draft
	NewPosts                []*NewPost                // scratch pads with text: new posts not saved as drafts yet
	Subscribers             *subscriptions.AuthorView // the "Email subscribers" section
}

// Controls builds the controls page from what the connections service gathered.
func Controls(ctx *gin.Context, userData *auth.UserData, c *connections.Controls, subscribers *subscriptions.AuthorView) *ControlsPage {
	return &ControlsPage{
		BasePage:                getBasePage(ctx, "Controls", userData),
		DirectConnections:       c.DirectConnections,
		SecondDegreeConnections: c.SecondDegreeConnections,
		WhitelistedConnections:  c.WhitelistedConnections,
		ConnectionRequests:      c.ConnectionRequests,
		MediationRequests:       c.MediationRequests,
		Drafts:                  c.Drafts,
		NewPosts:                c.NewPosts,
		Subscribers:             subscribers,
	}
}
