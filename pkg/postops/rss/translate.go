package rss

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/can3p/pcom/pkg/markdown"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Segment has the same shape as in pkg/markdown: the inline content of one
// block as minimal HTML. See markdown.Segment for the format.
type Segment = markdown.Segment

var (
	blockTags = toSet("p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "dl", "dt", "dd",
		"blockquote", "table", "thead", "tbody", "tfoot", "tr", "td", "th", "caption", "figure", "figcaption",
		"section", "article", "aside", "header", "footer", "nav", "main", "details", "summary", "address",
		"fieldset", "form", "center", "body", "html")
	// left alone, and they end a run of inline content
	skippedTags = toSet("pre", "hr", "textarea", "template", "head")
	// inline elements that must not change: sent as notranslate spans
	protectedTags = toSet("code", "kbd", "samp", "var", "tt", "wbr", "input", "button", "select", "svg",
		"math", "canvas", "iframe", "video", "audio", "object", "embed", "noscript", "script", "style", "img")
	// inline elements sent as bare tags when they have no attributes
	formatTags = toSet("b", "strong", "i", "em")
)

func toSet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}

	return m
}

type entry struct {
	text string
	run  *run       // a block's inline content, or nil
	alt  *html.Node // an <img> whose alt is the text, or nil
}

type run struct {
	parent *html.Node
	nodes  []*html.Node
	items  []*html.Node
	prot   map[int]bool // items sent as notranslate spans
}

type tree struct {
	root    *html.Node
	entries []*entry
}

// Segments splits sanitized HTML into the blocks to translate: every
// element with inline content (paragraphs, headings, list items, table
// cells, ...) becomes one segment, and so does each image alt. <pre>,
// embedded media, code and blocks without text of their own are left out.
func Segments(source string) ([]Segment, error) {
	t, err := parse(source)
	if err != nil {
		return nil, err
	}

	out := make([]Segment, len(t.entries))
	for i, e := range t.entries {
		out[i] = Segment{Text: e.text}
	}

	return out, nil
}

// Apply puts the translated segments in place of their blocks in source and
// returns the new HTML. segments must be what Segments returned for source;
// translated holds one translation per segment. Links and protected spans
// are restored by data-i. Any problem returns an error and no partial
// result. Applying the segments' own text returns source untouched.
func Apply(source string, segments []Segment, translated []string) (string, error) {
	t, err := parse(source)
	if err != nil {
		return "", err
	}

	if len(t.entries) != len(segments) || len(translated) != len(segments) {
		return "", fmt.Errorf("apply: %d segments, %d translations, source has %d", len(segments), len(translated), len(t.entries))
	}

	changed := false

	for i, e := range t.entries {
		if e.text != segments[i].Text {
			return "", fmt.Errorf("apply: segment %d does not match the source", i)
		}

		if translated[i] != e.text {
			changed = true
		}
	}

	if !changed {
		return source, nil
	}

	// Alts first: they edit the images that the runs then move around.
	for i, e := range t.entries {
		if e.alt == nil || translated[i] == e.text {
			continue
		}

		alt, err := markdown.PlainText(translated[i])
		if err != nil {
			return "", fmt.Errorf("apply: segment %d: %w", i, err)
		}

		setAttr(e.alt, "alt", strings.Join(strings.Fields(alt), " "))
	}

	// Build every replacement before touching the tree, so that an error
	// leaves nothing half done.
	type swap struct {
		r     *run
		nodes []*html.Node
	}

	var swaps []swap

	for i, e := range t.entries {
		if e.run == nil || translated[i] == e.text {
			continue
		}

		parsed, err := markdown.ParseSegment(translated[i])
		if err != nil {
			return "", fmt.Errorf("apply: segment %d: %w", i, err)
		}

		nodes, err := e.run.convert(parsed)
		if err != nil {
			return "", fmt.Errorf("apply: segment %d: %w", i, err)
		}

		swaps = append(swaps, swap{e.run, nodes})
	}

	for _, s := range swaps {
		s.r.replace(s.nodes)
	}

	var buf bytes.Buffer

	for c := t.root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return "", err
		}
	}

	return buf.String(), nil
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}

	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func attrVal(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}

	return "", false
}

func parse(source string) (*tree, error) {
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}

	nodes, err := html.ParseFragment(strings.NewReader(source), root)
	if err != nil {
		return nil, err
	}

	for _, n := range nodes {
		root.AppendChild(n)
	}

	t := &tree{root: root}
	t.container(root)

	return t, nil
}

func isInline(n *html.Node) bool {
	switch n.Type {
	case html.TextNode:
		return true
	case html.ElementNode:
		return !blockTags[n.Data] && !skippedTags[n.Data]
	}

	return false
}

