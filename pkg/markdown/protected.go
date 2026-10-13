package markdown

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	"github.com/can3p/pcom/pkg/markdown/mdext/blocktags"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Range is a half-open byte range [Start, End) of a markdown source.
type Range struct {
	Start, End int
}

// Overlaps reports whether the two ranges share at least one byte.
func (r Range) Overlaps(o Range) bool {
	return r.Start < o.End && o.Start < r.End
}

var (
	// a block tag may sit in a container: a quote or a list item
	containerPrefixRe = regexp.MustCompile(`^[ \t]*(?:(?:>|[-*+]|\d+[.)])[ \t]*)*`)
	refDefLineRe      = regexp.MustCompile(`^ {0,3}\[[^\]\n]+\]:[ \t]*\S.*$`)
)

// ProtectedRanges returns the byte ranges of md that a proofreader must not
// change, sorted and merged: code blocks (fences included) and code spans
// (backticks included), link and image destinations (the "url "title"" of an
// inline link, reference definitions), autolinks, raw HTML and pcom's
// block-tag lines. Link text, the content of a {spoiler} and any other prose
// are not protected.
func ProtectedRanges(md string) []Range {
	src := []byte(md)
	doc := NewParser(types.ViewEditPreview, nil, nil).Parser().Parse(text.NewReader(src))

	var out []Range

	add := func(start, end int) {
		start, end = max(start, 0), min(end, len(src))
		if start < end {
			out = append(out, Range{start, end})
		}
	}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n := n.(type) {
		case *ast.FencedCodeBlock:
			add(fencedRange(src, n))
		case *ast.CodeBlock:
			addLines(src, add, n.Lines())
		case *ast.HTMLBlock:
			addLines(src, add, n.Lines())

			if n.HasClosure() {
				add(n.ClosureLine.Start, n.ClosureLine.Stop)
			}
		case *ast.RawHTML:
			for i := range n.Segments.Len() {
				seg := n.Segments.At(i)
				add(seg.Start, seg.Stop)
			}
		case *ast.CodeSpan:
			add(codeSpanRange(src, n))
		case *ast.AutoLink:
			add(autoLinkRange(src, n))
		case *ast.Link:
			add(destinationRange(src, n, n.Destination))
		case *ast.Image:
			add(destinationRange(src, n, n.Destination))
		}

		return ast.WalkContinue, nil
	})

	// Block-tag lines are consumed by the tag parser and reference
	// definitions are kept aside by goldmark: neither leaves a segment in the
	// AST, so they are found by line. Code blocks are protected anyway.
	for pos := 0; pos < len(src); {
		end := lineEnd(src, pos)

		line := bytes.TrimRight(src[pos:end], "\r")
		if refDefLineRe.Match(line) {
			add(pos, end)
		} else if at := blockTagAt(line); at >= 0 {
			add(pos+at, end)
		}

		pos = end + 1
	}

	return mergeRanges(out)
}

// blockTagAt returns the offset of the block tag that opens or closes in
// line, or -1. Whatever follows the tag on its line is part of it.
func blockTagAt(line []byte) int {
	at := len(containerPrefixRe.Find(line))
	rest := line[at:]

	m := blocktags.OpeningBlockTagRe.FindSubmatch(rest)
	if m == nil {
		m = blocktags.ClosingBlockTagRe.FindSubmatch(rest)
	}

	if m == nil || !slices.ContainsFunc(blocktags.DefaultTags, func(d *blocktags.TagDef) bool {
		return d.Name == strings.ToLower(string(m[1]))
	}) {
		return -1
	}

	return at
}

func mergeRanges(rs []Range) []Range {
	if len(rs) == 0 {
		return nil
	}

	slices.SortFunc(rs, func(a, b Range) int { return a.Start - b.Start })

	merged := rs[:1]

	for _, r := range rs[1:] {
		last := &merged[len(merged)-1]
		if r.Start <= last.End {
			last.End = max(last.End, r.End)
			continue
		}

		merged = append(merged, r)
	}

	return merged
}

