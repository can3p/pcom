package repo

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// DraftPosts returns the unpublished posts of a user, the last edited first.
func (s *Store) DraftPosts(ctx context.Context, userID string) ([]*core.Post, error) {
	return core.Posts(
		core.PostWhere.UserID.EQ(userID),
		core.PostWhere.PublishedAt.IsNull(),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.UpdatedAt)),
	).All(ctx, s.exec)
}
