package markdown

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Test the ExtractImageUrls function from modify.go
func TestExtractImageURLs_ModifyPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "no images",
			input:    "Just text",
			expected: nil,
		},
		{
			name:  "single image",
			input: "![alt](https://example.com/image.jpg)",
			expected: []string{
				"https://example.com/image.jpg",
			},
		},
		{
			name: "multiple images",
			input: `![img1](https://example.com/1.jpg)

![img2](https://example.com/2.jpg)`,
			expected: []string{
				"https://example.com/1.jpg",
				"https://example.com/2.jpg",
			},
		},
		{
			name:  "image with complex URL",
			input: "![photo](https://cdn.example.com/path/to/image-2024-01-15.jpg?size=large&format=webp)",
			expected: []string{
				"https://cdn.example.com/path/to/image-2024-01-15.jpg?size=large&format=webp",
			},
		},
		{
			name:  "image with relative path",
			input: "![local](./images/photo.jpg)",
			expected: []string{
				"./images/photo.jpg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractImageUrls(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractImageUrls_InParagraph(t *testing.T) {
	t.Parallel()

	input := `Here is an image:

![screenshot](https://example.com/screenshot.png)

And text after it.`

	result := ExtractImageUrls(input)

	require.Len(t, result, 1)
	require.Equal(t, "https://example.com/screenshot.png", result[0])
}

func TestExtractImageUrls_InLinks(t *testing.T) {
	t.Parallel()

	input := `[![thumbnail](https://example.com/thumb.jpg)](https://example.com)`

	result := ExtractImageUrls(input)

	require.Len(t, result, 1)
	require.Equal(t, "https://example.com/thumb.jpg", result[0])
}

func TestExtractImageUrls_MixedContent(t *testing.T) {
	t.Parallel()

	input := `# Document

Text with [link](https://example.com)

![image1](https://example.com/img1.jpg)

More text

![image2](https://example.com/img2.jpg)

Final paragraph.`

	result := ExtractImageUrls(input)

	require.Len(t, result, 2)
	require.Equal(t, "https://example.com/img1.jpg", result[0])
	require.Equal(t, "https://example.com/img2.jpg", result[1])
}

func TestExtractImageUrls_EmptyString(t *testing.T) {
	t.Parallel()

	result := ExtractImageUrls("")
	require.Nil(t, result)
}

func TestExtractImageUrls_OnlyImages(t *testing.T) {
	t.Parallel()

	input := `![img1](url1.jpg)
![img2](url2.jpg)
![img3](url3.jpg)`

	result := ExtractImageUrls(input)

	require.Len(t, result, 3)
	require.Equal(t, []string{"url1.jpg", "url2.jpg", "url3.jpg"}, result)
}
