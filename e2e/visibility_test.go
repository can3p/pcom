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
	e1Password = "secret-pw"

	e1ChromeUA  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
	e1FirefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"

	// e1UnknownID is a well-formed id that no row has.
	e1UnknownID = "00000000-0000-0000-0000-000000000000"

	e1Issue110 = "known bug: https://github.com/can3p/pcom/issues/110"
	// e1DraftBug marks D2a cells where a draft is served to someone other
	// than its author: shared links, the author's journal and create_share
	// all treat drafts as private to the author, /posts/:id does not.
	e1DraftBug = "known bug: /posts/:id serves a draft to non-authors that the post's visibility radius admits"
)

// e1User creates a user who can log in with e1Password.
func e1User(t *testing.T, app *e2e.App, opts ...factory.UserOpt) *core.User {
	t.Helper()

	u, err := factory.User(context.Background(), app.DB, append([]factory.UserOpt{factory.WithPassword(e1Password)}, opts...)...)
	require.NoError(t, err)

	return u
}

// e1Login returns a client logged in as u.
func e1Login(t *testing.T, app *e2e.App, u *core.User) *e2e.Client {
	t.Helper()

	c := app.Client(t)
	c.LoginAs(u.Email, e1Password)

	return c
}

func e1Post(t *testing.T, app *e2e.App, authorID string, opts ...factory.PostOpt) *core.Post {
	t.Helper()

	p, err := factory.Post(context.Background(), app.DB, authorID, opts...)
	require.NoError(t, err)

	return p
}

func e1Connect(t *testing.T, app *e2e.App, a, b *core.User) {
	t.Helper()

	_, _, err := factory.Connect(context.Background(), app.DB, a.ID, b.ID)
	require.NoError(t, err)
}

// e1GetWith requests path with extra request headers.
func e1GetWith(t *testing.T, app *e2e.App, c *e2e.Client, path string, headers map[string]string) *e2e.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, app.URL+path, nil)
	require.NoError(t, err)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.Do(req)
}

// e1Status asserts resp's status on t. Response.RequireStatus fails the
// test that created the client, which is wrong inside a subtest.
func e1Status(t *testing.T, resp *e2e.Response, code int) *e2e.Response {
	t.Helper()
	require.Equal(t, code, resp.StatusCode, resp.Body)

	return resp
}

// e1RequireLoginRedirect asserts a redirect to the login page that brings
// the visitor back to path.
func e1RequireLoginRedirect(t *testing.T, resp *e2e.Response, path string) {
	t.Helper()

	e1Status(t, resp, http.StatusFound)

	loc, err := url.Parse(resp.Location())
	require.NoError(t, err)
	require.Equal(t, "/login", loc.Path)
	require.Equal(t, path, loc.Query().Get("return_url"))
	require.NotEmpty(t, loc.Query().Get("sign"))
}

// e1RequirePostPage asserts the single post page of p.
func e1RequirePostPage(t *testing.T, resp *e2e.Response, p *core.Post) {
	t.Helper()

	e1Status(t, resp, http.StatusOK)
	require.Contains(t, resp.Doc().Find("h1.us-post-header").Text(), p.Subject.String)
}

func e1ZipNames(t *testing.T, body string) []string {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	require.NoError(t, err)

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}

	return names
}

func TestE1Index(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := e1User(t, app)

	resp := e1Status(t, app.Client(t).Get("/"), http.StatusOK)
	require.Empty(t, resp.Location())
	require.Equal(t, 1, resp.Doc().Find(`a[href="/signup?attribution=index_page"]`).Length())

	resp = e1Status(t, e1Login(t, app, user).Get("/"), http.StatusFound)
	require.Equal(t, "/feed", resp.Location())
}

func TestE1Articles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	raw, err := os.ReadFile("../cmd/web/client/articles/why.md")
	require.NoError(t, err)
	title, _, _ := strings.Cut(string(raw), "\n")

	resp := e1Status(t, c.Get("/articles/why"), http.StatusOK)
	require.Contains(t, resp.Doc().Text(), strings.TrimSpace(title))

	e1Status(t, c.Get("/articles/terms_of_service"), http.StatusOK)

	for _, path := range []string{
		"/articles/does_not_exist",
		"/articles/Why",
		"/articles/bad-name",
		"/articles/_why",
		"/articles/..%2Fhtml%2Findex",
	} {
		e1Status(t, c.Get(path), http.StatusNotFound)
	}
}

