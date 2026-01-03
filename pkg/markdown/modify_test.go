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

func TestReplaceImageUrlsOrLinkify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		replacer ErrorReplacer
		expected string
	}{
		{
			name:  "successful replacement",
			input: "![alt text](https://example.com/image.jpg)",
			replacer: func(url string) (string, error) {
				return "abc", nil
			},
			expected: "![alt text](abc)",
		},
		{
			name:  "multiple images with mixed success and errors",
			input: "![ok](https://example.com/ok.jpg)\n\n![timeout](https://example.com/slow.jpg)\n\n![large](https://example.com/huge.jpg)",
			replacer: func(url string) (string, error) {
				switch url {
				case "https://example.com/ok.jpg":
					return "ok.jpg", nil
				case "https://example.com/slow.jpg":
					return "", errors.New("too slow")
				case "https://example.com/huge.jpg":
					return "", errors.New("too large")
				}
				return url, nil
			},
			expected: "![ok](ok.jpg)\n\n[timeout: too slow](https://example.com/slow.jpg)\n\n[large: too large](https://example.com/huge.jpg)",
		},
		{
			name:  "err for image with no caption uses url as caption",
			input: "![](https://example.com/fail.jpg)",
			replacer: func(url string) (string, error) {
				return "", errors.New("failed")
			},
			expected: "[https://example.com/fail.jpg: failed](https://example.com/fail.jpg)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ReplaceImageUrlsOrLinkify(tt.input, tt.replacer)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
