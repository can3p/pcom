package posts_test

import (
	"context"
	"database/sql"
	"errors"
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

// commentsNumber is the comment counter of a post, zero without a stat row.
func commentsNumber(t *testing.T, ctx context.Context, db *sqlx.DB, postID string) int64 {
	t.Helper()

	stat, err := core.PostStats(core.PostStatWhere.PostID.EQ(postID)).One(ctx, db)
	if errors.Is(err, sql.ErrNoRows) {
		return 0
	}

	require.NoError(t, err)

	return stat.CommentsNumber
}

func TestEditComment(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	out := fakesender.New()
	s := posts.New(repo.New(db), out, nil)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	comment := testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)
	// w.second has commented on the post before, so it follows the discussion
	testutil.Must(factory.Comment(ctx, db, post.ID, w.second.ID))(t)

	stored := func() *core.PostComment {
		return testutil.Must(core.FindPostComment(ctx, db, comment.ID))(t)
	}

	// the comment has never been edited, the same body with spaces around changes nothing
	counter := commentsNumber(t, ctx, db, post.ID)
	require.NoError(t, s.EditComment(ctx, w.direct, comment.ID, "  "+comment.Body+" \n"))
	require.False(t, stored().EditedAt.Valid)
	require.Empty(t, out.Sent())

	// two successive edits mail once per recipient each, the queue dedups on the unique id
	var prevEditedAt = stored().EditedAt

	for i, body := range []string{"first edit", "second edit"} {
		require.NoError(t, s.EditComment(ctx, w.direct, comment.ID, "  "+body+"\n"))

		got := stored()
		require.Equal(t, body, got.Body)
		require.True(t, got.EditedAt.Valid)
		require.True(t, got.EditedAt.Time.After(prevEditedAt.Time), "edited_at moves on every edit")
		prevEditedAt = got.EditedAt

		recipients := map[string]int{}
		for _, m := range out.Sent() {
			recipients[m.Mail.To[0].Address]++
			require.Contains(t, m.Mail.Subject, "Edited comment")
		}

		require.Equal(t, map[string]int{w.author.Email: i + 1, w.second.Email: i + 1}, recipients)

		for _, m := range out.Sent()[i*2:] {
			require.Contains(t, m.Mail.Text, body)
		}
	}

	ids := map[string]bool{}
	for _, m := range out.Sent() {
		ids[m.UniqueID] = true
	}

	require.Len(t, ids, 4)

	// the same body again changes nothing and tells nobody
	editedAt := stored().EditedAt
	require.NoError(t, s.EditComment(ctx, w.direct, comment.ID, "second edit"))
	require.Equal(t, editedAt, stored().EditedAt)
	require.Len(t, out.Sent(), 4)

	// an edit is not a new comment
	require.Equal(t, counter, commentsNumber(t, ctx, db, post.ID))
}

func TestEditComment_Refused(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	out := fakesender.New()
	s := posts.New(repo.New(db), out, nil)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	comment := testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)

	for _, tc := range []struct {
		name  string
		actor *core.User
		id    string
		body  string
		want  error
	}{
		{"another user", w.second, comment.ID, "new body", service.ErrNotFound},
		{"post's author", w.author, comment.ID, "new body", service.ErrNotFound},
		{"anonymous", nil, comment.ID, "new body", service.ErrNeedsLogin},
		{"missing id", w.direct, "00000000-0000-0000-0000-000000000000", "new body", service.ErrNotFound},
	} {
		require.ErrorIs(t, s.EditComment(ctx, tc.actor, tc.id, tc.body), tc.want, tc.name)
	}

	var ve *service.ValidationError
	require.ErrorAs(t, s.EditComment(ctx, w.direct, comment.ID, "x"), &ve)
	require.Equal(t, "body", ve.Field)

	got := testutil.Must(core.FindPostComment(ctx, db, comment.ID))(t)
	require.Equal(t, comment.Body, got.Body)
	require.False(t, got.EditedAt.Valid)
	require.Empty(t, out.Sent())
}

func TestEditComment_LostConnection(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	out := fakesender.New()
	s := posts.New(repo.New(db), out, nil)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	comment := testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)

	require.NoError(t, repo.New(db).DeleteConnectionsBetween(ctx, w.direct.ID, w.author.ID))
	require.ErrorIs(t, s.EditComment(ctx, w.direct, comment.ID, "new body"), service.ErrForbidden)

	got := testutil.Must(core.FindPostComment(ctx, db, comment.ID))(t)
	require.Equal(t, comment.Body, got.Body)
	require.False(t, got.EditedAt.Valid)
	require.Empty(t, out.Sent())
}
