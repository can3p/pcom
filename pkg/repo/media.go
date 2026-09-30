package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// CreateMediaUpload inserts a media upload record.
func (s *Store) CreateMediaUpload(ctx context.Context, upload *core.MediaUpload) error {
	return upload.Insert(ctx, s.exec, boil.Infer())
}
