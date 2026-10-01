// Package feeds manages RSS subscriptions: what a user follows, the items
// the poller delivers to them, and dismissing an item. The poller that
// fetches feeds lives here too (poller.go).
package feeds

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/util"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
)

type Service struct {
	store        *repo.Store
	fetcher      fetcher
	cleaner      cleaner
	mediaStorage server.MediaStorage
}

// New builds the service with the reader the poller uses in production.
func New(store *repo.Store, mediaStorage server.MediaStorage) *Service {
	return newService(store, reader.DefaultFetcher(), reader.DefaultCleaner(), mediaStorage)
}

func newService(store *repo.Store, fetcher fetcher, cleaner cleaner, mediaStorage server.MediaStorage) *Service {
	return &Service{store: store, fetcher: fetcher, cleaner: cleaner, mediaStorage: mediaStorage}
}

// RssFeed is a feed as its subscriber sees it. ID is the subscription's id.
type RssFeed struct {
	ID             string
	URL            string
	WebsiteURL     string
	Title          string
	NextFetchAt    *time.Time
	LastFetchedAt  *time.Time
	LastImportedAt *time.Time
	LastError      string
}

// RssFeedItem is an item in a user's reading list.
type RssFeedItem struct {
	// ID is the user_feed_item ID; RSSItemID is the item's own, which the
	// translation service takes, and Language its detected ISO 639-1 code.
	ID          string
	RSSItemID   string
	Language    null.String
	URL         string
	FeedTitle   string
	FeedURL     string
	Title       string
	PublishedAt time.Time
	AddedAt     time.Time
	Summary     string
}

// Subscribe follows a feed for the actor. Following it again does nothing.
func (s *Service) Subscribe(ctx context.Context, actor *core.User, rawURL string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	normalizedURL, err := util.NormalizeURL(rawURL)
	if err != nil {
		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		feed, err := tx.UpsertFeed(ctx, normalizedURL)
		if err != nil {
			return err
		}

		return tx.Subscribe(ctx, actor.ID, feed.ID)
	})
}

// Unsubscribe removes one of the actor's subscriptions. The feed itself stays,
// and the poller skips feeds nobody follows.
func (s *Service) Unsubscribe(ctx context.Context, actor *core.User, subscriptionID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if subscriptionID == "" {
		return service.Invalid("id", "No subscription found")
	}

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		return tx.DeleteSubscription(ctx, actor.ID, subscriptionID)
	})
	if err != nil {
		return operationFailed(err)
	}

	return nil
}

// Dismiss hides one of the actor's feed items from their reading list.
func (s *Service) Dismiss(ctx context.Context, actor *core.User, itemID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if itemID == "" {
		return service.Invalid("id", "No item found")
	}

	item, err := s.store.UserFeedItemByID(ctx, actor.ID, itemID)
	if errors.Is(err, repo.ErrNotFound) {
		return service.ErrNotFound
	} else if err != nil {
		return err
	}

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		return tx.DismissFeedItem(ctx, item)
	})
	if err != nil {
		return operationFailed(err)
	}

	return nil
}

// Subscriptions lists the actor's feeds, oldest subscription first.
func (s *Service) Subscriptions(ctx context.Context, actor *core.User) ([]*RssFeed, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	subs, err := s.store.SubscriptionsOf(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	feedIDs := lo.Map(subs, func(sub *core.UserFeedSubscription, _ int) string {
		return sub.FeedID
	})

	lastImported, err := s.store.LastImportedAt(ctx, feedIDs)
	if err != nil {
		return nil, err
	}

	return lo.Map(subs, func(sub *core.UserFeedSubscription, _ int) *RssFeed {
		feed := sub.R.Feed

		var importedAt *time.Time
		if t, ok := lastImported[sub.FeedID]; ok {
			importedAt = &t
		}

		return &RssFeed{
			ID:             sub.ID,
			URL:            feed.URL,
			WebsiteURL:     extractWebsiteURL(feed.URL),
			Title:          feed.Title.String,
			NextFetchAt:    feed.NextFetchAt.Ptr(),
			LastFetchedAt:  feed.LastFetchedAt.Ptr(),
			LastImportedAt: importedAt,
			LastError:      feed.LastFetchError.String,
		}
	}), nil
}

// Items is the actor's reading list: undismissed items, newest first.
func (s *Service) Items(ctx context.Context, actor *core.User) ([]*RssFeedItem, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	dbItems, err := s.store.UndismissedFeedItems(ctx, actor.ID, repo.Page{})
	if err != nil {
		return nil, err
	}

	return lo.Map(dbItems, func(item *core.UserFeedItem, _ int) *RssFeedItem {
		publishedAt := item.CreatedAt

		if !item.R.RSSItem.PublishedAt.IsZero() {
			publishedAt = item.R.RSSItem.PublishedAt
		}

		return &RssFeedItem{
			ID:          item.ID,
			RSSItemID:   item.RSSItemID,
			Language:    item.R.RSSItem.Language,
			URL:         item.R.URL.URL,
			Title:       item.R.RSSItem.Title,
			Summary:     item.R.RSSItem.SanitizedDescription,
			PublishedAt: publishedAt,
			AddedAt:     item.CreatedAt,
			FeedTitle:   item.R.RSSItem.R.Feed.Title.String,
			FeedURL:     item.R.RSSItem.R.Feed.URL,
		}
	}), nil
}

// operationFailed words a failed write the way the actions always did.
func operationFailed(err error) error {
	return fmt.Errorf("Operation Failed: %s", err.Error()) //nolint:staticcheck // shown to the user as is
}

func extractWebsiteURL(feedURL string) string {
	parsed, err := url.Parse(feedURL)
	if err != nil {
		return feedURL
	}

	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}
