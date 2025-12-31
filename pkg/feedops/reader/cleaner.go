package reader

import (
	"context"
	"fmt"
	"html"

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

type ImageDownloader interface {
	FetchMedia(ctx context.Context, mediaURL string) (interface{}, error)
}

func CreateImageReplacer(ctx context.Context, md string, downloader ImageDownloader, uploadFunc func(ctx context.Context, url string) (string, error)) markdown.ErrorReplacer {
	imageUrls := markdown.ExtractImageUrls(md)

	type result struct {
		newURL string
		err    error
	}
	downloadResults := make(map[string]result)

	for idx, url := range imageUrls {
		// Check if we've exceeded the max images limit
		if idx+1 >= MaxImagesPerFeedItem {
			downloadResults[url] = result{
				newURL: fmt.Sprintf("_[Image limit exceeded (%d max): %s]_", MaxImagesPerFeedItem, url),
				err:    errors.New("image limit exceeded"),
			}
			continue
		}

		newURL, err := uploadFunc(ctx, url)
		if err != nil {
			var errorMsg string
			if errors.Is(err, ErrMediaTimeout) {
				errorMsg = fmt.Sprintf("_[Image download timed out: %s]_", url)
			} else if errors.Is(err, ErrMediaTooLarge) {
				errorMsg = fmt.Sprintf("_[Image too large: %s]_", url)
			} else {
				errorMsg = fmt.Sprintf("_[Image download failed: %s]_", url)
			}
			downloadResults[url] = result{newURL: errorMsg, err: err}
		} else {
			downloadResults[url] = result{newURL: newURL, err: nil}
		}
	}

	return func(in string) (bool, string, error) {
		if res, ok := downloadResults[in]; ok {
			return true, res.newURL, res.err
		}
		return false, in, nil
	}
}

func (c *Cleaner) CleanField(in string) string {
	p := bluemonday.StrictPolicy()

	sanitized := p.Sanitize(in)

	return html.UnescapeString(sanitized)
}

func (c *Cleaner) HTMLToMarkdown(in string, replacer markdown.ErrorReplacer) (string, error) {
	p := bluemonday.UGCPolicy()

	sanitized := p.Sanitize(in)

	md, err := htmltomarkdown.ConvertString(sanitized)
	if err != nil {
		return "", err
	}

	if replacer != nil {
		md = markdown.ReplaceImageUrlsOrErrorMessage(md, replacer)
	}

	return md, nil
}
