package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
)

// UsePowChallenge records the salt of a proof-of-work challenge as used and
// reports whether it was new; false means it was answered before.
func (s *Store) UsePowChallenge(ctx context.Context, salt string, at time.Time) (bool, error) {
	res, err := s.query().NewInsert().Model(&model.PowChallengeUsed{Salt: salt, UsedAt: at}).
		On("CONFLICT (salt) DO NOTHING").Exec(ctx)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n == 1, err
}

// DeletePowChallengesBefore deletes the challenges used before t.
func (s *Store) DeletePowChallengesBefore(ctx context.Context, t time.Time) error {
	_, err := s.query().NewDelete().Model((*model.PowChallengeUsed)(nil)).Where("used_at < ?", t).Exec(ctx)

	return err
}
