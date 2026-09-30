package posts

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

// CommentInput is a comment as its author submitted it.
type CommentInput struct {
	PostID string
	// ReplyTo is the comment being answered, empty for a top level comment.
	ReplyTo string
	Body    string
}

// ValidateCommentBody checks the length of a comment.
func ValidateCommentBody(body string) error {
	return validation.ValidateMinMax("body", body, 3, 6_000)
}

// CheckComment reports whether the actor may leave a comment on the post,
// as a reply to replyTo if it is not empty: ErrNotFound for a post or a
// comment that is not there, ErrForbidden for a post the actor is not
// connected to.
func (s *Service) CheckComment(ctx context.Context, actor *core.User, postID, replyTo string) error {
	if err := requireActor(actor); err != nil {
		return err
	}

	_, err := s.checkComment(ctx, s.store, actor, postID, replyTo)

	return err
}

func (s *Service) checkComment(ctx context.Context, store *repo.Store, actor *core.User, postID, replyTo string) (*core.Post, error) {
	post, err := store.PostByID(ctx, postID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	radius, err := graph.RadiusBetween(ctx, store, actor.ID, post.UserID)
	if err != nil {
		return nil, err
	}

	if !postops.GetPostCapabilities(radius).CanLeaveComments {
		return nil, service.ErrForbidden
	}

	if replyTo != "" {
		if _, err := store.CommentInPost(ctx, replyTo, postID); errors.Is(err, repo.ErrNotFound) {
			return nil, service.ErrNotFound
		} else if err != nil {
			return nil, err
		}
	}

	return post, nil
}

// AddComment leaves a comment on a post the actor is connected to, and tells
// the post's author and everybody else who commented on it.
func (s *Service) AddComment(ctx context.Context, actor *core.User, in CommentInput) error {
	if err := requireActor(actor); err != nil {
		return err
	}

	if err := ValidateCommentBody(in.Body); err != nil {
		return service.Invalid("body", err.Error())
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if _, err := s.checkComment(ctx, tx, actor, in.PostID, in.ReplyTo); err != nil {
			return err
		}

		return s.addComment(ctx, tx, actor, in)
	})
}

func (s *Service) addComment(ctx context.Context, tx *repo.Store, actor *core.User, in CommentInput) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	// we really want time ordered uuids for data locality, pagination etc
	commentID := id.String()
	// always keep the id of the top comment in the thread for simpler queries
	topCommentID := commentID

	if in.ReplyTo != "" {
		parent, err := tx.CommentByID(ctx, in.ReplyTo)
		if err != nil {
			return err
		}

		topCommentID = parent.TopCommentID
	}

	comment := &core.PostComment{
		ID:              commentID,
		UserID:          actor.ID,
		Body:            strings.TrimSpace(in.Body),
		PostID:          in.PostID,
		ParentCommentID: null.NewString(in.ReplyTo, in.ReplyTo != ""),
		TopCommentID:    topCommentID,
	}

	if err := tx.InsertComment(ctx, comment); err != nil {
		return err
	}

	post, err := tx.PostWithAuthorAndURL(ctx, in.PostID)
	if err != nil {
		return err
	}

	// notify post author about discussion
	out, err := mail.PostCommentAuthor(links.MediaReplacer, actor, post.R.User, post, comment)
	if err := s.queueE(ctx, tx, out, err); err != nil {
		return err
	}

	// notify anyone else who left a comment to the post about discussion
	// @TODO: it's a lame implementation, we should schedule all emails and send them
	// in a separate process
	// also, we might want to notify users per thread, not in a blanket way
	// and give them an ability to unsubscribe
	participants, err := tx.CommentParticipants(ctx, post.ID, post.UserID)
	if err != nil {
		return err
	}

	slog.Debug("comment in the post", "participants", len(participants))

	for _, cmt := range participants {
		out, err := mail.PostCommentParticipants(links.MediaReplacer, actor, cmt.R.User, post, comment)
		if err := s.queueE(ctx, tx, out, err); err != nil {
			return err
		}
	}

	return tx.CountNewComment(ctx, in.PostID)
}
