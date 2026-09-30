// Package registry builds every service over one store. It is part of the
// composition root: transports get their services from here and never see
// pkg/repo.
package registry

import (
	"context"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/service/media"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/jmoiron/sqlx"
)

// Deps is what services are built from besides the database.
type Deps struct {
	Sender       sender.Sender
	MediaStorage server.MediaStorage
}

// Services is one field per area service.
type Services struct {
	Connections *connections.Service
	Feeds       *feeds.Service
	Shares      *shares.Service
	Reading     *reading.Service
	Media       *media.Service
	Posts       *posts.Service
	Accounts    *accounts.Service
}

func New(db *sqlx.DB, deps Deps) *Services {
	store := repo.New(db)

	return &Services{
		Connections: connections.New(store),
		Feeds:       feeds.New(store, deps.MediaStorage),
		Shares:      shares.New(store),
		Reading:     reading.New(store),
		Media:       media.New(store, deps.MediaStorage),
		Posts:       posts.New(store, deps.Sender, deps.MediaStorage),
		Accounts: accounts.New(store, deps.Sender, func(ctx context.Context, userID string) ([]*feedops.RssFeed, error) {
			return feedops.GetRssFeeds(ctx, db, userID)
		}),
	}
}
