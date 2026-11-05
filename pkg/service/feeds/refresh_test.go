package feeds

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	feedutil "github.com/can3p/pcom/pkg/feedops/testutil"
	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	. "github.com/ovechkin-dm/mockio/v2/mock"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readItems is the user's reading list, read through the service.
func readItems(ctx context.Context, db repo.Executor, user *model.User) ([]*RssFeedItem, error) {
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

// TestRefresh exercises the refresh worker's outcomes other than a
// successful fetch, which TestScheduleAndRefresh covers.
func TestRefresh(t *testing.T) {
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

		// A fetch failure is recorded on the feed rather than propagated: the
		// job completes, and the feed's next_fetch_at is its backoff.
		require.NoError(t, f.Refresh(ctx, RefreshArgs{FeedID: feedRow.ID}))

		reloaded := testutil.Must(repo.Using(testDB.DB).FeedByID(ctx, feedRow.ID))(t)
		require.Equal(t, fetchErr.Error(), lo.FromPtr(reloaded.LastFetchError))
		require.Equal(t, 0, reloaded.LastItemsCount)
		require.NotNil(t, reloaded.NextFetchAt)
		require.WithinDuration(t, time.Now().Add(reader.ErrorFetchInterval), *reloaded.NextFetchAt, 5*time.Second)
		require.NotNil(t, reloaded.LastFetchedAt)
		require.WithinDuration(t, time.Now(), *reloaded.LastFetchedAt, 5*time.Second)
	})

	t.Run("a deleted feed is skipped", func(t *testing.T) {
		t.Parallel()

		// a nil fetcher panics if called, and the panic comes back as an error
		f := newService(repo.New(testDB.DB), nil, nil, nil)
		require.NoError(t, f.Refresh(ctx, RefreshArgs{FeedID: uuid.NewString()}))
	})

	t.Run("recovers from a panic in per-feed processing", func(t *testing.T) {
		t.Parallel()

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
		// in a cleaner) must not crash the runner; it comes back as an
		// error instead.
		err := f.Refresh(ctx, RefreshArgs{FeedID: feedRow.ID})
		require.Error(t, err)
		require.Contains(t, err.Error(), "refreshing feed "+feedRow.ID+" panicked")
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

// TestScheduleAndRefresh exercises the schedule job and the refresh jobs it
// queues. ScheduleRefreshes scans every due feed in the database it's given,
// so each subtest needs its own database: sharing one would let one
// subtest's schedule queue another subtest's feed row.
func TestScheduleAndRefresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("processes due feeds end to end", func(t *testing.T) {
		t.Parallel()

		testDB := testdb.New(t)
		user := testutil.Must(factory.User(ctx, testDB.DB))(t)
		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour)), factory.WithoutTitle()))(t)
		testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, feedRow.ID))(t)

		future := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(time.Hour))))(t)
		testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, future.ID))(t)
		testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour))))(t)

		ctrl := NewMockController(t)
		fetcherMock := Mock[fetcher](ctrl)
		cleanerMock := Mock[cleaner](ctrl)

		content := &reader.Feed{Title: "fetched title", Description: "fetched description"}
		WhenDouble(fetcherMock.Fetch(Any[context.Context](), Any[string]())).ThenReturn(content, nil)
		WhenSingle(cleanerMock.CleanField(Any[string]())).ThenAnswer(func(args []any) string {
			return args[0].(string)
		})

		q := fakequeue.New()
		f := newService(repo.New(testDB.DB), fetcherMock, cleanerMock, nil, WithJobs(q))
		require.NoError(t, f.ScheduleRefreshes(ctx, ScheduleArgs{}))

		refreshes := fakequeue.Of[RefreshArgs](q)
		require.Equal(t, []RefreshArgs{{FeedID: feedRow.ID}}, refreshes, "only the due feed with a subscriber is queued for a refresh")

		for _, args := range refreshes {
			require.NoError(t, f.Refresh(ctx, args))
		}

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

	t.Run("queues one refresh per feed until it finishes", func(t *testing.T) {
		t.Parallel()

		testDB := testdb.New(t)
		user := testutil.Must(factory.User(ctx, testDB.DB))(t)
		feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB, factory.NextFetchAt(time.Now().Add(-time.Hour))))(t)
		testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, feedRow.ID))(t)

		q := testutil.Must(jobs.NewInserter(testDB.DB))(t)
		f := newService(repo.New(testDB.DB), nil, nil, nil, WithJobs(q))

		refreshJobs := func() int {
			var n int
			require.NoError(t, testDB.DB.QueryRowContext(ctx, "SELECT count(*) FROM river_job WHERE kind = $1", RefreshArgs{}.Kind()).Scan(&n))

			return n
		}

		require.NoError(t, f.ScheduleRefreshes(ctx, ScheduleArgs{}))
		require.NoError(t, f.ScheduleRefreshes(ctx, ScheduleArgs{}))
		require.Equal(t, 1, refreshJobs(), "an unfinished refresh of the feed is not queued again")

		_, err := testDB.DB.ExecContext(ctx, "UPDATE river_job SET state = 'completed', finalized_at = now() WHERE kind = $1", RefreshArgs{}.Kind())
		require.NoError(t, err)

		require.NoError(t, f.ScheduleRefreshes(ctx, ScheduleArgs{}))
		require.Equal(t, 2, refreshJobs(), "a feed whose refresh finished is queued again")
	})

	t.Run("propagates a FeedsToRefresh error", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t)

		require.NoError(t, db.DB.Close())

		f := newService(repo.New(db.DB), nil, nil, nil)

		err := f.ScheduleRefreshes(ctx, ScheduleArgs{})
		require.Error(t, err)
	})
}

