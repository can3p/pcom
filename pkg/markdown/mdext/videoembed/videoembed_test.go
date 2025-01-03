package videoembed

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/can3p/pcom/pkg/links/media"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func TestVideoEmbed(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithParagraphTransformers(
				util.Prioritized(NewVideoEmbedTransformer(media.DefaultParser()), 999)),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewVideoEmbedRenderer(types.ViewArticle), 500),
			),
		),
	)

	examples := []struct {
		in  []byte
		out string
	}{
		{
			in: []byte(`
this is test

https://www.youtube.com/watch?v=J2dQCd_kzkA

this is hidden
`),
			out: `<p>this is test</p>
<p><lite-youtube videoid="J2dQCd_kzkA" playlabel="Play Video" nocookie></lite-youtube></p>
<p>this is hidden</p>`,
		},
		{
			in: []byte(`
this is test

this video is a part of paragraph and should be left alone https://www.youtube.com/watch?v=J2dQCd_kzkA

this is hidden
`),
			out: `<p>this is test</p>
<p>this video is a part of paragraph and should be left alone https://www.youtube.com/watch?v=J2dQCd_kzkA</p>
<p>this is hidden</p>`,
		},
	}

	for idx, ex := range examples {
		in := ex.in
		out := ex.out

		var writer bytes.Buffer

		_ = parser.Convert(in, &writer)

		assert.Equal(t, out, strings.TrimSpace(writer.String()), "[ex %d]:", idx+1)
	}
}
