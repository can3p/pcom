//go:build browser

package browser_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// b1NavLink locates a link by its accessible name inside the page's <nav>
// landmark, the way a user would find it in the top navigation.
func b1NavLink(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("navigation").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: name, Exact: playwright.Bool(true)})
}

// b1ExpectTitleSuffix waits for the page title to end with suffix, the way a
// user reads the browser tab after a boosted navigation swaps the page.
func b1ExpectTitleSuffix(t *testing.T, page playwright.Page, suffix string) {
	t.Helper()
	require.NoError(t, browser.Expect.Page(page).ToHaveTitle(regexp.MustCompile(regexp.QuoteMeta(suffix)+`$`)))
}

// Boosted links swap every top-level page in place (feed, explore, write,
// controls, settings): the title and the og:title meta in <head> follow the
// page, the window is never torn down, and the browser's back/forward
// history replays the same swaps instead of reloading.
func TestNavigation_BoostedTopNavAndHistory(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	ogTitle := page.Locator(`head meta[property="og:title"]`)

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	b1ExpectTitleSuffix(t, page, "Your Feed")
	require.NoError(t, browser.Expect.Locator(ogTitle).ToHaveAttribute("content", regexp.MustCompile(`Your Feed$`)))

	_, err = page.Evaluate(`window.b1Marker = "kept"`)
	require.NoError(t, err)

	steps := []struct {
		link  string
		title string
		path  string
	}{
		{"Explore", "Explore", `/explore/?$`},
		{"Write", "New Post", `/write/?$`},
		{"Controls", "Controls", `/controls/?$`},
		{"Settings", "Settings", `/controls/settings/?$`},
	}

	for _, s := range steps {
		require.NoError(t, b1NavLink(page, s.link).Click())

		require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(s.path)))
		b1ExpectTitleSuffix(t, page, s.title)
		require.NoError(t, browser.Expect.Locator(ogTitle).ToHaveAttribute("content", regexp.MustCompile(regexp.QuoteMeta(s.title)+`$`)))
	}

	marker, err := page.Evaluate(`window.b1Marker`)
	require.NoError(t, err)
	require.Equal(t, "kept", marker, "a boosted link reloaded the whole page")

	backTitles := []string{"Controls", "New Post", "Explore", "Your Feed"}
	for _, title := range backTitles {
		_, err := page.GoBack()
		require.NoError(t, err)
		b1ExpectTitleSuffix(t, page, title)
	}

	marker, err = page.Evaluate(`window.b1Marker`)
	require.NoError(t, err)
	require.Equal(t, "kept", marker, "navigating back reloaded the whole page")

	forwardTitles := []string{"Explore", "New Post"}
	for _, title := range forwardTitles {
		_, err := page.GoForward()
		require.NoError(t, err)
		b1ExpectTitleSuffix(t, page, title)
	}
}

