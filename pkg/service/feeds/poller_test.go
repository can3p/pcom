package feeds

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	feedutil "github.com/can3p/pcom/pkg/feedops/testutil"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	. "github.com/ovechkin-dm/mockio/v2/mock"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// readItems is the user's reading list, read through the service.
func readItems(ctx context.Context, db boil.ContextExecutor, user *model.User) ([]*RssFeedItem, error) {
	return newService(repo.Using(db), nil, nil, nil).Items(ctx, user)
}

// fnameCapturingStorage wraps a server.MediaStorage and records the
// filename of every upload, so a test can look an upload back up through
// factory.GetMediaUploadByFname without knowing the generated filename in
// advance.
type fnameCapturingStorage struct {
	server.MediaStorage

	mu     sync.Mutex
	fnames []string
}

func (s *fnameCapturingStorage) UploadFile(ctx context.Context, fname string, b []byte, contentType string) error {
	s.mu.Lock()
	s.fnames = append(s.fnames, fname)
	s.mu.Unlock()

	return s.MediaStorage.UploadFile(ctx, fname, b, contentType)
}

func (s *fnameCapturingStorage) Fnames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, len(s.fnames))
	copy(out, s.fnames)

	return out
}

// TestTryFetchFeed exercises tryFetchFeed's two outcomes.
func TestTryFetchFeed(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	t.Run("fetch error saves failure, not returned", func(t *testing.T) {
		t.Parallel()

		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)

		ctrl := NewMockController(t)
		fetcherMock := Mock[fetcher](ctrl)
		cleanerMock := Mock[cleaner](ctrl)

		fetchErr := errors.New("boom")
		WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenReturn(nil, fetchErr)

		f := newService(repo.New(testDB.DB), fetcherMock, cleanerMock, nil)

		// A fetch failure must be recorded on the feed (via SaveFetchFailure)
		// rather than propagated, so one bad feed doesn't stop the poller
		// from trying the rest.
		require.NoError(t, f.tryFetchFeed(ctx, repo.Using(testDB.DB), feedRow))
		feedRow = testutil.Must(feedutil.GetRSSFeed(ctx, testDB.DB, feedRow.ID))(t)
		require.Equal(t, fetchErr.Error(), lo.FromPtr(feedRow.LastFetchError))
		require.NotNil(t, feedRow.NextFetchAt)
	})

	t.Run("fetch success saves feed", func(t *testing.T) {
		t.Parallel()

		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.WithoutTitle()))(t)

		ctrl := NewMockController(t)
		fetcherMock := Mock[fetcher](ctrl)
		cleanerMock := Mock[cleaner](ctrl)

		content := &reader.Feed{Title: "fetched title", Description: "fetched description"}
		WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenReturn(content, nil)
		WhenSingle(cleanerMock.CleanField(Any[string]())).ThenAnswer(func(args []any) string {
			return args[0].(string)
		})

		f := newService(repo.New(testDB.DB), fetcherMock, cleanerMock, nil)
		require.NoError(t, f.tryFetchFeed(ctx, repo.Using(testDB.DB), feedRow))

		// LockFeed doubles as the feed reader here: pkg/testutil/factory has
		// no RSSFeed reader.
		reloaded := testutil.Must(repo.Using(testDB.DB).LockFeed(ctx, feedRow.ID))(t)
		require.Empty(t, lo.FromPtr(reloaded.LastFetchError))
		require.NotNil(t, reloaded.LastFetchedAt)
		require.NotNil(t, reloaded.NextFetchAt, "a successful fetch schedules the next one")
		require.True(t, lo.FromPtr(reloaded.NextFetchAt).After(time.Now()), "the next fetch is scheduled in the future")
		require.Equal(t, "fetched title", lo.FromPtr(reloaded.Title), "a feed with no title yet should get the fetched title persisted")
	})
}

// TestGetFeedsToRefresh_RespectsNextFetchAt pins the "due" filter directly:
// a feed scheduled in the future is not returned, one scheduled in the past
// is, and a due feed with no subscribers is filtered out either way.
func TestGetFeedsToRefresh_RespectsNextFetchAt(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, testDB.DB))(t)

	future := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(time.Hour))))(t)
	testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, future.ID))(t)

	past := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour))))(t)
	testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, past.ID))(t)

	// Due in the past, but nobody subscribes to it: GetFeedsToRefresh filters
	// out feeds with no subscribers even when they're otherwise due.
	pastNoSubscribers := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour))))(t)

	feeds := testutil.Must(repo.Using(testDB.DB).FeedsToRefresh(ctx))(t)

	ids := make([]string, len(feeds))
	for i, f := range feeds {
		ids[i] = f.ID
	}

	require.Contains(t, ids, past.ID, "a feed due in the past should be refreshed")
	require.NotContains(t, ids, future.ID, "a feed due in the future should not be refreshed")
	require.NotContains(t, ids, pastNoSubscribers.ID, "a due feed with no subscribers should not be refreshed")
}

