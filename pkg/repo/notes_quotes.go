package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// LastSourcedNote returns the user's most recent note that has a source,
// with the source loaded, or nil when no note has one.
func (s *Store) LastSourcedNote(ctx context.Context, userID string) (*model.Note, error) {
	n := new(model.Note)

	return orNil(n, s.query().NewSelect().Model(n).Relation("Source").
		Where("note.user_id = ?", userID).Where("note.source_id IS NOT NULL").
		OrderExpr("note.captured_at DESC, note.id DESC").Limit(1).Scan(ctx))
}
