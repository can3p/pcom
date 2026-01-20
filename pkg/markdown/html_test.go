package markdown

import (
	"testing"

	"github.com/can3p/pcom/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestFeedViewLinksOpenInNewTab(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "regular markdown link",
			input: "[example](https://example.com)",
			contains: []string{
				`<a href="https://example.com" target="_blank" rel="noopener noreferrer">example</a>`,
			},
		},
		{
			name:  "autolinked URL",
			input: "Check out https://example.com for more info",
			contains: []string{
				`<a href="https://example.com" target="_blank" rel="noopener noreferrer">https://example.com</a>`,
			},
		},
		{
			name:  "link with title",
			input: `[example](https://example.com "Example Site")`,
			contains: []string{
				`<a href="https://example.com" title="Example Site" target="_blank" rel="noopener noreferrer">example</a>`,
			},
		},
		{
			name:  "multiple links",
			input: "[first](https://first.com) and [second](https://second.com)",
			contains: []string{
				`<a href="https://first.com" target="_blank" rel="noopener noreferrer">first</a>`,
				`<a href="https://second.com" target="_blank" rel="noopener noreferrer">second</a>`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToEnrichedTemplate(tt.input, types.ViewFeed, func(in string) (bool, string) {
				return false, in
			}, func(name string, args ...string) string {
				return "/" + name
			})

			output := string(result)
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected, "Output should contain expected HTML")
			}
		})
	}
}

func TestNonFeedViewLinksDoNotOpenInNewTab(t *testing.T) {
	tests := []struct {
		name string
		view types.HTMLView
	}{
		{
			name: "single post view",
			view: types.ViewSinglePost,
		},
		{
			name: "edit preview view",
			view: types.ViewEditPreview,
		},
		{
			name: "comment view",
			view: types.ViewComment,
		},
	}

	input := "[example](https://example.com)"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToEnrichedTemplate(input, tt.view, func(in string) (bool, string) {
				return false, in
			}, func(name string, args ...string) string {
				return "/" + name
			})

			output := string(result)
			// Should NOT contain target="_blank"
			assert.NotContains(t, output, `target="_blank"`, "Non-feed views should not open links in new tab")
			// Should still contain the link
			assert.Contains(t, output, `<a href="https://example.com">example</a>`, "Should contain regular link")
		})
	}
}

func TestFeedViewPreservesOtherMarkdownFeatures(t *testing.T) {
	input := `# Heading

This is a paragraph with a [link](https://example.com).

- List item 1
- List item 2

**Bold text** and *italic text*.`

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Verify link has target="_blank"
	assert.Contains(t, output, `target="_blank"`)

	// Verify other markdown features are preserved
	assert.Contains(t, output, `<h2>Heading</h2>`) // h2 because of header shift
	assert.Contains(t, output, `<ul>`)
	assert.Contains(t, output, `<li>List item 1</li>`)
	assert.Contains(t, output, `<strong>Bold text</strong>`)
	assert.Contains(t, output, `<em>italic text</em>`)
}

func TestFeedViewEmailAutolink(t *testing.T) {
	input := "<user@example.com>"

	result := ToEnrichedTemplate(input, types.ViewFeed, func(in string) (bool, string) {
		return false, in
	}, func(name string, args ...string) string {
		return "/" + name
	})

	output := string(result)

	// Email links should also open in new tab
	assert.Contains(t, output, `<a href="mailto:user@example.com" target="_blank" rel="noopener noreferrer">user@example.com</a>`)
}
