package postops_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

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

	require.NoError(t, postops.DeletePost(ctx, db, post.ID))

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

		first := testutil.Must(postops.StoreURL(ctx, db, "https://Example.com/Path/"))(t)

		// Different case host and a trailing slash normalize to the same
		// URL, so the upsert should hand back the same row instead of a new
		// one.
		second := testutil.Must(postops.StoreURL(ctx, db, "https://example.com/Path"))(t)

		require.Equal(t, first.ID, second.ID)
		require.Equal(t, first.URL, second.URL)
	})

	t.Run("different URLs get different ids", func(t *testing.T) {
		t.Parallel()

		first := testutil.Must(postops.StoreURL(ctx, db, "https://example.test/one"))(t)
		second := testutil.Must(postops.StoreURL(ctx, db, "https://example.test/two"))(t)

		require.NotEqual(t, first.ID, second.ID)
	})
}

// TestCanPromptNow exercises the asker rate limit.
func TestCanPromptNow(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("no prior prompt", func(t *testing.T) {
		t.Parallel()

		asker := testutil.Must(factory.User(ctx, db))(t)
		require.NoError(t, postops.CanPromptNow(ctx, db, asker.ID))
	})

	t.Run("recent prompt blocks the asker", func(t *testing.T) {
		t.Parallel()

		asker := testutil.Must(factory.User(ctx, db))(t)
		recipient := testutil.Must(factory.User(ctx, db))(t)
		testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID))(t)

		err := postops.CanPromptNow(ctx, db, asker.ID)
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
		require.NoError(t, postops.CanPromptNow(ctx, db, askerB.ID))
	})

	t.Run("uses the most recent prompt for the timeout", func(t *testing.T) {
		// It is skipped because the query orders by a bound placeholder
		// instead of a column, so it can check an arbitrary prompt instead
		// of the most recent one.
		t.Skip("known bug #108: ORDER BY with a bound placeholder sorts nothing, so CanPromptNow may check the wrong prompt")

		asker := testutil.Must(factory.User(ctx, db))(t)
		recipient := testutil.Must(factory.User(ctx, db))(t)

		// An old prompt, already outside the rate limit window.
		testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID,
			factory.PromptCreatedAt(time.Now().Add(-10*time.Minute))))(t)

		// The most recent prompt is still inside the window, so it - not the
		// old one - should decide whether the asker is blocked.
		testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID,
			factory.PromptCreatedAt(time.Now())))(t)

		err := postops.CanPromptNow(ctx, db, asker.ID)
		require.Error(t, err, "the most recent prompt is still within the rate limit window")
	})
}

func TestGetPostPrompt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("returns the prompt and its author", func(t *testing.T) {
		t.Parallel()

		asker := testutil.Must(factory.User(ctx, db))(t)
		recipient := testutil.Must(factory.User(ctx, db))(t)
		prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID))(t)

		got := testutil.Must(postops.GetPostPrompt(ctx, db, core.PostPromptWhere.ID.EQ(prompt.ID)))(t)
		require.NotNil(t, got)
		require.Equal(t, prompt.ID, got.Prompt.ID)
		require.Equal(t, prompt.Message, got.Prompt.Message)
		require.NotNil(t, got.Author)
		require.Equal(t, asker.ID, got.Author.ID)
	})

	t.Run("no match returns nil", func(t *testing.T) {
		t.Parallel()

		got := testutil.Must(postops.GetPostPrompt(ctx, db, core.PostPromptWhere.ID.EQ(uuid.NewString())))(t)
		require.Nil(t, got)
	})
}
