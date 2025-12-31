package feeder

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

const (
	pollEvery     = 10 * time.Second
	avgWindowDays = 3
)

type fetcher interface {
	Fetch(urL string) (*reader.Feed, error)
	FetchMedia(ctx context.Context, mediaURL string) (io.ReadCloser, error)
}

type cleaner interface {
	CleanField(in string) string
	HTMLToMarkdown(in string, replacer types.Replacer[string]) (string, error)
}

type Feeder struct {
	db           *sqlx.DB
	fetcher      fetcher
	cleaner      cleaner
	mediaStorage server.MediaStorage
}

func NewFeeder(db *sqlx.DB, fetcher fetcher, cleaner cleaner, mediaStorage server.MediaStorage) *Feeder {
	return &Feeder{
		db:           db,
		fetcher:      fetcher,
		cleaner:      cleaner,
		mediaStorage: mediaStorage,
	}
}

func (f *Feeder) RunPoller(ctx context.Context) {
	ticker := time.NewTicker(pollEvery)

	for {
		select {
		case <-ticker.C:
			if err := f.refreshFeeds(ctx); err != nil {
				slog.Warn("Failed to refreshFeeds", "err", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

func (f *Feeder) refreshFeeds(ctx context.Context) (err error) {
	// we don't want any code including the real sender to crash
	// the scheduler
	defer func() {
		if panicErr := recover(); panicErr != nil {
			err = fmt.Errorf("refreshFeeds panicked: %v - %s", panicErr, string(debug.Stack()))
		}
	}()

	feeds, err := GetFeedsToRefresh(ctx, f.db)

	// transaction per feed to make sure
	// we don't hammer all the feeds endlessly because of one bad actor
	for _, ff := range feeds {
		err := transact.Transact(f.db, func(tx *sql.Tx) error {
			feed, err := LockFeed(ctx, tx, ff.ID)

			if err != nil {
				return err
			}

			return f.tryFetchFeed(ctx, tx, feed)
		})

		if err != nil {
			slog.Warn("failed to fetch the feed", "feed_id", ff.ID, "err", err)
			continue
		}
	}

	return nil
}

func (f *Feeder) tryFetchFeed(ctx context.Context, exec boil.ContextExecutor, feed *core.RSSFeed) (err error) {
	rssFeed, fetchErr := f.fetcher.Fetch(feed.URL)

	if fetchErr != nil {
		err := SaveFetchFailure(ctx, exec, feed, fetchErr)

		if err != nil {
			return err
		}

		return nil
	}

	return SaveFeed(ctx, exec, feed, rssFeed, f.cleaner, f.fetcher, f.mediaStorage)
}

func GetFeedsToRefresh(ctx context.Context, exec boil.ContextExecutor) ([]*core.RSSFeed, error) {
	feeds, err := core.RSSFeeds(
		core.RSSFeedWhere.NextFetchAt.LT(null.TimeFrom(time.Now())),
		qm.Load(core.RSSFeedRels.FeedUserFeedSubscriptions, qm.Limit(1)),
		qm.Or2(core.RSSFeedWhere.NextFetchAt.IsNull()),
	).All(ctx, exec)

	if err != nil {
		return nil, err
	}

	// we're only interested in refreshing feeds with at least one subscription
	feeds = lo.Filter(feeds, func(f *core.RSSFeed, index int) bool {
		return len(f.R.FeedUserFeedSubscriptions) > 0
	})

	return feeds, nil
}

func LockFeed(ctx context.Context, exec boil.ContextExecutor, feedID string) (*core.RSSFeed, error) {
	feed, err := core.RSSFeeds(
		core.RSSFeedWhere.ID.EQ(feedID),
		qm.For("UPDATE SKIP LOCKED"),
	).One(ctx, exec)

	if err != nil {
		return nil, err
	}

	return feed, nil
}

func SaveFetchFailure(ctx context.Context, exec boil.ContextExecutor, feed *core.RSSFeed, fetchErr error) error {
	feed.LastFetchError = null.StringFrom(fetchErr.Error())
	feed.LastItemsCount = 0
	feed.NextFetchAt = null.TimeFrom(time.Now().Add(reader.MaxFetchInterval * time.Hour))
	feed.LastFetchedAt = null.TimeFrom(time.Now())

	_, err := feed.Update(ctx, exec, boil.Infer())

	return err
}

func SaveFeed(ctx context.Context, exec boil.ContextExecutor, feed *core.RSSFeed, rssFeed *reader.Feed, cleaner cleaner, fetcher fetcher, mediaStorage server.MediaStorage) error {
	var err error

	if feed.Title.IsZero() {
		cleaned := cleaner.CleanField(rssFeed.Title)
		feed.Title = null.NewString(cleaned, cleaned != "")
	}

	if feed.Description.IsZero() {
		cleaned := cleaner.CleanField(rssFeed.Description)
		feed.Description = null.NewString(cleaned, cleaned != "")
	}

	newItems := 0

	// items in an rss feed usually go in descending order
	for idx := len(rssFeed.Items) - 1; idx >= 0; idx-- {
		item := rssFeed.Items[idx]
		isNew, err := SaveFeedItem(ctx, exec, feed.ID, item, cleaner, fetcher, mediaStorage)

		if err != nil {
			return err
		}

		if isNew {
			newItems++
		}
	}

	// Update average items per day using exponential moving average
	feed.AvgItemsPerDay, err = calculateNewAverage(ctx, exec, feed.ID, avgWindowDays)

	if err != nil {
		return err
	}

	feed.LastItemsCount = newItems

	// Update consecutive empty fetches
	if newItems == 0 {
		feed.ConsecutiveEmptyFetches++
	} else {
		feed.ConsecutiveEmptyFetches = 0
	}

	// not implemented yet
	wasManual := false

	// Calculate next fetch time
	feed.NextFetchAt = null.TimeFrom(reader.CalculateNextFetchTime(feed.ConsecutiveEmptyFetches, feed.AvgItemsPerDay, wasManual))

	// Update last manual refresh time if this was a manual fetch
	if wasManual {
		feed.LastManualRefreshAt = null.TimeFrom(time.Now())
	}

	feed.LastFetchedAt = null.TimeFrom(time.Now())

	_, err = feed.Update(ctx, exec, boil.Infer())

	return err
}

func SaveFeedItem(ctx context.Context, exec boil.ContextExecutor, feedID string, rssFeedItem *reader.Item, cleaner cleaner, fetcher fetcher, mediaStorage server.MediaStorage) (bool, error) {
	if rssFeedItem.URL == "" {
		return false, fmt.Errorf("refuse to save an rss item without URL")
	}

	// this call takes care of empty urls
	url, err := postops.StoreURL(ctx, exec, rssFeedItem.URL)

	if err != nil {
		return false, err
	}

	// Generate UUID v7 for the new URL
	id, err := uuid.NewV7()
	if err != nil {
		return false, err
	}

	feedItemID := id.String()

	// First convert HTML to markdown without image replacement
	markdown, err := cleaner.HTMLToMarkdown(rssFeedItem.Summary, nil)

	if err != nil {
		markdown = fmt.Sprintf("Summary errors: %s", err.Error())
	} else {
		// Create a context with global timeout for all image downloads
		downloadCtx, cancel := context.WithTimeout(ctx, reader.GlobalImageDownloadTimeout)
		defer cancel()

		// Create upload function for images
		uploadFunc := func(ctx context.Context, imageURL string) (string, error) {
			readerIO, err := fetcher.FetchMedia(ctx, imageURL)
			if err != nil {
				return "", err
			}
			defer func() { _ = readerIO.Close() }()

			return media.HandleUpload(ctx, exec, mediaStorage, nil, &feedID, readerIO)
		}

		// Create replacer that downloads images and handles errors
		replacer := reader.CreateImageReplacer(downloadCtx, markdown, nil, uploadFunc)

		// Apply image URL replacement
		markdown, err = cleaner.HTMLToMarkdown(rssFeedItem.Summary, replacer)
		if err != nil {
			markdown = fmt.Sprintf("Summary errors: %s", err.Error())
		}
	}

	publishedAt := time.Now()

	if rssFeedItem.PublishedAt != nil {
		publishedAt = *rssFeedItem.PublishedAt
	}

	feedItem := &core.RSSItem{
		ID:                   feedItemID,
		FeedID:               feedID,
		URLID:                url.ID,
		GUID:                 rssFeedItem.URL,
		Title:                cleaner.CleanField(rssFeedItem.Title),
		Description:          rssFeedItem.Summary,
		PublishedAt:          publishedAt,
		SanitizedDescription: markdown,
	}

	// Try to insert, if URL exists, get existing ID
	// we need to pass true to get existing id back
	err = feedItem.Upsert(ctx, exec, true, []string{core.RSSItemColumns.FeedID, core.RSSItemColumns.URLID}, boil.Whitelist(core.RSSItemColumns.FeedID), boil.Infer())
	if err != nil {
		return false, err
	}

	// since we're generating a unique id every time
	// and upsert returns sets the model id to the existing value on update
	// we can use this fact to understand what happened
	wasUpdate := feedItemID != feedItem.ID

	// update means we've already seen this item
	if wasUpdate {
		return false, nil
	}

	subscribers, err := core.UserFeedSubscriptions(
		core.UserFeedSubscriptionWhere.FeedID.EQ(feedID),
	).All(ctx, exec)

	if err != nil {
		return false, err
	}

	for _, s := range subscribers {
		id, err := uuid.NewV7()
		if err != nil {
			return false, err
		}

		userItem := core.UserFeedItem{
			ID:        id.String(),
			UserID:    s.UserID,
			RSSItemID: feedItem.ID,
			URLID:     url.ID,
		}

		if err := userItem.Insert(ctx, exec, boil.Infer()); err != nil {
			return false, err
		}
	}

	// if we got this far, it's definitely a new item
	return true, nil
}

func calculateNewAverage(ctx context.Context, exec boil.ContextExecutor, feedID string, avgWindowDays int) (float64, error) {
	count, err := core.RSSItems(
		core.RSSItemWhere.FeedID.EQ(feedID),
	).Count(ctx, exec)

	if err != nil {
		return 0, err
	}

	if count == 0 {
		return 0, nil
	}

	return float64(count) / float64(avgWindowDays), nil
}
