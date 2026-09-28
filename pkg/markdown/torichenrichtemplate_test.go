package markdown

import (
	"fmt"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

func defaultMediaReplacer(in string) (bool, string) {
	return false, in
}

func defaultLinkBuilder(name string, args ...string) string {
	return "/" + name
}

func TestToEnrichedTemplate_AllViews(t *testing.T) {
	t.Parallel()

	input := `# Main Heading

This is a paragraph with **bold** and *italic* text.

## Subheading

Here's a [link](https://example.com) and an autolink: https://example.com

- List item 1
- List item 2
  - Nested item

An image: ![alt text](https://example.com/image.jpg)

Here's some inline code: code example

User handle: @john_doe

> A blockquote with some text`

	views := []types.HTMLView{
		types.ViewFeed,
		types.ViewSinglePost,
		types.ViewEditPreview,
		types.ViewComment,
		types.ViewArticle,
		types.ViewEmail,
		types.ViewRSS,
	}

	for _, view := range views {
		t.Run(string(view), func(t *testing.T) {
			result := ToEnrichedTemplate(input, view, defaultMediaReplacer, defaultLinkBuilder)
			golden.Assert(t, fmt.Sprintf("enriched_%s", view), []byte(string(result)))
		})
	}
}

func TestToEnrichedTemplate_WithMediaReplacement(t *testing.T) {
	t.Parallel()

	input := `Image in post: ![photo](https://example.com/photo.jpg)`

	mediaReplacer := func(in string) (bool, string) {
		if in == "https://example.com/photo.jpg" {
			return true, "https://cdn.example.com/12345abcde"
		}
		return false, in
	}

	result := ToEnrichedTemplate(input, types.ViewSinglePost, mediaReplacer, defaultLinkBuilder)
	output := string(result)

	// The lazyload renderer should replace the image URL with the CDN URL
	require.Contains(t, output, "https://cdn.example.com/12345abcde/thumb")
}

func TestToEnrichedTemplate_UserHandles(t *testing.T) {
	t.Parallel()

	input := `Hello @alice and @bob_smith, check this out @user123!`

	result := ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLinkBuilder)
	output := string(result)

	// User handles should be rendered through the link builder
	require.Contains(t, output, "@alice")
	require.Contains(t, output, "@bob_smith")
	require.Contains(t, output, "@user123")
}

func TestToEnrichedTemplate_EmptyInput(t *testing.T) {
	t.Parallel()

	result := ToEnrichedTemplate("", types.ViewSinglePost, defaultMediaReplacer, defaultLinkBuilder)
	// Should not panic and return empty or minimal output
	require.NotNil(t, result)
}

func TestToEnrichedTemplate_HeadingShift(t *testing.T) {
	t.Parallel()

	input := `# H1
## H2
### H3`

	result := ToEnrichedTemplate(input, types.ViewSinglePost, defaultMediaReplacer, defaultLinkBuilder)
	output := string(result)

	// Headers should be shifted by one level (H1 -> H2, etc.)
	require.Contains(t, output, "<h2>")
	require.Contains(t, output, "<h3>")
	require.Contains(t, output, "<h4>")
	require.NotContains(t, output, "<h1>")
}

func TestToEnrichedTemplate_FeedViewLinkTarget(t *testing.T) {
	t.Parallel()

	input := `[example](https://example.com)`

	result := ToEnrichedTemplate(input, types.ViewFeed, defaultMediaReplacer, defaultLinkBuilder)
	output := string(result)

	// Feed view should have target="_blank"
	require.Contains(t, output, `target="_blank"`)
}

func TestToEnrichedTemplate_NonFeedViewLinkTarget(t *testing.T) {
	t.Parallel()

	input := `[example](https://example.com)`

	views := []types.HTMLView{
		types.ViewSinglePost,
		types.ViewEditPreview,
		types.ViewComment,
		types.ViewArticle,
		types.ViewEmail,
		types.ViewRSS,
	}

	for _, view := range views {
		t.Run(string(view), func(t *testing.T) {
			result := ToEnrichedTemplate(input, view, defaultMediaReplacer, defaultLinkBuilder)
			output := string(result)

			// Non-feed views should NOT have target="_blank"
			require.NotContains(t, output, `target="_blank"`)
		})
	}
}
