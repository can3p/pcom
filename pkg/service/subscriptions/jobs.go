package subscriptions

import (
	"context"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
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
	jobs.Register(r, s.FanOutPost)
	jobs.RegisterMail(r, s.RenderConfirm)
	jobs.RegisterMail(r, s.RenderManageLink)
	jobs.RegisterMail(r, s.RenderNewPost)
	jobs.RegisterMail(r, s.RenderNewSubscriber)
	jobs.RegisterMail(r, s.RenderSubscriberLeft)
	jobs.RegisterMail(r, s.RenderBraked)
}

// PostPublishedArgs is the job NotifyPost enqueues: its worker mails the post
// to the author's active subscribers. It is unique by post.
type PostPublishedArgs struct {
	PostID string `json:"post_id"`
}

// Kind names the job kind.
func (PostPublishedArgs) Kind() string { return "subscriptions.post_published" }

// InsertOpts makes the job unique by post.
func (PostPublishedArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// ConfirmMailArgs is the mail that asks a reader to confirm a pending
// subscription.
type ConfirmMailArgs struct {
	SubscriptionID string `json:"subscription_id"`
}

// Kind names the job kind.
func (ConfirmMailArgs) Kind() string { return "mail.subscription_confirm" }

// ManageLinkMailArgs is the mail with a subscriber's manage link. RequestID
// tells the requests apart, so each one is mailed.
type ManageLinkMailArgs struct {
	SubscriberID string `json:"subscriber_id"`
	RequestID    string `json:"request_id"`
}

// Kind names the job kind.
func (ManageLinkMailArgs) Kind() string { return "mail.subscription_manage_link" }

// NewPostMailArgs is the mail of a post to one subscriber of its author.
type NewPostMailArgs struct {
	PostID         string `json:"post_id"`
	SubscriptionID string `json:"subscription_id"`
}

// Kind names the job kind.
func (NewPostMailArgs) Kind() string { return "mail.subscription_post" }

// NewSubscriberMailArgs tells an author that a subscription was confirmed.
type NewSubscriberMailArgs struct {
	SubscriptionID string `json:"subscription_id"`
}

// Kind names the job kind.
func (NewSubscriberMailArgs) Kind() string { return "mail.subscription_author_new" }

// SubscriberLeftMailArgs tells an author that a subscriber unsubscribed. The
// subscription is deleted with the change that enqueues it, so the args
// carry the address the mail names.
type SubscriberLeftMailArgs struct {
	SubscriptionID  string `json:"subscription_id"`
	AuthorID        string `json:"author_id"`
	SubscriberEmail string `json:"subscriber_email"`
}

// Kind names the job kind.
func (SubscriberLeftMailArgs) Kind() string { return "mail.subscription_author_left" }

// BrakedMailArgs tells the admin that the site-wide brake is on. It is unique
// by hour, as the mail is.
type BrakedMailArgs struct {
	// Hour is the start of the UTC hour the brake was hit in.
	Hour time.Time `json:"hour"`
}

// Kind names the job kind.
func (BrakedMailArgs) Kind() string { return "mail.admin_subscriptions_braked" }

// InsertOpts makes the job unique by hour.
func (BrakedMailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// gone turns a row that no longer exists into "nothing to send".
func gone[T any](v T, err error) (T, bool, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return v, true, nil
	}

	return v, false, err
}

// FanOutPost enqueues the post's mail to each active subscriber of its
// author, in one transaction, if the post is still published and public and
// its author can still be subscribed to. A mail already sent is dropped by
// its unique id, so running the job again sends nothing twice.
func (s *Service) FanOutPost(ctx context.Context, a PostPublishedArgs) error {
	post, missing, err := gone(s.store.PostByID(ctx, a.PostID))
	if missing || err != nil {
		return err
	}

	author, missing, err := gone(s.store.UserByID(ctx, post.UserID))
	if missing || err != nil {
		return err
	}

	if !s.mailable(author, post) {
		return nil
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		subs, err := tx.EmailSubscriptionsOfAuthor(ctx, author.ID, model.SubscriptionStatusActive)
		if err != nil {
			return err
		}

		for _, sub := range subs {
			if err := tx.Enqueue(ctx, s.jobs, NewPostMailArgs{PostID: post.ID, SubscriptionID: sub.ID}); err != nil {
				return err
			}
		}

		return nil
	})
}

