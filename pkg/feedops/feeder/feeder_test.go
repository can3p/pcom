package feeder_test

import (
	"context"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/feedops/feeder"
	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/feedops/testutil"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/testcontainers/postgres"
	. "github.com/ovechkin-dm/mockio/v2/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func TestGetFeedsToRefresh(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer func() { _ = testDB.Close() }()

	ctx := context.Background()

	user, err := testutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	pastTime := time.Now().Add(-1 * time.Hour)
	futureTime := time.Now().Add(1 * time.Hour)

	feed1, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed1", "Feed 1")
	require.NoError(t, err)
	feed1.NextFetchAt = null.TimeFrom(pastTime)
	_, err = feed1.Update(ctx, testDB.DB, boil.Infer())
	require.NoError(t, err)

	feed2, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed2", "Feed 2")
	require.NoError(t, err)
	feed2.NextFetchAt = null.TimeFrom(futureTime)
	_, err = feed2.Update(ctx, testDB.DB, boil.Infer())
	require.NoError(t, err)

	feed3, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed3", "Feed 3")
	require.NoError(t, err)
	feed3.NextFetchAt = null.TimeFrom(pastTime)
	_, err = feed3.Update(ctx, testDB.DB, boil.Infer())
	require.NoError(t, err)

	_, err = testutil.CreateUserFeedSubscription(ctx, testDB.DB, user.ID, feed1.ID)
	require.NoError(t, err)

	_, err = testutil.CreateUserFeedSubscription(ctx, testDB.DB, user.ID, feed2.ID)
	require.NoError(t, err)

	feeds, err := feeder.GetFeedsToRefresh(ctx, testDB.DB)
	require.NoError(t, err)

	require.Len(t, feeds, 1, "Should only return feed1 (past time and has subscription)")
	assert.Equal(t, feed1.ID, feeds[0].ID)
}

func TestSaveFetchFailure(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer func() { _ = testDB.Close() }()

	ctx := context.Background()

	feed, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed", "Test Feed")
	require.NoError(t, err)

	testError := assert.AnError

	err = feeder.SaveFetchFailure(ctx, testDB.DB, feed, testError)
	require.NoError(t, err)

	updatedFeed, err := testutil.GetRSSFeed(ctx, testDB.DB, feed.ID)
	require.NoError(t, err)

	assert.Equal(t, testError.Error(), updatedFeed.LastFetchError.String)
	assert.Equal(t, 0, updatedFeed.LastItemsCount)
	assert.False(t, updatedFeed.NextFetchAt.IsZero())
	assert.False(t, updatedFeed.LastFetchedAt.IsZero())
}

func TestLockFeed(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer func() { _ = testDB.Close() }()

	ctx := context.Background()

	feed, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed", "Test Feed")
	require.NoError(t, err)

	lockedFeed, err := feeder.LockFeed(ctx, testDB.DB, feed.ID)
	require.NoError(t, err)
	assert.Equal(t, feed.ID, lockedFeed.ID)
	assert.Equal(t, feed.URL, lockedFeed.URL)
}

type fetcher interface {
	Fetch(urL string) (*reader.Feed, error)
	FetchMedia(ctx context.Context, mediaURL string) (io.ReadCloser, error)
}

type cleaner interface {
	CleanField(in string) string
	HTMLToMarkdown(in string) (string, error)
}

func createFeedItems(num int, startTime time.Time) []*reader.Item {
	result := make([]*reader.Item, num)
	for idx := range num {
		n := strconv.Itoa(num - 1 - idx)
		result[idx] = &reader.Item{
			URL:         "https://example.com/post" + n,
			Title:       "Test Post " + n,
			Summary:     "Summary of test post " + n,
			PublishedAt: util.Pointer(startTime.Add(-time.Duration(idx) * time.Hour)),
		}
	}

	return result
}

