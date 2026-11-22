package app

import (
	"net/http"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountPostNoteRoutes registers the Notes section of a post page. Only the
// post's author gets it: anybody else, a reader of the post included, is told
// there is no such page.
func mountPostNoteRoutes(d *Deps, r *gin.RouterGroup) {
	r.GET("/posts/:id/notes", auth.EnforceAuth, requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)

		page, err := web.PostNotes(c, d.Services.Notes, &userData, c.Param("id"), c.Query("tag")).Get()
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "partial--post-notes.html", page)
	})
}

// mountPostNoteActions registers the actions that attach notes to a post and
// take them off; each is the author's alone.
func mountPostNoteActions(d *Deps, actions *gin.RouterGroup) {
	svc := d.Services.Notes

	actions.POST("/note_attach", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		NoteID string `json:"noteId"`
		PostID string `json:"postId"`
	}) error {
		return svc.Attach(c, u, in.NoteID, in.PostID)
	}))

	actions.POST("/note_detach", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		NoteID string `json:"noteId"`
		PostID string `json:"postId"`
	}) error {
		return svc.Detach(c, u, in.NoteID, in.PostID)
	}))

	// hides a note that a tag attached to the post brings, without untagging it
	actions.POST("/note_exclude", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		NoteID string `json:"noteId"`
		PostID string `json:"postId"`
	}) error {
		return svc.Exclude(c, u, in.NoteID, in.PostID)
	}))

	actions.POST("/note_attach_tag", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		PostID string `json:"postId"`
		Tag    string `json:"tag"`
	}) error {
		_, err := svc.AttachTag(c, u, in.PostID, in.Tag)

		return err
	}))

	actions.POST("/note_detach_tag", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		PostID string `json:"postId"`
		Tag    string `json:"tag"`
	}) error {
		return svc.DetachTag(c, u, in.PostID, in.Tag)
	}))
}
