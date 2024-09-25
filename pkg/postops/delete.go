package postops

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func DeletePost(ctx context.Context, exec boil.ContextExecutor, postID string) error {
	if _, err := core.PostComments(
		core.PostCommentWhere.PostID.EQ(postID),
	).DeleteAll(ctx, exec); err != nil {
		return err
	}

	if _, err := core.PostStats(
		core.PostStatWhere.PostID.EQ(postID),
	).DeleteAll(ctx, exec); err != nil {
		return err
	}

	if _, err := core.PostShares(
		core.PostShareWhere.PostID.EQ(postID),
	).DeleteAll(ctx, exec); err != nil {
		return err
	}

	if _, err := core.PostPrompts(
		core.PostPromptWhere.PostID.EQ(null.StringFrom(postID)),
	).DeleteAll(ctx, exec); err != nil {
		return err
	}

	if _, err := core.Posts(
		core.PostWhere.ID.EQ(postID),
	).DeleteAll(ctx, exec); err != nil {
		return err
	}

	return nil
}
