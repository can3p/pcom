package forms

import (
	"context"
	"net/http"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/gin-gonic/gin"
)

type NoteSourceFormInput struct {
	Title string `form:"title"`
}

// NoteSourceForm is the Add source form of the Tags and sources screen, or
// the rename form that takes the place of a source's row when Source is set.
type NoteSourceForm struct {
	*forms.FormBase[NoteSourceFormInput]
	Notes  *notes.Service
	User   *model.User
	Source *model.NoteSource
}

// NoteSourceFormNew builds the form; a rename form starts with the source's
// title.
func NoteSourceFormNew(svc *notes.Service, u *model.User, src *model.NoteSource) *NoteSourceForm {
	in := &NoteSourceFormInput{}
	if src != nil {
		in.Title = src.Title
	}

	return &NoteSourceForm{
		FormBase: &forms.FormBase[NoteSourceFormInput]{
			Name:                "note_source",
			FormTemplate:        "partial--note-source-form.html",
			KeepValuesAfterSave: true,
			Input:               in,
			ExtraTemplateData:   map[string]any{"Source": src},
		},
		Notes:  svc,
		User:   u,
		Source: src,
	}
}

// Validate leaves the title to the service, which collapses its whitespace
// before it checks it.
func (f *NoteSourceForm) Validate(c *gin.Context) error {
	return nil
}

// Save adds the source and reloads the screen to list it, or renames it and
// answers with its row, saved. Renaming into another source's title merges
// the two, and the screen reloads to show the one left.
func (f *NoteSourceForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if f.Source == nil {
		if _, err := f.Notes.AddSource(c, f.User, f.Input.Title); err != nil {
			return refused(f, err)
		}

		return forms.FormSaveFullReload, nil
	}

	src, err := f.Notes.RenameSource(c, f.User, f.Source.ID, f.Input.Title)
	if err != nil {
		return refused(f, err)
	}

	if src.ID != f.Source.ID {
		return forms.FormSaveFullReload, nil
	}

	row, err := f.Notes.SourceCount(c, f.User, src.ID)
	if err != nil {
		return nil, err
	}

	return func(c *gin.Context, _ forms.Form) {
		c.HTML(http.StatusOK, NoteSourceRowTemplate, map[string]any{"Source": row.Source, "Notes": row.Notes, "Saved": true})
	}, nil
}

// NoteSourceRowTemplate is a source's row on the Tags and sources screen.
const NoteSourceRowTemplate = "partial--note-source-row.html"
