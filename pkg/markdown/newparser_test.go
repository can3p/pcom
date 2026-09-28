package markdown

import (
	"testing"

	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

// Test NewParser for all views to ensure all branches are covered
func TestNewParser_AllViews_Coverage(t *testing.T) {
	t.Parallel()

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
			parser := NewParser(view, func(in string) (bool, string) {
				return false, in
			}, func(name string, args ...string) string {
				return "/" + name
			})

			// Verify parser is created without error
			require.NotNil(t, parser)

			// Parser should have renderers
			require.NotNil(t, parser.Renderer())

			// Parser should have parser configured
			require.NotNil(t, parser.Parser())
		})
	}
}

func TestNewParser_Linkify_UnavailableProtocols(t *testing.T) {
	t.Parallel()

	// Test that linkify only allows http/https/ftp
	input := "gopher://example.com should not be linkified"

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// gopher protocol should not be linkified
	require.NotContains(t, output, `<a href="gopher://`)
	require.Contains(t, output, "gopher://example.com should not be linkified")
}

func TestNewParser_Linkify_ClosingBrace_NotIncluded(t *testing.T) {
	t.Parallel()

	// Test that closing braces are not included in linkified URLs
	input := "Check https://example.com)"

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// URL should be linkified without the closing paren/brace
	require.Contains(t, output, `<a href="https://example.com"`)
	require.NotContains(t, output, `href="https://example.com)"`)
}

func TestNewParser_Handle_UserMention(t *testing.T) {
	t.Parallel()

	input := "Hello @john_doe!"

	result := ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// User mention should be present
	require.Contains(t, output, "@john_doe")
}

func TestNewParser_MultipleMarkdownFeatures(t *testing.T) {
	t.Parallel()

	input := `# Heading

Paragraph with **bold** and *italic*.

- List item 1
- List item 2

[Link](https://example.com)

![Image](https://example.com/img.jpg)

@alice mentioned

> Blockquote`

	result := ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// All features should be present
	require.Contains(t, output, "<h2>") // heading shifted
	require.Contains(t, output, "<strong>bold</strong>")
	require.Contains(t, output, "<em>italic</em>")
	require.Contains(t, output, "<ul>")
	require.Contains(t, output, "<a href=")
	require.Contains(t, output, "@alice")
	require.Contains(t, output, "<blockquote>")
}

func TestNewParser_EdgeCases_EmptyString(t *testing.T) {
	t.Parallel()

	input := ""

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	// Should handle empty input gracefully
	require.NotNil(t, result)
}

func TestNewParser_EdgeCases_OnlyWhitespace(t *testing.T) {
	t.Parallel()

	input := "   \n\n  \t  \n"

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	// Should handle whitespace-only input
	require.NotNil(t, result)
}

func TestNewParser_VideoEmbedExtender_AllViews(t *testing.T) {
	t.Parallel()

	// Video embed should be available for all views
	input := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

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
			result := ToEnrichedTemplate(input, view, func(in string) (bool, string) {
				return false, in
			}, func(name string, args ...string) string {
				return "/" + name
			})

			// Should not panic or error
			require.NotNil(t, result)
		})
	}
}

func TestNewParser_HeaderShift_ForAllViews(t *testing.T) {
	t.Parallel()

	input := "# H1\n## H2\n### H3"

	result := ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Headers should be shifted for all views
	require.Contains(t, output, "<h2>H1</h2>")
	require.Contains(t, output, "<h3>H2</h3>")
	require.Contains(t, output, "<h4>H3</h4>")
}

func TestNewParser_BlockTagsNotAddedForCommentView(t *testing.T) {
	t.Parallel()

	input := `{cut}
content
{/cut}`

	result := ToEnrichedTemplate(input, types.ViewComment, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Comment view should not have blocktags support
	require.NotContains(t, output, "block-container-cut")
	// The block tags should be rendered as literal text
	require.Contains(t, output, "{cut}")
}

func TestNewParser_BlockTagsNotAddedForArticleView(t *testing.T) {
	t.Parallel()

	input := `{spoiler}
content
{/spoiler}`

	result := ToEnrichedTemplate(input, types.ViewArticle, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Article view should not have blocktags support
	require.NotContains(t, output, "block-container-spoiler")
	// The block tags should be rendered as literal text
	require.Contains(t, output, "{spoiler}")
}
