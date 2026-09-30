package repo

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// CommentsOfPost returns a post's comments, oldest first, with their authors
// loaded.
func (s *Store) CommentsOfPost(ctx context.Context, postID string) (core.PostCommentSlice, error) {
	return core.PostComments(
		core.PostCommentWhere.PostID.EQ(postID),
		qm.Load(core.PostCommentRels.User),
		qm.OrderBy(fmt.Sprintf("%s ASC", core.PostCommentColumns.CreatedAt)),
	).All(ctx, s.exec)
}

// CommentedPostIDs returns the IDs of the posts userID has commented on.
func (s *Store) CommentedPostIDs(ctx context.Context, userID string) ([]string, error) {
	comments, err := core.PostComments(
		core.PostCommentWhere.UserID.EQ(userID),
		qm.Distinct(core.PostCommentColumns.PostID),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.PostID)
	}

	return ids, nil
}

// CommentsOnPostsNotBy returns the comments on postIDs that someone other
// than userID left, newest first, with their authors loaded.
func (s *Store) CommentsOnPostsNotBy(ctx context.Context, postIDs []string, userID string) (core.PostCommentSlice, error) {
	return core.PostComments(
		core.PostCommentWhere.UserID.NEQ(userID),
		core.PostCommentWhere.PostID.IN(postIDs),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostCommentColumns.CreatedAt)),
		qm.Load(core.PostCommentRels.User),
	).All(ctx, s.exec)
}
