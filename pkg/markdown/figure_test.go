package markdown

import (
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

func renderFigures(t *testing.T, src string, view types.HTMLView) string {
	t.Helper()

	out := ToEnrichedTemplate(src, view, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	// drop the newlines between tags so the expectations stay readable
	return strings.ReplaceAll(string(out), "\n", "")
}

func TestFigures(t *testing.T) {
	t.Parallel()

	const (
		imgA = `<img class="lazyload post-img standalone-img" data-src="a.jpg" alt="a">`
		imgB = `<img class="lazyload post-img standalone-img" data-src="b.jpg" alt="b">`
		galA = `<img src="a.jpg" alt="a">`
		galB = `<img src="b.jpg" alt="b">`
		galC = `<img src="c.jpg" alt="c">`
		fig  = `<figure class="post-figure">`
		cap  = `<figcaption class="post-figure__caption">`
	)

	gallery := func(content string) string {
		return `<div class="block-container-gallery" data-controller="gallery"><div class="block-container-gallery-content">` +
			content + `</div></div>`
	}

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "the line after an image is its caption",
			src:  "![a](a.jpg)\nA *sunny* day\nat the sea\n\nNext paragraph",
			want: fig + imgA + cap + `<p>A <em>sunny</em> dayat the sea</p></figcaption></figure><p>Next paragraph</p>`,
		},
		{
			name: "an image alone is a figure without a caption",
			src:  "![a](a.jpg)",
			want: fig + imgA + `</figure>`,
		},
		{
			name: "a paragraph after a blank line is not a caption",
			src:  "![a](a.jpg)\n\nJust text",
			want: fig + imgA + `</figure><p>Just text</p>`,
		},
		{
			name: "every image alone on its line starts a figure",
			src:  "![a](a.jpg)\nfor a\n![b](b.jpg)\nfor b",
			want: fig + imgA + cap + `<p>for a</p></figcaption></figure>` + fig + imgB + cap + `<p>for b</p></figcaption></figure>`,
		},
		{
			name: "an image inside a sentence stays in the paragraph",
			src:  "look ![a](a.jpg)\nhere",
			want: `<p>look ` + imgA + `here</p>`,
		},
		{
			name: "lines before an image stay a paragraph",
			src:  "text first\n![a](a.jpg)\nmore",
			want: `<p>text first</p>` + fig + imgA + cap + `<p>more</p></figcaption></figure>`,
		},
		{
			name: "a linked image is not a figure",
			src:  "[![a](a.jpg)](https://example.com)\ntext",
			want: `<p><a href="https://example.com">` + imgA + `</a>text</p>`,
		},
		{
			name: "gallery: everything up to the next image is the caption",
			src:  "{gallery}\n![a](a.jpg)\nsame paragraph\n\nnext paragraph\n\n- a list\n\n![b](b.jpg)\n{/gallery}",
			want: gallery(fig + galA + cap + `<p>same paragraph</p><p>next paragraph</p><ul><li>a list</li></ul></figcaption></figure>` +
				fig + galB + `</figure>`),
		},
		{
			name: "gallery: text before the first image stays outside the figures",
			src:  "{gallery}\nleading\n\n![a](a.jpg)\n{/gallery}",
			want: gallery(`<p>leading</p>` + fig + galA + `</figure>`),
		},
		{
			name: "gallery: several images on one line each get a figure",
			src:  "{gallery}\n![a](a.jpg) ![b](b.jpg)\nfor b\n{/gallery}",
			want: gallery(fig + galA + `</figure>` + fig + galB + cap + `<p>for b</p></figcaption></figure>`),
		},
		{
			name: "gallery: text before an image in its paragraph belongs to the previous image",
			src:  "{gallery}\n![a](a.jpg)\n\nfor a\n![b](b.jpg)\nfor b\n{/gallery}",
			want: gallery(fig + galA + cap + `<p>for a</p></figcaption></figure>` + fig + galB + cap + `<p>for b</p></figcaption></figure>`),
		},
		{
			name: "gallery: images in a list are left to the controller",
			src:  "{gallery}\n![a](a.jpg)\n\n- ![b](b.jpg)\n\nafter the list\n\n![c](c.jpg)\n{/gallery}",
			want: gallery(fig + galA + `</figure><ul><li>` + galB + `</li></ul><p>after the list</p>` + fig + galC + `</figure>`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, renderFigures(t, tt.src, types.ViewSinglePost))
		})
	}
}

func TestFigures_PlainInMailAndRSS(t *testing.T) {
	t.Parallel()

	for _, view := range []types.HTMLView{types.ViewEmail, types.ViewRSS} {
		t.Run(string(view), func(t *testing.T) {
			t.Parallel()

			require.Equal(t,
				`<figure><img src="a.jpg" alt="a"><figcaption><p>caption</p></figcaption></figure>`,
				renderFigures(t, "![a](a.jpg)\ncaption", view))
		})
	}
}

func TestFigures_NotInComments(t *testing.T) {
	t.Parallel()

	require.NotContains(t, renderFigures(t, "![a](a.jpg)\ncaption", types.ViewComment), "<figure")
}

func TestFigures_KeepImagesForExtraction(t *testing.T) {
	t.Parallel()

	urls := Parse("{gallery}\n![a](a.jpg)\nfor a\n\n![b](b.jpg)\n{/gallery}\n\n![c](c.jpg)\ncaption", types.ViewSinglePost, nil, nil).ExtractImageUrls()

	require.Len(t, urls, 3)
	require.Equal(t, []string{"a.jpg", "b.jpg", "c.jpg"}, []string{urls[0].URL, urls[1].URL, urls[2].URL})
}

func TestFigures_ProtectDestinationOfImageWithoutAlt(t *testing.T) {
	t.Parallel()

	src := "Intro\n\n![](a.jpg)\nthe caption"
	start := strings.Index(src, "a.jpg")

	require.Contains(t, ProtectedRanges(src), Range{start, start + len("a.jpg")})
}
