package app

import (
	"net/http"
	"strings"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// noteIDsOf splits the comma-separated ids a selection sends.
func noteIDsOf(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' })
}

// mountNoteRoutes registers the notes pages, the forms that save in place and
// the note actions.
func mountNoteRoutes(d *Deps, controls, controlsForms, actions *gin.RouterGroup) {
	svc := d.Services.Notes

	controls.GET("/notes", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		page := web.Notes(c, svc, &userData)
		if p, err := page.Get(); err == nil && isLoadMore(c) {
			c.HTML(http.StatusOK, "partial--note-list.html", p)
			return
		}

		ginhelpers.HTML(c, "notes.html", page)
	})

	controls.GET("/notes/manage", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "notes.html", web.NotesManagePage(c, svc, &userData))
	})

	controls.GET("/notes/:id/edit", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "edit_note.html", web.EditNote(c, svc, &userData, c.Param("id")))
	})

	controlsForms.POST("/note_create", func(c *gin.Context) {
		gogoForms.DefaultHandler(c, logging.FromContext(c), forms.NoteFormNew(svc, auth.GetUserData(c).DBUser, ""))
	})

	controlsForms.POST("/note_edit/:id", requireUUIDParam("id"), func(c *gin.Context) {
		gogoForms.DefaultHandler(c, logging.FromContext(c), forms.NoteFormNew(svc, auth.GetUserData(c).DBUser, c.Param("id")))
	})

	mountNoteSourceRoutes(svc, controls, controlsForms)

	actions.POST("/note_delete", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		NoteID string `json:"noteId"`
		// Then is where to go after the delete: "list" from the edit page,
		// whose note is gone.
		Then string `json:"then"`
	}) error {
		if err := svc.Delete(c, u, in.NoteID); err != nil {
			return err
		}

		if in.Then == "list" {
			c.Header("HX-Redirect", links.Link("notes"))
		}

		return nil
	}))

	actions.POST("/note_delete_many", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		IDs string `json:"ids"`
	}) error {
		_, err := svc.DeleteMany(c, u, noteIDsOf(in.IDs))

		return err
	}))

	actions.POST("/note_tag_many", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		IDs  string `json:"ids"`
		Tags string `json:"tags"`
	}) error {
		return svc.TagMany(c, u, noteIDsOf(in.IDs), forms.NoteTags(in.Tags))
	}))

	// renaming a tag to one that exists merges them
	actions.POST("/note_tag_rename", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		Tag string `json:"tag"`
		To  string `json:"to"`
	}) error {
		return svc.RenameTag(c, u, in.Tag, in.To)
	}))

	actions.POST("/note_source_set", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		IDs   string `json:"ids"`
		Title string `json:"title"`
	}) error {
		return svc.SetSource(c, u, noteIDsOf(in.IDs), in.Title)
	}))
}

// mountNoteSourceRoutes registers the sources of the Tags and sources screen:
// the Add source form, and each source's row, which swaps in its rename form
// and back.
func mountNoteSourceRoutes(svc *notes.Service, controls, controlsForms *gin.RouterGroup) {
	// a source's rename form, in place of its row
	controls.GET("/notes/sources/:id/edit", requireUUIDParam("id"), func(c *gin.Context) {
		user := auth.GetUserData(c).DBUser

		src, err := svc.Source(c, user, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		forms.NoteSourceFormNew(svc, user, src).RenderForm(c)
	})

	// a source's row, which Cancel brings back
	controls.GET("/notes/sources/:id", requireUUIDParam("id"), func(c *gin.Context) {
		row, err := svc.SourceCount(c, auth.GetUserData(c).DBUser, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, forms.NoteSourceRowTemplate, gin.H{"Source": row.Source, "Notes": row.Notes})
	})

	controlsForms.POST("/note_source_add", func(c *gin.Context) {
		gogoForms.DefaultHandler(c, logging.FromContext(c), forms.NoteSourceFormNew(svc, auth.GetUserData(c).DBUser, nil))
	})

	controlsForms.POST("/note_source_edit/:id", requireUUIDParam("id"), func(c *gin.Context) {
		user := auth.GetUserData(c).DBUser

		// a source that isn't the user's is refused by the rename, as not found
		src := &model.NoteSource{ID: c.Param("id")}
		gogoForms.DefaultHandler(c, logging.FromContext(c), forms.NoteSourceFormNew(svc, user, src))
	})
}
