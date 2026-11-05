// Package testdb gives a test its own empty Postgres database with pcom's
// migrations applied. The database is dropped when the test ends.
package testdb

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/can3p/gogo/testcontainers/postgres"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx database/sql driver
)

// migrationsDir is pcom's migrations directory, resolved from this file so
// that it works from any package's working directory.
var migrationsDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
}()

// New returns a fresh, fully migrated database. Use TestDB.DB (*sql.DB) as
// the executor and TestDB.URL to hand the database to a subprocess.
//
// gogo connects with lib/pq; New reconnects through pgx, the driver the
// server uses, so tests run on the production driver. gogo's cleanup closes
// whatever TestDB.DB holds before it drops the database, so the pgx
// connection is closed first.
func New(t testing.TB) *postgres.TestDB {
	t.Helper()

	tdb := postgres.New(t, postgres.WithMigrationsDir(migrationsDir))

	if err := tdb.DB.Close(); err != nil {
		t.Fatalf("testdb: closing the lib/pq connection: %v", err)
	}

	db, err := sql.Open("pgx", tdb.URL)
	if err != nil {
		t.Fatalf("testdb: connecting with pgx: %v", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("testdb: connecting with pgx: %v", err)
	}

	tdb.DB = db
	tdb.SQL = db

	return tdb
}
