package strikethrough

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
)

func TestStrikethrough(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithExtensions(New()),
	)

	examples := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "double tildes",
			in:   "The text should have ~~strikethrough~~ styling",
			out:  "<p>The text should have <del>strikethrough</del> styling</p>",
		},
		{
			name: "nested emphasis",
			in:   "~~struck **bold**~~",
			out:  "<p><del>struck <strong>bold</strong></del></p>",
		},
		{
			name: "single tildes stay text",
			in:   "It takes ~5 to ~10 minutes",
			out:  "<p>It takes ~5 to ~10 minutes</p>",
		},
		{
			name: "single tilde pair stays text",
			in:   "a ~word~ here",
			out:  "<p>a ~word~ here</p>",
		},
		{
			name: "triple tildes stay text",
			in:   "a ~~~word~~~ here",
			out:  "<p>a ~~~word~~~ here</p>",
		},
		{
			name: "unclosed",
			in:   "a ~~word here",
			out:  "<p>a ~~word here</p>",
		},
		{
			name: "inside code",
			in:   "`~~code~~`",
			out:  "<p><code>~~code~~</code></p>",
		},
	}

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			var writer bytes.Buffer

			require.NoError(t, parser.Convert([]byte(ex.in), &writer))
			require.Equal(t, ex.out, strings.TrimSpace(writer.String()))
		})
	}
}
