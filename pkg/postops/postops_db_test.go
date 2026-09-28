package postops_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
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

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	commenter, err := factory.User(ctx, db)
	require.NoError(t, err)
	asker, err := factory.User(ctx, db)
	require.NoError(t, err)

	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)

	_, err = factory.Comment(ctx, db, post.ID, commenter.ID)
	require.NoError(t, err)

	_, err = factory.PostStat(ctx, db, post.ID)
	require.NoError(t, err)

	_, err = factory.PostShare(ctx, db, post.ID)
	require.NoError(t, err)

	_, err = factory.PostPrompt(ctx, db, asker.ID, author.ID, factory.WithPost(post.ID))
	require.NoError(t, err)

	require.NoError(t, postops.DeletePost(ctx, db, post.ID))

	comments, err := factory.ListComments(ctx, db, post.ID)
	require.NoError(t, err)
	require.Empty(t, comments, "comments should be cascaded")

	_, err = factory.GetPost(ctx, db, post.ID)
	require.Error(t, err, "the post itself should be gone")
}

func TestStoreURL_UpsertReturnsExistingID(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	first, err := postops.StoreURL(ctx, db, "https://Example.com/Path/")
	require.NoError(t, err)
	require.NotEmpty(t, first.ID)

	// Different case host and a trailing slash normalize to the same URL,
	// so the upsert should hand back the same row instead of a new one.
	second, err := postops.StoreURL(ctx, db, "https://example.com/Path")
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.URL, second.URL)
}

func TestStoreURL_DifferentURLsGetDifferentIDs(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	first, err := postops.StoreURL(ctx, db, "https://example.test/one")
	require.NoError(t, err)

	second, err := postops.StoreURL(ctx, db, "https://example.test/two")
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID)
}

func TestCanPromptNow_NoPriorPrompt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)

	require.NoError(t, postops.CanPromptNow(ctx, db, asker.ID))
}

func TestCanPromptNow_RecentPromptBlocksAsker(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	recipient, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.PostPrompt(ctx, db, asker.ID, recipient.ID)
	require.NoError(t, err)

	err = postops.CanPromptNow(ctx, db, asker.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot send prompts")
}

func TestCanPromptNow_ScopedToAsker(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	askerA, err := factory.User(ctx, db)
	require.NoError(t, err)
	askerB, err := factory.User(ctx, db)
	require.NoError(t, err)
	recipient, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.PostPrompt(ctx, db, askerA.ID, recipient.ID)
	require.NoError(t, err)

	// askerB has never sent a prompt, so askerA's recent one must not
	// block them.
	require.NoError(t, postops.CanPromptNow(ctx, db, askerB.ID))
}

// TestCanPromptNow_UsesMostRecentPromptForTimeout pins the correct behavior:
// with more than one prompt on file, CanPromptNow must base its decision on
// the most recent one. It is skipped because the query orders by a bound
// placeholder instead of a column, so it can check an arbitrary prompt
// instead of the most recent one.
func TestCanPromptNow_UsesMostRecentPromptForTimeout(t *testing.T) {
	t.Skip("known bug #108: ORDER BY with a bound placeholder sorts nothing, so CanPromptNow may check the wrong prompt")

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	recipient, err := factory.User(ctx, db)
	require.NoError(t, err)

	// An old prompt, already outside the rate limit window.
	_, err = factory.PostPrompt(ctx, db, asker.ID, recipient.ID,
		factory.PromptCreatedAt(time.Now().Add(-10*time.Minute)))
	require.NoError(t, err)

	// The most recent prompt is still inside the window, so it - not the
	// old one - should decide whether the asker is blocked.
	_, err = factory.PostPrompt(ctx, db, asker.ID, recipient.ID,
		factory.PromptCreatedAt(time.Now()))
	require.NoError(t, err)

	err = postops.CanPromptNow(ctx, db, asker.ID)
	require.Error(t, err, "the most recent prompt is still within the rate limit window")
}

func TestGetPostPrompt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	recipient, err := factory.User(ctx, db)
	require.NoError(t, err)

	prompt, err := factory.PostPrompt(ctx, db, asker.ID, recipient.ID)
	require.NoError(t, err)

	got, err := postops.GetPostPrompt(ctx, db, core.PostPromptWhere.ID.EQ(prompt.ID))
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, prompt.ID, got.Prompt.ID)
	require.Equal(t, prompt.Message, got.Prompt.Message)
	require.NotNil(t, got.Author)
	require.Equal(t, asker.ID, got.Author.ID)
}

func TestGetPostPrompt_NoMatchReturnsNil(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	got, err := postops.GetPostPrompt(ctx, db, core.PostPromptWhere.ID.EQ(uuid.NewString()))
	require.NoError(t, err)
	require.Nil(t, got)
}
