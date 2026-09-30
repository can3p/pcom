package media_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/can3p/pcom/pkg/media"
)

func TestValidateImageType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		wantExt     string
		wantErr     bool
	}{
		{
			name:        "valid PNG",
			contentType: "image/png",
			wantExt:     ".png",
			wantErr:     false,
		},
		{
			name:        "valid JPEG",
			contentType: "image/jpeg",
			wantExt:     ".jpg",
			wantErr:     false,
		},
		{
			name:        "valid WebP",
			contentType: "image/webp",
			wantExt:     ".webp",
			wantErr:     false,
		},
		{
			name:        "unsupported GIF",
			contentType: "image/gif",
			wantExt:     "",
			wantErr:     true,
		},
		{
			name:        "unsupported SVG",
			contentType: "image/svg+xml",
			wantExt:     "",
			wantErr:     true,
		},
		{
			name:        "text content type",
			contentType: "text/plain",
			wantExt:     "",
			wantErr:     true,
		},
		{
			name:        "empty content type",
			contentType: "",
			wantExt:     "",
			wantErr:     true,
		},
		{
			name:        "invalid format",
			contentType: "application/json",
			wantExt:     "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ext, err := media.ValidateImageType(tt.contentType)

			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, "", ext)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantExt, ext)
			}
		})
	}
}
