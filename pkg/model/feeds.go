package model

import (
	"context"
	"fmt"
	"github.com/uptrace/bun"
	"slices"
	"time"
)

// RSSFeedDisableReason is the Postgres enum rss_feed_disable_reason.
type RSSFeedDisableReason string

// The values of RSSFeedDisableReason.
const (
	RSSFeedDisableReasonFetchFailure  RSSFeedDisableReason = "fetch_failure"
	RSSFeedDisableReasonNoSubscribers RSSFeedDisableReason = "no_subscribers"
)

// AllRSSFeedDisableReason returns every value of RSSFeedDisableReason, in the enum's order.
func AllRSSFeedDisableReason() []RSSFeedDisableReason {
	return []RSSFeedDisableReason{RSSFeedDisableReasonFetchFailure, RSSFeedDisableReasonNoSubscribers}
}

// IsValid reports an error when e is not a value of RSSFeedDisableReason.
func (e RSSFeedDisableReason) IsValid() error {
	if !slices.Contains(AllRSSFeedDisableReason(), e) {
		return fmt.Errorf("enum RSSFeedDisableReason: %q is not valid", string(e))
	}

	return nil
}

func (e RSSFeedDisableReason) String() string {
	return string(e)
}

// RSSFeed is a row of rss_feeds.
type RSSFeed struct {
	bun.BaseModel `bun:"table:rss_feeds"`

	ID                      string                `bun:"id,pk"`
	URL                     string                `bun:"url"`
	Title                   *string               `bun:"title"`
	Description             *string               `bun:"description"`
	LastFetchedAt           *time.Time            `bun:"last_fetched_at"`
	AvgItemsPerDay          float64               `bun:"avg_items_per_day"`
	LastItemsCount          int                   `bun:"last_items_count,default:0"`
	UpdateFrequencyMinutes  int                   `bun:"update_frequency_minutes"`
	NextFetchAt             *time.Time            `bun:"next_fetch_at"`
	LastManualRefreshAt     *time.Time            `bun:"last_manual_refresh_at"`
	ConsecutiveEmptyFetches int                   `bun:"consecutive_empty_fetches"`
	CreatedAt               time.Time             `bun:"created_at"`
	UpdatedAt               time.Time             `bun:"updated_at"`
	LastFetchError          *string               `bun:"last_fetch_error"`
	DisableReason           *RSSFeedDisableReason `bun:"disable_reason"`

	FeedUserFeedSubscriptions []*UserFeedSubscription `bun:"rel:has-many,join:id=feed_id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *RSSFeed) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// RSSItem is a row of rss_items.
type RSSItem struct {
	bun.BaseModel `bun:"table:rss_items"`

	ID                   string    `bun:"id,pk"`
	FeedID               string    `bun:"feed_id"`
	URLID                string    `bun:"url_id"`
	GUID                 string    `bun:"guid"`
	Title                string    `bun:"title"`
	Description          string    `bun:"description"`
	SanitizedDescription string    `bun:"sanitized_description"`
	PublishedAt          time.Time `bun:"published_at"`
	CreatedAt            time.Time `bun:"created_at"`
	UpdatedAt            time.Time `bun:"updated_at"`

	Feed *RSSFeed       `bun:"rel:belongs-to,join:feed_id=id"`
	URL  *NormalizedURL `bun:"rel:belongs-to,join:url_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *RSSItem) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserFeedItem is a row of user_feed_items.
type UserFeedItem struct {
	bun.BaseModel `bun:"table:user_feed_items"`

	ID          string    `bun:"id,pk"`
	UserID      string    `bun:"user_id"`
	RSSItemID   string    `bun:"rss_item_id"`
	URLID       string    `bun:"url_id"`
	IsDismissed bool      `bun:"is_dismissed"`
	CreatedAt   time.Time `bun:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at"`

	RSSItem *RSSItem       `bun:"rel:belongs-to,join:rss_item_id=id"`
	URL     *NormalizedURL `bun:"rel:belongs-to,join:url_id=id"`
	User    *User          `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserFeedItem) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserFeedSubscription is a row of user_feed_subscriptions.
type UserFeedSubscription struct {
	bun.BaseModel `bun:"table:user_feed_subscriptions"`

	ID        string    `bun:"id,pk"`
	UserID    string    `bun:"user_id"`
	FeedID    string    `bun:"feed_id"`
	CreatedAt time.Time `bun:"created_at"`
	UpdatedAt time.Time `bun:"updated_at"`

	Feed *RSSFeed `bun:"rel:belongs-to,join:feed_id=id"`
	User *User    `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserFeedSubscription) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserFeedToken is a row of user_feed_tokens.
type UserFeedToken struct {
	bun.BaseModel `bun:"table:user_feed_tokens"`

	ID        string    `bun:"id,pk"`
	UserID    string    `bun:"user_id"`
	Token     string    `bun:"token"`
	CreatedAt time.Time `bun:"created_at,default:now()"`
	UpdatedAt time.Time `bun:"updated_at,default:now()"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserFeedToken) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}
