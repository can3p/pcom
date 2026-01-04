package reader

import (
	"fmt"
	"html"

	"log/slog"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pkg/errors"
)

const (
	MaxImagesPerFeedItem = 20
)

type Cleaner struct{}

func DefaultCleaner() *Cleaner {
	return &Cleaner{}
}

func CreateImageReplacer(md string, uploadFunc func(url string) (string, error)) markdown.ErrorReplacer {
	imageUrls := markdown.ExtractImageUrls(md)

	uniqueUrls := make(map[string]bool)
	var dedupedUrls []string
	for _, url := range imageUrls {
		if !uniqueUrls[url] {
			uniqueUrls[url] = true
			dedupedUrls = append(dedupedUrls, url)
		}
	}

	type result struct {
		newURL string
		err    error
	}
	downloadResults := make(map[string]result)

	for idx, url := range dedupedUrls {
		// Check if we've exceeded the max images limit
		if idx+1 >= MaxImagesPerFeedItem {
			downloadResults[url] = result{
				err: fmt.Errorf("image limit exceeded (%d max)", MaxImagesPerFeedItem),
			}
			continue
		}

		newURL, err := uploadFunc(url)
		if err != nil {
			var readableErr error
			if errors.Is(err, ErrMediaTimeout) {
				readableErr = fmt.Errorf("image download timed out")
			} else if errors.Is(err, ErrMediaTooLarge) {
				readableErr = fmt.Errorf("image too large")
			} else {
				readableErr = fmt.Errorf("image download failed")
				slog.Warn("image download failed", "url", url, "error", err)
			}

			downloadResults[url] = result{err: readableErr}
		} else {
			downloadResults[url] = result{newURL: newURL, err: nil}
		}
	}

	return func(in string) (string, error) {
		if res, ok := downloadResults[in]; ok {
			return res.newURL, res.err
		}
		return in, nil
	}
}

func (c *Cleaner) CleanField(in string) string {
	p := bluemonday.StrictPolicy()

	sanitized := p.Sanitize(in)

	return html.UnescapeString(sanitized)
}

func (c *Cleaner) HTMLToMarkdown(in string) (string, error) {
	p := bluemonday.UGCPolicy()

	sanitized := p.Sanitize(in)

	md, err := htmltomarkdown.ConvertString(sanitized)
	if err != nil {
		return "", err
	}

	return md, nil
}
