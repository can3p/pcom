package posts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// world is an author with a direct connection, a second-degree user (through
// the connection) and a stranger.
type world struct {
	author, direct, second, stranger *core.User
}

func newWorld(t *testing.T, ctx context.Context, db *sqlx.DB) world {
	t.Helper()

	w := world{}
	for _, u := range []**core.User{&w.author, &w.direct, &w.second, &w.stranger} {
		*u = testutil.Must(factory.User(ctx, db))(t)
	}

	_, _, err := factory.Connect(ctx, db, w.author.ID, w.direct.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, w.direct.ID, w.second.ID)
	require.NoError(t, err)

	return w
}

func TestDelete_OnlyTheAuthor(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	s := svc(db, nil)
	del := func(actor *core.User, id string) error { return s.Delete(ctx, actor, id) }
	delDraft := func(actor *core.User, id string) error { return s.DeleteDraft(ctx, actor, id) }

	for _, tc := range []struct {
		name      string
		act       func(actor *core.User, postID string) error
		published bool
		actor     *core.User
		want      error
	}{
		{"delete: connection", del, true, w.direct, service.ErrNotFound},
		{"delete: stranger", del, true, w.stranger, service.ErrNotFound},
		{"delete: anonymous", del, true, nil, service.ErrNeedsLogin},
		{"delete draft: connection", delDraft, false, w.direct, nil},
		{"delete draft: stranger", delDraft, false, w.stranger, nil},
		{"delete draft: anonymous", delDraft, false, nil, service.ErrNeedsLogin},
		{"delete draft: author, published post", delDraft, true, w.author, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := []factory.PostOpt{}
			if tc.published {
				opts = append(opts, factory.Published())
			}

			post := testutil.Must(factory.Post(ctx, db, w.author.ID, opts...))(t)

			err := tc.act(tc.actor, post.ID)
			require.Error(t, err)

			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			}

			_, err = factory.GetPost(ctx, db, post.ID)
			require.NoError(t, err, "the post must survive")
		})
	}
}

func TestEditAndSave_OnlyTheAuthor(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	s := posts.New(repo.New(db), fakesender.New(), nil)

	for _, tc := range []struct {
		name  string
		actor *core.User
		want  error // for ForEdit and CheckEdit
	}{
		{"connection", w.direct, service.ErrForbidden},
		{"stranger", w.stranger, service.ErrForbidden},
		{"anonymous", nil, service.ErrNeedsLogin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)

			_, err := s.ForEdit(ctx, tc.actor, post.ID)
			require.ErrorIs(t, err, tc.want)
			require.ErrorIs(t, s.CheckEdit(ctx, tc.actor, post), tc.want)

			// somebody else's post is not found to a writer
			wantSave := service.ErrNotFound
			if tc.actor == nil {
				wantSave = service.ErrNeedsLogin
			}

			for _, action := range []posts.Action{posts.ActionSavePost, posts.ActionPublish, posts.ActionMakeDraft, posts.ActionDelete} {
				_, err = s.Save(ctx, tc.actor, posts.SaveInput{
					PostID: post.ID, Subject: "hijacked", Body: "hijacked",
					Visibility: core.PostVisibilityPublic, Action: action,
				})
				require.ErrorIs(t, err, wantSave, string(action))
			}

			got := testutil.Must(factory.GetPost(ctx, db, post.ID))(t)
			require.Equal(t, post.Body, got.Body)
			require.Equal(t, post.Subject, got.Subject)
			require.Equal(t, post.PublishedAt.Valid, got.PublishedAt.Valid)
		})
	}

	t.Run("the author may", func(t *testing.T) {
		t.Parallel()

		post := testutil.Must(factory.Post(ctx, db, w.author.ID))(t)
		_, err := s.ForEdit(ctx, w.author, post.ID)
		require.NoError(t, err)
		require.NoError(t, s.CheckEdit(ctx, w.author, post))
	})
}

func TestComment_ByRadius(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	s := posts.New(repo.New(db), fakesender.New(), nil)

	otherPost := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	otherComment := testutil.Must(factory.Comment(ctx, db, otherPost.ID, w.author.ID))(t)

	for _, tc := range []struct {
		name    string
		actor   *core.User
		replyTo string
		want    error
	}{
		{"author", w.author, "", nil},
		{"direct connection", w.direct, "", nil},
		{"second degree", w.second, "", service.ErrForbidden},
		{"stranger", w.stranger, "", service.ErrForbidden},
		{"anonymous", nil, "", service.ErrNeedsLogin},
		{"reply to a comment of another post", w.direct, otherComment.ID, service.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// each case has its own post, so the counts don't race
			post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
			before := 0

			check := s.CheckComment(ctx, tc.actor, post.ID, tc.replyTo)
			add := s.AddComment(ctx, tc.actor, posts.CommentInput{PostID: post.ID, ReplyTo: tc.replyTo, Body: "a fine comment"})

			after := len(testutil.Must(factory.ListComments(ctx, db, post.ID))(t))

			if tc.want == nil {
				require.NoError(t, check)
				require.NoError(t, add)
				require.Equal(t, before+1, after)

				return
			}

			require.ErrorIs(t, check, tc.want)
			require.ErrorIs(t, add, tc.want)
			require.Equal(t, before, after, "a rejected comment stores nothing")
		})
	}
}

func TestDismissPrompt_OnlyTheRecipient(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	s := svc(db, nil)

	for _, tc := range []struct {
		name  string
		actor *core.User
		ok    bool
	}{
		{"asker", w.author, false},
		{"stranger", w.stranger, false},
		{"anonymous", nil, false},
		{"recipient", w.direct, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prompt := testutil.Must(factory.PostPrompt(ctx, db, w.author.ID, w.direct.ID))(t)

			err := s.DismissPrompt(ctx, tc.actor, prompt.ID)
			got := testutil.Must(factory.GetPostPrompt(ctx, db, prompt.ID))(t)

			if tc.ok {
				require.NoError(t, err)
				require.True(t, got.DismissedAt.Valid)

				return
			}

			require.Error(t, err)
			require.False(t, got.DismissedAt.Valid, "the prompt stays undismissed")
		})
	}
}