func (t *tree) container(n *html.Node) {
	var nodes []*html.Node

	flush := func() {
		if len(nodes) > 0 {
			t.run(n, nodes)
			nodes = nil
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isInline(c) {
			nodes = append(nodes, c)
			continue
		}

		flush()

		if c.Type == html.ElementNode && blockTags[c.Data] {
			t.container(c)
		}
	}

	flush()
}

type runBuilder struct {
	t       *tree
	r       *run
	sb      strings.Builder
	alts    []*entry
	hasText bool
}

func (t *tree) run(parent *html.Node, nodes []*html.Node) {
	r := &run{parent: parent, nodes: nodes, prot: map[int]bool{}}
	b := &runBuilder{t: t, r: r}

	for _, n := range nodes {
		b.inline(n)
	}

	if b.hasText {
		t.entries = append(t.entries, &entry{text: b.sb.String(), run: r})
	}

	t.entries = append(t.entries, b.alts...)
}

func hasLetter(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

func (b *runBuilder) item(n *html.Node) int {
	b.r.items = append(b.r.items, n)
	return len(b.r.items) - 1
}

func (b *runBuilder) inline(n *html.Node) {
	if n.Type == html.TextNode {
		b.hasText = b.hasText || hasLetter(n.Data)
		b.sb.WriteString(html.EscapeString(n.Data))

		return
	}

	switch {
	case n.Data == "br":
		b.sb.WriteString("<br>")
	case protectedTags[n.Data] || isMention(n):
		var content string

		if n.Data == "a" || n.Data == "code" || n.Data == "kbd" || n.Data == "samp" || n.Data == "var" || n.Data == "tt" {
			content = nodeText(n)
		}

		if n.Data == "img" {
			if alt, _ := attrVal(n, "alt"); hasLetter(alt) {
				b.alts = append(b.alts, &entry{text: html.EscapeString(alt), alt: n})
			}
		}

		i := b.item(n)
		b.r.prot[i] = true

		fmt.Fprintf(&b.sb, `<span class="notranslate" data-i="%d">%s</span>`, i, html.EscapeString(content))
	case formatTags[n.Data] && len(n.Attr) == 0:
		b.sb.WriteString("<" + n.Data + ">")
		b.children(n)
		b.sb.WriteString("</" + n.Data + ">")
	default:
		fmt.Fprintf(&b.sb, `<%s data-i="%d">`, n.Data, b.item(n))
		b.children(n)
		b.sb.WriteString("</" + n.Data + ">")
	}
}

// isMention reports whether n is a link to a user: its text starts with @.
func isMention(n *html.Node) bool {
	return n.Data == "a" && strings.HasPrefix(strings.TrimSpace(nodeText(n)), "@")
}

func (b *runBuilder) children(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.inline(c)
	}
}

func nodeText(n *html.Node) string {
	var sb strings.Builder

	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return sb.String()
}

// convert turns the parsed translation back into nodes, restoring links
// and protected spans from the source by data-i. An element without data-i
// other than em, strong, b, i and br is dropped and its content kept.
func (r *run) convert(parsed []*html.Node) ([]*html.Node, error) {
	used := map[int]bool{}

	out, err := r.convertList(parsed, used)
	if err != nil {
		return nil, err
	}

	for i := range r.items {
		if r.prot[i] && !used[i] {
			return nil, fmt.Errorf("protected span data-i=%d is missing", i)
		}
	}

	return out, nil
}

func (r *run) convertList(parsed []*html.Node, used map[int]bool) ([]*html.Node, error) {
	var out []*html.Node

	for _, p := range parsed {
		ns, err := r.convertNode(p, used)
		if err != nil {
			return nil, err
		}

		out = append(out, ns...)
	}

	return out, nil
}

func (r *run) convertNode(p *html.Node, used map[int]bool) ([]*html.Node, error) {
	if p.Type == html.TextNode {
		return []*html.Node{{Type: html.TextNode, Data: p.Data}}, nil
	}

	v, hasIdx := attrVal(p, "data-i")

	if !hasIdx {
		children, err := r.convertList(childList(p), used)
		if err != nil {
			return nil, err
		}

		if p.Data != "br" && !formatTags[p.Data] {
			return children, nil // unknown tag: keep the text only
		}

		n := &html.Node{Type: html.ElementNode, Data: p.Data}
		for _, c := range children {
			n.AppendChild(c)
		}

		return []*html.Node{n}, nil
	}

	i, err := strconv.Atoi(v)
	if err != nil || i < 0 || i >= len(r.items) {
		return nil, fmt.Errorf("unknown data-i %q", v)
	}

	if used[i] {
		return nil, fmt.Errorf("data-i %d used twice", i)
	}

	used[i] = true
	orig := r.items[i]

	if r.prot[i] {
		if c, _ := attrVal(p, "class"); p.Data != "span" || c != "notranslate" {
			return nil, fmt.Errorf("data-i %d is a protected span", i)
		}

		// reuse the source node; it keeps its attributes and children
		return []*html.Node{orig}, nil
	}

	if p.Data != orig.Data {
		return nil, fmt.Errorf("data-i %d is <%s>, got <%s>", i, orig.Data, p.Data)
	}

	children, err := r.convertList(childList(p), used)
	if err != nil {
		return nil, err
	}

	n := &html.Node{Type: html.ElementNode, Data: orig.Data, DataAtom: orig.DataAtom, Attr: slices.Clone(orig.Attr)}
	for _, c := range children {
		n.AppendChild(c)
	}

	return []*html.Node{n}, nil
}

func childList(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}

	return out
}

// replace swaps the run's nodes for nodes in the tree.
func (r *run) replace(nodes []*html.Node) {
	next := r.nodes[len(r.nodes)-1].NextSibling

	for _, n := range r.nodes {
		r.parent.RemoveChild(n)
	}

	for _, n := range nodes {
		if n.Parent != nil { // a protected node reused from inside a removed element
			n.Parent.RemoveChild(n)
		}

		r.parent.InsertBefore(n, next)
	}
}
