package markdown

import (
	"bytes"
	"html/template"

	"github.com/can3p/pcom/pkg/links/media"
	"github.com/can3p/pcom/pkg/markdown/mdext"
	"github.com/can3p/pcom/pkg/markdown/mdext/blocktags"
	"github.com/can3p/pcom/pkg/markdown/mdext/videoembed"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

func NewParser(view types.HTMLView, mediaReplacer Replacer, link types.Link) goldmark.Markdown {
	extensions := []goldmark.Extender{
		extension.NewLinkify(
			extension.WithLinkifyAllowedProtocols([][]byte{
				[]byte("http:"),
				[]byte("https:"),
			}),
		),
		mdext.NewHandle(),
	}

	blockParsers := util.PrioritizedSlice{}

	nodeRenderers := util.PrioritizedSlice{
		util.Prioritized(NewImgLazyLoadRenderer(mediaReplacer), 500),
		util.Prioritized(mdext.NewUserHandleRenderer(
			func(in []byte) (bool, []byte) {
				return true, []byte(link("user", string(in)))
			}), 500),
		util.Prioritized(videoembed.NewVideoEmbedRenderer(), 500),
	}

	paragraphTransformers := util.PrioritizedSlice{
		util.Prioritized(videoembed.NewVideoEmbedTransformer(media.DefaultParser()), 500),
	}

	if view == types.ViewEditPreview || view == types.ViewFeed || view == types.ViewSinglePost {
		blockParsers = append(blockParsers, util.Prioritized(blocktags.NewBlockTagParser(blocktags.DefaultTags), 999))
		nodeRenderers = append(nodeRenderers, util.Prioritized(blocktags.NewBlockTagRenderer(view, link), 500))
	}

	return goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(
			parser.WithBlockParsers(blockParsers...),
			parser.WithParagraphTransformers(paragraphTransformers...),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(nodeRenderers...),
		),
	)
}

func ToEnrichedTemplate(s string, view types.HTMLView, mediaReplacer Replacer, link types.Link) template.HTML {
	text := Parse(s, view, mediaReplacer, link)

	var buf bytes.Buffer
	if err := text.Render(&buf); err != nil {
		panic(err)
	}

	return template.HTML(buf.String())
}

func NewImgLazyLoadRenderer(mediaReplacer Replacer, opts ...html.Option) renderer.NodeRenderer {
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
	mediaReplacer Replacer
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
