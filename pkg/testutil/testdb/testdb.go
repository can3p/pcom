// Package testdb gives a test its own empty Postgres database with pcom's
// migrations applied. The database is dropped when the test ends.
package testdb

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/can3p/gogo/testcontainers/postgres"
)

// migrationsDir is pcom's migrations directory, resolved from this file so
// that it works from any package's working directory.
var migrationsDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
}()

// New returns a fresh, fully migrated database. Use TestDB.DB (*sqlx.DB) as
// the executor and TestDB.URL to hand the database to a subprocess.
func New(t testing.TB) *postgres.TestDB {
	t.Helper()
	return postgres.New(t, postgres.WithMigrationsDir(migrationsDir))
}
