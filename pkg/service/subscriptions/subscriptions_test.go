package subscriptions_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
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

// newSvc is the service over a fake job queue, which it returns, and the
// fake resolver, timed by a clock it returns.
func newSvc(db *sql.DB, opts ...subscriptions.Option) (*subscriptions.Service, *clock, *fakequeue.Queue) {
	c := &clock{t: time.Now().UTC().Truncate(time.Second)}
	q := fakequeue.New()
	base := []subscriptions.Option{
		subscriptions.WithJobs(q),
		subscriptions.WithClock(c.now),
		subscriptions.WithEmailChecker(emailaddr.NewChecker(emailaddr.WithResolver(resolver{}))),
		subscriptions.WithIdentity(mail.Identity{Site: mail.SampleSite, From: "pcom@pcom.test", AdminAddress: adminAddr}),
	}

	return subscriptions.New(repo.New(db), append(base, opts...)...), c, q
}

// render builds the mail of a mail job, as the worker would now.
func render(t *testing.T, svc *subscriptions.Service, args repo.JobArgs) *mail.Envelope {
	t.Helper()

	var (
		env *mail.Envelope
		err error
		ctx = context.Background()
	)

	switch a := args.(type) {
	case subscriptions.ConfirmMailArgs:
		env, err = svc.RenderConfirm(ctx, a)
	case subscriptions.ManageLinkMailArgs:
		env, err = svc.RenderManageLink(ctx, a)
	case subscriptions.NewPostMailArgs:
		env, err = svc.RenderNewPost(ctx, a)
	case subscriptions.NewSubscriberMailArgs:
		env, err = svc.RenderNewSubscriber(ctx, a)
	case subscriptions.SubscriberLeftMailArgs:
		env, err = svc.RenderSubscriberLeft(ctx, a)
	case subscriptions.BrakedMailArgs:
		env, err = svc.RenderBraked(ctx, a)
	default:
		t.Fatalf("not a mail job: %T", args)
	}

	require.NoError(t, err, args.Kind())

	return env
}

type queued struct{ to, uniqueID string }

