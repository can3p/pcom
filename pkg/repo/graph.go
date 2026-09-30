package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/samber/lo"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// The connection graph is undirected and stored both ways: a connection
// between A and B is two rows, (A, B) and (B, A). The queries rely on it.

// ConnectedUserIDs returns the IDs of userID's direct connections.
func (s *Store) ConnectedUserIDs(ctx context.Context, userID string) ([]string, error) {
	connections, err := core.UserConnections(core.UserConnectionWhere.User1ID.EQ(userID)).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	return lo.Map(connections, func(conn *core.UserConnection, _ int) string { return conn.User2ID }), nil
}

// Hop is one path of length two from a user: UserID is reached through
// ViaUserID. UserID may be the user themselves or a direct connection too.
type Hop struct {
	UserID    string `boil:"user_id"`
	ViaUserID string `boil:"via_user_id"`
}

// TwoHops returns every path of length two from userID.
func (s *Store) TwoHops(ctx context.Context, userID string) ([]*Hop, error) {
	var hops []*Hop

	// https://www.linkedin.com/pulse/you-dont-need-graph-database-modeling-graphs-trees-viktor-qvarfordt-efzof/
	err := core.NewQuery(
		qm.Select("conn2.user2_id as user_id, conn1.user2_id as via_user_id"),
		qm.From("user_connections as conn1"),
		qm.LeftOuterJoin("user_connections as conn2 on conn1.user2_id = conn2.user1_id"),
		qm.Where("conn1.user1_id = ?", userID),
	).Bind(ctx, s.exec, &hops)

	return hops, err
}

// Connected reports whether two users are directly connected.
func (s *Store) Connected(ctx context.Context, userID, otherID string) (bool, error) {
	return core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(userID),
		core.UserConnectionWhere.User2ID.EQ(otherID),
	).Exists(ctx, s.exec)
}

// ConnectedThroughOne reports whether a path of length two joins the users.
func (s *Store) ConnectedThroughOne(ctx context.Context, userID, otherID string) (bool, error) {
	return core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(userID),
		qm.LeftOuterJoin("user_connections conn2 on user_connections.user2_id = conn2.user1_id"),
		qm.Where("conn2.user2_id = ?", otherID),
	).Exists(ctx, s.exec)
}
