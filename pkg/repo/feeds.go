package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// UpsertFeed returns the feed with this (already normalized) URL, creating it
// if nobody subscribed to it yet.
func (s *Store) UpsertFeed(ctx context.Context, normalizedURL string) (*core.RSSFeed, error) {
	feedID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	feed := &core.RSSFeed{ID: feedID.String(), URL: normalizedURL}

	// we update only url on upsert, since we don't really want to update anything
	// and id is refreshed in the model only incase we do at least some update
	err = feed.Upsert(ctx, s.exec, true, []string{core.RSSFeedColumns.URL}, boil.Whitelist(core.RSSFeedColumns.URL), boil.Infer())
	if err != nil {
		return nil, err
	}

	return feed, nil
}

// Subscribe subscribes a user to a feed, keeping an existing subscription.
func (s *Store) Subscribe(ctx context.Context, userID, feedID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	sub := core.UserFeedSubscription{ID: id.String(), FeedID: feedID, UserID: userID}

	return sub.Upsert(
		ctx,
		s.exec,
		true,
		[]string{core.UserFeedSubscriptionColumns.UserID, core.UserFeedSubscriptionColumns.FeedID},
		boil.Infer(), boil.Infer())
}

// DeleteSubscription removes a user's subscription. The feed row stays: the
// poller skips feeds without subscribers, and this avoids racing a user who
// adds the feed back.
func (s *Store) DeleteSubscription(ctx context.Context, userID, subscriptionID string) error {
	_, err := core.UserFeedSubscriptions(
		core.UserFeedSubscriptionWhere.ID.EQ(subscriptionID),
		core.UserFeedSubscriptionWhere.UserID.EQ(userID),
	).DeleteAll(ctx, s.exec)

	return err
}

// SubscriptionsOf returns a user's subscriptions oldest first, with the feed
// loaded (sub.R.Feed).
func (s *Store) SubscriptionsOf(ctx context.Context, userID string) (core.UserFeedSubscriptionSlice, error) {
	return core.UserFeedSubscriptions(
		core.UserFeedSubscriptionWhere.UserID.EQ(userID),
		qm.Load(core.UserFeedSubscriptionRels.Feed),
		qm.OrderBy(fmt.Sprintf("%s ASC", core.UserFeedSubscriptionColumns.ID)),
	).All(ctx, s.exec)
}

