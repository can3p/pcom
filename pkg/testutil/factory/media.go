package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// MediaUpload records an image uploaded by userID.
func MediaUpload(ctx context.Context, exec boil.ContextExecutor, userID string) (*core.MediaUpload, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	m := &core.MediaUpload{
		ID:            id,
		UserID:        null.StringFrom(userID),
		UploadedFname: fmt.Sprintf("test-upload-%d.png", n),
		ContentType:   "image/png",
	}

	if err := m.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return m, nil
}

// OutgoingEmail schedules a queued email of the given type.
func OutgoingEmail(ctx context.Context, exec boil.ContextExecutor, emailType string) (*core.OutgoingEmail, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	uniqueID, err := newID()
	if err != nil {
		return nil, err
	}

	e := &core.OutgoingEmail{
		ID:        id,
		UniqueID:  uniqueID,
		Payload:   []byte("{}"),
		Status:    core.OutgoingEmailStatusNew,
		TryAt:     time.Now(),
		EmailType: emailType,
	}

	if err := e.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return e, nil
}
