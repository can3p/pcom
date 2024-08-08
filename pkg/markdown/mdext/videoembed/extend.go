package videoembed

import (
	"github.com/can3p/pcom/pkg/links/media"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

type videoEmbed struct {
	view types.HTMLView
}

// NewVideoEmbed creates a new [goldmark.Extender] that
// allow you to parse text that seems like a @uservideoEmbed
func NewVideoEmbedExtender(view types.HTMLView) goldmark.Extender {
	return &videoEmbed{
		view: view,
	}
}

func (e *videoEmbed) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithParagraphTransformers(
			util.Prioritized(NewVideoEmbedTransformer(media.DefaultParser()), 500),
		),
	)

	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewVideoEmbedRenderer(e.view), 500),
		),
	)
}
