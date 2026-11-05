package posts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// A draft made before the job runs is told to nobody.
func TestFanOutPublished_DraftEnqueuesNothing(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)

	post := publish(t, ctx, s, w, "withdrawn")
	_, err := s.Save(ctx, w.author, posts.SaveInput{PostID: post.ID, Subject: "withdrawn", Body: "a body for the post",
		Visibility: model.PostVisibilityPublic, Action: posts.ActionMakeDraft})
	require.NoError(t, err)

	require.NoError(t, s.FanOutPublished(ctx, posts.PublishedArgs{PostID: post.ID}))
	require.Empty(t, fakequeue.Of[posts.NewPostMailArgs](q))
}

// Whatever the visibility of the post, the direct connections of the author
// are told and nobody else.
func TestFanOutPublished_ToWhoMayRead(t *testing.T) {
	t.Parallel()

	for _, v := range []model.PostVisibility{model.PostVisibilityDirectOnly, model.PostVisibilitySecondDegree, model.PostVisibilityPublic} {
		t.Run(string(v), func(t *testing.T) {
			t.Parallel()

			db := testdb.New(t).DB
			ctx := context.Background()
			w := newWorld(t, ctx, db) // second is a second degree connection, stranger is unrelated
			q := fakequeue.New()
			s := svcWith(db, q)

			saved, err := s.Save(ctx, w.author, posts.SaveInput{Subject: "visible", Body: "a body for the post",
				Visibility: v, Action: posts.ActionPublish})
			require.NoError(t, err)

			require.NoError(t, s.FanOutPublished(ctx, posts.PublishedArgs{PostID: saved.Post.ID}))
			require.Equal(t, []posts.NewPostMailArgs{{PostID: saved.Post.ID, ConnectionID: w.direct.ID}}, fakequeue.Of[posts.NewPostMailArgs](q))
		})
	}
}

