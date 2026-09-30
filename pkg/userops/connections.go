package userops

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// The connection logic moved to pkg/service/connections; these wrappers keep
// the callers RS has not converted yet (signup, the post page) working.

// CreateConnection assumes it's run in transaction.
func CreateConnection(ctx context.Context, db boil.ContextExecutor, user1ID string, user2ID string) (*core.UserConnection, *core.UserConnection, error) {
	return repo.Using(db).CreateConnection(ctx, user1ID, user2ID)
}

// IsConnectionAllowed is used to determine whether sourceUserID is allowed to connect with targetUserID
func IsConnectionAllowed(ctx context.Context, db boil.ContextExecutor, sourceUserID string, targetUserID string) (bool, error) {
	return repo.Using(db).OpenGrantExists(ctx, targetUserID, sourceUserID)
}

func GetMediationRequest(ctx context.Context, db boil.ContextExecutor, sourceUserID string, targetUserID string) (*core.UserConnectionMediationRequest, error) {
	req, err := repo.Using(db).MediationRequestBetween(ctx, sourceUserID, targetUserID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}

	return req, err
}
