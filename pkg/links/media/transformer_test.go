package media_test

import (
	"html/template"
	"testing"

	"github.com/can3p/pcom/pkg/links/media"
	"github.com/stretchr/testify/require"
)

// MockMediaLink is a test implementation of MediaLink interface
type MockMediaLink struct {
	key       string
	embedCode template.HTML
	url       string
}

func (m *MockMediaLink) EmbedCode() template.HTML {
	return m.embedCode
}

func (m *MockMediaLink) Key() string {
	return m.key
}

func (m *MockMediaLink) URL() string {
	return m.url
}

func TestMediaLinkSlice_Deduplicate(t *testing.T) {
	t.Parallel()

	t.Run("empty slice", func(t *testing.T) {
		slice := media.MediaLinkSlice{}
		result := slice.Deduplicate()
		require.Empty(t, result)
	})

	t.Run("single element", func(t *testing.T) {
		link := &MockMediaLink{key: "test-key", url: "https://example.com"}
		slice := media.MediaLinkSlice{link}
		result := slice.Deduplicate()

		require.Len(t, result, 1)
		require.Equal(t, "test-key", result[0].Key())
	})

	t.Run("all unique elements", func(t *testing.T) {
		links := []media.MediaLink{
			&MockMediaLink{key: "key1", url: "https://example1.com"},
			&MockMediaLink{key: "key2", url: "https://example2.com"},
			&MockMediaLink{key: "key3", url: "https://example3.com"},
		}
		slice := media.MediaLinkSlice(links)
		result := slice.Deduplicate()

		require.Len(t, result, 3)
		require.Equal(t, "key1", result[0].Key())
		require.Equal(t, "key2", result[1].Key())
		require.Equal(t, "key3", result[2].Key())
	})

	t.Run("duplicates are removed", func(t *testing.T) {
		link1 := &MockMediaLink{key: "youtube: abc123", url: "https://youtube.com/watch?v=abc123"}
		link2 := &MockMediaLink{key: "youtube: def456", url: "https://youtube.com/watch?v=def456"}
		link3 := &MockMediaLink{key: "youtube: abc123", url: "https://youtube.com/watch?v=abc123"}

		slice := media.MediaLinkSlice{link1, link2, link3}
		result := slice.Deduplicate()

		require.Len(t, result, 2)
		require.Equal(t, "youtube: abc123", result[0].Key())
		require.Equal(t, "youtube: def456", result[1].Key())
	})

	t.Run("maintains order of first occurrence", func(t *testing.T) {
		link1 := &MockMediaLink{key: "key-a", url: "url-a"}
		link2 := &MockMediaLink{key: "key-b", url: "url-b"}
		link3 := &MockMediaLink{key: "key-a", url: "url-a"}
		link4 := &MockMediaLink{key: "key-c", url: "url-c"}

		slice := media.MediaLinkSlice{link1, link2, link3, link4}
		result := slice.Deduplicate()

		require.Len(t, result, 3)
		require.Equal(t, "key-a", result[0].Key())
		require.Equal(t, "key-b", result[1].Key())
		require.Equal(t, "key-c", result[2].Key())
	})

	t.Run("multiple duplicates removed", func(t *testing.T) {
		link1 := &MockMediaLink{key: "key-x", url: "url-x"}
		link2 := &MockMediaLink{key: "key-y", url: "url-y"}
		link3 := &MockMediaLink{key: "key-x", url: "url-x"}
		link4 := &MockMediaLink{key: "key-y", url: "url-y"}
		link5 := &MockMediaLink{key: "key-z", url: "url-z"}

		slice := media.MediaLinkSlice{link1, link2, link3, link4, link5}
		result := slice.Deduplicate()

		require.Len(t, result, 3)
		require.Equal(t, "key-x", result[0].Key())
		require.Equal(t, "key-y", result[1].Key())
		require.Equal(t, "key-z", result[2].Key())
	})
}

type MockParser struct {
	shouldMatch bool
	result      media.MediaLink
}

func (p *MockParser) Parse(url string) media.MediaLink {
	if p.shouldMatch {
		return p.result
	}
	return nil
}

func TestAggregateParser_Parse(t *testing.T) {
	t.Parallel()

	t.Run("no parsers match", func(t *testing.T) {
		parser1 := &MockParser{shouldMatch: false}
		parser2 := &MockParser{shouldMatch: false}

		agg := &media.AggregateParser{
			Parsers: []media.MediaParser{parser1, parser2},
		}

		result := agg.Parse("https://example.com")
		require.Nil(t, result)
	})

	t.Run("first parser matches", func(t *testing.T) {
		link := &MockMediaLink{key: "test", url: "https://example.com"}
		parser1 := &MockParser{shouldMatch: true, result: link}
		parser2 := &MockParser{shouldMatch: true}

		agg := &media.AggregateParser{
			Parsers: []media.MediaParser{parser1, parser2},
		}

		result := agg.Parse("https://example.com")
		require.NotNil(t, result)
		require.Equal(t, "test", result.Key())
	})

	t.Run("second parser matches when first doesn't", func(t *testing.T) {
		link := &MockMediaLink{key: "second", url: "https://example.com"}
		parser1 := &MockParser{shouldMatch: false}
		parser2 := &MockParser{shouldMatch: true, result: link}

		agg := &media.AggregateParser{
			Parsers: []media.MediaParser{parser1, parser2},
		}

		result := agg.Parse("https://example.com")
		require.NotNil(t, result)
		require.Equal(t, "second", result.Key())
	})

	t.Run("stops at first match", func(t *testing.T) {
		link1 := &MockMediaLink{key: "first", url: "https://example.com"}
		link2 := &MockMediaLink{key: "second", url: "https://example.com"}
		parser1 := &MockParser{shouldMatch: true, result: link1}
		parser2 := &MockParser{shouldMatch: true, result: link2}

		agg := &media.AggregateParser{
			Parsers: []media.MediaParser{parser1, parser2},
		}

		result := agg.Parse("https://example.com")
		require.NotNil(t, result)
		require.Equal(t, "first", result.Key())
	})

	t.Run("empty parser list", func(t *testing.T) {
		agg := &media.AggregateParser{
			Parsers: []media.MediaParser{},
		}

		result := agg.Parse("https://example.com")
		require.Nil(t, result)
	})
}

func TestDefaultParser(t *testing.T) {
	t.Parallel()

	parser := media.DefaultParser()
	require.NotNil(t, parser)

	// DefaultParser should return an AggregateParser
	agg, ok := parser.(*media.AggregateParser)
	require.True(t, ok)

	// Should have at least one parser
	require.NotEmpty(t, agg.Parsers)
}
