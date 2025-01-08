package reader

import (
	"net/http"
	"time"

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

type Fetcher struct {
	parser *gofeed.Parser
}

func NewFetcher(httpClient *http.Client) *Fetcher {
	p := gofeed.NewParser()
	p.Client = httpClient

	return &Fetcher{
		parser: p,
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
