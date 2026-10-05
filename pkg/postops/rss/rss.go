package rss

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/gorilla/feeds"
	"github.com/samber/lo"
)

func ToFeed(site links.Site, title string, link string, posts []*postops.Post) *feeds.Feed {
	feed := &feeds.Feed{
		Title: title,
		Link:  &feeds.Link{Href: link},
	}

	items := []*feeds.Item{}

	for _, post := range posts {
		content := "Post is not public, follow the link to read the text"

		if post.VisibilityRadius == model.PostVisibilityPublic {
			content = string(markdown.ToEnrichedTemplate(post.Body, types.ViewRSS, site.MediaReplacer, func(in string, add2 ...string) string {
				return site.Abs(in, add2...)
			}))
		}

		items = append(items, &feeds.Item{
			Title: postops.PostSubject(post.Subject),
			Author: &feeds.Author{
				Name: func() string {
					var by string

					if post.Author != nil {
						by = "@" + post.Author.Username
					} else {
						by = "Anonymous User"
					}

					return by
				}(),
			},
			Link:        &feeds.Link{Href: site.Abs("post", post.ID)},
			Description: content,
			Created:     lo.FromPtr(post.PublishedAt).UTC(),
		})
	}

	feed.Items = items

	return feed
}
