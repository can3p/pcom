// Package app assembles the web server: it wires the handlers of every area
// onto one gin engine. cmd/web builds the dependencies and runs the result.
package app

import (
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/jmoiron/sqlx"
)

// Deps is everything the handlers need. RS replaces DB with the services.
type Deps struct {
	DB *sqlx.DB
	// Services is built from DB by New when it is nil.
	Services     *registry.Services
	Sender       repo.MailQueue
	MediaStorage server.MediaStorage
	MediaServer  server.MediaServer
	Config       Config
}

// Config carries the choices main() makes from flags and the environment.
type Config struct {
	// HTMLDir is the template directory, relative to the process cwd.
	HTMLDir string
	// ForceOpenRegistration allows signups even when system settings close them.
	ForceOpenRegistration bool
	// SessionSalt signs the session cookies.
	SessionSalt string
	// StaticAsset resolves a frontend asset name to its URL; see LoadStaticManifest.
	StaticAsset StaticAssetFunc

	// SiteRoot is the public root URL; absolute links start with it.
	SiteRoot string
	// StaticCDN and MediaCDN are the origins that serve /static and user
	// media; empty means the app serves them.
	StaticCDN, MediaCDN string
	// SenderAddress is the From of every mail; AdminAddress receives admin
	// notifications.
	SenderAddress, AdminAddress string
	// ProfileAboutMaxLength is the most characters a user's About text may have.
	ProfileAboutMaxLength int
	// SecureCookies marks the session cookie Secure.
	SecureCookies bool
	// HSTS sends Strict-Transport-Security.
	HSTS bool
	// StaticCache serves /static with a long immutable Cache-Control.
	StaticCache bool
	// ShowErrors shows error details in pages.
	ShowErrors bool
	// ReportPanics mails AdminAddress when a page panics.
	ReportPanics bool
	// PageSize is how many items a page of a list holds, RSSLimit how many
	// items an RSS output lists; zero means the reading service's default.
	PageSize, RSSLimit int
	// Login is the limits of login by code; zero fields keep accounts'
	// defaults.
	Login accounts.LoginLimits
}