func TestSaveFeed(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer func() { _ = testDB.Close() }()

	ctrl := NewMockController(t)

	ctx := context.Background()

	user, err := testutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	pastTime := time.Now().Add(-1 * time.Hour)

	feed1, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed1", "Feed 1")
	require.NoError(t, err)
	feed1.NextFetchAt = null.TimeFrom(pastTime)
	_, err = feed1.Update(ctx, testDB.DB, boil.Infer())
	require.NoError(t, err)

	_, err = testutil.CreateUserFeedSubscription(ctx, testDB.DB, user.ID, feed1.ID)
	require.NoError(t, err)

	feedContent := &reader.Feed{
		Title:       "test feed",
		Description: "test feed description",
		Items:       createFeedItems(2, time.Now()),
	}

	fetcher := Mock[fetcher](ctrl)
	cleaner := Mock[cleaner](ctrl)

	WhenDouble(cleaner.HTMLToMarkdown(Any[string]())).ThenAnswer(func(args []any) (string, error) {
		return args[0].(string), nil
	})

	err = feeder.SaveFeed(ctx, testDB.DB, feed1, feedContent, cleaner, fetcher, nil)
	require.NoError(t, err)

	fetchedFeeds, err := feedops.GetRssFeedItems(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, fetchedFeeds, 2)

	// Verify the order (newest first)
	require.Equal(t, "https://example.com/post1", fetchedFeeds[0].URL)
	require.Equal(t, "https://example.com/post0", fetchedFeeds[1].URL)

	// feed items are actually sorted by AddedAt field
	require.True(t, fetchedFeeds[0].AddedAt.After(fetchedFeeds[1].AddedAt))
}

func TestSaveFeedInitialAndFollowUp(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer func() { _ = testDB.Close() }()

	ctrl := NewMockController(t)

	ctx := context.Background()

	user, err := testutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	pastTime := time.Now().Add(-1 * time.Hour)

	feed1, err := testutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed1", "Feed 1")
	require.NoError(t, err)
	feed1.NextFetchAt = null.TimeFrom(pastTime)
	_, err = feed1.Update(ctx, testDB.DB, boil.Infer())
	require.NoError(t, err)

	_, err = testutil.CreateUserFeedSubscription(ctx, testDB.DB, user.ID, feed1.ID)
	require.NoError(t, err)

	n := time.Now()

	feedContent := &reader.Feed{
		Title:       "test feed",
		Description: "test feed description",
		Items:       createFeedItems(10, n),
	}

	fetcher := Mock[fetcher](ctrl)
	cleaner := Mock[cleaner](ctrl)

	WhenDouble(cleaner.HTMLToMarkdown(Any[string]())).ThenAnswer(func(args []any) (string, error) {
		return args[0].(string), nil
	})

	err = feeder.SaveFeed(ctx, testDB.DB, feed1, feedContent, cleaner, fetcher, nil)
	require.NoError(t, err)

	fetchedFeeds, err := feedops.GetRssFeedItems(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, fetchedFeeds, 5)

	// Verify the order (newest first)
	require.Equal(t, "https://example.com/post9", fetchedFeeds[0].URL)
	require.Equal(t, "https://example.com/post8", fetchedFeeds[1].URL)
	require.Equal(t, "https://example.com/post7", fetchedFeeds[2].URL)
	require.Equal(t, "https://example.com/post6", fetchedFeeds[3].URL)
	require.Equal(t, "https://example.com/post5", fetchedFeeds[4].URL)

	newItems := []*reader.Item{
		{
			Title:   "fresh item",
			URL:     "https://example.com/post100",
			Summary: "post 100",
		},
	}

	newItems = append(newItems, feedContent.Items...)
	feedContent = &reader.Feed{
		Title:       "test feed",
		Description: "test feed description",
		Items:       newItems,
	}

	err = feeder.SaveFeed(ctx, testDB.DB, feed1, feedContent, cleaner, fetcher, nil)
	require.NoError(t, err)

	fetchedFeeds, err = feedops.GetRssFeedItems(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, fetchedFeeds, 6)
	require.Equal(t, "https://example.com/post100", fetchedFeeds[0].URL)
	require.Equal(t, "https://example.com/post9", fetchedFeeds[1].URL)
	require.Equal(t, "https://example.com/post8", fetchedFeeds[2].URL)
	require.Equal(t, "https://example.com/post7", fetchedFeeds[3].URL)
	require.Equal(t, "https://example.com/post6", fetchedFeeds[4].URL)
	require.Equal(t, "https://example.com/post5", fetchedFeeds[5].URL)

}
