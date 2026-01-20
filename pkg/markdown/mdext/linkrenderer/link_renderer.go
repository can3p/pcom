package linkrenderer

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type LinkRenderer struct {
	html.Config
	openInNewTab bool
}

func NewLinkRenderer(openInNewTab bool, opts ...html.Option) renderer.NodeRenderer {
	r := &LinkRenderer{
		Config:       html.NewConfig(),
		openInNewTab: openInNewTab,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *LinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
}

func (r *LinkRenderer) renderLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if entering {
		_, _ = w.WriteString("<a href=\"")
		if r.Unsafe || !html.IsDangerousURL(n.Destination) {
			_, _ = w.Write(util.EscapeHTML(util.URLEscape(n.Destination, true)))
		}
		_ = w.WriteByte('"')
		if n.Title != nil {
			_, _ = w.WriteString(` title="`)
			r.Writer.Write(w, n.Title)
			_ = w.WriteByte('"')
		}
		if r.openInNewTab {
			_, _ = w.WriteString(` target="_blank" rel="noopener noreferrer"`)
		}
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

func (r *LinkRenderer) renderAutoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("<a href=\"")
	url := n.URL(source)
	label := n.Label(source)
	if n.AutoLinkType == ast.AutoLinkEmail && len(url) >= 7 && string(url[:7]) != "mailto:" {
		_, _ = w.WriteString("mailto:")
	}
	_, _ = w.Write(util.EscapeHTML(util.URLEscape(url, false)))
	if r.openInNewTab {
		_, _ = w.WriteString(`" target="_blank" rel="noopener noreferrer">`)
	} else {
		_, _ = w.WriteString(`">`)
	}
	_, _ = w.Write(util.EscapeHTML(label))
	_, _ = w.WriteString("</a>")
	return ast.WalkContinue, nil
}
