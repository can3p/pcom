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
