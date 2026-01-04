package feedops

import (
	"net/http"
	"time"

	"github.com/can3p/pcom/pkg/feedops/feeder"
	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/jmoiron/sqlx"
)

func DefaultRssReader(db *sqlx.DB, mediaStorage server.MediaStorage) *feeder.Feeder {
	httpClient := &http.Client{
		Timeout: 5 * time.Second, // we can unhardcode this value
	}

	fetcher := reader.NewFetcher(httpClient)
	cleaner := reader.DefaultCleaner()

	return feeder.NewFeeder(db, fetcher, cleaner, mediaStorage)
}
