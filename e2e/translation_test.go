package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// htmxGet is a GET the way htmx sends it.
func htmxGet(t *testing.T, app *e2e.App, c *e2e.Client, path string) *e2e.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, app.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("HX-Request", "true")

	return c.Do(req)
}

// The translation routes answer 404 for what the reader may not translate
// or what isn't a translation request, and say so when the budget is spent.
func TestTranslationRoutes(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithWireMock(), e2e.WithEnv("TRANSLATION_USER_DAILY_CHARS", "5"))
	ctx := context.Background()
	author := newUser(t, app)
	reader, client := newLoggedIn(t, app)

	german := []factory.PostOpt{factory.Published(), factory.WithLanguage("de"), factory.AllowTranslation()}

	direct, err := factory.Post(ctx, app.DB, author.ID, german...)
	require.NoError(t, err)
	draft, err := factory.Post(ctx, app.DB, reader.ID, factory.WithLanguage("de"), factory.AllowTranslation())
	require.NoError(t, err)
	public, err := factory.Post(ctx, app.DB, author.ID, append(german, factory.Visibility(core.PostVisibilityPublic))...)
	require.NoError(t, err)

	feed, err := factory.RSSFeed(ctx, app.DB)
	require.NoError(t, err)
	item, err := factory.RSSItem(ctx, app.DB, feed.ID, factory.WithItemLanguage("de"))
	require.NoError(t, err)

	for _, tc := range []struct{ name, path string }{
		{"a stranger's direct-only post", "/controls/translate/post/" + direct.ID},
		{"a draft", "/controls/translate/post/" + draft.ID},
		{"an unsubscribed RSS item", "/controls/translate/rss_item/" + item.ID},
		{"an unknown kind", "/controls/translate/comment/" + public.ID},
	} {
		for _, suffix := range []string{"", "/original"} {
			t.Run(tc.name+suffix, func(t *testing.T) {
				htmxGet(t, app, client, tc.path+suffix).RequireStatus(http.StatusNotFound)
			})
		}
	}

	t.Run("not htmx", func(t *testing.T) {
		for _, suffix := range []string{"", "/original"} {
			client.Get("/controls/translate/post/" + public.ID + suffix).RequireStatus(http.StatusNotFound)
		}
	})

	t.Run("a malformed ID", func(t *testing.T) {
		htmxGet(t, app, client, "/controls/translate/post/"+uuid.NewString()[:8]).RequireStatus(http.StatusNotFound)
	})

	t.Run("over the budget", func(t *testing.T) {
		resp := htmxGet(t, app, client, "/controls/translate/post/"+public.ID).RequireStatus(http.StatusOK)
		require.Contains(t, resp.Body, "Translation is not available right now")
		require.NotContains(t, resp.Body, "Automatically translated")
	})
}
