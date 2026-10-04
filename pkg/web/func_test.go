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
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/samber/mo"
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

// postsService is the posts service over db.
func postsService(db *sqlx.DB) *posts.Service {
	return registry.New(db, registry.Deps{}).Posts
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

// connect creates a direct connection between a and b, or fails the test.
func connect(t *testing.T, db boil.ContextExecutor, ctx context.Context, aID, bID string) {
	t.Helper()
	_, _, err := factory.Connect(ctx, db, aID, bID)
	require.NoError(t, err)
}

// feedPage builds the feed page the way /feed does: the reading service's
// feed, rendered by Feed.
func feedPage(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*FeedPage] {
	feed, err := reading.New(repo.Using(db)).Feed(c, userData.DBUser, "")
	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	return mo.Ok(Feed(c, userData, feed))
}

// settingsPage builds the settings page the way its route does: the service
// gathers what it shows, the page builder shapes it.
func settingsPage(c *gin.Context, exec boil.ContextExecutor, user *core.User) (*SettingsPage, error) {
	svc := accounts.New(repo.Using(exec), nil, feeds.New(repo.Using(exec), nil))

	view, err := svc.Settings(c, user)
	if err != nil {
		return nil, err
	}

	return Settings(c, svc, userDataFor(user), view), nil
}

func TestInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	inviter := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID))(t)

	c := newTestContext(t, http.MethodGet, "/invite/"+invite.ID)

	page := Invite(c, testutil.Must(accounts.New(repo.New(db), nil, nil).Invitation(ctx, invite.ID))(t), userDataFor(nil))

	require.Equal(t, "Accept Invitation", page.Name)
	require.Equal(t, invite.ID, page.Invite.ID)
	require.Equal(t, inviter.ID, page.Inviter.ID)
}

func TestWrite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	recipient := testutil.Must(factory.User(ctx, db))(t)
	stranger := testutil.Must(factory.User(ctx, db))(t)
	asker := testutil.Must(factory.User(ctx, db))(t)
	prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID))(t)

	cases := []struct {
		name       string
		user       *core.User
		target     string
		wantPrompt bool
	}{
		{"no prompt query param", recipient, "/write", false},
		{"prompt belongs to the recipient", recipient, "/write?prompt=" + prompt.ID, true},
		{"prompt id does not match the signed-in user", stranger, "/write?prompt=" + prompt.ID, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newTestContext(t, http.MethodGet, tc.target)
			page := testutil.Must(Write(c, postsService(db), userDataFor(tc.user)).Get())(t)

			if tc.wantPrompt {
				require.NotNil(t, page.Prompt)
				require.Equal(t, prompt.ID, page.Prompt.Prompt.ID)
			} else {
				require.Nil(t, page.Prompt)
			}
		})
	}
}

func TestEditPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author := testutil.Must(factory.User(ctx, db))(t)
	stranger := testutil.Must(factory.User(ctx, db))(t)
	url := testutil.Must(factory.NormalizedURL(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID, factory.WithURL(url.ID), factory.Visibility(core.PostVisibilitySecondDegree)))(t)
	asker := testutil.Must(factory.User(ctx, db))(t)
	prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, author.ID, factory.WithPost(post.ID)))(t)

	t.Run("author can edit", func(t *testing.T) {
		t.Parallel()

		c := newTestContext(t, http.MethodGet, "/posts/"+post.ID+"/edit")
		page := testutil.Must(EditPost(c, postsService(db), userDataFor(author), post.ID).Get())(t)

		require.Equal(t, post.ID, page.PostID)
		require.Equal(t, post.Subject.String, page.Input.Subject)
		require.Equal(t, post.Body, page.Input.Body)
		require.Equal(t, core.PostVisibilitySecondDegree, page.Input.Visibility)
		require.Equal(t, url.URL, page.Input.URL)
		require.False(t, page.IsPublished)
		require.NotNil(t, page.Prompt)
		require.Equal(t, prompt.ID, page.Prompt.Prompt.ID)
	})

	t.Run("prefills the translation toggle", func(t *testing.T) {
		t.Parallel()

		yes := true
		saved := testutil.Must(postsService(db).Save(ctx, author, posts.SaveInput{
			Body: "body", Visibility: core.PostVisibilityPublic, Action: posts.ActionSavePost, AllowTranslation: &yes,
		}))(t)

		c := newTestContext(t, http.MethodGet, "/posts/"+saved.Post.ID+"/edit")
		page := testutil.Must(EditPost(c, postsService(db), userDataFor(author), saved.Post.ID).Get())(t)

		require.True(t, page.Input.AllowTranslation)
	})

	errCases := []struct {
		name   string
		user   *core.User
		postID string
		want   error
	}{
		{"stranger cannot edit", stranger, post.ID, service.ErrForbidden},
		{"missing post", author, "0190a0a0-0000-7000-8000-000000000000", service.ErrNotFound},
	}

	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newTestContext(t, http.MethodGet, "/posts/"+tc.postID+"/edit")
			res := EditPost(c, postsService(db), userDataFor(tc.user), tc.postID)
			require.True(t, res.IsError())
			require.ErrorIs(t, res.Error(), tc.want)
		})
	}
}

