package feeds

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/can3p/pcom/pkg/service/media"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

const (
	pollEvery            = 10 * time.Second
	avgWindowDays        = 3
	maxInitialFetchItems = 5
)

type fetcher interface {
	Fetch(ctx context.Context, url string) (*reader.Feed, error)
	FetchMedia(ctx context.Context, mediaURL string) (io.ReadCloser, error)
}

type cleaner interface {
	CleanField(in string) string
	HTMLToMarkdown(in string) (string, error)
}

// RunPoller refreshes due feeds until ctx is done.
func (s *Service) RunPoller(ctx context.Context) {
	ticker := time.NewTicker(pollEvery)

	for {
		select {
		case <-ticker.C:
			if err := s.refreshFeeds(ctx); err != nil {
				slog.Warn("Failed to refreshFeeds", "err", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) refreshFeeds(ctx context.Context) (err error) {
	// we don't want any code including the real sender to crash
	// the scheduler
	defer func() {
		if panicErr := recover(); panicErr != nil {
			err = fmt.Errorf("refreshFeeds panicked: %v - %s", panicErr, string(debug.Stack()))
		}
	}()

	feeds, err := s.store.FeedsToRefresh(ctx)
	if err != nil {
		return err
	}

	// transaction per feed to make sure
	// we don't hammer all the feeds endlessly because of one bad actor.
	// The feed row stays locked while it is fetched (open question Q9).
	for _, ff := range feeds {
		err := s.store.Tx(ctx, func(tx *repo.Store) error {
			feed, err := tx.LockFeed(ctx, ff.ID)
			if err != nil {
				return err
			}

			return s.tryFetchFeed(ctx, tx, feed)
		})
		if err != nil {
			slog.Warn("failed to fetch the feed", "feed_id", ff.ID, "err", err)
			continue
		}
	}

	return nil
}

func (s *Service) tryFetchFeed(ctx context.Context, tx *repo.Store, feed *core.RSSFeed) error {
	rssFeed, fetchErr := s.fetcher.Fetch(ctx, feed.URL)
	if fetchErr != nil {
		return saveFetchFailure(ctx, tx, feed, fetchErr)
	}

	return s.saveFeed(ctx, tx, feed, rssFeed)
}

func saveFetchFailure(ctx context.Context, tx *repo.Store, feed *core.RSSFeed, fetchErr error) error {
	feed.LastFetchError = null.StringFrom(fetchErr.Error())
	feed.LastItemsCount = 0
	feed.NextFetchAt = null.TimeFrom(time.Now().Add(reader.ErrorFetchInterval))
	feed.LastFetchedAt = null.TimeFrom(time.Now())

	return tx.SaveFeed(ctx, feed)
}

func (s *Service) saveFeed(ctx context.Context, tx *repo.Store, feed *core.RSSFeed, rssFeed *reader.Feed) error {
	if feed.Title.IsZero() {
		cleaned := s.cleaner.CleanField(rssFeed.Title)
		feed.Title = null.NewString(cleaned, cleaned != "")
	}

	if feed.Description.IsZero() {
		cleaned := s.cleaner.CleanField(rssFeed.Description)
		feed.Description = null.NewString(cleaned, cleaned != "")
	}

	// Check if this is an initial fetch by seeing if any items exist for this feed
	existingCount, err := tx.FeedItemCount(ctx, feed.ID)
	if err != nil {
		return err
	}

	isInitialFetch := existingCount == 0

	totalItemsToFetch := len(rssFeed.Items)
	if isInitialFetch && totalItemsToFetch > maxInitialFetchItems {
		// Only process the N most recent items (which are at the beginning of the slice)
		totalItemsToFetch = maxInitialFetchItems
	}

	// Get subscribers once for all items
	subscribers, err := tx.FeedSubscriptions(ctx, feed.ID)
	if err != nil {
		return err
	}

	// Prefetch to find the index of the first known item
	// For initial fetch, all items (up to totalItemsToFetch) are new
	// For subsequent fetches, find where new items end
	firstKnownItemIdx := totalItemsToFetch
	if !isInitialFetch {
		for idx := 0; idx < totalItemsToFetch; idx++ {
			item := rssFeed.Items[idx]
			if item.URL == "" {
				continue
			}

			url, err := tx.StoreURL(ctx, item.URL)
			if err != nil {
				return err
			}

			exists, err := tx.FeedItemExists(ctx, feed.ID, url.ID)
			if err != nil {
				return err
			}

			if exists {
				// Found first known item, all items before this are new
				firstKnownItemIdx = idx
				break
			}
		}
	}

	// Insert items in chronological order (from oldest new item to newest)
	// This means iterating from firstKnownItemIdx-1 down to 0
	newItems := 0
	for idx := firstKnownItemIdx - 1; idx >= 0; idx-- {
		isNew, err := s.saveFeedItem(ctx, tx, feed.ID, rssFeed.Items[idx], subscribers)
		if err != nil {
			return err
		}

		if isNew {
			newItems++
		}
	}

	// Update average items per day using exponential moving average
	feed.AvgItemsPerDay, err = calculateNewAverage(ctx, tx, feed.ID, avgWindowDays)
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
	feed.LastFetchError = null.String{}

	return tx.SaveFeed(ctx, feed)
}

func (s *Service) saveFeedItem(ctx context.Context, tx *repo.Store, feedID string, rssFeedItem *reader.Item, subscribers core.UserFeedSubscriptionSlice) (bool, error) {
	if rssFeedItem.URL == "" {
		return false, fmt.Errorf("refuse to save an rss item without URL")
	}

	url, err := tx.StoreURL(ctx, rssFeedItem.URL)
	if err != nil {
		return false, err
	}

	// Generate a new UUID v7 for this item (chronological ordering)
	itemID, err := uuid.NewV7()
	if err != nil {
		return false, err
	}

	feedItemID := itemID.String()

	markdownContent, err := s.cleaner.HTMLToMarkdown(rssFeedItem.Summary)

	if err != nil {
		markdownContent = fmt.Sprintf("Summary errors: %s", err.Error())
	} else {
		// Create a context with global timeout for all image downloads
		downloadCtx, cancel := context.WithTimeout(ctx, reader.GlobalImageDownloadTimeout)
		defer cancel()

		// XXX: the code is kind of backwards because we're passing the control to cleaner
		// only for it to extract urls and call us back to upload them and the return a replacer
		// that we will call there. We could just
		// On the other hand I don't want to have another abstraction and also do not want to
		// inflate the function logic there.

		uploadFunc := func(imageURL string) (string, error) {
			readerIO, err := s.fetcher.FetchMedia(downloadCtx, imageURL)
			if err != nil {
				return "", err
			}
			defer func() { _ = readerIO.Close() }()

			// XXX: using download context for upload to maintain timeout consistency
			return media.StoreUpload(downloadCtx, tx, s.mediaStorage, nil, &feedID, readerIO)
		}

		replacer := reader.CreateImageReplacer(markdownContent, uploadFunc)

		markdownContent, err = markdown.ReplaceImageUrlsOrLinkify(markdownContent, replacer)
		if err != nil {
			return false, fmt.Errorf("failed to process images in feed item: %w", err)
		}
	}

	publishedAt := time.Now()

	if rssFeedItem.PublishedAt != nil {
		publishedAt = *rssFeedItem.PublishedAt
	}

	lang, langOK := translate.DetectLanguage(rssFeedItem.Title + "\n" + markdownContent)

	feedItem := &core.RSSItem{
		ID:                   feedItemID,
		FeedID:               feedID,
		URLID:                url.ID,
		GUID:                 rssFeedItem.URL,
		Title:                s.cleaner.CleanField(rssFeedItem.Title),
		Description:          rssFeedItem.Summary,
		PublishedAt:          publishedAt,
		SanitizedDescription: markdownContent,
		Language:             null.NewString(lang, langOK),
	}

	created, err := tx.UpsertFeedItem(ctx, feedItem)
	if err != nil {
		return false, err
	}

	// not created means we've already seen this item
	if !created {
		return false, nil
	}

	for _, sub := range subscribers {
		if err := tx.InsertUserFeedItem(ctx, sub.UserID, feedItem.ID, url.ID); err != nil {
			return false, err
		}
	}

	// if we got this far, it's definitely a new item
	return true, nil
}

func calculateNewAverage(ctx context.Context, tx *repo.Store, feedID string, avgWindowDays int) (float64, error) {
	count, err := tx.FeedItemCount(ctx, feedID)
	if err != nil {
		return 0, err
	}

	if count == 0 {
		return 0, nil
	}

	return float64(count) / float64(avgWindowDays), nil
}
