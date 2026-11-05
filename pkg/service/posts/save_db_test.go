package posts_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// svcWithSubscriptions is a posts service with the subscriptions service on
// the same store and queue, as the registry wires them.
func svcWithSubscriptions(store *repo.Store, q repo.JobQueue) *posts.Service {
	return posts.New(store, nil, posts.WithJobs(q), posts.WithSubscriptions(subscriptions.New(store, subscriptions.WithJobs(q))))
}

// authorWithReader is an author with one email subscriber, active unless
// opts say otherwise, whom readers can subscribe to if subscribable.
func authorWithReader(t *testing.T, ctx context.Context, db *sql.DB, subscribable bool, opts ...factory.EmailSubscriptionOpt) *model.User {
	t.Helper()

	var userOpts []factory.UserOpt
	if subscribable {
		userOpts = append(userOpts, factory.Subscribable())
	}

	author := testutil.Must(factory.User(ctx, db, userOpts...))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "reader-"+author.ID+"@example.test", opts...))(t)

	return author
}

func TestSave_NotifiesSubscribers(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	ctx := context.Background()
	yes, no := new(true), new(false)
	public := factory.Visibility(model.PostVisibilityPublic)
	connections := factory.Visibility(model.PostVisibilitySecondDegree)
	mailed := factory.SubscribersMailedAt(time.Now().Add(-time.Hour))
	publishedMailed := []factory.PostOpt{public, factory.Published(), mailed}

	cases := []struct {
		name         string
		existing     []factory.PostOpt // nil: a new post
		action       posts.Action
		notify       *bool
		wantMailed   bool
		wasMailedYet bool
	}{
		{name: "first publish by default", action: posts.ActionPublish, wantMailed: true},
		{name: "first publish, declined", action: posts.ActionPublish, notify: no},
		{name: "draft publish by default", existing: []factory.PostOpt{public}, action: posts.ActionPublish, wantMailed: true},
		{name: "published post becomes public by default", existing: []factory.PostOpt{connections, factory.Published()}, action: posts.ActionSavePost},
		{name: "published post becomes public, asked", existing: []factory.PostOpt{connections, factory.Published()}, action: posts.ActionSavePost, notify: yes, wantMailed: true},
		{name: "published public post never mailed, asked", existing: []factory.PostOpt{public, factory.Published()}, action: posts.ActionPublish, notify: yes, wantMailed: true},
		{name: "autosave never mails", existing: []factory.PostOpt{connections, factory.Published()}, action: posts.ActionAutosave, notify: yes},
		{name: "already mailed, asked again", existing: publishedMailed, action: posts.ActionSavePost, notify: yes, wasMailedYet: true},
		{name: "already mailed, declined", existing: publishedMailed, action: posts.ActionSavePost, notify: no, wasMailedYet: true},
		{name: "already mailed, autosaved", existing: publishedMailed, action: posts.ActionAutosave, wasMailedYet: true},
		{name: "already mailed, made a draft", existing: publishedMailed, action: posts.ActionMakeDraft, wasMailedYet: true},
		{name: "already mailed, republished after a draft", existing: []factory.PostOpt{public, mailed}, action: posts.ActionPublish, wasMailedYet: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			author := authorWithReader(t, ctx, db.DB, true)
			q := fakequeue.New()
			in := posts.SaveInput{Body: "a body", Visibility: model.PostVisibilityPublic, Action: tc.action, NotifySubscribers: tc.notify}

			var before *model.Post
			if tc.existing != nil {
				before = testutil.Must(factory.Post(ctx, db.DB, author.ID, tc.existing...))(t)
				in.PostID = before.ID
			}

			saved := testutil.Must(svcWithSubscriptions(repo.New(db.DB), q).Save(ctx, author, in))(t)
			stored := testutil.Must(factory.GetPost(ctx, db.DB, saved.Post.ID))(t)

			enqueued := fakequeue.Of[subscriptions.PostPublishedArgs](q)
			if tc.wantMailed {
				require.Equal(t, []subscriptions.PostPublishedArgs{{PostID: stored.ID}}, enqueued)
				require.NotNil(t, stored.SubscribersMailedAt)

				return
			}

			require.Empty(t, enqueued)

			if tc.wasMailedYet {
				require.NotNil(t, stored.SubscribersMailedAt)
				require.WithinDuration(t, *before.SubscribersMailedAt, *stored.SubscribersMailedAt, time.Second)
			} else {
				require.Nil(t, stored.SubscribersMailedAt)
			}
		})
	}
}

