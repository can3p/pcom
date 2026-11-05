// Package media handles media uploads.
package media

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// DefaultMaxUploadSize is the most bytes an uploaded image may have.
const DefaultMaxUploadSize = 20 * 1024 * 1024

type Service struct {
	store         *repo.Store
	mediaStorage  server.MediaStorage
	maxUploadSize int64
}

// Option changes how New builds the service.
type Option func(*Service)

// WithMaxUploadSize sets the most bytes an uploaded image may have; zero
// keeps DefaultMaxUploadSize.
func WithMaxUploadSize(n int64) Option {
	return func(s *Service) {
		if n > 0 {
			s.maxUploadSize = n
		}
	}
}

func New(store *repo.Store, mediaStorage server.MediaStorage, opts ...Option) *Service {
	s := &Service{
		store:         store,
		mediaStorage:  mediaStorage,
		maxUploadSize: DefaultMaxUploadSize,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Upload validates, reads, stores, and records a media upload for a user.
// It returns the filename. The record is inserted on the store's executor
// (which may be a transaction).
func (s *Service) Upload(ctx context.Context, actor *model.User, reader io.Reader) (string, error) {
	if actor == nil {
		return "", service.ErrNeedsLogin
	}

	return StoreUpload(ctx, s.store, s.mediaStorage, &actor.ID, nil, reader, s.maxUploadSize)
}

// StoreUpload validates an image, records the upload on store, owned by
// exactly one of a user or an RSS feed, and puts the file in the storage.
// An image larger than maxSize bytes (DefaultMaxUploadSize when not
// positive) is a validation error. Other services call it with their
// transaction. The row comes first; the file lands in the storage, which is
// not part of the transaction.
func StoreUpload(ctx context.Context, store *repo.Store, storage server.MediaStorage, userID, rssFeedID *string, reader io.Reader, maxSize int64) (string, error) {
	if (userID == nil) == (rssFeedID == nil) {
		return "", errors.New("exactly one of userID or rssFeedID must be provided")
	}

	if maxSize <= 0 {
		maxSize = DefaultMaxUploadSize
	}

	bytes, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return "", err
	}

	if int64(len(bytes)) > maxSize {
		return "", service.Invalid("file", fmt.Sprintf("The image is larger than %d MiB.", maxSize>>20))
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
