package feedops

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/samber/lo"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

type RssFeed struct {
	ID          string
	URL         string
	Title       string
	NextFetchAt *time.Time
	LastError   string
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

	feeds := lo.Map(rssFeeds, func(feed *core.UserFeedSubscription, idx int) *RssFeed {
		return &RssFeed{
			ID:          feed.ID,
			URL:         feed.R.Feed.URL,
			Title:       feed.R.Feed.Title.String,
			NextFetchAt: feed.R.Feed.NextFetchAt.Ptr(),
			LastError:   feed.R.Feed.LastFetchError.String,
		}
	})

	return feeds, nil

}

type RssFeedItem struct {
	ID          string
	URL         string
	FeedTitle   string
	FeedURL     string
	Title       string
	PublishedAt time.Time
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
			FeedTitle:   item.R.RSSItem.R.Feed.Title.String,
			FeedURL:     item.R.RSSItem.R.Feed.URL,
		}
	})

	return items, nil
}