// addLines protects whole lines, indentation included and line break not.
func addLines(src []byte, add func(int, int), lines *text.Segments) {
	for i := range lines.Len() {
		seg := lines.At(i)
		add(lineStart(src, seg.Start), len(bytes.TrimRight(src[:seg.Stop], "\r\n")))
	}
}

func lineStart(src []byte, pos int) int {
	return bytes.LastIndexByte(src[:pos], '\n') + 1
}

func lineEnd(src []byte, pos int) int {
	i := bytes.IndexByte(src[pos:], '\n')
	if i < 0 {
		return len(src)
	}

	return pos + i
}

// fencedRange covers the opening fence line, the content and the closing
// fence line if there is one.
func fencedRange(src []byte, n *ast.FencedCodeBlock) (int, int) {
	lines := n.Lines()
	start, end := -1, -1

	if n.Info != nil {
		start = lineStart(src, n.Info.Segment.Start)
		end = lineEnd(src, n.Info.Segment.Stop)
	}

	if lines.Len() > 0 {
		first, last := lines.At(0), lines.At(lines.Len()-1)
		if start < 0 && first.Start > 0 {
			start = lineStart(src, first.Start-1)
		}

		end = max(end, last.Stop)
	}

	if start < 0 {
		return 0, 0
	}

	// The closing fence is the line after the content when it only holds
	// fence characters.
	if next := lineEnd(src, max(end-1, 0)) + 1; next < len(src) {
		rest := strings.TrimSpace(string(src[next:lineEnd(src, next)]))
		if strings.HasPrefix(rest, "```") || strings.HasPrefix(rest, "~~~") {
			end = lineEnd(src, next)
		}
	}

	return start, end
}

func codeSpanRange(src []byte, n *ast.CodeSpan) (int, int) {
	start, end := -1, -1

	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			if start < 0 {
				start = t.Segment.Start
			}

			end = t.Segment.Stop
		}
	}

	if start < 0 {
		return 0, 0
	}

	// goldmark strips one space inside the backticks
	if start >= 2 && src[start-1] == ' ' && src[start-2] == '`' {
		start--
	}

	if end+1 < len(src) && src[end] == ' ' && src[end+1] == '`' {
		end++
	}

	for start > 0 && src[start-1] == '`' {
		start--
	}

	for end < len(src) && src[end] == '`' {
		end++
	}

	return start, end
}

// autoLinkRange covers the label, which is a slice of the source, and the
// angle brackets around it.
func autoLinkRange(src []byte, n *ast.AutoLink) (int, int) {
	label := n.Label(src)
	if len(label) == 0 {
		return 0, 0
	}

	start := cap(src) - cap(label)
	if start < 0 || start+len(label) > len(src) || !bytes.Equal(src[start:start+len(label)], label) {
		return 0, 0
	}

	end := start + len(label)
	if start > 0 && end < len(src) && src[start-1] == '<' && src[end] == '>' {
		start--
		end++
	}

	return start, end
}

// destinationRange covers the "url "title"" part of an inline link or image,
// without the parentheses. The AST keeps the destination but not where it
// was, so it is searched for after the link text.
func destinationRange(src []byte, n ast.Node, dest []byte) (int, int) {
	anchor := -1

	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			anchor = max(anchor, t.Segment.Stop)
		}

		return ast.WalkContinue, nil
	})

	if anchor < 0 {
		for p := n.Parent(); p != nil; p = p.Parent() {
			if p.Type() == ast.TypeBlock && p.Lines().Len() > 0 {
				anchor = p.Lines().At(0).Start
				break
			}
		}
	}

	if anchor < 0 {
		return 0, 0
	}

	firstStart, firstEnd := 0, 0

	for from := anchor; from < len(src); {
		i := bytes.Index(src[from:], []byte("]("))
		if i < 0 {
			break
		}

		start := from + i + 2
		end := closingParen(src, start)

		if firstEnd == 0 {
			firstStart, firstEnd = start, end
		}

		if len(dest) == 0 || bytes.Contains(src[start:end], dest) {
			return start, end
		}

		from = start
	}

	return firstStart, firstEnd
}

// closingParen returns the offset of the parenthesis that closes the one
// opened just before start, or the end of the source.
func closingParen(src []byte, start int) int {
	depth := 1

	for i := start; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return len(src)
}
