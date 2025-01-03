package blocktags

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func TestBlockTagsFullPost(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithBlockParsers(
				util.Prioritized(NewBlockTagParser(DefaultTags), 999)),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewBlockTagRenderer(types.ViewSinglePost, nil), 500),
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

{cut What we've been talking about}

this is hidden
`),
			out: `<p>this is test</p>
<p>this is hidden</p>`,
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
<p>this is hidden</p>
<p>this is not</p>`,
		},
		{
			in: []byte(`
this is test

{cut}

this is hidden

{/cut}`),
			out: `<p>this is test</p>
<p>this is hidden</p>`,
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
<div class="block-container-spoiler" data-controller="spoiler">
<div class="block-container-spoiler-summary">Open Spoiler</div>
<div class="block-container-spoiler-content">
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
<p>ttt</p>
<p>this is hidden</p>`,
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
<div class="block-container-spoiler" data-controller="spoiler">
<div class="block-container-spoiler-summary">Open Spoiler</div>
<div class="block-container-spoiler-content">
<div class="block-container-gallery" data-controller="gallery">
<div class="block-container-gallery-content">
<p>this is hidden</p>
</div>
</div>
</div>
</div>`,
		},
		{
			in: []byte(`
{gallery there is no title for gallery}
`),
			out: `<div class="block-container-gallery" data-controller="gallery">
<div class="block-container-gallery-content">
</div>
</div>`,
		},
	}

	for idx, ex := range examples {
		t.Run(string(ex.in), func(t *testing.T) {
			in := ex.in
			out := ex.out

			var writer bytes.Buffer

			_ = parser.Convert(in, &writer)

			assert.Equal(t, out, strings.TrimSpace(writer.String()), "[ex %d]:", idx+1)
		})
	}
}

func TestBlockTagsEditPreview(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithBlockParsers(
				util.Prioritized(NewBlockTagParser(DefaultTags), 999)),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewBlockTagRenderer(types.ViewEditPreview, nil), 500),
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

{cut What we've been talking about}

this is hidden
`),
			out: `<p>this is test</p>
<div class="block-container-edit-preview-cut">
<div class="block-container-edit-preview-cut-summary">What we've been talking about</div>
<div class="block-container-edit-preview-cut-content">
<p>this is hidden</p>
</div>
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
<div class="block-container-edit-preview-cut">
<div class="block-container-edit-preview-cut-summary">See more</div>
<div class="block-container-edit-preview-cut-content">
<p>this is hidden</p>
</div>
</div>
<p>this is not</p>`,
		},
		{
			in: []byte(`
this is test

{cut}

this is hidden

{/cut}`),
			out: `<p>this is test</p>
<div class="block-container-edit-preview-cut">
<div class="block-container-edit-preview-cut-summary">See more</div>
<div class="block-container-edit-preview-cut-content">
<p>this is hidden</p>
</div>
</div>`,
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
<div class="block-container-edit-preview-cut">
<div class="block-container-edit-preview-cut-summary">See more</div>
<div class="block-container-edit-preview-cut-content">
<div class="block-container-edit-preview-spoiler">
<div class="block-container-edit-preview-spoiler-summary">Open Spoiler</div>
<div class="block-container-edit-preview-spoiler-content">
<p>this is hidden</p>
</div>
</div>
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
<div class="block-container-edit-preview-cut">
<div class="block-container-edit-preview-cut-summary">See more</div>
<div class="block-container-edit-preview-cut-content">
<p>ttt</p>
<p>this is hidden</p>
</div>
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
<div class="block-container-edit-preview-cut">
<div class="block-container-edit-preview-cut-summary">See more</div>
<div class="block-container-edit-preview-cut-content">
<div class="block-container-edit-preview-spoiler">
<div class="block-container-edit-preview-spoiler-summary">Open Spoiler</div>
<div class="block-container-edit-preview-spoiler-content">
<div class="block-container-edit-preview-gallery" data-controller="gallery">
<div class="block-container-edit-preview-gallery-content">
<p>this is hidden</p>
</div>
</div>
</div>
</div>
</div>
</div>`,
		},
		{
			in: []byte(`
{gallery there is no title for gallery}
`),
			out: `<div class="block-container-edit-preview-gallery" data-controller="gallery">
<div class="block-container-edit-preview-gallery-content">
</div>
</div>`,
		},
	}

	for idx, ex := range examples {
		t.Run(string(ex.in), func(t *testing.T) {
			in := ex.in
			out := ex.out

			var writer bytes.Buffer

			_ = parser.Convert(in, &writer)

			assert.Equal(t, out, strings.TrimSpace(writer.String()), "[ex %d]:", idx+1)
		})
	}
}

func TestBlockTagsFeed(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithBlockParsers(
				util.Prioritized(NewBlockTagParser(DefaultTags), 999)),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewBlockTagRenderer(types.ViewFeed, func(name string, args ...string) string {
					return "https://test/" + name
				}), 500),
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

{cut What we've been talking about}

this is hidden
`),
			out: `<p>this is test</p>
<div class="post-cut-link"><a href="https://test/single_post_special">What we've been talking about</a></div>`,
		},
		{
			in: []byte(`
this is test

{cut What we've been talking about}

this is hidden

{/cut}

this is not
`),
			out: `<p>this is test</p>
<div class="post-cut-link"><a href="https://test/single_post_special">What we've been talking about</a></div>
<p>this is not</p>`,
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
