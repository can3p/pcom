package blocktags

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type BlockTagRenderer struct {
	html.Config
}

func NewBlockTagRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &BlockTagRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *BlockTagRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindBlockTag, r.renderBlockTag)
}

func (r *BlockTagRenderer) renderBlockTag(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*BlockTag)
	if entering {
		_, _ = w.WriteString(`<div data-container-type="` + n.BlockTagName + `">` + "\n")
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}
