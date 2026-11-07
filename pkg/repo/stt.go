package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/uptrace/bun"
)

// InsertSTTRequest records one call to a speech-to-text backend, giving it
// an id when it has none.
func (s *Store) InsertSTTRequest(ctx context.Context, r *model.STTRequest) error {
	if err := ensureID(&r.ID); err != nil {
		return err
	}

	_, err := s.query().NewInsert().Model(r).Exec(ctx)

	return err
}

// STTUsage is the seconds of audio the matching ok calls transcribed, and
// when the oldest of them was made (zero when none matched).
type STTUsage struct {
	Seconds int       `bun:"seconds"`
	Oldest  time.Time `bun:"oldest"`
}

// STTSecondsSince sums the seconds of the ok calls made at or after since;
// an empty userID counts every user's.
func (s *Store) STTSecondsSince(ctx context.Context, userID string, since time.Time) (STTUsage, error) {
	var (
		out    STTUsage
		oldest bun.NullTime
	)

	q := s.query().NewSelect().Model((*model.STTRequest)(nil)).
		ColumnExpr("coalesce(sum(?TableAlias.seconds), 0)").
		ColumnExpr("min(?TableAlias.created_at)").
		Where("?TableAlias.status = ?", model.STTStatusOK).
		Where("?TableAlias.created_at >= ?", since)

	if userID != "" {
		q = q.Where("?TableAlias.user_id = ?", userID)
	}

	if err := q.Scan(ctx, &out.Seconds, &oldest); err != nil {
		return out, err
	}

	out.Oldest = oldest.Time

	return out, nil
}
