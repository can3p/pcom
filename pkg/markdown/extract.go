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

func Parse(s string, mediaReplacer Replacer, userHandleReplacer types.Replacer[[]byte]) *parsedText {
	r := goldmarkText.NewReader([]byte(s))
	parser := NewParser(mediaReplacer, userHandleReplacer)
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

func (t *parsedText) ExtractLinks() []*EmbeddedLink {
	out := []*EmbeddedLink{}

	walkNode([]byte(t.s), t.node, 0, func(ch goldmarkAst.Node, onlySecondLevelChildElement bool) {
		if l, ok := ch.(*goldmarkAst.AutoLink); ok {
			out = append(out, &EmbeddedLink{
				URL:             string(l.URL(t.s)),
				OnlyLinkInBlock: onlySecondLevelChildElement,
			})
		} else if l, ok := ch.(*goldmarkAst.Link); ok {
			out = append(out, &EmbeddedLink{
				URL:             string(l.Destination),
				OnlyLinkInBlock: onlySecondLevelChildElement,
			})
		}
	})

	return out
}

func (t *parsedText) ExtractImageUrls() []*EmbeddedLink {
	out := []*EmbeddedLink{}

	walkNode([]byte(t.s), t.node, 0, func(ch goldmarkAst.Node, onlySecondLevelChildElement bool) {
		if l, ok := ch.(*goldmarkAst.Image); ok {
			out = append(out, &EmbeddedLink{
				URL: string(l.Destination),
			})
		}
	})

	return out
}

type visiter func(n goldmarkAst.Node, onlySecondLevelChildElement bool)

func walkNode(source []byte, n goldmarkAst.Node, level int, visitNode visiter) {

	// level 0 - root document
	// level 1 - top level block element
	onlySecondLevelChildElement := n.ChildCount() == 1 && level == 1

	if n.ChildCount() > 0 {
		ch := n.FirstChild()

		for ch != nil {
			visitNode(ch, onlySecondLevelChildElement)

			walkNode(source, ch, level+1, visitNode)

			ch = ch.NextSibling()
		}
	}
}
