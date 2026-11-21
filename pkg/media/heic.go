package media

import (
	"errors"
	"fmt"
	"strings"

	"github.com/davidbyttow/govips/v2/vips"
)

// heicJPEGQuality is the JPEG quality a converted HEIC photo is encoded at:
// high enough that a phone photo shows no loss, well below the 100 that
// only grows the file.
const heicJPEGQuality = 90

// ErrHEICUnsupported means this build of libvips has no HEIF loader, or no
// decoder for the codec of the photo (HEIC is HEVC), so it can't be converted.
var ErrHEICUnsupported = errors.New("libvips can't decode HEIC")

// ErrHEICInvalid means the bytes are not a HEIC photo libvips can decode.
var ErrHEICInvalid = errors.New("not a readable HEIC photo")

// HEICToJPEG converts a HEIC/HEIF photo to JPEG: the EXIF orientation is
// applied to the pixels and all metadata (GPS included) is dropped.
// It returns ErrHEICUnsupported when libvips has no HEIF loader and
// ErrHEICInvalid when the bytes don't decode.
func HEICToJPEG(heic []byte) ([]byte, error) {
	if !vips.IsTypeSupported(vips.ImageTypeHEIF) {
		return nil, ErrHEICUnsupported
	}

	img, err := vips.NewImageFromBuffer(heic)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHEICInvalid, err)
	}
	defer img.Close()

	if err := img.AutoRotate(); err != nil {
		return nil, fmt.Errorf("autorotate: %w", err)
	}

	params := vips.NewJpegExportParams()
	params.Quality = heicJPEGQuality
	params.StripMetadata = true

	// libvips decodes lazily, so a missing codec shows up here
	out, _, err := img.ExportJpeg(params)
	if err != nil {
		// libvips says "Unsupported"; libheif without the codec's plugin says
		// "No decoding plugin installed"
		if msg := err.Error(); strings.Contains(msg, "Unsupported") || strings.Contains(msg, "No decoding plugin") {
			return nil, fmt.Errorf("%w: %w", ErrHEICUnsupported, err)
		}

		return nil, fmt.Errorf("%w: %w", ErrHEICInvalid, err)
	}

	return out, nil
}
