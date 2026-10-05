package e2e_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

const (
	chromeUA  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
	firefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"

	// unknownID is a well-formed id that no row has.
	unknownID = "00000000-0000-0000-0000-000000000000"
)

func newPost(t *testing.T, app *e2e.App, authorID string, opts ...factory.PostOpt) *model.Post {
	t.Helper()

	p, err := factory.Post(context.Background(), app.DB, authorID, opts...)
	require.NoError(t, err)

	return p
}

func connectUsers(t *testing.T, app *e2e.App, a, b *model.User) {
	t.Helper()

	_, _, err := factory.Connect(context.Background(), app.DB, a.ID, b.ID)
	require.NoError(t, err)
}

// getWith requests path with extra request headers.
func getWith(t *testing.T, app *e2e.App, c *e2e.Client, path string, headers map[string]string) *e2e.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, app.URL+path, nil)
	require.NoError(t, err)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.Do(req)
}

// requireStatus asserts resp's status on t. Response.RequireStatus fails the
// test that created the client, which is wrong inside a subtest.
func requireStatus(t *testing.T, resp *e2e.Response, code int) *e2e.Response {
	t.Helper()
	require.Equal(t, code, resp.StatusCode, resp.Body)

	return resp
}

// requireLoginRedirect asserts a redirect to the login page that brings
// the visitor back to path.
func requireLoginRedirect(t *testing.T, resp *e2e.Response, path string) {
	t.Helper()

	requireStatus(t, resp, http.StatusFound)

	loc, err := url.Parse(resp.Location())
	require.NoError(t, err)
	require.Equal(t, "/login", loc.Path)
	require.Equal(t, path, loc.Query().Get("return_url"))
	require.NotEmpty(t, loc.Query().Get("sign"))
}

// requirePostPage asserts the single post page of p.
func requirePostPage(t *testing.T, resp *e2e.Response, p *model.Post) {
	t.Helper()

	requireStatus(t, resp, http.StatusOK)
	require.Contains(t, resp.Doc().Find("h1.us-post-header").Text(), lo.FromPtr(p.Subject))
}

func zipNames(t *testing.T, body string) []string {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	require.NoError(t, err)

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}

	return names
}

// TestVisibility_Index: the anonymous index is the public posts feed (the Q15 matrix
// is owned by the reading service); a logged in user is sent to the feed.
func TestVisibility_Index(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	pubAuthor := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	regAuthor := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityRegisteredUsers))
	included := newPost(t, app, pubAuthor.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	excluded := newPost(t, app, regAuthor.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))

	text := requireStatus(t, app.Client(t).Get("/"), http.StatusOK).Doc().Text()
	require.Contains(t, text, lo.FromPtr(included.Subject))
	require.NotContains(t, text, lo.FromPtr(excluded.Subject))

	resp := requireStatus(t, loginAs(t, app, pubAuthor).Get("/"), http.StatusFound)
	require.Equal(t, "/feed", resp.Location())
}

func TestVisibility_Articles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	raw, err := os.ReadFile("../cmd/web/client/articles/privacy_policy.md")
	require.NoError(t, err)
	title, _, _ := strings.Cut(string(raw), "\n")

	resp := requireStatus(t, c.Get("/articles/privacy_policy"), http.StatusOK)
	require.Contains(t, resp.Doc().Text(), strings.TrimSpace(title))

	requireStatus(t, c.Get("/articles/terms_of_service"), http.StatusOK)

	for _, path := range []string{
		"/articles/why",
		"/articles/does_not_exist",
		"/articles/Privacy_Policy",
		"/articles/bad-name",
		"/articles/_privacy_policy",
		"/articles/..%2Fhtml%2Findex",
	} {
		requireStatus(t, c.Get(path), http.StatusNotFound)
	}
}

