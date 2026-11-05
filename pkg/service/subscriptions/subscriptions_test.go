package subscriptions_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	confirmType = "subscription_confirm"
	manageType  = "subscription_manage_link"
	postType    = "subscription_post"
	newSubType  = "subscription_author_new"
	leftType    = "subscription_author_left"
	brakedType  = "admin_subscriptions_braked"
	adminAddr   = "admin@pcom.test"
	noMailHost  = "nomail.test"
)

// clock is a settable time for one service.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}

// resolver answers that every domain has an MX, except noMailHost, which
// doesn't exist.
type resolver struct{}

func (resolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if strings.TrimSuffix(name, ".") == noMailHost {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}

	return []*net.MX{{Host: "mx.pcom.test.", Pref: 10}}, nil
}

func (resolver) LookupIPAddr(_ context.Context, name string) ([]net.IPAddr, error) {
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

// newSvc is the service over the real mail queue and the fake resolver,
// timed by a clock it returns.
func newSvc(db *sql.DB, opts ...subscriptions.Option) (*subscriptions.Service, *clock) {
	c := &clock{t: time.Now().UTC().Truncate(time.Second)}
	queue := dbsender.NewSender(repo.New(db), fakesender.New().Delivery())
	base := []subscriptions.Option{
		subscriptions.WithClock(c.now),
		subscriptions.WithEmailChecker(emailaddr.NewChecker(emailaddr.WithResolver(resolver{}))),
		subscriptions.WithIdentity(mail.Identity{Site: mail.SampleSite, From: "pcom@pcom.test", AdminAddress: adminAddr}),
	}

	return subscriptions.New(repo.New(db), queue, append(base, opts...)...), c
}

type queued struct{ to, uniqueID string }

// mails returns the queued mails of emailType.
func mails(t *testing.T, db *sql.DB, emailType string) []queued {
	t.Helper()

	var out []queued
	for _, e := range testutil.Must(factory.ListOutgoingEmails(context.Background(), db, factory.EmailType(emailType)))(t) {
		var m sender.Mail
		require.NoError(t, json.Unmarshal(e.Payload, &m))
		out = append(out, queued{to: m.To[0].Address, uniqueID: e.UniqueID})
	}

	return out
}

func recipients(qs []queued) []string {
	out := make([]string, 0, len(qs))
	for _, q := range qs {
		out = append(out, q.to)
	}

	return out
}

// mailsTo counts the queued mails of emailType to the address.
func mailsTo(t *testing.T, db *sql.DB, emailType, to string) int {
	t.Helper()

	n := 0
	for _, m := range mails(t, db, emailType) {
		if m.to == to {
			n++
		}
	}

	return n
}

// subscriptionsOf returns the address's subscriptions, none when it has no
// subscriber row.
func subscriptionsOf(t *testing.T, db *sql.DB, email string) []*model.EmailSubscription {
	t.Helper()

	ctx := context.Background()

	subscriber, err := factory.GetEmailSubscriber(ctx, db, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	require.NoError(t, err)

	return testutil.Must(factory.ListEmailSubscriptions(ctx, db, subscriber.ID))(t)
}

func TestSubscribable(t *testing.T) {
	t.Parallel()

	svc := subscriptions.New(nil, nil)

	for _, r := range []struct {
		name       string
		visibility model.ProfileVisibility
		enabled    bool
		want       bool
	}{
		{"public and enabled", model.ProfileVisibilityPublic, true, true},
		{"public, not enabled", model.ProfileVisibilityPublic, false, false},
		{"registered users only", model.ProfileVisibilityRegisteredUsers, true, false},
		{"connections only", model.ProfileVisibilityConnections, true, false},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			author := &model.User{ProfileVisibility: r.visibility, EmailSubscriptionsEnabled: r.enabled}
			require.Equal(t, r.want, svc.Subscribable(author))
		})
	}

	require.False(t, svc.Subscribable(nil))
}

