package forms_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/stretchr/testify/require"
)

func TestNewCommentForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	direct, err := factory.User(ctx, db)
	require.NoError(t, err)
	stranger, err := factory.User(ctx, db)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, author.ID, direct.ID)
	require.NoError(t, err)

	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)
	topComment, err := factory.Comment(ctx, db, post.ID, author.ID)
	require.NoError(t, err)

	newForm := func(t *testing.T, u *core.User) *forms.NewCommentForm {
		t.Helper()
		f := forms.NewCommentFormNew(fakesender.New(), u, post.ID, mediaReplacer)
		cf, ok := f.(*forms.NewCommentForm)
		require.True(t, ok)
		cf.Input.Body = "A perfectly fine comment body"
		cf.Input.PostID = post.ID

		return cf
	}

	t.Run("the author can comment", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newForm(t, author)
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("a direct connection can comment", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newForm(t, direct)
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("a stranger cannot comment", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newForm(t, stranger)
		require.ErrorIs(t, form.Validate(c, db), ginhelpers.ErrForbidden)
	})

	t.Run("body too short", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newForm(t, author)
		form.Input.Body = "hi"
		require.Error(t, form.Validate(c, db))
		require.True(t, form.Errors.HasError("body"))
	})

	t.Run("replying to an existing comment on the post is allowed", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newForm(t, direct)
		form.Input.ReplyTo = topComment.ID
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("replying to a comment on another post is not found", func(t *testing.T) {
		t.Parallel()

		otherPost, err := factory.Post(ctx, db, author.ID)
		require.NoError(t, err)
		otherComment, err := factory.Comment(ctx, db, otherPost.ID, author.ID)
		require.NoError(t, err)

		c, _ := newCtx(t)
		form := newForm(t, direct)
		form.Input.ReplyTo = otherComment.ID
		require.ErrorIs(t, form.Validate(c, db), ginhelpers.ErrNotFound)
	})

	t.Run("replying to an unknown comment is not found", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newForm(t, direct)
		form.Input.ReplyTo = missingID
		require.ErrorIs(t, form.Validate(c, db), ginhelpers.ErrNotFound)
	})

	t.Run("commenting on an unknown post surfaces the lookup error", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newForm(t, direct)
		form.Input.PostID = missingID
		require.Error(t, form.Validate(c, db))
	})
}

func TestNewCommentForm_Save_TopLevel_NotifiesAuthorOnly(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	commenter, err := factory.User(ctx, db)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, author.ID, commenter.ID)
	require.NoError(t, err)

	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)

	sender := fakesender.New()
	f := forms.NewCommentFormNew(sender, commenter, post.ID, mediaReplacer)
	form, ok := f.(*forms.NewCommentForm)
	require.True(t, ok)
	form.Input.Body = "First comment on the post"
	form.Input.PostID = post.ID

	c, _ := newCtx(t)
	require.NoError(t, form.Validate(c, db))

	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	action(c, form)

	comments, err := factory.ListComments(ctx, db, post.ID)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	require.Equal(t, comments[0].ID, comments[0].TopCommentID)

	sent := sender.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "comment_notification", sent[0].EmailType)
	require.Equal(t, author.Email, sent[0].Mail.To[0].Address)
}

func TestNewCommentForm_Save_IncrementsPostStat(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	commenter, err := factory.User(ctx, db)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, author.ID, commenter.ID)
	require.NoError(t, err)
	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)

	saveComment := func(t *testing.T, u *core.User) {
		t.Helper()
		f := forms.NewCommentFormNew(fakesender.New(), u, post.ID, mediaReplacer)
		form, ok := f.(*forms.NewCommentForm)
		require.True(t, ok)
		form.Input.Body = "Another comment on the post"
		form.Input.PostID = post.ID

		c, _ := newCtx(t)
		require.NoError(t, form.Validate(c, db))
		action, err := form.Save(ctx, db)
		require.NoError(t, err)
		action(c, form)
	}

	// No stat row exists until the first comment is saved.
	_, err = factory.GetPostStat(ctx, db, post.ID)
	require.Error(t, err)

	saveComment(t, commenter)
	stat, err := factory.GetPostStat(ctx, db, post.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), stat.CommentsNumber)

	saveComment(t, author)
	stat, err = factory.GetPostStat(ctx, db, post.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), stat.CommentsNumber)
}

func TestNewCommentForm_Save_ReplyToVanishedComment(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)

	// Save is exercised directly, without Validate, the way the table asks
	// for it: a reply-to id that no longer resolves to a comment surfaces
	// the lookup error instead of panicking or inserting an orphan reply.
	f := forms.NewCommentFormNew(fakesender.New(), author, post.ID, mediaReplacer)
	form, ok := f.(*forms.NewCommentForm)
	require.True(t, ok)
	form.Input.Body = "A reply to nothing"
	form.Input.PostID = post.ID
	form.Input.ReplyTo = missingID

	_, err = form.Save(ctx, db)
	require.Error(t, err)
}

func TestNewCommentForm_Save_Reply_ThreadingAndParticipants(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	firstCommenter, err := factory.User(ctx, db)
	require.NoError(t, err)
	replier, err := factory.User(ctx, db)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, author.ID, firstCommenter.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, author.ID, replier.ID)
	require.NoError(t, err)

	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)
	topComment, err := factory.Comment(ctx, db, post.ID, firstCommenter.ID)
	require.NoError(t, err)

	sender := fakesender.New()
	f := forms.NewCommentFormNew(sender, replier, post.ID, mediaReplacer)
	form, ok := f.(*forms.NewCommentForm)
	require.True(t, ok)
	form.Input.Body = "A reply to the first comment"
	form.Input.PostID = post.ID
	form.Input.ReplyTo = topComment.ID

	c, _ := newCtx(t)
	require.NoError(t, form.Validate(c, db))

	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	action(c, form)

	comments, err := factory.ListComments(ctx, db, post.ID)
	require.NoError(t, err)
	require.Len(t, comments, 2)

	var reply *core.PostComment
	for _, cmt := range comments {
		if cmt.ID != topComment.ID {
			reply = cmt
		}
	}
	require.NotNil(t, reply)
	// The reply inherits the thread's top comment rather than starting a
	// new thread of its own.
	require.Equal(t, topComment.ID, reply.TopCommentID)
	require.Equal(t, topComment.ID, reply.ParentCommentID.String)

	sent := sender.Sent()
	require.Len(t, sent, 2)

	var gotAuthorMail, gotParticipantMail bool
	for _, s := range sent {
		require.Equal(t, "comment_notification", s.EmailType)
		switch s.Mail.To[0].Address {
		case author.Email:
			gotAuthorMail = true
		case firstCommenter.Email:
			gotParticipantMail = true
		default:
			t.Fatalf("unexpected recipient %q", s.Mail.To[0].Address)
		}
	}
	require.True(t, gotAuthorMail, "expected the post author to be notified")
	require.True(t, gotParticipantMail, "expected the first commenter to be notified as a participant")
}
