package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/uptrace/bun"
)

// UpsertFeed returns the feed with this (already normalized) URL, creating it
// if nobody subscribed to it yet.
func (s *Store) UpsertFeed(ctx context.Context, normalizedURL string) (*model.RSSFeed, error) {
	feedID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	feed := &model.RSSFeed{ID: feedID.String(), URL: normalizedURL, UpdatedAt: time.Now()}

	// we update only url on upsert, since we don't really want to update anything;
	// RETURNING brings back the existing row, so the id is the stored one
	_, err = s.query().NewInsert().Model(feed).
		On("CONFLICT (url) DO UPDATE").
		Set("url = EXCLUDED.url").
		Returning("*").
		Exec(ctx)
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

	sub := &model.UserFeedSubscription{ID: id.String(), FeedID: feedID, UserID: userID, UpdatedAt: time.Now()}

	_, err = s.query().NewInsert().Model(sub).
		On("CONFLICT (user_id, feed_id) DO UPDATE").
		Set("user_id = EXCLUDED.user_id").
		Set("feed_id = EXCLUDED.feed_id").
		Set("created_at = EXCLUDED.created_at").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)

	return err
}

// DeleteSubscription removes a user's subscription. The feed row stays: the
// poller skips feeds without subscribers, and this avoids racing a user who
// adds the feed back.
func (s *Store) DeleteSubscription(ctx context.Context, userID, subscriptionID string) error {
	_, err := s.query().NewDelete().Model((*model.UserFeedSubscription)(nil)).
		Where("id = ?", subscriptionID).
		Where("user_id = ?", userID).
		Exec(ctx)

	return err
}

// SubscriptionsOf returns a user's subscriptions oldest first, with the feed
// loaded (sub.Feed).
func (s *Store) SubscriptionsOf(ctx context.Context, userID string) ([]*model.UserFeedSubscription, error) {
	var subs []*model.UserFeedSubscription

	err := s.query().NewSelect().Model(&subs).
		Relation("Feed").
		Where("?TableAlias.user_id = ?", userID).
		OrderExpr("?TableAlias.id ASC").
		Scan(ctx)

	return subs, err
}

// LastImportedAt maps each feed to the time its newest item was stored.
// Feeds without items are missing from the map.
func (s *Store) LastImportedAt(ctx context.Context, feedIDs []string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	if len(feedIDs) == 0 {
		return out, nil
	}

	var latest []struct {
		FeedID    string    `bun:"feed_id"`
		CreatedAt time.Time `bun:"created_at"`
	}

	err := s.query().NewSelect().Model((*model.RSSItem)(nil)).
		ColumnExpr("feed_id").
		ColumnExpr("MAX(created_at) AS created_at").
		Where("feed_id IN (?)", bun.List(feedIDs)).
		GroupExpr("feed_id").
		Scan(ctx, &latest)
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
func (s *Store) UndismissedFeedItems(ctx context.Context, userID string, page Page) ([]*model.UserFeedItem, error) {
	var items []*model.UserFeedItem

	q := s.query().NewSelect().Model(&items).
		Where("?TableAlias.user_id = ?", userID).
		Where("?TableAlias.is_dismissed = ?", false).
		Relation("RSSItem.Feed").
		Relation("URL")

	err := page.apply(q, KindRSSItem, "?TableAlias.created_at", "?TableAlias.id").Scan(ctx)

	return items, err
}

// UserFeedItemByID returns a user's feed item, or ErrNotFound if the user has
// no such item.
func (s *Store) UserFeedItemByID(ctx context.Context, userID, itemID string) (*model.UserFeedItem, error) {
	item := new(model.UserFeedItem)

	err := s.query().NewSelect().Model(item).
		Where("id = ?", itemID).
		Where("user_id = ?", userID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return item, nil
}

// DismissFeedItem marks a feed item as dismissed.
func (s *Store) DismissFeedItem(ctx context.Context, item *model.UserFeedItem) error {
	item.IsDismissed = true

	_, err := s.query().NewUpdate().Model(item).WherePK().Exec(ctx)

	return err
}

// FeedsToRefresh returns the feeds due for a fetch that have at least one
// subscriber.
func (s *Store) FeedsToRefresh(ctx context.Context) ([]*model.RSSFeed, error) {
	var feeds []*model.RSSFeed

	err := s.query().NewSelect().Model(&feeds).
		Where("next_fetch_at < ?", time.Now()).
		WhereOr("next_fetch_at IS NULL").
		Relation("FeedUserFeedSubscriptions", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Limit(1)
		}).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return lo.Filter(feeds, func(f *model.RSSFeed, _ int) bool {
		return len(f.FeedUserFeedSubscriptions) > 0
	}), nil
}

// LockFeed loads a feed and locks its row until the transaction ends. A feed
// another transaction holds is skipped, which reads as ErrNotFound.
func (s *Store) LockFeed(ctx context.Context, feedID string) (*model.RSSFeed, error) {
	feed := new(model.RSSFeed)

	err := s.query().NewSelect().Model(feed).
		Where("id = ?", feedID).
		For("UPDATE SKIP LOCKED").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return feed, nil
}

// SaveFeed writes the feed's fields back.
func (s *Store) SaveFeed(ctx context.Context, feed *model.RSSFeed) error {
	_, err := s.query().NewUpdate().Model(feed).WherePK().Exec(ctx)

	return err
}

// FeedItemCount is the number of items stored for a feed.
func (s *Store) FeedItemCount(ctx context.Context, feedID string) (int64, error) {
	n, err := s.query().NewSelect().Model((*model.RSSItem)(nil)).
		Where("feed_id = ?", feedID).
		Count(ctx)

	return int64(n), err
}

// FeedSubscriptions returns every subscription to a feed.
func (s *Store) FeedSubscriptions(ctx context.Context, feedID string) ([]*model.UserFeedSubscription, error) {
	var subs []*model.UserFeedSubscription

	err := s.query().NewSelect().Model(&subs).
		Where("feed_id = ?", feedID).
		Scan(ctx)

	return subs, err
}

// FeedItemExists reports whether a feed has an item for a URL.
func (s *Store) FeedItemExists(ctx context.Context, feedID, urlID string) (bool, error) {
	return s.query().NewSelect().Model((*model.RSSItem)(nil)).
		Where("feed_id = ?", feedID).
		Where("url_id = ?", urlID).
		Exists(ctx)
}

// UpsertFeedItem stores an item. If the feed already has one for the URL, the
// stored one is kept and created is false.
func (s *Store) UpsertFeedItem(ctx context.Context, item *model.RSSItem) (created bool, err error) {
	id := item.ID
	item.UpdatedAt = time.Now()

	// RETURNING brings the existing id back; since callers hand in a fresh id,
	// an id that changed means the item was already there
	_, err = s.query().NewInsert().Model(item).
		On("CONFLICT (feed_id, url_id) DO UPDATE").
		Set("feed_id = EXCLUDED.feed_id").
		Returning("*").
		Exec(ctx)
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

	item := &model.UserFeedItem{ID: id.String(), UserID: userID, RSSItemID: rssItemID, URLID: urlID}

	_, err = s.query().NewInsert().Model(item).Exec(ctx)

	return err
}
