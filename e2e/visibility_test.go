package e2e_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

const (
	chromeUA  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
	firefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"

	// unknownID is a well-formed id that no row has.
	unknownID = "00000000-0000-0000-0000-000000000000"

	issue110 = "known bug: https://github.com/can3p/pcom/issues/110"
)

func newPost(t *testing.T, app *e2e.App, authorID string, opts ...factory.PostOpt) *core.Post {
	t.Helper()

	p, err := factory.Post(context.Background(), app.DB, authorID, opts...)
	require.NoError(t, err)

	return p
}

func connectUsers(t *testing.T, app *e2e.App, a, b *core.User) {
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
func requirePostPage(t *testing.T, resp *e2e.Response, p *core.Post) {
	t.Helper()

	requireStatus(t, resp, http.StatusOK)
	require.Contains(t, resp.Doc().Find("h1.us-post-header").Text(), p.Subject.String)
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

func TestVisibility_Index(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := newUser(t, app)

	resp := requireStatus(t, app.Client(t).Get("/"), http.StatusOK)
	require.Empty(t, resp.Location())
	require.Equal(t, 1, resp.Doc().Find(`a[href="/signup?attribution=index_page"]`).Length())

	resp = requireStatus(t, loginAs(t, app, user).Get("/"), http.StatusFound)
	require.Equal(t, "/feed", resp.Location())
}

func TestVisibility_Articles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	raw, err := os.ReadFile("../cmd/web/client/articles/why.md")
	require.NoError(t, err)
	title, _, _ := strings.Cut(string(raw), "\n")

	resp := requireStatus(t, c.Get("/articles/why"), http.StatusOK)
	require.Contains(t, resp.Doc().Text(), strings.TrimSpace(title))

	requireStatus(t, c.Get("/articles/terms_of_service"), http.StatusOK)

	for _, path := range []string{
		"/articles/does_not_exist",
		"/articles/Why",
		"/articles/bad-name",
		"/articles/_why",
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

	authors := map[core.ProfileVisibility]*core.User{}
	for _, v := range []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers, core.ProfileVisibilityConnections} {
		authors[v] = newUser(t, app, factory.WithVisibility(v))
		connectUsers(t, app, authors[v], direct)
	}

	cases := []struct {
		profile core.ProfileVisibility
		viewer  string
		want    int
	}{
		{core.ProfileVisibilityPublic, "anonymous", http.StatusOK},
		{core.ProfileVisibilityPublic, "unrelated", http.StatusOK},
		{core.ProfileVisibilityPublic, "direct", http.StatusOK},
		{core.ProfileVisibilityRegisteredUsers, "anonymous", http.StatusNotFound},
		{core.ProfileVisibilityRegisteredUsers, "unrelated", http.StatusOK},
		{core.ProfileVisibilityRegisteredUsers, "direct", http.StatusOK},
		{core.ProfileVisibilityConnections, "anonymous", http.StatusNotFound},
		{core.ProfileVisibilityConnections, "unrelated", http.StatusNotFound},
		{core.ProfileVisibilityConnections, "direct", http.StatusOK},
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

	author := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	direct := newUser(t, app)
	connectUsers(t, app, author, direct)

	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	directOnly := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	draft := newPost(t, app, author.ID, factory.Visibility(core.PostVisibilityPublic))

	anonText := requireStatus(t, app.Client(t).Get("/users/"+author.Username), http.StatusOK).Doc().Text()
	require.Contains(t, anonText, public.Subject.String)
	require.NotContains(t, anonText, directOnly.Subject.String)
	require.NotContains(t, anonText, draft.Subject.String)

	directText := requireStatus(t, loginAs(t, app, direct).Get("/users/"+author.Username), http.StatusOK).Doc().Text()
	require.Contains(t, directText, public.Subject.String)
	require.Contains(t, directText, directOnly.Subject.String)
	require.NotContains(t, directText, draft.Subject.String)
}

// TestVisibility_PublicRSS: only public profiles have a public feed, and it carries
// only published public posts.
func TestVisibility_PublicRSS(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	author := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	directOnly := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	draft := newPost(t, app, author.ID, factory.Visibility(core.PostVisibilityPublic))

	resp := requireStatus(t, c.Get("/rss/public/"+author.Username), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/xml"), resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Body, "<rss")
	require.Contains(t, resp.Body, public.Subject.String)
	require.NotContains(t, resp.Body, directOnly.Subject.String)
	require.NotContains(t, resp.Body, draft.Subject.String)

	for _, v := range []core.ProfileVisibility{core.ProfileVisibilityRegisteredUsers, core.ProfileVisibilityConnections} {
		hidden := newUser(t, app, factory.WithVisibility(v))
		newPost(t, app, hidden.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))

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
	profile core.ProfileVisibility
	post    core.PostVisibility
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
// (docs/open-questions.md, 2026-09-28).
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

	authorClients := map[core.ProfileVisibility]*e2e.Client{}
	posts := map[postKey]*core.Post{}

	for _, pv := range []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers, core.ProfileVisibilityConnections} {
		author := newUser(t, app, factory.WithVisibility(pv))
		connectUsers(t, app, author, direct)
		authorClients[pv] = loginAs(t, app, author)

		for _, v := range []core.PostVisibility{core.PostVisibilityPublic, core.PostVisibilitySecondDegree, core.PostVisibilityDirectOnly} {
			posts[postKey{pv, v, asPublished}] = newPost(t, app, author.ID, factory.Visibility(v), factory.Published())
			posts[postKey{pv, v, asDraft}] = newPost(t, app, author.ID, factory.Visibility(v))
		}
	}

	cases := []struct {
		viewer  viewerKind
		profile core.ProfileVisibility
		post    core.PostVisibility
		draft   bool
		want    outcome
	}{
		// Published posts
		// public profile
		{viewerAnon, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAnon, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asPublished, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asPublished, outcomeNeedsLogin},
		{viewerUnrelated, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerUnrelated, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asPublished, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerSecond, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerSecond, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerDirect, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerDirect, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		// registered profile
		{viewerAnon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asPublished, outcomeVisible}, // a public post is public whatever the profile visibility
		{viewerAnon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asPublished, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asPublished, outcomeNeedsLogin},
		{viewerUnrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerUnrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asPublished, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerSecond, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerSecond, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerDirect, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerDirect, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		// connections profile
		{viewerAnon, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asPublished, outcomeVisible}, // a public post is public whatever the profile visibility
		{viewerAnon, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asPublished, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asPublished, outcomeNeedsLogin},
		{viewerUnrelated, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asPublished, outcomeVisible}, // a public post is public whatever the profile visibility
		{viewerUnrelated, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asPublished, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerSecond, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerSecond, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asPublished, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerDirect, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerDirect, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asPublished, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asPublished, outcomeVisible},
		// Draft posts
		// public profile
		{viewerAnon, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asDraft, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asDraft, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asDraft, outcomeNeedsLogin},
		{viewerUnrelated, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerAuthor, core.ProfileVisibilityPublic, core.PostVisibilityPublic, asDraft, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, asDraft, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, asDraft, outcomeVisible},
		// registered profile
		{viewerAnon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asDraft, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asDraft, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asDraft, outcomeNeedsLogin},
		{viewerUnrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerAuthor, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, asDraft, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, asDraft, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, asDraft, outcomeVisible},
		// connections profile
		{viewerAnon, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asDraft, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asDraft, outcomeNeedsLogin},
		{viewerAnon, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asDraft, outcomeNeedsLogin},
		{viewerUnrelated, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerUnrelated, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerSecond, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asDraft, outcomeNotFound},
		{viewerDirect, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asDraft, outcomeNotFound},
		{viewerAuthor, core.ProfileVisibilityConnections, core.PostVisibilityPublic, asDraft, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, asDraft, outcomeVisible},
		{viewerAuthor, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, asDraft, outcomeVisible},
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

	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	directOnly := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))

	resp := requireStatus(t, loginAs(t, app, author).Get("/posts/"+directOnly.ID+"/md"), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain"), resp.Header.Get("Content-Type"))
	require.True(t, strings.HasPrefix(resp.Body, "---\n"), resp.Body)
	require.Contains(t, resp.Body, directOnly.ID)
	require.Contains(t, resp.Body, directOnly.Subject.String)
	require.Contains(t, resp.Body, directOnly.Body)

	anon := app.Client(t)
	resp = requireStatus(t, anon.Get("/posts/"+public.ID+"/md"), http.StatusOK)
	require.Contains(t, resp.Body, public.Body)

	requireLoginRedirect(t, anon.Get("/posts/"+directOnly.ID+"/md"), "/posts/"+directOnly.ID+"/md")
	requireStatus(t, loginAs(t, app, unrelated).Get("/posts/"+directOnly.ID+"/md"), http.StatusNotFound)
	requireStatus(t, anon.Get("/posts/"+unknownID+"/md"), http.StatusNotFound)
}

func TestVisibility_PostZipAuthor(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	post := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	newPost(t, app, author.ID, factory.Published())

	resp := requireStatus(t, loginAs(t, app, author).Get("/posts/"+post.ID+"/zip"), http.StatusOK)
	require.Equal(t, "application/zip", resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
	require.Equal(t, []string{post.ID + ".md"}, zipNames(t, resp.Body))
}

func TestVisibility_PostZipAccessGuards(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	unrelated := newUser(t, app)
	directOnly := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))

	path := "/posts/" + directOnly.ID + "/zip"
	requireLoginRedirect(t, app.Client(t).Get(path), path)
	requireStatus(t, loginAs(t, app, unrelated).Get(path), http.StatusNotFound)
}

// requireZipEitherAnswer asserts what must hold regardless of how Q7
// (docs/open-questions.md) resolves whether zip export is author-only: never
// a 5xx, and either a 200 whose zip holds exactly post's markdown, or a 404
// or a login redirect to path that shut a non-author out.
func requireZipEitherAnswer(t *testing.T, resp *e2e.Response, path string, post *core.Post) {
	t.Helper()

	require.Less(t, resp.StatusCode, 500, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		require.Equal(t, []string{post.ID + ".md"}, zipNames(t, resp.Body))
	case http.StatusFound:
		requireLoginRedirect(t, resp, path)
	default:
		require.Equal(t, http.StatusNotFound, resp.StatusCode, resp.Body)
	}
}

// TestVisibility_PostZipAnonymousPublic: whether an anonymous visitor may zip-export a
// public post is undecided (Q7, docs/open-questions.md); this pins only what
// holds under either answer.
func TestVisibility_PostZipAnonymousPublic(t *testing.T) {
	t.Skip(issue110)
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	path := "/posts/" + public.ID + "/zip"

	requireZipEitherAnswer(t, app.Client(t).Get(path), path, public)
}

// TestVisibility_PostZipDirectConnection: whether a direct connection (not the
// author) may zip-export a private post is undecided (Q7,
// docs/open-questions.md); this pins only what holds under either answer.
func TestVisibility_PostZipDirectConnection(t *testing.T) {
	t.Skip(issue110)
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	direct := newUser(t, app)
	connectUsers(t, app, author, direct)
	post := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	path := "/posts/" + post.ID + "/zip"

	requireZipEitherAnswer(t, loginAs(t, app, direct).Get(path), path, post)
}

func TestVisibility_PostEdit(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app)
	direct := newUser(t, app)
	unrelated := newUser(t, app)
	connectUsers(t, app, author, direct)

	post := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
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

	author := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityConnections))

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

// TestVisibility_Explore: explore lists published public posts of public profiles to
// everyone, and of registered_users profiles to logged-in users only.
func TestVisibility_Explore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	pubAuthor := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	regAuthor := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityRegisteredUsers))
	connAuthor := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityConnections))
	viewer := newUser(t, app)

	pubPost := newPost(t, app, pubAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	regPost := newPost(t, app, regAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	connPost := newPost(t, app, connAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	secondDegree := newPost(t, app, pubAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilitySecondDegree))
	draft := newPost(t, app, pubAuthor.ID, factory.Visibility(core.PostVisibilityPublic))

	anonText := requireStatus(t, app.Client(t).Get("/explore"), http.StatusOK).Doc().Text()
	require.Contains(t, anonText, pubPost.Subject.String)
	require.NotContains(t, anonText, regPost.Subject.String)
	require.NotContains(t, anonText, connPost.Subject.String)
	require.NotContains(t, anonText, secondDegree.Subject.String)
	require.NotContains(t, anonText, draft.Subject.String)

	userText := requireStatus(t, loginAs(t, app, viewer).Get("/explore"), http.StatusOK).Doc().Text()
	require.Contains(t, userText, pubPost.Subject.String)
	require.Contains(t, userText, regPost.Subject.String)
	require.NotContains(t, userText, connPost.Subject.String)
	require.NotContains(t, userText, secondDegree.Subject.String)
	require.NotContains(t, userText, draft.Subject.String)
}

// TestVisibility_UserMediaSpecialFiles: the media route answers robots.txt itself
// and sends favicon.ico to the static favicon. The route is
// /user-media/:fname/:class, so today both need a class segment.
func TestVisibility_UserMediaSpecialFiles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	resp := requireStatus(t, c.Get("/user-media/robots.txt/thumb"), http.StatusOK)
	require.Equal(t, "OK", resp.Body)

	resp = requireStatus(t, c.Get("/user-media/favicon.ico/thumb"), http.StatusMovedPermanently)
	require.Equal(t, "/static/static/favicon.ico", resp.Location())
}

// TestVisibility_UserMediaSpecialFilesAtRoot: robots.txt and favicon.ico are asked
// for without a class segment, which the route does not match.
func TestVisibility_UserMediaSpecialFilesAtRoot(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/151")
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	resp := requireStatus(t, c.Get("/user-media/robots.txt"), http.StatusOK)
	require.Equal(t, "OK", resp.Body)

	resp = requireStatus(t, c.Get("/user-media/favicon.ico"), http.StatusMovedPermanently)
	require.Equal(t, "/static/static/favicon.ico", resp.Location())
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

	author := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	post := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))

	c := app.Client(t)

	for _, path := range []string{"/", "/posts/" + post.ID, "/users/" + author.Username, "/explore", "/login"} {
		resp := requireStatus(t, c.Get(path), http.StatusOK)

		csp := resp.Header.Get("Content-Security-Policy")
		require.Contains(t, csp, "default-src 'self'", path)
		require.Contains(t, csp, "frame-ancestors 'none'", path)
		require.Contains(t, csp, "script-src 'self'", path)
		require.Contains(t, csp, "'nonce-", path)
		require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), path)
	}
}
