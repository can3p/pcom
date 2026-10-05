package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// UserByID returns the user, or ErrNotFound.
func (s *Store) UserByID(ctx context.Context, id string) (*model.User, error) {
	u, err := core.FindUser(ctx, s.exec, id)

	return toModel[model.User](u), notFound(err)
}

// UserByEmail returns the user whose stored email is email, or ErrNotFound.
func (s *Store) UserByEmail(ctx context.Context, email string) (*model.User, error) {
	u, err := core.Users(core.UserWhere.Email.EQ(email)).One(ctx, s.exec)

	return toModel[model.User](u), notFound(err)
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
// confirmed their address, were created before t and hold no invitations.
func (s *Store) UnconfirmedUserIDsCreatedBefore(ctx context.Context, t time.Time) ([]string, error) {
	users, err := core.Users(
		qm.Select(core.UserColumns.ID),
		core.UserWhere.EmailConfirmedAt.IsNull(),
		core.UserWhere.CreatedAt.LT(null.TimeFrom(t)),
		qm.Where(`not exists (select 1 from user_invitations i where i.user_id = users.id)`),
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
func (s *Store) InsertUser(ctx context.Context, u *model.User) error {
	return write(u, func(c *core.User) error {
		return c.Insert(ctx, s.exec, boil.Infer())
	})
}

// SaveUser writes the named columns of u, or all of them when none is named.
func (s *Store) SaveUser(ctx context.Context, u *model.User, columns ...string) error {
	cols := boil.Infer()
	if len(columns) > 0 {
		cols = boil.Whitelist(columns...)
	}

	return write(u, func(c *core.User) error {
		_, err := c.Update(ctx, s.exec, cols)

		return err
	})
}

// UserByUsername returns the user with that username.
func (s *Store) UserByUsername(ctx context.Context, username string) (*model.User, error) {
	user, err := core.Users(core.UserWhere.Username.EQ(username)).One(ctx, s.exec)
	return toModel[model.User](user), notFound(err)
}

// UsersByIDs returns the users with those IDs, in no particular order.
func (s *Store) UsersByIDs(ctx context.Context, ids []string) ([]*model.User, error) {
	return all[model.User](core.Users(core.UserWhere.ID.IN(ids)).All(ctx, s.exec))
}

// UsersByIDsNewestFirst returns the users with those IDs, the most recently
// signed up first.
func (s *Store) UsersByIDsNewestFirst(ctx context.Context, ids []string) ([]*model.User, error) {
	return all[model.User](core.Users(
		core.UserWhere.ID.IN(ids),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.UserColumns.CreatedAt)),
	).All(ctx, s.exec))
}
