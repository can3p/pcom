package markdown_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func isProtected(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Data != "span" {
		return false
	}

	for _, a := range n.Attr {
		if a.Key == "class" && a.Val == "notranslate" {
			return true
		}
	}

	return false
}

// transform parses a segment, lets f edit the nodes and renders them again.
func transform(t *testing.T, seg string, f func(nodes []*html.Node)) string {
	t.Helper()

	nodes, err := markdown.ParseSegment(seg)
	require.NoError(t, err)

	f(nodes)

	var sb strings.Builder
	for _, n := range nodes {
		require.NoError(t, html.Render(&sb, n))
	}

	return sb.String()
}

// shout is a fake translation: it uppercases the text a backend would
// translate and leaves tags and notranslate spans alone.
func shout(t *testing.T, seg string) string {
	var up func(n *html.Node)

	up = func(n *html.Node) {
		if n.Type == html.TextNode {
			n.Data = strings.ToUpper(n.Data)
		}

		if isProtected(n) {
			return
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			up(c)
		}
	}

	return transform(t, seg, func(nodes []*html.Node) {
		for _, n := range nodes {
			up(n)
		}
	})
}

// reverse reverses the order of the top-level elements of a segment, as a
// backend moving words around would, keeping the text where it is.
func reverse(t *testing.T, seg string) string {
	return transform(t, seg, func(nodes []*html.Node) {
		var els []*html.Node

		for _, n := range nodes {
			if n.Type == html.ElementNode {
				els = append(els, n)
			}
		}

		// swap the contents of the elements' slots: copy then reassign
		cp := make([]html.Node, len(els))
		for i, e := range els {
			cp[i] = *e
		}

		for i, e := range els {
			src := cp[len(els)-1-i]
			e.Data, e.DataAtom, e.Attr, e.FirstChild, e.LastChild = src.Data, src.DataAtom, src.Attr, src.FirstChild, src.LastChild
		}
	})
}

