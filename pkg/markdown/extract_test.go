package markdown

import (
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestExtractImageUrls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "no images",
			input:    "Just some text without images",
			expected: []string{},
		},
		{
			name:  "single image",
			input: "![alt text](https://example.com/image.jpg)",
			expected: []string{
				"https://example.com/image.jpg",
			},
		},
		{
			name: "multiple images",
			input: `![image1](https://example.com/1.jpg)

![image2](https://example.com/2.png)

Some text

![image3](https://example.com/3.gif)`,
			expected: []string{
				"https://example.com/1.jpg",
				"https://example.com/2.png",
				"https://example.com/3.gif",
			},
		},
		{
			name:  "image with empty alt text",
			input: "![](https://example.com/image.jpg)",
			expected: []string{
				"https://example.com/image.jpg",
			},
		},
		{
			name:  "image with complex filename",
			input: "![photo](https://cdn.example.com/path/to/image-2024-01-15_preview.jpg)",
			expected: []string{
				"https://cdn.example.com/path/to/image-2024-01-15_preview.jpg",
			},
		},
		{
			name:  "image with query parameters",
			input: "![thumb](https://example.com/image.jpg?size=small&format=webp)",
			expected: []string{
				"https://example.com/image.jpg?size=small&format=webp",
			},
		},
		{
			name:  "image in link",
			input: "[![alt](https://example.com/image.jpg)](https://example.com)",
			expected: []string{
				"https://example.com/image.jpg",
			},
		},
		{
			name:  "relative image URLs",
			input: "![local](./images/photo.jpg)",
			expected: []string{
				"./images/photo.jpg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := Parse(tt.input, types.ViewSinglePost, func(in string) (bool, string) {
				return false, in
			}, func(name string, args ...string) string {
				return "/" + name
			})

			result := parsed.ExtractImageUrls()

			urls := make([]string, 0, len(result))
			for _, link := range result {
				urls = append(urls, link.URL)
			}

			require.Equal(t, tt.expected, urls)
		})
	}
}

func TestParseAndRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		shouldMatch []string
	}{
		{
			name:  "simple paragraph",
			input: "Hello world",
			shouldMatch: []string{
				"<p>Hello world</p>",
			},
		},
		{
			name:  "bold text",
			input: "This is **bold** text",
			shouldMatch: []string{
				"<strong>bold</strong>",
			},
		},
		{
			name:  "italic text",
			input: "This is *italic* text",
			shouldMatch: []string{
				"<em>italic</em>",
			},
		},
		{
			name:  "inline code",
			input: "Use `const x = 1` in JavaScript",
			shouldMatch: []string{
				"<code>const x = 1</code>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := Parse(tt.input, types.ViewSinglePost, func(in string) (bool, string) {
				return false, in
			}, func(name string, args ...string) string {
				return "/" + name
			})

			var buf strings.Builder
			err := parsed.Render(&buf)
			require.NoError(t, err)

			output := buf.String()
			for _, expected := range tt.shouldMatch {
				require.Contains(t, output, expected)
			}
		})
	}
}
