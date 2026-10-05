package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// InsertLoginAttempt stores a new login attempt.
func (s *Store) InsertLoginAttempt(ctx context.Context, a *model.LoginAttempt) error {
	return write(a, func(c *core.LoginAttempt) error {
		return c.Insert(ctx, s.exec, boil.Infer())
	})
}

// LockLoginAttempt returns the attempt locked for update, or ErrNotFound.
func (s *Store) LockLoginAttempt(ctx context.Context, id string) (*model.LoginAttempt, error) {
	a, err := core.LoginAttempts(core.LoginAttemptWhere.ID.EQ(id), qm.For("update")).One(ctx, s.exec)

	return toModel[model.LoginAttempt](a), notFound(err)
}

// SaveLoginAttempt writes the given columns of the attempt (all when none).
func (s *Store) SaveLoginAttempt(ctx context.Context, a *model.LoginAttempt, columns ...string) error {
	cols := boil.Infer()
	if len(columns) > 0 {
		cols = boil.Whitelist(append(columns, core.LoginAttemptColumns.UpdatedAt)...)
	}

	return write(a, func(c *core.LoginAttempt) error {
		_, err := c.Update(ctx, s.exec, cols)

		return err
	})
}

// LoginCodesSince counts the user's attempts created after since that were
// issued a code.
func (s *Store) LoginCodesSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	return core.LoginAttempts(
		core.LoginAttemptWhere.UserID.EQ(null.StringFrom(userID)),
		core.LoginAttemptWhere.CodeHash.IsNotNull(),
		core.LoginAttemptWhere.CreatedAt.GT(since),
	).Count(ctx, s.exec)
}

// WrongLoginTriesSince sums the wrong codes tried on the user's attempts
// created after since.
func (s *Store) WrongLoginTriesSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	var n int64
	err := s.exec.QueryRowContext(ctx, `
		select coalesce(sum(wrong_tries), 0)
		from login_attempts
		where user_id = $1 and created_at > $2`, userID, since).Scan(&n)

	return n, err
}

// DeleteLoginAttemptsExpiredBefore deletes the attempts that expired before t
// and returns how many there were.
func (s *Store) DeleteLoginAttemptsExpiredBefore(ctx context.Context, t time.Time) (int64, error) {
	return core.LoginAttempts(core.LoginAttemptWhere.ExpiresAt.LT(t)).DeleteAll(ctx, s.exec)
}

// UnexpiredLoginAttempts returns the user's attempts that expire after now,
// newest first; the service decides which of them can still log in.
func (s *Store) UnexpiredLoginAttempts(ctx context.Context, userID string, now time.Time) ([]*model.LoginAttempt, error) {
	return all[model.LoginAttempt](core.LoginAttempts(
		core.LoginAttemptWhere.UserID.EQ(null.StringFrom(userID)),
		core.LoginAttemptWhere.ExpiresAt.GT(now),
		qm.OrderBy(core.LoginAttemptColumns.CreatedAt+" desc, "+core.LoginAttemptColumns.ID+" desc"),
	).All(ctx, s.exec))
}
