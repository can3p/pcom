// Package subscriptions lets readers without an account get an author's new
// posts by email: subscribing with a confirmation mail, the page where a
// subscriber manages their subscriptions, the author's list of subscribers,
// and the mail sent when a post is published.
package subscriptions

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// The messages of the service's validation errors, shown to the visitor as
// they are.
const (
	// InvalidEmailMessage refuses an address that isn't one.
	InvalidEmailMessage = "Invalid email"
	// DisposableMessage refuses a throwaway address.
	DisposableMessage = "Please use a permanent address"
	// PausedMessage is the answer of the subscribe and manage-link forms
	// while the site-wide brake is on.
	PausedMessage = "Subscriptions are paused, try again later"
)

// The windows the limits count mails in. They are part of what the limits
// mean, not settings: ConfirmsPerAddress and ManageLinksPerAddress are per
// day, ConfirmsPerHour per hour.
const (
	addressWindow = 24 * time.Hour
	brakeWindow   = time.Hour
)

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
// confirmation link. An author who can't be subscribed to is ErrNotFound. The
// answer is the same whatever the address's state: a new subscription, or a
// pending one whose link expired, gets a confirmation mail unless the
// mailbox had its share of them today; a pending one whose link still works
// and an active one get nothing.
func (s *Service) Subscribe(ctx context.Context, authorUsername, email string) error {
	author, err := notFound(s.store.UserByUsername(ctx, authorUsername))
	if err != nil {
		return err
	}

	if !s.Subscribable(author) {
		return service.ErrNotFound
	}

	addr, err := s.address(ctx, email)
	if err != nil {
		return err
	}

	if addr.Disposable {
		logging.FromContext(ctx).Info("refused a disposable address", zap.String("domain", domainOf(addr.Stored)))
		return service.Invalid("email", DisposableMessage)
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := tx.LockMailbox(ctx, repo.LockSubscribeMailbox, addr.Canonical); err != nil {
			return err
		}

		subscriber, err := orNil(tx.SubscriberByEmail(ctx, addr.Stored))
		if err != nil {
			return err
		}

		var sub *model.EmailSubscription

		if subscriber != nil {
			if subscriber, err = tx.LockSubscriber(ctx, subscriber.ID); err != nil {
				return err
			}

			if sub, err = tx.EmailSubscriptionOf(ctx, subscriber.ID, author.ID); err != nil {
				return err
			}
		}

		now := s.now()
		if sub != nil && (sub.Status == model.SubscriptionStatusActive || !s.expired(sub, now)) {
			return nil
		}

		if ok, err := s.underCap(ctx, tx, addr.Canonical, model.SubscriptionMailKindConfirm, s.limits.ConfirmsPerAddress, now); err != nil || !ok {
			return err
		}

		if err := s.checker.CheckDomain(ctx, domainOf(addr.Stored)); errors.Is(err, emailaddr.ErrCantReceiveMail) {
			return service.Invalid("email", emailaddr.UndeliverableMessage)
		} else if err != nil {
			return err
		}

		if subscriber == nil {
			subscriber = &model.EmailSubscriber{ID: uuid.NewString(), Email: addr.Stored, EmailCanonical: addr.Canonical, ManageToken: newToken()}
			if err := tx.InsertSubscriber(ctx, subscriber); err != nil {
				return err
			}
		}

		if sub == nil {
			sub = &model.EmailSubscription{
				ID: uuid.NewString(), SubscriberID: subscriber.ID, AuthorID: author.ID,
				Status: model.SubscriptionStatusPending, ConfirmToken: newToken(), ConfirmSentAt: now,
			}
			if err := tx.InsertEmailSubscription(ctx, sub); err != nil {
				return err
			}
		} else {
			sub.ConfirmToken, sub.ConfirmSentAt = newToken(), now
			if err := tx.SaveEmailSubscription(ctx, sub, "confirm_token", "confirm_sent_at"); err != nil {
				return err
			}
		}

		if err := tx.LogSubscriptionMail(ctx, &model.SubscriptionMailLog{EmailCanonical: addr.Canonical, Kind: model.SubscriptionMailKindConfirm, SentAt: now}); err != nil {
			return err
		}

		return s.send(ctx, tx, mail.SubscriberConfirm(s.ident.Site, s.ident.From, author, subscriber, sub))
	})
}

