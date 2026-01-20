package linkrenderer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func TestLinkRenderer_WithNewTab(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithExtensions(
			extension.NewLinkify(),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewLinkRenderer(true), 500),
			),
		),
	)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "regular link",
			input:    "[example](https://example.com)",
			expected: `<a href="https://example.com" target="_blank" rel="noopener noreferrer">example</a>`,
		},
		{
			name:     "link with title",
			input:    `[example](https://example.com "Example Site")`,
			expected: `<a href="https://example.com" title="Example Site" target="_blank" rel="noopener noreferrer">example</a>`,
		},
		{
			name:     "autolink",
			input:    "https://example.com",
			expected: `<a href="https://example.com" target="_blank" rel="noopener noreferrer">https://example.com</a>`,
		},
		{
			name:     "email autolink",
			input:    "<user@example.com>",
			expected: `<a href="mailto:user@example.com" target="_blank" rel="noopener noreferrer">user@example.com</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := parser.Convert([]byte(tt.input), &buf)
			assert.NoError(t, err)

			output := strings.TrimSpace(buf.String())
			// Remove wrapping <p> tags for comparison
			output = strings.TrimPrefix(output, "<p>")
			output = strings.TrimSuffix(output, "</p>")

			assert.Equal(t, tt.expected, output)
		})
	}
}

func TestLinkRenderer_WithoutNewTab(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithExtensions(
			extension.NewLinkify(),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewLinkRenderer(false), 500),
			),
		),
	)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "regular link",
			input:    "[example](https://example.com)",
			expected: `<a href="https://example.com">example</a>`,
		},
		{
			name:     "autolink",
			input:    "https://example.com",
			expected: `<a href="https://example.com">https://example.com</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := parser.Convert([]byte(tt.input), &buf)
			assert.NoError(t, err)

			output := strings.TrimSpace(buf.String())
			// Remove wrapping <p> tags for comparison
			output = strings.TrimPrefix(output, "<p>")
			output = strings.TrimSuffix(output, "</p>")

			assert.Equal(t, tt.expected, output)
		})
	}
}
