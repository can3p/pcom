package videoembed

import (
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type VideoEmbedRenderer struct {
	html.Config
	view types.HTMLView
}

func NewVideoEmbedRenderer(view types.HTMLView, opts ...html.Option) renderer.NodeRenderer {
	r := &VideoEmbedRenderer{
		Config: html.NewConfig(),
		view:   view,
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

	if r.view == types.ViewEmail {
		_, _ = w.WriteString(`<p><a href="` + n.media.URL() + `">` + n.media.URL() + "</a></p>\n")
	} else {
		_, _ = w.WriteString(string(n.media.EmbedCode()))
		_ = w.WriteByte('\n')
	}

	return ast.WalkSkipChildren, nil
}
