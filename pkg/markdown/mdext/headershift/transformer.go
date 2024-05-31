package headershift

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type headerTransformer struct {
	shift int
}

func NewHeaderTransformer(shift int) *headerTransformer {
	return &headerTransformer{
		shift: shift,
	}
}

func (t *headerTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	err := ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		heading, ok := n.(*ast.Heading)

		if !ok {
			return ast.WalkContinue, nil
		}

		heading.Level += t.shift
		return ast.WalkContinue, nil
	})

	if err != nil {
		panic(err)
	}
}

type headerShift struct {
	shift int
}

// NewVideoEmbed creates a new [goldmark.Extender] that
// allow you to parse text that seems like a @uservideoEmbed
func NewHeaderShiftExtender(shift int) goldmark.Extender {
	return &headerShift{
		shift: shift,
	}
}

func (e *headerShift) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(NewHeaderTransformer(e.shift), 500),
		),
	)
}
