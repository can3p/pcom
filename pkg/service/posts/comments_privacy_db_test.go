package posts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// mailsTo is the mail recorded for one address.
func mailsTo(out *fakesender.Sender, email string) []fakesender.Recorded {
	var got []fakesender.Recorded

	for _, m := range out.Sent() {
		if m.Mail.To[0].Address == email {
			got = append(got, m)
		}
	}

	return got
}

// Somebody else's draft is not found for commenting, also when it was
// published before; its author may still comment on it.
func TestComment_Draft(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	s := posts.New(repo.New(db), fakesender.New(), nil)

	draft := testutil.Must(factory.Post(ctx, db, w.author.ID))(t)
	parent := testutil.Must(factory.Comment(ctx, db, draft.ID, w.author.ID))(t)

	require.ErrorIs(t, s.CheckComment(ctx, w.direct, draft.ID, ""), service.ErrNotFound)
	require.ErrorIs(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: draft.ID, Body: "on your draft"}), service.ErrNotFound)
	require.ErrorIs(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: draft.ID, ReplyTo: parent.ID, Body: "on your draft"}), service.ErrNotFound)
	require.NoError(t, s.AddComment(ctx, w.author, posts.CommentInput{PostID: draft.ID, Body: "a note on my draft"}))

	saved, err := s.Save(ctx, w.author, posts.SaveInput{Subject: "published", Body: "a body for the post",
		Visibility: model.PostVisibilityPublic, Action: posts.ActionPublish})
	require.NoError(t, err)

	own := testutil.Must(factory.Comment(ctx, db, saved.Post.ID, w.direct.ID))(t)

	_, err = s.Save(ctx, w.author, posts.SaveInput{PostID: saved.Post.ID, Subject: "withdrawn", Body: "a body for the post",
		Visibility: model.PostVisibilityPublic, Action: posts.ActionMakeDraft})
	require.NoError(t, err)

	require.ErrorIs(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: saved.Post.ID, Body: "still here"}), service.ErrNotFound)
	require.ErrorIs(t, s.EditComment(ctx, w.direct, own.ID, "edited after the withdrawal"), service.ErrNotFound)
}

// The comment mails reach only the participants who may still read the
// comments: not one who lost the connection, nor anybody but the author once
// the post is a draft again.
func TestCommentMail_OnlyToWhoMayRead(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	out := fakesender.New()
	store := repo.New(db)
	s := posts.New(store, out, nil)

	other := testutil.Must(factory.User(ctx, db))(t)
	_, _, err := factory.Connect(ctx, db, w.author.ID, other.ID)
	require.NoError(t, err)

	saved, err := s.Save(ctx, w.author, posts.SaveInput{Subject: "direct only", Body: "a body for the post",
		Visibility: model.PostVisibilityDirectOnly, Action: posts.ActionPublish})
	require.NoError(t, err)

	postID := saved.Post.ID
	require.NoError(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: postID, Body: "from the direct connection"}))
	require.NoError(t, s.AddComment(ctx, other, posts.CommentInput{PostID: postID, Body: "from the other connection"}))

	require.NoError(t, store.DeleteConnectionsBetween(ctx, w.author.ID, w.direct.ID))

	before := len(mailsTo(out, w.direct.Email))
	require.NoError(t, s.AddComment(ctx, w.author, posts.CommentInput{PostID: postID, Body: "after the removal"}))
	require.Len(t, mailsTo(out, w.direct.Email), before, "a removed connection gets no more comments")
	require.NotEmpty(t, mailsTo(out, other.Email))

	_, err = s.Save(ctx, w.author, posts.SaveInput{PostID: postID, Subject: "withdrawn", Body: "a body for the post",
		Visibility: model.PostVisibilityDirectOnly, Action: posts.ActionMakeDraft})
	require.NoError(t, err)

	before = len(mailsTo(out, other.Email))
	require.NoError(t, s.AddComment(ctx, w.author, posts.CommentInput{PostID: postID, Body: "on my draft"}))
	require.Len(t, mailsTo(out, other.Email), before, "nobody is told about comments on a draft")
}

// The asker of a prompt is told about the answer only when they may read it.
func TestPromptAnswerMail_OnlyToWhoMayRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		visibility model.PostVisibility
		want       int
	}{
		{"direct only", model.PostVisibilityDirectOnly, 0},
		{"public", model.PostVisibilityPublic, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := testdb.New(t).DB
			ctx := context.Background()
			w := newWorld(t, ctx, db)
			out := fakesender.New()
			store := repo.New(db)
			s := posts.New(store, out, nil)

			prompt := testutil.Must(factory.PostPrompt(ctx, db, w.direct.ID, w.author.ID))(t)
			require.NoError(t, store.DeleteConnectionsBetween(ctx, w.author.ID, w.direct.ID))

			_, err := s.Save(ctx, w.author, posts.SaveInput{PromptID: prompt.ID, Subject: "the answer",
				Body: "a body for the post", Visibility: tc.visibility, Action: posts.ActionPublish})
			require.NoError(t, err)
			require.Len(t, mailsTo(out, w.direct.Email), tc.want)
		})
	}
}
