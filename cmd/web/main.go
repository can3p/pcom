package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
	_ "time/tzdata" // help go learn about timezones

	"github.com/can3p/gogo/sender"
	"github.com/can3p/gogo/sender/console"
	"github.com/can3p/gogo/sender/mailjet"
	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/media/server/storage/local"
	"github.com/can3p/pcom/pkg/media/server/storage/s3"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	_ "github.com/lib/pq" // postgres db driver
)

const MediaServerConcurrency = 3

var requiredVars = []string{
	"DATABASE_URL",
	"SESSION_SALT",
	"SITE_ROOT",
}

func enforceEnvVars(requiredVars []string) {
	for _, v := range requiredVars {
		if _, ok := os.LookupEnv(v); !ok {
			panic(fmt.Sprintf("var %s is not set", v))
		}
	}
}

func main() {
	var forceOpenRegistation bool
	var forceRealSender bool

	flag.BoolVar(&forceOpenRegistation, "force-signup", false, "allow new signups even if it's disabled in system settings")
	flag.BoolVar(&forceRealSender, "force-real-sender", false, "force real sender outside of cluster")

	html := flag.String("html", "client/html", "path to html templates")

	flag.Parse()

	shouldUseRealSender := util.InCluster() || forceRealSender
	shouldUseS3 := util.InCluster()

	enforceEnvVars(requiredVars)
	if shouldUseRealSender {
		enforceEnvVars(mailjet.RequiredEnv)
		enforceEnvVars([]string{"SENDER_ADDRESS"})
	}

	if shouldUseS3 {
		enforceEnvVars(s3.RequiredEnv)
	}

	// fly.io does not have sslmode enabled
	db := sqlx.MustConnect("postgres", os.Getenv("DATABASE_URL"))
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sender sender.Sender
	var mediaStorage server.MediaStorage

	if shouldUseRealSender {
		sender = mailjet.NewSender()
	} else {
		sender = console.NewSender()
	}

	dbSender := dbsender.NewSender(repo.New(db), sender)
	sender = dbSender

	go dbSender.RunPoller(ctx)

	mediaStorage = newMediaStorage(shouldUseS3)

	feeder := feedops.DefaultRssReader(db, mediaStorage)

	go feeder.RunPoller(ctx)

	mediaServer, mediaServerCleanup := newMediaServer(mediaStorage)
	defer mediaServerCleanup()

	if !util.InCluster() {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// developer timezone only messes things up
	time.Local = time.UTC

	deps := &app.Deps{
		DB:           db,
		Sender:       sender,
		MediaStorage: mediaStorage,
		MediaServer:  mediaServer,
		Config: app.Config{
			HTMLDir:               *html,
			ForceOpenRegistration: forceOpenRegistation,
			InCluster:             util.InCluster(),
			SessionSalt:           os.Getenv("SESSION_SALT"),
			StaticAsset:           app.LoadStaticManifest(),
		},
	}

	startPprof()

	if err := app.New(deps).Run(); err != nil {
		panic(err)
	}
}

func newMediaStorage(shouldUseS3 bool) server.MediaStorage {
	var mediaStorage server.MediaStorage
	var err error

	if shouldUseS3 {
		mediaStorage, err = s3.NewS3Server()
	} else {
		mediaStorage, err = local.NewLocalServer("user_media")
	}

	if err != nil {
		panic(err)
	}

	return mediaStorage
}

// newMediaServer serves user media in the thumb and full classes; the caller
// runs the returned cleanup on exit.
func newMediaServer(mediaStorage server.MediaStorage) (server.MediaServer, func()) {
	var mediaServer server.MediaServer

	mediaServer, mediaServerCleanup, err := server.New(mediaStorage,
		server.WithClass("thumb", server.ClassParams{Width: 720, Height: 540}),
		server.WithClass("full", server.ClassParams{Width: 1200, Height: 900}),
		server.WithPermaCache(util.InCluster()),
		server.WithClassResolver(func(c context.Context, req *http.Request) string {
			// we know that the context is gin
			ginCtx := c.(*gin.Context)
			return ginCtx.Param("class")
		}),
	)

	if err != nil {
		panic(err)
	}

	mediaServer = server.NewWrapper(mediaServer, MediaServerConcurrency)
	mediaServer, err = server.NewCachingServer(mediaServer, mediaStorage, 0)

	if err != nil {
		panic(err)
	}

	return mediaServer, mediaServerCleanup
}

// startPprof moves the pprof handlers off the default mux and serves them on
// :8081 when ENABLE_PPROF is true.
func startPprof() {
	pprofMux := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
	if os.Getenv("ENABLE_PPROF") == "true" {
		go func() {
			srv := &http.Server{
				Addr:    ":8081",
				Handler: pprofMux,
			}

			log.Fatal(srv.ListenAndServe())
		}()
	}
}
