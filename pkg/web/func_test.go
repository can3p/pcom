package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// errFeedInjectedQuery is the error a failingExecutor reports once its query
// budget runs out.
var errFeedInjectedQuery = errors.New("feed: injected query failure")

// failingExecutor wraps a boil.ContextExecutor and lets exactly
// failAfter queries through before failing every one after that. It exists
// to reach a handler's "return mo.Err(err)" branches, which a real database
// only takes on an actual failure, without the test body calling the ORM
// directly.
type failingExecutor struct {
	boil.ContextExecutor
	calls     int
	failAfter int
}

func (e *failingExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	e.calls++
	if e.calls > e.failAfter {
		return nil, errFeedInjectedQuery
	}
	return e.ContextExecutor.QueryContext(ctx, query, args...)
}

func (e *failingExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	e.calls++
	if e.calls > e.failAfter {
		// *sql.Row carries no exported way to inject an error directly, so
		// run a query that is guaranteed to fail instead: the error still
		// surfaces the normal way, on Scan.
		return e.ContextExecutor.QueryRowContext(ctx, "select 1/0")
	}
	return e.ContextExecutor.QueryRowContext(ctx, query, args...)
}

// userDataFor wraps u as a logged-in *auth.UserData, the way auth.Auth
// would for a signed-in request.
func userDataFor(u *core.User) *auth.UserData {
	return &auth.UserData{DBUser: u, IsLoggedIn: true}
}

// newTestContext returns a *gin.Context for method/target with no body, wired the
// way a real request is.
func newTestContext(t *testing.T, method, target string) *gin.Context {
	t.Helper()
	c, _ := ginctx.New(t, method, target, nil)
	return c
}

func TestInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID)
	require.NoError(t, err)

	c := newTestContext(t, http.MethodGet, "/invite/"+invite.ID)

	page := Invite(c, db, invite, userDataFor(nil))

	require.Equal(t, "Accept Invitation", page.Name)
	require.Equal(t, invite.ID, page.Invite.ID)
	require.Equal(t, inviter.ID, page.Inviter.ID)
}

func TestWrite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)

	recipient, err := factory.User(ctx, db)
	require.NoError(t, err)

	prompt, err := factory.PostPrompt(ctx, db, asker.ID, recipient.ID)
	require.NoError(t, err)

	t.Run("no prompt query param", func(t *testing.T) {
		c := newTestContext(t, http.MethodGet, "/write")

		res := Write(c, db, userDataFor(recipient))
		page, err := res.Get()
		require.NoError(t, err)
		require.Nil(t, page.Prompt)
	})

	t.Run("prompt belongs to the recipient", func(t *testing.T) {
		c := newTestContext(t, http.MethodGet, "/write?prompt="+prompt.ID)

		res := Write(c, db, userDataFor(recipient))
		page, err := res.Get()
		require.NoError(t, err)
		require.NotNil(t, page.Prompt)
		require.Equal(t, prompt.ID, page.Prompt.Prompt.ID)
	})

	t.Run("prompt id does not match the signed-in user", func(t *testing.T) {
		stranger, err := factory.User(ctx, db)
		require.NoError(t, err)

		c := newTestContext(t, http.MethodGet, "/write?prompt="+prompt.ID)

		res := Write(c, db, userDataFor(stranger))
		page, err := res.Get()
		require.NoError(t, err)
		require.Nil(t, page.Prompt)
	})
}

func TestEditPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)

	stranger, err := factory.User(ctx, db)
	require.NoError(t, err)

	url, err := factory.NormalizedURL(ctx, db)
	require.NoError(t, err)

	post, err := factory.Post(ctx, db, author.ID, factory.WithURL(url.ID), factory.Visibility(core.PostVisibilitySecondDegree))
	require.NoError(t, err)

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)

	prompt, err := factory.PostPrompt(ctx, db, asker.ID, author.ID, factory.WithPost(post.ID))
	require.NoError(t, err)

	t.Run("author can edit", func(t *testing.T) {
		c := newTestContext(t, http.MethodGet, "/posts/"+post.ID+"/edit")

		res := EditPost(c, db, userDataFor(author), post.ID)
		page, err := res.Get()
		require.NoError(t, err)

		require.Equal(t, post.ID, page.PostID)
		require.Equal(t, post.Subject.String, page.Input.Subject)
		require.Equal(t, post.Body, page.Input.Body)
		require.Equal(t, core.PostVisibilitySecondDegree, page.Input.Visibility)
		require.Equal(t, url.URL, page.Input.URL)
		require.False(t, page.IsPublished)
		require.NotNil(t, page.Prompt)
		require.Equal(t, prompt.ID, page.Prompt.Prompt.ID)
	})

	t.Run("stranger cannot edit", func(t *testing.T) {
		c := newTestContext(t, http.MethodGet, "/posts/"+post.ID+"/edit")

		res := EditPost(c, db, userDataFor(stranger), post.ID)
		require.True(t, res.IsError())
		require.ErrorIs(t, res.Error(), ginhelpers.ErrForbidden)
	})

	t.Run("missing post", func(t *testing.T) {
		c := newTestContext(t, http.MethodGet, "/posts/missing/edit")

		res := EditPost(c, db, userDataFor(author), "0190a0a0-0000-7000-8000-000000000000")
		require.True(t, res.IsError())
		require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
	})
}

