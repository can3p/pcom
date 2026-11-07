package app

import (
	"net/http"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountNotePickerRoutes registers what the post editor's Notes panel loads
// and the action that records a note as used in the post.
func mountNotePickerRoutes(d *Deps, controls, actions *gin.RouterGroup) {
	svc := d.Services.Notes

	// the panel's fragment: tag bar, source and stack filters, a page of
	// notes; "Load more" gets the notes alone
	controls.GET("/notes/picker", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		page, err := web.NotePicker(c, svc, &userData).Get()
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		name := "partial--note-picker.html"
		if isLoadMore(c) {
			name = "note_picker_list"
		}

		c.HTML(http.StatusOK, name, page)
	})

	actions.POST("/note_mark_used", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		NoteID string `json:"noteId"`
		PostID string `json:"postId"`
	}) error {
		return svc.MarkUsed(c, u, in.NoteID, in.PostID)
	}))
}