// TestRefreshFeeds exercises the poller's top-level loop. refreshFeeds scans
// every due feed in the database it's given, so each subtest needs its own
// database: sharing one would let one subtest's feeder process another
// subtest's feed row.
func TestRefreshFeeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("processes due feeds end to end", func(t *testing.T) {
		t.Parallel()

		testDB := testdb.New(t)
		user := testutil.Must(factory.User(ctx, testDB.DB))(t)
		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour)), factory.WithoutTitle()))(t)
		testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, feedRow.ID))(t)

		ctrl := NewMockController(t)
		fetcherMock := Mock[fetcher](ctrl)
		cleanerMock := Mock[cleaner](ctrl)

		content := &reader.Feed{Title: "fetched title", Description: "fetched description"}
		WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenReturn(content, nil)
		WhenSingle(cleanerMock.CleanField(Any[string]())).ThenAnswer(func(args []any) string {
			return args[0].(string)
		})

		f := newService(repo.New(testDB.DB), fetcherMock, cleanerMock, nil)
		require.NoError(t, f.refreshFeeds(ctx))

		// feedRow was scheduled an hour in the past to be due; a processed
		// feed must come out rescheduled into the future, not left at (or
		// near) that past due time.
		reloaded := testutil.Must(repo.Using(testDB.DB).LockFeed(ctx, feedRow.ID))(t)
		require.Empty(t, lo.FromPtr(reloaded.LastFetchError))
		require.NotNil(t, reloaded.LastFetchedAt)
		require.NotNil(t, reloaded.NextFetchAt)
		require.True(t, lo.FromPtr(reloaded.NextFetchAt).After(time.Now()), "a processed feed is rescheduled into the future")
		require.Equal(t, "fetched title", lo.FromPtr(reloaded.Title), "a feed with no title yet should get the fetched title persisted")
	})

	t.Run("recovers from a panic in per-feed processing", func(t *testing.T) {
		t.Parallel()

		testDB := testdb.New(t)
		user := testutil.Must(factory.User(ctx, testDB.DB))(t)
		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)
		testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, feedRow.ID))(t)

		ctrl := NewMockController(t)
		fetcherMock := Mock[fetcher](ctrl)
		cleanerMock := Mock[cleaner](ctrl)

		WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenAnswer(func(args []any) (*reader.Feed, error) {
			panic("boom")
		})

		f := newService(repo.New(testDB.DB), fetcherMock, cleanerMock, nil)

		// A panic anywhere in the per-feed processing (a bad fetcher, a bug
		// in a cleaner) must not crash the scheduler; it comes back as an
		// error instead.
		err := f.refreshFeeds(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "refreshFeeds panicked")
	})

	t.Run("propagates a GetFeedsToRefresh error", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)

		require.NoError(t, db.DB.Close())

		f := newService(repo.New(db.DB), nil, nil, nil)

		err := f.refreshFeeds(ctx)
		require.Error(t, err)
	})
}