// TestE1UserHome: a journal is visible to anyone for a public profile, to
// logged-in users for registered_users, and to connections only for
// connections; otherwise it does not exist (404).
func TestE1UserHome(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	unrelated := e1User(t, app)
	direct := e1User(t, app)

	clients := map[string]*e2e.Client{
		"anonymous": app.Client(t),
		"unrelated": e1Login(t, app, unrelated),
		"direct":    e1Login(t, app, direct),
	}

	authors := map[core.ProfileVisibility]*core.User{}
	for _, v := range []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers, core.ProfileVisibilityConnections} {
		authors[v] = e1User(t, app, factory.WithVisibility(v))
		e1Connect(t, app, authors[v], direct)
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
			resp := e1Status(t, clients[tc.viewer].Get("/users/"+author.Username), tc.want)

			if tc.want == http.StatusOK {
				require.Contains(t, resp.Doc().Text(), author.Username)
			}
		})
	}

	e1Status(t, clients["unrelated"].Get("/users/no_such_user"), http.StatusNotFound)
}

// TestE1UserHomePostsByRadius: the journal lists published posts the
// viewer's radius admits.
func TestE1UserHomePostsByRadius(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	direct := e1User(t, app)
	e1Connect(t, app, author, direct)

	public := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	directOnly := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	draft := e1Post(t, app, author.ID, factory.Visibility(core.PostVisibilityPublic))

	anonText := e1Status(t, app.Client(t).Get("/users/"+author.Username), http.StatusOK).Doc().Text()
	require.Contains(t, anonText, public.Subject.String)
	require.NotContains(t, anonText, directOnly.Subject.String)
	require.NotContains(t, anonText, draft.Subject.String)

	directText := e1Status(t, e1Login(t, app, direct).Get("/users/"+author.Username), http.StatusOK).Doc().Text()
	require.Contains(t, directText, public.Subject.String)
	require.Contains(t, directText, directOnly.Subject.String)
	require.NotContains(t, directText, draft.Subject.String)
}

// TestE1PublicRSS: only public profiles have a public feed, and it carries
// only published public posts.
func TestE1PublicRSS(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	author := e1User(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	public := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	directOnly := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	draft := e1Post(t, app, author.ID, factory.Visibility(core.PostVisibilityPublic))

	resp := e1Status(t, c.Get("/rss/public/"+author.Username), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/xml"), resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Body, "<rss")
	require.Contains(t, resp.Body, public.Subject.String)
	require.NotContains(t, resp.Body, directOnly.Subject.String)
	require.NotContains(t, resp.Body, draft.Subject.String)

	for _, v := range []core.ProfileVisibility{core.ProfileVisibilityRegisteredUsers, core.ProfileVisibilityConnections} {
		hidden := e1User(t, app, factory.WithVisibility(v))
		e1Post(t, app, hidden.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))

		e1Status(t, c.Get("/rss/public/"+hidden.Username), http.StatusNotFound)
	}

	e1Status(t, c.Get("/rss/public/no_such_user"), http.StatusNotFound)
}

func TestE1UserStyles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	const css = ".post { color: red; }"

	styled := e1User(t, app)
	_, err := factory.UserStyle(context.Background(), app.DB, styled.ID, css)
	require.NoError(t, err)

	plain := e1User(t, app)

	path := "/users/" + styled.Username + "/user_styles"
	referer := app.URL + "/users/" + styled.Username

	t.Run("no referer", func(t *testing.T) {
		e1Status(t, e1GetWith(t, app, c, path, map[string]string{"User-Agent": e1ChromeUA}), http.StatusNotFound)
	})

	t.Run("foreign referer", func(t *testing.T) {
		e1Status(t, e1GetWith(t, app, c, path, map[string]string{"User-Agent": e1ChromeUA, "Referer": "https://evil.example/" + styled.Username}), http.StatusNotFound)
	})

	t.Run("non-Firefox gets @scope", func(t *testing.T) {
		resp := e1Status(t, e1GetWith(t, app, c, path, map[string]string{"User-Agent": e1ChromeUA, "Referer": referer}), http.StatusOK)
		require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css"), resp.Header.Get("Content-Type"))
		require.True(t, strings.HasPrefix(resp.Body, "@scope (.user-styles-applied) {"), resp.Body)
		require.Contains(t, resp.Body, css)
		require.True(t, strings.HasSuffix(strings.TrimSpace(resp.Body), "}"), resp.Body)
	})

	t.Run("Firefox gets the css as is", func(t *testing.T) {
		resp := e1Status(t, e1GetWith(t, app, c, path, map[string]string{"User-Agent": e1FirefoxUA, "Referer": referer}), http.StatusOK)
		require.Equal(t, css, resp.Body)
	})

	t.Run("user without styles", func(t *testing.T) {
		resp := e1Status(t, e1GetWith(t, app, c, "/users/"+plain.Username+"/user_styles", map[string]string{"User-Agent": e1ChromeUA, "Referer": referer}), http.StatusOK)
		require.Empty(t, resp.Body)
	})

	t.Run("unknown user", func(t *testing.T) {
		resp := e1Status(t, e1GetWith(t, app, c, "/users/no_such_user/user_styles", map[string]string{"User-Agent": e1ChromeUA, "Referer": referer}), http.StatusOK)
		require.Empty(t, resp.Body)
	})
}