// rowCheckingFetcher serves one feed with one item that references an image,
// and on every download checks, from another connection, that no transaction
// is open on the database: the feed row can be locked without waiting, and no
// session is idle in a transaction.
type rowCheckingFetcher struct {
	db     *sql.DB
	feedID string

	mu     sync.Mutex
	checks []error
}

func (f *rowCheckingFetcher) checkRow(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var id string

	err := f.db.QueryRowContext(ctx, "SELECT id FROM rss_feeds WHERE id = $1 FOR UPDATE NOWAIT", f.feedID).Scan(&id)
	if err == nil {
		var idle int

		err = f.db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND pid <> pg_backend_pid() AND state LIKE 'idle in transaction%'`).Scan(&idle)
		if err == nil && idle > 0 {
			err = fmt.Errorf("%d sessions are idle in a transaction", idle)
		}
	}

	f.mu.Lock()
	f.checks = append(f.checks, err)
	f.mu.Unlock()
}

func (f *rowCheckingFetcher) Checks() []error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]error(nil), f.checks...)
}

func (f *rowCheckingFetcher) Fetch(ctx context.Context, _ string) (*reader.Feed, error) {
	f.checkRow(ctx)

	return &reader.Feed{Title: "t", Items: []*reader.Item{{URL: "https://example.com/with-image", Title: "t", Summary: "s"}}}, nil
}

func (f *rowCheckingFetcher) FetchMedia(ctx context.Context, _ string) (io.ReadCloser, error) {
	f.checkRow(ctx)

	return io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\nrest-of-file"))), nil
}

// refreshWithin runs Refresh and fails the test if it hasn't returned within
// a few seconds, so a refresh that deadlocks fails fast instead of hanging to
// the go test timeout.
func refreshWithin(t *testing.T, f *Service, args RefreshArgs) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)

	go func() { done <- f.Refresh(ctx, args) }()

	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Refresh did not return")

		return nil
	}
}

// TestRefresh_NoTransactionDuringDownload proves the refresh fetches the feed
// and its images outside any transaction.
func TestRefresh_NoTransactionDuringDownload(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, testDB.DB))(t)
	feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)
	testutil.Must(factory.Subscription(ctx, testDB.DB, user.ID, feedRow.ID))(t)

	imgURL := "https://img.example.test/pic.png"
	ctrl := NewMockController(t)
	cleanerMock := Mock[cleaner](ctrl)
	WhenDouble(cleanerMock.HTMLToMarkdown(Any[string]())).ThenReturn(fmt.Sprintf("![alt](%s)", imgURL), nil)

	fetcher := &rowCheckingFetcher{db: testDB.DB, feedID: feedRow.ID}
	f := newService(repo.New(testDB.DB), fetcher, cleanerMock, fakestorage.New())

	require.NoError(t, refreshWithin(t, f, RefreshArgs{FeedID: feedRow.ID}))

	checks := fetcher.Checks()
	require.Len(t, checks, 2, "the feed and its one image are downloaded")

	for _, err := range checks {
		require.NoError(t, err, "no transaction is open during a download")
	}

	items := testutil.Must(factory.ListRSSItems(ctx, testDB.DB, feedRow.ID))(t)
	require.Len(t, items, 1, "the item is saved after the downloads")
}

// racingFetcher serves one feed with one item that references an image. Its
// image download, which comes after the refresh has found the item new, saves
// the same item through another service first, as a concurrent refresh would.
type racingFetcher struct {
	db     *sql.DB
	other  *Service
	feedID string
	item   *reader.Item
}

func (f *racingFetcher) Fetch(context.Context, string) (*reader.Feed, error) {
	return &reader.Feed{Title: "t", Items: []*reader.Item{f.item}}, nil
}

func (f *racingFetcher) FetchMedia(ctx context.Context, _ string) (io.ReadCloser, error) {
	subscribers, err := repo.New(f.db).FeedSubscriptions(ctx, f.feedID)
	if err != nil {
		return nil, err
	}

	if _, err := f.other.saveFeedItem(ctx, f.feedID, f.item, subscribers); err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\nrest-of-file"))), nil
}

// TestRefresh_ItemSavedConcurrently pins saveItem's re-check: an item another
// refresh saved after this one prepared it is neither stored nor shown twice,
// and doesn't count as new.
func TestRefresh_ItemSavedConcurrently(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	feedRow := testutil.Must(factory.RSSFeed(ctx, testDB.DB))(t)
	users := []*model.User{
		testutil.Must(factory.User(ctx, testDB.DB))(t),
		testutil.Must(factory.User(ctx, testDB.DB))(t),
	}

	for _, u := range users {
		testutil.Must(factory.Subscription(ctx, testDB.DB, u.ID, feedRow.ID))(t)
	}

	ctrl := NewMockController(t)
	imageCleaner := Mock[cleaner](ctrl)
	WhenDouble(imageCleaner.HTMLToMarkdown(Any[string]())).ThenReturn("![alt](https://img.example.test/pic.png)", nil)
	WhenSingle(imageCleaner.CleanField(Any[string]())).ThenAnswer(func(args []any) string { return args[0].(string) })

	plainCleaner := Mock[cleaner](ctrl)
	WhenDouble(plainCleaner.HTMLToMarkdown(Any[string]())).ThenReturn("plain", nil)
	WhenSingle(plainCleaner.CleanField(Any[string]())).ThenAnswer(func(args []any) string { return args[0].(string) })

	fetcher := &racingFetcher{
		db:     testDB.DB,
		other:  newService(repo.New(testDB.DB), nil, plainCleaner, nil),
		feedID: feedRow.ID,
		item:   &reader.Item{URL: "https://example.com/raced", Title: "t", Summary: "s"},
	}
	f := newService(repo.New(testDB.DB), fetcher, imageCleaner, fakestorage.New())

	require.NoError(t, refreshWithin(t, f, RefreshArgs{FeedID: feedRow.ID}))

	items := testutil.Must(factory.ListRSSItems(ctx, testDB.DB, feedRow.ID))(t)
	require.Len(t, items, 1, "the item is stored once")
	require.Equal(t, "plain", items[0].SanitizedDescription, "the first save wins")

	for _, u := range users {
		require.Len(t, testutil.Must(readItems(ctx, testDB.DB, u))(t), 1, "each subscriber is shown the item once")
	}

	reloaded := testutil.Must(repo.Using(testDB.DB).FeedByID(ctx, feedRow.ID))(t)
	require.Equal(t, 0, reloaded.LastItemsCount, "an item saved by another refresh isn't new")
	require.Equal(t, feedRow.ConsecutiveEmptyFetches+1, reloaded.ConsecutiveEmptyFetches)
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
		_, err := newService(repo.Using(testDB.DB), nil, nil, nil).saveFeedItem(ctx, feedRow.ID, &reader.Item{URL: ""}, nil)
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
		isNew := testutil.Must(newService(repo.Using(testDB.DB), nil, cleanerMock, nil).saveFeedItem(ctx, feedRow.ID, item, nil))(t)
		require.True(t, isNew)

		isNew = testutil.Must(newService(repo.Using(testDB.DB), nil, cleanerMock, nil).saveFeedItem(ctx, feedRow.ID, item, nil))(t)
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

		isNew := testutil.Must(newService(repo.Using(testDB.DB), nil, cleanerMock, nil).saveFeedItem(ctx, feedRow.ID, item, nil))(t)
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

		isNew := testutil.Must(newService(repo.Using(testDB.DB), fetcherMock, cleanerMock, storage).saveFeedItem(ctx, feedRow.ID, item, nil))(t)
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

	err = newService(repo.Using(testDB.DB), fetcher, cleaner, nil).saveFeed(ctx, feed1, feedContent)
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

	err = newService(repo.Using(testDB.DB), fetcher, cleaner, nil).saveFeed(ctx, feed1, feedContent)
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

	err = newService(repo.Using(testDB.DB), fetcher, cleaner, nil).saveFeed(ctx, feed1, feedContent)
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
