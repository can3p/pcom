// Package stt turns a recording into text through a speech-to-text backend.
// A backend file (pkg/stt/<name>) holds only the HTTP call and reports a
// failed status as a *StatusError; Retry wraps any backend with the retries
// every one of them shares, and Registry picks the backend named by
// STT_PROVIDER. The package knows no configuration and no service: the
// composition root builds the registry.
package stt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Audio is a recording to transcribe.
type Audio struct {
	Bytes       []byte
	ContentType string
	// Language is an ISO 639-1 code that forces the language; empty lets the
	// backend detect it.
	Language string
}

// Result is what a backend heard.
type Result struct {
	Text string
	// Language is the language the backend reports having heard, as it names
	// it (OpenAI-compatible backends say "english", not "en").
	Language string
	// Seconds is the length of the recording.
	Seconds float64
}

// Backend is a speech-to-text service. Name and Model identify it in
// stt_requests.
type Backend interface {
	Name() string
	Model() string
	Transcribe(ctx context.Context, audio Audio) (Result, error)
}

// StatusError is a backend's answer with a failed HTTP status.
type StatusError struct {
	Status int
	// RetryAfter is the wait the backend asked for; zero when it did not.
	RetryAfter time.Duration
	// Message is the start of the answer's body.
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("stt: backend answered %d: %s", e.Status, e.Message)
}

// Retryable reports whether the same call may succeed later: a rate limit or
// a server error. A bad key or a rejected recording will not.
func (e *StatusError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= http.StatusInternalServerError
}

// ParseRetryAfter reads a Retry-After header, given in seconds or as an HTTP
// date counted from now; anything else, or a date in the past, is no wait.
func ParseRetryAfter(v string, now time.Time) time.Duration {
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(max(n, 0)) * time.Second
	}

	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0)
	}

	return 0
}

// Defaults of RetryOptions.
const (
	DefaultAttempts  = 3
	DefaultBaseDelay = time.Second
	DefaultMaxDelay  = 30 * time.Second

	backoffFactor = 2
)

// RetryOptions tune Retry; a zero field keeps its default.
type RetryOptions struct {
	// Attempts is the most calls of one transcription, the first included.
	Attempts int
	// BaseDelay is the wait before the second call; it doubles from there.
	BaseDelay time.Duration
	// MaxDelay caps every wait, Retry-After included.
	MaxDelay time.Duration
	// Sleep replaces the wait, for tests.
	Sleep func(ctx context.Context, d time.Duration) error
}

type retrying struct {
	Backend
	opts RetryOptions
}

// Retry wraps b so a call that fails with a retryable status is made again,
// waiting for the backend's Retry-After or else backing off exponentially.
// The context's end stops the waiting. The last error is returned as it was.
func Retry(b Backend, opts RetryOptions) Backend {
	if opts.Attempts <= 0 {
		opts.Attempts = DefaultAttempts
	}

	if opts.BaseDelay <= 0 {
		opts.BaseDelay = DefaultBaseDelay
	}

	if opts.MaxDelay <= 0 {
		opts.MaxDelay = DefaultMaxDelay
	}

	if opts.Sleep == nil {
		opts.Sleep = sleep
	}

	return &retrying{Backend: b, opts: opts}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (r *retrying) Transcribe(ctx context.Context, audio Audio) (Result, error) {
	delay := r.opts.BaseDelay

	for attempt := 1; ; attempt++ {
		res, err := r.Backend.Transcribe(ctx, audio)

		var se *StatusError
		if err == nil || !errors.As(err, &se) || !se.Retryable() || attempt >= r.opts.Attempts {
			return res, err
		}

		wait := delay
		if se.RetryAfter > 0 {
			wait = se.RetryAfter
		}

		if err := r.opts.Sleep(ctx, min(wait, r.opts.MaxDelay)); err != nil {
			return Result{}, err
		}

		delay *= backoffFactor
	}
}

// Registry maps a provider name, as STT_PROVIDER spells it, to a function
// that builds its backend.
type Registry map[string]func() (Backend, error)

// Backend builds the backend named provider. An empty provider is no
// backend and no error: transcription is off.
func (r Registry) Backend(provider string) (Backend, error) {
	if provider == "" {
		return nil, nil //nolint:nilnil // no provider is a valid answer: transcription is off
	}

	build, ok := r[provider]
	if !ok {
		return nil, fmt.Errorf("stt: unknown provider %q", provider)
	}

	return build()
}
