package feedops

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func SubscribeToFeed(ctx context.Context, exec boil.ContextExecutor, userID string, rawURL string) error {
	normalizedURL, err := util.NormalizeURL(rawURL)
	if err != nil {
		return err
	}

	feedID, err := uuid.NewV7()
	if err != nil {
		return err
	}

	feed := core.RSSFeed{
		ID:  feedID.String(),
		URL: normalizedURL,
	}

	// we update only url on upsert, since we don't really want to update anything
	// and id is refreshed in the model only incase we do at least some update
	err = feed.Upsert(ctx, exec, true, []string{core.RSSFeedColumns.URL}, boil.Whitelist(core.RSSFeedColumns.URL), boil.Infer())

	if err != nil {
		return err
	}

	userSubscriptionID, err := uuid.NewV7()
	if err != nil {
		return err
	}

	userSubscription := core.UserFeedSubscription{
		ID:     userSubscriptionID.String(),
		FeedID: feed.ID,
		UserID: userID,
	}

	err = userSubscription.Upsert(
		ctx,
		exec,
		true,
		[]string{core.UserFeedSubscriptionColumns.UserID, core.UserFeedSubscriptionColumns.FeedID},
		boil.Infer(), boil.Infer())

	if err != nil {
		return err
	}

	return nil
}

func UnsubscribeFromFeed(ctx context.Context, exec boil.ContextExecutor, userID string, subscriptionID string) error {
	_, err := core.UserFeedSubscriptions(
		core.UserFeedSubscriptionWhere.ID.EQ(subscriptionID),
		core.UserFeedSubscriptionWhere.UserID.EQ(userID),
	).DeleteAll(ctx, exec)

	// we don't do final clean up from rss_feeds table there just
	// now to keep things simple and avoid dealing with race conditions
	// when one user deletes a feed and another one adds it back.
	//
	// The consequence is that feed fetching job will have to exclude feeds
	// without subscriptions from the update.
	// We can always fix this later

	return err
}
