package repo

import (
	"context"
	"errors"
)

// LockScope names what a per-user advisory lock guards, so that locks taken
// on the same user for different reasons never wait on each other.
type LockScope string

const (
	// LockLoginCodes guards the counts behind a user's login code limits.
	LockLoginCodes LockScope = "login_codes"
	// LockSignupMailbox guards the check that a mailbox has no account yet.
	LockSignupMailbox LockScope = "signup_mailbox"
)

var errLockOutsideTx = errors.New("repo: an advisory lock is held only inside Tx")

// LockUser takes the transaction-level advisory lock of scope on the user,
// waiting while another transaction holds it; commit or rollback releases
// it. Outside a transaction it fails, since the lock would be gone before
// the next statement.
func (s *Store) LockUser(ctx context.Context, scope LockScope, userID string) error {
	return s.lock(ctx, scope, userID)
}

// LockMailbox is LockUser for a mailbox (see pgsession.CanonicalEmail), so
// every spelling of an address takes the same lock.
func (s *Store) LockMailbox(ctx context.Context, scope LockScope, canonical string) error {
	return s.lock(ctx, scope, canonical)
}

// lock takes the advisory lock on a 64-bit hash of scope and key; a rare
// collision only makes two unrelated holders wait on each other.
func (s *Store) lock(ctx context.Context, scope LockScope, key string) error {
	if s.db != nil {
		return errLockOutsideTx
	}

	_, err := s.query().NewRaw(`select pg_advisory_xact_lock(hashtextextended(?, 0))`, string(scope)+":"+key).Exec(ctx)

	return err
}
