package reading

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The sources a reader may translate are the ones they may read: published,
// visible posts and the items of the feeds they follow.
func TestTranslateSources(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))

	actor := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	second := testutil.Must(factory.User(ctx, db))(t)
	stranger := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, actor.ID, direct.ID)
	connect(t, db, ctx, direct.ID, second.ID)

	post := func(author *core.User, vis core.PostVisibility, opts ...factory.PostOpt) *core.Post {
		opts = append([]factory.PostOpt{factory.Visibility(vis)}, opts...)
		return testutil.Must(factory.Post(ctx, db, author.ID, opts...))(t)
	}

	own := post(actor, core.PostVisibilityDirectOnly, factory.Published())
	fromDirect := post(direct, core.PostVisibilityDirectOnly, factory.Published())
	fromSecond := post(second, core.PostVisibilitySecondDegree, factory.Published())
	hiddenSecond := post(second, core.PostVisibilityDirectOnly, factory.Published())
	hiddenStranger := post(stranger, core.PostVisibilityDirectOnly, factory.Published())
	draft := post(direct, core.PostVisibilityPublic)

	t.Run("PostToTranslate", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			actor *core.User
			id    string
			want  error
		}{
			{"anonymous", nil, own.ID, service.ErrNeedsLogin},
			{"unknown", actor, uuid.NewString(), service.ErrNotFound},
			{"a draft", actor, draft.ID, service.ErrNotFound},
			{"not visible", actor, hiddenStranger.ID, service.ErrNotFound},
			{"own", actor, own.ID, nil},
			{"direct", actor, fromDirect.ID, nil},
		} {
			got, err := svc.PostToTranslate(ctx, tc.actor, tc.id)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want, tc.name)
				continue
			}

			require.NoError(t, err, tc.name)
			require.Equal(t, tc.id, got.ID)
		}
	})

	t.Run("PostsToTranslate", func(t *testing.T) {
		batch := []*core.Post{own, fromDirect, fromSecond, hiddenSecond, hiddenStranger, draft}

		none, err := svc.PostsToTranslate(ctx, nil, batch)
		require.NoError(t, err)
		require.Empty(t, none)

		got, err := svc.PostsToTranslate(ctx, actor, batch)
		require.NoError(t, err)
		require.Equal(t, []string{own.ID, fromDirect.ID, fromSecond.ID}, []string{got[0].ID, got[1].ID, got[2].ID})
		require.Len(t, got, 3)
	})

	t.Run("RSSItemToTranslate", func(t *testing.T) {
		feed := testutil.Must(factory.RSSFeed(ctx, db))(t)
		item := testutil.Must(factory.RSSItem(ctx, db, feed.ID))(t)
		testutil.Must(factory.Subscription(ctx, db, actor.ID, feed.ID))(t)

		_, err := svc.RSSItemToTranslate(ctx, nil, item.ID)
		require.ErrorIs(t, err, service.ErrNeedsLogin)

		_, err = svc.RSSItemToTranslate(ctx, actor, uuid.NewString())
		require.ErrorIs(t, err, service.ErrNotFound)

		_, err = svc.RSSItemToTranslate(ctx, stranger, item.ID)
		require.ErrorIs(t, err, service.ErrNotFound)

		got, err := svc.RSSItemToTranslate(ctx, actor, item.ID)
		require.NoError(t, err)
		require.Equal(t, item.ID, got.ID)
	})

	t.Run("PublicPostsRSS", func(t *testing.T) {
		posts, err := svc.PublicPostsRSS(ctx)
		require.NoError(t, err)
		require.NotNil(t, posts)
	})
}

// A store failure is returned, never mistaken for "not found".
func TestTranslateSources_StoreErrors(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := New(repo.Using(db))
	actor := testutil.Must(factory.User(context.Background(), db))(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.PostToTranslate(ctx, actor, uuid.NewString())
	require.ErrorIs(t, err, context.Canceled)

	_, err = svc.PostsToTranslate(ctx, actor, []*core.Post{{ID: uuid.NewString()}})
	require.ErrorIs(t, err, context.Canceled)
}
