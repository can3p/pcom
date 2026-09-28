package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// RSSFeedOpt customizes an RSSFeed before it is inserted.
type RSSFeedOpt func(*core.RSSFeed)

// WithFeedURL overrides the made-up feed URL.
func WithFeedURL(url string) RSSFeedOpt {
	return func(f *core.RSSFeed) {
		f.URL = url
	}
}

// WithFeedTitle overrides the made-up feed title.
func WithFeedTitle(title string) RSSFeedOpt {
	return func(f *core.RSSFeed) {
		f.Title = null.StringFrom(title)
	}
}

// NextFetchAt sets when the poller should fetch the feed next.
func NextFetchAt(t time.Time) RSSFeedOpt {
	return func(f *core.RSSFeed) {
		f.NextFetchAt = null.TimeFrom(t)
	}
}

// WithoutTitle leaves the title empty, as for a feed that was never fetched.
func WithoutTitle() RSSFeedOpt {
	return func(f *core.RSSFeed) {
		f.Title = null.String{}
	}
}

// RSSFeed inserts a feed with a unique URL.
func RSSFeed(ctx context.Context, exec boil.ContextExecutor, opts ...RSSFeedOpt) (*core.RSSFeed, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	f := &core.RSSFeed{
		ID:                     id,
		URL:                    fmt.Sprintf("https://example.test/feed/%d.xml", n),
		Title:                  null.StringFrom(fmt.Sprintf("Test feed %d", n)),
		Description:            null.StringFrom("Test feed description"),
		UpdateFrequencyMinutes: 60,
	}

	for _, opt := range opts {
		opt(f)
	}

	if err := f.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return f, nil
}

// Subscription subscribes userID to feedID.
func Subscription(ctx context.Context, exec boil.ContextExecutor, userID, feedID string) (*core.UserFeedSubscription, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	s := &core.UserFeedSubscription{
		ID:     id,
		UserID: userID,
		FeedID: feedID,
	}

	if err := s.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return s, nil
}

// RSSItemOpt customizes an RSSItem before it is inserted.
type RSSItemOpt func(*core.RSSItem)

// WithURLID attaches the item to an already-created NormalizedURL instead of
// a freshly made-up one.
func WithURLID(urlID string) RSSItemOpt {
	return func(i *core.RSSItem) {
		i.URLID = urlID
	}
}

// WithItemTitle overrides the made-up item title.
func WithItemTitle(title string) RSSItemOpt {
	return func(i *core.RSSItem) {
		i.Title = title
	}
}

// RSSItem inserts an item published just now into feedID, making up a
// NormalizedURL for it unless WithURLID overrides that.
func RSSItem(ctx context.Context, exec boil.ContextExecutor, feedID string, opts ...RSSItemOpt) (*core.RSSItem, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	item := &core.RSSItem{
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

	if err := item.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return item, nil
}

// UserFeedItemOpt customizes a UserFeedItem before it is inserted.
type UserFeedItemOpt func(*core.UserFeedItem)

// IsDismissed marks the feed item as already dismissed by the user.
func IsDismissed() UserFeedItemOpt {
	return func(i *core.UserFeedItem) {
		i.IsDismissed = true
	}
}

// UserFeedItem inserts rssItemID into userID's feed, looking up the item's
// URL so a caller does not have to pass it separately.
func UserFeedItem(ctx context.Context, exec boil.ContextExecutor, userID, rssItemID string, opts ...UserFeedItemOpt) (*core.UserFeedItem, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	item, err := core.FindRSSItem(ctx, exec, rssItemID)
	if err != nil {
		return nil, err
	}

	i := &core.UserFeedItem{
		ID:        id,
		UserID:    userID,
		RSSItemID: rssItemID,
		URLID:     item.URLID,
	}

	for _, opt := range opts {
		opt(i)
	}

	if err := i.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return i, nil
}
