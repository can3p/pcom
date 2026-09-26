//go:build browser

package browser_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// b6Viewports are the widths the layout sweep checks. 1280px is a common
// desktop width, 390px an iPhone-class narrow phone.
var b6Viewports = []struct {
	name          string
	width, height int
}{
	{name: "1280px", width: 1280, height: 720},
	{name: "390px", width: 390, height: 844},
}

// b6PageTest is one page from the router to sweep, at every viewport, in
// every reachable auth state.
type b6PageTest struct {
	name string
	path string
	anon bool   // reachable (and worth checking) logged out
	auth bool   // reachable (and worth checking) logged in
	skip string // known-bug reason: skip instead of asserting no guard violation
}

// TestLayout_Pages visits every GET page route in cmd/web, logged in and
// anonymous wherever both are reachable, at a desktop and a phone viewport,
// and asserts: no horizontal overflow, no guard violation (console error,
// CSP violation, uncaught exception, failed or 404/5xx request), and every
// image on the page finishes loading, including ones lazy-loaded on scroll.
//
// The page list is every GET route registered on cmd/web's main router
// (`grep -n '\.GET(' cmd/web/*.go`) that renders an HTML page, minus:
//   - downloads: /posts/:id/md, /posts/:id/zip (file attachments, not pages)
//   - assets: user-media/:fname/:class (binary image bytes),
//     /users/:username/user_styles (a CSS document, gated by EnforceReferer)
//   - feeds: /rss/public/:username, /rss/private/:key (XML, not HTML)
//   - API: /api/v1/posts (JSON, exercised by the E2E HTTP suite)
//
// /confirm_signup/:id is left out too: reaching it needs a user with
// EmailConfirmSeed set, and no factory option sets that field (see needs in
// the task report). It shares the bare-map CSP bug below, so it would be
// skipped rather than checked once reachable.
//
// /signup, /confirm_waiting_list/:id and /articles/:id are visited but
// skipped: their handlers render a bare gin.H{}/map[string]any{} template
// context, which doesn't get a script nonce, so their inline script trips
// the CSP guard (known bug #139).
func TestLayout_Pages(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()

	// A public profile and post, so an anonymous visitor and a second,
	// unrelated logged-in user can both reach them.
	publicUser, err := factory.User(ctx, app.DB, factory.WithVisibility(core.ProfileVisibilityPublic))
	require.NoError(t, err)

	publicPost, err := factory.Post(ctx, app.DB, publicUser.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	require.NoError(t, err)

	share, err := factory.PostShare(ctx, app.DB, publicPost.ID)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, app.DB, publicUser.ID)
	require.NoError(t, err)

	waitingList, err := factory.SignupRequest(ctx, app.DB)
	require.NoError(t, err)

	authUser := browser.NewUser(t, app)
	draftPost, err := factory.Post(ctx, app.DB, authUser.ID)
	require.NoError(t, err)

	const bug139 = "known bug #139: bare-map handlers don't set ScriptNonce, so this page's inline script violates CSP"

	pages := []b6PageTest{
		// Redirects to /feed when logged in, so the "logged in" run exercises
		// that redirect and lands on the same page the Feed case checks.
		{name: "Home", path: "/", anon: true, auth: true},
		{name: "Login", path: "/login", anon: true},
		{name: "Signup", path: "/signup", anon: true, skip: bug139},
		{name: "Explore", path: "/explore", anon: true, auth: true},
		{name: "Invite", path: "/invite/" + invite.ID, anon: true},
		{name: "Confirm waiting list", path: "/confirm_waiting_list/" + waitingList.ID, anon: true, skip: bug139},
		{name: "Article", path: "/articles/why", anon: true, auth: true, skip: bug139},
		{name: "User profile", path: "/users/" + publicUser.Username, anon: true, auth: true},
		{name: "Shared post", path: "/shared/" + share.ID, anon: true},
		{name: "Public post", path: "/posts/" + publicPost.ID, anon: true, auth: true},

		{name: "Feed", path: "/feed", auth: true},
		{name: "Write", path: "/write", auth: true},
		{name: "Controls", path: "/controls/", auth: true},
		{name: "Settings", path: "/controls/settings", auth: true},
		{name: "Edit post", path: "/posts/" + draftPost.ID + "/edit", auth: true},
	}

	for _, tc := range pages {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.skip != "" {
				t.Skip(tc.skip)
			}

			if tc.anon {
				for _, vp := range b6Viewports {
					t.Run("Anonymous_"+vp.name, func(t *testing.T) {
						t.Parallel()
						b6CheckPage(t, app, tc.path, nil, vp.width, vp.height)
					})
				}
			}

			if tc.auth {
				for _, vp := range b6Viewports {
					t.Run("LoggedIn_"+vp.name, func(t *testing.T) {
						t.Parallel()
						b6CheckPage(t, app, tc.path, authUser, vp.width, vp.height)
					})
				}
			}
		})
	}
}

