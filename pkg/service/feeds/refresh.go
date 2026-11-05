package feeds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"time"

	"github.com/can3p/pcom/pkg/service/media"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

const (
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

// ScheduleRefreshes is the work of the feeds.schedule job: it enqueues a
// feeds.refresh job for every due feed with a subscriber. A feed whose
// refresh is still unfinished gets no second one (RefreshArgs.InsertOpts).
func (s *Service) ScheduleRefreshes(ctx context.Context, _ ScheduleArgs) error {
	feeds, err := s.store.FeedsToRefresh(ctx)
	if err != nil {
		return err
	}

	for _, feed := range feeds {
		if err := s.store.Enqueue(ctx, s.jobs, RefreshArgs{FeedID: feed.ID}); err != nil {
			return err
		}
	}

	return nil
}

// Refresh is the work of the feeds.refresh job: it fetches one feed and
// stores its new items. The network work (the feed, then the images of its
// new items) happens outside any transaction; only the save is
// transactional. A fetch error is saved on the feed, whose next_fetch_at is
// its backoff, and the job completes. A panic comes back as an error.
func (s *Service) Refresh(ctx context.Context, args RefreshArgs) (err error) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			err = fmt.Errorf("refreshing feed %s panicked: %v - %s", args.FeedID, panicErr, string(debug.Stack()))
		}
	}()

	ctx = logging.WithLogger(ctx, s.logger)

	feed, err := s.store.FeedByID(ctx, args.FeedID)
	if errors.Is(err, repo.ErrNotFound) {
		// nothing to refresh
		return nil
	}

	if err != nil {
		return err
	}

	return s.refreshFeed(ctx, feed)
}

func (s *Service) refreshFeed(ctx context.Context, feed *model.RSSFeed) error {
	rssFeed, fetchErr := s.fetcher.Fetch(ctx, feed.URL)
	if fetchErr != nil {
		return s.store.Tx(ctx, func(tx *repo.Store) error {
			locked, err := tx.LockFeed(ctx, feed.ID)
			if err != nil {
				return err
			}

			return saveFetchFailure(ctx, tx, locked, fetchErr)
		})
	}

	return s.saveFeed(ctx, feed, rssFeed)
}

func saveFetchFailure(ctx context.Context, tx *repo.Store, feed *model.RSSFeed, fetchErr error) error {
	feed.LastFetchError = new(fetchErr.Error())
	feed.LastItemsCount = 0
	feed.NextFetchAt = new(time.Now().Add(reader.ErrorFetchInterval))
	feed.LastFetchedAt = new(time.Now())

	return tx.SaveFeed(ctx, feed)
}

// preparedItem is a new item ready to be saved: converted to markdown, its
// images stored.
type preparedItem struct {
	item   *model.RSSItem
	rawURL string
}

// saveFeed stores the fetched feed's new items and its counters. It finds and
// prepares the new items with reads only, then saves them in one
// transaction that locks the feed row.
func (s *Service) saveFeed(ctx context.Context, feed *model.RSSFeed, rssFeed *reader.Feed) error {
	prepared, err := s.prepareNewItems(ctx, feed.ID, rssFeed)
	if err != nil {
		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		locked, err := tx.LockFeed(ctx, feed.ID)
		if err != nil {
			return err
		}

		return s.saveFetchedFeed(ctx, tx, locked, rssFeed, prepared)
	})
}

// prepareNewItems finds the items of rssFeed the feed doesn't have yet and
// prepares them, oldest first. It writes nothing but the images it stores.
func (s *Service) prepareNewItems(ctx context.Context, feedID string, rssFeed *reader.Feed) ([]*preparedItem, error) {
	// Check if this is an initial fetch by seeing if any items exist for this feed
	existingCount, err := s.store.FeedItemCount(ctx, feedID)
	if err != nil {
		return nil, err
	}

	isInitialFetch := existingCount == 0

	totalItemsToFetch := len(rssFeed.Items)
	if isInitialFetch && totalItemsToFetch > maxInitialFetchItems {
		// Only process the N most recent items (which are at the beginning of the slice)
		totalItemsToFetch = maxInitialFetchItems
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

			exists, err := s.store.FeedItemExistsForURL(ctx, feedID, item.URL)
			if err != nil {
				return nil, err
			}

			if exists {
				// Found first known item, all items before this are new
				firstKnownItemIdx = idx
				break
			}
		}
	}

	// Prepare items in chronological order (from oldest new item to newest)
	// This means iterating from firstKnownItemIdx-1 down to 0
	prepared := make([]*preparedItem, 0, firstKnownItemIdx)
	for idx := firstKnownItemIdx - 1; idx >= 0; idx-- {
		p, err := s.prepareItem(ctx, feedID, rssFeed.Items[idx])
		if err != nil {
			return nil, err
		}

		prepared = append(prepared, p)
	}

	return prepared, nil
}

