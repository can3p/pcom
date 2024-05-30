package videoembed

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type VideoEmbedRenderer struct {
	html.Config
}

func NewVideoEmbedRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &VideoEmbedRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *VideoEmbedRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindVideoEmbed, r.renderVideoEmbed)
}

func (r *VideoEmbedRenderer) renderVideoEmbed(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*VideoEmbed)

	if !entering {
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(string(n.media.EmbedCode()))
	_ = w.WriteByte('\n')

	return ast.WalkSkipChildren, nil
}
