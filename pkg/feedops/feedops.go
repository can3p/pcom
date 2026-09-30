// Package feedops is what is left of the RSS code before pkg/service/feeds:
// wrappers for callers that have not moved to the service yet (the account
// and reading pages, cmd/web). Delete it with the last of them.
package feedops

import (
	"context"

	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/jmoiron/sqlx"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type (
	RssFeed     = feeds.RssFeed
	RssFeedItem = feeds.RssFeedItem
)

func GetRssFeeds(ctx context.Context, db boil.ContextExecutor, userID string) ([]*RssFeed, error) {
	return feeds.New(repo.Using(db), nil).Subscriptions(ctx, &core.User{ID: userID})
}

func GetRssFeedItems(ctx context.Context, db boil.ContextExecutor, userID string) ([]*RssFeedItem, error) {
	return feeds.New(repo.Using(db), nil).Items(ctx, &core.User{ID: userID})
}

// DefaultRssReader is the feed poller: call RunPoller on it.
func DefaultRssReader(db *sqlx.DB, mediaStorage server.MediaStorage) *feeds.Service {
	return feeds.New(repo.New(db), mediaStorage)
}
