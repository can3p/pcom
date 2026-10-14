package mail_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/stretchr/testify/require"
)

// The mail keeps the site's colours: every colour in mail.css names its token
// in a comment, and equals that token of _tokens.scss, the light theme outside
// the dark @media block and the dark theme inside it. The files are read by
// path on purpose, so a move of either breaks this loudly.
func TestMailCSS_MatchesTokens(t *testing.T) {
	t.Parallel()

	css := readFile(t, "templates/mail.css")
	scss := readFile(t, "../../cmd/web/client/scss/_tokens.scss")
	layout := readFile(t, "templates/_layout.html")

	light := tokens(t, scss, ":root {")
	dark := tokens(t, scss, "@mixin dark-tokens {")

	lightCSS, darkCSS, found := strings.Cut(css, "@media (prefers-color-scheme: dark)")
	require.True(t, found, "mail.css has no dark @media block")

	for _, part := range []struct {
		name   string
		css    string
		tokens map[string]string
	}{{"light", lightCSS, light}, {"dark", darkCSS, dark}} {
		named := regexp.MustCompile(`(#[0-9a-fA-F]{3,8});\s*/\*\s*(--[a-z-]+)\s*\*/`).FindAllStringSubmatch(part.css, -1)
		require.NotEmpty(t, named, part.name)
		require.Len(t, regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`).FindAllString(part.css, -1), len(named),
			"every %s colour in mail.css names its token in a comment", part.name)

		// a colour written any other way escapes the token check
		require.NotRegexp(t, `(?i)(rgba?|hsla?)\(`, part.css, "%s colours are annotated hex", part.name)
		require.NotRegexp(t, `(?i)(color|background|border[a-z-]*)\s*:[^;]*\b(white|black|red|green|blue|gray|grey|silver|transparent)\b`,
			part.css, "%s colours are annotated hex, not named", part.name)

		for _, m := range named {
			require.Contains(t, part.tokens, m[2], "%s token %s", part.name, m[2])
			require.Equal(t, strings.ToLower(part.tokens[m[2]]), strings.ToLower(m[1]), "%s token %s", part.name, m[2])
		}
	}

	// the layout's bgcolor fallbacks for Outlook
	require.Contains(t, layout, `bgcolor="`+light["--ground"]+`"`)
	require.Contains(t, layout, `bgcolor="`+light["--surface"]+`"`)
}

// Every mail renders inside the layout: a full document whose footer links to
// the site, whose dark mode rules survive inlining, and whose classes all have
// a rule in mail.css, since inlining drops a class without one silently.
func TestMails_ClassesHaveRules(t *testing.T) {
	t.Parallel()

	// only the rules outside the dark block are inlined
	css, _, found := strings.Cut(readFile(t, "templates/mail.css"), "@media (prefers-color-scheme: dark)")
	require.True(t, found)

	for _, m := range mail.All() {
		for _, sample := range m.Samples {
			t.Run(sample.Name, func(t *testing.T) {
				t.Parallel()

				require.NoError(t, sample.Err)

				html := sample.Envelope.Mail.Html
				require.True(t, strings.HasPrefix(html, "<!DOCTYPE html>"), "starts with the doctype")
				require.True(t, strings.HasSuffix(sample.Envelope.Mail.Text, "\n\n--\npcom · "+mail.SampleSite.Root),
					"the text part ends in the shared footer")

				doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
				require.NoError(t, err)

				style := doc.Find("head style").Text()
				require.Contains(t, style, "@media (prefers-color-scheme: dark)")
				require.Contains(t, style, "#1f1c17", "dark --ground survives inlining")
				require.Contains(t, style, "#c5d38a", "dark --accent survives inlining")
				require.Positive(t, doc.Find(`.mail-footer a[href="`+mail.SampleSite.Root+`"]`).Length(), "footer links to the site")

				doc.Find("[class]").Each(func(_ int, s *goquery.Selection) {
					for c := range strings.FieldsSeq(s.AttrOr("class", "")) {
						require.Regexp(t, `\.`+regexp.QuoteMeta(c)+`([^\w-]|$)`, css, "class %s has no rule in mail.css", c)
					}
				})
			})
		}
	}
}

// The footer says why the reader gets the mail: an invitation or a waiting
// list confirmation goes to an address somebody entered, every other mail to
// an account.
func TestMails_FooterReason(t *testing.T) {
	t.Parallel()

	const (
		entered = "You get this mail because somebody entered this address at pcom."
		account = "You get this mail because you have an account at pcom."
	)

	noAccount := map[string]bool{"send_actual_invitation": true, "send_actual_confirm_waiting_list": true}

	for _, m := range mail.All() {
		for _, sample := range m.Samples {
			require.NoError(t, sample.Err)

			want, other := account, entered
			if noAccount[sample.Name] {
				want, other = entered, account
			}

			html := sample.Envelope.Mail.Html
			require.Contains(t, html, want, sample.Name)
			require.NotContains(t, html, other, sample.Name)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(b)
}

// tokens are the colour tokens (--name: #hex) of the block of scss that
// starts with opening and ends at the first closing brace.
func tokens(t *testing.T, scss, opening string) map[string]string {
	t.Helper()

	_, block, found := strings.Cut(scss, opening)
	require.True(t, found, "no %q in _tokens.scss", opening)
	block, _, _ = strings.Cut(block, "}")

	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(--[a-z-]+):\s*(#[0-9a-fA-F]{3,8});`).FindAllStringSubmatch(block, -1) {
		out[m[1]] = m[2]
	}

	require.NotEmpty(t, out, opening)

	return out
}
