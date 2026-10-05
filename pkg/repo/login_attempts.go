package repo

import (
	"context"
	"slices"
	"time"

	"github.com/can3p/pcom/pkg/model"
)

// InsertLoginAttempt stores a new login attempt.
func (s *Store) InsertLoginAttempt(ctx context.Context, a *model.LoginAttempt) error {
	_, err := s.query().NewInsert().Model(a).Exec(ctx)

	return err
}

// LockLoginAttempt returns the attempt locked for update, or ErrNotFound.
func (s *Store) LockLoginAttempt(ctx context.Context, id string) (*model.LoginAttempt, error) {
	a := new(model.LoginAttempt)
	err := s.query().NewSelect().Model(a).Where("id = ?", id).For("UPDATE").Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return a, nil
}

// SaveLoginAttempt writes the given columns of the attempt (all when none).
func (s *Store) SaveLoginAttempt(ctx context.Context, a *model.LoginAttempt, columns ...string) error {
	q := s.query().NewUpdate().Model(a).WherePK()
	if len(columns) > 0 {
		q = q.Column(append(slices.Clone(columns), "updated_at")...)
	}

	_, err := q.Exec(ctx)

	return err
}

// LoginCodesSince counts the user's attempts created after since that were
// issued a code.
func (s *Store) LoginCodesSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	n, err := s.query().NewSelect().Model((*model.LoginAttempt)(nil)).
		Where("user_id = ?", userID).
		Where("code_hash IS NOT NULL").
		Where("created_at > ?", since).
		Count(ctx)

	return int64(n), err
}

// WrongLoginTriesSince sums the wrong codes tried on the user's attempts
// created after since.
func (s *Store) WrongLoginTriesSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	var n int64
	err := s.query().NewRaw(`
		select coalesce(sum(wrong_tries), 0)
		from login_attempts
		where user_id = ? and created_at > ?`, userID, since).Scan(ctx, &n)

	return n, err
}

// DeleteLoginAttemptsExpiredBefore deletes the attempts that expired before t
// and returns how many there were.
func (s *Store) DeleteLoginAttemptsExpiredBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := s.query().NewDelete().Model((*model.LoginAttempt)(nil)).Where("expires_at < ?", t).Exec(ctx)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// UnexpiredLoginAttempts returns the user's attempts that expire after now,
// newest first; the service decides which of them can still log in.
func (s *Store) UnexpiredLoginAttempts(ctx context.Context, userID string, now time.Time) ([]*model.LoginAttempt, error) {
	var attempts []*model.LoginAttempt
	err := s.query().NewSelect().Model(&attempts).
		Where("user_id = ?", userID).
		Where("expires_at > ?", now).
		OrderExpr("created_at desc, id desc").
		Scan(ctx)

	return attempts, err
}
