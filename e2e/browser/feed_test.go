//go:build browser

// RSS items in the feed: dismissing one after confirming removes it for good,
// and cancelling the confirmation keeps it.
package browser_test

import (
	"context"
	"testing"
	"time"

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
