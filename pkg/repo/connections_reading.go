package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
)

// ConnectionAllowed reports whether targetUserID has allowed sourceUserID to
// connect, with a grant no connection has used yet.
func (s *Store) ConnectionAllowed(ctx context.Context, sourceUserID, targetUserID string) (bool, error) {
	return core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(targetUserID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(sourceUserID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
	).Exists(ctx, s.exec)
}
