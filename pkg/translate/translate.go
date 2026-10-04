// Package translate turns small HTML fragments from one language into another
// through a pluggable Backend (Azure Translator first). Everything a backend
// shares lives here: splitting a request into chunks under the backend's
// Limits, retrying rate limits and server errors with backoff, counting
// characters and checking that a result has the source's shape. A backend
// package holds only its HTTP call and the mapping to and from its wire
// format, and registers itself with Register.
package translate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"
)

// Request is one translation call. Every segment is a small HTML fragment:
// inline content as minimal HTML (<em>, <strong>, <a data-i="3">), with what
// must not change as <span class="notranslate" data-i="n">…</span>.
type Request struct {
	From     string // ISO 639-1, such as "ru"
	To       string // ISO 639-1, "en" for now
	Segments []string
}

// Limits is the most a backend accepts in one request.
type Limits struct {
	Segments int // segments per request
	Chars    int // characters (runes) per request, summed over its segments
}

// Backend is what a translation provider implements.
type Backend interface {
	// Name identifies the backend; it is stored with each translation.
	Name() string
	// DisplayName is shown to readers, such as "Azure Translator".
	DisplayName() string
	// Limits is the most one Translate call may carry.
	Limits() Limits
	// Translate returns exactly one translation per segment, in order, or an
	// error. A request within Limits is never split by the backend.
	Translate(ctx context.Context, req Request) ([]string, error)
}

// ErrTooLong is returned for a segment longer than the backend's character
// limit on its own: it can't be sent in any chunk.
var ErrTooLong = errors.New("translate: segment exceeds the backend's character limit")

// ErrShape is wrapped by every error about a result that doesn't match its
// source: a wrong segment count or a protected span lost or changed.
var ErrShape = errors.New("translate: result doesn't match the source")

// StatusError is a backend's HTTP failure. A backend returns it for every
// non-2xx answer, so that the shared layer can decide on retries.
type StatusError struct {
	Status     int
	Code       string        // the provider's own error code, if any
	Message    string        // the provider's message, if any
	RetryAfter time.Duration // from a Retry-After header; zero if none
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("translate: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// Retryable reports whether the call may succeed later: rate limits and
// server errors.
func (e *StatusError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// Chars counts the characters of segments the way budgets and Limits do, in
// runes.
func Chars(segments ...string) int {
	n := 0
	for _, s := range segments {
		n += utf8.RuneCountInString(s)
	}

	return n
}
