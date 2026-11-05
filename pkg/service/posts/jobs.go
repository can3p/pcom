package posts

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/riverqueue/river"
)

// WithJobs sets the queue the service enqueues its background jobs on.
func WithJobs(q repo.JobQueue) Option {
	return func(s *Service) {
		s.jobs = q
	}
}

// RegisterJobs registers the workers of the service's job kinds on r.
func (s *Service) RegisterJobs(r *jobs.Registry) {
	jobs.Register(r, s.FanOutPublished)
	jobs.RegisterMail(r, s.RenderPrompt)
	jobs.RegisterMail(r, s.RenderPromptAnswer)
	jobs.RegisterMail(r, s.RenderNewPost)
	jobs.RegisterMail(r, s.RenderComment)
}

// PublishedArgs is the job saving a published post enqueues: it tells the
// author's direct connections. It is unique by post, so a post saved again
// before the job ran is fanned out once.
type PublishedArgs struct {
	PostID string `json:"post_id"`
}

// Kind names the job kind.
func (PublishedArgs) Kind() string { return "posts.published" }

// InsertOpts makes the job unique by post.
func (PublishedArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// PromptMailArgs is the mail that tells a user about a prompt to write.
type PromptMailArgs struct {
	PromptID    string `json:"prompt_id"`
	RecipientID string `json:"recipient_id"`
}

// Kind names the job kind.
func (PromptMailArgs) Kind() string { return "mail.post_prompt" }

// PromptAnswerMailArgs is the mail that tells the asker of a prompt that the
// post answering it is published.
type PromptAnswerMailArgs struct {
	PostID string `json:"post_id"`
}

// Kind names the job kind.
func (PromptAnswerMailArgs) Kind() string { return "mail.post_prompt_answer" }

// NewPostMailArgs is the mail that tells a connection of the author about a
// published post.
type NewPostMailArgs struct {
	PostID       string `json:"post_id"`
	ConnectionID string `json:"connection_id"`
}

// Kind names the job kind.
func (NewPostMailArgs) Kind() string { return "mail.post_notification" }

// CommentMailArgs is the mail about a new or edited comment to the post's
// author or to another participant of the discussion; the recipient decides
// which of the two it is.
type CommentMailArgs struct {
	CommentID   string `json:"comment_id"`
	RecipientID string `json:"recipient_id"`
	Edited      bool   `json:"edited"`
}

// Kind names the job kind.
func (CommentMailArgs) Kind() string { return "mail.comment_notification" }

// gone turns a row that no longer exists into "nothing to send".
func gone[T any](v T, err error) (T, bool, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return v, true, nil
	}

	return v, false, err
}

// FanOutPublished enqueues the new post mail of every direct connection of
// the post's author, in one transaction. A mail already sent is dropped by
// its unique id, so running the job again sends nothing twice.
func (s *Service) FanOutPublished(ctx context.Context, a PublishedArgs) error {
	post, missing, err := gone(s.store.PostByID(ctx, a.PostID))
	if missing || err != nil {
		return err
	}

	// a post made draft since it was saved is told to nobody
	if post.PublishedAt == nil {
		return nil
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		ids, err := graph.DirectUserIDs(ctx, tx, post.UserID)
		if err != nil {
			return err
		}

		for _, id := range ids {
			// we're not sending email notifications to ourselves
			if id == post.UserID {
				continue
			}

			if err := tx.Enqueue(ctx, s.jobs, NewPostMailArgs{PostID: post.ID, ConnectionID: id}); err != nil {
				return err
			}
		}

		return nil
	})
}

// RenderPrompt builds the mail about a prompt.
func (s *Service) RenderPrompt(ctx context.Context, a PromptMailArgs) (*mail.Envelope, error) {
	prompt, missing, err := gone(s.store.PromptForRecipient(ctx, a.RecipientID, a.PromptID))
	if missing || err != nil {
		return nil, err
	}

	recipient, missing, err := gone(s.store.UserByID(ctx, a.RecipientID))
	if missing || err != nil {
		return nil, err
	}

	return mail.PostPrompt(s.ident.Site, s.ident.From, prompt.Asker, recipient, prompt), nil
}

// RenderPromptAnswer builds the mail that tells the asker about the post
// answering their prompt.
func (s *Service) RenderPromptAnswer(ctx context.Context, a PromptAnswerMailArgs) (*mail.Envelope, error) {
	post, missing, err := gone(s.store.PostWithAuthorAndURL(ctx, a.PostID))
	if missing || err != nil {
		return nil, err
	}

	prompt, missing, err := gone(s.store.PromptForPost(ctx, a.PostID))
	if missing || err != nil {
		return nil, err
	}

	// the mail names the post, so an asker who may not read it now is not told
	mayRead, err := s.maySee(ctx, prompt.Asker.ID, post)
	if !mayRead || err != nil {
		return nil, err
	}

	return mail.PostPromptAnswer(s.ident.Site, s.ident.From, prompt.Asker, post.User, post, prompt), nil
}

// RenderNewPost builds the mail that tells a connection about a post, as the
// post is now.
func (s *Service) RenderNewPost(ctx context.Context, a NewPostMailArgs) (*mail.Envelope, error) {
	post, missing, err := gone(s.store.PostWithAuthorAndURL(ctx, a.PostID))
	if missing || err != nil {
		return nil, err
	}

	conn, missing, err := gone(s.store.UserByID(ctx, a.ConnectionID))
	if missing || err != nil {
		return nil, err
	}

	mayRead, err := s.maySee(ctx, conn.ID, post)
	if !mayRead || err != nil {
		return nil, err
	}

	return mail.NewPost(s.ident.Site, s.ident.From, s.ident.Site.MediaReplacer, post.User, conn, post), nil
}

// RenderComment builds the mail about a comment, as the comment and its post
// are now.
func (s *Service) RenderComment(ctx context.Context, a CommentMailArgs) (*mail.Envelope, error) {
	comment, missing, err := gone(s.store.CommentByID(ctx, a.CommentID))
	if missing || err != nil {
		return nil, err
	}

	post, missing, err := gone(s.store.PostWithAuthorAndURL(ctx, comment.PostID))
	if missing || err != nil {
		return nil, err
	}

	actor, missing, err := gone(s.store.UserByID(ctx, comment.UserID))
	if missing || err != nil {
		return nil, err
	}

	recipient, missing, err := gone(s.store.UserByID(ctx, a.RecipientID))
	if missing || err != nil {
		return nil, err
	}

	mayRead, err := mayReadComments(ctx, s.store, recipient.ID, post)
	if !mayRead || err != nil {
		return nil, err
	}

	if recipient.ID == post.UserID {
		return mail.PostCommentAuthor(s.ident.Site, s.ident.From, s.ident.Site.MediaReplacer, actor, post.User, post, comment, a.Edited)
	}

	return mail.PostCommentParticipants(s.ident.Site, s.ident.From, s.ident.Site.MediaReplacer, actor, recipient, post, comment, a.Edited)
}

// maySee reports whether the user may read the post now.
func (s *Service) maySee(ctx context.Context, userID string, post *model.Post) (bool, error) {
	radius, err := graph.RadiusBetween(ctx, s.store, userID, post.UserID)
	if err != nil {
		return false, err
	}

	return postops.CanSeePost(post, radius), nil
}