// mails renders the mail jobs of emailType the queue holds; a job that
// renders no mail any more is left out.
func mails(t *testing.T, svc *subscriptions.Service, q *fakequeue.Queue, emailType string) []queued {
	t.Helper()

	var out []queued

	for _, args := range q.Inserted() {
		if args.Kind() != "mail."+emailType {
			continue
		}

		if env := render(t, svc, args); env != nil {
			require.Equal(t, emailType, env.Type)
			out = append(out, queued{to: env.Mail.To[0].Address, uniqueID: env.UniqueID})
		}
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
func mailsTo(t *testing.T, svc *subscriptions.Service, q *fakequeue.Queue, emailType, to string) int {
	t.Helper()

	n := 0
	for _, m := range mails(t, svc, q, emailType) {
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

	svc := subscriptions.New(nil)

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
		// opts configure the service beyond newSvc's.
		opts []subscriptions.Option
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
		{
			name: "disposable address where they are allowed", email: "y@mailinator.com",
			opts:         []subscriptions.Option{subscriptions.WithEmailChecker(emailaddr.NewChecker(emailaddr.WithResolver(resolver{}), emailaddr.WithAllowDisposable(true)))},
			wantConfirms: 1, wantLogged: 1, wantStatus: model.SubscriptionStatusPending,
		},
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
			svc, c, q := newSvc(db, r.opts...)

			if r.before != nil {
				r.before(t, svc, c, author, r.email)
			}

			prev := subscriptionsOf(t, db, r.email)

			require.Equal(t, r.wantErr, svc.Subscribe(ctx, author.Username, r.email))
			require.Equal(t, r.wantConfirms, mailsTo(t, svc, q, confirmType, r.email))
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
	svc, c, q := newSvc(db, subscriptions.WithConfirmsPerHour(1))
	paused := service.Invalid("", subscriptions.PausedMessage)

	require.NoError(t, svc.Subscribe(ctx, author.Username, "first@mail.test"))
	require.Equal(t, paused, svc.Subscribe(ctx, author.Username, "second@mail.test"))
	require.Equal(t, paused, svc.RequestManageLink(ctx, "first@mail.test"))
	require.Empty(t, subscriptionsOf(t, db, "second@mail.test"))
	require.Equal(t, paused, svc.Subscribe(ctx, author.Username, "second@mail.test"))

	// Every alert of the hour is the same job, which the queue keeps once
	// (TestBrakedMailArgs_UniqueByHour).
	alerts := fakequeue.Of[subscriptions.BrakedMailArgs](q)
	require.Len(t, alerts, 3)
	require.Equal(t, alerts[0], alerts[1])
	require.Equal(t, alerts[0], alerts[2])
	require.Equal(t, adminAddr, render(t, svc, alerts[0]).Mail.To[0].Address)

	c.advance(time.Hour + time.Second)
	require.NoError(t, svc.Subscribe(ctx, author.Username, "second@mail.test"))
	require.Equal(t, 1, mailsTo(t, svc, q, confirmType, "second@mail.test"))
}

// The inserter keeps one admin alert per hour.
func TestBrakedMailArgs_UniqueByHour(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	ins, err := jobs.NewInserter(db)
	require.NoError(t, err)

	count := func() int {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE kind = $1`, subscriptions.BrakedMailArgs{}.Kind()).Scan(&n))

		return n
	}

	hour := time.Now().UTC().Truncate(time.Hour)
	require.NoError(t, ins.Insert(ctx, db, subscriptions.BrakedMailArgs{Hour: hour}))
	require.NoError(t, ins.Insert(ctx, db, subscriptions.BrakedMailArgs{Hour: hour}))
	require.Equal(t, 1, count())

	require.NoError(t, ins.Insert(ctx, db, subscriptions.BrakedMailArgs{Hour: hour.Add(time.Hour)}))
	require.Equal(t, 2, count())
}

func TestConfirm(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	svc, c, q := newSvc(db)

	require.NoError(t, svc.Subscribe(ctx, author.Username, "reader@mail.test"))
	require.NoError(t, svc.Subscribe(ctx, author.Username, "late@mail.test"))

	require.Zero(t, mailsTo(t, svc, q, newSubType, author.Email), "the author hears of a subscriber only once confirmed")

	token := subscriptionsOf(t, db, "reader@mail.test")[0].ConfirmToken
	view, err := svc.ConfirmView(ctx, token)
	require.NoError(t, err)
	require.Equal(t, &subscriptions.ConfirmView{AuthorUsername: author.Username, Email: "reader@mail.test"}, view)
	require.Equal(t, model.SubscriptionStatusPending, subscriptionsOf(t, db, "reader@mail.test")[0].Status)

	require.NoError(t, svc.Confirm(ctx, token))
	confirmed := subscriptionsOf(t, db, "reader@mail.test")[0]
	require.Equal(t, model.SubscriptionStatusActive, confirmed.Status)
	require.NotNil(t, confirmed.ConfirmedAt)

	c.advance(time.Minute)
	require.NoError(t, svc.Confirm(ctx, token))
	require.Equal(t, confirmed.ConfirmedAt, subscriptionsOf(t, db, "reader@mail.test")[0].ConfirmedAt)
	require.Equal(t, 1, mailsTo(t, svc, q, newSubType, author.Email))

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

func TestConfirm_Concurrent(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	sub := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "reader@mail.test", factory.PendingSince(time.Now().UTC())))(t)
	queue := fakequeue.New()

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
		svc := subscriptions.New(store, subscriptions.WithJobs(queue), subscriptions.WithClock(c.now))

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
	require.Equal(t, []subscriptions.NewSubscriberMailArgs{{SubscriptionID: sub.ID}}, fakequeue.Of[subscriptions.NewSubscriberMailArgs](queue),
		"one author_new_subscriber mail")

	got := testutil.Must(factory.GetEmailSubscription(ctx, db, sub.ID))(t)
	require.Equal(t, model.SubscriptionStatusActive, got.Status)
	require.NotNil(t, got.ConfirmedAt)
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, _, q := newSvc(db)

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
	require.Equal(t, []string{alice.Email}, recipients(mails(t, svc, q, leftType)))

	require.NoError(t, svc.UnsubscribeAll(ctx, token))
	_, err := factory.GetEmailSubscriber(ctx, db, "reader@mail.test")
	require.ErrorIs(t, err, sql.ErrNoRows, "the subscriber goes with their last subscription")
	require.Equal(t, 1, mailsTo(t, svc, q, leftType, bob.Email), "the subscription is gone, the mail still names it")
	require.Zero(t, mailsTo(t, svc, q, leftType, carol.Email), "a pending subscription's author never knew of it")
	require.Len(t, subscriptionsOf(t, db, "other@mail.test"), 1)

	require.ErrorIs(t, svc.UnsubscribeAll(ctx, token), service.ErrNotFound)
}

func TestManage(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, _, _ := newSvc(db)

	zed := testutil.Must(factory.User(ctx, db, factory.WithUsername("zed"), factory.Subscribable()))(t)
	amy := testutil.Must(factory.User(ctx, db, factory.WithUsername("amy")))(t)
	pending := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)

	toZed := testutil.Must(factory.EmailSubscription(ctx, db, zed.ID, "reader@mail.test"))(t)
	toAmy := testutil.Must(factory.EmailSubscription(ctx, db, amy.ID, "reader@mail.test"))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, pending.ID, "reader@mail.test", factory.PendingSince(time.Now())))(t)

	view, err := svc.Manage(ctx, toZed.Subscriber.ManageToken)
	require.NoError(t, err)
	for i := range view.Rows {
		require.False(t, view.Rows[i].Since.IsZero(), "a row says since when")
		view.Rows[i].Since = time.Time{}
	}
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
	svc, c, q := newSvc(db)

	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "reader@mail.test"))(t)

	require.NoError(t, svc.RequestManageLink(ctx, "nobody@mail.test"))
	require.Empty(t, mails(t, svc, q, manageType))

	for range subscriptions.DefaultManageLinksPerAddress + 1 {
		require.NoError(t, svc.RequestManageLink(ctx, "reader@mail.test"))
		c.advance(time.Second)
	}

	sent := mails(t, svc, q, manageType)
	require.Len(t, sent, subscriptions.DefaultManageLinksPerAddress)
	require.Equal(t, subscriptions.DefaultManageLinksPerAddress, mailsTo(t, svc, q, manageType, "reader@mail.test"))
	require.NotEqual(t, sent[0].uniqueID, sent[1].uniqueID, "each request is its own mail")

	require.Equal(t, service.Invalid("email", subscriptions.InvalidEmailMessage), svc.RequestManageLink(ctx, "nope"))
}

func TestForAuthorAndRemove(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, _, q := newSvc(db)

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
	require.Empty(t, q.Inserted(), "removing a subscriber mails nobody")
}

func TestNotifyPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	public := factory.Visibility(model.PostVisibilityPublic)

	for _, r := range []struct {
		name          string
		paused        bool
		noSubscribers bool
		postOpts      []factory.PostOpt
		want          bool
	}{
		{name: "public post", postOpts: []factory.PostOpt{public, factory.Published()}, want: true},
		{name: "paused author", paused: true, postOpts: []factory.PostOpt{public, factory.Published()}},
		{name: "no subscribers", noSubscribers: true, postOpts: []factory.PostOpt{public, factory.Published()}},
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

			if !r.noSubscribers {
				testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "a-"+post.ID+"@mail.test"))(t)
			}

			testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "pending-"+post.ID+"@mail.test", factory.PendingSince(time.Now())))(t)

			// A stale copy, as if read before the post was mailed: only the
			// stored mark stops a second mailing.
			stale := *post
			stale.SubscribersMailedAt = nil

			svc, _, q := newSvc(db)
			store := repo.New(db)
			require.NoError(t, store.Tx(ctx, func(tx *repo.Store) error { return svc.NotifyPost(ctx, tx, author, new(stale)) }))
			require.NoError(t, svc.NotifyPost(ctx, nil, author, new(stale)))

			stored := testutil.Must(factory.GetPost(ctx, db, post.ID))(t)
			if !r.want {
				require.Empty(t, q.Inserted())
				require.Equal(t, mailedAt == nil, stored.SubscribersMailedAt == nil)

				return
			}

			require.Equal(t, []repo.JobArgs{subscriptions.PostPublishedArgs{PostID: post.ID}}, q.Inserted(), "one event job, no mail yet")
			require.NotNil(t, stored.SubscribersMailedAt)
		})
	}
}

// The event job mails the post to each subscriber active when it runs.
func TestFanOutPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Visibility(model.PostVisibilityPublic), factory.Published()))(t)
	a := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "a@mail.test"))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "pending@mail.test", factory.PendingSince(time.Now())))(t)

	svc, _, q := newSvc(db)
	require.NoError(t, svc.NotifyPost(ctx, nil, author, post))

	// subscribed after the post was saved, before the job ran
	b := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "b@mail.test"))(t)

	event := fakequeue.Of[subscriptions.PostPublishedArgs](q)
	require.Len(t, event, 1)
	require.NoError(t, svc.FanOutPost(ctx, event[0]))

	require.ElementsMatch(t, []subscriptions.NewPostMailArgs{
		{PostID: post.ID, SubscriptionID: a.ID},
		{PostID: post.ID, SubscriptionID: b.ID},
	}, fakequeue.Of[subscriptions.NewPostMailArgs](q))

	require.ElementsMatch(t, []queued{
		{to: "a@mail.test", uniqueID: post.ID + a.ID},
		{to: "b@mail.test", uniqueID: post.ID + b.ID},
	}, mails(t, svc, q, postType))
}

// The event job re-checks the post and its author when it runs.
func TestFanOutPost_RechecksAtRunTime(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	for name, change := range map[string]string{
		"post made draft":       `UPDATE posts SET published_at = NULL WHERE id = $1`,
		"post no longer public": `UPDATE posts SET visibility_radius = 'second_degree' WHERE id = $1`,
		"author paused":         `UPDATE users SET email_subscriptions_enabled = false WHERE id = (SELECT user_id FROM posts WHERE id = $1)`,
		"post deleted":          `DELETE FROM posts WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
			post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Visibility(model.PostVisibilityPublic), factory.Published()))(t)
			testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "reader-"+post.ID+"@mail.test"))(t)

			svc, _, q := newSvc(db)
			require.NoError(t, svc.NotifyPost(ctx, nil, author, post))

			_, err := db.ExecContext(ctx, change, post.ID)
			require.NoError(t, err)

			require.NoError(t, svc.FanOutPost(ctx, subscriptions.PostPublishedArgs{PostID: post.ID}))
			require.Empty(t, fakequeue.Of[subscriptions.NewPostMailArgs](q))
		})
	}
}