// TestVisibility_UserHome: a journal is visible to anyone for a public profile, to
// logged-in users for registered_users, and to connections only for
// connections; otherwise it does not exist (404).
func TestVisibility_UserHome(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	unrelated := newUser(t, app)
	direct := newUser(t, app)

	clients := map[string]*e2e.Client{
		"anonymous": app.Client(t),
		"unrelated": loginAs(t, app, unrelated),
		"direct":    loginAs(t, app, direct),
	}

	authors := map[model.ProfileVisibility]*model.User{}
	for _, v := range []model.ProfileVisibility{model.ProfileVisibilityPublic, model.ProfileVisibilityRegisteredUsers, model.ProfileVisibilityConnections} {
		authors[v] = newUser(t, app, factory.WithVisibility(v))
		connectUsers(t, app, authors[v], direct)
	}

	cases := []struct {
		profile model.ProfileVisibility
		viewer  string
		want    int
	}{
		{model.ProfileVisibilityPublic, "anonymous", http.StatusOK},
		{model.ProfileVisibilityPublic, "unrelated", http.StatusOK},
		{model.ProfileVisibilityPublic, "direct", http.StatusOK},
		{model.ProfileVisibilityRegisteredUsers, "anonymous", http.StatusNotFound},
		{model.ProfileVisibilityRegisteredUsers, "unrelated", http.StatusOK},
		{model.ProfileVisibilityRegisteredUsers, "direct", http.StatusOK},
		{model.ProfileVisibilityConnections, "anonymous", http.StatusNotFound},
		{model.ProfileVisibilityConnections, "unrelated", http.StatusNotFound},
		{model.ProfileVisibilityConnections, "direct", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s", tc.profile, tc.viewer), func(t *testing.T) {
			author := authors[tc.profile]
			resp := requireStatus(t, clients[tc.viewer].Get("/users/"+author.Username), tc.want)

			if tc.want == http.StatusOK {
				require.Contains(t, resp.Doc().Text(), author.Username)
			}
		})
	}

	requireStatus(t, clients["unrelated"].Get("/users/no_such_user"), http.StatusNotFound)
}

// TestVisibility_UserHomePostsByRadius: the journal lists published posts the
// viewer's radius admits.
func TestVisibility_UserHomePostsByRadius(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	direct := newUser(t, app)
	connectUsers(t, app, author, direct)

	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	directOnly := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityDirectOnly))
	draft := newPost(t, app, author.ID, factory.Visibility(model.PostVisibilityPublic))

	anonText := requireStatus(t, app.Client(t).Get("/users/"+author.Username), http.StatusOK).Doc().Text()
	require.Contains(t, anonText, lo.FromPtr(public.Subject))
	require.NotContains(t, anonText, lo.FromPtr(directOnly.Subject))
	require.NotContains(t, anonText, lo.FromPtr(draft.Subject))

	directText := requireStatus(t, loginAs(t, app, direct).Get("/users/"+author.Username), http.StatusOK).Doc().Text()
	require.Contains(t, directText, lo.FromPtr(public.Subject))
	require.Contains(t, directText, lo.FromPtr(directOnly.Subject))
	require.NotContains(t, directText, lo.FromPtr(draft.Subject))
}

// TestVisibility_PublicRSS: only public profiles have a public feed, and it carries
// only published public posts.
func TestVisibility_PublicRSS(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	author := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	directOnly := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityDirectOnly))
	draft := newPost(t, app, author.ID, factory.Visibility(model.PostVisibilityPublic))

	resp := requireStatus(t, c.Get("/rss/public/"+author.Username), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/xml"), resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Body, "<rss")
	require.Contains(t, resp.Body, lo.FromPtr(public.Subject))
	require.NotContains(t, resp.Body, lo.FromPtr(directOnly.Subject))
	require.NotContains(t, resp.Body, lo.FromPtr(draft.Subject))

	for _, v := range []model.ProfileVisibility{model.ProfileVisibilityRegisteredUsers, model.ProfileVisibilityConnections} {
		hidden := newUser(t, app, factory.WithVisibility(v))
		newPost(t, app, hidden.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))

		requireStatus(t, c.Get("/rss/public/"+hidden.Username), http.StatusNotFound)
	}

	requireStatus(t, c.Get("/rss/public/no_such_user"), http.StatusNotFound)
}

