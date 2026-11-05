package factory

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/repo"

	"github.com/can3p/pcom/pkg/model"
)

// MediaUpload records an image uploaded by userID.
func MediaUpload(ctx context.Context, exec repo.Executor, userID string) (*model.MediaUpload, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	m := &model.MediaUpload{
		ID:            id,
		UserID:        new(userID),
		UploadedFname: fmt.Sprintf("test-upload-%d.png", n),
		ContentType:   "image/png",
	}

	return insertRow(ctx, exec, m)
}
