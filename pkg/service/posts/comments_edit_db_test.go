package posts_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// commentsNumber is the comment counter of a post, zero without a stat row.
func commentsNumber(t *testing.T, ctx context.Context, db *sql.DB, postID string) int64 {
	t.Helper()

	stat, err := factory.GetPostStat(ctx, db, postID)
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
	q := fakequeue.New()
	s := svcWith(db, q)
	out := newMailbox(t, ctx, s, q)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	comment := testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)
	// another direct connection has commented on the post before, so it
	// follows the discussion
	other := testutil.Must(factory.User(ctx, db))(t)
	_, _, err := factory.Connect(ctx, db, w.author.ID, other.ID)
	require.NoError(t, err)
	testutil.Must(factory.Comment(ctx, db, post.ID, other.ID))(t)

	stored := func() *model.PostComment {
		return testutil.Must(repo.New(db).CommentByID(ctx, comment.ID))(t)
	}

	// the comment has never been edited, the same body with spaces around changes nothing
	counter := commentsNumber(t, ctx, db, post.ID)
	require.NoError(t, s.EditComment(ctx, w.direct, comment.ID, "  "+comment.Body+" \n"))
	require.Nil(t, stored().EditedAt)
	require.Empty(t, out.Sent())

	// two successive edits mail once per recipient each, the queue dedups on the unique id
	var prevEditedAt = stored().EditedAt

	for i, body := range []string{"first edit", "second edit"} {
		enqueued := len(q.Inserted())
		require.NoError(t, s.EditComment(ctx, w.direct, comment.ID, "  "+body+"\n"))

		got := stored()
		require.Equal(t, body, got.Body)
		require.NotNil(t, got.EditedAt)
		require.True(t, lo.FromPtr(got.EditedAt).After(lo.FromPtr(prevEditedAt)), "edited_at moves on every edit")
		prevEditedAt = got.EditedAt

		// one job per recipient, each run before the next edit
		require.Len(t, q.Inserted()[enqueued:], 2)

		recipients := map[string]int{}
		for _, m := range out.Sent() {
			recipients[m.Mail.To[0].Address]++
			require.Contains(t, m.Mail.Subject, "Edited comment")
		}

		require.Equal(t, map[string]int{w.author.Email: i + 1, other.Email: i + 1}, recipients)

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

	// two edits before any job runs: every job renders the comment as it is
	// then, so each recipient gets one mail (the unique id is the same) with
	// the last edit
	q2 := fakequeue.New()
	s2 := svcWith(db, q2)
	late := newMailbox(t, ctx, s2, q2)

	require.NoError(t, s2.EditComment(ctx, w.direct, comment.ID, "third edit"))
	require.NoError(t, s2.EditComment(ctx, w.direct, comment.ID, "fourth edit"))
	require.Len(t, q2.Inserted(), 4)

	lateIDs := map[string]bool{}

	for _, m := range late.Sent() {
		require.Contains(t, m.Mail.Text, "fourth edit")
		require.NotContains(t, m.Mail.Text, "third edit")

		lateIDs[m.Mail.To[0].Address+m.UniqueID] = true
	}

	require.Len(t, late.Sent(), 4, "four jobs, each rendering the last edit")
	require.Len(t, lateIDs, 2, "one mail per recipient once the sent record drops the repeat")

	// an edit is not a new comment
	require.Equal(t, counter, commentsNumber(t, ctx, db, post.ID))
}

func TestEditComment_Refused(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)
	out := newMailbox(t, ctx, s, q)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	comment := testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)

	for _, tc := range []struct {
		name  string
		actor *model.User
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

	got := testutil.Must(repo.New(db).CommentByID(ctx, comment.ID))(t)
	require.Equal(t, comment.Body, got.Body)
	require.Nil(t, got.EditedAt)
	require.Empty(t, out.Sent())
}

func TestEditComment_LostConnection(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)
	out := newMailbox(t, ctx, s, q)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	comment := testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)

	require.NoError(t, repo.New(db).DeleteConnectionsBetween(ctx, w.direct.ID, w.author.ID))
	require.ErrorIs(t, s.EditComment(ctx, w.direct, comment.ID, "new body"), service.ErrForbidden)

	got := testutil.Must(repo.New(db).CommentByID(ctx, comment.ID))(t)
	require.Equal(t, comment.Body, got.Body)
	require.Nil(t, got.EditedAt)
	require.Empty(t, out.Sent())
}

// A mail that cannot be queued fails the change it reports, and the
// transaction rolls the change back.
func TestSaveAndComment_SenderFailureRollsBack(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)
	sendErr := errors.New("queue is gone")
	q.FailWith(sendErr)

	_, err := s.Save(ctx, w.author, posts.SaveInput{
		Subject: "published", Body: "a body for the post", Visibility: model.PostVisibilityPublic, Action: posts.ActionPublish,
	})
	require.ErrorIs(t, err, sendErr)
	require.Empty(t, testutil.Must(factory.ListPosts(ctx, db, w.author.ID))(t))

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	require.ErrorIs(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: post.ID, Body: "a fine comment"}), sendErr)
	require.Empty(t, testutil.Must(factory.ListComments(ctx, db, post.ID))(t))
	require.Zero(t, commentsNumber(t, ctx, db, post.ID))
}
