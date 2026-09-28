package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSplitLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single line",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "two lines",
			input:    "hello\nworld",
			expected: []string{"hello", "world"},
		},
		{
			name:     "multiple lines",
			input:    "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "lines with trailing newline",
			input:    "line1\nline2\n",
			expected: []string{"line1", "line2"},
		},
		{
			name:     "lines with empty content",
			input:    "line1\n\nline3",
			expected: []string{"line1", "", "line3"},
		},
		{
			name:     "lines with whitespace",
			input:    "  hello  \n  world  ",
			expected: []string{"  hello  ", "  world  "},
		},
		{
			name:     "single newline",
			input:    "\n",
			expected: []string{""},
		},
		{
			name:     "multiple newlines",
			input:    "\n\n",
			expected: []string{"", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitLines(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "0m 0s",
		},
		{
			name:     "seconds only",
			duration: 30 * time.Second,
			expected: "0m 30s",
		},
		{
			name:     "one minute",
			duration: 1 * time.Minute,
			expected: "1m 0s",
		},
		{
			name:     "minutes and seconds",
			duration: 5*time.Minute + 30*time.Second,
			expected: "5m 30s",
		},
		{
			name:     "one hour",
			duration: 1 * time.Hour,
			expected: "1h 0m 0s",
		},
		{
			name:     "hour and minutes",
			duration: 1*time.Hour + 30*time.Minute,
			expected: "1h 30m 0s",
		},
		{
			name:     "hour, minutes and seconds",
			duration: 1*time.Hour + 30*time.Minute + 45*time.Second,
			expected: "1h 30m 45s",
		},
		{
			name:     "multiple hours",
			duration: 3*time.Hour + 15*time.Minute + 20*time.Second,
			expected: "3h 15m 20s",
		},
		{
			name:     "59 minutes 59 seconds",
			duration: 59*time.Minute + 59*time.Second,
			expected: "59m 59s",
		},
		{
			name:     "large duration",
			duration: 24*time.Hour + 12*time.Minute + 5*time.Second,
			expected: "24h 12m 5s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.duration)
			require.Equal(t, tt.expected, result)
		})
	}
}