func TestSubscribe(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	// logged counts the confirmation mails the limits count for the address.
	logged := func(t *testing.T, email string) int {
		t.Helper()

		return testutil.Must(repo.New(db).SubscriptionMailsTo(ctx, emailaddr.Parse(email).Canonical, model.SubscriptionMailKindConfirm, time.Time{}))(t)
	}

	for _, r := range []struct {
		name   string
		paused bool
		email  string
		// before runs ahead of the subscription the row checks.
		before       func(t *testing.T, svc *subscriptions.Service, c *clock, author *model.User, email string)
		wantErr      error
		wantConfirms int
		wantLogged   int
		wantStatus   model.SubscriptionStatus
		// sameToken is whether a subscription made in before keeps its link.
		sameToken bool
	}{
		{name: "new", email: "new@mail.test", wantConfirms: 1, wantLogged: 1, wantStatus: model.SubscriptionStatusPending},
		{
			name: "again while pending", email: "pending@mail.test",
			before: func(t *testing.T, svc *subscriptions.Service, c *clock, author *model.User, email string) {
				require.NoError(t, svc.Subscribe(ctx, author.Username, email))
				c.advance(subscriptions.DefaultConfirmLifetime - time.Minute)
			},
			wantConfirms: 1, wantLogged: 1, wantStatus: model.SubscriptionStatusPending, sameToken: true,
		},
		{
			name: "after expiry", email: "expired@mail.test",
			before: func(t *testing.T, svc *subscriptions.Service, c *clock, author *model.User, email string) {
				require.NoError(t, svc.Subscribe(ctx, author.Username, email))
				c.advance(subscriptions.DefaultConfirmLifetime)
			},
			wantConfirms: 2, wantLogged: 2, wantStatus: model.SubscriptionStatusPending,
		},
		{
			name: "while active", email: "active@mail.test",
			before: func(t *testing.T, _ *subscriptions.Service, _ *clock, author *model.User, email string) {
				testutil.Must(factory.EmailSubscription(ctx, db, author.ID, email))(t)
			},
			wantStatus: model.SubscriptionStatusActive, sameToken: true,
		},
		{name: "to a paused author", paused: true, email: "paused@mail.test", wantErr: service.ErrNotFound},
		{
			name: "over the cap of the mailbox", email: "capped+4@mail.test",
			before: func(t *testing.T, svc *subscriptions.Service, _ *clock, _ *model.User, _ string) {
				for i := range subscriptions.DefaultConfirmsPerAddress {
					other := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
					require.NoError(t, svc.Subscribe(ctx, other.Username, fmt.Sprintf("capped+%d@mail.test", i)))
				}
			},
			wantLogged: subscriptions.DefaultConfirmsPerAddress,
		},
		{
			name: "over the cap, on a domain without mail", email: "capped@" + noMailHost,
			before: func(t *testing.T, _ *subscriptions.Service, c *clock, _ *model.User, email string) {
				for range subscriptions.DefaultConfirmsPerAddress {
					require.NoError(t, repo.New(db).LogSubscriptionMail(ctx, &model.SubscriptionMailLog{
						EmailCanonical: email, Kind: model.SubscriptionMailKindConfirm, SentAt: c.now(),
					}))
				}
			},
			wantLogged: subscriptions.DefaultConfirmsPerAddress,
		},
		{name: "invalid address", email: "not an address", wantErr: service.Invalid("email", subscriptions.InvalidEmailMessage)},
		{name: "disposable address", email: "x@mailinator.com", wantErr: service.Invalid("email", subscriptions.DisposableMessage)},
		{name: "disposable subdomain", email: "x@foo.mailinator.com", wantErr: service.Invalid("email", subscriptions.DisposableMessage)},
		{name: "relay", email: "reader@duck.com", wantConfirms: 1, wantLogged: 1, wantStatus: model.SubscriptionStatusPending},
		{name: "domain without mail", email: "x@" + noMailHost, wantErr: service.Invalid("email", emailaddr.UndeliverableMessage)},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			opts := []factory.UserOpt{factory.Subscribable()}
			if r.paused {
				opts = append(opts, factory.WithVisibility(model.ProfileVisibilityRegisteredUsers))
			}

			author := testutil.Must(factory.User(ctx, db, opts...))(t)
			svc, c := newSvc(db)

			if r.before != nil {
				r.before(t, svc, c, author, r.email)
			}

			prev := subscriptionsOf(t, db, r.email)

			require.Equal(t, r.wantErr, svc.Subscribe(ctx, author.Username, r.email))
			require.Equal(t, r.wantConfirms, mailsTo(t, db, confirmType, r.email))
			require.Equal(t, r.wantLogged, logged(t, r.email))

			subs := subscriptionsOf(t, db, r.email)
			if r.wantStatus == "" {
				require.Empty(t, subs)
				return
			}

			require.Len(t, subs, 1)
			require.Equal(t, r.wantStatus, subs[0].Status)
			require.Equal(t, author.ID, subs[0].AuthorID)

			if len(prev) == 1 {
				same := prev[0].ConfirmToken == subs[0].ConfirmToken && prev[0].ConfirmSentAt.Equal(subs[0].ConfirmSentAt)
				require.Equal(t, r.sameToken, same)
			}
		})
	}
}

