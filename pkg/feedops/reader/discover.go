package reader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

// ErrNoFeedFound: the page is not a feed and announces none.
var ErrNoFeedFound = errors.New("no feed found")

// feedLinkTypes are the <link rel="alternate"> types that announce a feed.
var feedLinkTypes = []string{"application/rss+xml", "application/atom+xml", "application/feed+json"}

// Discover returns the feed URL for pageURL: pageURL itself when it is a feed,
// otherwise the first feed the HTML page announces with a
// <link rel="alternate">, resolved against the page's address. It downloads
// at most limits.MaxFeedSize bytes within limits.FeedTimeout; a page that
// neither is nor announces a feed fails with ErrNoFeedFound.
func (f *Fetcher) Discover(ctx context.Context, pageURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.limits.FeedTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", f.parser.UserAgent)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(newCappedReader(resp.Body, f.limits.MaxFeedSize, gofeed.ErrResponseTooLarge))
	if err != nil {
		return "", err
	}

	if gofeed.DetectFeedType(bytes.NewReader(body)) != gofeed.FeedTypeUnknown {
		return pageURL, nil
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoFeedFound, err)
	}

	if found := announcedFeed(doc, resp.Request.URL); found != "" {
		return found, nil
	}

	return "", ErrNoFeedFound
}

// announcedFeed is the first http(s) feed doc links to with
// rel="alternate", resolved against the page's <base href> or else pageURL,
// the address the page was served from.
func announcedFeed(doc *goquery.Document, pageURL *url.URL) string {
	base := pageURL
	if href, ok := doc.Find("base[href]").First().Attr("href"); ok {
		if u, err := pageURL.Parse(strings.TrimSpace(href)); err == nil {
			base = u
		}
	}

	var found string

	doc.Find("link[rel][href]").EachWithBreak(func(_ int, link *goquery.Selection) bool {
		if !slices.Contains(strings.Fields(strings.ToLower(link.AttrOr("rel", ""))), "alternate") {
			return true
		}

		mediaType, _, err := mime.ParseMediaType(link.AttrOr("type", ""))
		if err != nil || !slices.Contains(feedLinkTypes, mediaType) {
			return true
		}

		u, err := base.Parse(strings.TrimSpace(link.AttrOr("href", "")))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return true
		}

		found = u.String()

		return false
	})

	return found
}