// mailable reports whether the post may be mailed to the author's
// subscribers: it is the author's, published and public, and the author can
// be subscribed to.
func (s *Service) mailable(author *model.User, post *model.Post) bool {
	return s.Subscribable(author) && post.UserID == author.ID &&
		post.VisibilityRadius == model.PostVisibilityPublic && post.PublishedAt != nil
}

// RenderConfirm builds the confirmation mail of a subscription still pending,
// with its link still valid, to an author who can still be subscribed to.
func (s *Service) RenderConfirm(ctx context.Context, a ConfirmMailArgs) (*mail.Envelope, error) {
	sub, missing, err := gone(s.store.EmailSubscriptionByID(ctx, a.SubscriptionID))
	if missing || err != nil {
		return nil, err
	}

	if sub.Status != model.SubscriptionStatusPending || s.expired(sub, s.now()) {
		return nil, nil
	}

	subscriber, missing, err := gone(s.store.SubscriberByID(ctx, sub.SubscriberID))
	if missing || err != nil {
		return nil, err
	}

	author, missing, err := gone(s.store.UserByID(ctx, sub.AuthorID))
	if missing || err != nil || !s.Subscribable(author) {
		return nil, err
	}

	return mail.SubscriberConfirm(s.ident.Site, s.ident.From, author, subscriber, sub), nil
}

// RenderManageLink builds the mail with the manage link of a subscriber who
// still has subscriptions.
func (s *Service) RenderManageLink(ctx context.Context, a ManageLinkMailArgs) (*mail.Envelope, error) {
	subscriber, missing, err := gone(s.store.SubscriberByID(ctx, a.SubscriberID))
	if missing || err != nil {
		return nil, err
	}

	return mail.SubscriberManageLink(s.ident.Site, s.ident.From, subscriber, a.RequestID), nil
}

// RenderNewPost builds the mail of a post to a subscriber, while the
// subscription is active and the post may still be mailed.
func (s *Service) RenderNewPost(ctx context.Context, a NewPostMailArgs) (*mail.Envelope, error) {
	sub, missing, err := gone(s.store.EmailSubscriptionByID(ctx, a.SubscriptionID))
	if missing || err != nil || sub.Status != model.SubscriptionStatusActive {
		return nil, err
	}

	post, missing, err := gone(s.store.PostWithAuthorAndURL(ctx, a.PostID))
	if missing || err != nil {
		return nil, err
	}

	if post.User == nil || post.UserID != sub.AuthorID || !s.mailable(post.User, post) {
		return nil, nil
	}

	subscriber, missing, err := gone(s.store.SubscriberByID(ctx, sub.SubscriberID))
	if missing || err != nil {
		return nil, err
	}

	return mail.SubscriberNewPost(s.ident.Site, s.ident.From, s.ident.Site.MediaReplacer, post.User, subscriber, sub, post), nil
}

// RenderNewSubscriber builds the mail that tells the author about a
// subscription confirmed and still active.
func (s *Service) RenderNewSubscriber(ctx context.Context, a NewSubscriberMailArgs) (*mail.Envelope, error) {
	sub, missing, err := gone(s.store.EmailSubscriptionByID(ctx, a.SubscriptionID))
	if missing || err != nil || sub.Status != model.SubscriptionStatusActive {
		return nil, err
	}

	subscriber, missing, err := gone(s.store.SubscriberByID(ctx, sub.SubscriberID))
	if missing || err != nil {
		return nil, err
	}

	author, missing, err := gone(s.store.UserByID(ctx, sub.AuthorID))
	if missing || err != nil {
		return nil, err
	}

	return mail.AuthorNewSubscriber(s.ident.Site, s.ident.From, author, subscriber, sub), nil
}

// RenderSubscriberLeft builds the mail that tells the author an address
// unsubscribed, unless the author's account is gone.
func (s *Service) RenderSubscriberLeft(ctx context.Context, a SubscriberLeftMailArgs) (*mail.Envelope, error) {
	author, missing, err := gone(s.store.UserByID(ctx, a.AuthorID))
	if missing || err != nil {
		return nil, err
	}

	return mail.AuthorSubscriberLeft(s.ident.Site, s.ident.From, author,
		&model.EmailSubscriber{Email: a.SubscriberEmail}, &model.EmailSubscription{ID: a.SubscriptionID}), nil
}

// RenderBraked builds the admin's alert that subscriptions are braked.
func (s *Service) RenderBraked(_ context.Context, a BrakedMailArgs) (*mail.Envelope, error) {
	return mail.AdminSubscriptionsBraked(s.ident.Site, s.ident.From, s.ident.AdminAddress, a.Hour), nil
}
