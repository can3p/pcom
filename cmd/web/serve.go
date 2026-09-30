package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/can3p/gogo/sender/mailjet"
	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/media/server/storage/s3"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/gin-gonic/gin"
)

const MediaServerConcurrency = 3

type serveCmd struct {
	config.Serve
}

func (c *serveCmd) Execute([]string) error {
	cfg := c.Serve

	if cfg.Web.GinMode != "" {
		gin.SetMode(cfg.Web.GinMode)
	}

	slog.SetLogLoggerLevel(cfg.Web.SlogLevel())

	db, closeDB, err := openDB(cfg.Database.URL)
	if err != nil {
		return err
	}
	defer closeDB()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbSender := dbsender.NewSender(repo.New(db), mailjet.NewSenderFromConfig(&cfg.Mail.Mailjet))

	go dbSender.RunPoller(ctx, cfg.Mail.PollInterval)

	mediaStorage, err := newMediaStorage(cfg.Media)
	if err != nil {
		return err
	}

	feeder := feeds.New(repo.New(db), mediaStorage)

	go feeder.RunPoller(ctx)

	mediaServer, mediaServerCleanup, err := newMediaServer(mediaStorage, cfg.Web.MediaPermaCache.On())
	if err != nil {
		return err
	}
	defer mediaServerCleanup()

	// developer timezone only messes things up
	time.Local = time.UTC

	deps := &app.Deps{
		DB:           db,
		Sender:       dbSender,
		MediaStorage: mediaStorage,
		MediaServer:  mediaServer,
		Config: app.Config{
			HTMLDir:               cfg.Web.HTMLDir,
			ForceOpenRegistration: cfg.Web.ForceSignup.On(),
			SessionSalt:           cfg.Web.SessionSalt.Reveal(),
			StaticAsset:           app.LoadStaticManifest(),
			SiteRoot:              cfg.Web.SiteRoot,
			StaticCDN:             cfg.Web.StaticCDN,
			MediaCDN:              cfg.Media.CDN,
			SenderAddress:         cfg.Mail.SenderAddress,
			AdminAddress:          cfg.Mail.AdminAddress,
			SecureCookies:         cfg.Web.SecureCookies.On(),
			HSTS:                  cfg.Web.HSTS.On(),
			StaticCache:           cfg.Web.StaticCache.On(),
			ShowErrors:            cfg.Web.ShowErrors.On(),
			ReportPanics:          cfg.Web.ReportPanics.On(),
		},
	}

	startPprof(cfg.Web.EnablePprof.On())

	return app.New(deps).Run(fmt.Sprintf(":%d", cfg.Web.Port))
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
	})
}

// newMediaServer serves user media in the thumb and full classes; the caller
// runs the returned cleanup on exit.
func newMediaServer(mediaStorage server.MediaStorage, permaCache bool) (server.MediaServer, func(), error) {
	var mediaServer server.MediaServer

	mediaServer, mediaServerCleanup, err := server.New(mediaStorage,
		server.WithClass("thumb", server.ClassParams{Width: 720, Height: 540}),
		server.WithClass("full", server.ClassParams{Width: 1200, Height: 900}),
		server.WithPermaCache(permaCache),
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
// serves them on :8081.
func startPprof(enabled bool) {
	pprofMux := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()

	if enabled {
		go func() {
			srv := &http.Server{
				Addr:    ":8081",
				Handler: pprofMux,
			}

			log.Fatal(srv.ListenAndServe())
		}()
	}
}