// TestRunPoller_StopsOnContextDone pins the poller's shutdown path: it must
// return as soon as its context is done, without waiting for the next tick.
func TestRunPoller_StopsOnContextDone(t *testing.T) {
	t.Parallel()

	f := newService(nil, nil, nil, nil)

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

	_, err := repo.Using(testDB.DB).LockFeed(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
}

// TestSaveFeedItem exercises SaveFeedItem's guard, dedupe, error-fallback and
// image-upload behaviors.
func TestSaveFeedItem(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	t.Run("refuses an item without a URL", func(t *testing.T) {
		t.Parallel()

		// It has nothing to dedupe on, so it must be rejected rather than
		// silently stored.
		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)
		_, err := newService(repo.Using(testDB.DB), nil, nil, nil).saveFeedItem(ctx, repo.Using(testDB.DB), feedRow.ID, &reader.Item{URL: ""}, nil)
		require.Error(t, err)
	})

	t.Run("duplicate URL is not new", func(t *testing.T) {
		t.Parallel()

		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)

		ctrl := NewMockController(t)
		cleanerMock := Mock[cleaner](ctrl)
		WhenDouble(cleanerMock.HTMLToMarkdown(Any[string]())).ThenAnswer(func(args []any) (string, error) {
			return args[0].(string), nil
		})

		item := &reader.Item{URL: "https://example.com/dup", Title: "t", Summary: "s"}

		// Saving the same item URL twice upserts onto the same row instead
		// of creating a second one, and only the first call reports it as
		// new.
		isNew := testutil.Must(newService(repo.Using(testDB.DB), nil, cleanerMock, nil).saveFeedItem(ctx, repo.Using(testDB.DB), feedRow.ID, item, nil))(t)
		require.True(t, isNew)

		isNew = testutil.Must(newService(repo.Using(testDB.DB), nil, cleanerMock, nil).saveFeedItem(ctx, repo.Using(testDB.DB), feedRow.ID, item, nil))(t)
		require.False(t, isNew, "the same url upserts onto the existing item instead of creating a new one")
	})

	t.Run("HTMLToMarkdown error is recorded, item still stored", func(t *testing.T) {
		t.Parallel()

		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)

		ctrl := NewMockController(t)
		cleanerMock := Mock[cleaner](ctrl)
		boom := errors.New("boom")
		WhenDouble(cleanerMock.HTMLToMarkdown(Any[string]())).ThenReturn("", boom)

		item := &reader.Item{URL: "https://example.com/broken", Title: "t", Summary: "s"}

		isNew := testutil.Must(newService(repo.Using(testDB.DB), nil, cleanerMock, nil).saveFeedItem(ctx, repo.Using(testDB.DB), feedRow.ID, item, nil))(t)
		require.True(t, isNew)

		items := testutil.Must(factory.ListRSSItems(ctx, testDB.DB, feedRow.ID))(t)
		require.Len(t, items, 1)
		require.Equal(t, fmt.Sprintf("Summary errors: %s", boom.Error()), items[0].SanitizedDescription, "the cleaning error should be recorded as the item's body")
	})

	t.Run("uploads referenced images", func(t *testing.T) {
		t.Parallel()

		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)

		ctrl := NewMockController(t)
		fetcherMock := Mock[fetcher](ctrl)
		cleanerMock := Mock[cleaner](ctrl)

		imgURL := "https://img.example.test/pic.png"
		WhenDouble(cleanerMock.HTMLToMarkdown(Any[string]())).ThenReturn(fmt.Sprintf("![alt](%s)", imgURL), nil)

		pngBytes := []byte("\x89PNG\r\n\x1a\nrest-of-file")
		WhenDouble(fetcherMock.FetchMedia(Any[context.Context](), Any[string]())).ThenAnswer(func(args []any) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(pngBytes)), nil
		})

		storage := &fnameCapturingStorage{MediaStorage: fakestorage.New()}
		item := &reader.Item{URL: "https://example.com/with-image", Title: "t", Summary: "s"}

		isNew := testutil.Must(newService(repo.Using(testDB.DB), fetcherMock, cleanerMock, storage).saveFeedItem(ctx, repo.Using(testDB.DB), feedRow.ID, item, nil))(t)
		require.True(t, isNew)

		fnames := storage.Fnames()
		require.Len(t, fnames, 1, "the image referenced in the body should be uploaded exactly once")
		fname := fnames[0]

		exists := testutil.Must(storage.ObjectExists(ctx, fname))(t)
		require.True(t, exists, "the uploaded image should exist in storage")

		upload := testutil.Must(factory.GetMediaUploadByFname(ctx, testDB.DB, fname))(t)
		require.NotNil(t, upload.RSSFeedID)
		require.Equal(t, feedRow.ID, lo.FromPtr(upload.RSSFeedID), "the upload should be attributed to the feed it was fetched for")

		items := testutil.Must(factory.ListRSSItems(ctx, testDB.DB, feedRow.ID))(t)
		require.Len(t, items, 1)
		require.NotContains(t, items[0].SanitizedDescription, imgURL, "the stored body should no longer reference the original image URL")
	})
}

func TestSaveFetchFailure(t *testing.T) {
	testDB := testdb.New(t)

	ctx := context.Background()

	feed, err := feedutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed", "Test Feed")
	require.NoError(t, err)

	testError := assert.AnError

	err = saveFetchFailure(ctx, repo.Using(testDB.DB), feed, testError)
	require.NoError(t, err)

	updatedFeed, err := feedutil.GetRSSFeed(ctx, testDB.DB, feed.ID)
	require.NoError(t, err)

	assert.Equal(t, testError.Error(), lo.FromPtr(updatedFeed.LastFetchError))
	assert.Equal(t, 0, updatedFeed.LastItemsCount)
	assert.NotNil(t, updatedFeed.NextFetchAt)
	assert.NotNil(t, updatedFeed.LastFetchedAt)
}