// Mails are rendered as the rows are when they are sent: what changed since
// the enqueue sends nothing.
func TestRenderMails_RecheckAtSend(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)

	t.Run("confirmation of a subscription unsubscribed", func(t *testing.T) {
		t.Parallel()

		svc, _, q := newSvc(db)
		require.NoError(t, svc.Subscribe(ctx, author.Username, "gone@mail.test"))

		sub := subscriptionsOf(t, db, "gone@mail.test")[0]
		subscriber := testutil.Must(factory.GetEmailSubscriber(ctx, db, "gone@mail.test"))(t)
		require.NoError(t, svc.Unsubscribe(ctx, subscriber.ManageToken, sub.ID))

		require.Nil(t, render(t, svc, fakequeue.Of[subscriptions.ConfirmMailArgs](q)[0]))
	})

	t.Run("confirmation of a subscription confirmed", func(t *testing.T) {
		t.Parallel()

		svc, _, q := newSvc(db)
		require.NoError(t, svc.Subscribe(ctx, author.Username, "quick@mail.test"))
		require.NoError(t, svc.Confirm(ctx, subscriptionsOf(t, db, "quick@mail.test")[0].ConfirmToken))

		require.Nil(t, render(t, svc, fakequeue.Of[subscriptions.ConfirmMailArgs](q)[0]))
	})

	t.Run("confirmation whose link expired", func(t *testing.T) {
		t.Parallel()

		svc, c, q := newSvc(db)
		require.NoError(t, svc.Subscribe(ctx, author.Username, "slow@mail.test"))
		c.advance(subscriptions.DefaultConfirmLifetime)

		require.Nil(t, render(t, svc, fakequeue.Of[subscriptions.ConfirmMailArgs](q)[0]))
	})

	t.Run("new subscriber removed", func(t *testing.T) {
		t.Parallel()

		svc, _, q := newSvc(db)
		sub := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "removed@mail.test", factory.PendingSince(time.Now())))(t)
		require.NoError(t, svc.Confirm(ctx, sub.ConfirmToken))
		require.NoError(t, svc.Remove(ctx, author, sub.ID))

		require.Nil(t, render(t, svc, fakequeue.Of[subscriptions.NewSubscriberMailArgs](q)[0]))
	})

	t.Run("manage link of an address that left", func(t *testing.T) {
		t.Parallel()

		svc, _, q := newSvc(db)
		sub := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "left@mail.test"))(t)
		require.NoError(t, svc.RequestManageLink(ctx, "left@mail.test"))
		require.NoError(t, svc.Remove(ctx, author, sub.ID))

		require.Nil(t, render(t, svc, fakequeue.Of[subscriptions.ManageLinkMailArgs](q)[0]))
	})

	t.Run("post to a subscription ended", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newSvc(db)
		post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Visibility(model.PostVisibilityPublic), factory.Published()))(t)
		sub := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "ended@mail.test"))(t)
		args := subscriptions.NewPostMailArgs{PostID: post.ID, SubscriptionID: sub.ID}
		require.NotNil(t, render(t, svc, args))

		require.NoError(t, svc.Remove(ctx, author, sub.ID))
		require.Nil(t, render(t, svc, args))
	})

	t.Run("post made draft", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newSvc(db)
		post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Visibility(model.PostVisibilityPublic), factory.Published()))(t)
		sub := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "draft@mail.test"))(t)

		_, err := db.ExecContext(ctx, `UPDATE posts SET published_at = NULL WHERE id = $1`, post.ID)
		require.NoError(t, err)
		require.Nil(t, render(t, svc, subscriptions.NewPostMailArgs{PostID: post.ID, SubscriptionID: sub.ID}))
	})

	t.Run("subscriber left, author gone", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newSvc(db)
		require.Nil(t, render(t, svc, subscriptions.SubscriberLeftMailArgs{
			SubscriptionID: "s", AuthorID: "00000000-0000-0000-0000-000000000000", SubscriberEmail: "x@mail.test",
		}))
	})
}

