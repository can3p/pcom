package repo

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

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

// UserByUsername returns the user with that username.
func (s *Store) UserByUsername(ctx context.Context, username string) (*core.User, error) {
	user, err := core.Users(core.UserWhere.Username.EQ(username)).One(ctx, s.exec)
	return user, notFound(err)
}

// UsersByIDs returns the users with those IDs, in no particular order.
func (s *Store) UsersByIDs(ctx context.Context, ids []string) (core.UserSlice, error) {
	return core.Users(core.UserWhere.ID.IN(ids)).All(ctx, s.exec)
}

// UsersByIDsNewestFirst returns the users with those IDs, the most recently
// signed up first.
func (s *Store) UsersByIDsNewestFirst(ctx context.Context, ids []string) (core.UserSlice, error) {
	return core.Users(
		core.UserWhere.ID.IN(ids),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.UserColumns.CreatedAt)),
	).All(ctx, s.exec)
}
