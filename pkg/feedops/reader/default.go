package reader

import (
	"net/http"
)

// DefaultFetcher is the fetcher the poller and subscribing use, with each
// request bounded by l (zero fields keep the defaults). It connects only to
// public addresses unless l.AllowPrivateAddresses is set.
func DefaultFetcher(l Limits) *Fetcher {
	client := publicClient()
	if l.AllowPrivateAddresses {
		client = &http.Client{}
	}

	return NewFetcher(client, WithLimits(l))
}
