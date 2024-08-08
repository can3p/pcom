package markdown

import (
	"bytes"
	"html/template"

	"github.com/can3p/pcom/pkg/markdown/mdext"
	"github.com/can3p/pcom/pkg/markdown/mdext/blocktags"
	"github.com/can3p/pcom/pkg/markdown/mdext/headershift"
	"github.com/can3p/pcom/pkg/markdown/mdext/lazyload"
	"github.com/can3p/pcom/pkg/markdown/mdext/videoembed"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func NewParser(view types.HTMLView, mediaReplacer types.Replacer[string], link types.Link) goldmark.Markdown {
	extensions := []goldmark.Extender{
		extension.NewLinkify(
			extension.WithLinkifyAllowedProtocols([][]byte{
				[]byte("http:"),
				[]byte("https:"),
			}),
		),
		mdext.NewHandle(),
		videoembed.NewVideoEmbedExtender(),
		headershift.NewHeaderShiftExtender(1),
	}

	blockParsers := util.PrioritizedSlice{}

	nodeRenderers := util.PrioritizedSlice{
		// we let gallery block do it's own business
		util.Prioritized(lazyload.NewImgLazyLoadRenderer(mediaReplacer, func(n ast.Node) bool {
			t, ok := n.(*blocktags.BlockTag)

			if !ok {
				return false
			}

			return t.BlockTagName == "gallery"
		}), 500),
		util.Prioritized(mdext.NewUserHandleRenderer(
			func(in []byte) (bool, []byte) {
				return true, []byte(link("user", string(in)))
			}), 500),
	}

	if view == types.ViewEditPreview || view == types.ViewFeed || view == types.ViewSinglePost || view == types.ViewEmail {
		extensions = append(extensions, blocktags.NewBlockTagExtender(view, link))
	}

	return goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(
			parser.WithBlockParsers(blockParsers...),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(nodeRenderers...),
		),
	)
}

func ToEnrichedTemplate(s string, view types.HTMLView, mediaReplacer types.Replacer[string], link types.Link) template.HTML {
	text := Parse(s, view, mediaReplacer, link)

	var buf bytes.Buffer
	if err := text.Render(&buf); err != nil {
		panic(err)
	}

	return template.HTML(buf.String())
}
