package media_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/links/media"
	"github.com/stretchr/testify/require"
)

func TestYoutubeParser_ValidURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		videoID string
	}{
		{
			"standard youtube.com URL",
			"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
		{
			"youtube.com without www",
			"https://youtube.com/watch?v=dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
		{
			"youtu.be short URL",
			"https://youtu.be/dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
		{
			"youtube.com with v parameter in middle",
			"https://www.youtube.com/watch?t=10&v=dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
		{
			"youtube.com embed URL with v",
			"https://www.youtube.com/embed/dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
		{
			"youtube.com /v/ format",
			"https://www.youtube.com/v/dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
		{
			"youtube-nocookie.com URL",
			"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
		{
			"youtube-nocookie.com without www",
			"https://youtube-nocookie.com/embed/dQw4w9WgXcQ",
			"dQw4w9WgXcQ",
		},
	}

	parser := &media.YoutubeParser{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.Parse(tt.url)
			require.NotNil(t, result, "parser should match URL: %s", tt.url)

			youtubeLink, ok := result.(*media.YoutubeLink)
			require.True(t, ok)
			require.Equal(t, tt.videoID, youtubeLink.ID)
		})
	}
}

func TestYoutubeParser_InvalidURLs(t *testing.T) {
	t.Parallel()

	tests := []string{
		"https://www.youtube.com/watch",                    // missing video ID
		"https://www.youtube.com/watch?t=10",               // missing v parameter
		"https://www.youtube.com/watch?v=",                 // empty video ID
		"https://example.com/watch?v=dQw4w9WgXcQ",          // not youtube
		"https://www.youtube.com/watch?v=abc",              // video ID too short (not 11 chars)
		"https://youtu.be/abc",                             // too short
		"https://www.youtube.com/results?search_query=foo", // search results
		"not a url",                // not even a URL
		"",                         // empty string
		"https://www.youtube.com/", // just the domain
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ extra", // with trailing space and text
		"https://youtu.be/dQw4w9WgXcQ?t=10",                 // query params after video ID in youtu.be
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10",  // query params after video ID in watch
	}

	parser := &media.YoutubeParser{}

	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			result := parser.Parse(url)
			require.Nil(t, result, "parser should not match: %s", url)
		})
	}
}

func TestYoutubeLink_Key(t *testing.T) {
	t.Parallel()

	link := &media.YoutubeLink{ID: "dQw4w9WgXcQ"}
	expected := "youtube: dQw4w9WgXcQ"

	require.Equal(t, expected, link.Key())
}

func TestYoutubeLink_URL(t *testing.T) {
	t.Parallel()

	link := &media.YoutubeLink{ID: "dQw4w9WgXcQ"}
	expected := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	require.Equal(t, expected, link.URL())
}

func TestYoutubeLink_EmbedCode(t *testing.T) {
	t.Parallel()

	link := &media.YoutubeLink{ID: "dQw4w9WgXcQ"}
	embedCode := link.EmbedCode()

	// Check that embed code contains the video ID
	require.Contains(t, string(embedCode), "dQw4w9WgXcQ")
	// Check for lite-youtube element
	require.Contains(t, string(embedCode), "lite-youtube")
	// Check for videoid attribute
	require.Contains(t, string(embedCode), "videoid=")
	// Check for expected attributes
	require.Contains(t, string(embedCode), "nocookie")
	require.Contains(t, string(embedCode), "disablenoscript")
}

func TestYoutubeLink_DifferentVideoIDs(t *testing.T) {
	t.Parallel()

	videoIDs := []string{
		"aqz-KE-bpKQ",
		"9bZkp7q19f0",
		"Xa0Q8IL_xqI",
	}

	for _, id := range videoIDs {
		t.Run(id, func(t *testing.T) {
			link := &media.YoutubeLink{ID: id}

			// Key should contain the video ID
			require.Contains(t, link.Key(), id)

			// URL should contain the video ID
			require.Contains(t, link.URL(), "v="+id)

			// EmbedCode should contain the video ID
			require.Contains(t, string(link.EmbedCode()), id)
		})
	}
}

func TestYoutubeParser_WithYoutubeParser_Integration(t *testing.T) {
	t.Parallel()

	parser := media.DefaultParser()

	// Test that DefaultParser can parse YouTube URLs
	result := parser.Parse("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	require.NotNil(t, result)

	youtubeLink, ok := result.(*media.YoutubeLink)
	require.True(t, ok)
	require.Equal(t, "dQw4w9WgXcQ", youtubeLink.ID)
}

func TestYoutubeLink_EmbedCode_Format(t *testing.T) {
	t.Parallel()

	link := &media.YoutubeLink{ID: "dQw4w9WgXcQ"}
	embedCode := string(link.EmbedCode())

	// Verify the exact format of the embed code
	expectedFormat := `<lite-youtube videoid="dQw4w9WgXcQ" playlabel="Play Video" nocookie disablenoscript></lite-youtube>`
	require.Equal(t, expectedFormat, embedCode)
}

func TestYoutubeParser_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("video ID with hyphen and underscore", func(t *testing.T) {
		// YouTube video IDs can use underscores and hyphens
		parser := &media.YoutubeParser{}
		result := parser.Parse("https://www.youtube.com/watch?v=_-dQw4w9WgX")
		require.NotNil(t, result)
		youtubeLink := result.(*media.YoutubeLink)
		require.Equal(t, "_-dQw4w9WgX", youtubeLink.ID)
	})

	t.Run("very long URL with multiple parameters before video ID", func(t *testing.T) {
		parser := &media.YoutubeParser{}
		url := "https://www.youtube.com/watch?list=PLxxxxx&index=5&v=dQw4w9WgXcQ"
		result := parser.Parse(url)
		require.NotNil(t, result)
		youtubeLink := result.(*media.YoutubeLink)
		require.Equal(t, "dQw4w9WgXcQ", youtubeLink.ID)
	})

	t.Run("youtube.com /e/ embed format", func(t *testing.T) {
		parser := &media.YoutubeParser{}
		result := parser.Parse("https://www.youtube.com/e/dQw4w9WgXcQ")
		require.NotNil(t, result)
	})
}
