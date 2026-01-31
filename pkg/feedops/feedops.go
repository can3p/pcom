package feedops

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/samber/lo"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

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

func GetRssFeeds(ctx context.Context, db boil.ContextExecutor, userID string) ([]*RssFeed, error) {
	rssFeeds, err := core.UserFeedSubscriptions(
		core.UserFeedSubscriptionWhere.UserID.EQ(userID),
		qm.Load(core.UserFeedSubscriptionRels.Feed),
		qm.OrderBy(fmt.Sprintf("%s ASC", core.UserFeedSubscriptionColumns.ID)),
	).All(ctx, db)

	if err != nil {
		return nil, err
	}

	feedIDs := lo.Map(rssFeeds, func(feed *core.UserFeedSubscription, idx int) string {
		return feed.FeedID
	})

	lastImportedMap := make(map[string]*time.Time)
	if len(feedIDs) > 0 {
		latestItems, err := core.RSSItems(
			core.RSSItemWhere.FeedID.IN(feedIDs),
			qm.Select(core.RSSItemColumns.FeedID, fmt.Sprintf("MAX(%s) as created_at", core.RSSItemColumns.CreatedAt)),
			qm.GroupBy(core.RSSItemColumns.FeedID),
		).All(ctx, db)

		if err != nil {
			return nil, err
		}

		for _, item := range latestItems {
			t := item.CreatedAt
			lastImportedMap[item.FeedID] = &t
		}
	}

	feeds := lo.Map(rssFeeds, func(feed *core.UserFeedSubscription, idx int) *RssFeed {
		return &RssFeed{
			ID:             feed.ID,
			URL:            feed.R.Feed.URL,
			WebsiteURL:     extractWebsiteURL(feed.R.Feed.URL),
			Title:          feed.R.Feed.Title.String,
			NextFetchAt:    feed.R.Feed.NextFetchAt.Ptr(),
			LastFetchedAt:  feed.R.Feed.LastFetchedAt.Ptr(),
			LastImportedAt: lastImportedMap[feed.FeedID],
			LastError:      feed.R.Feed.LastFetchError.String,
		}
	})

	return feeds, nil
}

func extractWebsiteURL(feedURL string) string {
	parsed, err := url.Parse(feedURL)
	if err != nil {
		return feedURL
	}
	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}

type RssFeedItem struct {
	ID          string
	URL         string
	FeedTitle   string
	FeedURL     string
	Title       string
	PublishedAt time.Time
	AddedAt     time.Time
	Summary     string
}

func GetRssFeedItems(ctx context.Context, db boil.ContextExecutor, userID string) ([]*RssFeedItem, error) {
	dbItems, err := core.UserFeedItems(
		core.UserFeedItemWhere.UserID.EQ(userID),
		core.UserFeedItemWhere.IsDismissed.EQ(false),
		qm.Load(qm.Rels(
			core.UserFeedItemRels.RSSItem,
			core.RSSItemRels.Feed,
		)),
		qm.Load(core.UserFeedItemRels.URL),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.UserFeedItemColumns.ID)),
	).All(ctx, db)

	if err != nil {
		return nil, err
	}

	items := lo.Map(dbItems, func(item *core.UserFeedItem, idx int) *RssFeedItem {
		publishedAt := item.CreatedAt

		if !item.R.RSSItem.PublishedAt.IsZero() {
			publishedAt = item.R.RSSItem.PublishedAt
		}

		return &RssFeedItem{
			ID:          item.ID,
			URL:         item.R.URL.URL,
			Title:       item.R.RSSItem.Title,
			Summary:     item.R.RSSItem.SanitizedDescription,
			PublishedAt: publishedAt,
			AddedAt:     item.CreatedAt,
			FeedTitle:   item.R.RSSItem.R.Feed.Title.String,
			FeedURL:     item.R.RSSItem.R.Feed.URL,
		}
	})

	return items, nil
}
