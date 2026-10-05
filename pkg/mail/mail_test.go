package mail_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

const testFrom = "noreply@pcom.test"

// setPostURL sets the URL relation on a post, as if it had been loaded.
func setPostURL(post *model.Post, url *model.NormalizedURL) {
	post.URL = url
}

// deliver queues out on s, the way the posts service does, so the tests read
// what a recipient would get. A nil out sends nothing.
func deliver(ctx context.Context, s *fakesender.Sender) func(*mail.Outgoing) error {
	return func(out *mail.Outgoing) error {
		if out == nil {
			return nil
		}

		return s.Send(ctx, nil, out.UniqueID, out.Type, out.Mail)
	}
}

// deliverE is deliver for the formatters that can fail.
func deliverE(ctx context.Context, s *fakesender.Sender) func(*mail.Outgoing, error) error {
	return func(out *mail.Outgoing, err error) error {
		if err != nil {
			return err
		}

		return deliver(ctx, s)(out)
	}
}

// mailsToGolden serializes fakesender recordings to a human-readable format for golden testing
func mailsToGolden(sent []fakesender.Recorded) []byte {
	var buf bytes.Buffer
	for i, rec := range sent {
		if i > 0 {
			buf.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&buf, "EmailType: %s\n", rec.EmailType)
		fmt.Fprintf(&buf, "UniqueID: %s\n", rec.UniqueID)
		fmt.Fprintf(&buf, "From: %s <%s>\n", rec.Mail.From.Name, rec.Mail.From.Address)
		if len(rec.Mail.To) > 0 {
			fmt.Fprintf(&buf, "To: %s\n", rec.Mail.To[0].Address)
		}
		fmt.Fprintf(&buf, "Subject: %s\n", rec.Mail.Subject)
		fmt.Fprintf(&buf, "\n--- TEXT ---\n%s\n", rec.Mail.Text)
		fmt.Fprintf(&buf, "\n--- HTML ---\n%s\n", rec.Mail.Html)
	}
	return buf.Bytes()
}

// send queues an envelope the way a service does, through the sender with no database.
func send(ctx context.Context, s *fakesender.Sender, e *mail.Envelope) error {
	return s.Send(ctx, nil, e.UniqueID, e.Type, e.Mail)
}

func TestConfirmSignup(t *testing.T) {
	t.Parallel()

	sender := fakesender.New()

	require.NoError(t, send(context.Background(), sender, mail.ConfirmSignup(testFrom, "attempt-1", "user@example.test", "123456", 15*time.Minute)))

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "confirm_signup", mailsToGolden(sent))
}

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

func TestPostCommentAuthor(t *testing.T) {
	t.Parallel()

	commenter := &model.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	author := &model.User{
		ID:       "user-2",
		Email:    "author@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Original Post"),
		Body:    "Post body",
		UserID:  author.ID,
	}
	comment := &model.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Nice post!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliverE(ctx, sender)(mail.PostCommentAuthor(links.Site{}, testFrom, mediaReplacer, commenter, author, post, comment, false))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_author", mailsToGolden(sent))
}

func TestPostCommentAuthor_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Original Post"),
		Body:    "Post body",
		UserID:  user.ID,
	}
	comment := &model.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: user.ID,
		Body:   "Nice post!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	// Passing the same user as both commenter and author should skip sending
	err := deliverE(ctx, sender)(mail.PostCommentAuthor(links.Site{}, testFrom, mediaReplacer, user, user, post, comment, false))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostCommentParticipants(t *testing.T) {
	t.Parallel()

	commenter := &model.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	participant := &model.User{
		ID:       "user-2",
		Email:    "participant@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Original Post"),
		Body:    "Post body",
		UserID:  "user-3",
	}
	comment := &model.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Great comment!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliverE(ctx, sender)(mail.PostCommentParticipants(links.Site{}, testFrom, mediaReplacer, commenter, participant, post, comment, false))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_participants", mailsToGolden(sent))
}

func TestPostCommentParticipants_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &model.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Original Post"),
		Body:    "Post body",
		UserID:  "user-3",
	}
	comment := &model.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: user.ID,
		Body:   "Great comment!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	// Passing the same user as both commenter and participant should skip sending
	err := deliverE(ctx, sender)(mail.PostCommentParticipants(links.Site{}, testFrom, mediaReplacer, user, user, post, comment, false))
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

func TestPostCommentAuthor_WithURL(t *testing.T) {
	t.Parallel()

	commenter := &model.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	author := &model.User{
		ID:       "user-2",
		Email:    "author@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Original Post"),
		Body:    "Post body",
		UserID:  author.ID,
		URLID:   new("url-1"),
	}
	setPostURL(post, &model.NormalizedURL{
		ID:  "url-1",
		URL: "https://example.com/article",
	})
	comment := &model.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Nice post!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliverE(ctx, sender)(mail.PostCommentAuthor(links.Site{}, testFrom, mediaReplacer, commenter, author, post, comment, false))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_author_with_url", mailsToGolden(sent))
}

func TestPostCommentParticipants_WithURL(t *testing.T) {
	t.Parallel()

	commenter := &model.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	participant := &model.User{
		ID:       "user-2",
		Email:    "participant@example.test",
		Username: "bob",
	}
	post := &model.Post{
		ID:      "post-1",
		Subject: new("Original Post"),
		Body:    "Post body",
		UserID:  "user-3",
		URLID:   new("url-1"),
	}
	setPostURL(post, &model.NormalizedURL{
		ID:  "url-1",
		URL: "https://example.com/article",
	})
	comment := &model.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Great comment!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := deliverE(ctx, sender)(mail.PostCommentParticipants(links.Site{}, testFrom, mediaReplacer, commenter, participant, post, comment, false))
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_participants_with_url", mailsToGolden(sent))
}

func TestPostCommentEdited(t *testing.T) {
	t.Parallel()

	commenter := &model.User{ID: "user-1", Email: "commenter@example.test", Username: "alice"}
	recipient := &model.User{ID: "user-2", Email: "recipient@example.test", Username: "bob"}
	post := &model.Post{ID: "post-1", Subject: new("Original Post"), Body: "Post body", UserID: recipient.ID}
	comment := &model.PostComment{
		ID:       "comment-1",
		PostID:   post.ID,
		UserID:   commenter.ID,
		Body:     "Nice post, edited!",
		EditedAt: new(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)),
	}
	mediaReplacer := func(in string) (bool, string) { return false, in }

	for name, format := range map[string]func() (*mail.Outgoing, error){
		"post_comment_author_edited": func() (*mail.Outgoing, error) {
			return mail.PostCommentAuthor(links.Site{}, testFrom, mediaReplacer, commenter, recipient, post, comment, true)
		},
		"post_comment_participants_edited": func() (*mail.Outgoing, error) {
			return mail.PostCommentParticipants(links.Site{}, testFrom, mediaReplacer, commenter, recipient, post, comment, true)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			sender := fakesender.New()
			require.NoError(t, deliverE(context.Background(), sender)(format()))

			sent := sender.Sent()
			require.Len(t, sent, 1)
			require.Contains(t, sent[0].UniqueID, "2026-09-30T12:00:00Z")

			golden.Assert(t, name, mailsToGolden(sent))
		})
	}
}

func TestLoginCode(t *testing.T) {
	t.Parallel()

	sender := fakesender.New()
	require.NoError(t, send(context.Background(), sender, mail.LoginCode(testFrom, "attempt-1", "user@example.test", "123456", 15*time.Minute)))

	golden.Assert(t, "login_code", mailsToGolden(sender.Sent()))
}
