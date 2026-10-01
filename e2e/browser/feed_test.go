//go:build browser

// RSS items in the feed: dismissing one after confirming removes it for good,
// and cancelling the confirmation keeps it.
package browser_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// feedWithTwoItems subscribes a new user to a feed with two items in their
// feed and returns the user and the items.
func feedWithTwoItems(t *testing.T, app *e2e.App) (*core.User, *core.RSSItem, *core.RSSItem) {
	t.Helper()

	ctx := context.Background()
	user := browser.NewUser(t, app)

	feed, err := factory.RSSFeed(ctx, app.DB)
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, app.DB, user.ID, feed.ID)
	require.NoError(t, err)

	var items []*core.RSSItem
	for range 2 {
		item, err := factory.RSSItem(ctx, app.DB, feed.ID)
		require.NoError(t, err)
		_, err = factory.UserFeedItem(ctx, app.DB, user.ID, item.ID)
		require.NoError(t, err)
		items = append(items, item)
	}

	return user, items[0], items[1]
}

func feedItem(page playwright.Page, item *core.RSSItem) playwright.Locator {
	return page.Locator(".us-feed-rss-item").Filter(playwright.LocatorFilterOptions{HasText: item.Title})
}

// Dismissing an item after confirming removes it from the feed without a
// reload, and it stays gone on the next visit; the other item stays.
func TestFeed_DismissRSSItem(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user, dismissed, kept := feedWithTwoItems(t, app)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err := page.Goto("/feed")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(feedItem(page, dismissed)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(feedItem(page, kept)).ToBeVisible())

	_, err = page.Evaluate(`window.feedMarker = "kept"`)
	require.NoError(t, err)

	require.NoError(t, feedItem(page, dismissed).GetByRole("button").Click())
	require.NoError(t, browser.Expect.Locator(feedItem(page, dismissed)).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(feedItem(page, kept)).ToBeVisible())

	marker, err := page.Evaluate(`window.feedMarker`)
	require.NoError(t, err)
	require.Equal(t, "kept", marker, "dismissing an item reloaded the page")

	_, err = page.Goto("/feed")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(feedItem(page, kept)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(feedItem(page, dismissed)).ToHaveCount(0))
}

// Cancelling the confirmation keeps the item, also on the next visit.
func TestFeed_DismissRSSItemCancelled(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user, item, _ := feedWithTwoItems(t, app)

	page := browser.Page(t, app, browser.As(user))
	asked := make(chan string, 1)
	page.OnDialog(func(d playwright.Dialog) {
		asked <- d.Message()
		_ = d.Dismiss()
	})

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	require.NoError(t, feedItem(page, item).GetByRole("button").Click())

	select {
	case msg := <-asked:
		require.Contains(t, msg, "remove the item from the feed")
	case <-time.After(5 * time.Second):
		t.Fatal("dismissing did not ask for confirmation")
	}

	require.NoError(t, browser.Expect.Locator(feedItem(page, item)).ToBeVisible())

	_, err = page.Goto("/feed")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(feedItem(page, item)).ToBeVisible())
}

const pagedPosts = 35 // more than one page (reading.PageSize is 30)

// publishPosts publishes n posts by author, oldest first, with the subjects
// "<prefix> 01", "<prefix> 02", ...; the lowest numbers end up on the last page.
func publishPosts(t *testing.T, app *e2e.App, authorID, prefix string, vis core.PostVisibility, n int) {
	t.Helper()

	for i := 1; i <= n; i++ {
		_, err := factory.Post(context.Background(), app.DB, authorID, factory.Published(),
			factory.Visibility(vis), factory.WithSubject(fmt.Sprintf("%s %02d", prefix, i)))
		require.NoError(t, err)
	}
}

