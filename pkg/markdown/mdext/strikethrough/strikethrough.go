// Package strikethrough renders ~~text~~ as <del>text</del>.
//
// goldmark's own extension.Strikethrough also strikes text between single
// tildes, as GFM does, which turns everyday prose such as "~5 to ~10 minutes"
// into struck-through text. This extension accepts only double tildes and
// reuses goldmark's AST node and HTML renderer.
package strikethrough

import (
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const delimiterLength = 2

type delimiterProcessor struct{}

func (p *delimiterProcessor) IsDelimiter(b byte) bool {
	return b == '~'
}

func (p *delimiterProcessor) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char
}

func (p *delimiterProcessor) OnMatch(consumes int) gast.Node {
	return ast.NewStrikethrough()
}

var defaultDelimiterProcessor = &delimiterProcessor{}

type inlineParser struct{}

func (s *inlineParser) Trigger() []byte {
	return []byte{'~'}
}

func (s *inlineParser) Parse(parent gast.Node, block text.Reader, pc parser.Context) gast.Node {
	before := block.PrecendingCharacter()
	line, segment := block.PeekLine()
	node := parser.ScanDelimiter(line, before, 1, defaultDelimiterProcessor)
	if node == nil || node.OriginalLength != delimiterLength || before == '~' {
		return nil
	}

	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)

	return node
}

func (s *inlineParser) CloseBlock(parent gast.Node, pc parser.Context) {}

type strikethrough struct{}

// New returns the extender that adds double tilde strikethrough.
func New() goldmark.Extender {
	return &strikethrough{}
}

func (e *strikethrough) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&inlineParser{}, 500),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(extension.NewStrikethroughHTMLRenderer(), 500),
	))
}