// ConfirmView is what the confirmation link's page shows. It changes
// nothing, since mail scanners open links. An unknown or expired token is
// ErrNotFound.
func (s *Service) ConfirmView(ctx context.Context, token string) (*ConfirmView, error) {
	sub, err := s.byConfirmToken(ctx, s.store, token)
	if err != nil {
		return nil, err
	}

	author, err := s.store.UserByID(ctx, sub.AuthorID)
	if err != nil {
		return nil, err
	}

	return &ConfirmView{AuthorUsername: author.Username, Paused: !s.Subscribable(author)}, nil
}

// Confirm activates the subscription the token was mailed for and tells the
// author. Confirming an active subscription again changes nothing; an
// unknown or expired token is ErrNotFound.
func (s *Service) Confirm(ctx context.Context, token string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		sub, err := s.byConfirmToken(ctx, tx, token)
		if err != nil {
			return err
		}

		subscriber, err := notFound(tx.LockSubscriber(ctx, sub.SubscriberID))
		if err != nil {
			return err
		}

		// Read again under the lock: a concurrent confirmation may have won.
		if sub, err = s.byConfirmToken(ctx, tx, token); err != nil || sub.Status == model.SubscriptionStatusActive {
			return err
		}

		author, err := tx.UserByID(ctx, sub.AuthorID)
		if err != nil {
			return err
		}

		sub.Status, sub.ConfirmedAt = model.SubscriptionStatusActive, new(s.now())
		if err := tx.SaveEmailSubscription(ctx, sub, "status", "confirmed_at"); err != nil {
			return err
		}

		return s.send(ctx, tx, mail.AuthorNewSubscriber(s.ident.Site, s.ident.From, author, subscriber, sub))
	})
}

// byConfirmToken is the subscription of a confirmation token, or ErrNotFound
// when there is none or its pending link expired.
func (s *Service) byConfirmToken(ctx context.Context, store *repo.Store, token string) (*model.EmailSubscription, error) {
	sub, err := notFound(store.EmailSubscriptionByConfirmToken(ctx, token))
	if err != nil {
		return nil, err
	}

	if sub.Status == model.SubscriptionStatusPending && s.expired(sub, s.now()) {
		return nil, service.ErrNotFound
	}

	return sub, nil
}

// Manage is the active subscriptions of the subscriber the manage token
// belongs to, by author; an unknown token is ErrNotFound.
func (s *Service) Manage(ctx context.Context, manageToken string) (*ManageView, error) {
	subscriber, err := notFound(s.store.SubscriberByManageToken(ctx, manageToken))
	if err != nil {
		return nil, err
	}

	subs, err := s.store.EmailSubscriptionsOfSubscriber(ctx, subscriber.ID, model.SubscriptionStatusActive)
	if err != nil {
		return nil, err
	}

	view := &ManageView{Email: subscriber.Email, Rows: make([]ManageRow, 0, len(subs))}
	for _, sub := range subs {
		view.Rows = append(view.Rows, ManageRow{ID: sub.ID, AuthorUsername: sub.Author.Username, Paused: !s.Subscribable(sub.Author)})
	}

	return view, nil
}

// Unsubscribe ends one subscription of the manage token's subscriber and
// tells the author. An unknown token, or a subscription of somebody else, is
// ErrNotFound.
func (s *Service) Unsubscribe(ctx context.Context, manageToken, subscriptionID string) error {
	return s.unsubscribe(ctx, manageToken, func(sub *model.EmailSubscription) bool { return sub.ID == subscriptionID })
}

// UnsubscribeAll ends every subscription of the manage token's subscriber,
// telling each author whose subscriber left.
func (s *Service) UnsubscribeAll(ctx context.Context, manageToken string) error {
	return s.unsubscribe(ctx, manageToken, func(*model.EmailSubscription) bool { return true })
}

// unsubscribe deletes the subscriber's subscriptions that match, mailing the
// author of each active one first, and the subscriber with the last one.
func (s *Service) unsubscribe(ctx context.Context, manageToken string, match func(*model.EmailSubscription) bool) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		subscriber, err := notFound(tx.SubscriberByManageToken(ctx, manageToken))
		if err != nil {
			return err
		}

		if subscriber, err = notFound(tx.LockSubscriber(ctx, subscriber.ID)); err != nil {
			return err
		}

		var subs []*model.EmailSubscription

		for _, status := range []model.SubscriptionStatus{model.SubscriptionStatusActive, model.SubscriptionStatusPending} {
			some, err := tx.EmailSubscriptionsOfSubscriber(ctx, subscriber.ID, status)
			if err != nil {
				return err
			}

			for _, sub := range some {
				if match(sub) {
					subs = append(subs, sub)
				}
			}
		}

		if len(subs) == 0 {
			return service.ErrNotFound
		}

		for _, sub := range subs {
			if sub.Status == model.SubscriptionStatusActive {
				if err := s.send(ctx, tx, mail.AuthorSubscriberLeft(s.ident.Site, s.ident.From, sub.Author, subscriber, sub)); err != nil {
					return err
				}
			}

			if err := tx.DeleteEmailSubscription(ctx, sub.ID); err != nil {
				return err
			}
		}

		return tx.DeleteSubscriberIfEmpty(ctx, subscriber.ID)
	})
}

