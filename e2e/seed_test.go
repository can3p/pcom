package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/seed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateErrorMarkers are what a half-rendered Go template leaves in a page.
var templateErrorMarkers = []string{"template:", "executing \"", "<no value>", "%!"}

// requireRendered fails when body is not a complete page or carries a template error marker.
func requireRendered(t *testing.T, label, body string) {
	t.Helper()

	for _, m := range templateErrorMarkers {
		assert.NotContains(t, body, m, "%s: template error marker", label)
	}

	assert.Contains(t, strings.ToLower(body), "</html>", "%s: page is cut short", label)
}

// TestSeed_EveryPageRenders seeds the harness database, logs in as each
// seeded user and fetches every navigable page: it is a broad net for
// template and data-shape regressions.
func TestSeed_EveryPageRenders(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	require.NoError(t, seed.Run(ctx, app.DB.DB, &strings.Builder{}, seed.Options{
	}))

	ids := map[string]string{}

	for _, name := range seed.Usernames() {
		var id string

		require.NoError(t, app.DB.QueryRow(`SELECT id FROM users WHERE username = $1`, name).Scan(&id))

		ids[name] = id
	}

	type post struct{ id, author string }

	var posts []post

	for _, name := range seed.Usernames() {
		ps, err := factory.ListPosts(ctx, app.DB, ids[name])
		require.NoError(t, err)

		for _, p := range ps {
			posts = append(posts, post{p.ID, name})
		}
	}

	require.NotEmpty(t, posts)

	pages := []string{"/", "/feed", "/explore", "/controls/", "/controls/settings", "/write"}
	checked := 0

	for _, name := range seed.Usernames() {
		t.Run(name, func(t *testing.T) {
			c := app.Client(t)
			c.LoginAs(name+"@example.test", seed.Password)

			for _, path := range pages {
				resp := c.Get(path)
				require.Less(t, resp.StatusCode, 400, path)

				// Logged in, "/" redirects to the feed; everything else renders.
				if path == "/" {
					require.Equal(t, http.StatusFound, resp.StatusCode, path)
				} else {
					require.Equal(t, http.StatusOK, resp.StatusCode, path)
					requireRendered(t, path, resp.Body)
				}

				checked++
			}

			for _, other := range seed.Usernames() {
				path := "/users/" + other
				resp := c.Get(path)
				// A profile the viewer may not see is a 404, never an error.
				require.Contains(t, []int{http.StatusOK, http.StatusFound, http.StatusNotFound}, resp.StatusCode, path)

				if resp.StatusCode == http.StatusOK {
					requireRendered(t, path, resp.Body)
				}

				if other == name {
					require.Equal(t, http.StatusOK, resp.StatusCode, "own profile "+path)
				}

				checked++
			}

			for _, p := range posts {
				path := fmt.Sprintf("/posts/%s", p.id)
				resp := c.Get(path)

				if p.author == name {
					require.Equal(t, http.StatusOK, resp.StatusCode, "own post "+path)
				} else {
					require.Contains(t, []int{http.StatusOK, http.StatusFound, http.StatusNotFound}, resp.StatusCode, path)
				}

				if resp.StatusCode == http.StatusOK {
					requireRendered(t, path, resp.Body)
				}

				checked++
			}
		})
	}

	t.Logf("checked %d (user, page) pairs", checked)
}
