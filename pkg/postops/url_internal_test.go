package postops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// isURLMediaUpload is unexported, so this file pins its logic directly
// (docs/testing.md ground rule 4).
func TestIsURLMediaUpload(t *testing.T) {
	t.Parallel()

	require.True(t, isURLMediaUpload("3fa85f64-5717-4562-b3fc-2c963f66afa6.png"))
	require.True(t, isURLMediaUpload("3fa85f64-5717-4562-b3fc-2c963f66afa6.thumb.png"))
	require.False(t, isURLMediaUpload("not-a-uuid.png"))
	require.False(t, isURLMediaUpload("https://example.com/image.png"))
	require.False(t, isURLMediaUpload("no-extension-at-all"))
}
