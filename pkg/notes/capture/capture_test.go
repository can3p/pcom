package capture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

func TestParse_SamplesGolden(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("testdata/samples/*.txt")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	names := [...]string{"plain", "url", "quote"}

	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)

		var out strings.Builder

		for _, how := range []capture.How{capture.Typed, capture.Shared, capture.QuoteShortcut} {
			c := capture.Parse(string(raw), how)

			if c.Kind != capture.URL && how != capture.Typed {
				require.Equal(t, strings.TrimSpace(string(raw)), capture.Unquote(c.Text), "the passage comes back whole")
			}

			out.WriteString(names[c.Kind] + "\n" + c.Text + "\n\n")
		}

		golden.Assert(t, "samples/"+strings.TrimSuffix(filepath.Base(f), ".txt"), []byte(out.String()))
	}
}

func TestParse_Kinds(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in   string
		how  capture.How
		want capture.Kind
		text string
	}{
		"url":             {"  https://example.com/a?b=c#d \n", capture.Typed, capture.URL, "https://example.com/a?b=c#d"},
		"shared url":      {" \n http://example.com \n", capture.Shared, capture.URL, "http://example.com"},
		"url in text":     {"see https://example.com now", capture.Typed, capture.Plain, "see https://example.com now"},
		"two urls":        {"https://a.com https://b.com", capture.Typed, capture.Plain, "https://a.com https://b.com"},
		"other scheme":    {"ftp://example.com", capture.Typed, capture.Plain, "ftp://example.com"},
		"no host":         {"https://", capture.Typed, capture.Plain, "https://"},
		"nbsp inside":     {"https://a.com b", capture.Typed, capture.Plain, "https://a.com b"},
		"url by shortcut": {" \n https://example.com \n", capture.QuoteShortcut, capture.URL, "https://example.com"},
		"quote shortcut":  {"a", capture.QuoteShortcut, capture.Quote, "> a"},
		"empty":           {" \t\n", capture.Typed, capture.Plain, ""},
		"unicode kept":    {"  «Привет, мир»  ", capture.Typed, capture.Plain, "«Привет, мир»"},
		"inner newlines":  {"a\n\n  b  \nc\n", capture.Shared, capture.Plain, "> a\n>\n>   b  \n> c"},
		"emoji at edges":  {"\r\n\t👍 ok 👍\r\n\t", capture.Typed, capture.Plain, "👍 ok 👍"},
		"quote keeps all": {"a\n\nb", capture.QuoteShortcut, capture.Quote, "> a\n>\n> b"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := capture.Parse(tc.in, tc.how)
			require.Equal(t, tc.want, c.Kind)
			require.Equal(t, tc.text, c.Text)
		})
	}
}

func TestAttribution(t *testing.T) {
	t.Parallel()

	for want, s := range map[string]capture.Source{
		"— Author, *Title*, p. 12": {Author: "Author", Title: "Title", Page: "12"},
		"— Author, *Title*":        {Author: " Author ", Title: "Title"},
		"— *Title*, p. 12":         {Title: "Title", Page: "12"},
		"— Author, p. 3-4":         {Author: "Author", Page: "3-4"},
		"— *Title*":                {Title: "Title"},
		"— Author":                 {Author: "Author", Title: " \t "},
		"— p. 12":                  {Page: "12"},
		"":                         {},
	} {
		require.Equal(t, want, capture.Attribution(s))
	}
}

func TestUnquote_RoundTrip(t *testing.T) {
	t.Parallel()

	in := "a\n\n  b\r\n>c\n\n\nd"
	got := capture.Parse("\r\n"+in+"\r\n", capture.Shared).Text
	require.Equal(t, "> a\n>\n>   b\r\n> >c\n>\n>\n> d", got)
	require.Equal(t, in, capture.Unquote(got))
}
