package markdown

import (
	"io"

	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	goldmarkAst "github.com/yuin/goldmark/ast"
	goldmarkText "github.com/yuin/goldmark/text"
)

type EmbeddedLink struct {
	URL             string
	OnlyLinkInBlock bool
}

type parsedText struct {
	s      []byte
	node   goldmarkAst.Node
	parser goldmark.Markdown
}

func Parse(s string, view types.HTMLView, mediaReplacer Replacer, link types.Link) *parsedText {
	r := goldmarkText.NewReader([]byte(s))
	parser := NewParser(view, mediaReplacer, link)
	ast := parser.Parser().Parse(r)

	return &parsedText{
		s:      []byte(s),
		node:   ast,
		parser: parser,
	}
}

func (t *parsedText) Render(writer io.Writer) error {
	return t.parser.Renderer().Render(writer, t.s, t.node)
}

type Replacer func(inURL string) (bool, string)

func (t *parsedText) ExtractImageUrls() []*EmbeddedLink {
	out := []*EmbeddedLink{}

	walkNode([]byte(t.s), t.node, 0, func(ch goldmarkAst.Node) {
		if l, ok := ch.(*goldmarkAst.Image); ok {
			out = append(out, &EmbeddedLink{
				URL: string(l.Destination),
			})
		}
	})

	return out
}

type visiter func(n goldmarkAst.Node)

func walkNode(source []byte, n goldmarkAst.Node, level int, visitNode visiter) {
	if n.ChildCount() > 0 {
		ch := n.FirstChild()

		for ch != nil {
			visitNode(ch)

			walkNode(source, ch, level+1, visitNode)

			ch = ch.NextSibling()
		}
	}
}