// rerender writes the same segment with other bytes: attributes reversed,
// <i>/<b> for <em>/<strong>, <br/>.
func rerender(t *testing.T, seg string) string {
	var walk func(n *html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			slicesReverse(n.Attr)

			switch n.Data {
			case "em":
				n.Data = "i"
			case "strong":
				n.Data = "b"
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	return transform(t, seg, func(nodes []*html.Node) {
		for _, n := range nodes {
			walk(n)
		}
	})
}

func slicesReverse(a []html.Attribute) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

var translateCases = []struct{ name, src string }{
	{"paragraph_emphasis_link", "Hello *big* and **bold** world, see [a *fine* page](https://example.com/x \"Title\") now.\n\nSecond para\nwith a soft break and a hard break\\\nhere.\n"},
	{"headings", "# Title here\n\nSetext heading\n-------------\n\nbody\n"},
	{"list_code_span", "- first `code()` item\n- second item with @some_user handle\n  continued line\n\n1. one thing\n2. other thing\n"},
	{"handle_url", "Ping @alice_b about https://example.com/a?b=1 or <https://example.org> today.\n"},
	{"gallery_and_code", "Before text\n\n{gallery}\n![kept](http://x/a.png)\n![also](http://x/b.png)\n{/gallery}\n\n```go\nfmt.Println(\"no\")\n```\n\nAfter text\n"},
	{"cut_and_blockquote", "{cut more}\ninside the cut\n{/cut}\n\n> quoted words\n> second quoted line\n"},
	{"table", "| Name | Role |\n|------|------|\n| *Ann* | `admin` user |\n| Bob | plain \\| pipe |\n"},
	{"image_alt", "Look ![a red apple](http://x/apple.png \"On a tree\") here.\n\n![only alt text](http://x/only.png)\n\n![](http://x/a.png)\n"},
	{"raw_html_and_escapes", "Some <b>raw</b> html and 1 \\* 2 \\[x\\] AT&T more\n\n<div>block html</div>\n\n---\n\nplain\n"},
	{"nothing_to_translate", "`code`\n\nhttps://example.com\n\n![](http://x/a.png)\n"},
	{"nested_lists", "- top *one*\n  - nested two\n    - deep three\n- top four\n\n1. first\n   - sub of first\n2. second\n"},
	{"nested_blockquotes", "> outer words\n>\n> > inner words\n> > more inner\n>\n> back outside\n"},
	{"list_in_blockquote", "> intro line\n>\n> - item one\n>   continued here\n> - item **two**\n"},
	{"video_embed", "Watch this\n\nhttps://www.youtube.com/watch?v=dQw4w9WgXcQ\n\nand then that\n"},
}

func TestSegmentsGolden(t *testing.T) {
	for _, tc := range translateCases {
		t.Run(tc.name, func(t *testing.T) {
			segs, err := markdown.Segments(tc.src)
			require.NoError(t, err)

			var texts, same, up []string
			for _, s := range segs {
				texts = append(texts, s.Text)
				same = append(same, rerender(t, s.Text))
				up = append(up, shout(t, s.Text))
			}

			// identity round trip: other bytes, same meaning, same source
			got, err := markdown.Apply(tc.src, segs, same)
			require.NoError(t, err)
			assert.Equal(t, tc.src, got)

			// the fake translation keeps every protected span
			require.NoError(t, translate.CheckShape(texts, up))

			got, err = markdown.Apply(tc.src, segs, up)
			require.NoError(t, err)

			// applied markdown is segmented the same way again
			again, err := markdown.Segments(got)
			require.NoError(t, err)
			assert.Len(t, again, len(segs))

			var sb strings.Builder
			fmt.Fprintf(&sb, "--- source\n%s--- segments\n", tc.src)

			for i, s := range segs {
				fmt.Fprintf(&sb, "[%d] %q\n", i, s.Text)
			}

			fmt.Fprintf(&sb, "--- applied\n%s", got)
			golden.Assert(t, "translate/"+tc.name, []byte(sb.String()))
		})
	}
}

func TestReorderGolden(t *testing.T) {
	src := "A *one* then [two](http://a.test) and `x` plus **three** at [four](http://b.test \"T\").\n\nWith @hana and <https://example.org> then ![alt text](http://i.test/a.png) and [l2](http://l2.test).\n"

	segs, err := markdown.Segments(src)
	require.NoError(t, err)

	var texts, rev []string
	for _, s := range segs {
		texts = append(texts, s.Text)
		rev = append(rev, reverse(t, s.Text))
	}

	require.NoError(t, translate.CheckShape(texts, rev))

	got, err := markdown.Apply(src, segs, rev)
	require.NoError(t, err)

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- source\n%s--- segments\n", src)

	for i := range segs {
		fmt.Fprintf(&sb, "[%d] %q\n    %q\n", i, texts[i], rev[i])
	}

	fmt.Fprintf(&sb, "--- applied\n%s", got)
	golden.Assert(t, "translate/reorder", []byte(sb.String()))
}

func TestApplyRestoresByIndex(t *testing.T) {
	src := "See [one](http://a.test) and [two](http://b.test) and `x`.\n"
	segs, err := markdown.Segments(src)
	require.NoError(t, err)
	require.Len(t, segs, 1)

	got, err := markdown.Apply(src, segs, []string{
		`<a data-i="1">dos</a> y <a data-i="0">uno</a> y <span class="notranslate" data-i="2">x</span> (a &amp; b, 1. * _)`,
	})
	require.NoError(t, err)
	assert.Equal(t, "[dos](http://b.test) y [uno](http://a.test) y `x` (a & b, 1. \\* \\_)\n", got)
}

func TestApplyDropsUnknownTags(t *testing.T) {
	src := "A [link](http://a.test) here `c`\n"
	segs, err := markdown.Segments(src)
	require.NoError(t, err)

	span := ` <span class="notranslate" data-i="1">c</span>`

	for tr, want := range map[string]string{
		`<a>x</a>` + span:                   "x `c`\n",
		`<div>x <u>y</u></div>` + span:      "x y `c`\n",
		`<span>x</span>` + span:             "x `c`\n",
		`<b>x</b> <i>y</i> <br>z` + span:    "**x** *y* \\\nz `c`\n",
		`<a data-i="0"><u>x</u></a>` + span: "[x](http://a.test) `c`\n",
	} {
		got, err := markdown.Apply(src, segs, []string{tr})
		require.NoError(t, err, tr)
		assert.Equal(t, want, got, tr)
	}
}

func TestApplyErrors(t *testing.T) {
	src := "A [link](http://a.test) here `c`\n"
	segs, err := markdown.Segments(src)
	require.NoError(t, err)

	link := `<a data-i="0">x</a>`
	span := `<span class="notranslate" data-i="1">c</span>`

	for name, tr := range map[string]string{
		"unclosed tag":     `<em>` + link + " " + span,
		"stray close":      `</em>` + link + " " + span,
		"unknown index":    link + `<a data-i="9">y</a> ` + span,
		"unknown span":     link + " " + span + `<span class="notranslate" data-i="7">y</span>`,
		"bad index":        link + `<a data-i="z">y</a> ` + span,
		"dropped span":     link,
		"link as span":     `<span class="notranslate" data-i="0">x</span> ` + span,
		"span as link":     link + ` <a data-i="1">c</a>`,
		"plain span":       link + ` <span data-i="1">c</span>`,
		"used twice":       link + `<a data-i="0">y</a> ` + span,
		"span used twice":  link + " " + span + span,
		"comment":          `<!-- x -->` + link + " " + span,
		"link tag changed": `<em data-i="0">x</em> ` + span,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := markdown.Apply(src, segs, []string{tr})
			require.Error(t, err)
			assert.Empty(t, out)
		})
	}

	for _, tr := range [][]string{nil, {link + " " + span, "extra"}} {
		_, err := markdown.Apply(src, segs, tr)
		require.Error(t, err)
	}

	two, err := markdown.Segments("one\n\ntwo\n")
	require.NoError(t, err)
	require.Len(t, two, 2)

	_, err = markdown.Apply("one\n\ntwo\n", two, []string{"a"})
	require.Error(t, err)

	_, err = markdown.Apply("other source\n", segs, []string{"x"})
	require.Error(t, err)
}

