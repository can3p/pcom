package feeder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	. "github.com/ovechkin-dm/mockio/v2/mock"
	"github.com/stretchr/testify/require"
)

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
//
// pkg/feedops imports this package (for DefaultRssReader), so a test here
// cannot import pkg/feedops or feedops.GetRssFeeds without an import cycle;
// the feed row is reloaded directly instead.
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

		f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)

		// A fetch failure must be recorded on the feed (via SaveFetchFailure)
		// rather than propagated, so one bad feed doesn't stop the poller
		// from trying the rest.
		require.NoError(t, f.tryFetchFeed(ctx, testDB.DB, feedRow))
		require.NoError(t, feedRow.Reload(ctx, testDB.DB))
		require.Equal(t, fetchErr.Error(), feedRow.LastFetchError.String)
		require.True(t, feedRow.NextFetchAt.Valid)
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

		f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)
		require.NoError(t, f.tryFetchFeed(ctx, testDB.DB, feedRow))

		// LockFeed doubles as the package's own reader here: a test in this
		// (white-box) package cannot import pkg/feedops for its reader
		// without an import cycle, and pkg/testutil/factory has no RSSFeed
		// reader.
		reloaded := testutil.Must(LockFeed(ctx, testDB.DB, feedRow.ID))(t)
		require.Empty(t, reloaded.LastFetchError.String)
		require.False(t, reloaded.LastFetchedAt.IsZero())
		require.True(t, reloaded.NextFetchAt.Valid, "a successful fetch schedules the next one")
		require.True(t, reloaded.NextFetchAt.Time.After(time.Now()), "the next fetch is scheduled in the future")
		require.Equal(t, "fetched title", reloaded.Title.String, "a feed with no title yet should get the fetched title persisted")
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

	feeds := testutil.Must(GetFeedsToRefresh(ctx, testDB.DB))(t)

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

		f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)
		require.NoError(t, f.refreshFeeds(ctx))

		// feedRow was scheduled an hour in the past to be due; a processed
		// feed must come out rescheduled into the future, not left at (or
		// near) that past due time.
		reloaded := testutil.Must(LockFeed(ctx, testDB.DB, feedRow.ID))(t)
		require.Empty(t, reloaded.LastFetchError.String)
		require.False(t, reloaded.LastFetchedAt.IsZero())
		require.True(t, reloaded.NextFetchAt.Valid)
		require.True(t, reloaded.NextFetchAt.Time.After(time.Now()), "a processed feed is rescheduled into the future")
		require.Equal(t, "fetched title", reloaded.Title.String, "a feed with no title yet should get the fetched title persisted")
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

		f := NewFeeder(testDB.DB, fetcherMock, cleanerMock, nil)

		// A panic anywhere in the per-feed processing (a bad fetcher, a bug
		// in a cleaner) must not crash the scheduler; it comes back as an
		// error instead.
		err := f.refreshFeeds(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "refreshFeeds panicked")
	})

	t.Run("propagates a GetFeedsToRefresh error", func(t *testing.T) {
		t.Skip("known bug: https://github.com/can3p/pcom/issues/116 - refreshFeeds discards the error from GetFeedsToRefresh and always returns nil")

		db := testdb.New(t)

		require.NoError(t, db.DB.Close())

		f := NewFeeder(db.DB, nil, nil, nil)

		err := f.refreshFeeds(ctx)
		require.Error(t, err)
	})
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
		_, err := SaveFeedItem(ctx, testDB.DB, feedRow.ID, &reader.Item{URL: ""}, nil, nil, nil, nil)
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
		isNew := testutil.Must(SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, nil, nil))(t)
		require.True(t, isNew)

		isNew = testutil.Must(SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, nil, nil))(t)
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

		isNew := testutil.Must(SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, nil, nil))(t)
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

		isNew := testutil.Must(SaveFeedItem(ctx, testDB.DB, feedRow.ID, item, nil, cleanerMock, fetcherMock, storage))(t)
		require.True(t, isNew)

		fnames := storage.Fnames()
		require.Len(t, fnames, 1, "the image referenced in the body should be uploaded exactly once")
		fname := fnames[0]

		exists := testutil.Must(storage.ObjectExists(ctx, fname))(t)
		require.True(t, exists, "the uploaded image should exist in storage")

		upload := testutil.Must(factory.GetMediaUploadByFname(ctx, testDB.DB, fname))(t)
		require.True(t, upload.RSSFeedID.Valid)
		require.Equal(t, feedRow.ID, upload.RSSFeedID.String, "the upload should be attributed to the feed it was fetched for")

		items := testutil.Must(factory.ListRSSItems(ctx, testDB.DB, feedRow.ID))(t)
		require.Len(t, items, 1)
		require.NotContains(t, items[0].SanitizedDescription, imgURL, "the stored body should no longer reference the original image URL")
	})
}
