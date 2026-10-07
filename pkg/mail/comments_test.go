package mail_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/stretchr/testify/require"
)

type formatComment func(commenter, recipient *model.User, post *model.Post, comment *model.PostComment, edited bool) (*mail.Envelope, error)

func formatAuthor(commenter, recipient *model.User, post *model.Post, comment *model.PostComment, edited bool) (*mail.Envelope, error) {
	mediaReplacer := func(in string) (bool, string) { return false, in }

	return mail.PostCommentAuthor(mail.SampleSite, testFrom, mediaReplacer, commenter, recipient, post, comment, edited)
}

func formatParticipants(commenter, recipient *model.User, post *model.Post, comment *model.PostComment, edited bool) (*mail.Envelope, error) {
	mediaReplacer := func(in string) (bool, string) { return false, in }

	return mail.PostCommentParticipants(mail.SampleSite, testFrom, mediaReplacer, commenter, recipient, post, comment, edited)
}

// The comment mails are built from models; each case builds the models of
// the sample it names and expects the sample's rendering, so a wrong mapping
// from models to the input fails.
func TestPostCommentMails_MatchSamples(t *testing.T) {
	t.Parallel()

	commenter := &model.User{ID: "user-1", Email: "commenter@example.test", Username: "alice"}
	newPost := func() *model.Post {
		return &model.Post{ID: "post-1", Subject: new("Original Post"), Body: "Post body", UserID: "user-3"}
	}
	withURL := newPost()
	withURL.URLID = new("url-1")
	setPostURL(withURL, &model.NormalizedURL{ID: "url-1", URL: "https://example.com/article"})

	for _, tc := range []struct {
		name      string
		recipient string
		post      *model.Post
		body      string
		edited    bool
		format    formatComment
	}{
		{"post_comment_author", "author@example.test", newPost(), "Nice post!", false, formatAuthor},
		{"post_comment_author_edited", "recipient@example.test", newPost(), "Nice post, edited!", true, formatAuthor},
		{"post_comment_author_with_url", "author@example.test", withURL, "Nice post!", false, formatAuthor},
		{"post_comment_participants", "participant@example.test", newPost(), "Great comment!", false, formatParticipants},
		{"post_comment_participants_edited", "recipient@example.test", newPost(), "Nice post, edited!", true, formatParticipants},
		{"post_comment_participants_with_url", "participant@example.test", withURL, "Great comment!", false, formatParticipants},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			comment := &model.PostComment{ID: "comment-1", PostID: tc.post.ID, UserID: commenter.ID, Body: tc.body}
			if tc.edited {
				comment.EditedAt = new(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
			}

			recipient := &model.User{ID: "user-2", Email: tc.recipient, Username: "bob"}
			got, err := tc.format(commenter, recipient, tc.post, comment, tc.edited)
			require.NoError(t, err)
			require.Equal(t, sampleEnvelope(t, tc.name), got)

			if tc.edited {
				require.Contains(t, got.UniqueID, "2026-09-30T12:00:00Z")
			}
		})
	}
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
