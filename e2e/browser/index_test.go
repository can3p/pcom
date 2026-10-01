//go:build browser

// The index page: what an anonymous visitor sees on the way in.
package browser_test

import (
	"regexp"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// An anonymous visitor sees the public posts, opens one, finds the RSS link
// (icon and head) and the GitHub link, sees the anonymous menu, and logs in from it.
func TestIndex_AnonymousVisitor(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	author := browser.NewUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	post, err := factory.Post(t.Context(), app.DB, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	require.NoError(t, err)
	reader := browser.NewUser(t, app)

	page := browser.Page(t, app)
	_, err = page.Goto("/")
	require.NoError(t, err)

	rss := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "RSS feed"})
	require.NoError(t, browser.Expect.Locator(rss).ToHaveAttribute("href", "/rss/public"))
	require.NoError(t, browser.Expect.Locator(page.Locator(`head link[rel="alternate"]`)).ToHaveAttribute("href", "/rss/public"))

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: post.Subject.String}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+post.ID)))

	nav := page.GetByRole("navigation")
	github := nav.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "GitHub"})
	require.NoError(t, browser.Expect.Locator(github).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(github).ToHaveAttribute("href", "https://github.com/can3p/pcom"))

	menu := nav.Locator("ul.navbar-nav a")
	require.NoError(t, browser.Expect.Locator(menu).ToHaveText([]string{"Home", "Sign up", "Login"}))
	require.NoError(t, browser.Expect.Locator(menu.First()).ToHaveAttribute("href", "/"))
	require.NoError(t, browser.Expect.Locator(nav.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Explore"})).ToHaveCount(0))

	require.NoError(t, nav.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Login", Exact: new(true)}).Click())
	require.NoError(t, page.GetByLabel("Email address").Fill(reader.Email))
	require.NoError(t, page.GetByLabel("Password").Fill(browser.Password))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log in"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed`)))
}

// The menu's sign-up entry reaches the signup form.
func TestIndex_SignUpFromMenu(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	page := browser.Page(t, app)

	_, err := page.Goto("/")
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("navigation").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Sign up"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "New Account"})).ToBeVisible())
}

// The anonymous index shows the newest public posts; "Load more" appends the
// older ones in place.
func TestIndex_LoadMore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	author := browser.NewUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	publishPosts(t, app, author.ID, "Indexpost", core.PostVisibilityPublic, pagedPosts)

	expectLoadsMore(t, browser.Page(t, app), "/", "Indexpost")
}
