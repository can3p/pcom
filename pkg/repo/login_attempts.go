package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// InsertLoginAttempt stores a new login attempt.
func (s *Store) InsertLoginAttempt(ctx context.Context, a *core.LoginAttempt) error {
	return a.Insert(ctx, s.exec, boil.Infer())
}

// LockLoginAttempt returns the attempt locked for update, or ErrNotFound.
func (s *Store) LockLoginAttempt(ctx context.Context, id string) (*core.LoginAttempt, error) {
	a, err := core.LoginAttempts(core.LoginAttemptWhere.ID.EQ(id), qm.For("update")).One(ctx, s.exec)

	return a, notFound(err)
}

// SaveLoginAttempt writes the given columns of the attempt (all when none).
func (s *Store) SaveLoginAttempt(ctx context.Context, a *core.LoginAttempt, columns ...string) error {
	cols := boil.Infer()
	if len(columns) > 0 {
		cols = boil.Whitelist(append(columns, core.LoginAttemptColumns.UpdatedAt)...)
	}

	_, err := a.Update(ctx, s.exec, cols)

	return err
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

// WrongLoginTriesSince counts the wrong codes tried on the user's attempts
// created after since: every try but the one that used an attempt up.
func (s *Store) WrongLoginTriesSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	var n int64
	err := s.exec.QueryRowContext(ctx, `
		select coalesce(sum(tries - case when used_at is null then 0 else 1 end), 0)
		from login_attempts
		where user_id = $1 and created_at > $2`, userID, since).Scan(&n)

	return n, err
}

// DeleteLoginAttemptsExpiredBefore deletes the attempts that expired before t
// and returns how many there were.
func (s *Store) DeleteLoginAttemptsExpiredBefore(ctx context.Context, t time.Time) (int64, error) {
	return core.LoginAttempts(core.LoginAttemptWhere.ExpiresAt.LT(t)).DeleteAll(ctx, s.exec)
}

// NewestOpenLoginAttempt returns the user's newest attempt that is unused,
// expires after now and has had fewer than maxTries tries, or ErrNotFound.
func (s *Store) NewestOpenLoginAttempt(ctx context.Context, userID string, now time.Time, maxTries int) (*core.LoginAttempt, error) {
	a, err := core.LoginAttempts(
		core.LoginAttemptWhere.UserID.EQ(null.StringFrom(userID)),
		core.LoginAttemptWhere.UsedAt.IsNull(),
		core.LoginAttemptWhere.ExpiresAt.GT(now),
		core.LoginAttemptWhere.Tries.LT(maxTries),
		qm.OrderBy(core.LoginAttemptColumns.CreatedAt+" desc, "+core.LoginAttemptColumns.ID+" desc"),
	).One(ctx, s.exec)

	return a, notFound(err)
}
