package feeds_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops/testutil"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/feeds"
	testutil2 "github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func svc(db boil.ContextExecutor) *feeds.Service {
	return feeds.New(repo.Using(db), fakestorage.New())
}

func TestSubscribe(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("invalid URL", func(t *testing.T) {
		t.Parallel()

		user := testutil2.Must(factory.User(ctx, db))(t)
		require.Error(t, svc(db).Subscribe(ctx, user, "not-a-url"))
	})

	t.Run("idempotent on normalized host", func(t *testing.T) {
		t.Parallel()

		user := testutil2.Must(factory.User(ctx, db))(t)

		require.NoError(t, svc(db).Subscribe(ctx, user, "https://Example.com/feed.xml"))
		// Different case host, same feed once normalized: subscribing again
		// must not create a second feed row or a second subscription.
		require.NoError(t, svc(db).Subscribe(ctx, user, "https://example.com/feed.xml"))

		fs := testutil2.Must(svc(db).Subscriptions(ctx, user))(t)
		require.Len(t, fs, 1)
	})
}

// TestNew_FetchesAndParsesFeed exercises the reader
// New wires together: given a subscribed feed backed by a real
// HTTP server, its poller should fetch the RSS document, parse it and store
// the item, not just construct without panicking.
func TestNew_FetchesAndParsesFeed(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	const rssXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <description>Test feed description</description>
    <item>
      <title>Hello world</title>
      <link>https://example.test/hello-world</link>
      <description>hello</description>
      <guid>https://example.test/hello-world</guid>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssXML))
	}))
	t.Cleanup(srv.Close)

	user := testutil2.Must(factory.User(ctx, db))(t)
	require.NoError(t, svc(db).Subscribe(ctx, user, srv.URL))

	rssReader := feeds.New(repo.New(db), fakestorage.New())

	pollCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		rssReader.RunPoller(pollCtx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		items, err := svc(db).Items(ctx, user)
		return err == nil && len(items) == 1
	}, 20*time.Second, 200*time.Millisecond, "the poller should fetch and store the feed's item")

	cancel()
	<-done

	items := testutil2.Must(svc(db).Items(ctx, user))(t)
	require.Len(t, items, 1)
	require.Equal(t, "Hello world", items[0].Title)
}

func TestSubscriptions_LastImportedMap(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil2.Must(factory.User(ctx, db))(t)
	feedWithItems := testutil2.Must(factory.RSSFeed(ctx, db))(t)
	feedWithoutItems := testutil2.Must(factory.RSSFeed(ctx, db))(t)

	testutil2.Must(factory.Subscription(ctx, db, user.ID, feedWithItems.ID))(t)
	testutil2.Must(factory.Subscription(ctx, db, user.ID, feedWithoutItems.ID))(t)

	testutil2.Must(factory.RSSItem(ctx, db, feedWithItems.ID))(t)
	testutil2.Must(factory.RSSItem(ctx, db, feedWithItems.ID))(t)

	fs := testutil2.Must(svc(db).Subscriptions(ctx, user))(t)
	require.Len(t, fs, 2)

	byURL := map[string]*feeds.RssFeed{}
	for _, f := range fs {
		byURL[f.URL] = f
	}

	withItems, ok := byURL[feedWithItems.URL]
	require.True(t, ok)
	require.Equal(t, feedWithItems.Title.String, withItems.Title)
	require.NotNil(t, withItems.LastImportedAt, "a feed with items should have a last-imported time")
	require.WithinDuration(t, time.Now(), *withItems.LastImportedAt, 5*time.Second)

	withoutItems, ok := byURL[feedWithoutItems.URL]
	require.True(t, ok)
	require.Nil(t, withoutItems.LastImportedAt, "a feed with no items has no last-imported time")
}

func TestItems_ReturnsItemFields(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil2.Must(factory.User(ctx, db))(t)
	feed := testutil2.Must(factory.RSSFeed(ctx, db))(t)
	testutil2.Must(factory.Subscription(ctx, db, user.ID, feed.ID))(t)
	item := testutil2.Must(factory.RSSItem(ctx, db, feed.ID))(t)
	testutil2.Must(factory.UserFeedItem(ctx, db, user.ID, item.ID))(t)

	items := testutil2.Must(svc(db).Items(ctx, user))(t)
	require.Len(t, items, 1)

	got := items[0]
	require.Equal(t, item.Title, got.Title)
	require.Equal(t, item.SanitizedDescription, got.Summary)
	require.Equal(t, feed.Title.String, got.FeedTitle)
	require.Equal(t, feed.URL, got.FeedURL)
	require.False(t, got.PublishedAt.IsZero())
}

