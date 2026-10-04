//go:build browser

package browser_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	feedsvc "github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/google/uuid"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// b5PNGBytes returns a tiny valid PNG, for tests that upload an image
// through a file input.
func b5PNGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

// b5RSSTemplate is a minimal valid RSS 2.0 feed with one item, formatted
// with the serving httptest.Server's own URL (so the item's link is
// absolute), an item title and a pubDate.
const b5RSSTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>B5 Test Feed</title>
<link>%[1]s</link>
<description>Feed for browser tests</description>
<item>
<title>%[2]s</title>
<link>%[1]s/item1</link>
<guid>%[1]s/item1</guid>
<description>Item body for browser tests</description>
<pubDate>%[3]s</pubDate>
</item>
</channel></rss>`

// openSettings opens the settings page and marks the loaded document, so
// requireSamePage can tell a swap in place from a reload.
func openSettings(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	_, err = page.Evaluate(`() => { window.settingsLoaded = true }`)
	require.NoError(t, err)
}

// requireSamePage fails when the page was reloaded since openSettings.
func requireSamePage(t *testing.T, page playwright.Page) {
	t.Helper()

	same, err := page.Evaluate(`() => window.settingsLoaded === true`)
	require.NoError(t, err)
	require.Equal(t, true, same, "the page reloaded instead of swapping the form in place")
}

// section is the settings section with the given heading.
func section(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("region", playwright.PageGetByRoleOptions{Name: name, Exact: playwright.Bool(true)})
}

func saveButton(sec playwright.Locator, name string) playwright.Locator {
	return sec.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: name, Exact: playwright.Bool(true)})
}

// Saving the general settings reports "Saved" next to the button without a
// reload and stores the chosen profile visibility.
func TestSettings_GeneralSavesInPlace(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	openSettings(t, page)

	general := section(page, "General")
	_, err := general.GetByLabel("Who can see your journal").SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Public"}})
	require.NoError(t, err)
	require.NoError(t, saveButton(general, "Save").Click())

	require.NoError(t, browser.Expect.Locator(general.GetByRole("status")).ToHaveText("Saved"))
	require.NoError(t, browser.Expect.Locator(general.GetByLabel("Who can see your journal")).ToHaveValue("public"))
	requireSamePage(t, page)

	stored, err := factory.GetUser(context.Background(), app.DB, user.ID)
	require.NoError(t, err)
	require.Equal(t, core.ProfileVisibilityPublic, stored.ProfileVisibility)
}

// Styles over the limit are not saved and the field says why; valid styles
// report "Saved" in place and apply to the user's journal, scoped under
// .user-styles-applied.
func TestSettings_UserStylesSaveInPlace(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	openSettings(t, page)

	styles := section(page, "Custom styles")
	css := styles.GetByLabel("CSS")

	require.NoError(t, css.Fill(strings.Repeat("a", accounts.DefaultUserStylesMaxLength+1)))
	require.NoError(t, saveButton(styles, "Save").Click())
	require.NoError(t, browser.Expect.Locator(styles.GetByRole("status")).ToHaveText("Not saved. Fix the field above."))
	require.NoError(t, browser.Expect.Locator(css).ToHaveAttribute("aria-invalid", "true"))
	require.NoError(t, browser.Expect.Locator(styles.Locator(".field-error")).ToBeVisible())

	_, err := factory.GetUserStyle(context.Background(), app.DB, user.ID)
	require.Error(t, err, "rejected styles are not stored")

	require.NoError(t, css.Fill(".us-user-header { color: rgb(12, 34, 56); }"))
	require.NoError(t, saveButton(styles, "Save").Click())
	require.NoError(t, browser.Expect.Locator(styles.GetByRole("status")).ToHaveText("Saved"))
	require.NoError(t, browser.Expect.Locator(styles.Locator(".field-error")).ToHaveCount(0))
	requireSamePage(t, page)

	_, err = page.Goto("/users/" + user.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-user-header")).ToHaveCSS("color", "rgb(12, 34, 56)"))
}

// The About text is written in a box with the editor toolbar, saved in place,
// rendered as markdown on the journal for the owner and an allowed visitor,
// and clearing it removes the block.
func TestSettings_ProfileAboutShownOnJournal(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	visitor := browser.NewUser(t, app)
	_, _, err := factory.Connect(context.Background(), app.DB, user.ID, visitor.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	openSettings(t, page)

	profile := section(page, "Profile")
	require.NoError(t, browser.Expect.Locator(profile.GetByRole("toolbar", playwright.LocatorGetByRoleOptions{Name: "Formatting"})).ToBeVisible())

	require.NoError(t, profile.GetByLabel("About you").Fill("I like [the web](https://example.com/web)"))
	require.NoError(t, saveButton(profile, "Save").Click())
	require.NoError(t, browser.Expect.Locator(profile.GetByRole("status")).ToHaveText("Saved"))
	require.NoError(t, browser.Expect.Locator(profile.GetByLabel("About you")).ToHaveValue("I like [the web](https://example.com/web)"))
	requireSamePage(t, page)

	about := page.Locator(".us-profile-about")
	_, err = page.Goto("/users/" + user.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "About"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(about.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "the web"})).ToHaveAttribute("href", "https://example.com/web"))

	vpage := browser.Page(t, app, browser.As(visitor))
	_, err = vpage.Goto("/users/" + user.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(vpage.Locator(".us-profile-about").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "the web"})).ToBeVisible())

	openSettings(t, page)
	require.NoError(t, profile.GetByLabel("About you").Fill(""))
	require.NoError(t, saveButton(profile, "Save").Click())
	require.NoError(t, browser.Expect.Locator(profile.GetByRole("status")).ToHaveText("Saved"))
	requireSamePage(t, page)

	_, err = page.Goto("/users/" + user.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-profile-about")).ToHaveCount(0))
}

// Generating an API key swaps the section in place: the button gives way to
// the hidden key and its Copy button, and the key is stored. (Copying is
// TestSettings_CopyAPIKey; the clipboard is shared by parallel tests.)
func TestSettings_GenerateAPIKey(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	openSettings(t, page)

	api := section(page, "API key")
	require.NoError(t, api.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Generate an API key"}).Click())

	copyButton := api.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Copy"})
	require.NoError(t, browser.Expect.Locator(copyButton).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(api.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Generate an API key"})).ToHaveCount(0))
	requireSamePage(t, page)

	keys, err := factory.ListAPIKeys(context.Background(), app.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1)
}

// The Copy button next to an existing API key copies the key.
func TestSettings_CopyAPIKey(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	key, err := factory.APIKey(context.Background(), app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.Permissions = []string{"clipboard-read", "clipboard-write"}
	}))
	openSettings(t, page)

	require.NoError(t, browser.Expect.Locator(page.GetByText(key.APIKey)).ToHaveCount(0), "the key is not shown")
	require.NoError(t, section(page, "API key").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Copy"}).Click())

	copied, err := page.Evaluate(`() => navigator.clipboard.readText()`)
	require.NoError(t, err)
	require.Equal(t, key.APIKey, copied)
}

// Creating the private feed URL shows, in place, a URL that serves the feed;
// making a new one asks first and shows the replacement, which serves it too.
func TestSettings_PrivateFeedURL(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	asked := 0
	page.OnDialog(func(d playwright.Dialog) {
		asked++
		_ = d.Accept()
	})
	openSettings(t, page)

	feed := section(page, "Private feed")
	url := feed.Locator("code")
	require.NoError(t, feed.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Create feed URL"}).Click())
	require.NoError(t, browser.Expect.Locator(url).ToContainText("/rss/private/"))

	first := requireShownFeedToken(t, app, user, url)

	resp, err := page.Request().Get(first)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status())

	require.NoError(t, feed.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Make a new feed URL"}).Click())
	require.NoError(t, browser.Expect.Locator(url).Not().ToHaveText(first))
	require.Equal(t, 1, asked, "making a new URL asks first")
	requireSamePage(t, page)

	second := requireShownFeedToken(t, app, user, url)

	resp, err = page.Request().Get(second)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status())
}

// requireShownFeedToken returns the private feed URL the page shows and
// requires it to end with the token stored for user.
func requireShownFeedToken(t *testing.T, app *e2e.App, user *core.User, url playwright.Locator) string {
	t.Helper()

	shown, err := url.TextContent()
	require.NoError(t, err)

	token, err := repo.FeedTokenForUser(context.Background(), app.DB, user.ID)
	require.NoError(t, err)
	require.NotNil(t, token)
	require.True(t, strings.HasSuffix(shown, "/"+token.Token), "shown %s, stored %s", shown, token.Token)

	return shown
}

// Removing a feed asks first, then re-renders the feeds section in place
// without it and unsubscribes; the other feed stays.
func TestSettings_RemoveFeedInPlace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	var feedIDs []string
	for _, title := range []string{"Kept feed", "Removed feed"} {
		feed, err := factory.RSSFeed(ctx, app.DB, factory.WithFeedTitle(title), factory.NextFetchAt(time.Now().Add(time.Hour)))
		require.NoError(t, err)
		_, err = factory.Subscription(ctx, app.DB, user.ID, feed.ID)
		require.NoError(t, err)
		feedIDs = append(feedIDs, feed.ID)
	}

	page := browser.Page(t, app, browser.As(user))
	asked := 0
	page.OnDialog(func(d playwright.Dialog) {
		asked++
		_ = d.Accept()
	})
	openSettings(t, page)

	list := section(page, "RSS feeds").GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Your feeds"})
	removed := list.GetByRole("listitem").Filter(playwright.LocatorFilterOptions{HasText: "Removed feed"})
	require.NoError(t, removed.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Remove"}).Click())

	require.NoError(t, browser.Expect.Locator(removed).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(list.GetByRole("listitem")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(list.GetByRole("listitem")).ToContainText("Kept feed"))
	require.Equal(t, 1, asked, "removing asks first")
	requireSamePage(t, page)

	kept, err := factory.SubscriptionExists(ctx, app.DB, user.ID, feedIDs[0])
	require.NoError(t, err)
	require.True(t, kept)

	gone, err := factory.SubscriptionExists(ctx, app.DB, user.ID, feedIDs[1])
	require.NoError(t, err)
	require.False(t, gone)
}

// Sending an invite re-renders the invites section in place: the invitee is
// listed, the count goes down and the invitation is mailed. An address that
// already has an account is refused under the field.
func TestSettings_SendInviteInPlace(t *testing.T) {
	t.Parallel()

	// unique, since every test binary run shares one tommy
	inviteeEmail := "b5-invitee-" + uuid.NewString() + "@example.test"

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	member := browser.NewUser(t, app)
	_, err := factory.Invitation(context.Background(), app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	openSettings(t, page)

	invites := section(page, "Invites")
	email := invites.GetByLabel("Email address")
	require.NoError(t, browser.Expect.Locator(invites).ToContainText("You have 1 invite left."))

	require.NoError(t, email.Fill(member.Email))
	require.NoError(t, saveButton(invites, "Send invite").Click())
	require.NoError(t, browser.Expect.Locator(invites.Locator(".field-error")).ToContainText("already registered"))
	require.NoError(t, browser.Expect.Locator(invites.GetByRole("status")).ToHaveText("Not saved. Fix the field above."))

	require.NoError(t, email.Fill(inviteeEmail))
	require.NoError(t, saveButton(invites, "Send invite").Click())

	sent := invites.GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Sent invites"})
	require.NoError(t, browser.Expect.Locator(sent).ToContainText(inviteeEmail))
	require.NoError(t, browser.Expect.Locator(invites).ToContainText("You have no invites left."))
	require.NoError(t, browser.Expect.Locator(invites.GetByRole("status")).ToHaveText("Invite sent"))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Invites"})).ToHaveCount(1))
	requireSamePage(t, page)

	emails := app.Mails(t, inviteeEmail, func(m tommy.Mail) bool {
		return m.Subject == "Welcome to pcom" && strings.Contains(m.Text, app.URL+"/invite/")
	})
	require.Len(t, emails, 1)
}

// A URL without a protocol is refused under the field; a valid one re-renders
// the feeds section in place with the new feed listed and an empty form.
func TestSettings_AddFeedInPlace(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	openSettings(t, page)

	feeds := section(page, "RSS feeds")
	field := feeds.GetByLabel("Feed URL")

	require.NoError(t, field.Fill("example.org/feed.xml"))
	require.NoError(t, saveButton(feeds, "Add feed").Click())
	require.NoError(t, browser.Expect.Locator(feeds.Locator(".field-error")).ToHaveText("url should have http or https protocol"))
	require.NoError(t, browser.Expect.Locator(field).ToHaveAttribute("aria-invalid", "true"))
	require.Empty(t, subscribedURLs(t, app, user), "a refused URL is not stored")

	feedURL := srv.URL + "/feed.xml"
	require.NoError(t, field.Fill(feedURL))
	require.NoError(t, saveButton(feeds, "Add feed").Click())

	list := feeds.GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Your feeds"})
	require.NoError(t, browser.Expect.Locator(list.GetByRole("listitem")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(list.Locator(fmt.Sprintf("a[href='%s']", feedURL))).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(feeds.GetByRole("status")).ToHaveText("Feed added"))
	require.NoError(t, browser.Expect.Locator(feeds.GetByLabel("Feed URL")).ToHaveValue(""))
	require.NoError(t, browser.Expect.Locator(feeds.Locator(".field-error")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "RSS feeds"})).ToHaveCount(1))
	requireSamePage(t, page)
	require.Equal(t, []string{feedURL}, subscribedURLs(t, app, user))
}

// subscribedURLs are the URLs of the feeds user is subscribed to.
func subscribedURLs(t *testing.T, app *e2e.App, user *core.User) []string {
	t.Helper()

	subs, err := feedsvc.New(repo.New(app.DB), nil).Subscriptions(context.Background(), user)
	require.NoError(t, err)

	urls := []string{}
	for _, s := range subs {
		urls = append(urls, s.URL)
	}

	return urls
}

// A feed that was never fetched says so; once a fetch fails, the feed says
// "The last fetch failed" in the error color and keeps the error text in a
// disclosure.
func TestSettings_FeedFetchStates(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	later, err := factory.RSSFeed(ctx, app.DB, factory.WithFeedTitle("Later feed"), factory.NextFetchAt(time.Now().Add(time.Hour)))
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, app.DB, user.ID, later.ID)
	require.NoError(t, err)

	broken, err := factory.RSSFeed(ctx, app.DB, factory.WithFeedTitle("Broken feed"), factory.WithFeedURL(srv.URL+"/feed.xml"))
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, app.DB, user.ID, broken.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	openSettings(t, page)

	feedRow := func(title string) playwright.Locator {
		return section(page, "RSS feeds").GetByRole("listitem").Filter(playwright.LocatorFilterOptions{HasText: title})
	}

	require.NoError(t, browser.Expect.Locator(feedRow("Later feed")).ToContainText("Never fetched"))
	require.NoError(t, browser.Expect.Locator(feedRow("Later feed")).ToContainText("no posts yet"))

	// the background poller fetches the broken feed within its interval
	failed := feedRow("Broken feed").Locator("summary")
	require.Eventually(t, func() bool {
		if _, err := page.Goto("/controls/settings"); err != nil {
			return false
		}

		n, err := failed.Count()

		return err == nil && n == 1
	}, 25*time.Second, 500*time.Millisecond, "the broken feed was never fetched by the poller")

	require.NoError(t, browser.Expect.Locator(failed).ToHaveText("The last fetch failed"))
	require.NoError(t, browser.Expect.Locator(failed).ToHaveCSS("color", errColor(t, page)))

	detail := feedRow("Broken feed").Locator("details code")
	require.NoError(t, browser.Expect.Locator(detail).ToBeHidden())
	require.NoError(t, failed.Click())
	require.NoError(t, browser.Expect.Locator(detail).ToContainText("404"))
	require.NoError(t, browser.Expect.Locator(feedRow("Later feed").Locator("summary")).ToHaveCount(0))
}

// errColor is the computed color of the --err token on page.
func errColor(t *testing.T, page playwright.Page) string {
	t.Helper()

	c, err := page.Evaluate(`() => { const s = document.createElement("span"); s.style.color = "var(--err)"; document.body.append(s); const c = getComputedStyle(s).color; s.remove(); return c }`)
	require.NoError(t, err)

	return c.(string)
}

// A subscribed feed's item shows up in the feed once the background poller
// fetched it, and dismissing the item removes it.
func TestSettings_FeedItemShownAndDismissed(t *testing.T) {
	t.Parallel()

	const itemTitle = "B5 Sample Feed Item"

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, b5RSSTemplate, srv.URL, itemTitle, time.Now().Format(time.RFC1123Z))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	feed, err := factory.RSSFeed(ctx, app.DB, factory.WithFeedURL(srv.URL+"/feed.xml"))
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, app.DB, user.ID, feed.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	// The feed is fetched by a background poller, not on demand, so the
	// item only shows up on a page loaded after that runs: reload /feed
	// until it does, then assert on the loaded page.
	item := page.Locator(".us-feed-rss-item").Filter(playwright.LocatorFilterOptions{HasText: itemTitle})
	require.Eventually(t, func() bool {
		if _, err := page.Goto("/feed"); err != nil {
			return false
		}

		n, err := item.Count()

		return err == nil && n == 1
	}, 25*time.Second, 500*time.Millisecond, "the feed item was never fetched by the poller")

	require.NoError(t, browser.Expect.Locator(item).ToBeVisible())
	require.NoError(t, item.GetByRole("button").Click())
	require.NoError(t, browser.Expect.Locator(item).ToHaveCount(0))
}

// Exporting posts downloads an archive that can be imported straight back
// in; import matches posts by their original ID, so re-importing your own
// export updates the existing post rather than duplicating it.
func TestSettings_ExportAndReimportPosts(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	post, err := factory.Post(context.Background(), app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))

	_, err = page.Goto("/controls/settings")
	require.NoError(t, err)

	download, err := page.ExpectDownload(func() error {
		return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Export posts"}).Click()
	})
	require.NoError(t, err)

	path, err := download.Path()
	require.NoError(t, err)

	require.NoError(t, page.GetByLabel("Import posts").SetInputFiles(path))

	require.NoError(t, browser.Expect.Locator(page.Locator("#import_export_results")).ToContainText(`"PostsUpdated":1`))

	posts, err := factory.ListPosts(context.Background(), app.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, post.ID, posts[0].ID)
}

// Uploading an image through a post's comment form inserts a markdown
// image reference, and once the comment is posted the image renders from
// /user-media with a non-zero natural width.
func TestSettings_UploadedImageRendersFromUserMedia(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	author := browser.NewUser(t, app)
	post, err := factory.Post(context.Background(), app.DB, author.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(author))

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "No Comments yet"}).Click())

	commentBox := fmt.Sprintf("[id='post%s']", post.ID)

	fileInput := page.Locator(commentBox + " input[type=file]")
	require.NoError(t, fileInput.SetInputFiles(playwright.InputFile{
		Name:     "b5-upload.png",
		MimeType: "image/png",
		Buffer:   b5PNGBytes(),
	}))

	textarea := page.Locator(commentBox + " textarea.comment-textarea")
	require.NoError(t, browser.Expect.Locator(textarea).ToHaveValue(regexp.MustCompile(`!\[b5-upload\.png\]\(.+\)`)))

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Post a comment"}).Click())

	img := page.Locator(".us-comments-section img.standalone-img")
	require.NoError(t, browser.Expect.Locator(img).ToHaveAttribute("src", regexp.MustCompile(`/user-media/`), playwright.LocatorAssertionsToHaveAttributeOptions{Timeout: playwright.Float(15000)}))

	_, err = page.WaitForFunction(`sel => { const el = document.querySelector(sel); return !!el && el.naturalWidth > 0 }`, ".us-comments-section img.standalone-img")
	require.NoError(t, err)

	// the upload is in the bucket as sent, and rendering it stored the
	// resized variant of the class the page asked for
	src, err := img.GetAttribute("src")
	require.NoError(t, err)

	m := regexp.MustCompile(`/user-media/([^/?#]+)/([^/?#]+)`).FindStringSubmatch(src)
	require.NotNil(t, m, "img src %s", src)

	fname, class := m[1], m[2]
	require.Equal(t, "image/png", app.S3Object(t, fname).ContentType)
	require.Equal(t, "image/webp", app.ResizedVariant(t, fname, class).ContentType)
}
