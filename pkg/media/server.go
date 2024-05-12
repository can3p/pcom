package media

import (
	"context"
	"database/sql"
	"io"
	"net/http"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type MediaServer interface {
	UploadFile(ctx context.Context, fname string, b []byte, contentType string) error
	ServeFile(ctx context.Context, fname string) (io.Reader, int64, string, error)
}

func HandleUpload(ctx context.Context, exec *sqlx.DB, media MediaServer, userID string, reader io.Reader) (string, error) {
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

	err = transact.Transact(exec, func(tx *sql.Tx) error {
		// not used anywhere, just to track which users upload photos
		mediaUpload := &core.MediaUpload{
			ID:            id.String(),
			UploadedFname: fname,
			ContentType:   ftype,
			UserID:        userID,
		}

		mediaUpload.InsertP(ctx, tx, boil.Infer())

		return media.UploadFile(ctx, fname, bytes, ftype)
	})

	if err != nil {
		return "", err
	}

	return fname, nil
}
