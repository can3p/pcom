package mail_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/stretchr/testify/require"
)

func keepMedia(in string) (bool, string) { return false, in }

func postUsers() (user, connection *model.User) {
	return &model.User{ID: "user-1", Email: "user@example.test", Username: "alice"},
		&model.User{ID: "user-2", Email: "connection@example.test", Username: "bob"}
}

func TestNewPost_MatchesSamples(t *testing.T) {
	t.Parallel()

	user, connection := postUsers()
	newPost := func(subject *string, body string) *model.Post {
		return &model.Post{ID: "post-1", Subject: subject, Body: body, UserID: user.ID}
	}
	withURL := newPost(new("Check out this article"), "This is a test post body")
	withURL.URLID = new("url-1")
	setPostURL(withURL, &model.NormalizedURL{ID: "url-1", URL: "https://example.com/article"})

	tests := []struct {
		sample string
		post   *model.Post
	}{
		{"new_post", newPost(new("Test Post"), "This is a test post body")},
		{"new_post_no_subject", newPost(nil, "This is a test post body")},
		{"new_post_with_url", withURL},
		{"new_post_special_chars", newPost(new(`Test <b>bold</b> and "quotes"`), "This is a test post body")},
		{"new_post_empty_body", newPost(new("Test Post"), "")},
	}

	for _, tt := range tests {
		t.Run(tt.sample, func(t *testing.T) {
			t.Parallel()

			got := mail.NewPost(mail.SampleSite, testFrom, keepMedia, user, connection, tt.post)
			require.Equal(t, sampleEnvelope(t, tt.sample), got)
		})
	}
}

func TestNewPost_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Test Post"),
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()

	// Passing the same user as both author and connection should skip sending
	err := deliver(ctx, sender)(mail.NewPost(links.Site{}, testFrom, keepMedia, user, user, post))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostPrompt_MatchesSample(t *testing.T) {
	t.Parallel()

	asker := &model.User{ID: "user-1", Email: "asker@example.test", Username: "alice"}
	recipient := &model.User{ID: "user-2", Email: "recipient@example.test", Username: "bob"}
	prompt := &model.PostPrompt{
		ID:          "prompt-1",
		AskerID:     asker.ID,
		RecipientID: recipient.ID,
		Message:     "What is your favorite hobby?",
	}

	got := mail.PostPrompt(mail.SampleSite, testFrom, asker, recipient, prompt)
	require.Equal(t, sampleEnvelope(t, "post_prompt"), got)
}

func TestPostPrompt_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	prompt := &model.PostPrompt{
		ID:          "prompt-1",
		AskerID:     user.ID,
		RecipientID: user.ID,
		Message:     "What is your favorite hobby?",
	}

	sender := fakesender.New()
	ctx := context.Background()

	// Passing the same user as both asker and recipient should skip sending
	err := deliver(ctx, sender)(mail.PostPrompt(links.Site{}, testFrom, user, user, prompt))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostPromptAnswer_MatchesSample(t *testing.T) {
	t.Parallel()

	asker := &model.User{ID: "user-1", Email: "asker@example.test", Username: "alice"}
	responder := &model.User{ID: "user-2", Email: "responder@example.test", Username: "bob"}
	post := &model.Post{
		ID:      "post-2",
		Subject: new("My Answer"),
		Body:    "Here is my answer to the prompt",
		UserID:  responder.ID,
	}
	prompt := &model.PostPrompt{
		ID:          "prompt-1",
		AskerID:     asker.ID,
		RecipientID: responder.ID,
		Message:     "What is your favorite hobby?",
		PostID:      new("post-1"),
	}

	got := mail.PostPromptAnswer(mail.SampleSite, testFrom, asker, responder, post, prompt)
	require.Equal(t, sampleEnvelope(t, "post_prompt_answer"), got)
}
