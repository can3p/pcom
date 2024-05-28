package mdext

import (
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

func NewUserHandleRenderer(linkifier types.Replacer[[]byte], opts ...html.Option) renderer.NodeRenderer {
	r := &UserHandleRenderer{
		Config:    html.NewConfig(),
		linkifier: linkifier,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

type UserHandleRenderer struct {
	html.Config
	linkifier types.Replacer[[]byte]
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *UserHandleRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindUserHandle, r.renderUserHandle)
}

func (r *UserHandleRenderer) renderUserHandle(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*UserHandle)
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<a href="`)
	username := n.UserName(source)

	_, url := r.linkifier(username)

	label := n.Label(source)
	_, _ = w.Write(util.EscapeHTML(util.URLEscape(url, false)))
	_, _ = w.WriteString(`" class="user-handle">`)
	_, _ = w.Write(util.EscapeHTML(label))
	_, _ = w.WriteString(`</a>`)
	return ast.WalkContinue, nil
}
