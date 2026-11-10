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
