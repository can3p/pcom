package factory

import (
	"context"

	"github.com/can3p/pcom/pkg/repo"

	"github.com/can3p/pcom/pkg/model"
)

// MediaUpload records an image uploaded by userID.
func MediaUpload(ctx context.Context, exec repo.Executor, userID string) (*model.MediaUpload, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	// named as the media service names an upload, so markdown that shows it
	// renders it as an uploaded image
	m := &model.MediaUpload{
		ID:            id,
		UserID:        new(userID),
		UploadedFname: id + ".png",
		ContentType:   "image/png",
	}

	return insertRow(ctx, exec, m)
}
