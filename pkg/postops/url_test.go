package postops

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty url",
			input:    "",
			expected: "",
		},
		{
			name:     "invalid url",
			input:    "not a url",
			expected: "",
		},
		{
			name:     "simple url",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "url with trailing slash",
			input:    "https://example.com/path/",
			expected: "https://example.com/path",
		},
		{
			name:     "url with trailing question mark",
			input:    "https://example.com/path?",
			expected: "https://example.com/path",
		},
		{
			name:     "url with query parameters",
			input:    "https://example.com/path?b=2&a=1",
			expected: "https://example.com/path?a=1&b=2",
		},
		{
			name:     "url with multiple values for same parameter",
			input:    "https://example.com/path?a=2&a=1",
			expected: "https://example.com/path?a=1&a=2",
		},
		{
			name:     "url with multiple parameters and values",
			input:    "https://example.com/path?c=3&b=2&b=1&a=2&a=1",
			expected: "https://example.com/path?a=1&a=2&b=1&b=2&c=3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
