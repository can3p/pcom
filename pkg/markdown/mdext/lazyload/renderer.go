package lazyload

import (
	"bytes"

	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type ParentChecker func(n ast.Node) bool

func NewImgLazyLoadRenderer(view types.HTMLView, mediaReplacer types.Replacer[string], forbiddenParent ParentChecker, opts ...html.Option) renderer.NodeRenderer {
	r := &LazyLoadRenderer{
		Config:          html.NewConfig(),
		view:            view,
		mediaReplacer:   mediaReplacer,
		forbiddenParent: forbiddenParent,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

type LazyLoadRenderer struct {
	html.Config
	view            types.HTMLView
	mediaReplacer   types.Replacer[string]
	forbiddenParent ParentChecker
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

	p := node

	for {
		p = p.Parent()

		if p == nil {
			break
		}

		if r.forbiddenParent(p) {
			return r.renderImageClassic(w, source, node, entering)
		}
	}

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
	if r.view == types.ViewEmail || r.view == types.ViewRSS {
		_, _ = w.WriteString("<img src=\"")
	} else {
		_, _ = w.WriteString("<img class=\"lazyload mx-auto d-block img standalone-img\" data-src=\"")
	}
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
			buf.Write(s.Value)
		} else if !c.HasChildren() {
			buf.Write(util.EscapeHTML(c.Text(source))) //nolint:staticcheck
		} else {
			buf.Write(nodeToHTMLText(c, source))
		}
	}
	return buf.Bytes()
}

// copied from github.com/yuin/goldmark@v1.5.4/renderer/renderer.go
// I wish there was a way to fallback to default renderer
func (r *LazyLoadRenderer) renderImageClassic(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	shouldReplace, updatedLink := r.mediaReplacer(string(n.Destination))

	// just in case media replacer forgets to return initial string
	if !shouldReplace {
		updatedLink = string(n.Destination)
	}

	_, _ = w.WriteString("<img src=\"")
	if r.Unsafe || !html.IsDangerousURL([]byte(updatedLink)) {
		_, _ = w.Write(util.EscapeHTML(util.URLEscape([]byte(updatedLink), true)))
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
	return ast.WalkSkipChildren, nil
}
