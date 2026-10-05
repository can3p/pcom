package postops_test

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/stretchr/testify/require"
)

func TestPostSubject_FallsBackWhenEmpty(t *testing.T) {
	t.Parallel()

	require.Equal(t, "No Subject", postops.PostSubject(nil))
	require.Equal(t, "No Subject", postops.PostSubject(new("")))
	require.Equal(t, "Hello", postops.PostSubject(new("Hello")))
}

func TestPost_IsPublished(t *testing.T) {
	t.Parallel()

	draft := &postops.Post{Post: &model.Post{}}
	require.False(t, draft.IsPublished())

	published := &postops.Post{Post: &model.Post{PublishedAt: new(time.Now())}}
	require.True(t, published.IsPublished())
}

func TestPost_PostSubject(t *testing.T) {
	t.Parallel()

	p := &postops.Post{Post: &model.Post{Subject: new("A title")}}
	require.Equal(t, "A title", p.PostSubject())

	empty := &postops.Post{Post: &model.Post{}}
	require.Equal(t, "No Subject", empty.PostSubject())
}

func TestConstructPost(t *testing.T) {
	t.Parallel()

	author := &model.User{Username: "alice"}
	viaUser := &model.User{Username: "bob"}

	t.Run("direct radius exposes the cached comment count", func(t *testing.T) {
		t.Parallel()

		dbPost := &model.Post{ID: "p1", Subject: new("hi")}
		dbPost.User = author
		dbPost.PostStat = &model.PostStat{CommentsNumber: 3}

		p := postops.ConstructPost(author, dbPost, graph.RadiusDirect, []*model.User{viaUser}, false)

		require.Equal(t, author, p.Author)
		require.Equal(t, []*model.User{viaUser}, p.Via)
		require.EqualValues(t, 3, p.CommentsNumber)
		require.Equal(t, graph.RadiusDirect, p.Radius)
		require.False(t, p.EditPreview)
		require.True(t, p.Capabilities.CanViewComments)
	})

	t.Run("unrelated radius hides the comment count even when cached", func(t *testing.T) {
		t.Parallel()

		dbPost := &model.Post{ID: "p2"}
		dbPost.User = author
		dbPost.PostStat = &model.PostStat{CommentsNumber: 9}

		p := postops.ConstructPost(author, dbPost, graph.RadiusUnrelated, nil, true)

		require.EqualValues(t, 0, p.CommentsNumber)
		require.True(t, p.EditPreview)
		require.False(t, p.Capabilities.CanViewComments)
	})

	t.Run("carries the linked URL through when loaded", func(t *testing.T) {
		t.Parallel()

		dbPost := &model.Post{ID: "p3", URLID: new("url-1")}
		dbPost.User = author
		dbPost.URL = &model.NormalizedURL{ID: "url-1", URL: "https://example.com"}

		p := postops.ConstructPost(author, dbPost, graph.RadiusSameUser, nil, false)

		require.NotNil(t, p.LinkedURL)
		require.Equal(t, "https://example.com", p.LinkedURL.URL)
	})
}

func TestComment_String(t *testing.T) {
	t.Parallel()

	c := &postops.Comment{
		PostComment: &model.PostComment{
			ID:              "c1",
			ParentCommentID: new("parent-1"),
			CreatedAt:       time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC),
		},
		Author: &model.User{Username: "alice"},
	}

	s := c.String()
	require.Contains(t, s, "c1")
	require.Contains(t, s, "parent-1")
	require.Contains(t, s, "alice")
}
