package markdown

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtectedRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		md   string
		want []string // the protected substrings, in order
	}{
		{"prose only", "Just text, ничего особенного.", nil},
		{"code span", "Use `fmt.Println` here", []string{"`fmt.Println`"}},
		{"double backtick span", "a `` x`y `` b", []string{"`` x`y ``"}},
		{"fenced block", "text\n\n```go\nx := 1\n```\n\nafter", []string{"```go\nx := 1\n```"}},
		{"indented block", "text\n\n    code line\n\nafter", []string{"    code line"}},
		{"link text allowed, url protected", "see [the docs](https://example.com/a?b=1 \"T\") now",
			[]string{"https://example.com/a?b=1 \"T\""}},
		{"image destination", "![alt text](/img/pic.png)", []string{"/img/pic.png"}},
		{"nested image in link", "[![a](i.png)](https://x.org)", []string{"i.png", "https://x.org"}},
		{"cyrillic link", "смотри [документацию](https://example.com/док) тут", []string{"https://example.com/док"}},
		{"angle autolink", "mail <https://example.com/x> ok", []string{"<https://example.com/x>"}},
		{"bare url", "go to https://example.com/path now", []string{"https://example.com/path"}},
		{"raw html inline", "a <b>bold</b> c", []string{"<b>", "</b>"}},
		{"html block", "<div>\nhi\n</div>\n\nafter", []string{"<div>", "hi", "</div>"}},
		{"block tags", "{cut Читать дальше}\n\nтекст\n\n{/cut}", []string{"{cut Читать дальше}", "{/cut}"}},
		{"spoiler content allowed", "{spoiler}\ntext inside\n{/spoiler}", []string{"{spoiler}", "{/spoiler}"}},
		{"tag with trailing text", "{spoiler T} tail\nx\n{/spoiler}", []string{"{spoiler T} tail", "{/spoiler}"}},
		{"quoted tag", "> {spoiler}\n> x\n> {/spoiler}", []string{"{spoiler}", "{/spoiler}"}},
		{"list tag", "- {cut}\n\n  x", []string{"{cut}"}},
		{"gallery", "{gallery}\n![a](x.png)\n{/gallery}", []string{"{gallery}", "x.png", "{/gallery}"}},
		{"unknown tag is prose", "{foo}\ntext", nil},
		{"reference definition", "[a][r]\n\n[r]: https://example.com", []string{"[r]: https://example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			for _, r := range ProtectedRanges(tt.md) {
				got = append(got, tt.md[r.Start:r.End])
			}

			require.Equal(t, tt.want, got)
		})
	}
}

func TestRangeOverlaps(t *testing.T) {
	t.Parallel()

	require.True(t, Range{0, 5}.Overlaps(Range{4, 8}))
	require.False(t, Range{0, 5}.Overlaps(Range{5, 8}))
}
