package pgsession

import (
	"context"
	"database/sql"
	"time"

	"github.com/can3p/gogo/sessions/pgstore"
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
func NewStore(db *sql.DB, keyPairs ...[]byte) *pgstore.Store {
	return pgstore.New(db, keyPairs...)
}

// RunCleanup deletes expired sessions every interval until ctx is done.
func RunCleanup(ctx context.Context, db *sql.DB, interval time.Duration, logger *zap.Logger) {
	pgstore.New(db).Cleanup(ctx, interval, logger)
}
