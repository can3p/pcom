package app

import (
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/proofread"
	"github.com/gin-gonic/gin"
)

// mountProofreadRoutes registers the grammar check and the style review. The
// routes exist whether or not the feature is on; with it off every one
// answers 404.
func mountProofreadRoutes(d *Deps, controls, actions *gin.RouterGroup) {
	svc := d.Services.Proofread

	controls.GET("/proofread_style/:post_id", proofreadOnly(svc, func(c *gin.Context, u *model.User) (any, error) {
		return svc.StyleState(c, u, c.Param("post_id"))
	}))

	actions.POST("/proofread_grammar", proofreadJSON(svc, func(c *gin.Context, u *model.User, in struct {
		PostID  string `json:"post_id"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}) (any, error) {
		return svc.CheckGrammar(c, u, proofread.GrammarRequest{PostID: in.PostID, Subject: in.Subject, Body: in.Body})
	}))

	actions.POST("/proofread_style", proofreadJSON(svc, func(c *gin.Context, u *model.User, in struct {
		PostID     string `json:"post_id"`
		Subject    string `json:"subject"`
		Body       string `json:"body"`
		AuthorNote string `json:"author_note"`
	}) (any, error) {
		return svc.ReviewStyle(c, u, proofread.StyleRequest{PostID: in.PostID, Subject: in.Subject, Body: in.Body, AuthorNote: in.AuthorNote})
	}))

	actions.POST("/proofread_style_note", proofreadJSON(svc, func(c *gin.Context, u *model.User, in struct {
		NoteID string `json:"note_id"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}) (any, error) {
		return gin.H{}, svc.SetNoteStatus(c, u, in.NoteID, model.StyleNoteStatus(in.Status), in.Reason)
	}))

	actions.POST("/proofread_style_reset", proofreadJSON(svc, func(c *gin.Context, u *model.User, in struct {
		PostID string `json:"post_id"`
	}) (any, error) {
		return gin.H{}, svc.ResetStyle(c, u, in.PostID)
	}))
}

// proofreadOnly answers 404 while the feature is off, and otherwise runs fn
// for the logged-in user and answers with its result as JSON.
func proofreadOnly(svc *proofread.Service, fn func(c *gin.Context, u *model.User) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !svc.Enabled() {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		out, err := fn(c, auth.GetUserData(c).DBUser)
		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		c.JSON(http.StatusOK, out)
	}
}

// proofreadJSON is proofreadOnly for a route with a JSON body bound into T.
func proofreadJSON[T any](svc *proofread.Service, fn func(c *gin.Context, u *model.User, in T) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !svc.Enabled() {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		var in T

		if err := c.BindJSON(&in); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		out, err := fn(c, auth.GetUserData(c).DBUser, in)
		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		c.JSON(http.StatusOK, out)
	}
}
