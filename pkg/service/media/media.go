// Package media handles media uploads.
package media

import (
	"context"
	"errors"
	"io"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model"
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
func (s *Service) Upload(ctx context.Context, actor *model.User, reader io.Reader) (string, error) {
	if actor == nil {
		return "", service.ErrNeedsLogin
	}

	return StoreUpload(ctx, s.store, s.mediaStorage, &actor.ID, nil, reader)
}

// StoreUpload validates an image, records the upload on store, owned by
// exactly one of a user or an RSS feed, and puts the file in the storage.
// Other services call it with their transaction. The row comes first; the
// file lands in the storage, which is not part of the transaction.
func StoreUpload(ctx context.Context, store *repo.Store, storage server.MediaStorage, userID, rssFeedID *string, reader io.Reader) (string, error) {
	if (userID == nil) == (rssFeedID == nil) {
		return "", errors.New("exactly one of userID or rssFeedID must be provided")
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

	mediaUpload := &model.MediaUpload{
		ID:            id.String(),
		UploadedFname: fname,
		ContentType:   ftype,
	}

	if userID != nil {
		mediaUpload.UserID = new(*userID)
	} else {
		mediaUpload.RSSFeedID = new(*rssFeedID)
	}

	if err := store.CreateMediaUpload(ctx, mediaUpload); err != nil {
		return "", err
	}

	if err := storage.UploadFile(ctx, fname, bytes, ftype); err != nil {
		return "", err
	}

	return fname, nil
}
