package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/gogo/sender/mailjet"
	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/media/server/storage/s3"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/proofread/claude"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	proofreadsvc "github.com/can3p/pcom/pkg/service/proofread"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	MediaServerConcurrency = 3
	pprofAddr              = ":8081"
)

type serveCmd struct {
	config.Serve
}

func (c *serveCmd) Execute([]string) error {
	cfg := c.Serve

	applyProcessSettings(cfg)

	logger := newLogger(cfg.Web, os.Stdout)
	defer func() { _ = logger.Sync() }()

	db, closeDB, err := openDB(cfg.Database.URL, logger)
	if err != nil {
		return err
	}
	defer closeDB()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	delivery := mailjet.NewSenderFromConfig(&cfg.Mail.Mailjet)

	mediaStorage, err := newMediaStorage(cfg.Media)
	if err != nil {
		return err
	}

	go pgsession.RunCleanup(ctx, db, cfg.Web.SessionCleanupInterval, logger)

	go accounts.New(repo.New(db), nil, accounts.WithLoginLimits(loginLimits(cfg.Login)), accounts.WithLogger(logger)).RunPruner(ctx)

	// The subscriptions service checks that an address's domain can receive mail.
	emailChecker := emailaddr.NewChecker(emailaddr.WithTimeout(cfg.Email.MXTimeout))
	_ = emailChecker // TODO(F11.T0): pass to the subscriptions service

	mediaServer, mediaServerCleanup, err := newMediaServer(mediaStorage, cfg.Web)
	if err != nil {
		return err
	}
	defer mediaServerCleanup()

	// developer timezone only messes things up
	time.Local = time.UTC

	inserter, err := jobs.NewInserter(db)
	if err != nil {
		return err
	}

	deps := &app.Deps{
		DB:           db,
		Jobs:         inserter,
		MediaStorage: mediaStorage,
		MediaServer:  mediaServer,
		Logger:       logger,
		Config:       appConfig(cfg, app.LoadStaticManifest(cfg.Web.StaticCDN)),
	}
	deps.Services = app.NewServices(deps)

	stopJobs, err := startJobs(ctx, db, deps.Services, delivery, cfg, logger)
	if err != nil {
		return err
	}
	defer stopJobs()

	startPprof(cfg.Web.EnablePprof.On(), pprofAddr, logger)

	srv := app.NewServer(fmt.Sprintf(":%d", cfg.Web.Port), app.New(deps).Handler(), serverTimeouts(cfg.Web))

	return serveUntilDone(ctx, srv, cfg.Web.ShutdownTimeout, logger)
}

// serveUntilDone runs srv until ctx ends, then lets the requests in flight
// finish for up to shutdownTimeout.
func serveUntilDone(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration, logger *zap.Logger) error {
	errc := make(chan error, 1)

	go func() { errc <- srv.ListenAndServe() }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}

	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// startJobs starts the background job runner with every service's job
// kinds; the function it returns stops it, waiting up to the shutdown
// timeout for the jobs in progress.
func startJobs(ctx context.Context, db *sql.DB, services *registry.Services, delivery sender.Sender, cfg config.Serve, logger *zap.Logger) (func(), error) {
	reg := jobs.NewRegistry(repo.New(db), delivery, jobs.WithRetryIntervals(cfg.Mail.RetryIntervals))
	services.RegisterJobs(reg)

	runner, err := jobs.NewRunner(db, reg, jobs.WithPollInterval(cfg.Jobs.PollInterval), jobs.WithWorkers(cfg.Jobs.Workers), jobs.WithRetention(cfg.Jobs.Retention), jobs.WithLogger(logger))
	if err != nil {
		return nil, err
	}

	if err := runner.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting the job runner: %w", err)
	}

	return func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		if err := runner.Stop(stopCtx); err != nil {
			logger.Warn("failed to stop the job runner", zap.Error(err))
		}
	}, nil
}

// serverTimeouts are the app server's timeouts.
func serverTimeouts(web config.Web) app.ServerTimeouts {
	return app.ServerTimeouts{
		ReadHeader: web.ReadHeaderTimeout,
		Read:       web.ReadTimeout,
		Write:      web.WriteTimeout,
		Idle:       web.IdleTimeout,
	}
}

// feedLimits are the reader's view of the feed settings.
func feedLimits(cfg config.Feeds) reader.Limits {
	return reader.Limits{
		FeedTimeout:  cfg.FetchTimeout,
		MediaTimeout: cfg.ImageTimeout,
		MaxFeedSize:  cfg.MaxFeedSize,
	}
}

// applyProcessSettings sets the process-wide gin mode.
func applyProcessSettings(cfg config.Serve) {
	if cfg.Web.GinMode != "" {
		gin.SetMode(cfg.Web.GinMode)
	}
}

// newLogger is the server's logger, writing to w at --log-level in
// --log-format.
func newLogger(web config.Web, w io.Writer) *zap.Logger {
	return logging.New(web.ZapLevel(), web.LogFormat, w)
}