func TestPlainText(t *testing.T) {
	seg := markdown.TextSegment("a < b & <c>")
	assert.Equal(t, "a &lt; b &amp; &lt;c&gt;", seg)

	got, err := markdown.PlainText(seg)
	require.NoError(t, err)
	assert.Equal(t, "a < b & <c>", got)

	got, err = markdown.PlainText("x <em>y</em>")
	require.NoError(t, err)
	assert.Equal(t, "x y", got)

	_, err = markdown.PlainText("<em>x")
	require.Error(t, err)
}

func TestApplyEscapesTranslatedText(t *testing.T) {
	src := "plain\n\n| a |\n|---|\n| b |\n\n[l](<http://x.test/a b> \"t\") ![](x.png \"q\") `` a` ``\n"
	segs, err := markdown.Segments(src)
	require.NoError(t, err)
	require.Len(t, segs, 4)

	rest := func(first string) []string {
		// the link text is changed too: it holds markdown-looking text
		link := strings.Replace(segs[3].Text, ">l</a>", ">[x](y) *a*</a>", 1)
		return []string{first, "<em>A</em>", "b", link}
	}

	for _, tc := range []struct{ in, want string }{
		{"# h 1. x - y_z _w <b> `c` &copy; a|b", "\\# h 1. x - y_z \\_w \\<b\\> \\`c\\` \\&copy; a|b"},
		{"1. x", "1\\. x"},
		{"[x](y) and *a*", "\\[x\\](y) and \\*a\\*"},
		{"a\n- b\n# c\n> d\n+ e\n1) f", "a\n\\- b\n\\# c\n\\> d\n\\+ e\n1\\) f"},
	} {
		seg := markdown.TextSegment(tc.in)
		got, err := markdown.Apply(src, segs, rest(seg))
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(got, tc.want+"\n\n"), got)

		// whatever the translation says, it still reads back as the same text
		again, err := markdown.Segments(got)
		require.NoError(t, err)
		require.Len(t, again, 4)
		assert.Equal(t, seg, again[0].Text, tc.in)
		assert.Contains(t, again[3].Text, "[x](y) *a*") // text, not a nested link or emphasis
	}

	got, err := markdown.Apply(src, segs, rest("plain"))
	require.NoError(t, err)
	assert.Contains(t, got, "[\\[x\\](y) \\*a\\*](<http://x.test/a b> \"t\") ![](x.png \"q\") `` a` ``\n")

	got, err = markdown.Apply(src, segs, []string{"plain", "a|b", "b", segs[3].Text})
	require.NoError(t, err)
	assert.Contains(t, got, "| a\\|b |")

	got, err = markdown.Apply(src, segs, []string{"plain", "<em> spaced </em>!", "b", segs[3].Text})
	require.NoError(t, err)
	assert.Contains(t, got, "|  *spaced* ! |")
}
