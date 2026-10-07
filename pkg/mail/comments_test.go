package mail_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

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
