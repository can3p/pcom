package markdown

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/can3p/pcom/pkg/markdown/mdext"
	"github.com/can3p/pcom/pkg/markdown/mdext/blocktags"
	"github.com/can3p/pcom/pkg/markdown/mdext/videoembed"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/html"
)

// Segment is one piece of text sent to a translation backend: the inline
// content of one block as minimal HTML. Emphasis is <em>/<strong>, a link
// is <a data-i="n"> (its URL is not sent), and everything that must not
// change (code spans, URLs, @handles, images, raw HTML) is
// <span class="notranslate" data-i="n">...</span>. data-i is unique within
// a segment.
type Segment struct {
	Text string
}

type itemKind int

const (
	itemRaw   itemKind = iota // protected span, restored verbatim
	itemLink                  // <a data-i>, children are translated
	itemImage                 // protected span, its alt text may be a segment
)

type mdItem struct {
	kind        itemKind
	raw         string // itemRaw: the markdown to restore
	dest, title string // itemLink, itemImage
	alt         int    // itemImage: index of the alt segment, or -1
	altRaw      string // itemImage: the original alt, escaped as markdown
}

type mdSeg struct {
	Segment
	start, stop int    // source range replaced by the translation
	prefix      string // line prefix of continuation lines ("> ", "  ")
	cell        bool
	inner       bool // image alt: has no source range of its own
	silent      bool // only holds images: not sent, but rewritten when an alt changes
	items       []mdItem
}

// Segments splits markdown source into the blocks to translate: paragraphs,
// headings, list items, table cells and image alts. Code blocks, galleries,
// embedded videos, raw HTML blocks and blocks without any text of their own
// are left out. Each segment's Text is the inline content as HTML.
func Segments(source string) ([]Segment, error) {
	var out []Segment

	for _, s := range collectSegments(source) {
		if !s.silent {
			out = append(out, s.Segment)
		}
	}

	return out, nil
}

// TextSegment returns s as a plain-text segment.
func TextSegment(s string) string { return html.EscapeString(s) }

// PlainText returns the text of a segment, dropping any tags.
func PlainText(segment string) (string, error) {
	nodes, err := ParseSegment(segment)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, n := range nodes {
		writeNodeText(&sb, n)
	}

	return sb.String(), nil
}

func writeNodeText(sb *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeNodeText(sb, c)
	}
}

// ParseSegment strictly parses a segment into nodes: unbalanced or unclosed
// tags and comments are errors. Callers validate tag names themselves.
func ParseSegment(s string) ([]*html.Node, error) {
	root := &html.Node{Type: html.ElementNode, Data: "root"}
	cur := root
	z := html.NewTokenizer(strings.NewReader(s))

	for {
		switch z.Next() {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return nil, err
			}

			if cur != root {
				return nil, fmt.Errorf("unclosed <%s>", cur.Data)
			}

			var nodes []*html.Node
			for c := root.FirstChild; c != nil; c = c.NextSibling {
				nodes = append(nodes, c)
			}

			return nodes, nil
		case html.TextToken:
			cur.AppendChild(&html.Node{Type: html.TextNode, Data: string(z.Text())})
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			n := &html.Node{Type: html.ElementNode, Data: t.Data, Attr: t.Attr}
			cur.AppendChild(n)

			if !isVoid(t.Data) && t.Type == html.StartTagToken {
				cur = n
			}
		case html.EndTagToken:
			name := z.Token().Data
			if isVoid(name) {
				continue
			}

			if cur == root || cur.Data != name {
				return nil, fmt.Errorf("unexpected </%s>", name)
			}

			cur = cur.Parent
		default:
			return nil, fmt.Errorf("unexpected comment or doctype")
		}
	}
}

func isVoid(name string) bool {
	return name == "br" || name == "img" || name == "hr" || name == "wbr"
}

func nodeAttr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}

	return "", false
}

