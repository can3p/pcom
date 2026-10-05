package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// CreateConnection stores a connection between two users. Since connections
// form an undirected graph, the ids go in in both combinations to simplify
// queries, at the cost of twice the rows. It is meant to run in a
// transaction. The (user1, user2) row is returned first.
func (s *Store) CreateConnection(ctx context.Context, user1ID, user2ID string) (*model.UserConnection, *model.UserConnection, error) {
	id1, err := uuid.NewV7()
	if err != nil {
		return nil, nil, err
	}

	conn1 := &model.UserConnection{ID: id1.String(), User1ID: user1ID, User2ID: user2ID}
	if _, err := s.query().NewInsert().Model(conn1).Exec(ctx); err != nil {
		return nil, nil, err
	}

	id2, err := uuid.NewV7()
	if err != nil {
		return nil, nil, err
	}

	conn2 := &model.UserConnection{ID: id2.String(), User1ID: user2ID, User2ID: user1ID}
	if _, err := s.query().NewInsert().Model(conn2).Exec(ctx); err != nil {
		return nil, nil, err
	}

	return conn1, conn2, nil
}

// DeleteConnectionsBetween removes the connection of two users, in both
// directions, together with the whitelist grant and the mediation request
// (and its mediators) that led to it. Without a connection it does nothing.
// It is meant to run in a transaction.
func (s *Store) DeleteConnectionsBetween(ctx context.Context, userID, otherID string) error {
	var conns []*model.UserConnection

	err := s.query().NewSelect().Model(&conns).
		Relation("ConnectionWhitelistedConnection").
		Relation("ConnectionUserConnectionMediationRequest.MediationUserConnectionMediators").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
					return q.Where("?TableAlias.user1_id = ?", userID).Where("?TableAlias.user2_id = ?", otherID)
				}).
				WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
					return q.Where("?TableAlias.user1_id = ?", otherID).Where("?TableAlias.user2_id = ?", userID)
				})
		}).
		Scan(ctx)
	if err != nil {
		return err
	}

	for _, conn := range conns {
		if wl := conn.ConnectionWhitelistedConnection; wl != nil {
			if _, err := s.query().NewDelete().Model(wl).WherePK().Exec(ctx); err != nil {
				return err
			}
		}

		if request := conn.ConnectionUserConnectionMediationRequest; request != nil {
			for _, mediator := range request.MediationUserConnectionMediators {
				if _, err := s.query().NewDelete().Model(mediator).WherePK().Exec(ctx); err != nil {
					return err
				}
			}

			if _, err := s.query().NewDelete().Model(request).WherePK().Exec(ctx); err != nil {
				return err
			}
		}

		if _, err := s.query().NewDelete().Model(conn).WherePK().Exec(ctx); err != nil {
			return err
		}
	}

	return nil
}
