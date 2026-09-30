package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// A whitelist grant says: who allows allowsWho to connect without mediation.
// It is open until the connection is made, then it points at the connection.

// WhitelistTarget finds the user a whitelist grant is about, by username.
func (s *Store) WhitelistTarget(ctx context.Context, username string) (*core.User, error) {
	user, err := core.Users(core.UserWhere.Username.EQ(username)).One(ctx, s.exec)

	return user, notFound(err)
}

// WhitelistedUsers returns the users whoID has an open grant for.
func (s *Store) WhitelistedUsers(ctx context.Context, whoID string) ([]*core.User, error) {
	grants, err := core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(whoID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
		qm.Load(core.WhitelistedConnectionRels.AllowsWho),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	users := make([]*core.User, len(grants))
	for i, g := range grants {
		users[i] = g.R.AllowsWho
	}

	return users, nil
}

// OpenGrantExists reports whether who has an open grant for allowsWho.
func (s *Store) OpenGrantExists(ctx context.Context, whoID, allowsWhoID string) (bool, error) {
	return core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(whoID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(allowsWhoID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
	).Exists(ctx, s.exec)
}

// GrantExists reports whether who has a grant for allowsWho, open or used.
func (s *Store) GrantExists(ctx context.Context, whoID, allowsWhoID string) (bool, error) {
	return core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(whoID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(allowsWhoID),
	).Exists(ctx, s.exec)
}

// LockOpenGrant returns the open grant of who for allowsWho and locks it
// until the transaction ends. ErrNotFound when there is none.
func (s *Store) LockOpenGrant(ctx context.Context, whoID, allowsWhoID string) (*core.WhitelistedConnection, error) {
	grant, err := core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(whoID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(allowsWhoID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
		qm.For("UPDATE"),
	).One(ctx, s.exec)

	return grant, notFound(err)
}

// UseGrant marks a grant as used by the connection.
func (s *Store) UseGrant(ctx context.Context, grant *core.WhitelistedConnection, connectionID string) error {
	grant.ConnectionID = null.StringFrom(connectionID)
	_, err := grant.Update(ctx, s.exec, boil.Infer())

	return err
}

// CreateGrant lets allowsWho connect to who without mediation.
func (s *Store) CreateGrant(ctx context.Context, whoID, allowsWhoID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	grant := &core.WhitelistedConnection{ID: id.String(), WhoID: whoID, AllowsWhoID: allowsWhoID}

	return grant.Insert(ctx, s.exec, boil.Infer())
}

// DeleteOpenGrant withdraws the open grant of who for allowsWho.
func (s *Store) DeleteOpenGrant(ctx context.Context, whoID, allowsWhoID string) error {
	_, err := core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(whoID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(allowsWhoID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
	).DeleteAll(ctx, s.exec)

	return err
}