type e1Viewer string

const (
	e1Anon      e1Viewer = "anonymous"
	e1Unrelated e1Viewer = "registered-unrelated"
	e1Second    e1Viewer = "second-degree"
	e1Direct    e1Viewer = "direct"
	e1Author    e1Viewer = "author"
)

type e1Outcome string

const (
	e1Visible    e1Outcome = "visible"
	e1NeedsLogin e1Outcome = "needs-login"
	e1NotFound   e1Outcome = "not-found"
)

const (
	e1Published = false
	e1Draft     = true
)

type e1PostKey struct {
	profile core.ProfileVisibility
	post    core.PostVisibility
	draft   bool
}

// TestE1SinglePostMatrix is the D2a matrix at the HTTP level: who can open
// /posts/:id. A post the viewer may not see is a login redirect for an
// anonymous visitor and a 404 for everyone else, so its existence isn't
// revealed. The visibility radius decides: public for anyone, second_degree
// for connections of connections, direct_only for direct connections; the
// author sees everything. A draft is the author's alone.
//
// The profile's visibility plays no part at /posts/:id (unlike the journal,
// RSS and explore): a public post is visible to everybody, by decision
// (docs/open-questions.md, 2026-09-28).
func TestE1SinglePostMatrix(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	direct := e1User(t, app)
	second := e1User(t, app)
	unrelated := e1User(t, app)
	e1Connect(t, app, second, direct)

	clients := map[e1Viewer]*e2e.Client{
		e1Anon:      app.Client(t),
		e1Unrelated: e1Login(t, app, unrelated),
		e1Second:    e1Login(t, app, second),
		e1Direct:    e1Login(t, app, direct),
	}

	authorClients := map[core.ProfileVisibility]*e2e.Client{}
	posts := map[e1PostKey]*core.Post{}

	for _, pv := range []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers, core.ProfileVisibilityConnections} {
		author := e1User(t, app, factory.WithVisibility(pv))
		e1Connect(t, app, author, direct)
		authorClients[pv] = e1Login(t, app, author)

		for _, v := range []core.PostVisibility{core.PostVisibilityPublic, core.PostVisibilitySecondDegree, core.PostVisibilityDirectOnly} {
			posts[e1PostKey{pv, v, e1Published}] = e1Post(t, app, author.ID, factory.Visibility(v), factory.Published())
			posts[e1PostKey{pv, v, e1Draft}] = e1Post(t, app, author.ID, factory.Visibility(v))
		}
	}

	cases := []struct {
		viewer  e1Viewer
		profile core.ProfileVisibility
		post    core.PostVisibility
		draft   bool
		want    e1Outcome
		skip    string
	}{
		// Published posts
		// public profile
		{e1Anon, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Anon, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Published, e1NeedsLogin, ""},
		{e1Anon, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Published, e1NeedsLogin, ""},
		{e1Unrelated, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Unrelated, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Published, e1NotFound, ""},
		{e1Unrelated, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Published, e1NotFound, ""},
		{e1Second, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Second, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Second, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Published, e1NotFound, ""},
		{e1Direct, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Direct, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Direct, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Published, e1Visible, ""},
		// registered profile
		{e1Anon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Published, e1Visible, ""}, // a public post is public whatever the profile visibility
		{e1Anon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Published, e1NeedsLogin, ""},
		{e1Anon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Published, e1NeedsLogin, ""},
		{e1Unrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Unrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Published, e1NotFound, ""},
		{e1Unrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Published, e1NotFound, ""},
		{e1Second, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Second, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Second, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Published, e1NotFound, ""},
		{e1Direct, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Direct, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Direct, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Published, e1Visible, ""},
		// connections profile
		{e1Anon, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Published, e1Visible, ""}, // a public post is public whatever the profile visibility
		{e1Anon, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Published, e1NeedsLogin, ""},
		{e1Anon, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Published, e1NeedsLogin, ""},
		{e1Unrelated, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Published, e1Visible, ""}, // a public post is public whatever the profile visibility
		{e1Unrelated, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Published, e1NotFound, ""},
		{e1Unrelated, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Published, e1NotFound, ""},
		{e1Second, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Second, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Second, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Published, e1NotFound, ""},
		{e1Direct, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Direct, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Direct, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Published, e1Visible, ""},
		{e1Author, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Published, e1Visible, ""},
		// Draft posts
		// public profile
		{e1Anon, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Draft, e1NeedsLogin, e1DraftBug},
		{e1Anon, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Draft, e1NeedsLogin, ""},
		{e1Anon, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Draft, e1NeedsLogin, ""},
		{e1Unrelated, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Unrelated, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, ""},
		{e1Unrelated, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, ""},
		{e1Second, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Second, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, e1DraftBug},
		{e1Second, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, ""},
		{e1Direct, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Direct, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, e1DraftBug},
		{e1Direct, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, e1DraftBug},
		{e1Author, core.ProfileVisibilityPublic, core.PostVisibilityPublic, e1Draft, e1Visible, ""},
		{e1Author, core.ProfileVisibilityPublic, core.PostVisibilitySecondDegree, e1Draft, e1Visible, ""},
		{e1Author, core.ProfileVisibilityPublic, core.PostVisibilityDirectOnly, e1Draft, e1Visible, ""},
		// registered profile
		{e1Anon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Draft, e1NeedsLogin, e1DraftBug},
		{e1Anon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Draft, e1NeedsLogin, ""},
		{e1Anon, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Draft, e1NeedsLogin, ""},
		{e1Unrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Unrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, ""},
		{e1Unrelated, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, ""},
		{e1Second, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Second, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, e1DraftBug},
		{e1Second, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, ""},
		{e1Direct, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Direct, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, e1DraftBug},
		{e1Direct, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, e1DraftBug},
		{e1Author, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityPublic, e1Draft, e1Visible, ""},
		{e1Author, core.ProfileVisibilityRegisteredUsers, core.PostVisibilitySecondDegree, e1Draft, e1Visible, ""},
		{e1Author, core.ProfileVisibilityRegisteredUsers, core.PostVisibilityDirectOnly, e1Draft, e1Visible, ""},
		// connections profile
		{e1Anon, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Draft, e1NeedsLogin, e1DraftBug},
		{e1Anon, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Draft, e1NeedsLogin, ""},
		{e1Anon, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Draft, e1NeedsLogin, ""},
		{e1Unrelated, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Unrelated, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, ""},
		{e1Unrelated, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, ""},
		{e1Second, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Second, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, e1DraftBug},
		{e1Second, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, ""},
		{e1Direct, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Draft, e1NotFound, e1DraftBug},
		{e1Direct, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Draft, e1NotFound, e1DraftBug},
		{e1Direct, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Draft, e1NotFound, e1DraftBug},
		{e1Author, core.ProfileVisibilityConnections, core.PostVisibilityPublic, e1Draft, e1Visible, ""},
		{e1Author, core.ProfileVisibilityConnections, core.PostVisibilitySecondDegree, e1Draft, e1Visible, ""},
		{e1Author, core.ProfileVisibilityConnections, core.PostVisibilityDirectOnly, e1Draft, e1Visible, ""},
	}

	for _, tc := range cases {
		state := "published"
		if tc.draft {
			state = "draft"
		}

		t.Run(fmt.Sprintf("%s/%s-profile/%s-post/%s", tc.viewer, tc.profile, tc.post, state), func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}

			post := posts[e1PostKey{tc.profile, tc.post, tc.draft}]
			path := "/posts/" + post.ID

			client := clients[tc.viewer]
			if tc.viewer == e1Author {
				client = authorClients[tc.profile]
			}

			resp := client.Get(path)

			switch tc.want {
			case e1Visible:
				e1RequirePostPage(t, resp, post)
			case e1NeedsLogin:
				e1RequireLoginRedirect(t, resp, path)
			case e1NotFound:
				e1Status(t, resp, http.StatusNotFound)
			}
		})
	}

	t.Run("unknown post", func(t *testing.T) {
		e1Status(t, clients[e1Unrelated].Get("/posts/"+e1UnknownID), http.StatusNotFound)
	})
}

