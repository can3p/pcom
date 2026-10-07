package mail_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

func TestNewPost(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &model.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Test Post"),
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliver(ctx, sender)(mail.NewPost(links.Site{}, testFrom, mediaReplacer, user, connection, post))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post", mailsToGolden(sent))
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
	mediaReplacer := func(in string) (bool, string) { return false, in }

	// Passing the same user as both author and connection should skip sending
	err := deliver(ctx, sender)(mail.NewPost(links.Site{}, testFrom, mediaReplacer, user, user, post))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostPrompt(t *testing.T) {
	t.Parallel()

	asker := &model.User{
		ID:       "user-1",
		Email:    "asker@example.test",
		Username: "alice",
	}
	recipient := &model.User{
		ID:       "user-2",
		Email:    "recipient@example.test",
		Username: "bob",
	}
	prompt := &model.PostPrompt{
		ID:          "prompt-1",
		AskerID:     asker.ID,
		RecipientID: recipient.ID,
		Message:     "What is your favorite hobby?",
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := deliver(ctx, sender)(mail.PostPrompt(links.Site{}, testFrom, asker, recipient, prompt))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_prompt", mailsToGolden(sent))
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

func TestPostPromptAnswer(t *testing.T) {
	t.Parallel()

	asker := &model.User{
		ID:       "user-1",
		Email:    "asker@example.test",
		Username: "alice",
	}
	responder := &model.User{
		ID:       "user-2",
		Email:    "responder@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("My Answer"),
		Body:    "Here is my answer to the prompt",
		UserID:  responder.ID,
	}
	prompt := &model.PostPrompt{
		ID:          "prompt-1",
		AskerID:     asker.ID,
		RecipientID: responder.ID,
		Message:     "What is your favorite hobby?",
		PostID:      new(post.ID),
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := deliver(ctx, sender)(mail.PostPromptAnswer(links.Site{}, testFrom, asker, responder, post, prompt))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_prompt_answer", mailsToGolden(sent))
}

func TestNewPost_NoSubject(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &model.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: nil, // No subject
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliver(ctx, sender)(mail.NewPost(links.Site{}, testFrom, mediaReplacer, user, connection, post))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_no_subject", mailsToGolden(sent))
}

func TestNewPost_WithLinkedURL(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &model.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Check out this article"),
		Body:    "This is a test post body",
		UserID:  user.ID,
		URLID:   new("url-1"),
	}
	// Post has a linked URL relation loaded
	setPostURL(post, &model.NormalizedURL{
		ID:  "url-1",
		URL: "https://example.com/article",
	})

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliver(ctx, sender)(mail.NewPost(links.Site{}, testFrom, mediaReplacer, user, connection, post))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_with_url", mailsToGolden(sent))
}

func TestNewPost_SubjectWithSpecialChars(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &model.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new(`Test <b>bold</b> and "quotes"`),
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliver(ctx, sender)(mail.NewPost(links.Site{}, testFrom, mediaReplacer, user, connection, post))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_special_chars", mailsToGolden(sent))
}

func TestNewPost_EmptyBody(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &model.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Test Post"),
		Body:    "", // Empty body
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliver(ctx, sender)(mail.NewPost(links.Site{}, testFrom, mediaReplacer, user, connection, post))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_empty_body", mailsToGolden(sent))
}
