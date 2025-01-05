package validation

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "empty url",
			input:       "",
			expectError: false,
		},
		{
			name:        "invalid url - no scheme",
			input:       "example.com",
			expectError: true,
		},
		{
			name:        "invalid url - malformed",
			input:       "http://[::1]:namedport",
			expectError: true,
		},
		{
			name:        "valid url - http",
			input:       "http://example.com",
			expectError: false,
		},
		{
			name:        "valid url - https with path",
			input:       "https://example.com/path",
			expectError: false,
		},
		{
			name:        "valid url - with query params",
			input:       "https://example.com/path?param=value",
			expectError: false,
		},
		{
			name:        "valid url - with fragment",
			input:       "https://example.com/path#section",
			expectError: false,
		},
		{
			name:        "valid url - with port",
			input:       "https://example.com:8080/path",
			expectError: false,
		},
		{
			name:        "valid url - localhost",
			input:       "http://localhost:3000",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