// expectLoadsMore checks a list of pagedPosts posts, newest first: the newest
// is on the first page, "Load more" appends the older ones once each, and the
// button is gone on the last page.
func expectLoadsMore(t *testing.T, page playwright.Page, path, prefix string) {
	t.Helper()

	subject := func(i int) playwright.Locator {
		return page.GetByRole("link", playwright.PageGetByRoleOptions{Name: fmt.Sprintf("%s %02d", prefix, i), Exact: playwright.Bool(true)})
	}
	more := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Load more"})

	_, err := page.Goto(path)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(subject(pagedPosts)).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(subject(1)).ToHaveCount(0))

	_, err = page.Evaluate(`window.feedMarker = "kept"`)
	require.NoError(t, err)

	require.NoError(t, more.Click())
	require.NoError(t, browser.Expect.Locator(subject(1)).ToHaveCount(1))
	for _, i := range []int{pagedPosts, pagedPosts - 1, 6, 5, 1} {
		require.NoError(t, browser.Expect.Locator(subject(i)).ToHaveCount(1))
	}
	require.NoError(t, browser.Expect.Locator(more).ToHaveCount(0))

	// the older posts were appended to the list, not a second page pasted in
	for _, sel := range []string{"nav.navbar", "h1.us-user-header", ".us-user-home"} {
		require.NoError(t, browser.Expect.Locator(page.Locator(sel)).ToHaveCount(1), sel)
	}

	marker, err := page.Evaluate(`window.feedMarker`)
	require.NoError(t, err)
	require.Equal(t, "kept", marker, "loading more reloaded the page")
}

// The feed shows the newest page; "Load more" appends the older posts once
// each and the button disappears on the last page.
func TestFeed_LoadMore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	author := browser.NewUser(t, app)
	_, _, err := factory.Connect(context.Background(), app.DB, user.ID, author.ID)
	require.NoError(t, err)
	publishPosts(t, app, author.ID, "Feedpost", core.PostVisibilityDirectOnly, pagedPosts)

	expectLoadsMore(t, browser.Page(t, app, browser.As(user)), "/feed", "Feedpost")
}

// Explore loads a second page the same way.
func TestExplore_LoadMore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	author := browser.NewUser(t, app)
	publishPosts(t, app, author.ID, "Explorepost", core.PostVisibilityPublic, pagedPosts)

	expectLoadsMore(t, browser.Page(t, app, browser.As(user)), "/explore", "Explorepost")
}

// The server answers a cursor request from htmx with the items alone and a
// plain one with the whole page from that cursor; a bad cursor is a 400.
func TestExplore_CursorPages(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := browser.NewUser(t, app)
	author := browser.NewUser(t, app)
	publishPosts(t, app, author.ID, "Explorepost", core.PostVisibilityPublic, pagedPosts)

	c := app.Client(t)
	c.LoginAs(user.Email, browser.Password)

	href, ok := c.Get("/explore").RequireStatus(http.StatusOK).Doc().Find("a.btn").FilterFunction(
		func(_ int, s *goquery.Selection) bool { return strings.TrimSpace(s.Text()) == "Load more" }).First().Attr("href")
	require.True(t, ok, "first page has no Load more link")

	req, err := http.NewRequest(http.MethodGet, app.URL+href, nil)
	require.NoError(t, err)
	req.Header.Set("HX-Request", "true")
	frag := c.Do(req).RequireStatus(http.StatusOK)
	require.NotContains(t, frag.Body, "<html")
	require.NotContains(t, frag.Body, "navbar")
	require.Contains(t, frag.Body, "Explorepost 01")
	require.NotContains(t, frag.Body, "Load more", "the last page has no button")

	full := c.Get(href).RequireStatus(http.StatusOK)
	require.Contains(t, full.Body, "<html")
	require.Contains(t, full.Body, "navbar")
	require.Contains(t, full.Body, "Explorepost 01")
	require.NotContains(t, full.Body, "Explorepost 35", "the page starts at the cursor")

	c.Get("/explore?cursor=garbage").RequireStatus(http.StatusBadRequest)
}