// controlsPage runs what the /controls route does: the service gathers the
// data, the page builder shapes it.
func controlsPage(t *testing.T, db *sqlx.DB, user *core.User) *ControlsPage {
	t.Helper()

	view := testutil.Must(connections.New(repo.New(db)).Controls(context.Background(), user))(t)

	return Controls(newTestContext(t, http.MethodGet, "/controls"), userDataFor(user), view)
}

func TestControls(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)
	alice := testutil.Must(factory.User(ctx, db))(t)
	bob := testutil.Must(factory.User(ctx, db))(t)
	carol := testutil.Must(factory.User(ctx, db))(t)

	connect(t, db, ctx, user.ID, alice.ID)
	connect(t, db, ctx, user.ID, bob.ID)
	connect(t, db, ctx, user.ID, carol.ID)

	// drafts: two of the user's own unpublished posts, plus a published
	// one that must not show up as a draft.
	draft1 := testutil.Must(factory.Post(ctx, db, user.ID))(t)
	draft2 := testutil.Must(factory.Post(ctx, db, user.ID))(t)
	testutil.Must(factory.Post(ctx, db, user.ID, factory.Published()))(t)

	// whitelist: a stranger allowed to connect to the user without mediation.
	whitelisted := testutil.Must(factory.User(ctx, db))(t)
	testutil.Must(factory.Whitelist(ctx, db, user.ID, whitelisted.ID))(t)

	// mediation requests: alice asks to connect to bob, both of whom are
	// the user's direct connections, and the user has not decided yet.
	pendingMediation := testutil.Must(factory.MediationRequest(ctx, db, alice.ID, bob.ID))(t)

	// a second mediation request the user has already signed off on: it
	// must not show up among the pending ones anymore.
	decidedMediation := testutil.Must(factory.MediationRequest(ctx, db, alice.ID, carol.ID))(t)
	testutil.Must(factory.MediatorDecision(ctx, db, decidedMediation.ID, user.ID, core.ConnectionMediationDecisionSigned))(t)

	// connection requests: a stranger asking to connect to the user,
	// vouched for by alice.
	stranger := testutil.Must(factory.User(ctx, db))(t)
	connRequest := testutil.Must(factory.MediationRequest(ctx, db, stranger.ID, user.ID))(t)
	testutil.Must(factory.MediatorDecision(ctx, db, connRequest.ID, alice.ID, core.ConnectionMediationDecisionSigned))(t)

	// a second request targeting the user where nobody has signed yet:
	// it must not appear as a connection request.
	stranger2 := testutil.Must(factory.User(ctx, db))(t)
	testutil.Must(factory.MediationRequest(ctx, db, stranger2.ID, user.ID))(t)

	page := controlsPage(t, db, user)

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

	user := testutil.Must(factory.User(ctx, db))(t)
	c := newTestContext(t, http.MethodGet, "/controls/settings")

	// baseline: no invites, no api key, no custom style, no feeds.
	page := testutil.Must(settingsPage(c, db, user))(t)

	require.Equal(t, int64(0), page.AvailableInvites)
	require.Empty(t, page.UsedInvites)
	require.Nil(t, page.ActiveAPIKey)
	require.Empty(t, page.Feeds)
	defaultStyles := page.UserStyles.Input.Styles

	// invite arithmetic: three slots total, one already sent.
	testutil.Must(factory.Invitation(ctx, db, user.ID))(t)
	testutil.Must(factory.Invitation(ctx, db, user.ID))(t)
	testutil.Must(factory.Invitation(ctx, db, user.ID, factory.Sent("invitee@example.test")))(t)

	apiKey := testutil.Must(factory.APIKey(ctx, db, user.ID))(t)
	testutil.Must(factory.UserStyle(ctx, db, user.ID, "body { color: red }"))(t)

	feed := testutil.Must(factory.RSSFeed(ctx, db))(t)
	subscription := testutil.Must(factory.Subscription(ctx, db, user.ID, feed.ID))(t)

	// an unrelated feed the user is not subscribed to must not appear.
	testutil.Must(factory.RSSFeed(ctx, db))(t)

	page = testutil.Must(settingsPage(c, db, user))(t)

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

	user := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	secondDegree := testutil.Must(factory.User(ctx, db))(t)
	stranger := testutil.Must(factory.User(ctx, db))(t)

	connect(t, db, ctx, user.ID, direct.ID)
	connect(t, db, ctx, direct.ID, secondDegree.ID)

	directPost := testutil.Must(factory.Post(ctx, db, direct.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly)))(t)
	publicPost := testutil.Must(factory.Post(ctx, db, secondDegree.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic)))(t)
	secondDegreeVisiblePost := testutil.Must(factory.Post(ctx, db, secondDegree.ID, factory.Published(), factory.Visibility(core.PostVisibilitySecondDegree)))(t)
	hiddenPost := testutil.Must(factory.Post(ctx, db, secondDegree.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly)))(t)
	strangerPost := testutil.Must(factory.Post(ctx, db, stranger.ID, factory.Published()))(t)

	c := newTestContext(t, http.MethodGet, "/feed")
	page := testutil.Must(feedPage(c, db, userDataFor(user)).Get())(t)

	byID := map[string]*FeedItem{}
	for _, item := range page.Items {
		if item.Post != nil {
			byID[item.Post.ID] = item
		}
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

	user := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, user.ID, direct.ID)

	// oldest: an rss item in the user's subscribed feed.
	feed := testutil.Must(factory.RSSFeed(ctx, db))(t)
	testutil.Must(factory.Subscription(ctx, db, user.ID, feed.ID))(t)
	rssItem := testutil.Must(factory.RSSItem(ctx, db, feed.ID))(t)
	testutil.Must(factory.UserFeedItem(ctx, db, user.ID, rssItem.ID))(t)

	time.Sleep(5 * time.Millisecond)

	// middle: a comment left by someone else on the user's own (draft)
	// post.
	ownPost := testutil.Must(factory.Post(ctx, db, user.ID))(t)
	comment := testutil.Must(factory.Comment(ctx, db, ownPost.ID, direct.ID))(t)

	time.Sleep(5 * time.Millisecond)

	// newest: a published post from a direct connection.
	post := testutil.Must(factory.Post(ctx, db, direct.ID, factory.Published()))(t)

	c := newTestContext(t, http.MethodGet, "/feed")
	page := testutil.Must(feedPage(c, db, userDataFor(user)).Get())(t)
	require.Empty(t, page.RSSFeed, "no private feed link without an api key")

	require.Len(t, page.Items, 3)
	require.NotNil(t, page.Items[0].Post)
	require.Equal(t, post.ID, page.Items[0].Post.ID)
	require.NotNil(t, page.Items[1].Comment)
	require.Equal(t, comment.ID, page.Items[1].Comment.ID)
	require.NotNil(t, page.Items[2].FeedItem)
	require.Equal(t, rssItem.Title, page.Items[2].FeedItem.Title)

	feedToken := testutil.Must(repo.RegenerateFeedToken(ctx, db, user.ID))(t)
	withKeyPage := testutil.Must(feedPage(c, db, userDataFor(user)).Get())(t)
	require.Equal(t, links.Link("private_user_feed", feedToken.Token), withKeyPage.RSSFeed)
}

