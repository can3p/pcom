package links_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/stretchr/testify/require"
)

func TestMediaReplacer_ValidMedia_UUID_JPG(t *testing.T) {
	t.Parallel()

	site := links.Site{Root: "https://example.com"}

	valid, url := site.MediaReplacer("3fa85f64-5717-4562-b3fc-2c963f66afa6.jpg")
	require.True(t, valid)
	require.Equal(t, "https://example.com/user-media/3fa85f64-5717-4562-b3fc-2c963f66afa6.jpg", url)
}

func TestMediaReplacer_ValidMedia_UUID_PNG(t *testing.T) {
	t.Parallel()

	site := links.Site{Root: "https://example.com"}

	valid, url := site.MediaReplacer("a1b2c3d4-e5f6-7890-abcd-ef1234567890.png")
	require.True(t, valid)
	require.Equal(t, "https://example.com/user-media/a1b2c3d4-e5f6-7890-abcd-ef1234567890.png", url)
}

func TestMediaReplacer_ValidMedia_UUID_GIF(t *testing.T) {
	t.Parallel()

	site := links.Site{Root: "https://example.com"}

	valid, url := site.MediaReplacer("12345678-1234-1234-1234-123456789012.gif")
	require.True(t, valid)
	require.Equal(t, "https://example.com/user-media/12345678-1234-1234-1234-123456789012.gif", url)
}

func TestMediaReplacer_InvalidFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"no extension", "3fa85f64-5717-4562-b3fc-2c963f66afa6"},
		{"too many dots", "file.with.multiple.dots.jpg"},
		{"invalid UUID", "not-a-uuid.jpg"},
		{"empty string", ""},
		{"just extension", ".jpg"},
		{"just UUID", "3fa85f64-5717-4562-b3fc-2c963f66afa6"},
		{"three parts", "part1.part2.part3"},
		{"invalid UUID format in filename", "invalid-uuid-format.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, url := links.Site{}.MediaReplacer(tt.input)
			require.False(t, valid)
			require.Equal(t, "", url)
		})
	}
}

func TestMediaReplacer_WithCDN(t *testing.T) {
	t.Parallel()

	site := links.Site{Root: "https://example.com", MediaCDN: "https://cdn.example.com"}

	valid, url := site.MediaReplacer("3fa85f64-5717-4562-b3fc-2c963f66afa6.jpg")
	require.True(t, valid)
	require.Equal(t, "https://cdn.example.com/3fa85f64-5717-4562-b3fc-2c963f66afa6.jpg", url)
}
