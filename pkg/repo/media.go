package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// CreateMediaUpload inserts a media upload record.
func (s *Store) CreateMediaUpload(ctx context.Context, upload *model.MediaUpload) error {
	_, err := s.query().NewInsert().Model(upload).Exec(ctx)

	return err
}

// MediaUploadByID returns the upload with id, or ErrNotFound.
func (s *Store) MediaUploadByID(ctx context.Context, id string) (*model.MediaUpload, error) {
	u := new(model.MediaUpload)
	if err := s.query().NewSelect().Model(u).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return u, nil
}

// DeleteMediaUpload removes the upload record with id; one that is already
// gone is not an error.
func (s *Store) DeleteMediaUpload(ctx context.Context, id string) error {
	_, err := s.query().NewDelete().Model((*model.MediaUpload)(nil)).Where("id = ?", id).Exec(ctx)

	return err
}
