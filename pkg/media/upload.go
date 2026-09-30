package media

import (
	"context"
	"io"
	"net/http"

	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var (
	ErrNotFound            = errors.Errorf("Resource not found")
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
		return "", errors.Wrapf(ErrUnsupportedMimeType, "%s", contentType)
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

// HandleUpload is a legacy utility used by code that hasn't been refactored yet.
// New code should use the media service instead.
func HandleUpload(ctx context.Context, exec boil.ContextExecutor, media server.MediaStorage, userID *string, rssFeedID *string, reader io.Reader) (string, error) {
	if (userID == nil && rssFeedID == nil) || (userID != nil && rssFeedID != nil) {
		return "", errors.Errorf("exactly one of userID or rssFeedID must be provided")
	}

	bytes, err := io.ReadAll(reader)

	if err != nil {
		return "", err
	}

	ftype := http.DetectContentType(bytes)

	ext, err := ValidateImageType(ftype)
	if err != nil {
		return "", err
	}

	id, err := uuid.NewV7()

	if err != nil {
		return "", err
	}

	fname := id.String() + ext

	mediaUpload := &core.MediaUpload{
		ID:            id.String(),
		UploadedFname: fname,
		ContentType:   ftype,
	}

	if userID != nil {
		mediaUpload.UserID.SetValid(*userID)
	}

	if rssFeedID != nil {
		mediaUpload.RSSFeedID.SetValid(*rssFeedID)
	}

	// we do actions inside and outside db in one go
	// operation should be deferred with transaction, but file storage
	// part can still get corrupted
	if err := mediaUpload.Insert(ctx, exec, boil.Infer()); err != nil {
		return "", err
	}

	if err := media.UploadFile(ctx, fname, bytes, ftype); err != nil {
		return "", err
	}

	return fname, nil
}
