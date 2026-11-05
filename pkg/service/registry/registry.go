// Package registry builds every service over one store. It is part of the
// composition root: transports get their services from here and never see
// pkg/repo.
package registry

import (
	"database/sql"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/service/media"
	"github.com/can3p/pcom/pkg/service/posts"
	proofreadsvc "github.com/can3p/pcom/pkg/service/proofread"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"go.uber.org/zap"
)

// Deps is what services are built from besides the database.
type Deps struct {
	// Jobs is the queue services enqueue background jobs on.
	Jobs         repo.JobQueue
	MediaStorage server.MediaStorage
	// Site, SenderAddress and AdminAddress are what mail and its links are built from.
	Site          links.Site
	SenderAddress string
	AdminAddress  string
	// ProfileAboutMaxLength limits the About text; accounts' default when zero.
	ProfileAboutMaxLength int
	// The other text limits; each service's default when zero.
	CommentMaxLength, PostBodyMaxLength, PostSubjectMaxLength, PromptMaxLength, UserStylesMaxLength int
	// PageSize and RSSLimit size the reading service's lists; zero means its
	// defaults.
	PageSize, RSSLimit int
	// MaxUploadSize is the most bytes an uploaded or imported image may
	// have; media's default when zero.
	MaxUploadSize int64
	// FeedFetch bounds fetching RSS feeds; zero fields keep the reader's
	// defaults.
	FeedFetch reader.Limits
	// FeedPollInterval is how often due feeds are looked for; feeds'
	// default when zero.
	FeedPollInterval time.Duration
	// CodeKey keys the HMAC of login codes. Empty means no login codes can
	// be issued or checked.
	CodeKey string
	// Login is the limits of login by code; zero fields keep accounts'
	// defaults.
	Login accounts.LoginLimits
	// ProofreadModel answers grammar checks and style reviews; nil turns the
	// feature off.
	ProofreadModel proofread.Model
	// ProofreadLimits and ProofreadModels configure the proofread service;
	// zero fields keep its defaults. Its post text limits are
	// PostSubjectMaxLength and PostBodyMaxLength.
	ProofreadLimits proofreadsvc.Limits
	ProofreadModels proofreadsvc.Models
	// Logger is the logger of the services' background jobs; nil logs
	// nothing.
	Logger *zap.Logger
	// Subscriptions configures the email subscriptions service (its limits
	// and the address checker); none keeps its defaults.
	Subscriptions []subscriptions.Option
}

// Services is one field per area service.
type Services struct {
	Connections   *connections.Service
	Feeds         *feeds.Service
	Shares        *shares.Service
	Reading       *reading.Service
	Media         *media.Service
	Posts         *posts.Service
	Accounts      *accounts.Service
	Proofread     *proofreadsvc.Service
	Subscriptions *subscriptions.Service
}

func New(db *sql.DB, deps Deps) *Services {
	store := repo.New(db)
	ident := mail.Identity{Site: deps.Site, From: deps.SenderAddress, AdminAddress: deps.AdminAddress}

	feedSvc := feeds.New(store, deps.MediaStorage, feeds.WithFetchLimits(deps.FeedFetch), feeds.WithPollInterval(deps.FeedPollInterval), feeds.WithJobs(deps.Jobs), feeds.WithLogger(deps.Logger))

	postsSvc := posts.New(store, deps.MediaStorage, posts.WithJobs(deps.Jobs), posts.WithIdentity(ident), posts.WithMaxUploadSize(deps.MaxUploadSize), posts.WithTextLimits(posts.TextLimits{CommentMaxLength: deps.CommentMaxLength, PostBodyMaxLength: deps.PostBodyMaxLength, PostSubjectMaxLength: deps.PostSubjectMaxLength, PromptMaxLength: deps.PromptMaxLength}))

	return &Services{
		Connections:   connections.New(store),
		Feeds:         feedSvc,
		Shares:        shares.New(store),
		Reading:       reading.New(store, reading.WithLimits(deps.PageSize, deps.RSSLimit)),
		Media:         media.New(store, deps.MediaStorage, media.WithMaxUploadSize(deps.MaxUploadSize)),
		Posts:         postsSvc,
		Accounts:      accounts.New(store, feedSvc, accounts.WithJobs(deps.Jobs), accounts.WithIdentity(ident), accounts.WithProfileAboutMaxLength(deps.ProfileAboutMaxLength), accounts.WithUserStylesMaxLength(deps.UserStylesMaxLength), accounts.WithCodeKey(deps.CodeKey), accounts.WithLoginLimits(deps.Login)),
		Proofread:     proofreadsvc.New(store, postsSvc, deps.ProofreadModel, proofreadsvc.WithLimits(proofreadLimits(deps)), proofreadsvc.WithModels(deps.ProofreadModels)),
		Subscriptions: subscriptions.New(store, append([]subscriptions.Option{subscriptions.WithJobs(deps.Jobs), subscriptions.WithIdentity(ident)}, deps.Subscriptions...)...),
	}
}

// RegisterJobs registers the workers of every service's job kinds on r.
func (s *Services) RegisterJobs(r *jobs.Registry) {
	s.Accounts.RegisterJobs(r)
	s.Posts.RegisterJobs(r)
	s.Feeds.RegisterJobs(r)
	s.Subscriptions.RegisterJobs(r)
}

// proofreadLimits are the proofread service's limits, with the post form's
// text limits.
func proofreadLimits(deps Deps) proofreadsvc.Limits {
	l := deps.ProofreadLimits
	l.PostSubjectMaxLength = deps.PostSubjectMaxLength
	l.PostBodyMaxLength = deps.PostBodyMaxLength

	return l
}
