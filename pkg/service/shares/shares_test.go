package shares_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestShares(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := shares.New(repo.New(db))

	author := testutil.Must(factory.User(ctx, db))(t)
	friend := testutil.Must(factory.User(ctx, db))(t)
	_, _, err := factory.Connect(ctx, db, author.ID, friend.ID)
	require.NoError(t, err)

	newPost := func(t *testing.T, opts ...factory.PostOpt) *core.Post {
		return testutil.Must(factory.Post(ctx, db, author.ID, opts...))(t)
	}
	shareOf := func(t *testing.T, postID string) *core.PostShare {
		share, err := core.PostShares(core.PostShareWhere.PostID.EQ(postID)).One(ctx, db)
		require.NoError(t, err)
		return share
	}

	t.Run("the author creates one link, and deleting it breaks the URL", func(t *testing.T) {
		t.Parallel()

		post := newPost(t, factory.Published())

		require.NoError(t, svc.Create(ctx, author, post.ID))
		first := shareOf(t, post.ID)
		require.NoError(t, svc.Create(ctx, author, post.ID))
		require.Equal(t, first.ID, shareOf(t, post.ID).ID, "creating again keeps the link")

		shared := testutil.Must(svc.Get(ctx, first.ID))(t)
		require.Equal(t, post.ID, shared.Post.ID)
		require.Equal(t, author.ID, shared.Author.ID)

		require.NoError(t, svc.Delete(ctx, author, post.ID))
		_, err := svc.Get(ctx, first.ID)
		require.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("a link to a post that went back to draft resolves to nothing", func(t *testing.T) {
		t.Parallel()

		post := newPost(t)
		share := testutil.Must(factory.PostShare(ctx, db, post.ID))(t)

		_, err := svc.Get(ctx, share.ID)
		require.ErrorIs(t, err, service.ErrNotFound)
	})

	refused := []struct {
		name    string
		actor   *core.User
		postID  func(t *testing.T) string
		wantErr error
	}{
		{"a connection may not share", friend, func(t *testing.T) string { return newPost(t, factory.Published()).ID }, service.ErrForbidden},
		{"anonymous may not share", nil, func(t *testing.T) string { return newPost(t, factory.Published()).ID }, service.ErrNeedsLogin},
		{"a draft has no link", author, func(t *testing.T) string { return newPost(t).ID }, &service.ValidationError{Field: "postId", Message: "Cannot share a link for draft"}},
		{"unknown post", author, func(*testing.T) string { return uuid.NewString() }, service.ErrNotFound},
	}

	for _, tc := range refused {
		t.Run("create: "+tc.name, func(t *testing.T) {
			t.Parallel()

			postID := tc.postID(t)
			err := svc.Create(ctx, tc.actor, postID)

			if want, ok := tc.wantErr.(*service.ValidationError); ok {
				require.Equal(t, want, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}

			exists := testutil.Must(factory.ShareExists(ctx, db, postID))(t)
			require.False(t, exists)
		})
	}

	t.Run("delete: only the author removes the link", func(t *testing.T) {
		t.Parallel()

		post := newPost(t, factory.Published())
		testutil.Must(factory.PostShare(ctx, db, post.ID))(t)

		require.ErrorIs(t, svc.Delete(ctx, friend, post.ID), service.ErrForbidden)
		require.ErrorIs(t, svc.Delete(ctx, nil, post.ID), service.ErrNeedsLogin)
		require.True(t, testutil.Must(factory.ShareExists(ctx, db, post.ID))(t))
	})
}