func TestE1PostMarkdown(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app)
	unrelated := e1User(t, app)

	public := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	directOnly := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))

	resp := e1Status(t, e1Login(t, app, author).Get("/posts/"+directOnly.ID+"/md"), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain"), resp.Header.Get("Content-Type"))
	require.True(t, strings.HasPrefix(resp.Body, "---\n"), resp.Body)
	require.Contains(t, resp.Body, directOnly.ID)
	require.Contains(t, resp.Body, directOnly.Subject.String)
	require.Contains(t, resp.Body, directOnly.Body)

	anon := app.Client(t)
	resp = e1Status(t, anon.Get("/posts/"+public.ID+"/md"), http.StatusOK)
	require.Contains(t, resp.Body, public.Body)

	e1RequireLoginRedirect(t, anon.Get("/posts/"+directOnly.ID+"/md"), "/posts/"+directOnly.ID+"/md")
	e1Status(t, e1Login(t, app, unrelated).Get("/posts/"+directOnly.ID+"/md"), http.StatusNotFound)
	e1Status(t, anon.Get("/posts/"+e1UnknownID+"/md"), http.StatusNotFound)
}

func TestE1PostZipAuthor(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app)
	post := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	e1Post(t, app, author.ID, factory.Published())

	resp := e1Status(t, e1Login(t, app, author).Get("/posts/"+post.ID+"/zip"), http.StatusOK)
	require.Equal(t, "application/zip", resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
	require.Equal(t, []string{post.ID + ".md"}, e1ZipNames(t, resp.Body))
}