// Apply puts the translated segments in place of their blocks in source and
// returns the new markdown. segments must be what Segments returned for
// source, and translated holds one translation per segment. Links and
// protected spans are restored by data-i. Any problem returns an error and
// no partial result. A segment translated to itself leaves its block
// untouched, so applying the segments' own text returns source.
func Apply(source string, segments []Segment, translated []string) (string, error) {
	segs := collectSegments(source)

	// tr lines the translations up with segs; silent segments are not sent.
	tr := make([]string, len(segs))
	n := 0

	for i, s := range segs {
		if s.silent {
			tr[i] = s.Text
			continue
		}

		if n >= len(segments) || n >= len(translated) || segments[n].Text != s.Text {
			return "", fmt.Errorf("apply: segment %d does not match the source", n)
		}

		tr[i] = translated[n]
		n++
	}

	if n != len(segments) || n != len(translated) {
		return "", fmt.Errorf("apply: %d segments, %d translations, source has %d", len(segments), len(translated), n)
	}

	translated = tr

	// Image alts first: the paragraph holding the image needs them.
	alts := map[int]string{}

	for i, s := range segs {
		if !s.inner || translated[i] == s.Text {
			continue
		}

		nodes, err := ParseSegment(translated[i])
		if err != nil {
			return "", fmt.Errorf("apply: segment %d: %w", i, err)
		}

		var sb strings.Builder
		for _, n := range nodes {
			writeNodeText(&sb, n)
		}

		r := &mdRenderer{cell: s.cell}
		alts[i] = r.text(strings.Join(strings.Fields(sb.String()), " "))
	}

	var sb strings.Builder

	last := 0

	for i, s := range segs {
		if s.inner {
			continue
		}

		if translated[i] == s.Text && !altChanged(s, alts) {
			continue
		}

		r := &mdRenderer{seg: s, alts: alts, cell: s.cell, lineStart: true, used: map[int]bool{}}

		nodes, err := ParseSegment(translated[i])
		if err != nil {
			return "", fmt.Errorf("apply: segment %d: %w", i, err)
		}

		md, err := r.nodes(nodes)
		if err != nil {
			return "", fmt.Errorf("apply: segment %d: %w", i, err)
		}

		for j, it := range s.items {
			if it.kind != itemLink && !r.used[j] {
				return "", fmt.Errorf("apply: segment %d: protected span data-i=%d is missing", i, j)
			}
		}

		sb.WriteString(source[last:s.start])
		sb.WriteString(md)

		last = s.stop
	}

	sb.WriteString(source[last:])

	return sb.String(), nil
}

func altChanged(s mdSeg, alts map[int]string) bool {
	for _, it := range s.items {
		if it.kind == itemImage && it.alt >= 0 {
			if _, ok := alts[it.alt]; ok {
				return true
			}
		}
	}

	return false
}

// ---- markdown -> segments

var noopLink types.Link = func(in string, _ ...string) string { return in }

type collector struct {
	src  []byte
	segs []mdSeg
}

func collectSegments(source string) []mdSeg {
	md := NewParser(types.ViewEditPreview, nil, noopLink)
	extension.Table.Extend(md)

	src := []byte(source)
	doc := md.Parser().Parse(text.NewReader(src))
	c := &collector{src: src}
	c.walk(doc)

	return c.segs
}

func (c *collector) walk(n ast.Node) {
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		switch k := ch.(type) {
		case *ast.Paragraph, *ast.TextBlock, *ast.Heading, *east.TableCell:
			c.block(ch)
		case *blocktags.BlockTag:
			if k.BlockTagName != "gallery" {
				c.walk(ch)
			}
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock, *videoembed.VideoEmbed:
		default:
			c.walk(ch)
		}
	}
}

