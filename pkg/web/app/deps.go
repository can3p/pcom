// Package app assembles the web server: it wires the handlers of every area
// onto one gin engine. cmd/web builds the dependencies and runs the result.
package app

import (
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/jmoiron/sqlx"
)

// Deps is everything the handlers need. RS replaces DB with the services.
type Deps struct {
	DB           *sqlx.DB
	Sender       sender.Sender
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
	// InCluster turns on production behavior: the page failure reporter,
	// long-lived static caching and secure session cookies.
	InCluster bool
	// SessionSalt signs the session cookies.
	SessionSalt string
	// StaticAsset resolves a frontend asset name to its URL; see LoadStaticManifest.
	StaticAsset StaticAssetFunc
}