func TestUnsubscribe_ScopedToUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	feed := testutil2.Must(factory.RSSFeed(ctx, db))(t)
	userA := testutil2.Must(factory.User(ctx, db))(t)
	userB := testutil2.Must(factory.User(ctx, db))(t)

	subA := testutil2.Must(factory.Subscription(ctx, db, userA.ID, feed.ID))(t)
	testutil2.Must(factory.Subscription(ctx, db, userB.ID, feed.ID))(t)

	// userB guessing userA's subscription id must not be able to remove it.
	require.NoError(t, svc(db).Unsubscribe(ctx, userB, subA.ID))

	feedsA := testutil2.Must(svc(db).Subscriptions(ctx, userA))(t)
	require.Len(t, feedsA, 1, "userB's unsubscribe call must not remove userA's subscription")

	require.NoError(t, svc(db).Unsubscribe(ctx, userA, subA.ID))

	feedsA = testutil2.Must(svc(db).Subscriptions(ctx, userA))(t)
	require.Empty(t, feedsA)

	feedsB := testutil2.Must(svc(db).Subscriptions(ctx, userB))(t)
	require.Len(t, feedsB, 1, "userB's own subscription is untouched")
}

func TestItems_FiltersDismissedItems(t *testing.T) {
	testDB := testdb.New(t)

	ctx := context.Background()

	user, err := testutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	feed, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed", "Test Feed")
	require.NoError(t, err)

	_, err = testutil.CreateUserFeedSubscription(ctx, testDB.DB, user.ID, feed.ID)
	require.NoError(t, err)

	now := time.Now()

	url, err := testutil.CreateURL(ctx, testDB.DB, "https://example.com/item")
	require.NoError(t, err)
	rssItem, err := testutil.CreateRSSItem(ctx, testDB.DB, feed.ID, url.ID, "Active Item", now)
	require.NoError(t, err)
	_, err = testutil.CreateUserFeedItem(ctx, testDB.DB, user.ID, rssItem.ID, url.ID, now)
	require.NoError(t, err)

	items, err := svc(testDB.DB).Items(ctx, user)
	require.NoError(t, err)
	require.Len(t, items, 1, "Should return 1 active item")

	_, err = testDB.DB.Exec(
		"UPDATE user_feed_items SET is_dismissed = true WHERE user_id = $1",
		user.ID,
	)
	require.NoError(t, err)

	items, err = svc(testDB.DB).Items(ctx, user)
	require.NoError(t, err)
	require.Len(t, items, 0, "Should return 0 items after dismissing")
}

func TestItems_EmptyResult(t *testing.T) {
	testDB := testdb.New(t)

	ctx := context.Background()

	user, err := testutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	items, err := svc(testDB.DB).Items(ctx, user)
	require.NoError(t, err)
	require.Len(t, items, 0, "Should return empty list for user with no items")
}

func TestDismiss_ScopedToUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	owner := testutil2.Must(factory.User(ctx, db))(t)
	other := testutil2.Must(factory.User(ctx, db))(t)
	feed := testutil2.Must(factory.RSSFeed(ctx, db))(t)
	item := testutil2.Must(factory.RSSItem(ctx, db, feed.ID))(t)
	userItem := testutil2.Must(factory.UserFeedItem(ctx, db, owner.ID, item.ID))(t)

	require.ErrorIs(t, svc(db).Dismiss(ctx, other, userItem.ID), service.ErrNotFound, "another user's item is not found")

	var invalid *service.ValidationError
	require.ErrorAs(t, svc(db).Dismiss(ctx, owner, ""), &invalid)
	require.Equal(t, "No item found", invalid.Message)
	require.Len(t, testutil2.Must(svc(db).Items(ctx, owner))(t), 1)

	require.NoError(t, svc(db).Dismiss(ctx, owner, userItem.ID))
	require.Empty(t, testutil2.Must(svc(db).Items(ctx, owner))(t))
}

func TestActorAndInputErrors(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	user := testutil2.Must(factory.User(ctx, db))(t)

	t.Run("anonymous actor needs login", func(t *testing.T) {
		t.Parallel()

		s := svc(db)
		calls := map[string]func() error{
			"Subscribe":     func() error { return s.Subscribe(ctx, nil, "https://example.com/feed") },
			"Unsubscribe":   func() error { return s.Unsubscribe(ctx, nil, "id") },
			"Dismiss":       func() error { return s.Dismiss(ctx, nil, "id") },
			"Subscriptions": func() error { _, err := s.Subscriptions(ctx, nil); return err },
			"Items":         func() error { _, err := s.Items(ctx, nil); return err },
		}

		for name, call := range calls {
			require.ErrorIs(t, call(), service.ErrNeedsLogin, name)
		}
	})

	t.Run("empty subscription id", func(t *testing.T) {
		t.Parallel()

		var invalid *service.ValidationError
		require.ErrorAs(t, svc(db).Unsubscribe(ctx, user, ""), &invalid)
		require.Equal(t, "No subscription found", invalid.Message)
	})

	t.Run("dismiss with a malformed id shows the database error", func(t *testing.T) {
		t.Parallel()

		err := svc(db).Dismiss(ctx, user, "not-a-uuid")
		require.Error(t, err)
		require.NotErrorIs(t, err, service.ErrNotFound)
		require.NotContains(t, err.Error(), "Operation Failed")
	})
}
