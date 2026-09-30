package userops

import (
	"context"
	"database/sql"
	"time"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

var ErrNoConnectionRequest = errors.Errorf("No such connection request")

// CreateConnection assumes it's run in transaction
// Since connections form an undirected graph, we insert
// user ids in both combinations to simplify queries
// with a tradeoff that we need to monitor data for consistency and
// use twice the size needed
func CreateConnection(ctx context.Context, db boil.ContextExecutor, user1ID string, user2ID string) (*core.UserConnection, *core.UserConnection, error) {
	userConnectionID1, err := uuid.NewV7()

	if err != nil {
		return nil, nil, err
	}

	conn1 := &core.UserConnection{
		ID:      userConnectionID1.String(),
		User1ID: user1ID,
		User2ID: user2ID,
	}

	if err := conn1.Insert(ctx, db, boil.Infer()); err != nil {
		return nil, nil, err
	}

	userConnectionID2, err := uuid.NewV7()

	if err != nil {
		return nil, nil, err
	}

	conn2 := &core.UserConnection{
		ID:      userConnectionID2.String(),
		User1ID: user2ID,
		User2ID: user1ID,
	}

	if err := conn2.Insert(ctx, db, boil.Infer()); err != nil {
		return nil, nil, err
	}

	return conn1, conn2, nil
}

func DropConnectionGrant(ctx context.Context, exec boil.ContextExecutor, whoUserID string, allowsWhoUserID string) error {
	_, err := core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(whoUserID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(allowsWhoUserID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
	).DeleteAll(ctx, exec)

	return err
}

// IsConnectionAllowed is used to determine whether sourceUserID is allowed to connect with targetUserID
func IsConnectionAllowed(ctx context.Context, exec boil.ContextExecutor, sourceUserID string, targetUserID string) (bool, error) {
	whitelistExists, err := core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(targetUserID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(sourceUserID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
	).Exists(ctx, exec)

	if err != nil {
		return false, err
	}

	return whitelistExists, nil
}

func EstablishConnection(ctx context.Context, exec *sqlx.DB, sourceUserID string, targetUserID string) error {
	return transact.Transact(exec, func(tx *sql.Tx) error {
		whitelisted, err := core.WhitelistedConnections(
			core.WhitelistedConnectionWhere.WhoID.EQ(targetUserID),
			core.WhitelistedConnectionWhere.AllowsWhoID.EQ(sourceUserID),
			core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
			qm.For("UPDATE"),
		).One(ctx, tx)

		if err == sql.ErrNoRows {
			return errors.Errorf("No connection allowed")
		} else if err != nil {
			return err
		}

		if err := revokeMediationRequestInTx(ctx, tx, sourceUserID, targetUserID); err != nil && err != ErrNoConnectionRequest {
			return err
		}

		conn1, _, err := CreateConnection(ctx, tx, sourceUserID, targetUserID)

		if err != nil {
			return err
		}

		whitelisted.ConnectionID = null.StringFrom(conn1.ID)

		_, err = whitelisted.Update(ctx, tx, boil.Infer())

		return err
	})
}

func DropConnection(ctx context.Context, exec *sqlx.DB, sourceUserID string, targetUserID string) error {
	return transact.Transact(exec, func(tx *sql.Tx) error {
		conns, err := core.UserConnections(
			qm.Expr(
				core.UserConnectionWhere.User1ID.EQ(sourceUserID),
				core.UserConnectionWhere.User2ID.EQ(targetUserID),
			),
			qm.Or2(qm.Expr(
				core.UserConnectionWhere.User1ID.EQ(targetUserID),
				core.UserConnectionWhere.User2ID.EQ(sourceUserID),
			)),
			qm.Load(core.UserConnectionRels.ConnectionWhitelistedConnection),
			qm.Load(qm.Rels(
				core.UserConnectionRels.ConnectionUserConnectionMediationRequest,
				core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			)),
		).All(ctx, tx)

		if err != nil {
			return err
		}

		// no connection = nothing to do
		if len(conns) == 0 {
			return nil
		}

		for _, conn := range conns {
			if wl := conn.R.ConnectionWhitelistedConnection; wl != nil {
				if _, err := wl.Delete(ctx, tx); err != nil {
					return err
				}
			}

			if request := conn.R.ConnectionUserConnectionMediationRequest; request != nil {
				for _, mediations := range request.R.MediationUserConnectionMediators {
					if _, err := mediations.Delete(ctx, tx); err != nil {
						return err
					}
				}

				if _, err := request.Delete(ctx, tx); err != nil {
					return err
				}
			}

			if _, err := conn.Delete(ctx, tx); err != nil {
				return err
			}
		}

		return nil
	})

}

func GetMediationRequest(ctx context.Context, exec boil.ContextExecutor, sourceUserID string, targetUserID string) (*core.UserConnectionMediationRequest, error) {
	mediationRequest, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.EQ(sourceUserID),
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetUserID),
	).One(ctx, exec)

	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return mediationRequest, nil
}

func RequestMediation(ctx context.Context, exec *sqlx.DB, sourceUserID string, targetUserID string, mediationNote string) error {
	radius, err := GetConnectionRadius(ctx, exec, sourceUserID, targetUserID)

	if err != nil {
		return err
	}

	if radius != ConnectionRadiusSecondDegree {
		return errors.Errorf("You cannot request mediation with the user without common connections")
	}

	mediationExists, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.EQ(sourceUserID),
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetUserID),
	).Exists(ctx, exec)

	if err != nil {
		return err
	}

	if mediationExists {
		return errors.Errorf("Mediation has already been requested")
	}

	if exists, err := core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(targetUserID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(sourceUserID),
	).Exists(ctx, exec); err != nil {
		return err
	} else if exists {
		return errors.Errorf("Cannot create a connection request for a whitelisted connection")
	}

	id, err := uuid.NewV7()

	if err != nil {
		return err
	}

	mediationRequest := &core.UserConnectionMediationRequest{
		ID:           id.String(),
		WhoUserID:    sourceUserID,
		TargetUserID: targetUserID,
		SourceNote:   null.NewString(mediationNote, mediationNote != ""),
	}

	return mediationRequest.Insert(ctx, exec, boil.Infer())
}