// LastImportedAt maps each feed to the time its newest item was stored.
// Feeds without items are missing from the map.
func (s *Store) LastImportedAt(ctx context.Context, feedIDs []string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	if len(feedIDs) == 0 {
		return out, nil
	}

	latest, err := core.RSSItems(
		core.RSSItemWhere.FeedID.IN(feedIDs),
		qm.Select(core.RSSItemColumns.FeedID, fmt.Sprintf("MAX(%s) as created_at", core.RSSItemColumns.CreatedAt)),
		qm.GroupBy(core.RSSItemColumns.FeedID),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	for _, item := range latest {
		out[item.FeedID] = item.CreatedAt
	}

	return out, nil
}

// UndismissedFeedItems returns a page of a user's feed items that are not
// dismissed, newest addition first, with the item, its feed and its URL
// loaded.
func (s *Store) UndismissedFeedItems(ctx context.Context, userID string, page Page) (core.UserFeedItemSlice, error) {
	m := []qm.QueryMod{
		core.UserFeedItemWhere.UserID.EQ(userID),
		core.UserFeedItemWhere.IsDismissed.EQ(false),
		qm.Load(qm.Rels(
			core.UserFeedItemRels.RSSItem,
			core.RSSItemRels.Feed,
		)),
		qm.Load(core.UserFeedItemRels.URL),
	}

	return core.UserFeedItems(append(m, page.mods(KindRSSItem,
		core.UserFeedItemTableColumns.CreatedAt, core.UserFeedItemTableColumns.ID)...)...).All(ctx, s.exec)
}

// UserFeedItemByID returns a user's feed item, or ErrNotFound if the user has
// no such item.
func (s *Store) UserFeedItemByID(ctx context.Context, userID, itemID string) (*core.UserFeedItem, error) {
	item, err := core.UserFeedItems(
		core.UserFeedItemWhere.ID.EQ(itemID),
		core.UserFeedItemWhere.UserID.EQ(userID),
	).One(ctx, s.exec)

	return item, notFound(err)
}

// DismissFeedItem marks a feed item as dismissed.
func (s *Store) DismissFeedItem(ctx context.Context, item *core.UserFeedItem) error {
	item.IsDismissed = true
	_, err := item.Update(ctx, s.exec, boil.Infer())

	return err
}

// FeedsToRefresh returns the feeds due for a fetch that have at least one
// subscriber.
func (s *Store) FeedsToRefresh(ctx context.Context) ([]*core.RSSFeed, error) {
	feeds, err := core.RSSFeeds(
		core.RSSFeedWhere.NextFetchAt.LT(null.TimeFrom(time.Now())),
		qm.Load(core.RSSFeedRels.FeedUserFeedSubscriptions, qm.Limit(1)),
		qm.Or2(core.RSSFeedWhere.NextFetchAt.IsNull()),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	return lo.Filter(feeds, func(f *core.RSSFeed, _ int) bool {
		return len(f.R.FeedUserFeedSubscriptions) > 0
	}), nil
}

// LockFeed loads a feed and locks its row until the transaction ends. A feed
// another transaction holds is skipped, which reads as ErrNotFound.
func (s *Store) LockFeed(ctx context.Context, feedID string) (*core.RSSFeed, error) {
	feed, err := core.RSSFeeds(
		core.RSSFeedWhere.ID.EQ(feedID),
		qm.For("UPDATE SKIP LOCKED"),
	).One(ctx, s.exec)

	return feed, notFound(err)
}

// SaveFeed writes the feed's fields back.
func (s *Store) SaveFeed(ctx context.Context, feed *core.RSSFeed) error {
	_, err := feed.Update(ctx, s.exec, boil.Infer())
	return err
}

// FeedItemCount is the number of items stored for a feed.
func (s *Store) FeedItemCount(ctx context.Context, feedID string) (int64, error) {
	return core.RSSItems(core.RSSItemWhere.FeedID.EQ(feedID)).Count(ctx, s.exec)
}

// FeedSubscriptions returns every subscription to a feed.
func (s *Store) FeedSubscriptions(ctx context.Context, feedID string) (core.UserFeedSubscriptionSlice, error) {
	return core.UserFeedSubscriptions(core.UserFeedSubscriptionWhere.FeedID.EQ(feedID)).All(ctx, s.exec)
}

// FeedItemExists reports whether a feed has an item for a URL.
func (s *Store) FeedItemExists(ctx context.Context, feedID, urlID string) (bool, error) {
	return core.RSSItems(
		core.RSSItemWhere.FeedID.EQ(feedID),
		core.RSSItemWhere.URLID.EQ(urlID),
	).Exists(ctx, s.exec)
}

// UpsertFeedItem stores an item. If the feed already has one for the URL, the
// stored one is kept and created is false.
func (s *Store) UpsertFeedItem(ctx context.Context, item *core.RSSItem) (created bool, err error) {
	id := item.ID

	// pass true to get the existing id back; since callers hand in a fresh id,
	// an id that changed means the item was already there
	err = item.Upsert(ctx, s.exec, true, []string{core.RSSItemColumns.FeedID, core.RSSItemColumns.URLID}, boil.Whitelist(core.RSSItemColumns.FeedID), boil.Infer())
	if err != nil {
		return false, err
	}

	return id == item.ID, nil
}

// InsertUserFeedItem shows a stored item to a user.
func (s *Store) InsertUserFeedItem(ctx context.Context, userID, rssItemID, urlID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	item := core.UserFeedItem{ID: id.String(), UserID: userID, RSSItemID: rssItemID, URLID: urlID}

	return item.Insert(ctx, s.exec, boil.Infer())
}
