package posts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

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

	first, err := s.RenderNewPost(ctx, both[0])
	require.NoError(t, err)
	second, err := s.RenderNewPost(ctx, both[1])
	require.NoError(t, err)
	require.Equal(t, first.UniqueID, second.UniqueID, "a re-run sends nothing twice")
	require.Equal(t, "post_notification", first.Type)
	require.Equal(t, w.direct.Email, first.Mail.To[0].Address)
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
	require.Contains(t, answer.Mail.Text, "the answer")
}
