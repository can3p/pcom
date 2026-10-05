package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestCommentQueries(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	author := testutil.Must(factory.User(ctx, db))(t)
	alice := testutil.Must(factory.User(ctx, db))(t)
	bob := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	other := testutil.Must(factory.Post(ctx, db, author.ID))(t)

	base := time.Now().Add(-time.Hour)
	c1 := testutil.Must(factory.Comment(ctx, db, post.ID, alice.ID, factory.CommentCreatedAt(base.Add(2*time.Minute))))(t)
	c2 := testutil.Must(factory.Comment(ctx, db, post.ID, alice.ID, factory.CommentCreatedAt(base)))(t)
	c3 := testutil.Must(factory.Comment(ctx, db, post.ID, bob.ID, factory.CommentCreatedAt(base.Add(time.Minute))))(t)
	testutil.Must(factory.Comment(ctx, db, post.ID, author.ID))(t)
	testutil.Must(factory.Comment(ctx, db, other.ID, alice.ID))(t)

	t.Run("CommentParticipants has one row per commenter but the author", func(t *testing.T) {
		got := testutil.Must(store.CommentParticipants(ctx, post.ID, author.ID))(t)
		ids := lo.Map(got, func(c *model.PostComment, _ int) string { return c.User.ID })
		require.ElementsMatch(t, []string{alice.ID, bob.ID}, ids)
	})

	t.Run("CommentsOfPost is oldest first with authors", func(t *testing.T) {
		got := testutil.Must(store.CommentsOfPost(ctx, post.ID))(t)
		require.Len(t, got, 4)
		require.Equal(t, []string{c2.ID, c3.ID, c1.ID}, lo.Map(got[:3], func(c *model.PostComment, _ int) string { return c.ID }))
		require.Equal(t, alice.ID, got[0].User.ID)
	})

	t.Run("CommentedPostIDs is distinct", func(t *testing.T) {
		require.ElementsMatch(t, []string{post.ID, other.ID}, testutil.Must(store.CommentedPostIDs(ctx, alice.ID))(t))
		require.Empty(t, testutil.Must(store.CommentedPostIDs(ctx, testutil.Must(factory.User(ctx, db))(t).ID))(t))
	})

	t.Run("CommentInPost checks the post", func(t *testing.T) {
		got := testutil.Must(store.CommentInPost(ctx, c1.ID, post.ID))(t)
		require.Equal(t, c1.ID, got.ID)

		_, err := store.CommentInPost(ctx, c1.ID, other.ID)
		require.ErrorIs(t, err, repo.ErrNotFound)
	})

	t.Run("CommentForMail loads post, its author and the commenter", func(t *testing.T) {
		got := testutil.Must(store.CommentForMail(ctx, c3.ID))(t)
		require.Equal(t, post.ID, got.Post.ID)
		require.Equal(t, author.ID, got.Post.User.ID)
		require.Equal(t, bob.ID, got.User.ID)
		require.Nil(t, got.Post.URL)
	})

	t.Run("UpdateCommentBody writes body and edited_at", func(t *testing.T) {
		require.NoError(t, store.UpdateCommentBody(ctx, c2, "new body"))

		got := testutil.Must(store.CommentByID(ctx, c2.ID))(t)
		require.Equal(t, "new body", got.Body)
		require.NotNil(t, got.EditedAt)
	})

	t.Run("InsertComment and PostWithAuthorAndURL", func(t *testing.T) {
		id := uuid.NewString()
		c := &model.PostComment{ID: id, UserID: bob.ID, PostID: other.ID, Body: "b", TopCommentID: id}
		require.NoError(t, store.InsertComment(ctx, c))
		require.False(t, c.CreatedAt.IsZero())

		p := testutil.Must(store.PostWithAuthorAndURL(ctx, other.ID))(t)
		require.Equal(t, author.ID, p.User.ID)
		require.Nil(t, p.URL)

		_, err := store.PostWithAuthorAndURL(ctx, uuid.NewString())
		require.ErrorIs(t, err, repo.ErrNotFound)
	})

	t.Run("CountNewComment counts up", func(t *testing.T) {
		require.NoError(t, store.CountNewComment(ctx, post.ID))
		require.NoError(t, store.CountNewComment(ctx, post.ID))

		var n int64
		require.NoError(t, db.GetContext(ctx, &n, "SELECT comments_number FROM post_stats WHERE post_id = $1", post.ID))
		require.EqualValues(t, 2, n)
	})
}

