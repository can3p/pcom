package feedops_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/feedops/testutil"
	"github.com/can3p/pcom/testcontainers/postgres"
	"github.com/stretchr/testify/require"
)

func TestGetRssFeedItems_FiltersDismissedItems(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer testDB.Close()

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

	items, err := feedops.GetRssFeedItems(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, items, 1, "Should return 1 active item")

	_, err = testDB.DB.Exec(
		"UPDATE user_feed_items SET is_dismissed = true WHERE user_id = $1",
		user.ID,
	)
	require.NoError(t, err)

	items, err = feedops.GetRssFeedItems(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, items, 0, "Should return 0 items after dismissing")
}

func TestGetRssFeedItems_EmptyResult(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer testDB.Close()

	ctx := context.Background()

	user, err := testutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	items, err := feedops.GetRssFeedItems(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, items, 0, "Should return empty list for user with no items")
}
