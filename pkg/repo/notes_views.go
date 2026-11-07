package repo

import (
	"context"
	"time"

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

// NoteStackNewest returns the capture time and ID of the newest note of the
// user's stack, a stack being named by the ID of its oldest note (see
// ListNotes), or ErrNotFound when the user has no such stack.
func (s *Store) NoteStackNewest(ctx context.Context, userID string, gap time.Duration, stackID string) (time.Time, string, error) {
	var row struct {
		ID         string    `bun:"id"`
		CapturedAt time.Time `bun:"captured_at"`
	}

	err := s.query().NewSelect().
		With("stacks", s.query().NewRaw(stacksSQL, gap.Seconds(), userID)).
		TableExpr("notes AS n").
		Join("JOIN stacks st ON st.id = n.id").
		ColumnExpr("n.id, n.captured_at").
		Where("st.stack_id = ?", stackID).
		OrderExpr("n.captured_at DESC, n.id DESC").
		Limit(1).
		Scan(ctx, &row)
	if err != nil {
		return time.Time{}, "", notFound(err)
	}

	return row.CapturedAt, row.ID, nil
}
