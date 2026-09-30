package forms_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func connect(t *testing.T, ctx context.Context, db *sqlx.DB, aID, bID string) {
	t.Helper()
	_, _, err := factory.Connect(ctx, db, aID, bID)
	require.NoError(t, err)
}

func newCommentForm(t *testing.T, db *sqlx.DB, sender *fakesender.Sender, u *core.User, postID, replyTo string) *forms.NewCommentForm {
	t.Helper()
	form, ok := forms.NewCommentFormNew(postsService(db, sender), u, postID).(*forms.NewCommentForm)
	require.True(t, ok)
	form.Input.Body = "A perfectly fine comment body"
	form.Input.PostID = postID
	form.Input.ReplyTo = replyTo

	return form
}

// saveComment validates and saves a comment and runs the returned action.
func saveComment(t *testing.T, ctx context.Context, db *sqlx.DB, form *forms.NewCommentForm) {
	t.Helper()
	c, _ := newCtx(t)
	require.NoError(t, form.Validate(c))
	action := testutil.Must(form.Save(ctx))(t)
	action(c, form)
}

func TestNewCommentForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	stranger := testutil.Must(factory.User(ctx, db))(t)
	connect(t, ctx, db, author.ID, direct.ID)

	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	topComment := testutil.Must(factory.Comment(ctx, db, post.ID, author.ID))(t)
	otherPost := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	otherComment := testutil.Must(factory.Comment(ctx, db, otherPost.ID, author.ID))(t)

	for _, tc := range []struct {
		name    string
		user    *core.User
		postID  string
		replyTo string
		body    string
		wantErr error // nil: any error when wantAny is set
		wantAny bool
	}{
		{name: "the author can comment", user: author},
		{name: "a direct connection can comment", user: direct},
		{name: "a stranger cannot comment", user: stranger, wantErr: service.ErrForbidden},
		{name: "body too short", user: author, body: "hi", wantAny: true},
		{name: "reply to a comment on the post", user: direct, replyTo: topComment.ID},
		{name: "reply to a comment on another post", user: direct, replyTo: otherComment.ID, wantErr: service.ErrNotFound},
		{name: "reply to an unknown comment", user: direct, replyTo: missingID, wantErr: service.ErrNotFound},
		{name: "unknown post", user: direct, postID: missingID, wantAny: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			form := newCommentForm(t, db, fakesender.New(), tc.user, post.ID, tc.replyTo)
			if tc.postID != "" {
				form.Input.PostID = tc.postID
			}
			if tc.body != "" {
				form.Input.Body = tc.body
			}

			c, _ := newCtx(t)
			err := form.Validate(c)

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantAny:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
			if tc.body != "" {
				require.True(t, form.Errors.HasError("body"))
			}
		})
	}
}

func TestNewCommentForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("a top-level comment starts a thread and notifies the author only", func(t *testing.T) {
		t.Parallel()

		author := testutil.Must(factory.User(ctx, db))(t)
		commenter := testutil.Must(factory.User(ctx, db))(t)
		connect(t, ctx, db, author.ID, commenter.ID)
		post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

		sender := fakesender.New()
		saveComment(t, ctx, db, newCommentForm(t, db, sender, commenter, post.ID, ""))

		comments := testutil.Must(factory.ListComments(ctx, db, post.ID))(t)
		require.Len(t, comments, 1)
		require.Equal(t, comments[0].ID, comments[0].TopCommentID)

		sent := sender.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, "comment_notification", sent[0].EmailType)
		require.Equal(t, author.Email, sent[0].Mail.To[0].Address)
	})

	t.Run("each comment increments the post's counter", func(t *testing.T) {
		t.Parallel()

		author := testutil.Must(factory.User(ctx, db))(t)
		commenter := testutil.Must(factory.User(ctx, db))(t)
		connect(t, ctx, db, author.ID, commenter.ID)
		post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

		_, err := factory.GetPostStat(ctx, db, post.ID)
		require.Error(t, err, "no stat row before the first comment")

		for i, u := range []*core.User{commenter, author} {
			saveComment(t, ctx, db, newCommentForm(t, db, fakesender.New(), u, post.ID, ""))
			stat := testutil.Must(factory.GetPostStat(ctx, db, post.ID))(t)
			require.Equal(t, int64(i+1), stat.CommentsNumber)
		}
	})

	t.Run("a reply to a vanished comment fails without inserting", func(t *testing.T) {
		t.Parallel()

		author := testutil.Must(factory.User(ctx, db))(t)
		post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

		// Save without Validate, as a race with a deleted comment would.
		_, err := newCommentForm(t, db, fakesender.New(), author, post.ID, missingID).Save(ctx)
		require.Error(t, err)
		require.Empty(t, testutil.Must(factory.ListComments(ctx, db, post.ID))(t))
	})

	t.Run("a reply joins the thread and notifies the author and participants", func(t *testing.T) {
		t.Parallel()

		author := testutil.Must(factory.User(ctx, db))(t)
		firstCommenter := testutil.Must(factory.User(ctx, db))(t)
		replier := testutil.Must(factory.User(ctx, db))(t)
		connect(t, ctx, db, author.ID, firstCommenter.ID)
		connect(t, ctx, db, author.ID, replier.ID)
		post := testutil.Must(factory.Post(ctx, db, author.ID))(t)
		topComment := testutil.Must(factory.Comment(ctx, db, post.ID, firstCommenter.ID))(t)

		sender := fakesender.New()
		saveComment(t, ctx, db, newCommentForm(t, db, sender, replier, post.ID, topComment.ID))

		comments := testutil.Must(factory.ListComments(ctx, db, post.ID))(t)
		require.Len(t, comments, 2)
		for _, cmt := range comments {
			if cmt.ID != topComment.ID {
				require.Equal(t, topComment.ID, cmt.TopCommentID)
				require.Equal(t, topComment.ID, cmt.ParentCommentID.String)
			}
		}

		var recipients []string
		for _, s := range sender.Sent() {
			require.Equal(t, "comment_notification", s.EmailType)
			recipients = append(recipients, s.Mail.To[0].Address)
		}
		require.ElementsMatch(t, []string{author.Email, firstCommenter.Email}, recipients)
	})
}
