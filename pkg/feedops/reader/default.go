package reader

import (
	"net/http"
	"time"
)

// DefaultFetcher is the fetcher the poller uses: a plain HTTP client with a
// short timeout.
func DefaultFetcher() *Fetcher {
	return NewFetcher(&http.Client{
		Timeout: 5 * time.Second, // we can unhardcode this value
	})
}