// appConfig maps the settings onto what the app needs.
func appConfig(cfg config.Serve, staticAsset app.StaticAssetFunc) app.Config {
	return app.Config{
		HTMLDir:               cfg.Web.HTMLDir,
		ForceOpenRegistration: cfg.Web.ForceSignup.On(),
		SessionSalt:           cfg.Web.SessionSalt.Reveal(),
		StaticAsset:           staticAsset,
		SiteRoot:              cfg.Web.SiteRoot,
		StaticCDN:             cfg.Web.StaticCDN,
		MediaCDN:              cfg.Media.CDN,
		SenderAddress:         cfg.Mail.SenderAddress,
		AdminAddress:          cfg.Mail.AdminAddress,
		SecureCookies:         cfg.Web.SecureCookies.On(),
		SessionMaxAge:         cfg.Web.SessionMaxAge,
		HSTS:                  cfg.Web.HSTS.On(),
		StaticCache:           cfg.Web.StaticCache.On(),
		DevRoutes:             cfg.Web.DevRoutes.On(),
		ShowErrors:            cfg.Web.ShowErrors.On(),
		ReportPanics:          cfg.Web.ReportPanics.On(),
		ProfileAboutMaxLength: cfg.Limits.ProfileAboutMaxLength,
		CommentMaxLength:      cfg.Limits.CommentMaxLength,
		PostBodyMaxLength:     cfg.Limits.PostBodyMaxLength,
		PostSubjectMaxLength:  cfg.Limits.PostSubjectMaxLength,
		PromptMaxLength:       cfg.Limits.PromptMaxLength,
		UserStylesMaxLength:   cfg.Limits.UserStylesMaxLength,
		PageSize:              cfg.Limits.PageSize,
		RSSLimit:              cfg.Limits.RSSLimit,
		MaxBodySize:           cfg.Limits.MaxBodySize,
		MaxUploadSize:         cfg.Limits.MaxUploadSize,
		MaxImportSize:         cfg.Limits.MaxImportSize,
		FeedFetch:             feedLimits(cfg.Feeds),
		FeedPollInterval:      cfg.Feeds.PollInterval,
		Login:                 loginLimits(cfg.Login),
		Proofread:             proofreadConfig(cfg.Proofread),
	}
}

// proofreadConfig is the app's view of the proofread settings.
func proofreadConfig(cfg config.Proofread) app.Proofread {
	return app.Proofread{
		Claude: claude.Options{
			APIKey:           cfg.AnthropicKey.Reveal(),
			Endpoint:         cfg.AnthropicEndpoint,
			Timeout:          cfg.Timeout,
			GrammarMaxTokens: cfg.GrammarMaxTokens,
			StyleMaxTokens:   cfg.StyleMaxTokens,
		},
		Limits: proofreadsvc.Limits{
			GrammarPerHour:    cfg.GrammarPerHour,
			StylePerHour:      cfg.StylePerHour,
			SiteDailyRequests: cfg.SiteDailyRequests,
			NoteMaxLength:     cfg.NoteMaxLength,
		},
		Models: proofreadsvc.Models{
			Grammar: proofread.ModelChoice{Name: cfg.GrammarModel, Effort: proofread.Effort(cfg.GrammarEffort)},
			Style:   proofread.ModelChoice{Name: cfg.StyleModel, Effort: proofread.Effort(cfg.StyleEffort)},
		},
	}
}

// loginLimits is the accounts service's view of the login settings.
func loginLimits(cfg config.Login) accounts.LoginLimits {
	return accounts.LoginLimits{
		CodeLifetime:        cfg.CodeLifetime,
		CodeTries:           cfg.CodeTries,
		CodesMailed:         cfg.CodesMailed,
		CodesMailedWindow:   cfg.CodesMailedWindow,
		WrongTries:          cfg.WrongTries,
		WrongTriesWindow:    cfg.WrongTriesWindow,
		UnconfirmedLifetime: cfg.UnconfirmedLifetime,
		PruneEvery:          cfg.PruneEvery,
	}
}

// newMediaStorage is the S3 bucket user media lives in.
func newMediaStorage(cfg config.Media) (server.MediaStorage, error) {
	return s3.New(s3.Options{
		Endpoint:  cfg.Endpoint,
		Bucket:    cfg.Bucket,
		Region:    cfg.Region,
		Key:       cfg.Key,
		Secret:    cfg.Secret.Reveal(),
		PathStyle: cfg.PathStyle.On(),
		Timeout:   cfg.Timeout,
	})
}

// newMediaServer serves user media in the thumb and full classes; the caller
// runs the returned cleanup on exit.
func newMediaServer(mediaStorage server.MediaStorage, web config.Web) (server.MediaServer, func(), error) {
	var mediaServer server.MediaServer

	mediaServer, mediaServerCleanup, err := server.New(mediaStorage,
		server.WithClass("thumb", server.ClassParams{Width: 720, Height: 540}),
		server.WithClass("full", server.ClassParams{Width: 1200, Height: 900}),
		server.WithPermaCache(web.MediaPermaCache.On()),
		server.WithClassResolver(func(c context.Context, req *http.Request) string {
			// we know that the context is gin
			ginCtx := c.(*gin.Context)
			return ginCtx.Param("class")
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("starting the media server: %w", err)
	}

	mediaServer = server.NewWrapper(mediaServer, MediaServerConcurrency)

	mediaServer, err = server.NewCachingServer(mediaServer, mediaStorage, 0)
	if err != nil {
		mediaServerCleanup()
		return nil, nil, fmt.Errorf("starting the caching media server: %w", err)
	}

	return mediaServer, mediaServerCleanup, nil
}

// startPprof moves the pprof handlers off the default mux and, when enabled,
// serves them on addr.
func startPprof(enabled bool, addr string, logger *zap.Logger) {
	pprofMux := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()

	if enabled {
		go func() {
			// fly.toml publishes only the app's port; this one is reachable from
			// the private network alone.
			srv := &http.Server{ //nolint:gosec // G112: private debug listener
				Addr:    addr,
				Handler: pprofMux,
			}

			logger.Fatal("pprof server stopped", zap.Error(srv.ListenAndServe()))
		}()
	}
}
