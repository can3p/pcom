// Package media handles media uploads.
package media

import (
	"context"
	"io"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

type Service struct {
	store        *repo.Store
	mediaStorage server.MediaStorage
}

func New(store *repo.Store, mediaStorage server.MediaStorage) *Service {
	return &Service{
		store:        store,
		mediaStorage: mediaStorage,
	}
}

// Upload validates, reads, stores, and records a media upload for a user.
// It returns the filename. The record is inserted on the store's executor
// (which may be a transaction).
func (s *Service) Upload(ctx context.Context, actor *core.User, reader io.Reader) (string, error) {
	if actor == nil {
		return "", service.ErrNeedsLogin
	}

	bytes, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	ext, ftype, err := media.DetectAndValidateImageTypeAndExt(bytes)
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
	mediaUpload.UserID.SetValid(actor.ID)

	// we do actions inside and outside db in one go
	// operation should be deferred with transaction, but file storage
	// part can still get corrupted
	if err := s.store.CreateMediaUpload(ctx, mediaUpload); err != nil {
		return "", err
	}

	if err := s.mediaStorage.UploadFile(ctx, fname, bytes, ftype); err != nil {
		return "", err
	}

	return fname, nil
}