// saveFetchedFeed saves the prepared items and the feed's counters on tx,
// which holds the feed row locked.
func (s *Service) saveFetchedFeed(ctx context.Context, tx *repo.Store, feed *model.RSSFeed, rssFeed *reader.Feed, prepared []*preparedItem) error {
	if feed.Title == nil {
		cleaned := s.cleaner.CleanField(rssFeed.Title)
		feed.Title = lo.EmptyableToPtr(cleaned)
	}

	if feed.Description == nil {
		cleaned := s.cleaner.CleanField(rssFeed.Description)
		feed.Description = lo.EmptyableToPtr(cleaned)
	}

	// Get subscribers once for all items
	subscribers, err := tx.FeedSubscriptions(ctx, feed.ID)
	if err != nil {
		return err
	}

	newItems := 0
	for _, p := range prepared {
		isNew, err := saveItem(ctx, tx, p, subscribers)
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
	feed.NextFetchAt = new(reader.CalculateNextFetchTime(feed.ConsecutiveEmptyFetches, feed.AvgItemsPerDay, wasManual))

	// Update last manual refresh time if this was a manual fetch
	if wasManual {
		feed.LastManualRefreshAt = new(time.Now())
	}

	feed.LastFetchedAt = new(time.Now())
	feed.LastFetchError = nil

	return tx.SaveFeed(ctx, feed)
}

// saveFeedItem prepares one item and saves it in a transaction of its own.
func (s *Service) saveFeedItem(ctx context.Context, feedID string, rssFeedItem *reader.Item, subscribers []*model.UserFeedSubscription) (bool, error) {
	p, err := s.prepareItem(ctx, feedID, rssFeedItem)
	if err != nil {
		return false, err
	}

	var isNew bool

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		isNew, err = saveItem(ctx, tx, p, subscribers)

		return err
	})

	return isNew, err
}

// prepareItem converts an item's summary to markdown, storing the images it
// references on the plain store, outside any transaction. Images stored for
// an item another refresh saves first stay stored.
func (s *Service) prepareItem(ctx context.Context, feedID string, rssFeedItem *reader.Item) (*preparedItem, error) {
	if rssFeedItem.URL == "" {
		return nil, fmt.Errorf("refuse to save an rss item without URL")
	}

	// Generate a new UUID v7 for this item (chronological ordering)
	itemID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

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
			return media.StoreUpload(downloadCtx, s.store, s.mediaStorage, nil, &feedID, readerIO, reader.MaxMediaSize)
		}

		replacer := reader.CreateImageReplacer(logging.FromContext(ctx), markdownContent, uploadFunc)

		markdownContent, err = markdown.ReplaceImageUrlsOrLinkify(markdownContent, replacer, markdown.WithLogger(logging.FromContext(ctx)))
		if err != nil {
			return nil, fmt.Errorf("failed to process images in feed item: %w", err)
		}
	}

	publishedAt := time.Now()

	if rssFeedItem.PublishedAt != nil {
		publishedAt = *rssFeedItem.PublishedAt
	}

	return &preparedItem{
		rawURL: rssFeedItem.URL,
		item: &model.RSSItem{
			ID:                   itemID.String(),
			FeedID:               feedID,
			GUID:                 rssFeedItem.URL,
			Title:                s.cleaner.CleanField(rssFeedItem.Title),
			Description:          rssFeedItem.Summary,
			PublishedAt:          publishedAt,
			SanitizedDescription: markdownContent,
		},
	}, nil
}

// saveItem stores a prepared item and shows it to the subscribers, unless
// the feed has it already (another refresh may have saved it since it was
// prepared). It reports whether the item is new.
func saveItem(ctx context.Context, tx *repo.Store, p *preparedItem, subscribers []*model.UserFeedSubscription) (bool, error) {
	url, err := tx.StoreURL(ctx, p.rawURL)
	if err != nil {
		return false, err
	}

	exists, err := tx.FeedItemExists(ctx, p.item.FeedID, url.ID)
	if err != nil {
		return false, err
	}

	if exists {
		return false, nil
	}

	feedItem := p.item
	feedItem.URLID = url.ID

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
