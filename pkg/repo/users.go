package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/uptrace/bun"
)

// UserByID returns the user, or ErrNotFound.
func (s *Store) UserByID(ctx context.Context, id string) (*model.User, error) {
	u := new(model.User)
	err := s.query().NewSelect().Model(u).Where("id = ?", id).Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return u, nil
}

// UserByEmail returns the user whose stored email is email, or ErrNotFound.
func (s *Store) UserByEmail(ctx context.Context, email string) (*model.User, error) {
	u := new(model.User)
	err := s.query().NewSelect().Model(u).Where("email = ?", email).Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return u, nil
}

// UserEmailExists reports whether an account uses the (normalized) email.
func (s *Store) UserEmailExists(ctx context.Context, email string) (bool, error) {
	return s.query().NewSelect().Model((*model.User)(nil)).Where("email = ?", email).Exists(ctx)
}

// MailboxHasAccount reports whether an account uses an address that delivers
// to the mailbox canonical (see pgsession.CanonicalEmail).
func (s *Store) MailboxHasAccount(ctx context.Context, canonical string) (bool, error) {
	return s.query().NewSelect().Model((*model.User)(nil)).Where("email_canonical = ?", canonical).Exists(ctx)
}

// UnconfirmedUserIDsCreatedBefore returns the ids of the users who never
// confirmed their address, were created before t and hold no invitations.
func (s *Store) UnconfirmedUserIDsCreatedBefore(ctx context.Context, t time.Time) ([]string, error) {
	ids := []string{}
	err := s.query().NewSelect().Model((*model.User)(nil)).
		Column("id").
		Where("?TableAlias.email_confirmed_at IS NULL").
		Where("?TableAlias.created_at < ?", t).
		Where("not exists (select 1 from user_invitations i where i.user_id = ?TableAlias.id)").
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// DeleteUnconfirmedUser deletes the user unless they have confirmed their
// address meanwhile, and reports whether it did.
func (s *Store) DeleteUnconfirmedUser(ctx context.Context, id string) (bool, error) {
	res, err := s.query().NewDelete().Model((*model.User)(nil)).
		Where("id = ?", id).
		Where("email_confirmed_at IS NULL").
		Exec(ctx)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// UsernameExists reports whether an account has the username.
func (s *Store) UsernameExists(ctx context.Context, username string) (bool, error) {
	return s.query().NewSelect().Model((*model.User)(nil)).Where("username = ?", username).Exists(ctx)
}

// InsertUser inserts a new account.
func (s *Store) InsertUser(ctx context.Context, u *model.User) error {
	_, err := s.query().NewInsert().Model(u).Exec(ctx)

	return err
}

// SaveUser writes the named columns of u, or all of them when none is named.
func (s *Store) SaveUser(ctx context.Context, u *model.User, columns ...string) error {
	q := s.query().NewUpdate().Model(u).WherePK()
	if len(columns) > 0 {
		q = q.Column(columns...)
	}

	_, err := q.Exec(ctx)

	return err
}

// UserByUsername returns the user with that username.
func (s *Store) UserByUsername(ctx context.Context, username string) (*model.User, error) {
	u := new(model.User)
	err := s.query().NewSelect().Model(u).Where("username = ?", username).Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return u, nil
}

// UsersByIDs returns the users with those IDs, in no particular order.
func (s *Store) UsersByIDs(ctx context.Context, ids []string) ([]*model.User, error) {
	var users []*model.User
	err := s.query().NewSelect().Model(&users).Where("id IN (?)", bun.List(ids)).Scan(ctx)

	return users, err
}

// UsersByIDsNewestFirst returns the users with those IDs, the most recently
// signed up first.
func (s *Store) UsersByIDsNewestFirst(ctx context.Context, ids []string) ([]*model.User, error) {
	var users []*model.User
	err := s.query().NewSelect().Model(&users).
		Where("id IN (?)", bun.List(ids)).
		OrderExpr("created_at DESC").
		Scan(ctx)

	return users, err
}
