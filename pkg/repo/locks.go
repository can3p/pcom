package repo

import (
	"context"
	"errors"
)

// LockScope names what a per-user advisory lock guards, so that locks taken
// on the same user for different reasons never wait on each other.
type LockScope string

// LockLoginCodes guards the counts behind a user's login code limits.
const LockLoginCodes LockScope = "login_codes"

var errLockOutsideTx = errors.New("repo: an advisory lock is held only inside Tx")

// LockUser takes the transaction-level advisory lock of scope on the user,
// waiting while another transaction holds it; commit or rollback releases
// it. Outside a transaction it fails, since the lock would be gone before
// the next statement.
func (s *Store) LockUser(ctx context.Context, scope LockScope, userID string) error {
	return s.lock(ctx, `$1 || ':' || $2`, scope, userID)
}

// lock takes the advisory lock on a 64-bit hash of the key keySQL builds
// from scope and arg; a rare collision only makes two unrelated holders wait
// on each other.
func (s *Store) lock(ctx context.Context, keySQL string, scope LockScope, arg string) error {
	if s.db != nil {
		return errLockOutsideTx
	}

	_, err := s.exec.ExecContext(ctx, `select pg_advisory_xact_lock(hashtextextended(`+keySQL+`, 0))`, string(scope), arg)

	return err
}
