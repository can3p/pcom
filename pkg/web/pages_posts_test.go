package web

import (
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestQuoteText_AttributionOfASource(t *testing.T) {
	t.Parallel()

	quote := "> words\n"

	tests := []struct {
		name string
		note model.Note
		want string
	}{
		{"no source", model.Note{Body: quote}, quote},
		{"the title", model.Note{Body: quote, Source: &model.NoteSource{Title: "A Book"}}, "> words\n\n— *A Book*"},
		{"spacing collapsed", model.Note{Body: "> words", Source: &model.NoteSource{Title: " A  Book "}}, "> words\n\n— *A Book*"},
		{"a source that says nothing", model.Note{Body: quote, Source: &model.NoteSource{Title: " "}}, quote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, quoteText(&tt.note))
		})
	}
}

func TestScratchInput(t *testing.T) {
	t.Parallel()

	typed := scratchInput(&model.PostScratch{
		Subject: new("S"), URL: new("u"), Body: "B", Visibility: model.PostVisibilityPublic,
		NotifySubscribers: new(false), NoteIDs: []string{"a", "b"}, Revision: 3,
	})
	require.Equal(t, "S", typed.Subject)
	require.Equal(t, "u", typed.URL)
	require.Equal(t, model.PostVisibilityPublic, typed.Visibility)
	require.True(t, typed.NotifyOffered)
	require.False(t, typed.NotifySubscribers)
	require.Equal(t, "a,b", typed.NoteIDs)
	require.Equal(t, 3, typed.ScratchRev)

	// an emptied pad shows the form's defaults and keeps the revision
	emptied := scratchInput(&model.PostScratch{Visibility: model.PostVisibilityPublic, Revision: 5})
	require.Equal(t, 5, emptied.ScratchRev)
	require.Equal(t, model.PostVisibilitySecondDegree, emptied.Visibility)
	require.Empty(t, emptied.Body)
}
