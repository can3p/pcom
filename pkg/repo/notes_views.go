package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// NotePostLinkCount returns in how many posts the note was inserted.
func (s *Store) NotePostLinkCount(ctx context.Context, noteID string) (int, error) {
	n, err := s.query().NewSelect().Model((*model.NotePostLink)(nil)).Where("note_id = ?", noteID).Count(ctx)

	return int(n), err
}
