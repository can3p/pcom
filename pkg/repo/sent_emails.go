package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// RecordSentEmail inserts the record of a mail about to be sent. It reports
// false, inserting nothing, when a mail of the same (email_type, unique_id)
// is already recorded: that mail was sent and this one is a repeat.
func (s *Store) RecordSentEmail(ctx context.Context, email *model.SentEmail) (bool, error) {
	res, err := s.query().NewInsert().Model(email).
		On("CONFLICT (email_type, unique_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return n == 1, nil
}
