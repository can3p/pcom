package videoembed_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/links/media"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestVideoEmbedRenderer_YouTube_EditPreviewView(t *testing.T) {
	t.Parallel()

	input := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewEditPreview, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// In non-email/RSS views, should have embed code
	require.Contains(t, output, "lite-youtube")
	require.Contains(t, output, `videoid="dQw4w9WgXcQ"`)
}

func TestVideoEmbedRenderer_YouTube_EmailView(t *testing.T) {
	t.Parallel()

	input := "Check this video: https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewEmail, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// In email view, should be a plain link
	require.Contains(t, output, `<a href="https://www.youtube.com/watch?v=dQw4w9WgXcQ"`)
	require.NotContains(t, output, "lite-youtube")
}

func TestVideoEmbedRenderer_YouTube_RSSView(t *testing.T) {
	t.Parallel()

	input := "Check this video: https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewRSS, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// In RSS view, should be a plain link
	require.Contains(t, output, `<a href="https://www.youtube.com/watch?v=dQw4w9WgXcQ"`)
	require.NotContains(t, output, "lite-youtube")
}

func TestVideoEmbedRenderer_YouTubeShortenedURL(t *testing.T) {
	t.Parallel()

	input := "https://youtu.be/dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should recognize and embed shortened YouTube URLs
	require.Contains(t, output, "lite-youtube")
	require.Contains(t, output, `videoid="dQw4w9WgXcQ"`)
}

func TestVideoEmbedRenderer_YouTubeNoCookie(t *testing.T) {
	t.Parallel()

	input := "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should handle nocookie variant
	require.Contains(t, output, "lite-youtube")
}

func TestVideoEmbedRenderer_FeedView(t *testing.T) {
	t.Parallel()

	input := `https://www.youtube.com/watch?v=dQw4w9WgXcQ

More text after`

	result := markdown.ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should have embed code in feed
	require.Contains(t, output, "lite-youtube")
	require.Contains(t, output, "More text after")
}

func TestVideoEmbedRenderer_NoAutoplayAttribute(t *testing.T) {
	t.Parallel()

	input := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// lite-youtube should have nocookie and disablenoscript attributes
	require.Contains(t, output, `nocookie`)
	require.Contains(t, output, `disablenoscript`)
	require.Contains(t, output, `playlabel="Play Video"`)
}

func TestVideoEmbedRenderer_CommentView(t *testing.T) {
	t.Parallel()

	input := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewComment, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Comment view should have the embed
	require.Contains(t, output, "lite-youtube")
}

func TestVideoEmbedRenderer_ArticleView(t *testing.T) {
	t.Parallel()

	input := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	result := markdown.ToEnrichedTemplate(input, types.ViewArticle, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Article view should have the embed
	require.Contains(t, output, "lite-youtube")
}

func TestVideoEmbedRenderer_InParagraph(t *testing.T) {
	t.Parallel()

	input := `Here is a video for you:

https://www.youtube.com/watch?v=dQw4w9WgXcQ

That was nice.`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Video should be in a paragraph
	require.Contains(t, output, "<p>")
	require.Contains(t, output, "lite-youtube")
	require.Contains(t, output, "</p>")
	require.Contains(t, output, "That was nice")
}

func TestVideoEmbedRenderer_YouTubeID_Validation(t *testing.T) {
	t.Parallel()

	// Test that invalid YouTube IDs are not recognized
	input := "Not a video: https://www.youtube.com/watch?v=short"

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Should not embed invalid YouTube URL
	require.NotContains(t, output, "lite-youtube")
	// But should still have it as a link or text
	require.Contains(t, output, "youtube")
}

func TestYoutubeLink_EmbedCode(t *testing.T) {
	t.Parallel()

	parser := &media.YoutubeParser{}
	link := parser.Parse("https://www.youtube.com/watch?v=dQw4w9WgXcQ")

	require.NotNil(t, link)
	require.Equal(t, "youtube: dQw4w9WgXcQ", link.Key())
	require.Equal(t, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", link.URL())

	embedCode := link.EmbedCode()
	embedStr := string(embedCode)
	require.Contains(t, embedStr, "lite-youtube")
	require.Contains(t, embedStr, "dQw4w9WgXcQ")
}

func TestYoutubeLink_URL(t *testing.T) {
	t.Parallel()

	parser := &media.YoutubeParser{}
	link := parser.Parse("https://youtu.be/dQw4w9WgXcQ")

	require.NotNil(t, link)
	require.Equal(t, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", link.URL())
}

func TestVideoEmbedRenderer_WithOtherContent(t *testing.T) {
	t.Parallel()

	input := `# Video Post

Watch this **amazing** video:

https://www.youtube.com/watch?v=dQw4w9WgXcQ

Hope you enjoyed it!`

	result := markdown.ToEnrichedTemplate(input, types.ViewSinglePost, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// All elements should be present
	require.Contains(t, output, "<h2>Video Post</h2>") // h2 due to header shift
	require.Contains(t, output, "<strong>amazing</strong>")
	require.Contains(t, output, "lite-youtube")
	require.Contains(t, output, "Hope you enjoyed it!")
}
