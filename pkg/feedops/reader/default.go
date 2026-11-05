package reader

import (
	"net/http"
)

// DefaultFetcher is the fetcher the poller uses: a plain HTTP client, with
// each request bounded by l (zero fields keep the defaults).
func DefaultFetcher(l Limits) *Fetcher {
	return NewFetcher(&http.Client{}, WithLimits(l))
}
