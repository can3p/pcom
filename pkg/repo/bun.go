package repo

import (
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// builder is a bun DB without a connection: it only builds queries. Every
// query it makes is pointed at an executor with .Conn, so a query runs on
// whatever the Store holds, the database or a transaction.
var builder = bun.NewDB(nil, pgdialect.New())

// Queries builds bun queries that run on one executor.
type Queries struct {
	conn bun.IConn
}

// Query returns bun query builders that run on exec: the database or a
// transaction. The Store's methods use s.query(); test factories call it
// with the executor they are given.
func Query(exec bun.IConn) Queries {
	return Queries{conn: exec}
}

// NewSelect starts a SELECT.
func (q Queries) NewSelect() *bun.SelectQuery {
	return builder.NewSelect().Conn(q.conn)
}

// NewInsert starts an INSERT.
func (q Queries) NewInsert() *bun.InsertQuery {
	return builder.NewInsert().Conn(q.conn)
}

// NewUpdate starts an UPDATE.
func (q Queries) NewUpdate() *bun.UpdateQuery {
	return builder.NewUpdate().Conn(q.conn)
}

// NewDelete starts a DELETE.
func (q Queries) NewDelete() *bun.DeleteQuery {
	return builder.NewDelete().Conn(q.conn)
}

// NewRaw runs hand-written SQL, with bun's ? placeholders.
func (q Queries) NewRaw(query string, args ...any) *bun.RawQuery {
	return builder.NewRaw(query, args...).Conn(q.conn)
}

// query returns the builders for the Store's executor.
//
//nolint:unused // the repositories start using it as R5's pass C rewrites them
func (s *Store) query() Queries {
	return Query(s.exec)
}
