package markdown

import (
	"bytes"
	"html/template"

	"github.com/can3p/pcom/pkg/links/media"
	"github.com/can3p/pcom/pkg/markdown/mdext"
	"github.com/can3p/pcom/pkg/markdown/mdext/blocktags"
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

func ToEnrichedTemplate(s string, view types.HTMLView, mediaReplacer types.Replacer[string], link types.Link) template.HTML {
	text := Parse(s, view, mediaReplacer, link)

	var buf bytes.Buffer
	if err := text.Render(&buf); err != nil {
		panic(err)
	}

	return template.HTML(buf.String())
}
