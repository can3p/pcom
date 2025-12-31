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

var ErrNotFound = errors.Errorf("Resource not found")

func HandleUpload(ctx context.Context, exec boil.ContextExecutor, media server.MediaStorage, userID *string, rssFeedID *string, reader io.Reader) (string, error) {
	if (userID == nil && rssFeedID == nil) || (userID != nil && rssFeedID != nil) {
		return "", errors.Errorf("exactly one of userID or rssFeedID must be provided")
	}

	bytes, err := io.ReadAll(reader)

	if err != nil {
		panic(err)
	}

	ftype := http.DetectContentType(bytes)
	var ext string

	switch ftype {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	default:
		return "", errors.Errorf("unsupported mime type: %s", ftype)
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
	// operation should be defened with transaction, but file storage
	// part can still get corrupted
	if err := mediaUpload.Insert(ctx, exec, boil.Infer()); err != nil {
		return "", err
	}

	if err := media.UploadFile(ctx, fname, bytes, ftype); err != nil {
		return "", err
	}

	return fname, nil
}