func (c *collector) block(n ast.Node) {
	lines := n.Lines()
	if lines.Len() == 0 {
		return
	}

	first, last := lines.At(0), lines.At(lines.Len()-1)
	start, stop := first.Start, last.Stop

	for stop > start && strings.ContainsRune(" \t\r\n", rune(c.src[stop-1])) {
		stop--
	}

	for start < stop && (c.src[start] == ' ' || c.src[start] == '\t') {
		start++
	}

	if start >= stop {
		return
	}

	_, isCell := n.(*east.TableCell)
	seg := mdSeg{start: start, stop: stop, cell: isCell}

	if lines.Len() > 1 {
		seg.prefix = string(c.src[first.Stop:lines.At(1).Start])
	}

	c.build(n, &seg)
}

// build serializes n's inline children into seg and appends it, followed by
// the image alt segments found on the way.
func (c *collector) build(n ast.Node, seg *mdSeg) {
	idx := len(c.segs)
	c.segs = append(c.segs, mdSeg{}) // reserve: the parent precedes its alts

	b := &segBuilder{c: c, seg: seg}
	b.children(n)

	if !b.hasText {
		if len(c.segs) == idx+1 {
			// Nothing to translate at all.
			c.segs = slices.Delete(c.segs, idx, idx+1)
			return
		}

		seg.silent = true // images with alts: kept so the alts have a home
	}

	seg.Text = b.sb.String()
	seg.items = b.items
	c.segs[idx] = *seg
}

type segBuilder struct {
	c       *collector
	seg     *mdSeg
	sb      strings.Builder
	items   []mdItem
	hasText bool
}

func (b *segBuilder) children(n ast.Node) {
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		b.inline(ch)
	}
}

func (b *segBuilder) protect(content string, it mdItem) {
	fmt.Fprintf(&b.sb, `<span class="notranslate" data-i="%d">%s</span>`, len(b.items), html.EscapeString(content))
	b.items = append(b.items, it)
}

var (
	entityRe   = regexp.MustCompile(`^&(#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{1,31});`)
	backtickRe = regexp.MustCompile("`+")
)

// semantic turns raw markdown text (backslash escapes, entities) into what
// the reader sees.
func semantic(raw []byte) string {
	var sb strings.Builder

	for i := 0; i < len(raw); {
		switch {
		case raw[i] == '\\' && i+1 < len(raw) && util.IsPunct(raw[i+1]):
			sb.WriteByte(raw[i+1])
			i += 2
		case raw[i] == '&' && entityRe.Match(raw[i:min(len(raw), i+34)]):
			m := entityRe.Find(raw[i:min(len(raw), i+34)])
			sb.Write(util.ResolveEntityNames(util.ResolveNumericReferences(m)))
			i += len(m)
		default:
			sb.WriteByte(raw[i])
			i++
		}
	}

	return sb.String()
}

func (b *segBuilder) text(s string) {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.hasText = true
			break
		}
	}

	b.sb.WriteString(html.EscapeString(s))
}

func (b *segBuilder) inline(n ast.Node) {
	src := b.c.src

	switch k := n.(type) {
	case *ast.Text:
		b.text(semantic(k.Segment.Value(src)))

		switch {
		case k.HardLineBreak():
			b.sb.WriteString("<br>")
		case k.SoftLineBreak():
			b.sb.WriteString("\n")
		}
	case *ast.String:
		b.text(string(k.Value))
	case *ast.Emphasis:
		tag := "em"
		if k.Level >= 2 {
			tag = "strong"
		}

		b.sb.WriteString("<" + tag + ">")
		b.children(n)
		b.sb.WriteString("</" + tag + ">")
	case *ast.Link:
		fmt.Fprintf(&b.sb, `<a data-i="%d">`, len(b.items))
		b.items = append(b.items, mdItem{kind: itemLink, dest: string(k.Destination), title: string(k.Title)})
		b.children(n)
		b.sb.WriteString("</a>")
	case *ast.Image:
		b.image(k)
	case *ast.CodeSpan:
		var content strings.Builder

		for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
			if t, ok := ch.(*ast.Text); ok {
				content.Write(t.Segment.Value(src))
			}
		}

		b.protect(content.String(), mdItem{kind: itemRaw, raw: codeSpan(content.String())})
	case *ast.AutoLink:
		label := string(k.Label(src))
		raw := label

		// a <...> autolink keeps its brackets: look for them in the block
		if bracketed := "<" + label + ">"; strings.Contains(string(src[b.seg.start:b.seg.stop]), bracketed) {
			raw = bracketed
		}

		b.protect(label, mdItem{kind: itemRaw, raw: raw})
	case *mdext.UserHandle:
		label := string(k.Label(src))
		b.protect(label, mdItem{kind: itemRaw, raw: label})
	case *ast.RawHTML:
		var raw strings.Builder

		for i := 0; i < k.Segments.Len(); i++ {
			s := k.Segments.At(i)
			raw.Write(s.Value(src))
		}

		b.protect(raw.String(), mdItem{kind: itemRaw, raw: raw.String()})
	default:
		b.children(n)
	}
}

