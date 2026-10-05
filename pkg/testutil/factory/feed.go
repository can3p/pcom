package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/repo"

	"github.com/can3p/pcom/pkg/model"
)

// RSSFeedOpt customizes an RSSFeed before it is inserted.
type RSSFeedOpt func(*model.RSSFeed)

// WithFeedURL overrides the made-up feed URL.
func WithFeedURL(url string) RSSFeedOpt {
	return func(f *model.RSSFeed) {
		f.URL = url
	}
}

// WithFeedTitle overrides the made-up feed title.
func WithFeedTitle(title string) RSSFeedOpt {
	return func(f *model.RSSFeed) {
		f.Title = new(title)
	}
}

// NextFetchAt sets when the poller should fetch the feed next.
func NextFetchAt(t time.Time) RSSFeedOpt {
	return func(f *model.RSSFeed) {
		f.NextFetchAt = new(t)
	}
}

// WithoutTitle leaves the title empty, as for a feed that was never fetched.
func WithoutTitle() RSSFeedOpt {
	return func(f *model.RSSFeed) {
		f.Title = nil
	}
}

// RSSFeed inserts a feed with a unique URL.
func RSSFeed(ctx context.Context, exec repo.Executor, opts ...RSSFeedOpt) (*model.RSSFeed, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	f := &model.RSSFeed{
		ID:                     id,
		URL:                    fmt.Sprintf("https://example.test/feed/%d.xml", n),
		Title:                  new(fmt.Sprintf("Test feed %d", n)),
		Description:            new("Test feed description"),
		UpdateFrequencyMinutes: 60,
	}

	for _, opt := range opts {
		opt(f)
	}

	return insertRow(ctx, exec, f)
}

// Subscription subscribes userID to feedID.
func Subscription(ctx context.Context, exec repo.Executor, userID, feedID string) (*model.UserFeedSubscription, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	s := &model.UserFeedSubscription{
		ID:     id,
		UserID: userID,
		FeedID: feedID,
	}

	return insertRow(ctx, exec, s)
}

// RSSItemOpt customizes an RSSItem before it is inserted.
type RSSItemOpt func(*model.RSSItem)

// WithURLID attaches the item to an already-created NormalizedURL instead of
// a freshly made-up one.
func WithURLID(urlID string) RSSItemOpt {
	return func(i *model.RSSItem) {
		i.URLID = urlID
	}
}

// WithItemTitle overrides the made-up item title.
func WithItemTitle(title string) RSSItemOpt {
	return func(i *model.RSSItem) {
		i.Title = title
	}
}

// RSSItem inserts an item published just now into feedID, making up a
// NormalizedURL for it unless WithURLID overrides that.
func RSSItem(ctx context.Context, exec repo.Executor, feedID string, opts ...RSSItemOpt) (*model.RSSItem, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	item := &model.RSSItem{
		ID:                   id,
		FeedID:               feedID,
		GUID:                 fmt.Sprintf("test-guid-%d", n),
		Title:                fmt.Sprintf("Test item %d", n),
		Description:          "Test item description",
		SanitizedDescription: "Test item description",
		PublishedAt:          time.Now(),
	}

	for _, opt := range opts {
		opt(item)
	}

	if item.URLID == "" {
		url, err := NormalizedURL(ctx, exec)
		if err != nil {
			return nil, err
		}

		item.URLID = url.ID
	}

	return insertRow(ctx, exec, item)
}

// UserFeedItemOpt customizes a UserFeedItem before it is inserted.
type UserFeedItemOpt func(*model.UserFeedItem)

// IsDismissed marks the feed item as already dismissed by the user.
func IsDismissed() UserFeedItemOpt {
	return func(i *model.UserFeedItem) {
		i.IsDismissed = true
	}
}

// FeedItemCreatedAt backdates the moment the item entered the user's feed,
// which orders it there.
func FeedItemCreatedAt(t time.Time) UserFeedItemOpt {
	return func(i *model.UserFeedItem) {
		i.CreatedAt = t
	}
}

// UserFeedItem inserts rssItemID into userID's feed, looking up the item's
// URL so a caller does not have to pass it separately.
func UserFeedItem(ctx context.Context, exec repo.Executor, userID, rssItemID string, opts ...UserFeedItemOpt) (*model.UserFeedItem, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	item, err := find(ctx, exec, &model.RSSItem{ID: rssItemID})
	if err != nil {
		return nil, err
	}

	i := &model.UserFeedItem{
		ID:        id,
		UserID:    userID,
		RSSItemID: rssItemID,
		URLID:     item.URLID,
	}

	for _, opt := range opts {
		opt(i)
	}

	return insertRow(ctx, exec, i)
}
