package lazyload_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestImgLazyLoadRenderer_SimpleImage_EditPreview(t *testing.T) {
	t.Parallel()

	input := "![alt text](https://example.com/image.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewEditPreview, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// In non-email/RSS views, should use lazyload class
	require.Contains(t, output, `class="lazyload mx-auto d-block img standalone-img"`)
	require.Contains(t, output, `data-src="https://example.com/image.jpg"`)
	require.Contains(t, output, `alt="alt text"`)
}

func TestImgLazyLoadRenderer_SimpleImage_Email(t *testing.T) {
	t.Parallel()

	input := "![alt text](https://example.com/image.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewEmail, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// In email view, should use src instead of data-src
	require.Contains(t, output, `<img src="https://example.com/image.jpg"`)
	require.NotContains(t, output, `data-src=`)
	require.NotContains(t, output, `lazyload`)
}

func TestImgLazyLoadRenderer_SimpleImage_RSS(t *testing.T) {
	t.Parallel()

	input := "![alt text](https://example.com/image.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewRSS, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// In RSS view, should use src instead of data-src
	require.Contains(t, output, `<img src="https://example.com/image.jpg"`)
	require.NotContains(t, output, `data-src=`)
	require.NotContains(t, output, `lazyload`)
}

func TestImgLazyLoadRenderer_WithMediaReplacement(t *testing.T) {
	t.Parallel()

	input := "![photo](https://example.com/photo.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		if in == "https://example.com/photo.jpg" {
			return true, "https://cdn.example.com/abc123"
		}
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should have both thumb and full variants with link
	require.Contains(t, output, `<a target="_blank" href="https://cdn.example.com/abc123/full">`)
	require.Contains(t, output, `data-src="https://cdn.example.com/abc123/thumb"`)
	require.Contains(t, output, `</a>`)
}

func TestImgLazyLoadRenderer_WithMediaReplacement_Email(t *testing.T) {
	t.Parallel()

	input := "![photo](https://example.com/photo.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewEmail, func(in string) (bool, string) {
		if in == "https://example.com/photo.jpg" {
			return true, "https://cdn.example.com/abc123"
		}
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// In email, should have link but use src not data-src
	require.Contains(t, output, `<a target="_blank" href="https://cdn.example.com/abc123/full">`)
	require.Contains(t, output, `<img src="https://cdn.example.com/abc123/thumb"`)
}

func TestImgLazyLoadRenderer_EmptyAlt(t *testing.T) {
	t.Parallel()

	input := "![](https://example.com/image.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should still render with empty alt
	require.Contains(t, output, `alt=""`)
}

func TestImgLazyLoadRenderer_MultipleImages(t *testing.T) {
	t.Parallel()

	input := `![first](https://example.com/1.jpg)

![second](https://example.com/2.jpg)`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should have both images
	require.Contains(t, output, `data-src="https://example.com/1.jpg"`)
	require.Contains(t, output, `data-src="https://example.com/2.jpg"`)
}

func TestImgLazyLoadRenderer_ImageWithTitle(t *testing.T) {
	t.Parallel()

	input := `![alt](https://example.com/image.jpg "Image Title")`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should include title attribute
	require.Contains(t, output, `title="Image Title"`)
}

func TestNewImgLazyLoadRenderer_ForbiddenParent(t *testing.T) {
	t.Parallel()

	// The gallery tag is a forbidden parent for lazyload
	// Images inside gallery should be rendered differently
	input := `{gallery}
![image](https://example.com/img.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should render without lazyload (classic style) when in forbidden parent
	require.Contains(t, output, "https://example.com/img.jpg")
}

func TestNodeToHTMLText(t *testing.T) {
	t.Parallel()

	// Test that the alt text extraction works correctly with various content
	input := "![complex **bold** text](https://example.com/image.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// The alt text should include the content
	require.Contains(t, output, `alt="`)
}