func TestControls(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	alice, err := factory.User(ctx, db)
	require.NoError(t, err)

	bob, err := factory.User(ctx, db)
	require.NoError(t, err)

	carol, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user.ID, alice.ID)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user.ID, bob.ID)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user.ID, carol.ID)
	require.NoError(t, err)

	// drafts: two of the user's own unpublished posts, plus a published
	// one that must not show up as a draft.
	draft1, err := factory.Post(ctx, db, user.ID)
	require.NoError(t, err)

	draft2, err := factory.Post(ctx, db, user.ID)
	require.NoError(t, err)

	_, err = factory.Post(ctx, db, user.ID, factory.Published())
	require.NoError(t, err)

	// whitelist: a stranger allowed to connect to the user without
	// mediation.
	whitelisted, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.Whitelist(ctx, db, user.ID, whitelisted.ID)
	require.NoError(t, err)

	// mediation requests: alice asks to connect to bob, both of whom are
	// the user's direct connections, and the user has not decided yet.
	pendingMediation, err := factory.MediationRequest(ctx, db, alice.ID, bob.ID)
	require.NoError(t, err)

	// a second mediation request the user has already signed off on: it
	// must not show up among the pending ones anymore.
	decidedMediation, err := factory.MediationRequest(ctx, db, alice.ID, carol.ID)
	require.NoError(t, err)

	_, err = factory.MediatorDecision(ctx, db, decidedMediation.ID, user.ID, core.ConnectionMediationDecisionSigned)
	require.NoError(t, err)

	// connection requests: a stranger asking to connect to the user,
	// vouched for by alice.
	stranger, err := factory.User(ctx, db)
	require.NoError(t, err)

	connRequest, err := factory.MediationRequest(ctx, db, stranger.ID, user.ID)
	require.NoError(t, err)

	_, err = factory.MediatorDecision(ctx, db, connRequest.ID, alice.ID, core.ConnectionMediationDecisionSigned)
	require.NoError(t, err)

	// a second request targeting the user where nobody has signed yet:
	// it must not appear as a connection request.
	stranger2, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.MediationRequest(ctx, db, stranger2.ID, user.ID)
	require.NoError(t, err)

	c := newTestContext(t, http.MethodGet, "/controls")

	res := Controls(c, db, userDataFor(user))
	page, err := res.Get()
	require.NoError(t, err)

	gotDraftIDs := make([]string, len(page.Drafts))
	for i, d := range page.Drafts {
		gotDraftIDs[i] = d.PostID
	}
	assert.ElementsMatch(t, []string{draft1.ID, draft2.ID}, gotDraftIDs)

	gotWhitelistedIDs := make([]string, len(page.WhitelistedConnections))
	for i, u := range page.WhitelistedConnections {
		gotWhitelistedIDs[i] = u.ID
	}
	assert.ElementsMatch(t, []string{whitelisted.ID}, gotWhitelistedIDs)

	require.Len(t, page.MediationRequests, 1)
	require.Equal(t, pendingMediation.ID, page.MediationRequests[0].Request.ID)
	require.Equal(t, alice.ID, page.MediationRequests[0].Requester.ID)
	require.Equal(t, bob.ID, page.MediationRequests[0].Target.ID)

	require.Len(t, page.ConnectionRequests, 1)
	require.Equal(t, connRequest.ID, page.ConnectionRequests[0].Request.ID)
	require.Equal(t, stranger.ID, page.ConnectionRequests[0].Requester.ID)
	require.Len(t, page.ConnectionRequests[0].Mediations, 1)
	require.Equal(t, alice.ID, page.ConnectionRequests[0].Mediations[0].Mediator.ID)
}

