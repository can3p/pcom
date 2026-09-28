package postops_test

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

func TestPostSubject_FallsBackWhenEmpty(t *testing.T) {
	t.Parallel()

	require.Equal(t, "No Subject", postops.PostSubject(null.String{}))
	require.Equal(t, "No Subject", postops.PostSubject(null.StringFrom("")))
	require.Equal(t, "Hello", postops.PostSubject(null.StringFrom("Hello")))
}

func TestPost_IsPublished(t *testing.T) {
	t.Parallel()

	draft := &postops.Post{Post: &core.Post{}}
	require.False(t, draft.IsPublished())

	published := &postops.Post{Post: &core.Post{PublishedAt: null.TimeFrom(time.Now())}}
	require.True(t, published.IsPublished())
}

func TestPost_PostSubject(t *testing.T) {
	t.Parallel()

	p := &postops.Post{Post: &core.Post{Subject: null.StringFrom("A title")}}
	require.Equal(t, "A title", p.PostSubject())

	empty := &postops.Post{Post: &core.Post{}}
	require.Equal(t, "No Subject", empty.PostSubject())
}

func TestConstructPost(t *testing.T) {
	t.Parallel()

	author := &core.User{Username: "alice"}
	viaUser := &core.User{Username: "bob"}

	t.Run("direct radius exposes the cached comment count", func(t *testing.T) {
		t.Parallel()

		dbPost := &core.Post{ID: "p1", Subject: null.StringFrom("hi")}
		dbPost.R = dbPost.R.NewStruct()
		dbPost.R.User = author
		dbPost.R.PostStat = &core.PostStat{CommentsNumber: 3}

		p := postops.ConstructPost(author, dbPost, userops.ConnectionRadiusDirect, []*core.User{viaUser}, false)

		require.Equal(t, author, p.Author)
		require.Equal(t, []*core.User{viaUser}, p.Via)
		require.EqualValues(t, 3, p.CommentsNumber)
		require.Equal(t, userops.ConnectionRadiusDirect, p.Radius)
		require.False(t, p.EditPreview)
		require.True(t, p.Capabilities.CanViewComments)
	})

	t.Run("unrelated radius hides the comment count even when cached", func(t *testing.T) {
		t.Parallel()

		dbPost := &core.Post{ID: "p2"}
		dbPost.R = dbPost.R.NewStruct()
		dbPost.R.User = author
		dbPost.R.PostStat = &core.PostStat{CommentsNumber: 9}

		p := postops.ConstructPost(author, dbPost, userops.ConnectionRadiusUnrelated, nil, true)

		require.EqualValues(t, 0, p.CommentsNumber)
		require.True(t, p.EditPreview)
		require.False(t, p.Capabilities.CanViewComments)
	})

	t.Run("carries the linked URL through when loaded", func(t *testing.T) {
		t.Parallel()

		dbPost := &core.Post{ID: "p3", URLID: null.StringFrom("url-1")}
		dbPost.R = dbPost.R.NewStruct()
		dbPost.R.User = author
		dbPost.R.URL = &core.NormalizedURL{ID: "url-1", URL: "https://example.com"}

		p := postops.ConstructPost(author, dbPost, userops.ConnectionRadiusSameUser, nil, false)

		require.NotNil(t, p.LinkedURL)
		require.Equal(t, "https://example.com", p.LinkedURL.URL)
	})
}

func TestComment_String(t *testing.T) {
	t.Parallel()

	c := &postops.Comment{
		PostComment: &core.PostComment{
			ID:              "c1",
			ParentCommentID: null.StringFrom("parent-1"),
			CreatedAt:       time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC),
		},
		Author: &core.User{Username: "alice"},
	}

	s := c.String()
	require.Contains(t, s, "c1")
	require.Contains(t, s, "parent-1")
	require.Contains(t, s, "alice")
}