// Every mail kind renders the mail it names, to its recipient, with a fact
// of the rows it was enqueued for.
func TestRenderMails_EveryKind(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, _, _ := newSvc(db)

	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Visibility(model.PostVisibilityPublic), factory.Published(), factory.WithSubject("Hello subscribers")))(t)
	active := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "active@mail.test"))(t)
	pending := testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "pending@mail.test", factory.PendingSince(time.Now().UTC())))(t)
	hour := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		args    repo.JobArgs
		emailTp string
		to      string
		body    string
	}{
		{subscriptions.ConfirmMailArgs{SubscriptionID: pending.ID}, confirmType, "pending@mail.test", pending.ConfirmToken},
		{subscriptions.ManageLinkMailArgs{SubscriberID: active.SubscriberID, RequestID: "r1"}, manageType, "active@mail.test", active.Subscriber.ManageToken},
		{subscriptions.NewPostMailArgs{PostID: post.ID, SubscriptionID: active.ID}, postType, "active@mail.test", "Hello subscribers"},
		{subscriptions.NewSubscriberMailArgs{SubscriptionID: active.ID}, newSubType, author.Email, "active@mail.test"},
		{subscriptions.SubscriberLeftMailArgs{SubscriptionID: "gone", AuthorID: author.ID, SubscriberEmail: "left@mail.test"}, leftType, author.Email, "left@mail.test"},
		{subscriptions.BrakedMailArgs{Hour: hour}, brakedType, adminAddr, ""},
	} {
		t.Run(tt.args.Kind(), func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "mail."+tt.emailTp, tt.args.Kind())

			env := render(t, svc, tt.args)
			require.NotNil(t, env)
			require.Equal(t, tt.emailTp, env.Type)
			require.NotEmpty(t, env.UniqueID)
			require.Equal(t, tt.to, env.Mail.To[0].Address)
			require.Contains(t, env.Mail.Text, tt.body)
		})
	}
}

func TestPrune(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc, c, _ := newSvc(db)
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