func TestSubscribe_Brake(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	svc, c := newSvc(db, subscriptions.WithConfirmsPerHour(1))
	paused := service.Invalid("", subscriptions.PausedMessage)

	require.NoError(t, svc.Subscribe(ctx, author.Username, "first@mail.test"))
	require.Equal(t, paused, svc.Subscribe(ctx, author.Username, "second@mail.test"))
	require.Equal(t, paused, svc.RequestManageLink(ctx, "first@mail.test"))
	require.Empty(t, subscriptionsOf(t, db, "second@mail.test"))
	require.Equal(t, paused, svc.Subscribe(ctx, author.Username, "second@mail.test"))
	require.Equal(t, 1, mailsTo(t, db, brakedType, adminAddr), "one alert per hour")

	c.advance(time.Hour + time.Second)
	require.NoError(t, svc.Subscribe(ctx, author.Username, "second@mail.test"))
	require.Equal(t, 1, mailsTo(t, db, confirmType, "second@mail.test"))
}

func TestConfirm(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	svc, c := newSvc(db)

	require.NoError(t, svc.Subscribe(ctx, author.Username, "reader@mail.test"))
	require.NoError(t, svc.Subscribe(ctx, author.Username, "late@mail.test"))

	require.Zero(t, mailsTo(t, db, newSubType, author.Email), "the author hears of a subscriber only once confirmed")

	token := subscriptionsOf(t, db, "reader@mail.test")[0].ConfirmToken
	view, err := svc.ConfirmView(ctx, token)
	require.NoError(t, err)
	require.Equal(t, &subscriptions.ConfirmView{AuthorUsername: author.Username}, view)
	require.Equal(t, model.SubscriptionStatusPending, subscriptionsOf(t, db, "reader@mail.test")[0].Status)

	require.NoError(t, svc.Confirm(ctx, token))
	confirmed := subscriptionsOf(t, db, "reader@mail.test")[0]
	require.Equal(t, model.SubscriptionStatusActive, confirmed.Status)
	require.NotNil(t, confirmed.ConfirmedAt)

	c.advance(time.Minute)
	require.NoError(t, svc.Confirm(ctx, token))
	require.Equal(t, confirmed.ConfirmedAt, subscriptionsOf(t, db, "reader@mail.test")[0].ConfirmedAt)
	require.Equal(t, 1, mailsTo(t, db, newSubType, author.Email))

	_, err = svc.ConfirmView(ctx, "unknown")
	require.ErrorIs(t, err, service.ErrNotFound)
	require.ErrorIs(t, svc.Confirm(ctx, "unknown"), service.ErrNotFound)

	c.advance(subscriptions.DefaultConfirmLifetime)

	late := subscriptionsOf(t, db, "late@mail.test")[0].ConfirmToken
	_, err = svc.ConfirmView(ctx, late)
	require.ErrorIs(t, err, service.ErrNotFound)
	require.ErrorIs(t, svc.Confirm(ctx, late), service.ErrNotFound)
	require.Equal(t, model.SubscriptionStatusPending, subscriptionsOf(t, db, "late@mail.test")[0].Status)

	_, err = svc.ConfirmView(ctx, token)
	require.NoError(t, err, "an active subscription's link keeps working")
}

