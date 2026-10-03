package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// UserByID returns the user, or ErrNotFound.
func (s *Store) UserByID(ctx context.Context, id string) (*core.User, error) {
	u, err := core.FindUser(ctx, s.exec, id)

	return u, notFound(err)
}

// UserByEmail returns the user whose stored email is email, or ErrNotFound.
func (s *Store) UserByEmail(ctx context.Context, email string) (*core.User, error) {
	u, err := core.Users(core.UserWhere.Email.EQ(email)).One(ctx, s.exec)

	return u, notFound(err)
}

// UserEmailExists reports whether an account uses the (normalized) email.
func (s *Store) UserEmailExists(ctx context.Context, email string) (bool, error) {
	return core.Users(core.UserWhere.Email.EQ(email)).Exists(ctx, s.exec)
}

// MailboxHasAccount reports whether an account uses an address that delivers
// to the mailbox canonical (see pgsession.CanonicalEmail).
func (s *Store) MailboxHasAccount(ctx context.Context, canonical string) (bool, error) {
	return core.Users(core.UserWhere.EmailCanonical.EQ(canonical)).Exists(ctx, s.exec)
}

// UnconfirmedUserIDsCreatedBefore returns the ids of the users who never
// confirmed their address and were created before t.
func (s *Store) UnconfirmedUserIDsCreatedBefore(ctx context.Context, t time.Time) ([]string, error) {
	users, err := core.Users(
		qm.Select(core.UserColumns.ID),
		core.UserWhere.EmailConfirmedAt.IsNull(),
		core.UserWhere.CreatedAt.LT(null.TimeFrom(t)),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}

	return ids, nil
}

// DeleteUnconfirmedUser deletes the user unless they have confirmed their
// address meanwhile, and reports whether it did.
func (s *Store) DeleteUnconfirmedUser(ctx context.Context, id string) (bool, error) {
	n, err := core.Users(core.UserWhere.ID.EQ(id), core.UserWhere.EmailConfirmedAt.IsNull()).DeleteAll(ctx, s.exec)

	return n > 0, err
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
