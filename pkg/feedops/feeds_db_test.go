package feedops_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSubscribeToFeed_InvalidURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	err = feedops.SubscribeToFeed(ctx, db, user.ID, "not-a-url")
	require.Error(t, err)
}

func TestDefaultRssReader(t *testing.T) {
	t.Parallel()

	require.NotNil(t, feedops.DefaultRssReader(testdb.New(t).DB, fakestorage.New()))
}

func TestGetRssFeeds_LastImportedMap(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	feedWithItems, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)
	feedWithoutItems, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)

	_, err = factory.Subscription(ctx, db, user.ID, feedWithItems.ID)
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, db, user.ID, feedWithoutItems.ID)
	require.NoError(t, err)

	_, err = factory.RSSItem(ctx, db, feedWithItems.ID)
	require.NoError(t, err)
	_, err = factory.RSSItem(ctx, db, feedWithItems.ID)
	require.NoError(t, err)

	feeds, err := feedops.GetRssFeeds(ctx, db, user.ID)
	require.NoError(t, err)
	require.Len(t, feeds, 2)

	byURL := map[string]*feedops.RssFeed{}
	for _, f := range feeds {
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

func TestGetRssFeedItems_ReturnsItemFields(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	feed, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, db, user.ID, feed.ID)
	require.NoError(t, err)

	item, err := factory.RSSItem(ctx, db, feed.ID)
	require.NoError(t, err)

	_, err = factory.UserFeedItem(ctx, db, user.ID, item.ID)
	require.NoError(t, err)

	items, err := feedops.GetRssFeedItems(ctx, db, user.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

	got := items[0]
	require.Equal(t, item.Title, got.Title)
	require.Equal(t, item.SanitizedDescription, got.Summary)
	require.Equal(t, feed.Title.String, got.FeedTitle)
	require.Equal(t, feed.URL, got.FeedURL)
	require.False(t, got.PublishedAt.IsZero())
}

func TestSubscribeToFeed_Idempotent(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	require.NoError(t, feedops.SubscribeToFeed(ctx, db, user.ID, "https://Example.com/feed.xml"))
	// Different case host, same feed once normalized: subscribing again
	// must not create a second feed row or a second subscription.
	require.NoError(t, feedops.SubscribeToFeed(ctx, db, user.ID, "https://example.com/feed.xml"))

	feeds, err := feedops.GetRssFeeds(ctx, db, user.ID)
	require.NoError(t, err)
	require.Len(t, feeds, 1)
}

func TestUnsubscribeFromFeed_ScopedToUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	feed, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)

	userA, err := factory.User(ctx, db)
	require.NoError(t, err)
	userB, err := factory.User(ctx, db)
	require.NoError(t, err)

	subA, err := factory.Subscription(ctx, db, userA.ID, feed.ID)
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, db, userB.ID, feed.ID)
	require.NoError(t, err)

	// userB guessing userA's subscription id must not be able to remove it.
	require.NoError(t, feedops.UnsubscribeFromFeed(ctx, db, userB.ID, subA.ID))

	feedsA, err := feedops.GetRssFeeds(ctx, db, userA.ID)
	require.NoError(t, err)
	require.Len(t, feedsA, 1, "userB's unsubscribe call must not remove userA's subscription")

	require.NoError(t, feedops.UnsubscribeFromFeed(ctx, db, userA.ID, subA.ID))

	feedsA, err = feedops.GetRssFeeds(ctx, db, userA.ID)
	require.NoError(t, err)
	require.Empty(t, feedsA)

	feedsB, err := feedops.GetRssFeeds(ctx, db, userB.ID)
	require.NoError(t, err)
	require.Len(t, feedsB, 1, "userB's own subscription is untouched")
}