// countingQueue is the real queue that also counts every mail handed to it,
// so a repeat the queue would drop as a duplicate still shows.
type countingQueue struct {
	repo.MailQueue
	sent atomic.Int32
}

func (q *countingQueue) Send(ctx context.Context, exec repo.Executor, uniqueID, emailType string, m *sender.Mail) error {
	q.sent.Add(1)

	return q.MailQueue.Send(ctx, exec, uniqueID, emailType, m)
}

func TestConfirm_Concurrent(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	sub := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "reader@mail.test", factory.PendingSince(time.Now().UTC())))(t)
	queue := &countingQueue{MailQueue: dbsender.NewSender(store, fakesender.New().Delivery())}

	// Hold the subscriber's lock until both confirmations have read the
	// pending subscription and wait for it.
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- store.Tx(ctx, func(tx *repo.Store) error {
			if _, err := tx.LockSubscriber(ctx, sub.SubscriberID); err != nil {
				return err
			}

			close(held)
			<-release

			return nil
		})
	}()
	<-held

	var wg sync.WaitGroup

	errs := make([]error, 2)
	for i := range errs {
		c := &clock{t: time.Now().UTC().Add(time.Duration(i) * time.Minute)}
		svc := subscriptions.New(store, queue, subscriptions.WithClock(c.now))

		wg.Go(func() { errs[i] = svc.Confirm(ctx, sub.ConfirmToken) })
	}

	require.Eventually(t, func() bool {
		var waiting int
		require.NoError(t, db.QueryRowContext(ctx, `select count(*) from pg_stat_activity
			where datname = current_database() and wait_event_type = 'Lock'`).Scan(&waiting))

		return waiting == 2
	}, 10*time.Second, 10*time.Millisecond)

	close(release)
	wg.Wait()
	require.NoError(t, <-done)
	require.Equal(t, []error{nil, nil}, errs)
	require.Equal(t, int32(1), queue.sent.Load(), "one author_new_subscriber mail")

	got := testutil.Must(factory.GetEmailSubscription(ctx, db, sub.ID))(t)
	require.Equal(t, model.SubscriptionStatusActive, got.Status)
	require.NotNil(t, got.ConfirmedAt)
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, _ := newSvc(db)

	alice := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	bob := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	carol := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)

	toAlice := testutil.Must(factory.EmailSubscription(ctx, db, alice.ID, "reader@mail.test"))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, bob.ID, "reader@mail.test"))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, carol.ID, "reader@mail.test", factory.PendingSince(time.Now())))(t)
	other := testutil.Must(factory.EmailSubscription(ctx, db, alice.ID, "other@mail.test"))(t)
	token := toAlice.Subscriber.ManageToken

	require.ErrorIs(t, svc.Unsubscribe(ctx, "unknown", toAlice.ID), service.ErrNotFound)
	require.ErrorIs(t, svc.Unsubscribe(ctx, token, other.ID), service.ErrNotFound)

	require.NoError(t, svc.Unsubscribe(ctx, token, toAlice.ID))
	require.Len(t, subscriptionsOf(t, db, "reader@mail.test"), 2)
	require.Equal(t, []string{alice.Email}, recipients(mails(t, db, leftType)))

	require.NoError(t, svc.UnsubscribeAll(ctx, token))
	_, err := factory.GetEmailSubscriber(ctx, db, "reader@mail.test")
	require.ErrorIs(t, err, sql.ErrNoRows, "the subscriber goes with their last subscription")
	require.Equal(t, 1, mailsTo(t, db, leftType, bob.Email))
	require.Zero(t, mailsTo(t, db, leftType, carol.Email), "a pending subscription's author never knew of it")
	require.Len(t, subscriptionsOf(t, db, "other@mail.test"), 1)

	require.ErrorIs(t, svc.UnsubscribeAll(ctx, token), service.ErrNotFound)
}