func TestPromptQueries(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	asker := testutil.Must(factory.User(ctx, db))(t)
	recipient := testutil.Must(factory.User(ctx, db))(t)
	answer := testutil.Must(factory.Post(ctx, db, recipient.ID))(t)

	base := time.Now().Add(-time.Hour)
	older := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID, factory.PromptCreatedAt(base), factory.WithPost(answer.ID)))(t)
	newer := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID, factory.PromptCreatedAt(base.Add(time.Minute))))(t)
	testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID, factory.Dismissed()))(t)

	t.Run("OpenPromptsFor skips dismissed, newest first", func(t *testing.T) {
		got := testutil.Must(store.OpenPromptsFor(ctx, recipient.ID))(t)
		require.Equal(t, []string{newer.ID, older.ID}, lo.Map(got, func(p *model.PostPrompt, _ int) string { return p.ID }))
		require.Equal(t, asker.ID, got[0].Asker.ID)
		require.Nil(t, got[0].Post)
		require.Equal(t, answer.ID, got[1].Post.ID)
	})

	t.Run("PromptForPost and PromptForRecipient", func(t *testing.T) {
		require.Equal(t, older.ID, testutil.Must(store.PromptForPost(ctx, answer.ID))(t).ID)

		got := testutil.Must(store.PromptForRecipient(ctx, recipient.ID, newer.ID))(t)
		require.Equal(t, asker.ID, got.Asker.ID)

		_, err := store.PromptForRecipient(ctx, asker.ID, newer.ID)
		require.ErrorIs(t, err, repo.ErrNotFound)
	})

	t.Run("InsertPrompt, UpdatePrompt and LastPromptBy", func(t *testing.T) {
		_, err := store.LastPromptBy(ctx, recipient.ID)
		require.ErrorIs(t, err, repo.ErrNotFound)

		p := &model.PostPrompt{ID: uuid.NewString(), AskerID: recipient.ID, RecipientID: asker.ID, Message: "hi"}
		require.NoError(t, store.InsertPrompt(ctx, p))
		require.False(t, p.CreatedAt.IsZero())

		p.Message = "edited"
		require.NoError(t, store.UpdatePrompt(ctx, p))

		got := testutil.Must(store.LastPromptBy(ctx, recipient.ID))(t)
		require.Equal(t, p.ID, got.ID)
		require.Equal(t, "edited", got.Message)
	})

	t.Run("DismissPrompt only for the recipient", func(t *testing.T) {
		require.ErrorIs(t, store.DismissPrompt(ctx, asker.ID, newer.ID, time.Now()), repo.ErrNotFound)
		require.NoError(t, store.DismissPrompt(ctx, recipient.ID, newer.ID, time.Now()))

		got := testutil.Must(store.OpenPromptsFor(ctx, recipient.ID))(t)
		require.Len(t, got, 1)
	})
}

func TestShareQueries(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	author := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

	share, err := store.ShareOfPost(ctx, post.ID)
	require.NoError(t, err)
	require.Nil(t, share)

	require.NoError(t, store.CreateShare(ctx, post.ID))
	first := testutil.Must(store.ShareOfPost(ctx, post.ID))(t)

	require.NoError(t, store.CreateShare(ctx, post.ID))
	require.Equal(t, first.ID, testutil.Must(store.ShareOfPost(ctx, post.ID))(t).ID)

	got := testutil.Must(store.ShareByID(ctx, first.ID))(t)
	require.Equal(t, author.ID, got.Post.User.ID)

	require.NoError(t, store.DeleteShares(ctx, post.ID))
	_, err = store.ShareByID(ctx, first.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}
