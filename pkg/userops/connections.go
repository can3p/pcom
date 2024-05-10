package userops

import (
	"context"
	"database/sql"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

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

func GetDirectUserIDs(ctx context.Context, db boil.ContextExecutor, userID string) ([]string, error) {
	connections, err := core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(userID),
	).All(ctx, db)

	if err != nil {
		return nil, err
	}

	return lo.Map(connections, func(conn *core.UserConnection, index int) string { return conn.User2ID }), nil
}

func GetDirectAndSecondDegreeUserIDs(ctx context.Context, db boil.ContextExecutor, userID string) (directUserIDs []string, secondDegreeUserIDs []string, err error) {
	connections, err := core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(userID),
	).All(ctx, db)

	if err != nil {
		return nil, nil, err
	}

	// we want to explude direct connection as well as the user themselves from secondDegree connections results
	// this has to be done, since the connection graph is undirected
	directUserIDs = lo.Map(connections, func(conn *core.UserConnection, index int) string { return conn.User2ID })
	toExclude := []string{userID}
	toExclude = append(toExclude, directUserIDs...)

	type connResult struct {
		UserID string `boil:"user_id"`
	}

	var secondDegreeUserIDStruct []*connResult

	// https://www.linkedin.com/pulse/you-dont-need-graph-database-modeling-graphs-trees-viktor-qvarfordt-efzof/
	err = core.NewQuery(
		qm.Select("conn2.user2_id as user_id"),
		qm.From("user_connections as conn1"),
		qm.LeftOuterJoin("user_connections as conn2 on conn1.user2_id = conn2.user1_id"),
		qm.Where("conn1.user1_id = ?", userID),
	).Bind(ctx, db, &secondDegreeUserIDStruct)

	if err != nil {
		return nil, nil, err
	}

	secondDegreeUserIDs = lo.Without(
		lo.Map(secondDegreeUserIDStruct, func(conn *connResult, index int) string { return conn.UserID }),
		toExclude...,
	)

	return directUserIDs, secondDegreeUserIDs, nil
}

type ConnectionRadius int

const (
	ConnectionRadiusSameUser ConnectionRadius = iota
	ConnectionRadiusDirect
	ConnectionRadiusSecondDegree
	ConnectionRadiusUnrelated
	ConnectionRadiusUnknown
)

func (cr ConnectionRadius) IsSameUser() bool {
	return cr == ConnectionRadiusSameUser
}

func (cr ConnectionRadius) IsDirect() bool {
	return cr == ConnectionRadiusDirect
}

func (cr ConnectionRadius) IsSecondDegree() bool {
	return cr == ConnectionRadiusSecondDegree
}

func (cr ConnectionRadius) IsUnrelated() bool {
	return cr == ConnectionRadiusUnrelated
}

func GetConnectionRadius(ctx context.Context, db boil.ContextExecutor, fromUserID string, toUserID string) (ConnectionRadius, error) {
	if fromUserID == toUserID {
		return ConnectionRadiusSameUser, nil
	}

	directConnectionExists, err := core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(fromUserID),
		core.UserConnectionWhere.User2ID.EQ(toUserID),
	).Exists(ctx, db)

	if err != nil {
		return ConnectionRadiusUnknown, err
	}

	if directConnectionExists {
		return ConnectionRadiusDirect, nil
	}

	secondDegreeConnectionExists, err := core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(fromUserID),
		qm.LeftOuterJoin("user_connections conn2 on user_connections.user2_id = conn2.user1_id"),
		qm.Where("conn2.user2_id = ?", toUserID),
	).Exists(ctx, db)

	if err != nil {
		return ConnectionRadiusUnknown, err
	}

	if secondDegreeConnectionExists {
		return ConnectionRadiusSecondDegree, nil
	}

	return ConnectionRadiusUnrelated, nil
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

			if _, err := conn.Delete(ctx, tx); err != nil {
				return err
			}
		}

		return nil
	})

}