func TestManage(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, _ := newSvc(db)

	zed := testutil.Must(factory.User(ctx, db, factory.WithUsername("zed"), factory.Subscribable()))(t)
	amy := testutil.Must(factory.User(ctx, db, factory.WithUsername("amy")))(t)
	pending := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)

	toZed := testutil.Must(factory.EmailSubscription(ctx, db, zed.ID, "reader@mail.test"))(t)
	toAmy := testutil.Must(factory.EmailSubscription(ctx, db, amy.ID, "reader@mail.test"))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, pending.ID, "reader@mail.test", factory.PendingSince(time.Now())))(t)

	view, err := svc.Manage(ctx, toZed.Subscriber.ManageToken)
	require.NoError(t, err)
	require.Equal(t, &subscriptions.ManageView{Email: "reader@mail.test", Rows: []subscriptions.ManageRow{
		{ID: toAmy.ID, AuthorUsername: "amy", Paused: true},
		{ID: toZed.ID, AuthorUsername: "zed"},
	}}, view)

	_, err = svc.Manage(ctx, "unknown")
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestRequestManageLink(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, c := newSvc(db)

	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "reader@mail.test"))(t)

	require.NoError(t, svc.RequestManageLink(ctx, "nobody@mail.test"))
	require.Empty(t, mails(t, db, manageType))

	for range subscriptions.DefaultManageLinksPerAddress + 1 {
		require.NoError(t, svc.RequestManageLink(ctx, "reader@mail.test"))
		c.advance(time.Second)
	}

	require.Equal(t, subscriptions.DefaultManageLinksPerAddress, mailsTo(t, db, manageType, "reader@mail.test"))

	require.Equal(t, service.Invalid("email", subscriptions.InvalidEmailMessage), svc.RequestManageLink(ctx, "nope"))
}

func TestForAuthorAndRemove(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, _ := newSvc(db)

	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	stranger := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	now := time.Now().UTC().Truncate(time.Second)

	newest := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "newest@mail.test", factory.ConfirmedAt(now)))(t)
	oldest := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "oldest@mail.test", factory.ConfirmedAt(now.Add(-time.Hour))))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "pending@mail.test", factory.PendingSince(now)))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, stranger.ID, "oldest@mail.test"))(t)

	view, err := svc.ForAuthor(ctx, author)
	require.NoError(t, err)
	require.Equal(t, 2, view.Count)
	require.Equal(t, []string{newest.ID, oldest.ID}, []string{view.Rows[0].ID, view.Rows[1].ID})
	require.Equal(t, []string{"newest@mail.test", "oldest@mail.test"}, []string{view.Rows[0].Email, view.Rows[1].Email})
	require.True(t, now.Equal(*view.Rows[0].ConfirmedAt), "got %v", view.Rows[0].ConfirmedAt)
	require.True(t, now.Add(-time.Hour).Equal(*view.Rows[1].ConfirmedAt), "got %v", view.Rows[1].ConfirmedAt)
	require.Equal(t, 2, testutil.Must(svc.ActiveCount(ctx, nil, author.ID))(t))

	_, err = svc.ForAuthor(ctx, nil)
	require.ErrorIs(t, err, service.ErrNeedsLogin)

	require.ErrorIs(t, svc.Remove(ctx, stranger, newest.ID), service.ErrNotFound)
	require.Len(t, subscriptionsOf(t, db, "newest@mail.test"), 1)
	require.NoError(t, svc.Remove(ctx, author, newest.ID))
	require.NoError(t, svc.Remove(ctx, author, oldest.ID))

	_, err = factory.GetEmailSubscriber(ctx, db, "newest@mail.test")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Len(t, subscriptionsOf(t, db, "oldest@mail.test"), 1, "the subscription to another author stays")
	require.Equal(t, 0, testutil.Must(svc.ActiveCount(ctx, nil, author.ID))(t))
	require.Empty(t, testutil.Must(factory.ListOutgoingEmails(ctx, db))(t), "removing a subscriber mails nobody")
}

func TestNotifyPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	public := factory.Visibility(model.PostVisibilityPublic)

	for _, r := range []struct {
		name     string
		paused   bool
		postOpts []factory.PostOpt
		want     int
	}{
		{name: "public post", postOpts: []factory.PostOpt{public, factory.Published()}, want: 2},
		{name: "paused author", paused: true, postOpts: []factory.PostOpt{public, factory.Published()}},
		{name: "non-public post", postOpts: []factory.PostOpt{factory.Visibility(model.PostVisibilitySecondDegree), factory.Published()}},
		{name: "draft", postOpts: []factory.PostOpt{public}},
		{name: "already mailed", postOpts: []factory.PostOpt{public, factory.Published(), factory.SubscribersMailedAt(time.Now())}},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			opts := []factory.UserOpt{factory.Subscribable()}
			if r.paused {
				opts = append(opts, factory.WithVisibility(model.ProfileVisibilityConnections))
			}

			author := testutil.Must(factory.User(ctx, db, opts...))(t)
			post := testutil.Must(factory.Post(ctx, db, author.ID, r.postOpts...))(t)
			mailedAt := post.SubscribersMailedAt
			emails := []string{"a-" + post.ID + "@mail.test", "b-" + post.ID + "@mail.test"}

			wantIDs := map[string]string{}
			for _, e := range emails {
				sub := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, e))(t)
				wantIDs[e] = uuid.NewSHA1(uuid.NameSpaceURL, []byte(post.ID+sub.ID)).String() // as dbsender keys it
			}

			testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "pending-"+post.ID+"@mail.test", factory.PendingSince(time.Now())))(t)

			// A stale copy, as if read before the post was mailed: only the
			// stored mark stops a second mailing.
			stale := *post
			stale.SubscribersMailedAt = nil

			svc, _ := newSvc(db)
			store := repo.New(db)
			require.NoError(t, store.Tx(ctx, func(tx *repo.Store) error { return svc.NotifyPost(ctx, tx, author, new(stale)) }))
			require.NoError(t, svc.NotifyPost(ctx, nil, author, new(stale)))

			var got []queued
			for _, m := range mails(t, db, postType) {
				if strings.Contains(m.to, post.ID) {
					got = append(got, m)
				}
			}

			stored := testutil.Must(factory.GetPost(ctx, db, post.ID))(t)
			if r.want == 0 {
				require.Empty(t, got)
				require.Equal(t, mailedAt == nil, stored.SubscribersMailedAt == nil)

				return
			}

			require.ElementsMatch(t, emails, recipients(got))
			for _, m := range got {
				require.Equal(t, wantIDs[m.to], m.uniqueID)
			}
			require.NotNil(t, stored.SubscribersMailedAt)
		})
	}
}

func TestPrune(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, c := newSvc(db)
	store := repo.New(db)

	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "active@mail.test"))(t)
	require.NoError(t, svc.Subscribe(ctx, author.Username, "expired@mail.test"))

	c.advance(subscriptions.DefaultConfirmLifetime + time.Second)
	require.NoError(t, svc.Subscribe(ctx, author.Username, "fresh@mail.test"))
	require.NoError(t, svc.Prune(ctx))

	_, err := factory.GetEmailSubscriber(ctx, db, "expired@mail.test")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Len(t, subscriptionsOf(t, db, "fresh@mail.test"), 1)
	require.Len(t, subscriptionsOf(t, db, "active@mail.test"), 1)
	require.Equal(t, 1, testutil.Must(store.SubscriptionMailsSince(ctx, model.SubscriptionMailKindConfirm, time.Time{}))(t),
		"the mail log keeps the last day only")
}
