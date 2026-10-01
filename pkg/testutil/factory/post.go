package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// PostOpt customizes a Post before it is inserted.
type PostOpt func(*core.Post)

// Published marks the post as published now. A Post is an unpublished draft
// by default.
func Published() PostOpt {
	return func(p *core.Post) {
		p.PublishedAt = null.TimeFrom(time.Now())
	}
}

// PublishedAt marks the post as published at t, for data that depends on
// the order of posts.
func PublishedAt(t time.Time) PostOpt {
	return func(p *core.Post) {
		p.PublishedAt = null.TimeFrom(t)
	}
}

// Visibility overrides the post's visibility radius (direct_only by
// default).
func Visibility(v core.PostVisibility) PostOpt {
	return func(p *core.Post) {
		p.VisibilityRadius = v
	}
}

// WithSubject overrides the made-up subject.
func WithSubject(subject string) PostOpt {
	return func(p *core.Post) {
		p.Subject = null.StringFrom(subject)
	}
}

// WithBody overrides the made-up body.
func WithBody(body string) PostOpt {
	return func(p *core.Post) {
		p.Body = body
	}
}

// WithURL attaches the post to an already-created NormalizedURL.
func WithURL(urlID string) PostOpt {
	return func(p *core.Post) {
		p.URLID = null.StringFrom(urlID)
	}
}

// PostUpdatedAt backdates the post's UpdatedAt timestamp, for tests that
// depend on post order.
func PostUpdatedAt(t time.Time) PostOpt {
	return func(p *core.Post) {
		p.UpdatedAt = null.TimeFrom(t)
	}
}

// Post inserts a draft, direct_only post owned by authorID.
func Post(ctx context.Context, exec boil.ContextExecutor, authorID string, opts ...PostOpt) (*core.Post, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	p := &core.Post{
		ID:               id,
		Subject:          null.StringFrom(fmt.Sprintf("Test post %d", n)),
		Body:             fmt.Sprintf("Test post body %d", n),
		UserID:           authorID,
		VisibilityRadius: core.PostVisibilityDirectOnly,
	}

	for _, opt := range opts {
		opt(p)
	}

	if err := p.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return p, nil
}

// CommentOpt customizes a PostComment before it is inserted.
type CommentOpt func(*core.PostComment)

// ReplyTo makes the new comment a reply to the existing comment commentID,
// inheriting its thread's top comment the way the comment form does.
func ReplyTo(commentID string) CommentOpt {
	return func(c *core.PostComment) {
		c.ParentCommentID = null.StringFrom(commentID)
	}
}

// WithCommentBody overrides the made-up comment body.
func WithCommentBody(body string) CommentOpt {
	return func(c *core.PostComment) {
		c.Body = body
	}
}

// CommentCreatedAt backdates the comment, for data that depends on the order
// of comments.
func CommentCreatedAt(t time.Time) CommentOpt {
	return func(c *core.PostComment) {
		c.CreatedAt = t
	}
}

// Comment inserts a top-level comment on postID by authorID, or a reply
// when ReplyTo is given.
func Comment(ctx context.Context, exec boil.ContextExecutor, postID, authorID string, opts ...CommentOpt) (*core.PostComment, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	c := &core.PostComment{
		ID:           id,
		UserID:       authorID,
		PostID:       postID,
		Body:         fmt.Sprintf("Test comment %d", n),
		TopCommentID: id,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.ParentCommentID.Valid {
		parent, err := core.FindPostComment(ctx, exec, c.ParentCommentID.String)
		if err != nil {
			return nil, err
		}

		c.TopCommentID = parent.TopCommentID
	}

	if err := c.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return c, nil
}

// PostStat inserts the cached stats row (one per post) for postID.
func PostStat(ctx context.Context, exec boil.ContextExecutor, postID string) (*core.PostStat, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	s := &core.PostStat{
		ID:     id,
		PostID: postID,
	}

	if err := s.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return s, nil
}

// PostShare marks postID as shared (one row per post).
func PostShare(ctx context.Context, exec boil.ContextExecutor, postID string) (*core.PostShare, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	s := &core.PostShare{
		ID:     id,
		PostID: postID,
	}

	if err := s.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return s, nil
}

// PostPromptOpt customizes a PostPrompt before it is inserted.
type PostPromptOpt func(*core.PostPrompt)

// WithPost attaches the prompt to the post written in answer to it.
func WithPost(postID string) PostPromptOpt {
	return func(p *core.PostPrompt) {
		p.PostID = null.StringFrom(postID)
	}
}

// WithPromptMessage overrides the made-up prompt message.
func WithPromptMessage(msg string) PostPromptOpt {
	return func(p *core.PostPrompt) {
		p.Message = msg
	}
}

// Dismissed marks the prompt as dismissed by its recipient.
func Dismissed() PostPromptOpt {
	return func(p *core.PostPrompt) {
		p.DismissedAt = null.TimeFrom(time.Now())
	}
}

// PromptCreatedAt backdates the prompt, for tests that depend on prompt order.
func PromptCreatedAt(t time.Time) PostPromptOpt {
	return func(p *core.PostPrompt) {
		p.CreatedAt = t
	}
}

// PostPrompt inserts askerID's prompt asking recipientID to write about
// something.
func PostPrompt(ctx context.Context, exec boil.ContextExecutor, askerID, recipientID string, opts ...PostPromptOpt) (*core.PostPrompt, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	p := &core.PostPrompt{
		ID:          id,
		AskerID:     askerID,
		RecipientID: recipientID,
		Message:     fmt.Sprintf("Test prompt %d", n),
	}

	for _, opt := range opts {
		opt(p)
	}

	if err := p.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return p, nil
}

// NormalizedURL inserts a made-up, unique URL.
func NormalizedURL(ctx context.Context, exec boil.ContextExecutor) (*core.NormalizedURL, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	u := &core.NormalizedURL{
		ID:  id,
		URL: fmt.Sprintf("https://example.test/url/%d", n),
	}

	if err := u.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return u, nil
}