func codeSpan(content string) string {
	longest := 0
	for _, m := range backtickRe.FindAllString(content, -1) {
		longest = max(longest, len(m))
	}

	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`") {
		content = " " + content + " "
	}

	return fence + content + fence
}

// image protects the whole image and, when its alt is plain text with
// something to translate, adds the alt as a segment of its own.
func (b *segBuilder) image(k *ast.Image) {
	it := mdItem{kind: itemImage, alt: -1, dest: string(k.Destination), title: string(k.Title)}

	var alt strings.Builder

	plain := k.FirstChild() != nil

	for ch := k.FirstChild(); ch != nil; ch = ch.NextSibling() {
		t, ok := ch.(*ast.Text)
		if !ok {
			plain = false
			break
		}

		alt.WriteString(string(t.Segment.Value(b.c.src)))

		if t.SoftLineBreak() {
			alt.WriteString(" ")
		}
	}

	if !plain {
		var all strings.Builder
		collectText(&all, k, b.c.src)
		it.altRaw = all.String()
		b.protect("", it)

		return
	}

	it.altRaw = alt.String()
	sem := semantic([]byte(it.altRaw))

	if hasLetter(sem) {
		it.alt = len(b.c.segs)
		b.c.segs = append(b.c.segs, mdSeg{
			Segment: Segment{Text: html.EscapeString(sem)},
			inner:   true,
			cell:    b.seg.cell,
		})
	}

	b.protect("", it)
}

func collectText(sb *strings.Builder, n ast.Node, src []byte) {
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		if t, ok := ch.(*ast.Text); ok {
			sb.Write(t.Segment.Value(src))
		} else {
			collectText(sb, ch, src)
		}
	}
}

func hasLetter(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// ---- segments -> markdown

type mdRenderer struct {
	seg       mdSeg
	alts      map[int]string
	cell      bool
	lineStart bool
	used      map[int]bool
}

func (r *mdRenderer) nodes(ns []*html.Node) (string, error) {
	var sb strings.Builder

	for _, n := range ns {
		s, err := r.node(n)
		if err != nil {
			return "", err
		}

		sb.WriteString(s)
	}

	return sb.String(), nil
}

func (r *mdRenderer) item(n *html.Node) (mdItem, error) {
	v, ok := nodeAttr(n, "data-i")
	if !ok {
		return mdItem{}, fmt.Errorf("<%s> without data-i", n.Data)
	}

	i, err := strconv.Atoi(v)
	if err != nil || i < 0 || i >= len(r.seg.items) {
		return mdItem{}, fmt.Errorf("unknown data-i %q", v)
	}

	if r.used[i] {
		return mdItem{}, fmt.Errorf("data-i %d used twice", i)
	}

	r.used[i] = true

	return r.seg.items[i], nil
}

func (r *mdRenderer) node(n *html.Node) (string, error) {
	if n.Type == html.TextNode {
		return r.text(n.Data), nil
	}

	if _, ok := nodeAttr(n, "data-i"); !ok {
		switch n.Data {
		case "em", "strong", "b", "i":
			return r.emphasis(n)
		case "br":
			if r.cell {
				return " ", nil
			}

			r.lineStart = true

			return "\\\n" + r.seg.prefix, nil
		}

		return r.nodes(childNodes(n)) // unknown tag: keep the text only
	}

	it, err := r.item(n)
	if err != nil {
		return "", err
	}

	r.lineStart = false

	switch it.kind {
	case itemLink:
		if n.Data != "a" {
			return "", fmt.Errorf("data-i on <%s> is a link", n.Data)
		}

		inner, err := r.nodes(childNodes(n))
		if err != nil {
			return "", err
		}

		return "[" + inner + "](" + linkTarget(it) + ")", nil
	case itemRaw, itemImage:
		if c, _ := nodeAttr(n, "class"); n.Data != "span" || c != "notranslate" {
			return "", fmt.Errorf("data-i on <%s> is a protected span", n.Data)
		}

		if it.kind == itemRaw {
			return it.raw, nil
		}

		alt := it.altRaw

		if it.alt >= 0 {
			if a, ok := r.alts[it.alt]; ok {
				alt = a
			}
		}

		return "![" + alt + "](" + linkTarget(it) + ")", nil
	}

	return "", fmt.Errorf("unexpected data-i")
}

func (r *mdRenderer) emphasis(n *html.Node) (string, error) {
	r.lineStart = false

	inner, err := r.nodes(childNodes(n))
	if err != nil {
		return "", err
	}

	mark := "*"
	if n.Data == "strong" || n.Data == "b" {
		mark = "**"
	}

	trimmed := strings.TrimSpace(inner)
	if trimmed == "" {
		return inner, nil
	}

	lead := inner[:len(inner)-len(strings.TrimLeft(inner, " \t\n"))]
	trail := inner[len(strings.TrimRight(inner, " \t\n")):]

	return lead + mark + trimmed + mark + trail, nil
}

func childNodes(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}

	return out
}

func linkTarget(it mdItem) string {
	dest := it.dest
	if dest == "" || strings.ContainsAny(dest, " ()<>\n") {
		dest = "<" + strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A").Replace(dest) + ">"
	}

	if it.title != "" {
		dest += ` "` + strings.ReplaceAll(it.title, `"`, `\"`) + `"`
	}

	return dest
}

// text escapes plain text so that it reads back as the same text.
func (r *mdRenderer) text(s string) string {
	var sb strings.Builder

	rs := []rune(s)

	for i, c := range rs {
		switch {
		case c == '\n':
			if r.cell {
				sb.WriteByte(' ')
				continue
			}

			sb.WriteString("\n" + r.seg.prefix)

			r.lineStart = true

			continue
		case strings.ContainsRune("\\`*[]<>", c), c == '|' && r.cell:
			sb.WriteByte('\\')
		case c == '_':
			if i == 0 || i == len(rs)-1 || !isWordRune(rs[i-1]) || !isWordRune(rs[i+1]) {
				sb.WriteByte('\\')
			}
		case c == '&':
			if entityRe.MatchString(string(rs[i:min(len(rs), i+34)])) {
				sb.WriteByte('\\')
			}
		case r.lineStart && strings.ContainsRune("#-+=", c):
			sb.WriteByte('\\')
		case r.lineStart && unicode.IsDigit(c):
			// a digit run at a line start may be a list marker: keep
			// lineStart so that the . or ) after it gets escaped
			sb.WriteRune(c)
			continue
		case r.lineStart && (c == '.' || c == ')') && i > 0 && unicode.IsDigit(rs[i-1]):
			sb.WriteByte('\\')
		}

		sb.WriteRune(c)

		if c != ' ' && c != '\t' {
			r.lineStart = false
		}
	}

	return sb.String()
}

func isWordRune(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }
