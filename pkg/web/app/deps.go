// Package app assembles the web server: it wires the handlers of every area
// onto one gin engine. cmd/web builds the dependencies and runs the result.
package app

import (
	"context"
	"database/sql"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"time"

	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/proofread/claude"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	proofreadsvc "github.com/can3p/pcom/pkg/service/proofread"
	"github.com/can3p/pcom/pkg/service/registry"
	"go.uber.org/zap"
)

// Deps is everything the handlers need. RS replaces DB with the services.
type Deps struct {
	DB *sql.DB
	// Services is built from DB by New when it is nil.
	Services *registry.Services
	// Guard keeps scripts out of the public forms that mail an address;
	// New builds it from Config.Abuse and Config.SessionSalt when it is nil.
	Guard *AbuseGuard
	// Jobs is the queue services enqueue background jobs on.
	Jobs         repo.JobQueue
	MediaStorage server.MediaStorage
	MediaServer  server.MediaServer
	// Logger is the base of every request's logger; nil logs nothing.
	Logger *zap.Logger
	Config Config
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
	// CommentMaxLength, PostBodyMaxLength, PostSubjectMaxLength,
	// PromptMaxLength and UserStylesMaxLength limit the other texts.
	CommentMaxLength, PostBodyMaxLength, PostSubjectMaxLength, PromptMaxLength, UserStylesMaxLength int
	// SecureCookies marks the session cookie Secure.
	SecureCookies bool
	// SessionMaxAge is how long a session lasts after its last save;
	// pgsession.DefaultMaxAge when zero.
	SessionMaxAge time.Duration
	// HSTS sends Strict-Transport-Security.
	HSTS bool
	// StaticCache serves /static with a long immutable Cache-Control.
	StaticCache bool
	// DevRoutes mounts the /dev pages (mail previews); local development only.
	DevRoutes bool
	// ShowErrors shows error details in pages.
	ShowErrors bool
	// ReportPanics mails AdminAddress when a page panics.
	ReportPanics bool
	// MaxBodySize caps every request body; MaxUploadSize an uploaded image
	// and MaxImportSize a blog archive to import, on top of it. Zero keeps
	// DefaultMaxBodySize, media.DefaultMaxUploadSize and
	// DefaultMaxImportSize.
	MaxBodySize, MaxUploadSize, MaxImportSize int64
	// FeedFetch bounds fetching RSS feeds; zero fields keep the reader's
	// defaults.
	FeedFetch reader.Limits
	// FeedPollInterval is how often due feeds are looked for; zero keeps
	// feeds.DefaultPollInterval.
	FeedPollInterval time.Duration
	// PageSize is how many items a page of a list holds, RSSLimit how many
	// items an RSS output lists; zero means the reading service's default.
	PageSize, RSSLimit int
	// Login is the limits of login by code; zero fields keep accounts'
	// defaults.
	Login accounts.LoginLimits
	// Proofread is the grammar check and style review; an empty
	// Claude.APIKey turns the feature off.
	Proofread Proofread
	// Subscriptions configures the email subscriptions service: its limits
	// and the address checker.
	Subscriptions []subscriptions.Option
	// Abuse configures the guard of the public forms (pkg/abuse): proof-of-work
	// difficulty, challenge age and the least time to fill a form. The guard
	// is built with the session salt as its key where a form needs it.
	Abuse []abuse.Option
}

// Proofread is the settings of the grammar check and style review: the
// Claude client's, and the service's limits and models (zero fields keep the
// service's defaults; the post text limits come from Config).
type Proofread struct {
	Claude claude.Options
	Limits proofreadsvc.Limits
	Models proofreadsvc.Models
}

// AbuseGuard is pkg/abuse's guard with the store that remembers which
// challenges were answered, so a handler verifies a submission with one call.
type AbuseGuard struct {
	*abuse.Guard
	store *repo.Store
}

// NewAbuseGuard builds the guard of d's public forms.
func NewAbuseGuard(d *Deps) *AbuseGuard {
	return &AbuseGuard{Guard: abuse.New(d.Config.SessionSalt, d.Config.Abuse...), store: repo.New(d.DB)}
}

// Verify checks a submitted proof of work, honeypot and fill time; see
// abuse.Guard.Verify.
func (g *AbuseGuard) Verify(ctx context.Context, sub abuse.Submission) error {
	return g.Guard.Verify(ctx, g.store, sub)
}

// RunPruner forgets answered challenges too old to be answered again, every
// `every` (subscriptions.DefaultPruneEvery when zero), until ctx is done. A
// failure is retried on the next round.
func (g *AbuseGuard) RunPruner(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = subscriptions.DefaultPruneEvery
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		_ = g.Prune(ctx, g.store)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
