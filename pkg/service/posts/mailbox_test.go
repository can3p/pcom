package posts_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/stretchr/testify/require"
)

// svcWith is the posts service over db that enqueues its jobs on q.
func svcWith(db *sql.DB, q *fakequeue.Queue) *posts.Service {
	return posts.New(repo.New(db), nil, posts.WithJobs(q))
}

// mailbox plays the workers over the jobs a service enqueued on q: Sent runs
// the jobs recorded since the last call, in order, the way the runner does
// (the post-published job enqueues its mails, which run in turn) and
// returns every mail built so far.
type mailbox struct {
	t    *testing.T
	ctx  context.Context
	svc  *posts.Service
	q    *fakequeue.Queue
	next int
	sent []*mail.Envelope
}

func newMailbox(t *testing.T, ctx context.Context, svc *posts.Service, q *fakequeue.Queue) *mailbox {
	t.Helper()

	return &mailbox{t: t, ctx: ctx, svc: svc, q: q}
}

// Sent returns the mails of every job run so far.
func (m *mailbox) Sent() []*mail.Envelope {
	m.t.Helper()

	for m.next < len(m.q.Inserted()) {
		args := m.q.Inserted()[m.next]
		m.next++

		var (
			env *mail.Envelope
			err error
		)

		switch a := args.(type) {
		case posts.PublishedArgs:
			require.NoError(m.t, m.svc.FanOutPublished(m.ctx, a))
		case posts.PromptMailArgs:
			env, err = m.svc.RenderPrompt(m.ctx, a)
		case posts.PromptAnswerMailArgs:
			env, err = m.svc.RenderPromptAnswer(m.ctx, a)
		case posts.NewPostMailArgs:
			env, err = m.svc.RenderNewPost(m.ctx, a)
		case posts.CommentMailArgs:
			env, err = m.svc.RenderComment(m.ctx, a)
		default:
			m.t.Fatalf("unexpected job %T", args)
		}

		require.NoError(m.t, err)

		if env != nil {
			m.sent = append(m.sent, env)
		}
	}

	return m.sent
}
