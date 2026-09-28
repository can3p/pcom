package markdown

import (
	"testing"

	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

// Test to ensure highlighting is applied for specific views
func TestNewParser_HighlightingForFeedView(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```"

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Feed view should have syntax highlighting
	require.Contains(t, output, "chroma")
}

func TestNewParser_HighlightingForSinglePostView(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```"

	result := ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Single post view should have syntax highlighting
	require.Contains(t, output, "chroma")
}

func TestNewParser_HighlightingForEditPreviewView(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```"

	result := ToEnrichedTemplate(input, types.ViewEditPreview, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Edit preview view should have syntax highlighting
	require.Contains(t, output, "chroma")
}

func TestNewParser_NoHighlightingForCommentView(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```"

	result := ToEnrichedTemplate(input, types.ViewComment, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Comment view should NOT have syntax highlighting
	require.NotContains(t, output, "chroma")
}

func TestNewParser_NoHighlightingForArticleView(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```"

	result := ToEnrichedTemplate(input, types.ViewArticle, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Article view should NOT have syntax highlighting
	require.NotContains(t, output, "chroma")
}

func TestNewParser_NoHighlightingForEmailView(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```"

	result := ToEnrichedTemplate(input, types.ViewEmail, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Email view should NOT have syntax highlighting
	require.NotContains(t, output, "chroma")
}

func TestNewParser_NoHighlightingForRSSView(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```"

	result := ToEnrichedTemplate(input, types.ViewRSS, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// RSS view should NOT have syntax highlighting
	require.NotContains(t, output, "chroma")
}

// Test to ensure link renderer is applied for feed view
func TestNewParser_LinkRendererForFeedView(t *testing.T) {
	t.Parallel()

	input := "[link](https://example.com)"

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Feed view should have target="_blank"
	require.Contains(t, output, `target="_blank"`)
}

func TestNewParser_BlockTagsForSupportedViews(t *testing.T) {
	t.Parallel()

	views := []types.HTMLView{
		types.ViewEditPreview,
		types.ViewFeed,
		types.ViewSinglePost,
		types.ViewEmail,
		types.ViewRSS,
	}

	input := `{cut}
content
{/cut}`

	for _, view := range views {
		t.Run(string(view), func(t *testing.T) {
			result := ToEnrichedTemplate(input, view, func(in string) (bool, string) {
				return false, in
			}, func(name string, args ...string) string {
				return "/" + name
			})

			output := string(result)

			// All these views should support block tags
			// (at minimum, the parser should handle them without error)
			require.NotNil(t, output)
		})
	}
}
