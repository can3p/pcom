package reader

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/can3p/pcom/pkg/media"
	"github.com/mmcdole/gofeed"
	"github.com/pkg/errors"
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
	MediaDownloadTimeout       = 30 * time.Second
	GlobalImageDownloadTimeout = 2 * time.Minute
)

var (
	ErrMediaTooLarge = errors.New("media file exceeds maximum size limit")
	ErrMediaTimeout  = errors.New("media download exceeded timeout limit")
)

type Fetcher struct {
	parser     *gofeed.Parser
	httpClient *http.Client
}

func NewFetcher(httpClient *http.Client) *Fetcher {
	p := gofeed.NewParser()
	p.Client = httpClient

	return &Fetcher{
		parser:     p,
		httpClient: httpClient,
	}
}

func (f *Fetcher) Fetch(rssURL string) (*Feed, error) {
	feed, err := f.parser.ParseURL(rssURL)
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
	ctx, cancel := context.WithTimeout(ctx, MediaDownloadTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", mediaURL, nil)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrMediaTimeout
		}
		return nil, errors.Wrap(err, "failed to fetch media")
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		return nil, errors.Errorf("failed to fetch media: HTTP %d", resp.StatusCode)
	}

	if resp.ContentLength > MaxMediaSize {
		_ = resp.Body.Close()
		cancel()
		return nil, errors.Wrapf(ErrMediaTooLarge, "content-length: %d bytes", resp.ContentLength)
	}

	peekBuf := make([]byte, 512)
	n, err := io.ReadFull(resp.Body, peekBuf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		_ = resp.Body.Close()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrMediaTimeout
		}
		return nil, errors.Wrap(err, "failed to read media header")
	}

	contentType := http.DetectContentType(peekBuf[:n])
	if _, err := media.ValidateImageType(contentType); err != nil {
		_ = resp.Body.Close()
		cancel()
		return nil, err
	}

	combinedReader := io.MultiReader(bytes.NewReader(peekBuf[:n]), resp.Body)
	limitedReader := io.LimitReader(combinedReader, MaxMediaSize)

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
