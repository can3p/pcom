package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/can3p/gogo/util/transact"
	"github.com/jmoiron/sqlx"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// ErrNotFound replaces sql.ErrNoRows at the repository boundary.
var ErrNotFound = errors.New("not found")

// Store is the entry point to every query. Methods live one file per
// aggregate (shares.go, posts.go, ...). They take no executor: the Store
// carries it, either the database or the transaction it was handed by Tx.
type Store struct {
	db   *sqlx.DB // nil inside a transaction
	exec boil.ContextExecutor
}

// New returns a Store over db.
func New(db *sqlx.DB) *Store {
	return &Store{db: db, exec: db}
}

// Using returns a Store over an executor that already exists: a transaction
// opened elsewhere, or the executor a legacy function or a test factory
// holds. Tx on such a Store joins the executor instead of opening a
// transaction.
func Using(exec boil.ContextExecutor) *Store {
	return &Store{exec: exec}
}

// Tx runs fn in a transaction and commits when fn returns nil. Inside a
// transaction, Tx joins it, so a service method that opens one may call
// another that does. Repositories never call Tx; services do.
func (s *Store) Tx(ctx context.Context, fn func(tx *Store) error) error {
	if s.db == nil {
		return fn(s)
	}

	return transact.Transact(s.db, func(tx *sql.Tx) error {
		return fn(&Store{exec: tx})
	})
}

// Exec is the executor, for code that still takes one: forms' Save and the
// packages RS has not converted yet. Every call is a step RS removes.
func (s *Store) Exec() boil.ContextExecutor {
	return s.exec
}

// notFound turns sql.ErrNoRows into ErrNotFound and leaves other errors alone.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	return err
}