func TestSettings_QueryErrorsPropagate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)
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
			t.Parallel()

			exec := &failingExecutor{ContextExecutor: db, failAfter: tc.failAfter}
			_, err := settingsPage(c, exec, user)
			require.Error(t, err)
		})
	}
}

func TestOrderByColumns(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// three timestamps, oldest first
	ts := []time.Time{base, base.Add(time.Hour), base.Add(2 * time.Hour)}

	// Each row inserts its records in the opposite of the order it expects,
	// so heap order and the requested order can never coincide.
	for _, tc := range []struct {
		name string
		run  func(t *testing.T) (want, got []string)
	}{
		{"Controls drafts: updated_at DESC", func(t *testing.T) ([]string, []string) {
			user := testutil.Must(factory.User(ctx, db))(t)
			var ids []string
			for _, at := range ts { // oldest inserted first
				ids = append(ids, testutil.Must(factory.Post(ctx, db, user.ID, factory.PostUpdatedAt(at)))(t).ID)
			}
			page := controlsPage(t, db, user)
			var got []string
			for _, d := range page.Drafts {
				got = append(got, d.PostID)
			}
			return []string{ids[2], ids[1], ids[0]}, got
		}},
		{"Feed open prompts: created_at DESC", func(t *testing.T) ([]string, []string) {
			asker := testutil.Must(factory.User(ctx, db))(t)
			recipient := testutil.Must(factory.User(ctx, db))(t)
			connect(t, db, ctx, recipient.ID, asker.ID)
			var ids []string
			for _, at := range ts { // oldest inserted first
				ids = append(ids, testutil.Must(factory.PostPrompt(ctx, db, asker.ID, recipient.ID, factory.PromptCreatedAt(at)))(t).ID)
			}
			page := testutil.Must(feedPage(newTestContext(t, http.MethodGet, "/feed"), db, userDataFor(recipient)).Get())(t)
			var got []string
			for _, p := range page.OpenPrompts {
				got = append(got, p.Prompt.ID)
			}
			return []string{ids[2], ids[1], ids[0]}, got
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, got := tc.run(t)
			require.Equal(t, want, got)
		})
	}
}
