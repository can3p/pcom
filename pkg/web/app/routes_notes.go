package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// noteTagsOf splits what a person typed into tags: they are separated by
// spaces or commas.
func noteTagsOf(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}

// noteIDsOf splits the comma-separated ids a selection sends.
func noteIDsOf(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' })
}

// noteFormResult answers a form that saves in place: the status shown next
// to its button, and a page refresh when the list the form belongs to has
// changed.
func noteFormResult(c *gin.Context, err error, ok string, refresh bool) {
	if err != nil {
		c.HTML(http.StatusOK, "partial--error-message.html", gin.H{"Message": "Not saved: " + actionMessage(err)})
		return
	}

	if refresh {
		c.Header("HX-Refresh", "true")
	}

	c.HTML(http.StatusOK, "partial--success-message.html", gin.H{"Message": ok})
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
		body := c.PostForm("body")

		if strings.TrimSpace(body) == "" {
			noteFormResult(c, service.Invalid("body", "Write something first."), "", false)
			return
		}

		_, err := svc.Create(c, auth.GetUserData(c).DBUser, notes.Input{
			Body: body, Tags: noteTagsOf(c.PostForm("tags")), SourceID: c.PostForm("source_id"),
		})

		noteFormResult(c, err, "Saved", true)
	})

	// the edit form: text, tags and the source chosen from the author's own
	// ("" is none)
	controlsForms.POST("/note_edit/:id", requireUUIDParam("id"), func(c *gin.Context) {
		body, tags, sourceID := c.PostForm("body"), noteTagsOf(c.PostForm("tags")), c.PostForm("source_id")

		note, err := svc.Update(c, auth.GetUserData(c).DBUser, c.Param("id"), notes.Edit{Body: &body, Tags: &tags, SourceID: &sourceID})
		if err != nil {
			noteFormResult(c, err, "", false)
			return
		}

		c.HTML(http.StatusOK, "note_saved", gin.H{"Tags": web.NoteTagsOf(note)})
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
		return svc.TagMany(c, u, noteIDsOf(in.IDs), noteTagsOf(in.Tags))
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
// the Add source form, and each source's row, which swaps in its edit form
// and back.
func mountNoteSourceRoutes(svc *notes.Service, controls, controlsForms *gin.RouterGroup) {
	// a source's edit form, in place of its row
	controls.GET("/notes/sources/:id/edit", requireUUIDParam("id"), func(c *gin.Context) {
		src, err := svc.Source(c, auth.GetUserData(c).DBUser, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "partial--note-source-form.html", &web.SourceForm{Source: src, Title: src.Title})
	})

	// a source's row, which Cancel brings back
	controls.GET("/notes/sources/:id", requireUUIDParam("id"), func(c *gin.Context) {
		row, err := web.NoteSourceRow(c, svc, auth.GetUserData(c).DBUser, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "partial--note-source-row.html", row)
	})

	// Add source: the source with the title, found or made; the list shows it
	// after the page reloads
	controlsForms.POST("/note_source_add", func(c *gin.Context) {
		title := c.PostForm("title")

		if _, err := svc.AddSource(c, auth.GetUserData(c).DBUser, title); err != nil {
			sourceFormError(c, &web.SourceForm{Title: title}, err)
			return
		}

		c.Header("HX-Refresh", "true")
		c.HTML(http.StatusOK, "partial--note-source-form.html", &web.SourceForm{Saved: true})
	})

	// a rename answers with the row, saved; renaming into another source's
	// title merges the two, and the page reloads to show the one left
	controlsForms.POST("/note_source_edit/:id", requireUUIDParam("id"), func(c *gin.Context) {
		user, id, title := auth.GetUserData(c).DBUser, c.Param("id"), c.PostForm("title")

		src, err := svc.RenameSource(c, user, id, title)
		if err != nil {
			form := &web.SourceForm{Source: &model.NoteSource{ID: id}, Title: title}
			sourceFormError(c, form, err)

			return
		}

		if src.ID != id {
			c.Header("HX-Refresh", "true")
		}

		row, err := web.NoteSourceRow(c, svc, user, src.ID)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		row.Saved = true
		c.HTML(http.StatusOK, "partial--note-source-row.html", row)
	})
}

// sourceFormError draws a source form again with what went wrong: a title the
// service refused under the field, anything else as its status.
func sourceFormError(c *gin.Context, form *web.SourceForm, err error) {
	if invalid, ok := errors.AsType[*service.ValidationError](err); ok && invalid.Field == "title" {
		form.Error = invalid.Message
	} else {
		form.Failure = "Not saved: " + actionMessage(err)
	}

	c.HTML(http.StatusOK, "partial--note-source-form.html", form)
}
