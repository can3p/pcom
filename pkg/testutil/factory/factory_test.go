package factory_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// TestFactoriesBuildEveryEntity builds one of every entity the factory
// package offers, proving that every default satisfies the schema and that
// every documented option works.
func TestFactoriesBuildEveryEntity(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	// Users and graph.
	alice, err := factory.User(ctx, db)
	require.NoError(t, err)

	bob, err := factory.User(ctx, db,
		factory.WithPassword("s3cret-password"),
		factory.WithVisibility(core.ProfileVisibilityPublic),
	)
	require.NoError(t, err)
	require.True(t, bob.Pwdhash.Valid)
	require.Equal(t, core.ProfileVisibilityPublic, bob.ProfileVisibility)

	carol, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, alice.ID, bob.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, alice.ID, carol.ID)
	require.NoError(t, err)

	_, err = factory.Whitelist(ctx, db, alice.ID, carol.ID)
	require.NoError(t, err)

	mediationRequest, err := factory.MediationRequest(ctx, db, carol.ID, bob.ID,
		factory.WithSourceNote("we have a friend in common"),
	)
	require.NoError(t, err)

	_, err = factory.MediatorDecision(ctx, db, mediationRequest.ID, alice.ID, core.ConnectionMediationDecisionSigned)
	require.NoError(t, err)

	// Posts.
	url, err := factory.NormalizedURL(ctx, db)
	require.NoError(t, err)

	post, err := factory.Post(ctx, db, alice.ID,
		factory.Published(),
		factory.Visibility(core.PostVisibilityPublic),
		factory.WithURL(url.ID),
	)
	require.NoError(t, err)
	require.True(t, post.PublishedAt.Valid)
	require.Equal(t, core.PostVisibilityPublic, post.VisibilityRadius)
	require.True(t, post.URLID.Valid)

	draft, err := factory.Post(ctx, db, bob.ID)
	require.NoError(t, err)
	require.False(t, draft.PublishedAt.Valid)

	topComment, err := factory.Comment(ctx, db, post.ID, bob.ID)
	require.NoError(t, err)

	reply, err := factory.Comment(ctx, db, post.ID, carol.ID, factory.ReplyTo(topComment.ID))
	require.NoError(t, err)
	require.Equal(t, topComment.ID, reply.TopCommentID)

	_, err = factory.PostStat(ctx, db, post.ID)
	require.NoError(t, err)

	_, err = factory.PostShare(ctx, db, post.ID)
	require.NoError(t, err)

	_, err = factory.PostPrompt(ctx, db, alice.ID, bob.ID,
		factory.WithPost(post.ID),
		factory.Dismissed(),
	)
	require.NoError(t, err)

	// Account.
	_, err = factory.Invitation(ctx, db, alice.ID, factory.Sent("invitee@example.test"))
	require.NoError(t, err)

	_, err = factory.SignupRequest(ctx, db)
	require.NoError(t, err)

	_, err = factory.APIKey(ctx, db, alice.ID)
	require.NoError(t, err)

	_, err = factory.UserStyle(ctx, db, alice.ID, "body { color: red; }")
	require.NoError(t, err)

	require.NoError(t, factory.SetRegistrationOpen(ctx, db, false))

	// Feeds.
	feed, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)

	_, err = factory.Subscription(ctx, db, alice.ID, feed.ID)
	require.NoError(t, err)

	item, err := factory.RSSItem(ctx, db, feed.ID)
	require.NoError(t, err)

	_, err = factory.UserFeedItem(ctx, db, alice.ID, item.ID, factory.IsDismissed())
	require.NoError(t, err)

	item2, err := factory.RSSItem(ctx, db, feed.ID, factory.WithURLID(url.ID))
	require.NoError(t, err)
	require.Equal(t, url.ID, item2.URLID)

	// Media and mail.
	_, err = factory.MediaUpload(ctx, db, alice.ID)
	require.NoError(t, err)

	_, err = factory.OutgoingEmail(ctx, db, "test_email")
	require.NoError(t, err)

	// Readers.
	gotUser, err := factory.GetUser(ctx, db, alice.ID)
	require.NoError(t, err)
	require.Equal(t, alice.ID, gotUser.ID)

	gotPost, err := factory.GetPost(ctx, db, post.ID)
	require.NoError(t, err)
	require.Equal(t, post.ID, gotPost.ID)

	posts, err := factory.ListPosts(ctx, db, alice.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)

	comments, err := factory.ListComments(ctx, db, post.ID)
	require.NoError(t, err)
	require.Len(t, comments, 2)

	emails, err := factory.ListOutgoingEmails(ctx, db, core.OutgoingEmailWhere.EmailType.EQ("test_email"))
	require.NoError(t, err)
	require.Len(t, emails, 1)

	connected, err := factory.ConnectionExists(ctx, db, alice.ID, bob.ID)
	require.NoError(t, err)
	require.True(t, connected)

	connected, err = factory.ConnectionExists(ctx, db, bob.ID, alice.ID)
	require.NoError(t, err)
	require.True(t, connected)

	connected, err = factory.ConnectionExists(ctx, db, bob.ID, carol.ID)
	require.NoError(t, err)
	require.False(t, connected)

	gotRequest, err := factory.GetMediationRequest(ctx, db, mediationRequest.ID)
	require.NoError(t, err)
	require.Equal(t, mediationRequest.ID, gotRequest.ID)
}
