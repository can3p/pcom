package web

import (
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestQuoteText_AttributionOfASource(t *testing.T) {
	t.Parallel()

	author, page := "Ann Author", "42"
	blank := "  "
	quote := "> words\n"

	tests := []struct {
		name string
		note model.Note
		want string
	}{
		{"no source", model.Note{Body: quote, Locator: &page}, quote},
		{"title only", model.Note{Body: quote, Source: &model.NoteSource{Title: "A Book"}}, "> words\n\n— *A Book*"},
		{"author, title and page", model.Note{Body: quote, Locator: &page, Source: &model.NoteSource{Title: "A Book", Author: &author}},
			"> words\n\n— Ann Author, *A Book*, p. 42"},
		{"blank pieces are left out", model.Note{Body: "> words", Locator: &blank, Source: &model.NoteSource{Title: "A  Book", Author: &blank}},
			"> words\n\n— *A Book*"},
		{"a source that says nothing", model.Note{Body: quote, Source: &model.NoteSource{Title: " "}}, quote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, quoteText(&tt.note))
		})
	}
}
