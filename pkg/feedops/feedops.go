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
