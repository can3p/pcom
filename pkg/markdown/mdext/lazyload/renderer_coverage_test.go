package lazyload_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

// Test to ensure renderImageClassic is called for images in forbidden parents
func TestImgLazyLoadRenderer_ImageInGallery_NoMediaReplacement(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![image](https://example.com/image.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Image in gallery should use classic rendering (no lazyload class, no data-src)
	require.NotContains(t, output, `class="lazyload`)
	require.NotContains(t, output, `data-src=`)
	require.Contains(t, output, `<img src="https://example.com/image.jpg"`)
}

func TestImgLazyLoadRenderer_ImageInGallery_WithMediaReplacement(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![img](https://example.com/photo.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		if in == "https://example.com/photo.jpg" {
			return true, "https://cdn.example.com/xyz"
		}
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Image in gallery with replacement should have data-full attribute
	require.Contains(t, output, `<img src="https://cdn.example.com/xyz/thumb"`)
	require.Contains(t, output, `data-full="https://cdn.example.com/xyz/full"`)
	require.NotContains(t, output, `data-src=`)
}

func TestImgLazyLoadRenderer_ComplexAltText(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![**bold** and *italic*](https://example.com/image.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Alt text should be properly extracted even with nested markdown
	require.Contains(t, output, `alt="`)
}

func TestImgLazyLoadRenderer_ImageInGallery_EditPreview(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![img](https://example.com/image.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewEditPreview, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Gallery in edit preview should use classic rendering
	require.NotContains(t, output, `data-src=`)
}

func TestImgLazyLoadRenderer_LazyLoadInEditPreview(t *testing.T) {
	t.Parallel()

	input := "![image](https://example.com/image.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewEditPreview, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Regular image in edit preview should use lazyload
	require.Contains(t, output, `class="lazyload post-img standalone-img"`)
	require.Contains(t, output, `data-src="https://example.com/image.jpg"`)
}

func TestImgLazyLoadRenderer_MultipleImagesInGallery(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![img1](https://example.com/1.jpg)

![img2](https://example.com/2.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Both images in gallery should use classic rendering
	require.Contains(t, output, `<img src="https://example.com/1.jpg"`)
	require.Contains(t, output, `<img src="https://example.com/2.jpg"`)
	require.NotContains(t, output, `data-src=`)
}

func TestImgLazyLoadRenderer_XHTMLMode(t *testing.T) {
	t.Parallel()

	// Test XHTML-style self-closing tags
	input := "![image](https://example.com/image.jpg)"

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should be self-closing in HTML5 style (just >)
	require.Contains(t, output, `>`)
}

func TestImgLazyLoadRenderer_EmptyAltInGallery(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![](https://example.com/image.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should have empty alt even in gallery
	require.Contains(t, output, `alt=""`)
}

func TestImgLazyLoadRenderer_ImageWithTitleInGallery(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![alt](https://example.com/image.jpg "Title text")
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should include title attribute even in classic rendering
	require.Contains(t, output, `title="Title text"`)
}
