package media_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
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

func TestHandleUpload_ArgumentValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userID    *string
		rssFeedID *string
		desc      string
	}{
		{
			name:      "both_nil",
			userID:    nil,
			rssFeedID: nil,
			desc:      "both userID and rssFeedID are nil",
		},
		{
			name:      "both_provided",
			userID:    new("user123"),
			rssFeedID: new("feed456"),
			desc:      "both userID and rssFeedID are provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// For argument validation tests, we don't need to provide a real executor
			// since the error is returned early before the executor is used.
			// We use nil as a sentinel to ensure early return.
			var nilExec boil.ContextExecutor
			mockStorage := fakestorage.New()

			// Create a simple reader with valid image data
			pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
			reader := bytes.NewReader(pngData)

			_, err := media.HandleUpload(context.Background(), nilExec, mockStorage, tt.userID, tt.rssFeedID, reader)

			require.Error(t, err, "expected error when "+tt.desc)
			require.Contains(t, err.Error(), "exactly one of")
		})
	}
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestHandleUpload_ReaderError(t *testing.T) {
	t.Parallel()

	_, err := media.HandleUpload(context.Background(), nil, fakestorage.New(), new("user123"), nil, failReader{})
	require.Error(t, err)
}

func TestHandleUpload_InvalidImageType(t *testing.T) {
	t.Parallel()

	userID := new("user123")
	mockStorage := fakestorage.New()

	// Create a reader with data that detects as text, not an image
	textData := []byte("This is plain text, not an image")
	reader := bytes.NewReader(textData)

	_, err := media.HandleUpload(context.Background(), nil, mockStorage, userID, nil, reader)

	// Should fail because text is not a supported image type
	require.Error(t, err)
	require.ErrorIs(t, err, media.ErrUnsupportedMimeType)
}

// Note: Full integration tests for HandleUpload success paths are left for W2
// because they require testdb.New(t) which needs Docker/Postgres testcontainers.
// W1 focuses on unit tests: argument validation, image type validation, and
// error cases that don't require database access.