func TestLockFeed(t *testing.T) {
	testDB := testdb.New(t)

	ctx := context.Background()

	feed, err := feedutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed", "Test Feed")
	require.NoError(t, err)

	lockedFeed, err := repo.Using(testDB.DB).LockFeed(ctx, feed.ID)
	require.NoError(t, err)
	assert.Equal(t, feed.ID, lockedFeed.ID)
	assert.Equal(t, feed.URL, lockedFeed.URL)
}

func createFeedItems(num int, startTime time.Time) []*reader.Item {
	result := make([]*reader.Item, num)
	for idx := range num {
		n := strconv.Itoa(num - 1 - idx)
		result[idx] = &reader.Item{
			URL:         "https://example.com/post" + n,
			Title:       "Test Post " + n,
			Summary:     "Summary of test post " + n,
			PublishedAt: new(startTime.Add(-time.Duration(idx) * time.Hour)),
		}
	}

	return result
}

func TestSaveFeed(t *testing.T) {
	testDB := testdb.New(t)

	ctrl := NewMockController(t)

	ctx := context.Background()

	user, err := feedutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	pastTime := time.Now().Add(-1 * time.Hour)

	feed1, err := feedutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed1", "Feed 1")
	require.NoError(t, err)
	feed1.NextFetchAt = new(pastTime)
	err = repo.Using(testDB.DB).SaveFeed(ctx, feed1)
	require.NoError(t, err)

	_, err = feedutil.CreateUserFeedSubscription(ctx, testDB.DB, user.ID, feed1.ID)
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

	err = newService(repo.Using(testDB.DB), fetcher, cleaner, nil).saveFeed(ctx, repo.Using(testDB.DB), feed1, feedContent)
	require.NoError(t, err)

	fetchedFeeds, err := readItems(ctx, testDB.DB, user)
	require.NoError(t, err)
	require.Len(t, fetchedFeeds, 2)

	// Verify the order (newest first)
	require.Equal(t, "https://example.com/post1", fetchedFeeds[0].URL)
	require.Equal(t, "https://example.com/post0", fetchedFeeds[1].URL)

	// feed items are actually sorted by AddedAt field
	require.True(t, fetchedFeeds[0].AddedAt.After(fetchedFeeds[1].AddedAt))
}

func TestSaveFeedInitialAndFollowUp(t *testing.T) {
	testDB := testdb.New(t)

	ctrl := NewMockController(t)

	ctx := context.Background()

	user, err := feedutil.CreateUser(ctx, testDB.DB, "test@example.com")
	require.NoError(t, err)

	pastTime := time.Now().Add(-1 * time.Hour)

	feed1, err := feedutil.CreateRSSFeed(ctx, testDB.DB, "https://example.com/feed1", "Feed 1")
	require.NoError(t, err)
	feed1.LastFetchError = new("error fetching")
	feed1.NextFetchAt = new(pastTime)
	err = repo.Using(testDB.DB).SaveFeed(ctx, feed1)
	require.NoError(t, err)

	_, err = feedutil.CreateUserFeedSubscription(ctx, testDB.DB, user.ID, feed1.ID)
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

	err = newService(repo.Using(testDB.DB), fetcher, cleaner, nil).saveFeed(ctx, repo.Using(testDB.DB), feed1, feedContent)
	require.NoError(t, err)

	feed1, err = feedutil.GetRSSFeed(ctx, testDB.DB, feed1.ID)
	require.NoError(t, err)

	assert.Empty(t, lo.FromPtr(feed1.LastFetchError), "successful fetch should erase any previous error")

	fetchedFeeds, err := readItems(ctx, testDB.DB, user)
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

	err = newService(repo.Using(testDB.DB), fetcher, cleaner, nil).saveFeed(ctx, repo.Using(testDB.DB), feed1, feedContent)
	require.NoError(t, err)

	fetchedFeeds, err = readItems(ctx, testDB.DB, user)
	require.NoError(t, err)
	require.Len(t, fetchedFeeds, 6)
	require.Equal(t, "https://example.com/post100", fetchedFeeds[0].URL)
	require.Equal(t, "https://example.com/post9", fetchedFeeds[1].URL)
	require.Equal(t, "https://example.com/post8", fetchedFeeds[2].URL)
	require.Equal(t, "https://example.com/post7", fetchedFeeds[3].URL)
	require.Equal(t, "https://example.com/post6", fetchedFeeds[4].URL)
	require.Equal(t, "https://example.com/post5", fetchedFeeds[5].URL)

}