func TestE1PostZipAccessGuards(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app)
	unrelated := e1User(t, app)
	directOnly := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))

	path := "/posts/" + directOnly.ID + "/zip"
	e1RequireLoginRedirect(t, app.Client(t).Get(path), path)
	e1Status(t, e1Login(t, app, unrelated).Get(path), http.StatusNotFound)
}

// e1RequireZipEitherAnswer asserts what must hold regardless of how Q7
// (docs/open-questions.md) resolves whether zip export is author-only: never
// a 5xx, and either a 200 whose zip holds exactly post's markdown, or a 404
// or a login redirect to path that shut a non-author out.
func e1RequireZipEitherAnswer(t *testing.T, resp *e2e.Response, path string, post *core.Post) {
	t.Helper()

	require.Less(t, resp.StatusCode, 500, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		require.Equal(t, []string{post.ID + ".md"}, e1ZipNames(t, resp.Body))
	case http.StatusFound:
		e1RequireLoginRedirect(t, resp, path)
	default:
		require.Equal(t, http.StatusNotFound, resp.StatusCode, resp.Body)
	}
}

// TestE1PostZipAnonymousPublic: whether an anonymous visitor may zip-export a
// public post is undecided (Q7, docs/open-questions.md); this pins only what
// holds under either answer.
func TestE1PostZipAnonymousPublic(t *testing.T) {
	t.Skip(e1Issue110)
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app)
	public := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	path := "/posts/" + public.ID + "/zip"

	e1RequireZipEitherAnswer(t, app.Client(t).Get(path), path, public)
}

// TestE1PostZipDirectConnection: whether a direct connection (not the
// author) may zip-export a private post is undecided (Q7,
// docs/open-questions.md); this pins only what holds under either answer.
func TestE1PostZipDirectConnection(t *testing.T) {
	t.Skip(e1Issue110)
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app)
	direct := e1User(t, app)
	e1Connect(t, app, author, direct)
	post := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))
	path := "/posts/" + post.ID + "/zip"

	e1RequireZipEitherAnswer(t, e1Login(t, app, direct).Get(path), path, post)
}

func TestE1PostEdit(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app)
	direct := e1User(t, app)
	unrelated := e1User(t, app)
	e1Connect(t, app, author, direct)

	post := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	path := "/posts/" + post.ID + "/edit"

	e1RequireLoginRedirect(t, app.Client(t).Get(path), path)
	e1Status(t, e1Login(t, app, direct).Get(path), http.StatusForbidden)
	e1Status(t, e1Login(t, app, unrelated).Get(path), http.StatusForbidden)

	resp := e1Status(t, e1Login(t, app, author).Get(path), http.StatusOK)
	require.Contains(t, resp.Body, post.Body)
}

