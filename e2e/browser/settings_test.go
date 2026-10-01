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

// Submitting the change-password form with the wrong current password shows
// the error in place, without leaving the settings page (the htmx form is
// swapped, not the whole page); fixing the password then lets the user log
// back in with it after logging out.
func TestSettings_ChangePasswordValidatesInPlaceThenLogsInAgain(t *testing.T) {
	t.Parallel()

	const newPassword = "b5-new-password"

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.GetByLabel("Old Password").Fill("not-the-real-password"))
	require.NoError(t, page.GetByLabel("New Password").Fill(newPassword))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Change password"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByText("old password is not correct")).ToBeVisible())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/settings$`)))

	require.NoError(t, page.GetByLabel("Old Password").Fill(browser.Password))
	require.NoError(t, page.GetByLabel("New Password").Fill(newPassword))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Change password"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToContainText("Password has been changed successfully"))

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log out"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/$`)))

	_, err = page.Goto("/login")
	require.NoError(t, err)

	browser.LogInWithCode(t, app, page, user.Email)

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
}

// Saving the general settings form shows a success message and persists the
// chosen profile visibility across a reload.
func TestSettings_GeneralSavesProfileVisibility(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	labels := []string{"Public"}
	_, err = page.Locator("select[name='profile_visibility']").SelectOption(playwright.SelectOptionValues{Labels: &labels})
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Settings"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToContainText("Settings have been saved"))

	_, err = page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.Locator("select[name='profile_visibility']")).ToHaveValue("public"))
}

// A saved user style is applied to the user's own journal page, scoped
// under .user-styles-applied.
func TestSettings_UserStylesApplyOnProfilePage(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.GetByLabel("Add your css below").Fill(".us-user-header { color: rgb(12, 34, 56); }"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save styles"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToContainText("Styles have been saved successfully!"))

	_, err = page.Goto("/users/" + user.Username)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.Locator(".us-user-header")).ToHaveCSS("color", "rgb(12, 34, 56)"))
}

// Generating an API key replaces the button with the key display.
func TestSettings_GenerateAPIKey(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Generate an API Key"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByText("Your API key, click to copy")).ToBeVisible())
}

// Clicking the clipboard icon next to an existing API key copies the key.
func TestSettings_CopyAPIKey(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	key, err := factory.APIKey(context.Background(), app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.Permissions = []string{"clipboard-read", "clipboard-write"}
	}))

	_, err = page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.Locator("i.bi-clipboard[role=button]").Click())

	copied, err := page.Evaluate(`() => navigator.clipboard.readText()`)
	require.NoError(t, err)
	require.Equal(t, key.APIKey, copied)
}

// Sending an invite with an available slot lists the invitee under used
// invites and queues the invitation email.
func TestSettings_SendInviteQueuesEmail(t *testing.T) {
	t.Parallel()

	// unique, since every test binary run shares one tommy
	inviteeEmail := "b5-invitee-" + uuid.NewString() + "@example.test"

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	_, err := factory.Invitation(context.Background(), app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))

	_, err = page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.Locator("#sendInviteEmail").Fill(inviteeEmail))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Send an invite"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByText(inviteeEmail)).ToBeVisible())

	emails := app.Mails(t, inviteeEmail, func(m tommy.Mail) bool {
		return m.Subject == "Welcome to pcom" && strings.Contains(m.Text, app.URL+"/invite/")
	})
	require.Len(t, emails, 1)
}

// Adding a feed that points at a test-owned server eventually shows its
// item in the feed, dismissing the item removes it, and unsubscribing
// removes the feed from settings.
func TestSettings_FeedAddSeeItemDismissAndUnsubscribe(t *testing.T) {
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

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	feedURL := srv.URL + "/feed.xml"

	require.NoError(t, page.Locator("input[name='url']").Fill(feedURL))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add url"}).Click())

	require.NoError(t, browser.Expect.Locator(page.Locator(fmt.Sprintf("a[href='%s']", feedURL))).ToBeVisible())

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

	_, err = page.Goto("/controls/settings")
	require.NoError(t, err)

	feedRow := page.Locator(".list-group-item").Filter(playwright.LocatorFilterOptions{HasText: "B5 Test Feed"})
	require.NoError(t, browser.Expect.Locator(feedRow).ToHaveCount(1))

	require.NoError(t, feedRow.GetByRole("button").Click())
	require.NoError(t, browser.Expect.Locator(feedRow).ToHaveCount(0))
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

// The About text written in the settings is rendered as markdown on the
// journal for the owner and for an allowed visitor, and clearing it removes
// the block.
func TestSettings_ProfileAboutShownOnJournal(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	visitor := browser.NewUser(t, app)
	_, _, err := factory.Connect(context.Background(), app.DB, user.ID, visitor.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	_, err = page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.GetByLabel("About you").Fill("I like [the web](https://example.com/web)"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save profile"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToContainText("Profile has been saved"))

	about := page.Locator(".us-profile-about")
	_, err = page.Goto("/users/" + user.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "About"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(about.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "the web"})).ToHaveAttribute("href", "https://example.com/web"))

	vpage := browser.Page(t, app, browser.As(visitor))
	_, err = vpage.Goto("/users/" + user.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(vpage.Locator(".us-profile-about").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "the web"})).ToBeVisible())

	_, err = page.Goto("/controls/settings")
	require.NoError(t, err)
	require.NoError(t, page.GetByLabel("About you").Fill(""))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save profile"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToContainText("Profile has been saved"))

	_, err = page.Goto("/users/" + user.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-profile-about")).ToHaveCount(0))
}
