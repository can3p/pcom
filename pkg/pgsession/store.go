package pgsession

import (
	"context"
	"time"

	"github.com/can3p/gogo/sessions/pgstore"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// DefaultMaxAge is how long a session lasts after it was last saved, for
// callers built without configuration (config's --session-max-age).
const DefaultMaxAge = 30 * 24 * time.Hour

// DefaultCleanupInterval is how often expired sessions are deleted, for callers
// built without configuration (config's --session-cleanup-interval).
const DefaultCleanupInterval = time.Hour

// NewStore returns gogo's PostgreSQL session store over db. Its table comes
// from the gogo-pgstore migration in migrations/; pgstore.Regenerate rotates a
// session at login and logout.
func NewStore(db *sqlx.DB, keyPairs ...[]byte) *pgstore.Store {
	return pgstore.New(db.DB, keyPairs...)
}

// RunCleanup deletes expired sessions every interval until ctx is done.
func RunCleanup(ctx context.Context, db *sqlx.DB, interval time.Duration, logger *zap.Logger) {
	pgstore.New(db.DB).Cleanup(ctx, interval, logger)
}
