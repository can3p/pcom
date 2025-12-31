package markdown

import (
	"errors"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestReplaceImageUrls(t *testing.T) {
	src := `test

![IMG_2693.jpeg](rEplaceme.jpg)

wwww

![IMG_2693.jpeg](KeePME.jpg)`

	expected := `test

![IMG_2693.jpeg](replaced11111111111111111111111111111.jpg)

wwww

![IMG_2693.jpeg](keepme.jpg)`

	res := ReplaceImageUrls(src, ImportReplacer(
		map[string]string{
			"replaceme.jpg": "replaced11111111111111111111111111111.jpg",
		},
		map[string]struct{}{
			"keepme.jpg": {},
		},
	))

	assert.Equal(t, expected, res)
}

func TestReplaceImageUrlsOrErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		replacer ErrorReplacer
		expected string
	}{
		{
			name:  "successful replacement",
			input: "![alt text](https://example.com/image.jpg)",
			replacer: func(url string) (bool, string, error) {
				if url == "https://example.com/image.jpg" {
					return true, "/local/image.jpg", nil
				}
				return false, url, nil
			},
			expected: "![alt text](/local/image.jpg)",
		},
		{
			name:  "error replaces entire image with message",
			input: "![alt text](https://example.com/timeout.jpg)",
			replacer: func(url string) (bool, string, error) {
				if url == "https://example.com/timeout.jpg" {
					return true, "_[Image download timed out: https://example.com/timeout.jpg]_", errors.New("timeout")
				}
				return false, url, nil
			},
			expected: "*[Image download timed out: https://example.com/timeout.jpg]*",
		},
		{
			name:  "multiple images with mixed success and errors",
			input: "![ok](https://example.com/ok.jpg)\n\n![timeout](https://example.com/slow.jpg)\n\n![large](https://example.com/huge.jpg)",
			replacer: func(url string) (bool, string, error) {
				switch url {
				case "https://example.com/ok.jpg":
					return true, "/local/ok.jpg", nil
				case "https://example.com/slow.jpg":
					return true, "_[Image download timed out: https://example.com/slow.jpg]_", errors.New("timeout")
				case "https://example.com/huge.jpg":
					return true, "_[Image too large: https://example.com/huge.jpg]_", errors.New("too large")
				}
				return false, url, nil
			},
			expected: "![ok](/local/ok.jpg)\n\n*[Image download timed out: https://example.com/slow.jpg]*\n\n*[Image too large: https://example.com/huge.jpg]*",
		},
		{
			name:  "no replacement needed",
			input: "![img](https://example.com/keep.jpg)",
			replacer: func(url string) (bool, string, error) {
				return false, url, nil
			},
			expected: "![img](https://example.com/keep.jpg)",
		},
		{
			name:  "text with no images",
			input: "Just some text without images",
			replacer: func(url string) (bool, string, error) {
				return false, url, nil
			},
			expected: "Just some text without images",
		},
		{
			name:  "image limit exceeded error",
			input: "![img](https://example.com/image.jpg)",
			replacer: func(url string) (bool, string, error) {
				return true, "_[Image limit exceeded (20 max): https://example.com/image.jpg]_", errors.New("limit exceeded")
			},
			expected: "*[Image limit exceeded (20 max): https://example.com/image.jpg]*",
		},
		{
			name:  "generic download error",
			input: "![failed](https://example.com/broken.jpg)",
			replacer: func(url string) (bool, string, error) {
				return true, "_[Image download failed: https://example.com/broken.jpg]_", errors.New("network error")
			},
			expected: "*[Image download failed: https://example.com/broken.jpg]*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceImageUrlsOrErrorMessage(tt.input, tt.replacer)
			assert.Equal(t, tt.expected, result)
		})
	}
}
