package mail_test

import (
	"context"
	"errors"
	"testing"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// hostile carries both a tag and quotes, so it breaks out of text and of
// attribute values unless escaped.
const hostile = `"><b>x</b>`

const (
	rawTag     = `<b>x</b>`
	escapedTag = `&lt;b&gt;x&lt;/b&gt;`
	escapedQuo = `&#34;`
)

// mailers lists every notification that interpolates user content, with all
// user-controlled fields set to hostile markup.
func mailers() map[string]func(s *fakesender.Sender) error {
	ctx := context.Background()
	replacer := func(in string) (bool, string) { return false, in }

	alice := &core.User{ID: "user-1", Email: "a@example.test", Username: hostile}
	bob := &core.User{ID: "user-2", Email: "b@example.test", Username: hostile + "b"}
	post := &core.Post{ID: "post-1", Subject: null.StringFrom(hostile), Body: hostile, UserID: alice.ID, URLID: null.StringFrom("url-1")}
	setPostURL(post, &core.NormalizedURL{ID: "url-1", URL: "https://example.com/" + hostile})
	comment := &core.PostComment{ID: "comment-1", PostID: post.ID, UserID: alice.ID, Body: hostile}
	prompt := &core.PostPrompt{ID: "prompt-1", AskerID: bob.ID, RecipientID: alice.ID, Message: hostile}

	return map[string]func(s *fakesender.Sender) error{
		"NewPost": func(s *fakesender.Sender) error {
			return deliver(ctx, s)(mail.NewPost(replacer, alice, bob, post))
		},
		"PostCommentAuthor": func(s *fakesender.Sender) error {
			return deliverE(ctx, s)(mail.PostCommentAuthor(replacer, alice, bob, post, comment))
		},
		"PostCommentParticipants": func(s *fakesender.Sender) error {
			return deliverE(ctx, s)(mail.PostCommentParticipants(replacer, alice, bob, post, comment))
		},
		"PostPrompt": func(s *fakesender.Sender) error {
			return deliver(ctx, s)(mail.PostPrompt(alice, bob, prompt))
		},
		"PostPromptAnswer": func(s *fakesender.Sender) error {
			return deliver(ctx, s)(mail.PostPromptAnswer(bob, alice, post, prompt))
		},
	}
}

func TestMailers_EscapeUserContentInHTML(t *testing.T) {
	t.Parallel()

	for name, send := range mailers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			require.NoError(t, send(s))

			sent := s.Sent()
			require.Len(t, sent, 1)

			html := sent[0].Mail.Html
			require.NotContains(t, html, rawTag)
			require.NotContains(t, html, `"><`)
			require.Contains(t, html, escapedTag)
			require.Contains(t, html, escapedQuo)
			require.Contains(t, sent[0].Mail.Text, hostile)
		})
	}
}

func TestMailers_ReturnSendErrors(t *testing.T) {
	t.Parallel()

	sendErr := errors.New("database is gone")

	for name, send := range mailers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			s.FailWith(sendErr)

			require.ErrorIs(t, send(s), sendErr)
		})
	}
}

func TestSignupMailers_ReturnSendErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB

	tests := map[string]func(s *fakesender.Sender) error{
		"ConfirmSignup": func(s *fakesender.Sender) error {
			user := &core.User{ID: "user-1", Email: "u@example.test", EmailConfirmSeed: null.StringFrom("seed")}
			return mail.ConfirmSignup(ctx, nil, s, user)
		},
		"ConfirmWaitingList": func(s *fakesender.Sender) error {
			return mail.ConfirmWaitingList(ctx, db, s, testutil.Must(factory.SignupRequest(ctx, db))(t))
		},
	}

	for name, send := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			sendErr := errors.New("database is gone")
			s := fakesender.New()
			s.FailWith(sendErr)

			require.ErrorIs(t, send(s), sendErr)
		})
	}
}
