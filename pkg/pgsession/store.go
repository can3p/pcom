package pgsession

import (
	"github.com/antonlindstrom/pgstore"
	"github.com/gin-contrib/sessions"
	"github.com/jmoiron/sqlx"
)

type Store interface {
	sessions.Store
}

func NewStore(db *sqlx.DB, keyPairs ...[]byte) Store {
	s, err := pgstore.NewPGStoreFromPool(db.DB, keyPairs...)

	if err != nil {
		panic(err)
	}

	return &store{s}
}

type store struct {
	*pgstore.PGStore
}

func (c *store) Options(options sessions.Options) {
	c.PGStore.Options = options.ToGorillaOptions()
}