// RequestManageLink mails the address its manage link, if it subscribes to
// anyone and the mailbox had fewer than ManageLinksPerAddress of them today.
// The answer is the same either way.
func (s *Service) RequestManageLink(ctx context.Context, email string) error {
	addr, err := s.address(ctx, email)
	if err != nil {
		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := tx.LockMailbox(ctx, repo.LockSubscribeMailbox, addr.Canonical); err != nil {
			return err
		}

		subscriber, err := orNil(tx.SubscriberByEmail(ctx, addr.Stored))
		if err != nil || subscriber == nil {
			return err
		}

		now := s.now()
		if ok, err := s.underCap(ctx, tx, addr.Canonical, model.SubscriptionMailKindManageLink, s.limits.ManageLinksPerAddress, now); err != nil || !ok {
			return err
		}

		if err := tx.LogSubscriptionMail(ctx, &model.SubscriptionMailLog{EmailCanonical: addr.Canonical, Kind: model.SubscriptionMailKindManageLink, SentAt: now}); err != nil {
			return err
		}

		return s.send(ctx, tx, mail.SubscriberManageLink(s.ident.Site, s.ident.From, subscriber, now.UTC().Format(time.RFC3339Nano)))
	})
}

// Subscribable reports whether readers may subscribe to the author: their
// profile is public and they turned email subscriptions on.
func (s *Service) Subscribable(author *model.User) bool {
	return author != nil && author.ProfileVisibility == model.ProfileVisibilityPublic && author.EmailSubscriptionsEnabled
}

// ForAuthor is the actor's own confirmed subscribers, the most recently
// confirmed first; pending subscriptions are neither counted nor listed.
func (s *Service) ForAuthor(ctx context.Context, actor *model.User) (*AuthorView, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	subs, err := s.store.EmailSubscriptionsOfAuthor(ctx, actor.ID, model.SubscriptionStatusActive)
	if err != nil {
		return nil, err
	}

	view := &AuthorView{Count: len(subs), Rows: make([]AuthorRow, 0, len(subs))}
	for _, sub := range subs {
		view.Rows = append(view.Rows, AuthorRow{ID: sub.ID, Email: sub.Subscriber.Email, ConfirmedAt: sub.ConfirmedAt})
	}

	return view, nil
}

// Remove ends a subscription to the actor's posts. Nobody is mailed about it.
// A subscription to somebody else is ErrNotFound.
func (s *Service) Remove(ctx context.Context, actor *model.User, subscriptionID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		sub, err := notFound(tx.EmailSubscriptionByID(ctx, subscriptionID))
		if err != nil {
			return err
		}

		if sub.AuthorID != actor.ID {
			return service.ErrNotFound
		}

		if _, err := notFound(tx.LockSubscriber(ctx, sub.SubscriberID)); err != nil {
			return err
		}

		if err := tx.DeleteEmailSubscription(ctx, sub.ID); err != nil {
			return err
		}

		return tx.DeleteSubscriberIfEmpty(ctx, sub.SubscriberID)
	})
}

// ActiveCount is how many confirmed subscribers the author has, read on tx
// (the service's store when nil).
func (s *Service) ActiveCount(ctx context.Context, tx *repo.Store, authorID string) (int, error) {
	return s.on(tx).EmailSubscriptionCount(ctx, authorID, model.SubscriptionStatusActive)
}

