package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// CreateNoteImage attaches an upload to a note as its photo at img.Position.
func (s *Store) CreateNoteImage(ctx context.Context, img *model.NoteImage) error {
	_, err := s.query().NewInsert().Model(img).Exec(ctx)

	return err
}
