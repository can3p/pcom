package rss_test

import (
	"github.com/can3p/pcom/pkg/links"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/postops/rss"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

func mkPost(id string, body string, vis core.PostVisibility, author *core.User) *postops.Post {
	return &postops.Post{
		Post: &core.Post{
			ID:               id,
			Subject:          null.StringFrom("A subject"),
			Body:             body,
			VisibilityRadius: vis,
			CreatedAt:        null.TimeFrom(time.Date(2025, time.March, 1, 10, 0, 0, 0, time.UTC)),
			PublishedAt:      null.TimeFrom(time.Date(2025, time.March, 4, 10, 0, 0, 0, time.UTC)),
		},
		Author: author,
	}
}

func TestToFeed_PublicPostIsRendered(t *testing.T) {
	t.Parallel()

	author := &core.User{Username: "alice"}
	post := mkPost("post-1", "Some *markdown* body", core.PostVisibilityPublic, author)

	feed := rss.ToFeed(links.Site{}, "My Blog", "https://example.com", []*postops.Post{post})

	require.Equal(t, "My Blog", feed.Title)
	require.Equal(t, "https://example.com", feed.Link.Href)
	require.Len(t, feed.Items, 1)

	item := feed.Items[0]
	require.Equal(t, "A subject", item.Title)
	require.Equal(t, "@alice", item.Author.Name)
	require.Contains(t, item.Description, "Some")
	require.NotContains(t, item.Description, "not public")
	require.Contains(t, item.Link.Href, "post-1")
	// a draft published later carries its publication date, not its creation date
	require.Equal(t, time.Date(2025, time.March, 4, 10, 0, 0, 0, time.UTC), item.Created)
}

func TestToFeed_NonPublicPostHidesBody(t *testing.T) {
	t.Parallel()

	author := &core.User{Username: "alice"}

	testCases := []core.PostVisibility{
		core.PostVisibilityDirectOnly,
		core.PostVisibilitySecondDegree,
	}

	for _, vis := range testCases {
		t.Run(vis.String(), func(t *testing.T) {
			t.Parallel()

			post := mkPost("post-2", "Secret content that must not leak", vis, author)
			feed := rss.ToFeed(links.Site{}, "My Blog", "https://example.com", []*postops.Post{post})

			require.Len(t, feed.Items, 1)
			require.Equal(t, "Post is not public, follow the link to read the text", feed.Items[0].Description)
			require.NotContains(t, feed.Items[0].Description, "Secret")
		})
	}
}

func TestToFeed_AnonymousAuthorFallsBack(t *testing.T) {
	t.Parallel()

	post := mkPost("post-3", "body", core.PostVisibilityPublic, nil)

	feed := rss.ToFeed(links.Site{}, "My Blog", "https://example.com", []*postops.Post{post})

	require.Len(t, feed.Items, 1)
	require.Equal(t, "Anonymous User", feed.Items[0].Author.Name)
}

func TestToFeed_PreservesOrderAndSubjectFallback(t *testing.T) {
	t.Parallel()

	author := &core.User{Username: "alice"}

	noSubject := &postops.Post{
		Post: &core.Post{
			ID:               "post-a",
			Body:             "a",
			VisibilityRadius: core.PostVisibilityPublic,
			PublishedAt:      null.TimeFrom(time.Now()),
		},
		Author: author,
	}
	withSubject := mkPost("post-b", "b", core.PostVisibilityPublic, author)

	feed := rss.ToFeed(links.Site{}, "My Blog", "https://example.com", []*postops.Post{noSubject, withSubject})

	require.Len(t, feed.Items, 2)
	require.Equal(t, "No Subject", feed.Items[0].Title)
	require.Contains(t, feed.Items[0].Link.Href, "post-a")
	require.Equal(t, "A subject", feed.Items[1].Title)
	require.Contains(t, feed.Items[1].Link.Href, "post-b")
}

func TestToFeed_NoPosts(t *testing.T) {
	t.Parallel()

	feed := rss.ToFeed(links.Site{}, "Empty Blog", "https://example.com", nil)
	require.Empty(t, feed.Items)
}

// The site decides the item link and where uploaded media comes from.
func TestToFeed_SiteRootAndMediaCDN(t *testing.T) {
	t.Parallel()

	const media = "3fa85f64-5717-4562-b3fc-2c963f66afa6.png"

	author := &core.User{Username: "alice"}
	post := mkPost("post-1", "look ![pic]("+media+")", core.PostVisibilityPublic, author)

	for _, tc := range []struct {
		name      string
		site      links.Site
		wantMedia string
	}{
		{"with CDN", links.Site{Root: "https://pcom.test", MediaCDN: "https://media.test"}, "https://media.test/" + media},
		{"without CDN", links.Site{Root: "https://pcom.test"}, "https://pcom.test/user-media/" + media},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			item := rss.ToFeed(tc.site, "My Blog", "https://example.com", []*postops.Post{post}).Items[0]

			require.Equal(t, "https://pcom.test"+links.Link("post", "post-1"), item.Link.Href)
			require.Contains(t, item.Description, tc.wantMedia)
		})
	}
}
