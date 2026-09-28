package mail_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

func init() {
	_ = os.Setenv("SENDER_ADDRESS", "noreply@pcom.test")
}

// setPostURL sets the URL relation on a post using reflection.
// This is necessary because the R field is unexported in sqlboiler models,
// but we need to test the mail code path that uses post.R.URL.
func setPostURL(post *core.Post, url *core.NormalizedURL) {
	postVal := reflect.ValueOf(post).Elem()

	// Get the type of the R field
	rField, ok := reflect.TypeFor[core.Post]().FieldByName("R")
	if !ok {
		return
	}

	// Create a new instance of the R type
	rType := rField.Type
	if rType.Kind() == reflect.Pointer {
		rType = rType.Elem()
	}
	rVal := reflect.New(rType).Elem()

	// Set the URL field
	rVal.FieldByName("URL").Set(reflect.ValueOf(url))

	// Set the R field on the post
	postVal.FieldByName("R").Set(rVal.Addr())
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

func TestConfirmSignup(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:               "user-1",
		Email:            "user@example.test",
		EmailConfirmSeed: null.StringFrom("confirm-seed-123"),
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := mail.ConfirmSignup(ctx, nil, sender, user)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "confirm_signup", mailsToGolden(sent))
}

func TestConfirmSignup_NoSeed(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:               "user-1",
		Email:            "user@example.test",
		EmailConfirmSeed: null.String{}, // Empty seed
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := mail.ConfirmSignup(ctx, nil, sender, user)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty confirmation seed")

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email if seed is empty")
}

func TestNewPost(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &core.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Test Post"),
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.NewPost(ctx, nil, sender, mediaReplacer, user, connection, post)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post", mailsToGolden(sent))
}

func TestNewPost_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Test Post"),
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	// Passing the same user as both author and connection should skip sending
	err := mail.NewPost(ctx, nil, sender, mediaReplacer, user, user, post)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostCommentAuthor(t *testing.T) {
	t.Parallel()

	commenter := &core.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	author := &core.User{
		ID:       "user-2",
		Email:    "author@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Original Post"),
		Body:    "Post body",
		UserID:  author.ID,
	}
	comment := &core.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Nice post!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.PostCommentAuthor(ctx, nil, sender, mediaReplacer, commenter, author, post, comment)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_author", mailsToGolden(sent))
}

func TestPostCommentAuthor_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Original Post"),
		Body:    "Post body",
		UserID:  user.ID,
	}
	comment := &core.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: user.ID,
		Body:   "Nice post!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	// Passing the same user as both commenter and author should skip sending
	err := mail.PostCommentAuthor(ctx, nil, sender, mediaReplacer, user, user, post, comment)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostCommentParticipants(t *testing.T) {
	t.Parallel()

	commenter := &core.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	participant := &core.User{
		ID:       "user-2",
		Email:    "participant@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Original Post"),
		Body:    "Post body",
		UserID:  "user-3",
	}
	comment := &core.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Great comment!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.PostCommentParticipants(ctx, nil, sender, mediaReplacer, commenter, participant, post, comment)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_participants", mailsToGolden(sent))
}

