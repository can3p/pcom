package rss_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/postops/rss"
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

// rerender writes the same segment with other bytes: attributes reversed, <br/>.
func rerender(t *testing.T, seg string) string {
	var walk func(n *html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			slicesReverse(n.Attr)
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
	{"paragraph_inline", `<p>Hello <em>big</em> and <strong>bold</strong> world, see <a href="https://example.com/x" title="T">a <em>fine</em> page</a> now.</p><p>Line one<br/>line two</p>`},
	{"headings_lists", `<h2>Title here</h2><ul><li>first <code>code()</code> item</li><li>second <a href="/user/alice">@alice</a> item<ul><li>nested item</li></ul></li></ul><ol><li>one</li></ol>`},
	{"table", `<table><thead><tr><th>Name</th><th>Role</th></tr></thead><tbody><tr><td><em>Ann</em></td><td><code>admin</code> user</td></tr></tbody></table>`},
	{"quote_and_loose_text", `<blockquote>quoted words<p>inner para</p></blockquote>bare text after`},
	{"image_alt", `<p>Look <img src="/a.png" alt="a red apple"/> here.</p><p><img src="/b.png" alt="only alt"/></p><p><img src="/c.png" alt=""/></p>`},
	{"image_attrs", `<p>See <img src="/a.png" title="A title" class="wide" width="10" alt="a tall tree"/> now.</p>`},
	{"not_sent", `<pre>keep   this
as is</pre><p><iframe src="https://video.example/e"></iframe></p><p><code>only_code</code></p><hr/><p>after</p>`},
	{"script_style", `<style>p { color: red }</style><p>before <script>var a = "x < y";</script> after</p><script>alert(1)</script>`},
	{"entities_and_attrs", `<p class="lead">Fish &amp; chips &lt;3 <span class="x">styled</span> <b class="y">bold</b> <u>under</u></p>`},
	{"mention", `<p>Hi <a href="/user/bob">@bob</a>, read <a href="/p/1">this post</a>.</p>`},
	{"nested_lists", `<ul><li>top <em>one</em><ul><li>nested two<ul><li>deep three</li></ul></li></ul></li><li>top four</li></ul>`},
	{"nested_blockquotes", `<blockquote><p>outer words</p><blockquote><p>inner words</p></blockquote>back outside</blockquote>`},
	{"list_in_blockquote", `<blockquote><p>intro line</p><ul><li>item one</li><li>item <strong>two</strong></li></ul></blockquote>`},
}

func TestSegmentsGolden(t *testing.T) {
	for _, tc := range translateCases {
		t.Run(tc.name, func(t *testing.T) {
			segs, err := rss.Segments(tc.src)
			require.NoError(t, err)

			var texts, same, up []string
			for _, s := range segs {
				texts = append(texts, s.Text)
				same = append(same, rerender(t, s.Text))
				up = append(up, shout(t, s.Text))
			}

			// identity round trip: other bytes, same meaning, same source
			got, err := rss.Apply(tc.src, segs, same)
			require.NoError(t, err)
			assert.Equal(t, tc.src, got)

			require.NoError(t, translate.CheckShape(texts, up))

			got, err = rss.Apply(tc.src, segs, up)
			require.NoError(t, err)

			again, err := rss.Segments(got)
			require.NoError(t, err)
			assert.Len(t, again, len(segs))

			var sb strings.Builder
			fmt.Fprintf(&sb, "--- source\n%s\n--- segments\n", tc.src)

			for i, s := range segs {
				fmt.Fprintf(&sb, "[%d] %q\n", i, s.Text)
			}

			fmt.Fprintf(&sb, "--- applied\n%s\n", got)
			golden.Assert(t, "translate/"+tc.name, []byte(sb.String()))
		})
	}
}

func TestReorderGolden(t *testing.T) {
	src := `<p>A <em>one</em> then <a href="http://a.test" rel="x">two</a> and <code>x</code> plus <strong>three</strong> at <a href="http://b.test">four</a>.</p><p>With <a href="/u/hana">@hana</a> then <img src="/i.png" alt="alt text"/> and <a href="http://l2.test">l2</a>.</p>`

	segs, err := rss.Segments(src)
	require.NoError(t, err)

	var texts, rev []string
	for _, s := range segs {
		texts = append(texts, s.Text)
		rev = append(rev, reverse(t, s.Text))
	}

	require.NoError(t, translate.CheckShape(texts, rev))

	got, err := rss.Apply(src, segs, rev)
	require.NoError(t, err)

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- source\n%s\n--- segments\n", src)

	for i := range segs {
		fmt.Fprintf(&sb, "[%d] %q\n    %q\n", i, texts[i], rev[i])
	}

	fmt.Fprintf(&sb, "--- applied\n%s\n", got)
	golden.Assert(t, "translate/reorder", []byte(sb.String()))
}

func TestApplyRestoresByIndex(t *testing.T) {
	src := `<p>See <a href="http://a.test" rel="x">one</a> and <a href="http://b.test">two</a> and <code>x</code>.</p>`
	segs, err := rss.Segments(src)
	require.NoError(t, err)
	require.Len(t, segs, 1)

	got, err := rss.Apply(src, segs, []string{
		`<a data-i="1">dos</a> y <a data-i="0">uno</a> y <span class="notranslate" data-i="2">x</span> <b>!</b>`,
	})
	require.NoError(t, err)
	assert.Equal(t, `<p><a href="http://b.test">dos</a> y <a href="http://a.test" rel="x">uno</a> y <code>x</code> <b>!</b></p>`, got)
}

func TestApplyDropsUnknownTags(t *testing.T) {
	src := `<p>A <a href="http://a.test">link</a> here <code>c</code></p>`
	segs, err := rss.Segments(src)
	require.NoError(t, err)

	span := ` <span class="notranslate" data-i="1">c</span>`

	for tr, want := range map[string]string{
		`<a>x</a>` + span:                   `<p>x <code>c</code></p>`,
		`<div>x <u>y</u></div>` + span:      `<p>x y <code>c</code></p>`,
		`<span>x</span>` + span:             `<p>x <code>c</code></p>`,
		`<em>x</em> <br>z` + span:           `<p><em>x</em> <br/>z <code>c</code></p>`,
		`<a data-i="0"><u>x</u></a>` + span: `<p><a href="http://a.test">x</a> <code>c</code></p>`,
	} {
		got, err := rss.Apply(src, segs, []string{tr})
		require.NoError(t, err, tr)
		assert.Equal(t, want, got, tr)
	}
}

func TestApplyAltOnly(t *testing.T) {
	src := `<p><img src="/b.png" alt="only alt"/></p>`
	segs, err := rss.Segments(src)
	require.NoError(t, err)
	require.Len(t, segs, 1)

	got, err := rss.Apply(src, segs, []string{"solo alt &amp; more"})
	require.NoError(t, err)
	assert.Equal(t, `<p><img src="/b.png" alt="solo alt &amp; more"/></p>`, got)
}

func TestApplyErrors(t *testing.T) {
	src := `<p>A <a href="http://a.test">link</a> here <code>c</code></p>`
	segs, err := rss.Segments(src)
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
			out, err := rss.Apply(src, segs, []string{tr})
			require.Error(t, err)
			assert.Empty(t, out)
		})
	}

	for _, tr := range [][]string{nil, {link + " " + span, "extra"}} {
		_, err := rss.Apply(src, segs, tr)
		require.Error(t, err)
	}

	two, err := rss.Segments("<p>one</p><p>two</p>")
	require.NoError(t, err)
	require.Len(t, two, 2)

	_, err = rss.Apply("<p>one</p><p>two</p>", two, []string{"a"})
	require.Error(t, err)

	_, err = rss.Apply("<p>other</p>", segs, []string{"x"})
	require.Error(t, err)
}
