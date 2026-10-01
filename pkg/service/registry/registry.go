// Package registry builds every service over one store. It is part of the
// composition root: transports get their services from here and never see
// pkg/repo.
package registry

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
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
	Sender       repo.MailQueue
	MediaStorage server.MediaStorage
	// Site, SenderAddress and AdminAddress are what mail and its links are built from.
	Site          links.Site
	SenderAddress string
	AdminAddress  string
	// ProfileAboutMaxLength limits the About text; accounts' default when zero.
	ProfileAboutMaxLength int
	// PageSize and RSSLimit size the reading service's lists; zero means its
	// defaults.
	PageSize, RSSLimit int
	// CodeKey keys the HMAC of login codes. Empty means no login codes can
	// be issued or checked.
	CodeKey string
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
	ident := mail.Identity{Site: deps.Site, From: deps.SenderAddress, AdminAddress: deps.AdminAddress}

	feedSvc := feeds.New(store, deps.MediaStorage)

	return &Services{
		Connections: connections.New(store),
		Feeds:       feedSvc,
		Shares:      shares.New(store),
		Reading:     reading.New(store, reading.WithLimits(deps.PageSize, deps.RSSLimit)),
		Media:       media.New(store, deps.MediaStorage),
		Posts:       posts.New(store, deps.Sender, deps.MediaStorage, posts.WithIdentity(ident)),
		Accounts:    accounts.New(store, deps.Sender, feedSvc, accounts.WithIdentity(ident), accounts.WithProfileAboutMaxLength(deps.ProfileAboutMaxLength), accounts.WithCodeKey(deps.CodeKey)),
	}
}