func TestVisibility_UserStyles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	const css = ".post { color: red; }"

	styled := newUser(t, app)
	_, err := factory.UserStyle(context.Background(), app.DB, styled.ID, css)
	require.NoError(t, err)

	plain := newUser(t, app)

	path := "/users/" + styled.Username + "/user_styles"
	referer := app.URL + "/users/" + styled.Username

	t.Run("no referer", func(t *testing.T) {
		requireStatus(t, getWith(t, app, c, path, map[string]string{"User-Agent": chromeUA}), http.StatusNotFound)
	})

	t.Run("foreign referer", func(t *testing.T) {
		requireStatus(t, getWith(t, app, c, path, map[string]string{"User-Agent": chromeUA, "Referer": "https://evil.example/" + styled.Username}), http.StatusNotFound)
	})

	t.Run("non-Firefox gets @scope", func(t *testing.T) {
		resp := requireStatus(t, getWith(t, app, c, path, map[string]string{"User-Agent": chromeUA, "Referer": referer}), http.StatusOK)
		require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css"), resp.Header.Get("Content-Type"))
		require.True(t, strings.HasPrefix(resp.Body, "@scope (.user-styles-applied) {"), resp.Body)
		require.Contains(t, resp.Body, css)
		require.True(t, strings.HasSuffix(strings.TrimSpace(resp.Body), "}"), resp.Body)
	})

	t.Run("Firefox gets the css as is", func(t *testing.T) {
		resp := requireStatus(t, getWith(t, app, c, path, map[string]string{"User-Agent": firefoxUA, "Referer": referer}), http.StatusOK)
		require.Equal(t, css, resp.Body)
	})

	t.Run("user without styles", func(t *testing.T) {
		resp := requireStatus(t, getWith(t, app, c, "/users/"+plain.Username+"/user_styles", map[string]string{"User-Agent": chromeUA, "Referer": referer}), http.StatusOK)
		require.Empty(t, resp.Body)
	})

	t.Run("unknown user", func(t *testing.T) {
		resp := requireStatus(t, getWith(t, app, c, "/users/no_such_user/user_styles", map[string]string{"User-Agent": chromeUA, "Referer": referer}), http.StatusOK)
		require.Empty(t, resp.Body)
	})
}

type viewerKind string

const (
	viewerAnon      viewerKind = "anonymous"
	viewerUnrelated viewerKind = "registered-unrelated"
	viewerSecond    viewerKind = "second-degree"
	viewerDirect    viewerKind = "direct"
	viewerAuthor    viewerKind = "author"
)

type outcome string

const (
	outcomeVisible    outcome = "visible"
	outcomeNeedsLogin outcome = "needs-login"
	outcomeNotFound   outcome = "not-found"
)

const (
	asPublished = false
	asDraft     = true
)

type postKey struct {
	profile model.ProfileVisibility
	post    model.PostVisibility
	draft   bool
}

