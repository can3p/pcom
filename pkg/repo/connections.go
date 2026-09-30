package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// CreateConnection stores a connection between two users. Since connections
// form an undirected graph, the ids go in in both combinations to simplify
// queries, at the cost of twice the rows. It is meant to run in a
// transaction. The (user1, user2) row is returned first.
func (s *Store) CreateConnection(ctx context.Context, user1ID, user2ID string) (*core.UserConnection, *core.UserConnection, error) {
	id1, err := uuid.NewV7()
	if err != nil {
		return nil, nil, err
	}

	conn1 := &core.UserConnection{ID: id1.String(), User1ID: user1ID, User2ID: user2ID}
	if err := conn1.Insert(ctx, s.exec, boil.Infer()); err != nil {
		return nil, nil, err
	}

	id2, err := uuid.NewV7()
	if err != nil {
		return nil, nil, err
	}

	conn2 := &core.UserConnection{ID: id2.String(), User1ID: user2ID, User2ID: user1ID}
	if err := conn2.Insert(ctx, s.exec, boil.Infer()); err != nil {
		return nil, nil, err
	}

	return conn1, conn2, nil
}

// DeleteConnectionsBetween removes the connection of two users, in both
// directions, together with the whitelist grant and the mediation request
// (and its mediators) that led to it. Without a connection it does nothing.
// It is meant to run in a transaction.
func (s *Store) DeleteConnectionsBetween(ctx context.Context, userID, otherID string) error {
	conns, err := core.UserConnections(
		qm.Expr(
			core.UserConnectionWhere.User1ID.EQ(userID),
			core.UserConnectionWhere.User2ID.EQ(otherID),
		),
		qm.Or2(qm.Expr(
			core.UserConnectionWhere.User1ID.EQ(otherID),
			core.UserConnectionWhere.User2ID.EQ(userID),
		)),
		qm.Load(core.UserConnectionRels.ConnectionWhitelistedConnection),
		qm.Load(qm.Rels(
			core.UserConnectionRels.ConnectionUserConnectionMediationRequest,
			core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
		)),
	).All(ctx, s.exec)
	if err != nil {
		return err
	}

	for _, conn := range conns {
		if wl := conn.R.ConnectionWhitelistedConnection; wl != nil {
			if _, err := wl.Delete(ctx, s.exec); err != nil {
				return err
			}
		}

		if request := conn.R.ConnectionUserConnectionMediationRequest; request != nil {
			for _, mediator := range request.R.MediationUserConnectionMediators {
				if _, err := mediator.Delete(ctx, s.exec); err != nil {
					return err
				}
			}

			if _, err := request.Delete(ctx, s.exec); err != nil {
				return err
			}
		}

		if _, err := conn.Delete(ctx, s.exec); err != nil {
			return err
		}
	}

	return nil
}

// ConnectionUsers returns the users with the given ids.
func (s *Store) ConnectionUsers(ctx context.Context, ids []string) (core.UserSlice, error) {
	return core.Users(core.UserWhere.ID.IN(ids)).All(ctx, s.exec)
}
