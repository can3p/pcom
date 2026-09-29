package pgsession

import (
	"github.com/antonlindstrom/pgstore"
	"github.com/gin-contrib/sessions"
	gsessions "github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
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

// Regenerate starts a new session in place of s, as login and logout must
// (session fixation): the old session's row is deleted, its values are
// cleared and the next Save writes the session under a fresh ID and cookie.
func Regenerate(s sessions.Session) error {
	gb, ok := s.(interface{ Session() *gsessions.Session })
	if !ok {
		return errors.Errorf("session %T is not backed by gorilla/sessions", s)
	}

	gs := gb.Session()
	if gs == nil {
		return errors.New("no session to regenerate")
	}

	if ps, ok := gs.Store().(*pgstore.PGStore); ok && gs.ID != "" {
		if _, err := ps.DbPool.Exec("DELETE FROM http_sessions WHERE key = $1", gs.ID); err != nil {
			return errors.Wrap(err, "failed to delete the old session")
		}
	}

	s.Clear()
	gs.ID = ""
	gs.IsNew = true

	return nil
}
