package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
)

// NoticeDismissed reports whether the user dismissed the notice.
func (s *Store) NoticeDismissed(ctx context.Context, userID, notice string) (bool, error) {
	return s.query().NewSelect().Model((*model.UserNotice)(nil)).
		Where("user_id = ?", userID).Where("notice = ?", notice).Exists(ctx)
}

// DismissNotice records that the user dismissed the notice; a repeat keeps
// the first time.
func (s *Store) DismissNotice(ctx context.Context, userID, notice string, at time.Time) error {
	_, err := s.query().NewInsert().Model(&model.UserNotice{UserID: userID, Notice: notice, DismissedAt: at}).
		On("CONFLICT (user_id, notice) DO NOTHING").Exec(ctx)

	return err
}
