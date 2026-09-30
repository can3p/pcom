package repo

import (
	"context"
	"os"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// ConnectScript opens the database named by DATABASE_URL for a command line
// script (the caller imports the postgres driver) and returns a Store over
// it, and the function that closes it.
func ConnectScript() (*Store, func() error, error) {
	db, err := sqlx.Connect("postgres", os.Getenv("DATABASE_URL")+"?sslmode=disable")
	if err != nil {
		return nil, nil, err
	}

	return New(db), db.Close, nil
}

// UserByID returns the user, or ErrNotFound.
func (s *Store) UserByID(ctx context.Context, id string) (*core.User, error) {
	u, err := core.FindUser(ctx, s.exec, id)

	return u, notFound(err)
}

// UserByEmail returns the user whose stored email is email, or ErrNotFound.
// Only a confirmed user when confirmedOnly is set.
func (s *Store) UserByEmail(ctx context.Context, email string, confirmedOnly bool) (*core.User, error) {
	mods := []qm.QueryMod{core.UserWhere.Email.EQ(email)}
	if confirmedOnly {
		mods = append(mods, core.UserWhere.EmailConfirmedAt.IsNotNull())
	}

	u, err := core.Users(mods...).One(ctx, s.exec)

	return u, notFound(err)
}

// UserBySignupSeed returns the user whose confirmation link carries seed,
// or ErrNotFound.
func (s *Store) UserBySignupSeed(ctx context.Context, seed string) (*core.User, error) {
	u, err := core.Users(core.UserWhere.EmailConfirmSeed.EQ(null.StringFrom(seed))).One(ctx, s.exec)

	return u, notFound(err)
}

// UserEmailExists reports whether an account uses the (normalized) email.
func (s *Store) UserEmailExists(ctx context.Context, email string) (bool, error) {
	return core.Users(core.UserWhere.Email.EQ(email)).Exists(ctx, s.exec)
}

// UsernameExists reports whether an account has the username.
func (s *Store) UsernameExists(ctx context.Context, username string) (bool, error) {
	return core.Users(core.UserWhere.Username.EQ(username)).Exists(ctx, s.exec)
}

// InsertUser inserts a new account.
func (s *Store) InsertUser(ctx context.Context, u *core.User) error {
	return u.Insert(ctx, s.exec, boil.Infer())
}

// SaveUser writes the named columns of u, or all of them when none is named.
func (s *Store) SaveUser(ctx context.Context, u *core.User, columns ...string) error {
	cols := boil.Infer()
	if len(columns) > 0 {
		cols = boil.Whitelist(columns...)
	}

	_, err := u.Update(ctx, s.exec, cols)

	return err
}

// CreateConnectionPair connects two users. Connections are stored in both
// directions, so it inserts both rows.
func (s *Store) CreateConnectionPair(ctx context.Context, user1ID, user2ID string) error {
	for _, pair := range [][2]string{{user1ID, user2ID}, {user2ID, user1ID}} {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}

		conn := &core.UserConnection{ID: id.String(), User1ID: pair[0], User2ID: pair[1]}
		if err := conn.Insert(ctx, s.exec, boil.Infer()); err != nil {
			return err
		}
	}

	return nil
}