// TestVisibility_SinglePostMatrix is the post visibility matrix: who can open
// /posts/:id. A post the viewer may not see is a login redirect for an
// anonymous visitor and a 404 for everyone else, so its existence isn't
// revealed. The visibility radius decides: public for anyone, second_degree
// for connections of connections, direct_only for direct connections; the
// author sees everything. A draft is the author's alone.
//
// The profile's visibility plays no part at /posts/:id (unlike the journal,
// RSS and explore): a public post is visible to everybody, by decision
// (docs/product.md, "A public post is public by URL").
func TestVisibility_SinglePostMatrix(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	direct := newUser(t, app)
	second := newUser(t, app)
	unrelated := newUser(t, app)
	connectUsers(t, app, second, direct)

	clients := map[viewerKind]*e2e.Client{
		viewerAnon:      app.Client(t),
		viewerUnrelated: loginAs(t, app, unrelated),
		viewerSecond:    loginAs(t, app, second),
		viewerDirect:    loginAs(t, app, direct),
	}

	authorClients := map[model.ProfileVisibility]*e2e.Client{}
	posts := map[postKey]*model.Post{}

	for _, pv := range []model.ProfileVisibility{model.ProfileVisibilityPublic, model.ProfileVisibilityRegisteredUsers, model.ProfileVisibilityConnections} {
		author := newUser(t, app, factory.WithVisibility(pv))
		connectUsers(t, app, author, direct)
		authorClients[pv] = loginAs(t, app, author)

		for _, v := range []model.PostVisibility{model.PostVisibilityPublic, model.PostVisibilitySecondDegree, model.PostVisibilityDirectOnly} {
			posts[postKey{pv, v, asPublished}] = newPost(t, app, author.ID, factory.Visibility(v), factory.Published())
			posts[postKey{pv, v, asDraft}] = newPost(t, app, author.ID, factory.Visibility(v))
		}
	}

	cases := []struct {
		viewer  viewerKind
		profile model.ProfileVisibility
		post    model.PostVisibility
		draft   bool
		want    outcome
	}{
		// Published posts
		// public profile
		{viewerAnon, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAnon, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asPublished, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asPublished, outcomeNeedsLogin},
		{viewerUnrelated, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerUnrelated, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asPublished, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerSecond, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerSecond, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerDirect, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerDirect, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		// registered profile
		{viewerAnon, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asPublished, outcomeVisible}, // a public post is public whatever the profile visibility
		{viewerAnon, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asPublished, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asPublished, outcomeNeedsLogin},
		{viewerUnrelated, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerUnrelated, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asPublished, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerSecond, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerSecond, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerDirect, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerDirect, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		// connections profile
		{viewerAnon, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asPublished, outcomeVisible}, // a public post is public whatever the profile visibility
		{viewerAnon, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asPublished, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asPublished, outcomeNeedsLogin},
		{viewerUnrelated, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asPublished, outcomeVisible}, // a public post is public whatever the profile visibility
		{viewerUnrelated, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asPublished, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerSecond, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerSecond, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerDirect, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerDirect, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		// Draft posts
		// public profile
		{viewerAnon, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asDraft, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asDraft, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asDraft, outcomeNeedsLogin},
		{viewerUnrelated, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerAuthor, model.ProfileVisibilityPublic, model.PostVisibilityPublic, asDraft, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityPublic, model.PostVisibilitySecondDegree, asDraft, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityPublic, model.PostVisibilityDirectOnly, asDraft, outcomeVisible},
		// registered profile
		{viewerAnon, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asDraft, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asDraft, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asDraft, outcomeNeedsLogin},
		{viewerUnrelated, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerAuthor, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityPublic, asDraft, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityRegisteredUsers, model.PostVisibilitySecondDegree, asDraft, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityRegisteredUsers, model.PostVisibilityDirectOnly, asDraft, outcomeVisible},
		// connections profile
		{viewerAnon, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asDraft, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asDraft, outcomeNeedsLogin},
		{viewerAnon, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asDraft, outcomeNeedsLogin},
		{viewerUnrelated, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerUnrelated, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerSecond, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerDirect, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerAuthor, model.ProfileVisibilityConnections, model.PostVisibilityPublic, asDraft, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityConnections, model.PostVisibilitySecondDegree, asDraft, outcomeVisible},
		{viewerAuthor, model.ProfileVisibilityConnections, model.PostVisibilityDirectOnly, asDraft, outcomeVisible},
	}

	for _, tc := range cases {
		state := "published"
		if tc.draft {
			state = "draft"
		}

		t.Run(fmt.Sprintf("%s/%s-profile/%s-post/%s", tc.viewer, tc.profile, tc.post, state), func(t *testing.T) {
			post := posts[postKey{tc.profile, tc.post, tc.draft}]
			path := "/posts/" + post.ID

			client := clients[tc.viewer]
			if tc.viewer == viewerAuthor {
				client = authorClients[tc.profile]
			}

			resp := client.Get(path)

			switch tc.want {
			case outcomeVisible:
				requirePostPage(t, resp, post)
			case outcomeNeedsLogin:
				requireLoginRedirect(t, resp, path)
			case outcomeNotFound:
				requireStatus(t, resp, http.StatusNotFound)
			}
		})
	}

	t.Run("unknown post", func(t *testing.T) {
		requireStatus(t, clients[viewerUnrelated].Get("/posts/"+unknownID), http.StatusNotFound)
	})
}

func TestVisibility_PostMarkdown(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	unrelated := newUser(t, app)

	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	directOnly := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityDirectOnly))

	resp := requireStatus(t, loginAs(t, app, author).Get("/posts/"+directOnly.ID+"/md"), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain"), resp.Header.Get("Content-Type"))
	require.True(t, strings.HasPrefix(resp.Body, "---\n"), resp.Body)
	require.Contains(t, resp.Body, directOnly.ID)
	require.Contains(t, resp.Body, lo.FromPtr(directOnly.Subject))
	require.Contains(t, resp.Body, directOnly.Body)

	anon := app.Client(t)
	resp = requireStatus(t, anon.Get("/posts/"+public.ID+"/md"), http.StatusOK)
	require.Contains(t, resp.Body, public.Body)

	requireLoginRedirect(t, anon.Get("/posts/"+directOnly.ID+"/md"), "/posts/"+directOnly.ID+"/md")
	requireStatus(t, loginAs(t, app, unrelated).Get("/posts/"+directOnly.ID+"/md"), http.StatusNotFound)
	requireStatus(t, anon.Get("/posts/"+unknownID+"/md"), http.StatusNotFound)
}

// TestVisibility_PostZip: whoever may see a post (Q7) may export it, by the
// same rules as /posts/:id: an anonymous visitor to a post they can't see is
// sent to log in, any other viewer gets a 404. Every 200 archive holds this
// post and none of the author's other posts.
func TestVisibility_PostZip(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	direct := newUser(t, app)
	second := newUser(t, app)
	unrelated := newUser(t, app)
	connectUsers(t, app, author, direct)
	connectUsers(t, app, second, direct)

	clients := map[viewerKind]*e2e.Client{
		viewerAnon:      app.Client(t),
		viewerUnrelated: loginAs(t, app, unrelated),
		viewerSecond:    loginAs(t, app, second),
		viewerDirect:    loginAs(t, app, direct),
		viewerAuthor:    loginAs(t, app, author),
	}

	posts := map[model.PostVisibility]*model.Post{}
	for _, v := range []model.PostVisibility{model.PostVisibilityPublic, model.PostVisibilitySecondDegree, model.PostVisibilityDirectOnly} {
		posts[v] = newPost(t, app, author.ID, factory.Visibility(v), factory.Published())
	}

	cases := []struct {
		viewer viewerKind
		post   model.PostVisibility
		want   int
	}{
		{viewerAuthor, model.PostVisibilityPublic, http.StatusOK},
		{viewerAuthor, model.PostVisibilitySecondDegree, http.StatusOK},
		{viewerAuthor, model.PostVisibilityDirectOnly, http.StatusOK},
		{viewerAnon, model.PostVisibilityPublic, http.StatusOK},
		{viewerAnon, model.PostVisibilitySecondDegree, http.StatusFound},
		{viewerAnon, model.PostVisibilityDirectOnly, http.StatusFound},
		{viewerUnrelated, model.PostVisibilityPublic, http.StatusOK},
		{viewerUnrelated, model.PostVisibilitySecondDegree, http.StatusNotFound},
		{viewerUnrelated, model.PostVisibilityDirectOnly, http.StatusNotFound},
		{viewerDirect, model.PostVisibilityPublic, http.StatusOK},
		{viewerDirect, model.PostVisibilitySecondDegree, http.StatusOK},
		{viewerDirect, model.PostVisibilityDirectOnly, http.StatusOK},
		{viewerSecond, model.PostVisibilityPublic, http.StatusOK},
		{viewerSecond, model.PostVisibilitySecondDegree, http.StatusOK},
		{viewerSecond, model.PostVisibilityDirectOnly, http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(string(tc.viewer)+"/"+string(tc.post), func(t *testing.T) {
			post := posts[tc.post]
			path := "/posts/" + post.ID + "/zip"
			resp := clients[tc.viewer].Get(path)

			switch tc.want {
			case http.StatusOK:
				requireStatus(t, resp, http.StatusOK)
				require.Equal(t, "application/zip", resp.Header.Get("Content-Type"))
				require.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
				require.Equal(t, []string{post.ID + ".md"}, zipNames(t, resp.Body))
				require.Contains(t, zipFileText(t, resp.Body, post.ID+".md"), post.Body)
			case http.StatusFound:
				requireLoginRedirect(t, resp, path)
			default:
				requireStatus(t, resp, tc.want)
			}
		})
	}
}

func zipFileText(t *testing.T, body, name string) string {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	require.NoError(t, err)

	f, err := r.Open(name)
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	b, err := io.ReadAll(f)
	require.NoError(t, err)

	return string(b)
}

func TestVisibility_PostEdit(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	direct := newUser(t, app)
	unrelated := newUser(t, app)
	connectUsers(t, app, author, direct)

	post := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	path := "/posts/" + post.ID + "/edit"

	requireLoginRedirect(t, app.Client(t).Get(path), path)
	requireStatus(t, loginAs(t, app, direct).Get(path), http.StatusForbidden)
	requireStatus(t, loginAs(t, app, unrelated).Get(path), http.StatusForbidden)

	resp := requireStatus(t, loginAs(t, app, author).Get(path), http.StatusOK)
	require.Contains(t, resp.Body, post.Body)
}

func TestVisibility_SharedPost(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	anon := app.Client(t)

	author := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityConnections))

	t.Run("share of a draft", func(t *testing.T) {
		draft := newPost(t, app, author.ID)
		draftShare, err := factory.PostShare(ctx, app.DB, draft.ID)
		require.NoError(t, err)

		requireStatus(t, anon.Get("/shared/"+draftShare.ID), http.StatusNotFound)
	})

	t.Run("unknown share", func(t *testing.T) {
		requireStatus(t, anon.Get("/shared/"+unknownID), http.StatusNotFound)
	})
}

// TestVisibility_Explore: explore lists published public posts of public and
// registered_users profiles to logged-in users; an anonymous visitor is sent to /.
func TestVisibility_Explore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	pubAuthor := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	regAuthor := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityRegisteredUsers))
	connAuthor := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityConnections))
	viewer := newUser(t, app)

	pubPost := newPost(t, app, pubAuthor.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	regPost := newPost(t, app, regAuthor.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	connPost := newPost(t, app, connAuthor.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	secondDegree := newPost(t, app, pubAuthor.ID, factory.Published(), factory.Visibility(model.PostVisibilitySecondDegree))
	draft := newPost(t, app, pubAuthor.ID, factory.Visibility(model.PostVisibilityPublic))

	resp := requireStatus(t, app.Client(t).Get("/explore"), http.StatusFound)
	require.Equal(t, "/", resp.Location())

	userText := requireStatus(t, loginAs(t, app, viewer).Get("/explore"), http.StatusOK).Doc().Text()
	require.Contains(t, userText, lo.FromPtr(pubPost.Subject))
	require.Contains(t, userText, lo.FromPtr(regPost.Subject))
	require.NotContains(t, userText, lo.FromPtr(connPost.Subject))
	require.NotContains(t, userText, lo.FromPtr(secondDegree.Subject))
	require.NotContains(t, userText, lo.FromPtr(draft.Subject))
}

// TestVisibility_UserMediaSpecialFiles: the media route answers robots.txt itself
// and sends favicon.ico to the static favicon, with or without a class segment.
func TestVisibility_UserMediaSpecialFiles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	for _, tc := range []struct {
		path     string
		status   int
		body     string
		location string
	}{
		{path: "/user-media/robots.txt/thumb", status: http.StatusOK, body: "OK"},
		{path: "/user-media/favicon.ico/thumb", status: http.StatusMovedPermanently, location: "/static/static/favicon.ico"},
		{path: "/user-media/robots.txt", status: http.StatusOK, body: "OK"},
		{path: "/user-media/favicon.ico", status: http.StatusMovedPermanently, location: "/static/static/favicon.ico"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp := requireStatus(t, app.Client(t).Get(tc.path), tc.status)
			if tc.body != "" {
				require.Equal(t, tc.body, resp.Body)
			}

			if tc.location != "" {
				require.Equal(t, tc.location, resp.Location())
			}
		})
	}
}

func TestVisibility_Static(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	resp := requireStatus(t, c.Get("/static/main.css"), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css"), resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Body, "e2e stub")

	requireStatus(t, c.Get("/static/no-such-file.css"), http.StatusNotFound)
}

func TestVisibility_SecurityHeaders(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	post := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))

	c := app.Client(t)

	for _, path := range []string{"/", "/posts/" + post.ID, "/users/" + author.Username, "/login"} {
		resp := requireStatus(t, c.Get(path), http.StatusOK)

		csp := resp.Header.Get("Content-Security-Policy")
		require.Contains(t, csp, "default-src 'self'", path)
		require.Contains(t, csp, "frame-ancestors 'none'", path)
		require.Contains(t, csp, "script-src 'self'", path)
		require.Contains(t, csp, "'nonce-", path)
		require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), path)
	}
}
