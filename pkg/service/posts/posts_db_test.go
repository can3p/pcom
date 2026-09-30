package posts_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// svc is the service over db and a media storage.
func svc(db *sqlx.DB, storage server.MediaStorage) *posts.Service {
	return posts.New(repo.New(db), nil, storage)
}

// PostStat, PostShare and PostPrompt all reference posts(id) with no ON
// DELETE action, so if DeletePost failed to remove them first, deleting the
// post row itself would fail with a foreign key violation instead of
// succeeding below.
func TestDeletePost_CascadesRelatedRows(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author := testutil.Must(factory.User(ctx, db))(t)
	commenter := testutil.Must(factory.User(ctx, db))(t)
	asker := testutil.Must(factory.User(ctx, db))(t)

	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	testutil.Must(factory.Comment(ctx, db, post.ID, commenter.ID))(t)
	testutil.Must(factory.PostStat(ctx, db, post.ID))(t)
	testutil.Must(factory.PostShare(ctx, db, post.ID))(t)
	testutil.Must(factory.PostPrompt(ctx, db, asker.ID, author.ID, factory.WithPost(post.ID)))(t)

	require.NoError(t, svc(db, nil).Delete(ctx, author, post.ID))

	comments := testutil.Must(factory.ListComments(ctx, db, post.ID))(t)
	require.Empty(t, comments, "comments should be cascaded")

	_, err := factory.GetPost(ctx, db, post.ID)
	require.Error(t, err, "the post itself should be gone")
}

func TestStoreURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("upsert returns the existing id", func(t *testing.T) {
		t.Parallel()

		first := testutil.Must(repo.New(db).StoreURL(ctx, "https://Example.com/Path/"))(t)

		// Different case host and a trailing slash normalize to the same
		// URL, so the upsert should hand back the same row instead of a new
		// one.
		second := testutil.Must(repo.New(db).StoreURL(ctx, "https://example.com/Path"))(t)

		require.Equal(t, first.ID, second.ID)
		require.Equal(t, first.URL, second.URL)
	})

	t.Run("different URLs get different ids", func(t *testing.T) {
		t.Parallel()

		first := testutil.Must(repo.New(db).StoreURL(ctx, "https://example.test/one"))(t)
		second := testutil.Must(repo.New(db).StoreURL(ctx, "https://example.test/two"))(t)

		require.NotEqual(t, first.ID, second.ID)
	})
}

// TestCanPrompt exercises the asker rate limit.
func TestCanPrompt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("no prior prompt", func(t *testing.T) {
		t.Parallel()

		asker := testutil.Must(factory.User(ctx, db))(t)
		require.NoError(t, svc(db, nil).CanPrompt(ctx, asker))
	})

	t.Run("recent prompt blocks the asker", func(t *testing.T) {
		t.Parallel()

		asker := testutil.Must(factory.User(ctx, db))(t)
		recipient := testutil.Must(factory.User(ctx, db))(t)
		testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID))(t)

		err := svc(db, nil).CanPrompt(ctx, asker)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot send prompts")
	})

	t.Run("scoped to the asker", func(t *testing.T) {
		t.Parallel()

		askerA := testutil.Must(factory.User(ctx, db))(t)
		askerB := testutil.Must(factory.User(ctx, db))(t)
		recipient := testutil.Must(factory.User(ctx, db))(t)
		testutil.Must(factory.PostPrompt(ctx, db, askerA.ID, recipient.ID))(t)

		// askerB has never sent a prompt, so askerA's recent one must not
		// block them.
		require.NoError(t, svc(db, nil).CanPrompt(ctx, askerB))
	})

	t.Run("uses the most recent prompt for the timeout", func(t *testing.T) {
		asker := testutil.Must(factory.User(ctx, db))(t)
		recipient := testutil.Must(factory.User(ctx, db))(t)

		// An old prompt, already outside the rate limit window. Inserted
		// first, so an unordered query returns it.
		testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID,
			factory.PromptCreatedAt(time.Now().Add(-10*time.Minute))))(t)

		// The most recent prompt is still inside the window, so it - not the
		// old one - should decide whether the asker is blocked.
		testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID,
			factory.PromptCreatedAt(time.Now())))(t)

		err := svc(db, nil).CanPrompt(ctx, asker)
		require.Error(t, err, "the most recent prompt is still within the rate limit window")
	})
}

func TestPromptFor(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("returns the prompt and its author", func(t *testing.T) {
		t.Parallel()

		asker := testutil.Must(factory.User(ctx, db))(t)
		recipient := testutil.Must(factory.User(ctx, db))(t)
		prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID))(t)

		got := testutil.Must(svc(db, nil).PromptFor(ctx, recipient, prompt.ID))(t)
		require.NotNil(t, got)
		require.Equal(t, prompt.ID, got.Prompt.ID)
		require.Equal(t, prompt.Message, got.Prompt.Message)
		require.NotNil(t, got.Author)
		require.Equal(t, asker.ID, got.Author.ID)
	})

	t.Run("no match returns nil", func(t *testing.T) {
		t.Parallel()

		recipient := testutil.Must(factory.User(ctx, db))(t)
		got := testutil.Must(svc(db, nil).PromptFor(ctx, recipient, uuid.NewString()))(t)
		require.Nil(t, got)
	})
}