// The inserter drops a second post-published job of the same post.
func TestPublishedArgs_UniqueByPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	ins, err := jobs.NewInserter(db)
	require.NoError(t, err)

	count := func() int {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE kind = 'posts.published'`).Scan(&n))

		return n
	}

	require.NoError(t, ins.Insert(ctx, db, posts.PublishedArgs{PostID: "p1"}))
	require.NoError(t, ins.Insert(ctx, db, posts.PublishedArgs{PostID: "p1"}))
	require.Equal(t, 1, count())

	require.NoError(t, ins.Insert(ctx, db, posts.PublishedArgs{PostID: "p2"}))
	require.Equal(t, 2, count())
}

// Mails are rendered for who may read the post when they are sent: a
// connection removed after the enqueue is told nothing.
func TestRenderMails_ConnectionLostAfterEnqueue(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)
	store := repo.New(db)

	prompt := testutil.Must(factory.PostPrompt(ctx, db, w.direct.ID, w.author.ID))(t)
	saved, err := s.Save(ctx, w.author, posts.SaveInput{PromptID: prompt.ID, Subject: "the answer",
		Body: "a body for the post", Visibility: model.PostVisibilityDirectOnly, Action: posts.ActionPublish})
	require.NoError(t, err)
	require.NoError(t, s.FanOutPublished(ctx, posts.PublishedArgs{PostID: saved.Post.ID}))
	require.NoError(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: saved.Post.ID, Body: "a fine comment"}))
	runCommentFanOut(t, ctx, s, q)

	answer := fakequeue.Of[posts.PromptAnswerMailArgs](q)
	newPost := fakequeue.Of[posts.NewPostMailArgs](q)
	comment := fakequeue.Of[posts.CommentMailArgs](q)
	require.Len(t, answer, 1)
	require.Len(t, newPost, 1)
	require.Len(t, comment, 1)

	require.NoError(t, store.DeleteConnectionsBetween(ctx, w.author.ID, w.direct.ID))

	env, err := s.RenderPromptAnswer(ctx, answer[0])
	require.NoError(t, err)
	require.Nil(t, env)

	env, err = s.RenderNewPost(ctx, newPost[0])
	require.NoError(t, err)
	require.Nil(t, env)

	// the author's own mail about the comment stays: the author may read it
	env, err = s.RenderComment(ctx, posts.CommentMailArgs{CommentID: comment[0].CommentID, RecipientID: w.direct.ID})
	require.NoError(t, err)
	require.Nil(t, env)
}

// A new comment mails the post's author and the other participants, each
// with the wording of their mail.
func TestRenderComment_AuthorAndParticipant(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)

	other := testutil.Must(factory.User(ctx, db))(t)
	_, _, err := factory.Connect(ctx, db, w.author.ID, other.ID)
	require.NoError(t, err)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	testutil.Must(factory.Comment(ctx, db, post.ID, other.ID))(t)
	require.NoError(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: post.ID, Body: "a fine new comment"}))
	require.Empty(t, fakequeue.Of[posts.CommentMailArgs](q), "the request records the comment; the job picks the recipients")
	runCommentFanOut(t, ctx, s, q)

	args := fakequeue.Of[posts.CommentMailArgs](q)
	require.Len(t, args, 2)

	byRecipient := map[string]*mail.Envelope{}

	for _, a := range args {
		env, err := s.RenderComment(ctx, a)
		require.NoError(t, err)
		require.NotNil(t, env)

		byRecipient[env.Mail.To[0].Address] = env
	}

	require.Len(t, byRecipient, 2)

	author := byRecipient[w.author.Email]
	require.NotNil(t, author)
	require.Contains(t, author.Mail.Subject, "New comment in your post")
	require.Contains(t, author.Mail.Text, "a fine new comment")

	participant := byRecipient[other.Email]
	require.NotNil(t, participant)
	require.Contains(t, participant.Mail.Subject, "New comment in the post")
	require.Contains(t, participant.Mail.Text, "a fine new comment")
}

func publish(t *testing.T, ctx context.Context, s *posts.Service, w world, subject string) *model.Post {
	t.Helper()

	saved, err := s.Save(ctx, w.author, posts.SaveInput{Subject: subject, Body: "a body for the post",
		Visibility: model.PostVisibilityPublic, Action: posts.ActionPublish})
	require.NoError(t, err)

	return saved.Post
}

// Saving a published post enqueues one job and renders nothing; the job
// enqueues one mail per direct connection of the author, and running it
// again enqueues the same mails, which share their unique ids.
func TestPublish_EnqueuesOneFanOutJob(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)

	post := publish(t, ctx, s, w, "published")
	require.Len(t, q.Inserted(), 1)
	require.Equal(t, []posts.PublishedArgs{{PostID: post.ID}}, fakequeue.Of[posts.PublishedArgs](q))
	require.Equal(t, "posts.published", posts.PublishedArgs{}.Kind())

	require.NoError(t, s.FanOutPublished(ctx, posts.PublishedArgs{PostID: post.ID}))
	require.Equal(t, []posts.NewPostMailArgs{{PostID: post.ID, ConnectionID: w.direct.ID}}, fakequeue.Of[posts.NewPostMailArgs](q))

	require.NoError(t, s.FanOutPublished(ctx, posts.PublishedArgs{PostID: post.ID}))

	both := fakequeue.Of[posts.NewPostMailArgs](q)
	require.Len(t, both, 2)

	// a re-run enqueues the same mails, whose unique id is the post and the
	// connection, so the sent mail record drops the repeat
	for _, a := range both {
		env, err := s.RenderNewPost(ctx, a)
		require.NoError(t, err)
		require.Equal(t, post.ID+w.direct.ID, env.UniqueID)
		require.Equal(t, "post_notification", env.Type)
		require.Equal(t, w.direct.Email, env.Mail.To[0].Address)
	}
}

// The mail is built when the job runs, so it shows the post as it was
// edited after the save that enqueued it.
func TestNewPostMail_ShowsPostEditedAfterEnqueue(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)

	post := publish(t, ctx, s, w, "first subject")

	_, err := s.Save(ctx, w.author, posts.SaveInput{PostID: post.ID, Subject: "edited subject", Body: "an edited body for the post",
		Visibility: model.PostVisibilityPublic, Action: posts.ActionPublish})
	require.NoError(t, err)

	sent := newMailbox(t, ctx, s, q).Sent()
	require.Len(t, sent, 1)
	require.Contains(t, sent[0].Mail.Subject, "edited subject")
	require.Contains(t, sent[0].Mail.Html, "an edited body")
	require.NotContains(t, sent[0].Mail.Subject, "first subject")
}

func TestFanOutPublished_PostGoneEnqueuesNothing(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	q := fakequeue.New()
	s := svcWith(db, q)

	require.NoError(t, s.FanOutPublished(ctx, posts.PublishedArgs{PostID: "00000000-0000-0000-0000-000000000000"}))
	require.Empty(t, q.Inserted())
}

func TestRenderMails_RowsGoneSendNothing(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	s := svcWith(db, nil)
	missing := "00000000-0000-0000-0000-000000000000"

	for name, render := range map[string]func() (any, error){
		"prompt": func() (any, error) {
			return s.RenderPrompt(ctx, posts.PromptMailArgs{PromptID: missing, RecipientID: missing})
		},
		"prompt answer": func() (any, error) { return s.RenderPromptAnswer(ctx, posts.PromptAnswerMailArgs{PostID: missing}) },
		"new post": func() (any, error) {
			return s.RenderNewPost(ctx, posts.NewPostMailArgs{PostID: missing, ConnectionID: missing})
		},
		"comment": func() (any, error) {
			return s.RenderComment(ctx, posts.CommentMailArgs{CommentID: missing, RecipientID: missing})
		},
	} {
		env, err := render()
		require.NoError(t, err, name)
		require.Nil(t, env, name)
	}
}

func TestSendPrompt_EnqueuesTheMail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)

	require.NoError(t, s.SendPrompt(ctx, w.author, w.direct, "Tell us about your week!"))

	args := fakequeue.Of[posts.PromptMailArgs](q)
	require.Len(t, args, 1)
	require.Equal(t, w.direct.ID, args[0].RecipientID)

	sent := newMailbox(t, ctx, s, q).Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "post_prompt", sent[0].Type)
	require.Equal(t, w.direct.Email, sent[0].Mail.To[0].Address)
	require.Equal(t, "New prompt from "+w.author.Username, sent[0].Mail.Subject)
	require.Contains(t, sent[0].Mail.Text, "Tell us about your week!")
}

func TestPromptAnswer_EnqueuesTheMail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)

	prompt := testutil.Must(factory.PostPrompt(ctx, db, w.direct.ID, w.author.ID))(t)
	_, err := s.Save(ctx, w.author, posts.SaveInput{PromptID: prompt.ID, Subject: "the answer",
		Body: "a body for the post", Visibility: model.PostVisibilityPublic, Action: posts.ActionPublish})
	require.NoError(t, err)
	require.Len(t, fakequeue.Of[posts.PromptAnswerMailArgs](q), 1)

	sent := newMailbox(t, ctx, s, q).Sent()
	require.Len(t, sent, 2, "the prompt answer, then the new post mail of the same connection")

	var answer = sent[0]
	require.Equal(t, "post_prompt_answer", answer.Type)
	require.Equal(t, w.direct.Email, answer.Mail.To[0].Address)
	require.Equal(t, "Response to your prompt from "+w.author.Username, answer.Mail.Subject)
	require.Contains(t, answer.Mail.Text, "the answer")
}

// runCommentFanOut asserts that one comment was posted, not edited, and
// runs its fan-out job, as the worker would.
func runCommentFanOut(t *testing.T, ctx context.Context, s *posts.Service, q *fakequeue.Queue) {
	t.Helper()

	posted := fakequeue.Of[posts.CommentPostedArgs](q)
	require.Len(t, posted, 1)
	require.False(t, posted[0].Edited)
	require.Equal(t, "posts.comment_posted", posted[0].Kind())
	require.NoError(t, s.FanOutComment(ctx, posted[0]))
}

// A comment deleted before its fan-out runs tells nobody.
func TestFanOutComment_CommentGoneEnqueuesNothing(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	q := fakequeue.New()
	s := svcWith(db, q)

	require.NoError(t, s.FanOutComment(ctx, posts.CommentPostedArgs{CommentID: "00000000-0000-0000-0000-000000000000"}))
	require.Empty(t, q.Inserted())
}

// The fan-out enqueues a comment mail for the post's author and for the
// participants who may still read the comments, never for the comment's own
// author.
func TestFanOutComment_EnqueuesWhoMayRead(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)
	store := repo.New(db)

	stays := testutil.Must(factory.User(ctx, db))(t)
	leaves := testutil.Must(factory.User(ctx, db))(t)

	for _, u := range []*model.User{stays, leaves} {
		_, _, err := factory.Connect(ctx, db, w.author.ID, u.ID)
		require.NoError(t, err)
	}

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	testutil.Must(factory.Comment(ctx, db, post.ID, stays.ID))(t)
	testutil.Must(factory.Comment(ctx, db, post.ID, leaves.ID))(t)
	testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)
	require.NoError(t, store.DeleteConnectionsBetween(ctx, w.author.ID, leaves.ID))

	require.NoError(t, s.AddComment(ctx, w.direct, posts.CommentInput{PostID: post.ID, Body: "another one"}))
	runCommentFanOut(t, ctx, s, q)

	var recipients []string
	for _, a := range fakequeue.Of[posts.CommentMailArgs](q) {
		recipients = append(recipients, a.RecipientID)
	}

	require.ElementsMatch(t, []string{w.author.ID, stays.ID}, recipients)
}

// The post's author commenting on their own post tells the participants, not
// the author.
func TestFanOutComment_AuthorsOwnCommentSkipsAuthor(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	q := fakequeue.New()
	s := svcWith(db, q)

	post := testutil.Must(factory.Post(ctx, db, w.author.ID, factory.Published()))(t)
	testutil.Must(factory.Comment(ctx, db, post.ID, w.direct.ID))(t)

	require.NoError(t, s.AddComment(ctx, w.author, posts.CommentInput{PostID: post.ID, Body: "thanks"}))
	runCommentFanOut(t, ctx, s, q)

	args := fakequeue.Of[posts.CommentMailArgs](q)
	require.Len(t, args, 1)
	require.Equal(t, w.direct.ID, args[0].RecipientID)
}
