package repo

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // postgres db driver
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// CommentInPost returns a comment of the given post, or ErrNotFound.
func (s *Store) CommentInPost(ctx context.Context, commentID, postID string) (*core.PostComment, error) {
	comment, err := core.PostComments(
		core.PostCommentWhere.ID.EQ(commentID),
		core.PostCommentWhere.PostID.EQ(postID),
	).One(ctx, s.exec)

	return comment, notFound(err)
}

// CommentByID returns a comment, or ErrNotFound.
func (s *Store) CommentByID(ctx context.Context, id string) (*core.PostComment, error) {
	comment, err := core.PostComments(core.PostCommentWhere.ID.EQ(id)).One(ctx, s.exec)

	return comment, notFound(err)
}

// InsertComment stores a new comment.
func (s *Store) InsertComment(ctx context.Context, comment *core.PostComment) error {
	return comment.Insert(ctx, s.exec, boil.Infer())
}

// PostWithAuthorAndURL returns a post with its author and linked URL loaded
// (post.R.User, post.R.URL).
func (s *Store) PostWithAuthorAndURL(ctx context.Context, id string) (*core.Post, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(id),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.URL),
	).One(ctx, s.exec)

	return post, notFound(err)
}

// CommentParticipants returns one comment of every user, other than the
// post's author, who commented on the post, with the commenter loaded
// (comment.R.User).
func (s *Store) CommentParticipants(ctx context.Context, postID, authorID string) ([]*core.PostComment, error) {
	return core.PostComments(
		core.PostCommentWhere.PostID.EQ(postID),
		core.PostCommentWhere.UserID.NEQ(authorID),
		qm.Distinct(core.PostCommentColumns.UserID),
		qm.Load(core.PostCommentRels.User),
	).All(ctx, s.exec)
}

// CountNewComment adds one to the comment counter of a post.
func (s *Store) CountNewComment(ctx context.Context, postID string) error {
	statID, err := uuid.NewV7()
	if err != nil {
		return err
	}

	postStat := &core.PostStat{
		ID:             statID.String(),
		PostID:         postID,
		CommentsNumber: 1,
	}

	return postStat.Upsert(
		ctx, s.exec, true, []string{core.PostStatColumns.PostID},
		boil.Whitelist(core.PostStatColumns.UpdatedAt, core.PostStatColumns.CommentsNumber),
		boil.Infer(),
		core.UpsertUpdateSet("comments_number = post_stats.comments_number + excluded.comments_number"),
	)
}

// CommentForMail returns a comment with its author, its post and the post's
// author and linked URL loaded.
func (s *Store) CommentForMail(ctx context.Context, id string) (*core.PostComment, error) {
	comment, err := core.PostComments(
		core.PostCommentWhere.ID.EQ(id),
		qm.Load(qm.Rels(core.PostCommentRels.Post, core.PostRels.URL)),
		qm.Load(qm.Rels(core.PostCommentRels.Post, core.PostRels.User)),
		qm.Load(core.PostCommentRels.User),
	).One(ctx, s.exec)

	return comment, notFound(err)
}

// ConnectPostgres opens a store on the database at dsn, for commands that
// run outside the web server. Close it with the returned function.
func ConnectPostgres(dsn string) (*Store, func() error, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, nil, err
	}

	return New(db), db.Close, nil
}

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