func TestSettings(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	c := newTestContext(t, http.MethodGet, "/controls/settings")

	// baseline: no invites, no api key, no custom style, no feeds.
	res := Settings(c, db, userDataFor(user))
	page, err := res.Get()
	require.NoError(t, err)

	require.Equal(t, int64(0), page.AvailableInvites)
	require.Empty(t, page.UsedInvites)
	require.Nil(t, page.ActiveAPIKey)
	require.Empty(t, page.Feeds)
	defaultStyles := page.UserStyles.Input.Styles

	// invite arithmetic: three slots total, one already sent.
	_, err = factory.Invitation(ctx, db, user.ID)
	require.NoError(t, err)

	_, err = factory.Invitation(ctx, db, user.ID)
	require.NoError(t, err)

	_, err = factory.Invitation(ctx, db, user.ID, factory.Sent("invitee@example.test"))
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, db, user.ID)
	require.NoError(t, err)

	_, err = factory.UserStyle(ctx, db, user.ID, "body { color: red }")
	require.NoError(t, err)

	feed, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)

	subscription, err := factory.Subscription(ctx, db, user.ID, feed.ID)
	require.NoError(t, err)

	// an unrelated feed the user is not subscribed to must not appear.
	_, err = factory.RSSFeed(ctx, db)
	require.NoError(t, err)

	res = Settings(c, db, userDataFor(user))
	page, err = res.Get()
	require.NoError(t, err)

	require.Equal(t, int64(2), page.AvailableInvites, "3 slots minus 1 already sent")
	require.Len(t, page.UsedInvites, 1)
	require.Equal(t, "invitee@example.test", page.UsedInvites[0].InvitationEmail.String)

	require.NotNil(t, page.ActiveAPIKey)
	require.Equal(t, apiKey.APIKey, page.ActiveAPIKey.APIKey)

	require.NotEqual(t, defaultStyles, page.UserStyles.Input.Styles)
	require.Equal(t, "body { color: red }", page.UserStyles.Input.Styles)

	require.Len(t, page.Feeds, 1)
	require.Equal(t, subscription.ID, page.Feeds[0].ID)
	require.Equal(t, feed.URL, page.Feeds[0].URL)
}

func TestFeed_PostVisibilityAndVia(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	direct, err := factory.User(ctx, db)
	require.NoError(t, err)

	secondDegree, err := factory.User(ctx, db)
	require.NoError(t, err)

	stranger, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user.ID, direct.ID)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, direct.ID, secondDegree.ID)
	require.NoError(t, err)

	directPost, err := factory.Post(ctx, db, direct.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	require.NoError(t, err)

	publicPost, err := factory.Post(ctx, db, secondDegree.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	require.NoError(t, err)

	secondDegreeVisiblePost, err := factory.Post(ctx, db, secondDegree.ID, factory.Published(), factory.Visibility(core.PostVisibilitySecondDegree))
	require.NoError(t, err)

	hiddenPost, err := factory.Post(ctx, db, secondDegree.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	require.NoError(t, err)

	strangerPost, err := factory.Post(ctx, db, stranger.ID, factory.Published())
	require.NoError(t, err)

	c := newTestContext(t, http.MethodGet, "/feed")

	res := Feed(c, db, userDataFor(user), true)
	page, err := res.Get()
	require.NoError(t, err)

	byID := map[string]*FeedItem{}
	for _, item := range page.Items {
		byID[item.Post.ID] = item
	}

	require.Contains(t, byID, directPost.ID, "a direct connection's post is always visible")
	require.Contains(t, byID, publicPost.ID, "a public post from a second-degree connection is visible")
	require.Contains(t, byID, secondDegreeVisiblePost.ID, "a second-degree post from a second-degree connection is visible")
	require.NotContains(t, byID, hiddenPost.ID, "a direct-only post from a second-degree connection is filtered out")
	require.NotContains(t, byID, strangerPost.ID, "a post from an unconnected user never shows up")

	require.Empty(t, byID[directPost.ID].Post.Via, "a direct connection's post has no via users")
	require.Equal(t, []string{direct.ID}, lo.Map(byID[publicPost.ID].Post.Via, func(u *core.User, _ int) string { return u.ID }))
	require.Equal(t, []string{direct.ID}, lo.Map(byID[secondDegreeVisiblePost.ID].Post.Via, func(u *core.User, _ int) string { return u.ID }))
}

func TestFeed_RSSCommentsOrderingAndLinks(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	direct, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user.ID, direct.ID)
	require.NoError(t, err)

	// oldest: an rss item in the user's subscribed feed.
	feed, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)

	_, err = factory.Subscription(ctx, db, user.ID, feed.ID)
	require.NoError(t, err)

	rssItem, err := factory.RSSItem(ctx, db, feed.ID)
	require.NoError(t, err)

	_, err = factory.UserFeedItem(ctx, db, user.ID, rssItem.ID)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	// middle: a comment left by someone else on the user's own (draft)
	// post.
	ownPost, err := factory.Post(ctx, db, user.ID)
	require.NoError(t, err)

	comment, err := factory.Comment(ctx, db, ownPost.ID, direct.ID)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	// newest: a published post from a direct connection.
	post, err := factory.Post(ctx, db, direct.ID, factory.Published())
	require.NoError(t, err)

	c := newTestContext(t, http.MethodGet, "/feed")

	res := Feed(c, db, userDataFor(user), false)
	page, err := res.Get()
	require.NoError(t, err)
	require.Empty(t, page.RSSFeed, "no private feed link without an api key")

	require.Len(t, page.Items, 3)
	require.NotNil(t, page.Items[0].Post)
	require.Equal(t, post.ID, page.Items[0].Post.ID)
	require.NotNil(t, page.Items[1].Comment)
	require.Equal(t, comment.ID, page.Items[1].Comment.ID)
	require.NotNil(t, page.Items[2].FeedItem)
	require.Equal(t, rssItem.Title, page.Items[2].FeedItem.Title)

	onlyPostsRes := Feed(c, db, userDataFor(user), true)
	onlyPostsPage, err := onlyPostsRes.Get()
	require.NoError(t, err)
	require.Nil(t, onlyPostsPage.BasePage, "onlyPosts skips the rest of page composition")
	require.Len(t, onlyPostsPage.Items, 1, "onlyPosts drops rss items and comments")
	require.Equal(t, post.ID, onlyPostsPage.Items[0].Post.ID)

	apiKey, err := factory.APIKey(ctx, db, user.ID)
	require.NoError(t, err)

	withKeyRes := Feed(c, db, userDataFor(user), false)
	withKeyPage, err := withKeyRes.Get()
	require.NoError(t, err)
	require.Equal(t, links.Link("private_user_feed", apiKey.APIKey), withKeyPage.RSSFeed)
}

