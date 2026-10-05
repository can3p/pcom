package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx database/sql driver
	"github.com/jmoiron/sqlx"
	"github.com/uptrace/bun"
)

// CommentInPost returns a comment of the given post, or ErrNotFound.
func (s *Store) CommentInPost(ctx context.Context, commentID, postID string) (*model.PostComment, error) {
	comment := new(model.PostComment)
	err := s.query().NewSelect().Model(comment).
		Where("id = ?", commentID).
		Where("post_id = ?", postID).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return comment, nil
}

// CommentByID returns a comment, or ErrNotFound.
func (s *Store) CommentByID(ctx context.Context, id string) (*model.PostComment, error) {
	comment := new(model.PostComment)
	if err := s.query().NewSelect().Model(comment).Where("id = ?", id).Limit(1).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return comment, nil
}

// InsertComment stores a new comment.
func (s *Store) InsertComment(ctx context.Context, comment *model.PostComment) error {
	_, err := s.query().NewInsert().Model(comment).Exec(ctx)
	return err
}

// UpdateCommentBody stores a new body of a comment and marks it as edited.
func (s *Store) UpdateCommentBody(ctx context.Context, comment *model.PostComment, body string) error {
	comment.Body = body
	comment.EditedAt = new(time.Now())

	_, err := s.query().NewUpdate().Model(comment).Column("body", "edited_at").WherePK().Exec(ctx)

	return err
}

// PostWithAuthorAndURL returns a post with its author and linked URL loaded
// (post.User, post.URL).
func (s *Store) PostWithAuthorAndURL(ctx context.Context, id string) (*model.Post, error) {
	post := new(model.Post)
	err := s.query().NewSelect().Model(post).
		Relation("User").
		Relation("URL").
		Where("?TableAlias.id = ?", id).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return post, nil
}

// CommentParticipants returns one comment of every user, other than the
// post's author, who commented on the post, with the commenter loaded
// (comment.User). Only the comment's user_id is read.
func (s *Store) CommentParticipants(ctx context.Context, postID, authorID string) ([]*model.PostComment, error) {
	var comments []*model.PostComment
	err := s.query().NewSelect().Model(&comments).
		DistinctOn("?TableAlias.user_id").
		Column("user_id").
		Relation("User").
		Where("?TableAlias.post_id = ?", postID).
		Where("?TableAlias.user_id <> ?", authorID).
		Scan(ctx)

	return comments, err
}

// CountNewComment adds one to the comment counter of a post.
func (s *Store) CountNewComment(ctx context.Context, postID string) error {
	statID, err := uuid.NewV7()
	if err != nil {
		return err
	}

	postStat := &model.PostStat{
		ID:             statID.String(),
		PostID:         postID,
		CommentsNumber: 1,
	}

	// On conflict only the counter changes; updated_at keeps its value.
	_, err = s.query().NewInsert().Model(postStat).
		On("CONFLICT (post_id) DO UPDATE").
		Set("comments_number = ?TableAlias.comments_number + EXCLUDED.comments_number").
		Exec(ctx)

	return err
}

// CommentForMail returns a comment with its author, its post and the post's
// author and linked URL loaded.
func (s *Store) CommentForMail(ctx context.Context, id string) (*model.PostComment, error) {
	comment := new(model.PostComment)
	err := s.query().NewSelect().Model(comment).
		Relation("Post.URL").
		Relation("Post.User").
		Relation("User").
		Where("?TableAlias.id = ?", id).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return comment, nil
}

// ConnectPostgres opens a store on the database at dsn, for commands that
// run outside the web server. Close it with the returned function.
func ConnectPostgres(dsn string) (*Store, func() error, error) {
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}

	return New(db), db.Close, nil
}

// CommentsOfPost returns a post's comments, oldest first, with their authors
// loaded.
func (s *Store) CommentsOfPost(ctx context.Context, postID string) ([]*model.PostComment, error) {
	var comments []*model.PostComment
	err := s.query().NewSelect().Model(&comments).
		Relation("User").
		Where("?TableAlias.post_id = ?", postID).
		OrderExpr("?TableAlias.created_at ASC").
		Scan(ctx)

	return comments, err
}

// CommentedPostIDs returns the IDs of the posts userID has commented on.
func (s *Store) CommentedPostIDs(ctx context.Context, userID string) ([]string, error) {
	ids := []string{}

	err := s.query().NewSelect().Model((*model.PostComment)(nil)).
		Distinct().
		Column("post_id").
		Where("user_id = ?", userID).
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// CommentsOnPostsNotBy returns a page of the comments on postIDs that
// someone other than userID left, newest first, with their authors loaded.
func (s *Store) CommentsOnPostsNotBy(ctx context.Context, postIDs []string, userID string, page Page) ([]*model.PostComment, error) {
	var comments []*model.PostComment

	q := s.query().NewSelect().Model(&comments).
		Relation("User").
		Where("?TableAlias.user_id <> ?", userID).
		Where("?TableAlias.post_id IN (?)", bun.List(postIDs))

	err := page.apply(q, KindComment, "?TableAlias.created_at", "?TableAlias.id").Scan(ctx)

	return comments, err
}
