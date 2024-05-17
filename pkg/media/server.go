package media

import (
	"context"
	"io"
	"net/http"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var ErrNotFound = errors.Errorf("Resource not found")

type MediaServer interface {
	UploadFile(ctx context.Context, fname string, b []byte, contentType string) error
	ServeFile(ctx context.Context, fname string) (io.Reader, int64, string, error)
}

func HandleUpload(ctx context.Context, exec boil.ContextExecutor, media MediaServer, userID string, reader io.Reader) (string, error) {
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
		UserID:        userID,
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
