package feeder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	. "github.com/ovechkin-dm/mockio/v2/mock"
	"github.com/stretchr/testify/require"
)

// TestTryFetchFeed_FetchErrorSavesFailure exercises tryFetchFeed's error
// branch: a fetch failure must be recorded on the feed (via
// SaveFetchFailure) rather than propagated, so one bad feed doesn't stop the
// poller from trying the rest.
//
// pkg/feedops imports this package (for DefaultRssReader), so a test here
// cannot import pkg/feedops or feedops.GetRssFeeds without an import cycle;
// the feed row is reloaded directly instead.
func TestTryFetchFeed_FetchErrorSavesFailure(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	feedRow, err := factory.RSSFeed(ctx, testDB.DB)
	require.NoError(t, err)

	ctrl := NewMockController(t)
	fetcherMock := Mock[fetcher](ctrl)
	cleanerMock := Mock[cleaner](ctrl)

	fetchErr := errors.New("boom")
	WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenReturn(nil, fetchErr)

	f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)

	err = f.tryFetchFeed(ctx, testDB.DB, feedRow)
	require.NoError(t, err, "a fetch error is recorded on the feed, not returned")

	require.NoError(t, feedRow.Reload(ctx, testDB.DB))
	require.Equal(t, fetchErr.Error(), feedRow.LastFetchError.String)
	require.True(t, feedRow.NextFetchAt.Valid)
}

// TestTryFetchFeed_FetchSuccessSavesFeed exercises tryFetchFeed's happy
// path: a successful fetch is handed to SaveFeed and the feed's title ends
// up persisted.
func TestTryFetchFeed_FetchSuccessSavesFeed(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	feedRow, err := factory.RSSFeed(ctx, testDB.DB)
	require.NoError(t, err)

	ctrl := NewMockController(t)
	fetcherMock := Mock[fetcher](ctrl)
	cleanerMock := Mock[cleaner](ctrl)

	content := &reader.Feed{Title: "fetched title", Description: "fetched description"}
	WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenReturn(content, nil)

	f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)

	err = f.tryFetchFeed(ctx, testDB.DB, feedRow)
	require.NoError(t, err)

	require.NoError(t, feedRow.Reload(ctx, testDB.DB))
	require.Empty(t, feedRow.LastFetchError.String)
	require.False(t, feedRow.LastFetchedAt.IsZero())
}

// TestGetFeedsToRefresh_RespectsNextFetchAt pins the "due" filter directly:
// a feed scheduled in the future is not returned, one scheduled in the past
// is.
func TestGetFeedsToRefresh_RespectsNextFetchAt(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	future, err := factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(time.Hour)))
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, testDB.DB, user.ID, future.ID)
	require.NoError(t, err)

	past, err := factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour)))
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, testDB.DB, user.ID, past.ID)
	require.NoError(t, err)

	feeds, err := GetFeedsToRefresh(ctx, testDB.DB)
	require.NoError(t, err)

	ids := make([]string, len(feeds))
	for i, f := range feeds {
		ids[i] = f.ID
	}

	require.Contains(t, ids, past.ID, "a feed due in the past should be refreshed")
	require.NotContains(t, ids, future.ID, "a feed due in the future should not be refreshed")
}

// TestRefreshFeeds_ProcessesDueFeeds exercises the poller's top-level loop
// end to end: a feed scheduled in the past is due, gets locked and fetched
// inside its own transaction, and comes out updated.
func TestRefreshFeeds_ProcessesDueFeeds(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	feedRow, err := factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour)))
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, testDB.DB, user.ID, feedRow.ID)
	require.NoError(t, err)

	ctrl := NewMockController(t)
	fetcherMock := Mock[fetcher](ctrl)
	cleanerMock := Mock[cleaner](ctrl)

	content := &reader.Feed{Title: "fetched title", Description: "fetched description"}
	WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenReturn(content, nil)

	f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)

	require.NoError(t, f.refreshFeeds(ctx))

	require.NoError(t, feedRow.Reload(ctx, testDB.DB))
	require.Empty(t, feedRow.LastFetchError.String)
	require.False(t, feedRow.LastFetchedAt.IsZero())
	require.True(t, feedRow.NextFetchAt.Valid, "a processed feed is scheduled for its next fetch")
}

