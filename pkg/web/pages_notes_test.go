package web_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/web"
	"github.com/stretchr/testify/require"
)

func TestNotes_DeletePrompt(t *testing.T) {
	t.Parallel()

	audio := "upload"
	photos := func(n int) []*model.NoteImage { return make([]*model.NoteImage, n) }

	tests := []struct {
		name string
		note model.Note
		used bool
		want string
	}{
		{"plain", model.Note{}, false, "Delete this note? This can't be undone."},
		{"recording", model.Note{AudioUploadID: &audio}, false, "Delete this note and its recording? This can't be undone."},
		{"photo", model.Note{Images: photos(1)}, false, "Delete this note and 1 photo? This can't be undone."},
		{"recording and photos", model.Note{AudioUploadID: &audio, Images: photos(2)}, false,
			"Delete this note, its recording and 2 photos? This can't be undone."},
		{"used in posts", model.Note{}, true,
			"Delete this note? This can't be undone. The text already inserted into posts stays there."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, web.NoteDeletePrompt(&tt.note, tt.used))
		})
	}
}

func TestNotes_QueryCyclesATagThroughIncludeExcludeOff(t *testing.T) {
	t.Parallel()

	q := web.NoteQuery{}.Cycle("essay")
	require.Equal(t, []string{"essay"}, q.Tags)
	require.Equal(t, "/controls/notes?tag=essay", q.URL(web.NotesPath))

	q = q.Cycle("essay")
	require.Empty(t, q.Tags)
	require.Equal(t, []string{"essay"}, q.Not)

	q = q.Cycle("essay")
	require.Equal(t, web.NotesPath, q.URL(web.NotesPath))
}
