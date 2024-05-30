package lazyload

import (
	"bytes"

	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

func NewImgLazyLoadRenderer(mediaReplacer types.Replacer[string], opts ...html.Option) renderer.NodeRenderer {
	r := &LazyLoadRenderer{
		Config:        html.NewConfig(),
		mediaReplacer: mediaReplacer,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

type LazyLoadRenderer struct {
	html.Config
	mediaReplacer types.Replacer[string]
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *LazyLoadRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.renderImage)
}

func (r *LazyLoadRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.Image)

	shouldReplace, updatedLink := r.mediaReplacer(string(n.Destination))

	imgUrl := string(n.Destination)

	if shouldReplace {
		imgUrl = updatedLink + "?class=thumb"
		linkUrl := updatedLink + "?class=full"

		_, _ = w.WriteString("<a target=\"_blank\" href=\"")
		if r.Unsafe || !html.IsDangerousURL([]byte(linkUrl)) {
			_, _ = w.Write(util.EscapeHTML(util.URLEscape([]byte(linkUrl), true)))
		}
		_, _ = w.WriteString("\">")
	}
	_, _ = w.WriteString("<img class=\"lazyload\" data-src=\"")
	if r.Unsafe || !html.IsDangerousURL([]byte(imgUrl)) {
		_, _ = w.Write(util.EscapeHTML(util.URLEscape([]byte(imgUrl), true)))
	}
	_, _ = w.WriteString(`" alt="`)
	_, _ = w.Write(nodeToHTMLText(n, source))
	_ = w.WriteByte('"')
	if n.Title != nil {
		_, _ = w.WriteString(` title="`)
		r.Writer.Write(w, n.Title)
		_ = w.WriteByte('"')
	}
	if n.Attributes() != nil {
		html.RenderAttributes(w, n, html.ImageAttributeFilter)
	}
	if r.XHTML {
		_, _ = w.WriteString(" />")
	} else {
		_, _ = w.WriteString(">")
	}

	if shouldReplace {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkSkipChildren, nil
}

func nodeToHTMLText(n ast.Node, source []byte) []byte {
	var buf bytes.Buffer
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s, ok := c.(*ast.String); ok && s.IsCode() {
			buf.Write(s.Text(source))
		} else if !c.HasChildren() {
			buf.Write(util.EscapeHTML(c.Text(source)))
		} else {
			buf.Write(nodeToHTMLText(c, source))
		}
	}
	return buf.Bytes()
}
