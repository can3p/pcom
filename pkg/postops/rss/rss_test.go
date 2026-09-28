package rss_test

import (
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
			CreatedAt:        null.TimeFrom(time.Date(2025, time.March, 4, 10, 0, 0, 0, time.UTC)),
		},
		Author: author,
	}
}

func TestToFeed_PublicPostIsRendered(t *testing.T) {
	t.Parallel()

	author := &core.User{Username: "alice"}
	post := mkPost("post-1", "Some *markdown* body", core.PostVisibilityPublic, author)

	feed := rss.ToFeed("My Blog", "https://example.com", author, []*postops.Post{post})

	require.Equal(t, "My Blog", feed.Title)
	require.Equal(t, "https://example.com", feed.Link.Href)
	require.Len(t, feed.Items, 1)

	item := feed.Items[0]
	require.Equal(t, "A subject", item.Title)
	require.Equal(t, "@alice", item.Author.Name)
	require.Contains(t, item.Description, "Some")
	require.NotContains(t, item.Description, "not public")
	require.Contains(t, item.Link.Href, "post-1")
}

func TestToFeed_NonPublicPostHidesBody(t *testing.T) {
	t.Parallel()

	author := &core.User{Username: "alice"}

	testCases := []core.PostVisibility{
		core.PostVisibilityDirectOnly,
		core.PostVisibilitySecondDegree,
	}

	for _, vis := range testCases {
		vis := vis
		t.Run(vis.String(), func(t *testing.T) {
			t.Parallel()

			post := mkPost("post-2", "Secret content that must not leak", vis, author)
			feed := rss.ToFeed("My Blog", "https://example.com", author, []*postops.Post{post})

			require.Len(t, feed.Items, 1)
			require.Equal(t, "Post is not public, follow the link to read the text", feed.Items[0].Description)
			require.NotContains(t, feed.Items[0].Description, "Secret")
		})
	}
}

func TestToFeed_AnonymousAuthorFallsBack(t *testing.T) {
	t.Parallel()

	post := mkPost("post-3", "body", core.PostVisibilityPublic, nil)

	feed := rss.ToFeed("My Blog", "https://example.com", nil, []*postops.Post{post})

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
			CreatedAt:        null.TimeFrom(time.Now()),
		},
		Author: author,
	}
	withSubject := mkPost("post-b", "b", core.PostVisibilityPublic, author)

	feed := rss.ToFeed("My Blog", "https://example.com", author, []*postops.Post{noSubject, withSubject})

	require.Len(t, feed.Items, 2)
	require.Equal(t, "No Subject", feed.Items[0].Title)
	require.Contains(t, feed.Items[0].Link.Href, "post-a")
	require.Equal(t, "A subject", feed.Items[1].Title)
	require.Contains(t, feed.Items[1].Link.Href, "post-b")
}

func TestToFeed_NoPosts(t *testing.T) {
	t.Parallel()

	feed := rss.ToFeed("Empty Blog", "https://example.com", nil, nil)
	require.Empty(t, feed.Items)
}
