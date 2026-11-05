// Package subscriptions lets readers without an account get an author's new
// posts by email: subscribing with a confirmation mail, the page where a
// subscriber manages their subscriptions, the author's list of subscribers,
// and the mail sent when a post is published.
package subscriptions

import (
	"context"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"go.uber.org/zap"
)

// errNotImplemented is what a method returns until it is built.
var errNotImplemented = errors.New("subscriptions: not implemented")

// The settings' values when the configuration sets none; they are also the
// configuration's defaults.
const (
	// DefaultConfirmLifetime is how long a confirmation link works.
	DefaultConfirmLifetime = 72 * time.Hour
	// DefaultConfirmsPerAddress is how many confirmation mails one mailbox
	// gets.
	DefaultConfirmsPerAddress = 3
	// DefaultManageLinksPerAddress is how many manage-link mails one mailbox
	// gets.
	DefaultManageLinksPerAddress = 3
	// DefaultConfirmsPerHour is how many confirmation mails the whole site
	// sends per hour.
	DefaultConfirmsPerHour = 100
	// DefaultPruneEvery is how often RunPruner deletes expired rows.
	DefaultPruneEvery = time.Hour
)

// Limits are the service's settings, as Limits reports them.
type Limits struct {
	ConfirmLifetime       time.Duration
	ConfirmsPerAddress    int
	ManageLinksPerAddress int
	ConfirmsPerHour       int
	PruneEvery            time.Duration
}

// Service is email subscriptions to authors.
type Service struct {
	store   *repo.Store
	sender  repo.MailQueue
	ident   mail.Identity
	checker *emailaddr.Checker
	limits  Limits
	now     func() time.Time
	logger  *zap.Logger
}

// Option changes how New builds the service.
type Option func(*Service)

// WithConfirmLifetime sets how long a confirmation link works; zero keeps
// DefaultConfirmLifetime.
func WithConfirmLifetime(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.limits.ConfirmLifetime = d
		}
	}
}

// WithConfirmsPerAddress sets how many confirmation mails one mailbox gets;
// zero keeps DefaultConfirmsPerAddress.
func WithConfirmsPerAddress(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.limits.ConfirmsPerAddress = n
		}
	}
}

// WithManageLinksPerAddress sets how many manage-link mails one mailbox gets;
// zero keeps DefaultManageLinksPerAddress.
func WithManageLinksPerAddress(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.limits.ManageLinksPerAddress = n
		}
	}
}

// WithConfirmsPerHour sets how many confirmation mails the site sends per
// hour; zero keeps DefaultConfirmsPerHour.
func WithConfirmsPerHour(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.limits.ConfirmsPerHour = n
		}
	}
}

// WithPruneEvery sets how often RunPruner runs; zero keeps
// DefaultPruneEvery.
func WithPruneEvery(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.limits.PruneEvery = d
		}
	}
}

// WithEmailChecker sets what asks DNS whether an address's domain can receive
// mail; nil keeps emailaddr.NewChecker().
func WithEmailChecker(c *emailaddr.Checker) Option {
	return func(s *Service) {
		if c != nil {
			s.checker = c
		}
	}
}

// WithIdentity tells the service the site its links point to and the
// addresses its mail uses. Without it links are relative and mail has no sender.
func WithIdentity(ident mail.Identity) Option {
	return func(s *Service) { s.ident = ident }
}

// WithClock replaces the clock links and limits are timed by, for tests.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithLogger sets the logger of the background pruner (RunPruner); requests
// log through the logger their context carries. The default logs nothing.
func WithLogger(l *zap.Logger) Option {
	return func(s *Service) {
		if l != nil {
			s.logger = l
		}
	}
}

