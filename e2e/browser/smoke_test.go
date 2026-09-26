//go:build browser

package browser_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// Logging in through the real form lands on the feed, and boosted links swap
// the page in place: the window survives, the title changes, and the head is
// merged (the journal's RSS link comes and goes with the page).
func TestSmoke_LoginAndBoostedNavigation(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	page := browser.Page(t, app)
	rss := page.Locator(`head link[rel="alternate"][type="application/rss+xml"]`)

	_, err := page.Goto("/login")
	require.NoError(t, err)

	require.NoError(t, page.GetByLabel("Email address").Fill(user.Email))
	require.NoError(t, page.GetByLabel("Password").Fill(browser.Password))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log in"}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
	require.NoError(t, browser.Expect.Locator(rss).ToHaveCount(0))

	_, err = page.Evaluate(`window.smokeMarker = "kept"`)
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("navigation").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: user.Username, Exact: playwright.Bool(true)}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveTitle(regexp.MustCompile(`Journal$`)))
	require.NoError(t, browser.Expect.Locator(rss).ToHaveCount(1))

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Controls", Exact: playwright.Bool(true)}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/?$`)))
	require.NoError(t, browser.Expect.Page(page).ToHaveTitle(regexp.MustCompile(`Controls$`)))
	require.NoError(t, browser.Expect.Locator(rss).ToHaveCount(0))

	marker, err := page.Evaluate(`window.smokeMarker`)
	require.NoError(t, err)
	require.Equal(t, "kept", marker, "a boosted link reloaded the whole page")
}

// An action button posts its payload as JSON through htmx and reloads the
// page: deleting a draft on /controls after confirming removes it.
func TestSmoke_ActionButton(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	draft, err := factory.Post(context.Background(), app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err = page.Goto("/controls")
	require.NoError(t, err)

	row := page.GetByRole("row").Filter(playwright.LocatorFilterOptions{HasText: draft.Subject.String})
	require.NoError(t, row.GetByRole("button").Click())

	require.NoError(t, browser.Expect.Locator(row).ToHaveCount(0))

	posts, err := factory.ListPosts(context.Background(), app.DB, user.ID)
	require.NoError(t, err)
	require.Empty(t, posts)
}

// A server error on a boosted navigation shows the error toast.
func TestSmoke_ServerErrorShowsToast(t *testing.T) {
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

	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToContainText("Server error"))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
}