// NotifyPost queues the mail of a published post to the author's active
// subscribers, once per post, inside the caller's transaction (a transaction
// of its own when tx is nil), so the post is marked mailed only together with
// its mails. It does nothing for an author who can't be subscribed to, a post
// that isn't public and published, a post already mailed, or an author
// without subscribers.
func (s *Service) NotifyPost(ctx context.Context, tx *repo.Store, author *model.User, post *model.Post) error {
	if !s.Subscribable(author) || post.UserID != author.ID || post.VisibilityRadius != model.PostVisibilityPublic ||
		post.PublishedAt == nil || post.SubscribersMailedAt != nil {
		return nil
	}

	return s.on(tx).Tx(ctx, func(tx *repo.Store) error {
		subs, err := tx.EmailSubscriptionsOfAuthor(ctx, author.ID, model.SubscriptionStatusActive)
		if err != nil || len(subs) == 0 {
			return err
		}

		now := s.now()
		if marked, err := tx.MarkPostSubscribersMailed(ctx, post.ID, now); err != nil || !marked {
			return err
		}

		for _, sub := range subs {
			if err := s.send(ctx, tx, mail.SubscriberNewPost(s.ident.Site, s.ident.From, s.ident.Site.MediaReplacer, author, sub.Subscriber, sub, post)); err != nil {
				return err
			}
		}

		post.SubscribersMailedAt = &now

		return nil
	})
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
			if err := s.Prune(ctx); err != nil {
				logging.FromContext(ctx).Warn("failed to prune email subscriptions", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

// Prune deletes the pending subscriptions whose link expired, the
// subscribers left without a subscription, and the mail log rows no limit
// counts any more.
func (s *Service) Prune(ctx context.Context) error {
	now := s.now()

	if _, err := s.store.DeletePendingEmailSubscriptionsSentBefore(ctx, now.Add(-s.limits.ConfirmLifetime)); err != nil {
		return err
	}

	if _, err := s.store.DeleteSubscribersWithoutSubscriptions(ctx); err != nil {
		return err
	}

	_, err := s.store.DeleteSubscriptionMailsBefore(ctx, now.Add(-addressWindow))

	return err
}

// address checks what both forms check before they touch a mailbox: the
// address's syntax and the site-wide brake. Past ConfirmsPerHour
// confirmation mails in the last hour the forms are paused, and the admin is
// told once per hour.
func (s *Service) address(ctx context.Context, email string) (emailaddr.Address, error) {
	if !validation.EmailRE.MatchString(emailaddr.Normalize(email)) {
		return emailaddr.Address{}, service.Invalid("email", InvalidEmailMessage)
	}

	now := s.now()

	sent, err := s.store.SubscriptionMailsSince(ctx, model.SubscriptionMailKindConfirm, now.Add(-brakeWindow))
	if err != nil {
		return emailaddr.Address{}, err
	}

	if sent >= s.limits.ConfirmsPerHour {
		if err := s.send(ctx, s.store, mail.AdminSubscriptionsBraked(s.ident.Site, s.ident.From, s.ident.AdminAddress, now)); err != nil {
			return emailaddr.Address{}, err
		}

		return emailaddr.Address{}, service.Invalid("", PausedMessage)
	}

	return emailaddr.Parse(email), nil
}

// underCap reports whether the mailbox got fewer than limit mails of kind in
// the last day.
func (s *Service) underCap(ctx context.Context, tx *repo.Store, canonical string, kind model.SubscriptionMailKind, limit int, now time.Time) (bool, error) {
	sent, err := tx.SubscriptionMailsTo(ctx, canonical, kind, now.Add(-addressWindow))

	return sent < limit, err
}

// expired reports whether a pending subscription's confirmation link no
// longer works at now.
func (s *Service) expired(sub *model.EmailSubscription, now time.Time) bool {
	return !now.Before(sub.ConfirmSentAt.Add(s.limits.ConfirmLifetime))
}

// send queues a mail on store, so inside Tx it goes out only if the change
// commits.
func (s *Service) send(ctx context.Context, store *repo.Store, e *mail.Envelope) error {
	return store.SendMail(ctx, s.sender, e.UniqueID, e.Type, e.Mail)
}

// on is tx, or the service's store when tx is nil.
func (s *Service) on(tx *repo.Store) *repo.Store {
	if tx == nil {
		return s.store
	}

	return tx
}

// newToken is 32 random bytes, base64url: a confirmation or manage token.
func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)

	return base64.RawURLEncoding.EncodeToString(b)
}

// domainOf is the part of an address after its last @.
func domainOf(address string) string {
	return address[strings.LastIndex(address, "@")+1:]
}

func notFound[T any](v T, err error) (T, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return v, service.ErrNotFound
	}

	return v, err
}

// orNil turns repo.ErrNotFound into a nil result without an error.
func orNil[T any](v *T, err error) (*T, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}

	return v, err
}