// The mark and the subscribers' job are written in the save's transaction,
// so a save that rolls back leaves neither.
func TestSave_NotifySubscribersRollsBack(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	ins := testutil.Must(jobs.NewInserter(db))(t)
	author := authorWithReader(t, ctx, db, true)
	post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published()))(t)
	rollback := errors.New("rollback")

	err := repo.New(db).Tx(ctx, func(tx *repo.Store) error {
		_, err := svcWithSubscriptions(tx, ins).Save(ctx, author, posts.SaveInput{PostID: post.ID, Body: "a body",
			Visibility: model.PostVisibilityPublic, Action: posts.ActionSavePost, NotifySubscribers: new(true)})
		require.NoError(t, err)

		// the save did mark it, inside the transaction
		require.NotNil(t, testutil.Must(factory.GetPost(ctx, tx.Exec(), post.ID))(t).SubscribersMailedAt)

		return rollback
	})
	require.ErrorIs(t, err, rollback)

	require.Nil(t, testutil.Must(factory.GetPost(ctx, db, post.ID))(t).SubscribersMailedAt)

	var queued int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE kind = $1`, subscriptions.PostPublishedArgs{}.Kind()).Scan(&queued))
	require.Zero(t, queued)
}

func TestSubscriberReach(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	ctx := context.Background()
	pending := factory.PendingSince(time.Now())

	cases := []struct {
		name      string
		closed    bool
		noReaders bool
		reader    []factory.EmailSubscriptionOpt
		post      []factory.PostOpt // nil: a new post
		want      posts.SubscriberReach
	}{
		{name: "author not subscribable", closed: true, want: posts.SubscriberReach{NotifyByDefault: true}},
		{name: "no subscribers", noReaders: true, want: posts.SubscriberReach{Subscribable: true, NotifyByDefault: true}},
		{name: "only a pending subscriber", reader: []factory.EmailSubscriptionOpt{pending}, want: posts.SubscriberReach{Subscribable: true, NotifyByDefault: true}},
		{name: "new post", want: posts.SubscriberReach{Subscribable: true, Count: 1, CanNotify: true, NotifyByDefault: true}},
		{name: "draft", post: []factory.PostOpt{}, want: posts.SubscriberReach{Subscribable: true, Count: 1, CanNotify: true, NotifyByDefault: true}},
		{name: "published post", post: []factory.PostOpt{factory.Published()}, want: posts.SubscriberReach{Subscribable: true, Count: 1, CanNotify: true}},
		{name: "mailed post", post: []factory.PostOpt{factory.Published(), factory.SubscribersMailedAt(time.Now())}, want: posts.SubscriberReach{Subscribable: true, Count: 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var author *model.User
			if tc.noReaders {
				author = testutil.Must(factory.User(ctx, db.DB, factory.Subscribable()))(t)
			} else {
				author = authorWithReader(t, ctx, db.DB, !tc.closed, tc.reader...)
			}

			var post *model.Post
			if tc.post != nil {
				post = testutil.Must(factory.Post(ctx, db.DB, author.ID, tc.post...))(t)
			}

			reach := testutil.Must(svcWithSubscriptions(repo.New(db.DB), fakequeue.New()).SubscriberReach(ctx, author, post))(t)
			require.Equal(t, tc.want, *reach)
		})
	}
}
