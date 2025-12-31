package reader

import (
	"context"
	"fmt"
	"html"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/types"
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

func CreateImageReplacer(ctx context.Context, md string, downloader ImageDownloader, uploadFunc func(ctx context.Context, url string) (string, error)) types.Replacer[string] {
	imageUrls := markdown.ExtractImageUrls(md)

	downloadResults := make(map[string]string)

	for idx, url := range imageUrls {
		// Check if we've exceeded the max images limit
		if idx+1 >= MaxImagesPerFeedItem {
			downloadResults[url] = fmt.Sprintf("_[Image limit exceeded (%d max): %s]_", MaxImagesPerFeedItem, url)
			continue
		}

		newURL, err := uploadFunc(ctx, url)
		if err != nil {
			if errors.Is(err, ErrMediaTimeout) {
				downloadResults[url] = fmt.Sprintf("_[Image download timed out: %s]_", url)
			} else if errors.Is(err, ErrMediaTooLarge) {
				downloadResults[url] = fmt.Sprintf("_[Image too large: %s]_", url)
			} else {
				downloadResults[url] = fmt.Sprintf("_[Image download failed: %s]_", url)
			}
		} else {
			downloadResults[url] = newURL
		}
	}

	return func(in string) (bool, string) {
		if replacement, ok := downloadResults[in]; ok {
			return true, replacement
		}
		return false, in
	}
}

func (c *Cleaner) CleanField(in string) string {
	p := bluemonday.StrictPolicy()

	sanitized := p.Sanitize(in)

	return html.UnescapeString(sanitized)
}

func (c *Cleaner) HTMLToMarkdown(in string, replacer types.Replacer[string]) (string, error) {
	p := bluemonday.UGCPolicy()

	sanitized := p.Sanitize(in)

	md, err := htmltomarkdown.ConvertString(sanitized)
	if err != nil {
		return "", err
	}

	if replacer != nil {
		md = markdown.ReplaceImageUrls(md, replacer)
	}

	return md, nil
}
