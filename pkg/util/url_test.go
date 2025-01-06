package util

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty url",
			input:       "",
			expectError: true,
			errorMsg:    "URL cannot be empty",
		},
		{
			name:        "invalid url",
			input:       "http://[::1]:namedport",
			expectError: true,
			errorMsg:    "invalid URL format",
		},
		{
			name:        "missing protocol",
			input:       "example.com",
			expectError: true,
			errorMsg:    "URL must include a protocol",
		},
		{
			name:        "missing domain",
			input:       "https:///path",
			expectError: true,
			errorMsg:    "URL must include a domain name",
		},
		{
			name:        "relative path",
			input:       "/path/to/something",
			expectError: true,
			errorMsg:    "URL must include a protocol",
		},
		{
			name:     "simple url",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "capitalised case in the url",
			input:    "hTTps://eXAMple.Com/ABC",
			expected: "https://example.com/ABC",
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
		{
			name:     "url with ip address",
			input:    "https://192.168.1.1/path",
			expected: "https://192.168.1.1/path",
		},
		{
			name:     "url with port",
			input:    "https://example.com:8080/path",
			expected: "https://example.com:8080/path",
		},
		{
			name:     "url with auth",
			input:    "https://user:pass@example.com/path",
			expected: "https://user:pass@example.com/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURL(tt.input)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
