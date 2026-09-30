package repo

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

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

// FeedOwner returns the user a private RSS feed token belongs to, or
// ErrNotFound when the token is unknown.
func (s *Store) FeedOwner(ctx context.Context, token string) (*core.User, error) {
	user, err := s.FeedTokenOwner(ctx, token)
	return user, notFound(err)
}
