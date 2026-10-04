//go:build browser

// What a first-time visitor downloads: the stylesheet, the fonts and the
// scripts of the feed and of a single post, each against a budget.
package browser_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"sync"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

const (
	// A reader waits for the stylesheet before the first paint, so it has to
	// stay small once compressed.
	maxCSSBytes = 25 * 1024
	// One page loads the weights of the Latin and Cyrillic faces its text
	// uses and nothing else; more means a face is loaded that no text needs.
	maxFontBytes = 70 * 1024
	// The production bundle (htmx, Stimulus, the editor, lite-youtube) was
	// 45.5 KB gzipped when the budget was set, plus 10% headroom. Growing
	// past it needs a reason.
	maxJSBytes = 50 * 1024
)

// gzipped is what the production server would send for body: the test server
// sends it as is, so it is compressed here.
func gzipped(t *testing.T, body []byte) int {
	t.Helper()

	var buf bytes.Buffer

	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	require.NoError(t, err)
	_, err = zw.Write(body)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Len()
}

// Opening the feed and a post in a fresh browser stays within the CSS, font
// and JS budgets. The feed and the post hold both a Russian and an English
// post, so the Latin and Cyrillic faces both load.
func TestFirstVisitWeight(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	russian, err := factory.Post(ctx, app.DB, user.ID, factory.Published(),
		factory.WithSubject("Привет, мир"), factory.WithBody("Первый пост на русском языке."))
	require.NoError(t, err)
	english, err := factory.Post(ctx, app.DB, user.ID, factory.Published(),
		factory.WithSubject("Hello, world"), factory.WithBody("The first post in English."))
	require.NoError(t, err)

	for _, path := range []string{"/feed", "/posts/" + russian.ID, "/posts/" + english.ID} {
		page := browser.Page(t, app, browser.As(user))

		var (
			mu        sync.Mutex
			responses = map[string][]playwright.Response{}
		)

		page.OnResponse(func(r playwright.Response) {
			mu.Lock()
			defer mu.Unlock()

			kind := r.Request().ResourceType()
			responses[kind] = append(responses[kind], r)
		})

		_, err := page.Goto(path, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle})
		require.NoError(t, err)
		// fonts are fetched once the text that needs them is laid out
		_, err = page.Evaluate("document.fonts.ready")
		require.NoError(t, err)
		require.NoError(t, page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle}))

		mu.Lock()

		var css, js, fonts int

		for _, r := range responses["stylesheet"] {
			body, err := r.Body()
			require.NoError(t, err)

			css += gzipped(t, body)
		}

		for _, r := range responses["script"] {
			body, err := r.Body()
			require.NoError(t, err)

			js += gzipped(t, body)
		}

		// woff2 is compressed already, so fonts count as sent
		for _, r := range responses["font"] {
			body, err := r.Body()
			require.NoError(t, err)

			fonts += len(body)
		}

		mu.Unlock()

		t.Logf("%s: css %d, fonts %d, js %d bytes", path, css, fonts, js)

		require.NotZero(t, css, path)
		require.NotZero(t, fonts, path)
		require.LessOrEqual(t, css, maxCSSBytes, "%s: stylesheet over budget", path)
		require.LessOrEqual(t, fonts, maxFontBytes, "%s: fonts over budget", path)
		require.LessOrEqual(t, js, maxJSBytes, "%s: scripts over budget", path)
	}
}
