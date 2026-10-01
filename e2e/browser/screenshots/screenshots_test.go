//go:build browser && screenshots

// Package screenshots produces the PNGs of the user guide. It is not a test
// of the app: it asserts nothing about the pages and is behind the build tag
// `screenshots`, so `make test-ui` never runs it. Run it with
// `make screenshots`, which overwrites docs/guide/screenshots/.
package screenshots_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/testutil/seed"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) { browser.Main(m) }

// outDir is docs/guide/screenshots of the repository, resolved from this file.
var outDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "guide", "screenshots")
}()

// shot is one screenshot: the page, the seeded user who views it (empty for
// an anonymous visitor), the file name and what to wait for before capturing.
// Path may use the {post} placeholder: the id of a seeded post with comments.
type shot struct {
	file string
	user string
	path string
	// prepare runs after the page has loaded, e.g. to fill in an editor.
	prepare func(t *testing.T, page playwright.Page)
	// ready is a selector that must be visible before the capture.
	ready string
}

var shots = []shot{
	{file: "index.png", path: "/", ready: "h1"},
	{file: "journal.png", user: "bob", path: "/users/alice", ready: "h1"},
	{file: "post.png", user: "bob", path: "/posts/{post}", ready: "text=Level 3 (bob)"},
	{file: "feed.png", user: "alice", path: "/feed", ready: "h1"},
	{file: "editor.png", user: "alice", path: "/write", prepare: fillEditor, ready: "#show_preview"},
	{file: "settings.png", user: "alice", path: "/controls/settings", ready: "h1"},
	{file: "explore.png", user: "alice", path: "/explore", ready: "h1"},
}

const editorBody = `Hello from **pcom**!

- write in *markdown*
- share it with your connections

> Quotes work too.`

// fillEditor types a draft; the autosave reveals the preview link.
func fillEditor(t *testing.T, page playwright.Page) {
	require.NoError(t, page.GetByPlaceholder("Subject").Fill("My first post"))

	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, body.Fill(editorBody))
	// Fill sets the value in one go; a keystroke triggers the autosave.
	require.NoError(t, body.PressSequentially(" "))
}

func TestScreenshots(t *testing.T) {
	app := e2e.Start(t, e2e.WithRealAssets())
	require.NoError(t, seed.Run(context.Background(), app.DB.DB, io.Discard, seed.Options{}))
	require.NoError(t, os.MkdirAll(outDir, 0o755))

	var postID string
	require.NoError(t, app.DB.QueryRow(`SELECT post_id FROM post_comments LIMIT 1`).Scan(&postID))

	for _, s := range shots {
		t.Run(s.file, func(t *testing.T) {
			page := browser.Page(t, app, browser.Configure(func(o *playwright.BrowserNewContextOptions) {
				o.Viewport = &playwright.Size{Width: 1280, Height: 800}
				o.ColorScheme = playwright.ColorSchemeLight
			}))

			if s.user != "" {
				login(t, page, s.user)
			}

			path := regexp.MustCompile(`\{post\}`).ReplaceAllString(s.path, postID)
			_, err := page.Goto(path)
			require.NoError(t, err)

			if s.prepare != nil {
				s.prepare(t, page)
			}

			require.NoError(t, page.Locator(s.ready).First().WaitFor(playwright.LocatorWaitForOptions{
				State: playwright.WaitForSelectorStateVisible,
			}))
			require.NoError(t, page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State: playwright.LoadStateNetworkidle,
			}))

			_, err = page.Screenshot(playwright.PageScreenshotOptions{
				Path:     new(filepath.Join(outDir, s.file)),
				FullPage: new(false),
			})
			require.NoError(t, err)
		})
	}
}

// login signs the page in through the login form with the seed password.
func login(t *testing.T, page playwright.Page, name string) {
	t.Helper()

	_, err := page.Goto("/login")
	require.NoError(t, err)
	require.NoError(t, page.GetByLabel("Email address").Fill(name+"@example.test"))
	require.NoError(t, page.GetByLabel("Password").Fill(seed.Password))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log in"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
}
