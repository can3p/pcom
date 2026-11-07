package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// NoteInboxCount returns how many of the user's notes carry no tag, from
// tags or from the text.
func (s *Store) NoteInboxCount(ctx context.Context, userID string) (int, error) {
	n, err := s.query().NewSelect().Model((*model.Note)(nil)).
		Where("user_id = ?", userID).
		Where("cardinality(tags || text_tags) = 0").
		Count(ctx)

	return int(n), err
}

// NotePostLinkCount returns in how many posts the note was inserted.
func (s *Store) NotePostLinkCount(ctx context.Context, noteID string) (int, error) {
	n, err := s.query().NewSelect().Model((*model.NotePostLink)(nil)).Where("note_id = ?", noteID).Count(ctx)

	return int(n), err
}
