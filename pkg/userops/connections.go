package userops

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// CreateConnection assumes it's run in transaction
// Since connections form an undirected graph, we insert
// user ids in both combinations to simplify queries
// with a tradeoff that we need to monitor data for consistency and
// use twice the size needed
func CreateConnection(ctx context.Context, db boil.ContextExecutor, user1ID string, user2ID string) error {
	conn1 := &core.UserConnection{
		ID:      uuid.NewString(),
		User1ID: user1ID,
		User2ID: user2ID,
	}

	if err := conn1.Insert(ctx, db, boil.Infer()); err != nil {
		return err
	}

	conn2 := &core.UserConnection{
		ID:      uuid.NewString(),
		User1ID: user2ID,
		User2ID: user1ID,
	}

	if err := conn2.Insert(ctx, db, boil.Infer()); err != nil {
		return err
	}

	return nil
}
