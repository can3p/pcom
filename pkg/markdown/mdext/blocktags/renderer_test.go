package blocktags_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

func defaultLink(name string, args ...string) string {
	return "/" + name
}

func defaultMediaReplacer(in string) (bool, string) {
	return false, in
}

func TestBlockTagRenderer_Cut_SinglePostView(t *testing.T) {
	t.Parallel()

	input := `{cut}
This content should be visible in single post view
{/cut}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In single post view, cut should disappear (content shows but no wrapper)
	require.Contains(t, output, "This content should be visible in single post view")
	require.NotContains(t, output, "block-container-cut")
}

func TestBlockTagRenderer_Cut_FeedView(t *testing.T) {
	t.Parallel()

	input := `{cut}
Hidden in feed
{/cut}`

	result := markdown.ToEnrichedTemplate(input, types.ViewFeed, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In feed view, cut should render as a link
	require.Contains(t, output, "post-cut-link")
	require.Contains(t, output, `<a href="/single_post_special">`)
	require.NotContains(t, output, "Hidden in feed")
}

func TestBlockTagRenderer_Cut_FeedView_WithTitle(t *testing.T) {
	t.Parallel()

	input := `{cut Read More}
Hidden content
{/cut}`

	result := markdown.ToEnrichedTemplate(input, types.ViewFeed, defaultMediaReplacer, defaultLink)
	output := string(result)

	// Should use custom title
	require.Contains(t, output, "Read More")
}

func TestBlockTagRenderer_Cut_EmailView(t *testing.T) {
	t.Parallel()

	input := `{cut}
Hidden in email
{/cut}`

	result := markdown.ToEnrichedTemplate(input, types.ViewEmail, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In email view, cut should show a link
	require.Contains(t, output, "Check the rest of the post")
	require.Contains(t, output, `<a href="/single_post_special">`)
	require.NotContains(t, output, "Hidden in email")
}

func TestBlockTagRenderer_Spoiler_SinglePostView(t *testing.T) {
	t.Parallel()

	input := `{spoiler}
Secret content!
{/spoiler}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In single post view, spoiler should have controller
	require.Contains(t, output, "block-container-spoiler")
	require.Contains(t, output, `data-controller="spoiler"`)
	require.Contains(t, output, "Secret content!")
}

func TestBlockTagRenderer_Spoiler_EditPreviewView(t *testing.T) {
	t.Parallel()

	input := `{spoiler}
Spoiler text
{/spoiler}`

	result := markdown.ToEnrichedTemplate(input, types.ViewEditPreview, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In edit preview view, spoiler should NOT have controller
	require.Contains(t, output, "edit-preview-spoiler")
	require.NotContains(t, output, `data-controller="spoiler"`)
}

func TestBlockTagRenderer_Spoiler_FeedView(t *testing.T) {
	t.Parallel()

	input := `{spoiler}
Spoiler text in feed
{/spoiler}`

	result := markdown.ToEnrichedTemplate(input, types.ViewFeed, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In feed view, spoiler should be rendered as a normal block container
	require.Contains(t, output, "block-container-spoiler")
	require.Contains(t, output, "Spoiler text in feed")
}

func TestBlockTagRenderer_Spoiler_EmailView(t *testing.T) {
	t.Parallel()

	input := `{spoiler}
Spoiler content
{/spoiler}`

	result := markdown.ToEnrichedTemplate(input, types.ViewEmail, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In email view, spoiler should show a link
	require.Contains(t, output, "Got to the post page to expand spoiler")
	require.NotContains(t, output, "Spoiler content")
}

func TestBlockTagRenderer_Spoiler_WithTitle(t *testing.T) {
	t.Parallel()

	input := `{spoiler Custom Title}
Hidden content
{/spoiler}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLink)
	output := string(result)

	// Should use custom title
	require.Contains(t, output, "Custom Title")
}

func TestBlockTagRenderer_Gallery_FeedView(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![img1](https://example.com/1.jpg)
![img2](https://example.com/2.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewFeed, defaultMediaReplacer, defaultLink)
	output := string(result)

	// Gallery should have controller
	require.Contains(t, output, "block-container-gallery")
	require.Contains(t, output, `data-controller="gallery"`)
}

func TestBlockTagRenderer_Gallery_EmailView(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![img1](https://example.com/1.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewEmail, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In email view, gallery block should be skipped
	require.NotContains(t, output, "block-container-gallery")
}

func TestBlockTagRenderer_Gallery_EditPreviewView(t *testing.T) {
	t.Parallel()

	input := `{gallery}
![image](https://example.com/image.jpg)
{/gallery}`

	result := markdown.ToEnrichedTemplate(input, types.ViewEditPreview, defaultMediaReplacer, defaultLink)
	output := string(result)

	// In edit preview, gallery should have edit-preview prefix
	require.Contains(t, output, "edit-preview-gallery")
	require.Contains(t, output, `data-controller="gallery"`)
}

func TestBlockTagRenderer_MultipleBlockTags(t *testing.T) {
	t.Parallel()

	input := `{cut}
Intro text
{/cut}

Some visible content

{spoiler Spoiler}
Secret!
{/spoiler}

More content`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLink)
	output := string(result)

	// Both should be present
	require.Contains(t, output, "Intro text")
	require.Contains(t, output, "Secret!")
	require.Contains(t, output, "More content")
}

func TestBlockTagRenderer_NestedContent(t *testing.T) {
	t.Parallel()

	input := `{spoiler}
# Title
**Bold text** and *italic*
- List item
{/spoiler}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLink)
	output := string(result)

	// Should preserve nested markdown
	require.Contains(t, output, "<strong>Bold text</strong>")
	require.Contains(t, output, "<em>italic</em>")
	require.Contains(t, output, "<li>")
}

func TestBlockTagRenderer_EmptyBlockTag(t *testing.T) {
	t.Parallel()

	input := `{spoiler}
{/spoiler}`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLink)
	output := string(result)

	// Should still render the container even if empty
	require.Contains(t, output, "block-container-spoiler")
}