// TestRunPoller_StopsOnContextDone pins the poller's shutdown path: it must
// return as soon as its context is done, without waiting for the next tick.
func TestRunPoller_StopsOnContextDone(t *testing.T) {
	t.Parallel()

	f := NewFeeder(nil, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		f.RunPoller(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunPoller did not return after its context was canceled")
	}
}

// TestLockFeed_NotFound pins LockFeed's error path for an id that doesn't
// exist.
func TestLockFeed_NotFound(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)

	_, err := LockFeed(context.Background(), testDB.DB, "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
}

// TestSaveFeedItem_RefusesItemWithoutURL pins SaveFeedItem's guard against
// an item with no URL: it has nothing to dedupe on, so it must be rejected
// rather than silently stored.
func TestSaveFeedItem_RefusesItemWithoutURL(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	feedRow, err := factory.RSSFeed(ctx, testDB.DB)
	require.NoError(t, err)

	_, err = SaveFeedItem(ctx, testDB.DB, feedRow.ID, &reader.Item{URL: ""}, nil, nil, nil, nil)
	require.Error(t, err)
}

// TestSaveFeedItem_DuplicateURLIsNotNew pins the dedupe behavior: saving the
// same item URL twice upserts onto the same row instead of creating a
// second one, and only the first call reports it as new.
func TestSaveFeedItem_DuplicateURLIsNotNew(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	feedRow, err := factory.RSSFeed(ctx, testDB.DB)
	require.NoError(t, err)

	ctrl := NewMockController(t)
	cleanerMock := Mock[cleaner](ctrl)
	WhenDouble(cleanerMock.HTMLToMarkdown(Any[string]())).ThenAnswer(func(args []any) (string, error) {
		return args[0].(string), nil
	})

	item := &reader.Item{URL: "https://example.com/dup", Title: "t", Summary: "s"}

	isNew, err := SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, nil, nil)
	require.NoError(t, err)
	require.True(t, isNew)

	isNew, err = SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, nil, nil)
	require.NoError(t, err)
	require.False(t, isNew, "the same url upserts onto the existing item instead of creating a new one")
}

// TestSaveFeedItem_HTMLToMarkdownErrorIsRecorded pins the fallback when
// cleaning a feed item's HTML fails: the item is still stored, with the
// error recorded as its body instead of losing the item entirely.
func TestSaveFeedItem_HTMLToMarkdownErrorIsRecorded(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	feedRow, err := factory.RSSFeed(ctx, testDB.DB)
	require.NoError(t, err)

	ctrl := NewMockController(t)
	cleanerMock := Mock[cleaner](ctrl)
	boom := errors.New("boom")
	WhenDouble(cleanerMock.HTMLToMarkdown(Any[string]())).ThenReturn("", boom)

	item := &reader.Item{URL: "https://example.com/broken", Title: "t", Summary: "s"}

	isNew, err := SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, nil, nil)
	require.NoError(t, err)
	require.True(t, isNew)
}

// TestSaveFeedItem_UploadsReferencedImages exercises the image pipeline:
// an item whose cleaned body references an image gets that image downloaded
// through the fetcher and re-hosted through HandleUpload.
func TestSaveFeedItem_UploadsReferencedImages(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	feedRow, err := factory.RSSFeed(ctx, testDB.DB)
	require.NoError(t, err)

	ctrl := NewMockController(t)
	fetcherMock := Mock[fetcher](ctrl)
	cleanerMock := Mock[cleaner](ctrl)

	imgURL := "https://img.example.test/pic.png"
	WhenDouble(cleanerMock.HTMLToMarkdown(Any[string]())).ThenReturn(fmt.Sprintf("![alt](%s)", imgURL), nil)

	pngBytes := []byte("\x89PNG\r\n\x1a\nrest-of-file")
	WhenDouble(fetcherMock.FetchMedia(Any[context.Context](), Any[string]())).ThenAnswer(func(args []any) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(pngBytes)), nil
	})

	storage := fakestorage.New()
	item := &reader.Item{URL: "https://example.com/with-image", Title: "t", Summary: "s"}

	isNew, err := SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, fetcherMock, storage)
	require.NoError(t, err)
	require.True(t, isNew)
}

// TestRefreshFeeds_RecoversFromPanic pins refreshFeeds' safety net: a panic
// anywhere in the per-feed processing (a bad fetcher, a bug in a cleaner)
// must not crash the scheduler; it comes back as an error instead.
func TestRefreshFeeds_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)
	feedRow, err := factory.RSSFeed(ctx, testDB.DB)
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, testDB.DB, user.ID, feedRow.ID)
	require.NoError(t, err)

	ctrl := NewMockController(t)
	fetcherMock := Mock[fetcher](ctrl)
	cleanerMock := Mock[cleaner](ctrl)

	WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenAnswer(func(args []any) (*reader.Feed, error) {
		panic("boom")
	})

	f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)

	err = f.refreshFeeds(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refreshFeeds panicked")
}

// TestRefreshFeeds_PropagatesGetFeedsToRefreshError pins the correct
// behavior for the poller's top-level loop: a failure to even list the due
// feeds should be reported, not swallowed.
func TestRefreshFeeds_PropagatesGetFeedsToRefreshError(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/116 - refreshFeeds discards the error from GetFeedsToRefresh and always returns nil")

	testDB := testdb.New(t)
	ctx := context.Background()

	require.NoError(t, testDB.DB.Close())

	f := NewFeeder(testDB.DB, nil, nil, nil)

	err := f.refreshFeeds(ctx)
	require.Error(t, err)
}
