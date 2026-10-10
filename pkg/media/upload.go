package media

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrNotFound            = errors.New("Resource not found") //nolint:staticcheck // ST1005: message text kept as is
	ErrUnsupportedMimeType = errors.New("unsupported mime type")
)

var supportedImageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
}

func ValidateImageType(contentType string) (string, error) {
	ext, ok := supportedImageTypes[contentType]
	if !ok {
		return "", fmt.Errorf("%s: %w", contentType, ErrUnsupportedMimeType)
	}
	return ext, nil
}

// DetectAndValidateImageTypeAndExt reads bytes, detects their MIME type, and validates
// that it's a supported image format. Returns the file extension and the full MIME type.
func DetectAndValidateImageTypeAndExt(bytes []byte) (string, string, error) {
	ftype := http.DetectContentType(bytes)
	ext, err := ValidateImageType(ftype)
	if err != nil {
		return "", "", err
	}
	return ext, ftype, nil
}
