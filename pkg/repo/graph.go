package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// The connection graph is undirected and stored both ways: a connection
// between A and B is two rows, (A, B) and (B, A). The queries rely on it.

// ConnectedUserIDs returns the IDs of userID's direct connections.
func (s *Store) ConnectedUserIDs(ctx context.Context, userID string) ([]string, error) {
	ids := []string{}

	err := s.query().NewSelect().Model((*model.UserConnection)(nil)).
		Column("user2_id").
		Where("?TableAlias.user1_id = ?", userID).
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}

	// An empty, non-nil slice without connections, as before.
	if ids == nil {
		ids = []string{}
	}

	return ids, nil
}

// Hop is one path of length two from a user: UserID is reached through
// ViaUserID. UserID may be the user themselves or a direct connection too.
type Hop struct {
	UserID    string `bun:"user_id"`
	ViaUserID string `bun:"via_user_id"`
}

// TwoHops returns every path of length two from userID.
func (s *Store) TwoHops(ctx context.Context, userID string) ([]*Hop, error) {
	var hops []*Hop

	// https://www.linkedin.com/pulse/you-dont-need-graph-database-modeling-graphs-trees-viktor-qvarfordt-efzof/
	err := s.query().NewRaw(`SELECT conn2.user2_id as user_id, conn1.user2_id as via_user_id
FROM user_connections as conn1
LEFT OUTER JOIN user_connections as conn2 on conn1.user2_id = conn2.user1_id
WHERE (conn1.user1_id = ?)`, userID).Scan(ctx, &hops)

	return hops, err
}

// Connected reports whether two users are directly connected.
func (s *Store) Connected(ctx context.Context, userID, otherID string) (bool, error) {
	return s.query().NewSelect().Model((*model.UserConnection)(nil)).
		Where("?TableAlias.user1_id = ?", userID).
		Where("?TableAlias.user2_id = ?", otherID).
		Exists(ctx)
}

// ConnectedThroughOne reports whether a path of length two joins the users.
func (s *Store) ConnectedThroughOne(ctx context.Context, userID, otherID string) (bool, error) {
	return s.query().NewSelect().Model((*model.UserConnection)(nil)).
		Join("LEFT OUTER JOIN user_connections conn2 on ?TableAlias.user2_id = conn2.user1_id").
		Where("?TableAlias.user1_id = ?", userID).
		Where("conn2.user2_id = ?", otherID).
		Exists(ctx)
}
