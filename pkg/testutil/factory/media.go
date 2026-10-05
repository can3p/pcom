package factory

import (
	"context"
	"fmt"
	"time"

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

// OutgoingEmail schedules a queued email of the given type.
func OutgoingEmail(ctx context.Context, exec repo.Executor, emailType string) (*model.OutgoingEmail, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	uniqueID, err := newID()
	if err != nil {
		return nil, err
	}

	e := &model.OutgoingEmail{
		ID:        id,
		UniqueID:  uniqueID,
		Payload:   []byte("{}"),
		Status:    model.OutgoingEmailStatusNew,
		TryAt:     time.Now(),
		EmailType: emailType,
	}

	return insertRow(ctx, exec, e)
}
