package userops

import (
	"context"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var ErrUserNotSignedIn = graph.ErrUserNotSignedIn

// The graph reads moved to pkg/service/graph; these wrappers keep the
// callers RS has not converted yet working.

func GetDirectUserIDs(ctx context.Context, db boil.ContextExecutor, userID string) ([]string, error) {
	return graph.DirectUserIDs(ctx, repo.Using(db), userID)
}

func GetDirectAndSecondDegreeUserIDs(ctx context.Context, db boil.ContextExecutor, userID string) (directUserIDs []string, secondDegreeUserIDs []string, via map[string][]string, err error) {
	return graph.DirectAndSecondDegree(ctx, repo.Using(db), userID)
}

type ConnectionRadius = graph.Radius

const (
	ConnectionRadiusSameUser     = graph.RadiusSameUser
	ConnectionRadiusDirect       = graph.RadiusDirect
	ConnectionRadiusSecondDegree = graph.RadiusSecondDegree
	ConnectionRadiusUnrelated    = graph.RadiusUnrelated
	ConnectionRadiusUnknown      = graph.RadiusUnknown
)

func GetConnectionRadius(ctx context.Context, db boil.ContextExecutor, fromUserID string, toUserID string) (ConnectionRadius, error) {
	return graph.RadiusBetween(ctx, repo.Using(db), fromUserID, toUserID)
}
