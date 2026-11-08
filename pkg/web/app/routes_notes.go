package app

import (
	"net/http"
	"strings"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
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

		_, err := svc.Create(c, auth.GetUserData(c).DBUser, notes.Input{Body: body, Tags: noteTagsOf(c.PostForm("tags")), ApplyFocus: true})

		noteFormResult(c, err, "Saved", true)
	})

	controlsForms.POST("/note_edit/:id", requireUUIDParam("id"), func(c *gin.Context) {
		body, tags := c.PostForm("body"), noteTagsOf(c.PostForm("tags"))

		note, err := svc.Update(c, auth.GetUserData(c).DBUser, c.Param("id"), notes.Edit{Body: &body, Tags: &tags})
		if err != nil {
			noteFormResult(c, err, "", false)
			return
		}

		c.HTML(http.StatusOK, "note_saved", gin.H{"Tags": web.NoteTagsOf(note)})
	})

	// The edit page's Source form: the source with this title and author,
	// found or made, and where in it the note comes from; an empty title
	// takes the source away.
	controlsForms.POST("/note_source/:id", requireUUIDParam("id"), func(c *gin.Context) {
		locator := strings.TrimSpace(c.PostForm("locator"))
		src := notes.SourceInput{Title: c.PostForm("title"), Author: c.PostForm("author")}

		note, err := svc.Update(c, auth.GetUserData(c).DBUser, c.Param("id"), notes.Edit{Locator: &locator, Source: &src})

		msg := "Saved"
		if err == nil && note.SourceID == nil {
			msg = "No source"
		}

		noteFormResult(c, err, msg, false)
	})

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
			c.Header("HX-Redirect", web.NotesPath)
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

	actions.POST("/note_tag", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		NoteID string `json:"noteId"`
		Tag    string `json:"tag"`
	}) error {
		return svc.TagMany(c, u, []string{in.NoteID}, []string{in.Tag})
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
		title := strings.TrimSpace(in.Title)
		if title == "" {
			return svc.SetSource(c, u, noteIDsOf(in.IDs), nil)
		}

		return svc.SetSource(c, u, noteIDsOf(in.IDs), &notes.SourceInput{Kind: model.NoteSourceBook, Title: title})
	}))

	actions.POST("/note_source_rename", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		SourceID string               `json:"sourceId"`
		Title    string               `json:"title"`
		Kind     model.NoteSourceKind `json:"kind"`
		Author   string               `json:"author"`
		URL      string               `json:"url"`
	}) error {
		return svc.RenameSource(c, u, in.SourceID, notes.SourceInput{Kind: in.Kind, Title: in.Title, Author: in.Author, URL: in.URL})
	}))

	actions.POST("/note_source_merge", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		From string `json:"from"`
		Into string `json:"into"`
	}) error {
		if err := svc.MergeSources(c, u, []string{in.From}, in.Into); err != nil {
			return err
		}

		// sent by a form, not by the action controller, which reloads
		c.Header("HX-Refresh", "true")

		return nil
	}))

	actions.POST("/note_focus", jsonAction(d, func(c *gin.Context, u *model.User, in struct {
		Tag string `json:"tag"`
	}) error {
		return svc.SetFocus(c, u, in.Tag)
	}))
}
