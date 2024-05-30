package blocktags

import (
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

type blockTag struct {
	view types.HTMLView
	link types.Link
}

// NewBlockTag creates a new [goldmark.Extender] that
// allow you to parse text that seems like a @userblockTag
func NewBlockTagExtender(view types.HTMLView, link types.Link) goldmark.Extender {
	return &blockTag{
		view: view,
		link: link,
	}
}

func (e *blockTag) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(NewBlockTagParser(DefaultTags), 999),
		),
	)

	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewBlockTagRenderer(e.view, e.link), 500),
		),
	)
}