func RevokeMediationRequest(ctx context.Context, exec *sqlx.DB, whoUserID string, targetUserID string) error {
	return transact.Transact(exec, func(tx *sql.Tx) error {
		return revokeMediationRequestInTx(ctx, tx, whoUserID, targetUserID)
	})
}

func revokeMediationRequestInTx(ctx context.Context, tx *sql.Tx, whoUserID string, targetUserID string) error {
	request, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.EQ(whoUserID),
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetUserID),
		qm.Load(core.UserConnectionMediationRequestRels.MediationUserConnectionMediators, qm.For("UPDATE")),
		qm.For("UPDATE"),
	).One(ctx, tx)

	if err == sql.ErrNoRows {
		return ErrNoConnectionRequest
	} else if err != nil {
		return err
	}

	for _, m := range request.R.MediationUserConnectionMediators {
		if _, err := m.Delete(ctx, tx); err != nil {
			return err
		}
	}

	_, err = request.Delete(ctx, tx)

	return err
}

func DecideForwardMediationRequest(ctx context.Context, exec *sqlx.DB, whoUserID string, requestID string, decision core.ConnectionMediationDecision, note string) error {
	directUserIDs, err := GetDirectUserIDs(ctx, exec, whoUserID)

	if err != nil {
		return err
	}

	request, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.ID.EQ(requestID),
		core.UserConnectionMediationRequestWhere.WhoUserID.IN(directUserIDs),
		core.UserConnectionMediationRequestWhere.TargetUserID.IN(directUserIDs),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
	).One(ctx, exec)

	if err == sql.ErrNoRows {
		return errors.Errorf("No such request")
	} else if err != nil {
		return err
	}

	id, err := uuid.NewV7()

	if err != nil {
		return err
	}

	mediationResult := &core.UserConnectionMediator{
		ID:           id.String(),
		MediationID:  request.ID,
		UserID:       whoUserID,
		Decision:     decision,
		DecidedAt:    time.Now(),
		MediatorNote: null.NewString(note, note != ""),
	}

	return mediationResult.Insert(ctx, exec, boil.Infer())
}

func DecideConnectionRequest(ctx context.Context, exec *sqlx.DB, targetUserID string, requestID string, decision core.ConnectionRequestDecision, note string) error {
	return transact.Transact(exec, func(tx *sql.Tx) error {
		request, err := core.UserConnectionMediationRequests(
			core.UserConnectionMediationRequestWhere.ID.EQ(requestID),
			core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetUserID),
			core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
			qm.For("UPDATE"),
		).One(ctx, tx)

		if err == sql.ErrNoRows {
			return errors.Errorf("No such request")
		} else if err != nil {
			return err
		}

		fromUserID := request.WhoUserID

		if decision == core.ConnectionRequestDecisionApproved {
			conn1, _, err := CreateConnection(ctx, tx, fromUserID, targetUserID)

			if err != nil {
				return err
			}

			request.ConnectionID = null.StringFrom(conn1.ID)
		}

		request.TargetDecision = core.NullConnectionRequestDecisionFrom(decision)
		request.TargetDecidedAt = null.TimeFrom(time.Now())
		request.TargetNote = null.NewString(note, note != "")

		if _, err := request.Update(ctx, tx, boil.Infer()); err != nil {
			return err
		}

		return nil
	})
}