func TestE1SharedPost(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	anon := app.Client(t)

	author := e1User(t, app, factory.WithVisibility(core.ProfileVisibilityConnections))

	t.Run("share of a draft", func(t *testing.T) {
		draft := e1Post(t, app, author.ID)
		draftShare, err := factory.PostShare(ctx, app.DB, draft.ID)
		require.NoError(t, err)

		e1Status(t, anon.Get("/shared/"+draftShare.ID), http.StatusNotFound)
	})

	t.Run("unknown share", func(t *testing.T) {
		e1Status(t, anon.Get("/shared/"+e1UnknownID), http.StatusNotFound)
	})
}

// TestE1Explore: explore lists published public posts of public profiles to
// everyone, and of registered_users profiles to logged-in users only.
func TestE1Explore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	pubAuthor := e1User(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	regAuthor := e1User(t, app, factory.WithVisibility(core.ProfileVisibilityRegisteredUsers))
	connAuthor := e1User(t, app, factory.WithVisibility(core.ProfileVisibilityConnections))
	viewer := e1User(t, app)

	pubPost := e1Post(t, app, pubAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	regPost := e1Post(t, app, regAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	connPost := e1Post(t, app, connAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	secondDegree := e1Post(t, app, pubAuthor.ID, factory.Published(), factory.Visibility(core.PostVisibilitySecondDegree))
	draft := e1Post(t, app, pubAuthor.ID, factory.Visibility(core.PostVisibilityPublic))

	anonText := e1Status(t, app.Client(t).Get("/explore"), http.StatusOK).Doc().Text()
	require.Contains(t, anonText, pubPost.Subject.String)
	require.NotContains(t, anonText, regPost.Subject.String)
	require.NotContains(t, anonText, connPost.Subject.String)
	require.NotContains(t, anonText, secondDegree.Subject.String)
	require.NotContains(t, anonText, draft.Subject.String)

	userText := e1Status(t, e1Login(t, app, viewer).Get("/explore"), http.StatusOK).Doc().Text()
	require.Contains(t, userText, pubPost.Subject.String)
	require.Contains(t, userText, regPost.Subject.String)
	require.NotContains(t, userText, connPost.Subject.String)
	require.NotContains(t, userText, secondDegree.Subject.String)
	require.NotContains(t, userText, draft.Subject.String)
}

// TestE1UserMediaSpecialFiles: the media route answers robots.txt itself
// and sends favicon.ico to the static favicon. The route is
// /user-media/:fname/:class, so today both need a class segment.
func TestE1UserMediaSpecialFiles(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	resp := e1Status(t, c.Get("/user-media/robots.txt/thumb"), http.StatusOK)
	require.Equal(t, "OK", resp.Body)

	resp = e1Status(t, c.Get("/user-media/favicon.ico/thumb"), http.StatusMovedPermanently)
	require.Equal(t, "/static/static/favicon.ico", resp.Location())
}

// TestE1UserMediaSpecialFilesAtRoot: robots.txt and favicon.ico are asked
// for without a class segment, which the route does not match.
func TestE1UserMediaSpecialFilesAtRoot(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/151")
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	resp := e1Status(t, c.Get("/user-media/robots.txt"), http.StatusOK)
	require.Equal(t, "OK", resp.Body)

	resp = e1Status(t, c.Get("/user-media/favicon.ico"), http.StatusMovedPermanently)
	require.Equal(t, "/static/static/favicon.ico", resp.Location())
}

func TestE1Static(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	c := app.Client(t)

	resp := e1Status(t, c.Get("/static/main.css"), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css"), resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Body, "e2e stub")

	e1Status(t, c.Get("/static/no-such-file.css"), http.StatusNotFound)
}

func TestE1SecurityHeaders(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := e1User(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	post := e1Post(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))

	c := app.Client(t)

	for _, path := range []string{"/", "/posts/" + post.ID, "/users/" + author.Username, "/explore", "/login"} {
		resp := e1Status(t, c.Get(path), http.StatusOK)

		csp := resp.Header.Get("Content-Security-Policy")
		require.Contains(t, csp, "default-src 'self'", path)
		require.Contains(t, csp, "frame-ancestors 'none'", path)
		require.Contains(t, csp, "script-src 'self'", path)
		require.Contains(t, csp, "'nonce-", path)
		require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), path)
	}
}