// New builds the service. sender may be nil when no mail is sent, as for the
// pruner.
func New(store *repo.Store, snd repo.MailQueue, opts ...Option) *Service {
	s := &Service{
		store:   store,
		sender:  snd,
		checker: emailaddr.NewChecker(),
		limits: Limits{
			ConfirmLifetime:       DefaultConfirmLifetime,
			ConfirmsPerAddress:    DefaultConfirmsPerAddress,
			ManageLinksPerAddress: DefaultManageLinksPerAddress,
			ConfirmsPerHour:       DefaultConfirmsPerHour,
			PruneEvery:            DefaultPruneEvery,
		},
		now:    time.Now,
		logger: zap.NewNop(),
	}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Limits reports the settings the service runs with.
func (s *Service) Limits() Limits {
	return s.limits
}

// ConfirmView is what the confirmation page shows before the subscriber
// confirms: whose posts they will get, and whether that author has paused
// subscriptions.
type ConfirmView struct {
	AuthorUsername string
	Paused         bool
}

// ManageView is a subscriber's subscriptions, for the page their manage link
// opens.
type ManageView struct {
	Email string
	Rows  []ManageRow
}

// ManageRow is one subscription on the manage page. Paused is set when the
// author no longer takes subscriptions, so no posts are mailed.
type ManageRow struct {
	ID             string
	AuthorUsername string
	Paused         bool
}

// AuthorView is an author's subscribers: how many are active, and each
// subscription with its address and when it was confirmed (nil while pending).
type AuthorView struct {
	Count int
	Rows  []AuthorRow
}

// AuthorRow is one subscriber in AuthorView.
type AuthorRow struct {
	ID          string
	Email       string
	ConfirmedAt *time.Time
}

// Subscribe starts a subscription of email to the author's posts and mails a
// confirmation link.
func (s *Service) Subscribe(ctx context.Context, authorUsername, email string) error {
	return errNotImplemented
}

// ConfirmView is what the confirmation link's page shows.
func (s *Service) ConfirmView(ctx context.Context, token string) (*ConfirmView, error) {
	return nil, errNotImplemented
}

// Confirm activates the subscription the token was mailed for.
func (s *Service) Confirm(ctx context.Context, token string) error {
	return errNotImplemented
}

// Manage is the subscriptions of the subscriber the manage token belongs to.
func (s *Service) Manage(ctx context.Context, manageToken string) (*ManageView, error) {
	return nil, errNotImplemented
}

// Unsubscribe ends one subscription of the manage token's subscriber.
func (s *Service) Unsubscribe(ctx context.Context, manageToken, subscriptionID string) error {
	return errNotImplemented
}

// UnsubscribeAll ends every subscription of the manage token's subscriber.
func (s *Service) UnsubscribeAll(ctx context.Context, manageToken string) error {
	return errNotImplemented
}

// RequestManageLink mails the address its manage link, if it subscribes to
// anyone.
func (s *Service) RequestManageLink(ctx context.Context, email string) error {
	return errNotImplemented
}

// Subscribable reports whether readers may subscribe to the author: their
// profile is public and they turned email subscriptions on.
func (s *Service) Subscribable(author *model.User) bool {
	return author != nil && author.ProfileVisibility == model.ProfileVisibilityPublic && author.EmailSubscriptionsEnabled
}

// ForAuthor is the actor's own subscribers.
func (s *Service) ForAuthor(ctx context.Context, actor *model.User) (*AuthorView, error) {
	return nil, errNotImplemented
}

// Remove ends a subscription to the actor's posts.
func (s *Service) Remove(ctx context.Context, actor *model.User, subscriptionID string) error {
	return errNotImplemented
}

// ActiveCount is how many confirmed subscribers the author has.
func (s *Service) ActiveCount(ctx context.Context, tx *repo.Store, authorID string) (int, error) {
	return 0, errNotImplemented
}

// NotifyPost queues the mail of a published post to the author's active
// subscribers, once per post, inside the caller's transaction.
func (s *Service) NotifyPost(ctx context.Context, tx *repo.Store, author *model.User, post *model.Post) error {
	return errNotImplemented
}

// RunPruner deletes expired pending subscriptions and old mail log rows every
// PruneEvery until ctx is done.
func (s *Service) RunPruner(ctx context.Context) {
	ctx = logging.WithLogger(ctx, s.logger)

	ticker := time.NewTicker(s.limits.PruneEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.prune(ctx); err != nil {
				logging.FromContext(ctx).Warn("failed to prune email subscriptions", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

// prune deletes the pending subscriptions whose link expired and the mail log
// rows no limit counts any more.
func (s *Service) prune(ctx context.Context) error {
	return errNotImplemented
}
