package forms

import (
	"context"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/gin-gonic/gin"
)

// noteFormResultTemplate answers a note form next to its button with why it
// didn't save; the form itself stays as typed.
const noteFormResultTemplate = "partial--note-form-result.html"

type NoteFormInput struct {
	Body string `form:"body"`
	// Tags are typed separated by spaces or commas.
	Tags string `form:"tags"`
	// SourceID is one of the author's sources, "" for none.
	SourceID string `form:"source_id"`
}

// NoteForm is the quick capture of the notes page, or the edit form of a note
// when NoteID is set, which saves on every pause in typing.
type NoteForm struct {
	*forms.FormBase[NoteFormInput]
	Notes  *notes.Service
	User   *model.User
	NoteID string
}

func NoteFormNew(svc *notes.Service, u *model.User, noteID string) *NoteForm {
	return &NoteForm{
		FormBase: &forms.FormBase[NoteFormInput]{
			Name:                "note",
			FormTemplate:        noteFormResultTemplate,
			KeepValuesAfterSave: true,
			Input:               &NoteFormInput{},
			ExtraTemplateData:   map[string]any{},
		},
		Notes:  svc,
		User:   u,
		NoteID: noteID,
	}
}

func (f *NoteForm) Validate(c *gin.Context) error {
	// a note may lose its text later, while it has a recording or photos, but
	// a typed one starts with some
	if f.NoteID == "" && strings.TrimSpace(f.Input.Body) == "" {
		f.AddError("body", "Write something first.")
	}

	return f.Errors.PassedValidation()
}

// Save adds the note and reloads the list, or saves the edit and answers
// with "Saved" and the note's tags, which the text may have changed.
func (f *NoteForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if f.NoteID == "" {
		_, err := f.Notes.Create(c, f.User, notes.Input{Body: f.Input.Body, Tags: notes.SplitTags(f.Input.Tags), SourceID: f.Input.SourceID})
		if err != nil {
			return refused(f, err)
		}

		return forms.FormSaveFullReload, nil
	}

	note, err := f.Notes.Update(c, f.User, f.NoteID, notes.Edit{Body: f.Input.Body, Tags: notes.SplitTags(f.Input.Tags), SourceID: f.Input.SourceID})
	if err != nil {
		return refused(f, err)
	}

	return func(c *gin.Context, _ forms.Form) {
		c.HTML(http.StatusOK, "note_saved", map[string]any{"Note": note})
	}, nil
}