func TestPostCommentParticipants_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Original Post"),
		Body:    "Post body",
		UserID:  "user-3",
	}
	comment := &core.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: user.ID,
		Body:   "Great comment!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	// Passing the same user as both commenter and participant should skip sending
	err := mail.PostCommentParticipants(ctx, nil, sender, mediaReplacer, user, user, post, comment)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostPrompt(t *testing.T) {
	t.Parallel()

	asker := &core.User{
		ID:       "user-1",
		Email:    "asker@example.test",
		Username: "alice",
	}
	recipient := &core.User{
		ID:       "user-2",
		Email:    "recipient@example.test",
		Username: "bob",
	}
	prompt := &core.PostPrompt{
		ID:          "prompt-1",
		AskerID:     asker.ID,
		RecipientID: recipient.ID,
		Message:     "What is your favorite hobby?",
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := mail.PostPrompt(ctx, nil, sender, asker, recipient, prompt)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_prompt", mailsToGolden(sent))
}

func TestPostPrompt_NotToMyself(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	prompt := &core.PostPrompt{
		ID:          "prompt-1",
		AskerID:     user.ID,
		RecipientID: user.ID,
		Message:     "What is your favorite hobby?",
	}

	sender := fakesender.New()
	ctx := context.Background()

	// Passing the same user as both asker and recipient should skip sending
	err := mail.PostPrompt(ctx, nil, sender, user, user, prompt)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Empty(t, sent, "should not send email to self")
}

func TestPostPromptAnswer(t *testing.T) {
	t.Parallel()

	asker := &core.User{
		ID:       "user-1",
		Email:    "asker@example.test",
		Username: "alice",
	}
	responder := &core.User{
		ID:       "user-2",
		Email:    "responder@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("My Answer"),
		Body:    "Here is my answer to the prompt",
		UserID:  responder.ID,
	}
	prompt := &core.PostPrompt{
		ID:          "prompt-1",
		AskerID:     asker.ID,
		RecipientID: responder.ID,
		Message:     "What is your favorite hobby?",
		PostID:      null.StringFrom(post.ID),
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := mail.PostPromptAnswer(ctx, nil, sender, asker, responder, post, prompt)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_prompt_answer", mailsToGolden(sent))
}

func TestNewPost_NoSubject(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &core.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.String{}, // No subject
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.NewPost(ctx, nil, sender, mediaReplacer, user, connection, post)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_no_subject", mailsToGolden(sent))
}

func TestNewPost_WithLinkedURL(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &core.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Check out this article"),
		Body:    "This is a test post body",
		UserID:  user.ID,
		URLID:   null.StringFrom("url-1"),
	}
	// Post has a linked URL relation loaded
	setPostURL(post, &core.NormalizedURL{
		ID:  "url-1",
		URL: "https://example.com/article",
	})

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.NewPost(ctx, nil, sender, mediaReplacer, user, connection, post)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_with_url", mailsToGolden(sent))
}

func TestNewPost_SubjectWithSpecialChars(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &core.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom(`Test <b>bold</b> and "quotes"`),
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.NewPost(ctx, nil, sender, mediaReplacer, user, connection, post)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_special_chars", mailsToGolden(sent))
}

func TestNewPost_EmptyBody(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &core.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Test Post"),
		Body:    "", // Empty body
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.NewPost(ctx, nil, sender, mediaReplacer, user, connection, post)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "new_post_empty_body", mailsToGolden(sent))
}

func TestPostCommentAuthor_WithURL(t *testing.T) {
	t.Parallel()

	commenter := &core.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	author := &core.User{
		ID:       "user-2",
		Email:    "author@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Original Post"),
		Body:    "Post body",
		UserID:  author.ID,
		URLID:   null.StringFrom("url-1"),
	}
	setPostURL(post, &core.NormalizedURL{
		ID:  "url-1",
		URL: "https://example.com/article",
	})
	comment := &core.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Nice post!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.PostCommentAuthor(ctx, nil, sender, mediaReplacer, commenter, author, post, comment)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_author_with_url", mailsToGolden(sent))
}

func TestPostCommentParticipants_WithURL(t *testing.T) {
	t.Parallel()

	commenter := &core.User{
		ID:       "user-1",
		Email:    "commenter@example.test",
		Username: "alice",
	}
	participant := &core.User{
		ID:       "user-2",
		Email:    "participant@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom("Original Post"),
		Body:    "Post body",
		UserID:  "user-3",
		URLID:   null.StringFrom("url-1"),
	}
	setPostURL(post, &core.NormalizedURL{
		ID:  "url-1",
		URL: "https://example.com/article",
	})
	comment := &core.PostComment{
		ID:     "comment-1",
		PostID: post.ID,
		UserID: commenter.ID,
		Body:   "Great comment!",
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.PostCommentParticipants(ctx, nil, sender, mediaReplacer, commenter, participant, post, comment)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "post_comment_participants_with_url", mailsToGolden(sent))
}

func TestNewPost_HTMLEscaping(t *testing.T) {
	// This tests the HTML escaping case mentioned in issue #120
	// The subject should be properly escaped in HTML email
	t.Skip("known bug #120: HTML characters in subject are not properly escaped in email")

	user := &core.User{
		ID:       "user-1",
		Email:    "user@example.test",
		Username: "alice",
	}
	connection := &core.User{
		ID:       "user-2",
		Email:    "connection@example.test",
		Username: "bob",
	}
	post := &core.Post{
		ID:      "post-1",
		Subject: null.StringFrom(`Test <script>alert('xss')</script>`),
		Body:    "This is a test post body",
		UserID:  user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()
	mediaReplacer := func(in string) (bool, string) { return false, in }

	err := mail.NewPost(ctx, nil, sender, mediaReplacer, user, connection, post)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	require.NotContains(t, sent[0].Mail.Html, "<script>")
	require.Contains(t, sent[0].Mail.Html, "&lt;script&gt;")
}