// At a phone viewport the top navigation starts collapsed; the navbar
// toggler (the "collapse" Stimulus controller wrapping Bootstrap's Collapse)
// reveals it, and a link inside still navigates normally.
func TestNavigation_MobileMenuAt390(t *testing.T) {
	t.Parallel()
	t.Skip("known bug #140: clicking the navbar toggler (data-bs-toggle=\"collapse\") makes Bootstrap's Collapse component set an inline style attribute on #navbarNavDropdown for the show/hide transition; the page's CSP (style-src with a per-request nonce, no 'unsafe-inline') blocks that mutation, which the browser guard reports as a console.error - repro: browser.Page at a 390px viewport, page.Goto(\"/feed\"), click the \"Toggle navigation\" button")

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.Viewport = &playwright.Size{Width: 390, Height: 844}
	}))

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	feedLink := b1NavLink(page, "Feed")
	require.NoError(t, browser.Expect.Locator(feedLink).ToBeHidden())

	toggler := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Toggle navigation"})
	require.NoError(t, toggler.Click())

	require.NoError(t, browser.Expect.Locator(feedLink).ToBeVisible())

	settingsLink := b1NavLink(page, "Settings")
	require.NoError(t, settingsLink.Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/settings/?$`)))
	b1ExpectTitleSuffix(t, page, "Settings")
}

// Under an emulated dark color scheme, the page picks up dark styling from
// the `prefers-color-scheme` media query alone: no `data-bs-theme` override
// is written, yet the body's computed background comes from the dark-mode
// stylesheet. A page emulating light stays off that background.
func TestNavigation_DarkModeBackground(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	const darkBackground = "rgb(26, 26, 26)"

	darkPage := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.ColorScheme = playwright.ColorSchemeDark
	}))

	_, err := darkPage.Goto("/feed")
	require.NoError(t, err)

	theme, err := darkPage.Locator("html").GetAttribute("data-bs-theme")
	require.NoError(t, err)
	require.Empty(t, theme, "dark styling must come from prefers-color-scheme, not a manual data-bs-theme override")

	require.NoError(t, browser.Expect.Locator(darkPage.Locator("body")).ToHaveCSS("background-color", darkBackground))

	lightPage := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.ColorScheme = playwright.ColorSchemeLight
	}))

	_, err = lightPage.Goto("/feed")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(lightPage.Locator("body")).Not().ToHaveCSS("background-color", darkBackground))
}

// Saving the general settings form swaps in a success flash wrapped by the
// "auto-dismiss" controller, which fades it out and removes it on its own
// after a few seconds, with no further user action.
func TestNavigation_SettingsFlashAutoDismisses(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Settings"}).Click())

	flash := page.GetByRole("alert").Filter(playwright.LocatorFilterOptions{HasText: "Settings have been saved"})
	require.NoError(t, browser.Expect.Locator(flash).ToBeVisible())

	require.NoError(t, browser.Expect.Locator(flash).ToHaveCount(0, playwright.LocatorAssertionsToHaveCountOptions{
		Timeout: playwright.Float(6000),
	}))
}

// On a post's own page, the "toggle" controller opens and closes the
// comment form, and the "spoiler" controller reveals a hidden spoiler block
// from the post body.
func TestNavigation_CommentToggleAndSpoiler(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	post, err := factory.Post(context.Background(), app.DB, user.ID,
		factory.Published(),
		factory.Visibility(core.PostVisibilityPublic),
		func(p *core.Post) {
			p.Body = "Intro paragraph.\n\n{cut}\n\n{spoiler}\nSecret content here.\n{/spoiler}\n\n{/cut}\n\nOutro paragraph."
		},
	)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	spoilerSummary := page.Locator(".block-container-spoiler-summary")
	spoilerContent := page.Locator(".block-container-spoiler-content")
	require.NoError(t, browser.Expect.Locator(spoilerContent).ToBeHidden())

	require.NoError(t, spoilerSummary.Click())

	require.NoError(t, browser.Expect.Locator(spoilerContent).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(spoilerContent).ToContainText("Secret content here."))
	require.NoError(t, browser.Expect.Locator(spoilerSummary).ToBeHidden())

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "No Comments yet"}).Click())

	commentForm := page.Locator(fmt.Sprintf(`[id="post%s"]`, post.ID))
	require.NoError(t, browser.Expect.Locator(commentForm).ToBeVisible())

	require.NoError(t, commentForm.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click())
	require.NoError(t, browser.Expect.Locator(commentForm).ToBeHidden())
}

// A boosted navigation that hits a 500 shows the error toast (the "toast"
// and "toaster" controllers), and its close button dismisses it.
func TestNavigation_ServerErrorTogglesToastAndDismiss(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user), browser.Allow(`\b500\b`))

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	require.NoError(t, page.Route("**/controls**", func(r playwright.Route) {
		_ = r.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(500), Body: "boom"})
	}))

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Controls", Exact: playwright.Bool(true)}).Click())

	toast := page.GetByRole("alert")
	require.NoError(t, browser.Expect.Locator(toast).ToContainText("Server error"))

	require.NoError(t, toast.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click())
	require.NoError(t, browser.Expect.Locator(toast).ToHaveCount(0))
}

// A boosted navigation whose connection is dropped mid-flight (the request
// is aborted rather than answered) shows the network-error toast, and the
// page stays put since the navigation never completed.
func TestNavigation_DroppedConnectionShowsNetworkErrorToast(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user),
		browser.Allow(`ERR_FAILED`),
		// htmx logs its own htmx:afterRequest/htmx:sendError events through
		// console.error whenever a request fails at the network layer; that
		// is htmx working as designed for the dropped connection this test
		// causes on purpose, not an application error.
		browser.Allow(`htmx:(sendError|afterRequest)`),
	)

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	require.NoError(t, page.Route("**/controls**", func(r playwright.Route) {
		_ = r.Abort("failed")
	}))

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Controls", Exact: playwright.Bool(true)}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToContainText("Network error"))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
}
