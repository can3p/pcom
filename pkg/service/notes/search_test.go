package notes_test

import (
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/stretchr/testify/require"
)

func TestParseSearch(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", notes.DefaultTagMaxLength)

	rows := []struct {
		name, q   string
		want      notes.Search
		canonical string
	}{
		{"empty", "", notes.Search{}, ""},
		{"spaces only", "   \t ", notes.Search{}, ""},
		{"text", "  the   quick fox ", notes.Search{Query: "the quick fox"}, "the quick fox"},
		{"tags, not-tags and text", "fox #Books -#todo quick", notes.Search{Tags: []string{"books"}, NotTags: []string{"todo"}, Query: "fox quick"}, "#books -#todo fox quick"},
		{"unicode tags", "#Ёлки -#Café", notes.Search{Tags: []string{"ёлки"}, NotTags: []string{"café"}}, "#ёлки -#café"},
		{"dashed tag", "#to-read", notes.Search{Tags: []string{"to-read"}}, "#to-read"},
		{"duplicates", "#a #A -#b -#B x", notes.Search{Tags: []string{"a"}, NotTags: []string{"b"}, Query: "x"}, "#a -#b x"},
		{"hash alone is text", "# x", notes.Search{Query: "# x"}, "# x"},
		{"minus hash alone is text", "-# x", notes.Search{Query: "-# x"}, "-# x"},
		{"digits only is text", "#2024", notes.Search{Query: "#2024"}, "#2024"},
		{"punctuation is text", "#a! ##b #-c", notes.Search{Query: "#a! ##b #-c"}, "#a! ##b #-c"},
		{"longest tag", "#" + long, notes.Search{Tags: []string{long}}, "#" + long},
		{"too long tag is text", "#" + long + "a", notes.Search{Query: "#" + long + "a"}, "#" + long + "a"},
		{"exclude wins", "#a -#a #b", notes.Search{Tags: []string{"b"}, NotTags: []string{"a"}}, "#b -#a"},
		{"exclude wins whatever the order", "-#a #a", notes.Search{NotTags: []string{"a"}}, "-#a"},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			got := notes.ParseSearch(r.q)
			require.Equal(t, r.want, got)
			require.Equal(t, r.canonical, got.String())
			require.Equal(t, got, notes.ParseSearch(got.String()))
			require.Equal(t, r.canonical == "", got.IsZero())
		})
	}
}

func TestSearch_Edits(t *testing.T) {
	t.Parallel()

	s := notes.ParseSearch("#a #b -#c -#d some text")

	rows := []struct {
		name string
		got  notes.Search
		want string
	}{
		{"without tag", s.WithoutTag("a"), "#b -#c -#d some text"},
		{"without missing tag", s.WithoutTag("x"), "#a #b -#c -#d some text"},
		{"without not-tag", s.WithoutNotTag("d"), "#a #b -#c some text"},
		{"without query", s.WithoutQuery(), "#a #b -#c -#d"},
		{"with tag", s.WithTag("e"), "#a #b #e -#c -#d some text"},
		{"with tag it has", s.WithTag("a"), "#a #b -#c -#d some text"},
		{"with excluded tag includes it", s.WithTag("c"), "#a #b #c -#d some text"},
		{"empty", notes.Search{}.WithTag("a"), "#a"},
	}

	for _, r := range rows {
		require.Equal(t, r.want, r.got.String(), r.name)
	}

	require.Equal(t, "#a #b -#c -#d some text", s.String(), "edits leave the search they start from unchanged")
}
