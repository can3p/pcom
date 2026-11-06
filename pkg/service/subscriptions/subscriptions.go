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
// day, ConfirmsPerHour and DomainLookupsPerHour per hour.
const (
	addressWindow = 24 * time.Hour
	brakeWindow   = time.Hour
	lookupWindow  = time.Hour
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
	// DefaultDomainLookupsPerHour is how many DNS lookups of subscribing
	// addresses' domains the whole site makes per hour.
	DefaultDomainLookupsPerHour = 300
	// DefaultPruneEvery is how often RunPruner deletes expired rows.
	DefaultPruneEvery = time.Hour
)

// Limits are the service's settings, as Limits reports them.
type Limits struct {
	ConfirmLifetime       time.Duration
	ConfirmsPerAddress    int
	ManageLinksPerAddress int
	ConfirmsPerHour       int
	DomainLookupsPerHour  int
	PruneEvery            time.Duration
}

// Service is email subscriptions to authors.
type Service struct {
	store   *repo.Store
	jobs    repo.JobQueue
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

// WithDomainLookupsPerHour sets how many DNS lookups of subscribing
// addresses' domains the site makes per hour; zero keeps
// DefaultDomainLookupsPerHour.
func WithDomainLookupsPerHour(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.limits.DomainLookupsPerHour = n
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

// New builds the service. Its mail is enqueued as jobs on the queue WithJobs
// sets; the pruner needs none.
func New(store *repo.Store, opts ...Option) *Service {
	s := &Service{
		store:   store,
		checker: emailaddr.NewChecker(),
		limits: Limits{
			ConfirmLifetime:       DefaultConfirmLifetime,
			ConfirmsPerAddress:    DefaultConfirmsPerAddress,
			ManageLinksPerAddress: DefaultManageLinksPerAddress,
			ConfirmsPerHour:       DefaultConfirmsPerHour,
			DomainLookupsPerHour:  DefaultDomainLookupsPerHour,
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
	// Email is the address the subscription is for.
	Email  string
	Paused bool
}

// ManageView is a subscriber's subscriptions, for the page their manage link
// opens.
type ManageView struct {
	Email string
	Rows  []ManageRow
}

// ManageRow is one subscription on the manage page. Paused is set when the
// author no longer takes subscriptions, so no posts are mailed. Since is when
// the subscription was confirmed.
type ManageRow struct {
	ID             string
	AuthorUsername string
	Since          time.Time
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
// and an active one get nothing. Only an address that would get a mail has
// its domain checked (checkDomain), and one whose domain can't receive mail
// is refused.
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

	if addr.Disposable && !s.checker.AllowsDisposable() {
		logging.FromContext(ctx).Info("refused a disposable address", zap.String("domain", domainOf(addr.Stored)))
		return service.Invalid("email", DisposableMessage)
	}

	now := s.now()

	// The DNS lookup runs outside the transaction, so it holds neither the
	// mailbox lock nor a connection: a read without locks tells whether a
	// mail is due, and the transaction asks again under the lock.
	st, err := s.subscribeState(ctx, s.store, author, addr, now, false)
	if err != nil || !st.due {
		return err
	}

	if err := s.checkDomain(ctx, addr, now); err != nil {
		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := tx.LockMailbox(ctx, repo.LockSubscribeMailbox, addr.Canonical); err != nil {
			return err
		}

		// A mail that stopped being due since the read, as a concurrent
		// request sent it, isn't sent again.
		st, err := s.subscribeState(ctx, tx, author, addr, now, true)
		if err != nil || !st.due {
			return err
		}

		subscriber, sub := st.subscriber, st.sub
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

		return tx.Enqueue(ctx, s.jobs, ConfirmMailArgs{SubscriptionID: sub.ID})
	})
}

// subscribeState is an address's subscriber and subscription to the author,
// either nil when there is none, and whether a confirmation mail is due.
type subscribeState struct {
	subscriber *model.EmailSubscriber
	sub        *model.EmailSubscription
	due        bool
}

// subscribeState reads the address's state on store: a confirmation mail is
// due for a new subscription or a pending one whose link expired, unless the
// mailbox had its share of them today. lock locks the subscriber's row, inside
// a transaction.
func (s *Service) subscribeState(ctx context.Context, store *repo.Store, author *model.User, addr emailaddr.Address, now time.Time, lock bool) (subscribeState, error) {
	var st subscribeState

	subscriber, err := orNil(store.SubscriberByEmail(ctx, addr.Stored))
	if err != nil {
		return st, err
	}

	if subscriber != nil {
		if lock {
			if subscriber, err = store.LockSubscriber(ctx, subscriber.ID); err != nil {
				return st, err
			}
		}

		if st.sub, err = store.EmailSubscriptionOf(ctx, subscriber.ID, author.ID); err != nil {
			return st, err
		}
	}

	st.subscriber = subscriber

	if st.sub != nil && (st.sub.Status == model.SubscriptionStatusActive || !s.expired(st.sub, now)) {
		return st, nil
	}

	st.due, err = s.underCap(ctx, store, addr.Canonical, model.SubscriptionMailKindConfirm, s.limits.ConfirmsPerAddress, now)

	return st, err
}

// checkDomain refuses an address whose domain can't receive mail. A DNS
// lookup it makes is logged as a domain_lookup row, which commits whatever
// the request returns; past DomainLookupsPerHour of them in the last hour the
// check is skipped and the address goes through, as when DNS times out. A
// domain the checker remembers as accepted costs no lookup and no budget.
func (s *Service) checkDomain(ctx context.Context, addr emailaddr.Address, now time.Time) error {
	domain := domainOf(addr.Stored)
	if !s.checker.NeedsLookup(domain) {
		return nil
	}

	lookups, err := s.store.SubscriptionMailsSince(ctx, now.Add(-lookupWindow), model.SubscriptionMailKindDomainLookup)
	if err != nil {
		return err
	}

	if lookups >= s.limits.DomainLookupsPerHour {
		logging.FromContext(ctx).Info("domain lookup budget spent, letting the address through", zap.String("domain", domain))
		return nil
	}

	looked, err := s.checker.CheckDomain(ctx, domain)
	if looked {
		row := &model.SubscriptionMailLog{EmailCanonical: addr.Canonical, Kind: model.SubscriptionMailKindDomainLookup, SentAt: now}
		if lerr := s.store.LogSubscriptionMail(context.WithoutCancel(ctx), row); lerr != nil {
			return lerr
		}
	}

	if errors.Is(err, emailaddr.ErrCantReceiveMail) {
		return service.Invalid("email", emailaddr.UndeliverableMessage)
	}

	return err
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

	return &ConfirmView{AuthorUsername: author.Username, Email: sub.Subscriber.Email, Paused: !s.Subscribable(author)}, nil
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

		if _, err := notFound(tx.LockSubscriber(ctx, sub.SubscriberID)); err != nil {
			return err
		}

		// Read again under the lock: a concurrent confirmation may have won.
		if sub, err = s.byConfirmToken(ctx, tx, token); err != nil || sub.Status == model.SubscriptionStatusActive {
			return err
		}

		sub.Status, sub.ConfirmedAt = model.SubscriptionStatusActive, new(s.now())
		if err := tx.SaveEmailSubscription(ctx, sub, "status", "confirmed_at"); err != nil {
			return err
		}

		return tx.Enqueue(ctx, s.jobs, NewSubscriberMailArgs{SubscriptionID: sub.ID})
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
		since := sub.CreatedAt
		if sub.ConfirmedAt != nil {
			since = *sub.ConfirmedAt
		}

		view.Rows = append(view.Rows, ManageRow{ID: sub.ID, AuthorUsername: sub.Author.Username, Since: since, Paused: !s.Subscribable(sub.Author)})
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
				left := SubscriberLeftMailArgs{SubscriptionID: sub.ID, AuthorID: sub.AuthorID, SubscriberEmail: subscriber.Email}
				if err := tx.Enqueue(ctx, s.jobs, left); err != nil {
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

		return tx.Enqueue(ctx, s.jobs, ManageLinkMailArgs{SubscriberID: subscriber.ID, RequestID: now.UTC().Format(time.RFC3339Nano)})
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

// NotifyPost marks a published post mailed to the author's subscribers and
// enqueues the job that mails them (PostPublishedArgs), once per post, inside
// the caller's transaction (a transaction of its own when tx is nil), so the
// post is marked only together with its job. It does nothing for an author
// who can't be subscribed to, a post that isn't public and published, a post
// already mailed, or an author without subscribers. The job picks the
// subscribers as they are when it runs.
func (s *Service) NotifyPost(ctx context.Context, tx *repo.Store, author *model.User, post *model.Post) error {
	if !s.Subscribable(author) || post.UserID != author.ID || post.VisibilityRadius != model.PostVisibilityPublic ||
		post.PublishedAt == nil || post.SubscribersMailedAt != nil {
		return nil
	}

	return s.on(tx).Tx(ctx, func(tx *repo.Store) error {
		active, err := tx.EmailSubscriptionCount(ctx, author.ID, model.SubscriptionStatusActive)
		if err != nil || active == 0 {
			return err
		}

		now := s.now()
		if marked, err := tx.MarkPostSubscribersMailed(ctx, post.ID, now); err != nil || !marked {
			return err
		}

		if err := tx.Enqueue(ctx, s.jobs, PostPublishedArgs{PostID: post.ID}); err != nil {
			return err
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
// counts any more, whatever their kind.
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
// told once per hour: the alert's job is unique by hour.
func (s *Service) address(ctx context.Context, email string) (emailaddr.Address, error) {
	if !validation.EmailRE.MatchString(emailaddr.Normalize(email)) {
		return emailaddr.Address{}, service.Invalid("email", InvalidEmailMessage)
	}

	now := s.now()

	sent, err := s.store.SubscriptionMailsSince(ctx, now.Add(-brakeWindow), model.SubscriptionMailKindConfirm)
	if err != nil {
		return emailaddr.Address{}, err
	}

	if sent >= s.limits.ConfirmsPerHour {
		if err := s.store.Enqueue(ctx, s.jobs, BrakedMailArgs{Hour: now.UTC().Truncate(time.Hour)}); err != nil {
			return emailaddr.Address{}, err
		}

		return emailaddr.Address{}, service.Invalid("", PausedMessage)
	}

	return emailaddr.Parse(email), nil
}

// underCap reports whether the mailbox got fewer than limit mails of kind in
// the last day.
func (s *Service) underCap(ctx context.Context, store *repo.Store, canonical string, kind model.SubscriptionMailKind, limit int, now time.Time) (bool, error) {
	sent, err := store.SubscriptionMailsTo(ctx, canonical, kind, now.Add(-addressWindow))

	return sent < limit, err
}

// expired reports whether a pending subscription's confirmation link no
// longer works at now.
func (s *Service) expired(sub *model.EmailSubscription, now time.Time) bool {
	return !now.Before(sub.ConfirmSentAt.Add(s.limits.ConfirmLifetime))
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
