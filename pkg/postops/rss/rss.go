package rss

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/types"
	"github.com/gorilla/feeds"
)

func ToFeed(title string, link string, author *core.User, posts []*core.Post) *feeds.Feed {
	feed := &feeds.Feed{
		Title:  title,
		Link:   &feeds.Link{Href: link},
		Author: &feeds.Author{Name: author.Username, Email: "undisclosed"},
	}

	items := []*feeds.Item{}

	for _, post := range posts {
		content := markdown.ToEnrichedTemplate(post.Body, types.ViewRSS, links.MediaReplacer, func(in string, add2 ...string) string {
			return links.AbsLink(in, add2...)
		})

		items = append(items, &feeds.Item{
			Title: post.Subject,
			Author: &feeds.Author{
				Name: func() string {
					var by string

					if post.R.User != nil {
						by = "@" + post.R.User.Username
					} else {
						by = "Anonymous User"
					}

					return by
				}(),
			},
			Link:        &feeds.Link{Href: links.AbsLink("post", post.ID)},
			Description: string(content),
			Created:     post.CreatedAt.Time,
		})
	}

	feed.Items = items

	return feed
}
