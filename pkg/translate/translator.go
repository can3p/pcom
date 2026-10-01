package translate

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

// Translator is the entry point services call: it wraps a Backend with the
// shared chunking, retries and shape checks.
type Translator struct {
	backend  Backend
	attempts int
	backoff  time.Duration
	maxDelay time.Duration
	sleep    func(ctx context.Context, d time.Duration) error
}

// Option configures a Translator.
type Option func(*Translator)

// WithAttempts sets how many times one chunk is tried (default 4).
func WithAttempts(n int) Option { return func(t *Translator) { t.attempts = max(n, 1) } }

// WithBackoff sets the first retry delay, doubled on every further retry
// (default 1s), and the longest delay, Retry-After included (default 1m).
func WithBackoff(first, longest time.Duration) Option {
	return func(t *Translator) { t.backoff, t.maxDelay = first, longest }
}

// WithSleep replaces the wait between attempts, so tests don't sleep.
func WithSleep(sleep func(ctx context.Context, d time.Duration) error) Option {
	return func(t *Translator) { t.sleep = sleep }
}

// NewTranslator wraps b.
func NewTranslator(b Backend, opts ...Option) *Translator {
	t := &Translator{
		backend:  b,
		attempts: 4,
		backoff:  time.Second,
		maxDelay: time.Minute,
		sleep:    sleepCtx,
	}

	for _, o := range opts {
		o(t)
	}

	return t
}

// Name is the backend's Name, stored with each translation.
func (t *Translator) Name() string { return t.backend.Name() }

// DisplayName is the backend's name for readers.
func (t *Translator) DisplayName() string { return t.backend.DisplayName() }

// Translate returns one translation per segment of req, in order. It splits
// req into chunks under the backend's Limits, retries rate limits and server
// errors, and checks every chunk's shape: a mismatch fails the whole call,
// never giving a partial translation.
func (t *Translator) Translate(ctx context.Context, req Request) ([]string, error) {
	chunks, err := Chunk(req.Segments, t.backend.Limits())
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(req.Segments))

	for _, chunk := range chunks {
		got, err := t.call(ctx, Request{From: req.From, To: req.To, Segments: chunk})
		if err != nil {
			return nil, err
		}

		if err := CheckShape(chunk, got); err != nil {
			return nil, err
		}

		out = append(out, got...)
	}

	return out, nil
}

// call sends one chunk, retrying what StatusError calls retryable.
func (t *Translator) call(ctx context.Context, req Request) ([]string, error) {
	delay := t.backoff

	for attempt := 1; ; attempt++ {
		got, err := t.backend.Translate(ctx, req)
		if err == nil {
			return got, nil
		}

		var se *StatusError
		if !errors.As(err, &se) || !se.Retryable() || attempt >= t.attempts {
			return nil, fmt.Errorf("%s: %w", t.backend.Name(), err)
		}

		wait := min(max(delay, se.RetryAfter), t.maxDelay)
		if err := t.sleep(ctx, wait); err != nil {
			return nil, err
		}

		delay *= 2
	}
}

// Chunk splits segments, in order, into runs of at most l.Segments segments
// and l.Chars characters; a zero limit is no limit. A segment longer than
// l.Chars on its own is ErrTooLong.
func Chunk(segments []string, l Limits) ([][]string, error) {
	var (
		chunks [][]string
		cur    []string
		chars  int
	)

	for _, s := range segments {
		n := utf8.RuneCountInString(s)
		if l.Chars > 0 && n > l.Chars {
			return nil, fmt.Errorf("%w: %d characters, limit %d", ErrTooLong, n, l.Chars)
		}

		full := (l.Segments > 0 && len(cur) == l.Segments) || (l.Chars > 0 && chars+n > l.Chars)
		if full && len(cur) > 0 {
			chunks = append(chunks, cur)
			cur, chars = nil, 0
		}

		cur = append(cur, s)
		chars += n
	}

	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}

	return chunks, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
