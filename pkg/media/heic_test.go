package media_test

import (
	"bytes"
	_ "embed"
	"errors"
	"image/jpeg"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/can3p/pcom/pkg/media"
)

//go:embed testdata/photo.heic
var photoHEIC []byte

//go:embed testdata/photo.avif
var photoAVIF []byte

// requireJPEG checks out is a 40x24 JPEG.
func requireJPEG(t *testing.T, out []byte) {
	t.Helper()

	_, ctype, err := media.DetectAndValidateImageTypeAndExt(out)
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", ctype)

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, 40, cfg.Width)
	require.Equal(t, 24, cfg.Height)
}

// The converter reads any HEIF container; the AVIF photo stands in where
// libvips has a HEIF loader but no HEVC decoder.
func TestHEICToJPEG_HEIFContainer(t *testing.T) {
	t.Parallel()

	out, err := media.HEICToJPEG(photoAVIF)
	if errors.Is(err, media.ErrHEICUnsupported) {
		if os.Getenv("CI") != "" {
			t.Fatal("libvips has no HEIF loader: CI must decode HEIC (libheif with an HEVC decoder)")
		}

		t.Skip("libvips has no HEIF loader")
	}

	require.NoError(t, err)
	requireJPEG(t, out)
}

func TestHEICToJPEG(t *testing.T) {
	t.Parallel()

	out, err := media.HEICToJPEG(photoHEIC)
	if errors.Is(err, media.ErrHEICUnsupported) {
		if os.Getenv("CI") != "" {
			t.Fatal("libvips can't decode HEVC: CI must decode HEIC (libheif with an HEVC decoder)")
		}

		t.Skip("libvips can't decode HEVC")
	}

	require.NoError(t, err)
	requireJPEG(t, out)
}

func TestHEICToJPEG_Invalid(t *testing.T) {
	t.Parallel()

	_, err := media.HEICToJPEG([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00 not a photo"))
	require.Error(t, err)

	if errors.Is(err, media.ErrHEICUnsupported) {
		if os.Getenv("CI") != "" {
			t.Fatal("libvips has no HEIF loader: CI must decode HEIC (libheif with an HEVC decoder)")
		}

		t.Skip("libvips has no HEIF loader")
	}

	require.ErrorIs(t, err, media.ErrHEICInvalid)
}
