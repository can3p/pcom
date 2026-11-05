package reader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/can3p/pcom/pkg/media"
	"github.com/mmcdole/gofeed"
	"github.com/samber/lo"
)

type Feed struct {
	Title       string
	Description string
	Items       []*Item
}

type Item struct {
	URL         string
	Title       string
	Summary     string
	PublishedAt *time.Time
}

const (
	MaxMediaSize               = 10 * 1024 * 1024
	GlobalImageDownloadTimeout = 2 * time.Minute

	// DefaultFeedTimeout is how long fetching one feed document may take,
	// body included.
	DefaultFeedTimeout = 5 * time.Second
	// DefaultMediaTimeout is how long downloading one image may take, body
	// included.
	DefaultMediaTimeout = 30 * time.Second
	// DefaultMaxFeedSize is the most bytes a feed document may have.
	DefaultMaxFeedSize = 5 * 1024 * 1024
)

var (
	ErrMediaTooLarge = errors.New("media file exceeds maximum size limit")
	ErrMediaTimeout  = errors.New("media download exceeded timeout limit")
)

// Limits bound what fetching a feed may cost; a zero field keeps its
// default (DefaultFeedTimeout, DefaultMediaTimeout, DefaultMaxFeedSize).
type Limits struct {
	FeedTimeout  time.Duration
	MediaTimeout time.Duration
	MaxFeedSize  int64
}

func (l Limits) withDefaults() Limits {
	if l.FeedTimeout <= 0 {
		l.FeedTimeout = DefaultFeedTimeout
	}

	if l.MediaTimeout <= 0 {
		l.MediaTimeout = DefaultMediaTimeout
	}

	if l.MaxFeedSize <= 0 {
		l.MaxFeedSize = DefaultMaxFeedSize
	}

	return l
}

type Fetcher struct {
	parser     *gofeed.Parser
	httpClient *http.Client
	limits     Limits
}

// FetcherOption changes how NewFetcher builds the fetcher.
type FetcherOption func(*Fetcher)

// WithLimits sets the timeouts and the feed size cap; zero fields keep the
// defaults.
func WithLimits(l Limits) FetcherOption {
	return func(f *Fetcher) { f.limits = l.withDefaults() }
}

// NewFetcher fetches with httpClient. The timeouts come from the limits, as
// a context deadline per request, so the client needs no Timeout of its own:
// one would cut image downloads short of their own timeout.
func NewFetcher(httpClient *http.Client, opts ...FetcherOption) *Fetcher {
	f := &Fetcher{
		parser:     gofeed.NewParser(),
		httpClient: httpClient,
		limits:     Limits{}.withDefaults(),
	}

	for _, opt := range opts {
		opt(f)
	}

	f.parser.Client = httpClient
	// a larger document fails with gofeed.ErrResponseTooLarge
	f.parser.MaxByteSize = f.limits.MaxFeedSize

	return f
}

// Fetch downloads the feed document at rssURL, at most limits.MaxFeedSize
// bytes within limits.FeedTimeout, and parses it.
func (f *Fetcher) Fetch(ctx context.Context, rssURL string) (*Feed, error) {
	ctx, cancel := context.WithTimeout(ctx, f.limits.FeedTimeout)
	defer cancel()

	feed, err := f.parser.ParseURLWithContext(rssURL, ctx)
	if err != nil {
		return nil, err
	}

	items := lo.Map(feed.Items, func(item *gofeed.Item, idx int) *Item {
		return &Item{
			URL:         item.Link,
			Title:       item.Title,
			Summary:     item.Description,
			PublishedAt: item.PublishedParsed,
		}
	})

	return &Feed{
		Title:       feed.Title,
		Description: feed.Description,
		Items:       items,
	}, nil
}

type limitedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (lrc *limitedReadCloser) Close() error {
	return lrc.closer.Close()
}

func (f *Fetcher) FetchMedia(ctx context.Context, mediaURL string) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, f.limits.MediaTimeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrMediaTimeout
		}
		return nil, fmt.Errorf("failed to fetch media: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("failed to fetch media: HTTP %d", resp.StatusCode)
	}

	if resp.ContentLength > MaxMediaSize {
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("content-length: %d bytes: %w", resp.ContentLength, ErrMediaTooLarge)
	}

	peekBuf := make([]byte, 512)
	n, err := io.ReadFull(resp.Body, peekBuf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		_ = resp.Body.Close()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrMediaTimeout
		}
		return nil, fmt.Errorf("failed to read media header: %w", err)
	}

	contentType := http.DetectContentType(peekBuf[:n])
	if _, err := media.ValidateImageType(contentType); err != nil {
		_ = resp.Body.Close()
		cancel()
		return nil, err
	}

	combinedReader := io.MultiReader(bytes.NewReader(peekBuf[:n]), resp.Body)
	limitedReader := newCappedReader(combinedReader, MaxMediaSize, ErrMediaTooLarge)

	return &limitedReadCloser{
		Reader: limitedReader,
		closer: &closeFunc{
			body:   resp.Body,
			cancel: cancel,
		},
	}, nil
}

type closeFunc struct {
	body   io.Closer
	cancel context.CancelFunc
}

func (cf *closeFunc) Close() error {
	defer cf.cancel()
	return cf.body.Close()
}

// cappedReader reads at most maxBytes of r and fails with tooLarge when r
// has more, rather than cutting it short silently as io.LimitReader does.
type cappedReader struct {
	r        io.Reader
	left     int64
	tooLarge error
}

func newCappedReader(r io.Reader, maxBytes int64, tooLarge error) *cappedReader {
	return &cappedReader{r: r, left: maxBytes, tooLarge: tooLarge}
}

func (c *cappedReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		var probe [1]byte

		n, err := c.r.Read(probe[:])
		if n > 0 {
			return 0, c.tooLarge
		}

		return 0, err
	}

	if int64(len(p)) > c.left {
		p = p[:c.left]
	}

	n, err := c.r.Read(p)
	c.left -= int64(n)

	return n, err
}
