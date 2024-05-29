package blocktags

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldmarkText "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func TestBlockTags(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithBlockParsers(
				util.Prioritized(NewBlockTagParser(DefaultTags), 999)),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewBlockTagRenderer(), 500),
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

{cut}

this is hidden
`),
			out: `<p>this is test</p>
<div data-container-type="cut">
<p>this is hidden</p>
</div>`,
		},
		{
			in: []byte(`
this is test

{cut}

this is hidden

{/cut}

this is not
`),
			out: `<p>this is test</p>
<div data-container-type="cut">
<p>this is hidden</p>
</div>
<p>this is not</p>`,
		},
		{
			in: []byte(`
this is test

{cut}

{spoiler}
this is hidden
{/spoiler}

{/cut}

this is not
`),
			out: `<p>this is test</p>
<div data-container-type="cut">
<div data-container-type="spoiler">
<p>this is hidden</p>
</div>
</div>
<p>this is not</p>`,
		},
		{
			in: []byte(`
this is test

{cut}

ttt

{cut}
this is hidden
`),
			out: `<p>this is test</p>
<div data-container-type="cut">
<p>ttt</p>
<p>this is hidden</p>
</div>`,
		},
		{
			in: []byte(`
this is test

{cut}

{spoiler}
{gallery}
this is hidden
`),
			out: `<p>this is test</p>
<div data-container-type="cut">
<div data-container-type="spoiler">
<div data-container-type="gallery">
<p>this is hidden</p>
</div>
</div>
</div>`,
		},
	}

	for idx, ex := range examples {
		in := ex.in
		out := ex.out

		r := goldmarkText.NewReader(in)
		ast := parser.Parser().Parse(r)

		var writer bytes.Buffer

		_ = parser.Renderer().Render(&writer, in, ast)

		assert.Equal(t, out, strings.TrimSpace(writer.String()), "[ex %d]:", idx+1)
	}
}