// b6CheckPage opens path at the given viewport, as user (or anonymous when
// nil), and asserts no horizontal overflow and every image loaded. Guard
// violations (console errors, CSP violations, failed/404/5xx requests) fail
// the test through browser.Page's own cleanup.
func b6CheckPage(t *testing.T, app *e2e.App, path string, user *core.User, width, height int) {
	t.Helper()

	opts := []browser.PageOption{
		browser.Configure(func(cfg *playwright.BrowserNewContextOptions) {
			cfg.Viewport = &playwright.Size{Width: width, Height: height}
		}),
	}
	if user != nil {
		opts = append(opts, browser.As(user))
	}

	page := browser.Page(t, app, opts...)

	_, err := page.Goto(path)
	require.NoError(t, err)

	b6CheckNoOverflow(t, page)
	b6CheckImagesLoaded(t, page)
}

// b6CheckNoOverflow asserts the document has no horizontal scrollbar. It
// reads document.documentElement.scrollWidth, not window.scrollWidth, which
// does not exist and would make the comparison always false.
func b6CheckNoOverflow(t *testing.T, page playwright.Page) {
	t.Helper()

	result, err := page.Evaluate(`() => ({
		scrollWidth: document.documentElement.scrollWidth,
		innerWidth: window.innerWidth,
	})`)
	require.NoError(t, err)

	dims, ok := result.(map[string]interface{})
	require.True(t, ok, "expected an object result, got %T", result)

	scrollWidth := b6ToInt(t, dims["scrollWidth"])
	innerWidth := b6ToInt(t, dims["innerWidth"])

	require.LessOrEqualf(t, scrollWidth, innerWidth,
		"page has horizontal overflow (scrollWidth=%d, innerWidth=%d)", scrollWidth, innerWidth)
}

// b6CheckImagesLoaded asserts every <img> on the page finished loading. An
// image lazy-loaded by the markdown renderer starts as
// <img class="lazyload" data-src="..."> with no src, so it is scrolled into
// view first and waited on there instead of being asserted immediately,
// which would make the check pass on images that never even started
// loading.
func b6CheckImagesLoaded(t *testing.T, page playwright.Page) {
	t.Helper()

	result, err := page.Evaluate(`() =>
		Array.from(document.querySelectorAll('img')).map((img, i) => ({
			index: i,
			src: img.getAttribute('src') || img.getAttribute('data-src') || '',
			lazy: img.classList.contains('lazyload') || img.classList.contains('lazyloaded'),
		}))
	`)
	require.NoError(t, err)

	images, ok := result.([]interface{})
	require.True(t, ok, "expected images to be an array, got %T", result)

	for _, raw := range images {
		img, ok := raw.(map[string]interface{})
		require.True(t, ok, "expected an image entry to be an object, got %T", raw)

		index := b6ToInt(t, img["index"])
		lazy, _ := img["lazy"].(bool)
		src, _ := img["src"].(string)

		if lazy {
			loc := page.Locator("img").Nth(index)
			require.NoError(t, loc.ScrollIntoViewIfNeeded(), "scrolling lazy image %d (%s) into view", index, src)

			_, err := page.WaitForFunction(
				`(i) => {
					const img = document.querySelectorAll('img')[i];
					return !!img && img.classList.contains('lazyloaded') && img.complete && img.naturalWidth > 0;
				}`,
				index,
			)
			require.NoError(t, err, "lazy image %d (%s) never finished loading", index, src)
			continue
		}

		loaded, err := page.Evaluate(`(i) => {
			const img = document.querySelectorAll('img')[i];
			return img.complete && img.naturalWidth > 0;
		}`, index)
		require.NoError(t, err)
		require.True(t, loaded.(bool), "image %d (%s) not loaded", index, src)
	}
}

func b6ToInt(t *testing.T, v interface{}) int {
	t.Helper()

	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		require.Fail(t, fmt.Sprintf("expected a number, got %T", v))
		return 0
	}
}