func TestGetComments(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	direct, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user.ID, direct.ID)
	require.NoError(t, err)

	// a comment by someone else on the user's own post: always included.
	ownPost, err := factory.Post(ctx, db, user.ID)
	require.NoError(t, err)

	ownPostComment, err := factory.Comment(ctx, db, ownPost.ID, direct.ID)
	require.NoError(t, err)

	// a direct connection's post the user has participated in: a further
	// comment from someone else on it is included too.
	participatedPost, err := factory.Post(ctx, db, direct.ID)
	require.NoError(t, err)

	_, err = factory.Comment(ctx, db, participatedPost.ID, user.ID)
	require.NoError(t, err)

	otherCommenter, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user.ID, otherCommenter.ID)
	require.NoError(t, err)

	participatedComment, err := factory.Comment(ctx, db, participatedPost.ID, otherCommenter.ID)
	require.NoError(t, err)

	// a direct connection's post the user never commented on: excluded,
	// even though someone else left a comment on it.
	untouchedPost, err := factory.Post(ctx, db, direct.ID)
	require.NoError(t, err)

	_, err = factory.Comment(ctx, db, untouchedPost.ID, otherCommenter.ID)
	require.NoError(t, err)

	items, err := getComments(ctx, db, user.ID)
	require.NoError(t, err)

	gotIDs := make([]string, len(items))
	for i, item := range items {
		require.NotNil(t, item.Comment)
		gotIDs[i] = item.Comment.ID
	}

	assert.ElementsMatch(t, []string{ownPostComment.ID, participatedComment.ID}, gotIDs)

	for _, item := range items {
		require.NotEqual(t, user.ID, item.Comment.UserID, "the user's own comments are never echoed back")
	}
}

func TestGetComments_QueryErrorsPropagate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	for _, tc := range []struct {
		name      string
		failAfter int
	}{
		{"own comments lookup fails", 0},
		{"direct user ids lookup fails", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := &failingExecutor{ContextExecutor: db, failAfter: tc.failAfter}

			_, err := getComments(ctx, exec, user.ID)
			require.ErrorIs(t, err, errFeedInjectedQuery)
		})
	}
}

func TestSettings_QueryErrorsPropagate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	c := newTestContext(t, http.MethodGet, "/controls/settings")

	for _, tc := range []struct {
		name      string
		failAfter int
	}{
		{"invite count fails", 0},
		{"used invites lookup fails", 1},
		{"api key lookup fails", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := &failingExecutor{ContextExecutor: db, failAfter: tc.failAfter}

			res := Settings(c, exec, userDataFor(user))
			require.True(t, res.IsError())
		})
	}
}
