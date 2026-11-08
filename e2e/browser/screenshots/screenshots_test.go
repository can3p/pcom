//go:build browser && screenshots

// Package screenshots produces the PNGs of the user guide. It is not a test
// of the app: it asserts nothing about the pages and is behind the build tag
// `screenshots`, so `make test-ui` never runs it. Run it with
// `make screenshots`, which overwrites docs/guide/screenshots/.
package screenshots_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/seed"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

var _ = func() struct{} {
	wiremock.Register("testdata/wiremock")

	return struct{}{}
}()

func TestMain(m *testing.M) { browser.Main(m) }

// outDir is docs/guide/screenshots of the repository, resolved from this file.
var outDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "guide", "screenshots")
}()

// shot is one screenshot: the page, the seeded user who views it (empty for
// an anonymous visitor), the file name and what to wait for before capturing.
// Path may use the {post} placeholder (the id of a seeded post with comments)
// and {token} (the manage token of a subscriber).
type shot struct {
	file string
	user string
	path string
	// prepare runs after the page has loaded, e.g. to fill in an editor.
	prepare func(t *testing.T, page playwright.Page)
	// ready is a selector that must be visible before the capture.
	ready string
	// phone takes the shot at a phone's size.
	phone bool
}

var shots = []shot{
	{file: "index.png", path: "/", ready: "h1"},
	{file: "journal.png", user: "bob", path: "/users/alice", ready: "h1"},
	{file: "post.png", user: "bob", path: "/posts/{post}", ready: "text=Level 3 (bob)"},
	{file: "feed.png", user: "alice", path: "/feed", ready: "h1"},
	{file: "editor.png", user: "alice", path: "/write", prepare: fillEditor, ready: "#show_preview"},
	{file: "grammar.png", user: "alice", path: "/write", prepare: checkGrammar, ready: ".pr-card"},
	{file: "settings.png", user: "alice", path: "/controls/settings", ready: "h1"},
	{file: "explore.png", user: "alice", path: "/explore", ready: "h1"},
	{file: "subscribe.png", path: "/users/carol", prepare: openSubscribe, ready: "[data-testid=subscribe-form]"},
	{file: "subscriptions.png", path: "/subscriptions/{token}", ready: "[data-testid=subscriptions-list]"},
	{file: "controls.png", user: "alice", path: "/controls", ready: "#email-subscribers-title"},
	{file: "notes.png", user: "alice", path: "/controls/notes", ready: "[data-testid=note]"},
	{file: "note-edit.png", user: "alice", path: "/controls/notes/{note}/edit", ready: "#note-tags"},
	{file: "capture.png", user: "alice", path: "/controls/notes/new", phone: true, ready: ".capture-mode"},
	{file: "notes-panel.png", user: "alice", path: "/write", prepare: openNotesPanel, ready: "[data-testid=picker-note]"},
	{file: "api-keys.png", user: "alice", path: "/controls/settings", prepare: createNotesKey, ready: "[data-testid=api-key]"},
}

// screenshotAPIKey is a full API key of alice's that the shots make notes with.
const screenshotAPIKey = "screenshots-api-key"

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

// checkGrammar types a Russian draft whose mistakes the WireMock stub in
// testdata/wiremock knows, and asks for a grammar check, so the suggestion
// panel is open in the shot.
func checkGrammar(t *testing.T, page playwright.Page) {
	require.NoError(t, page.GetByPlaceholder("Subject").Fill("Перрон"))
	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill("Она одела пальто, достала телефон, но связи не было. Не смотря на усталость, она решила идти пешком. В течении часа она бродила по пустым улицам, и ни один фонарь не горел."))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Check grammar", Exact: new(true)}).Click())
}

func TestScreenshots(t *testing.T) {
	// STT_PROVIDER shows the record button; no recording is sent to the stub
	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock(),
		e2e.WithEnv("STT_PROVIDER", "openai-compat"),
		e2e.WithEnv("STT_OPENAI_COMPAT_ENDPOINT", "http://127.0.0.1:9/stt"),
		e2e.WithEnv("STT_OPENAI_COMPAT_KEY", "unused"))
	require.NoError(t, seed.Run(context.Background(), app.DB, io.Discard, seed.Options{}))
	require.NoError(t, os.MkdirAll(outDir, 0o755))

	var postID string
	require.NoError(t, app.DB.QueryRow(`SELECT post_id FROM post_comments LIMIT 1`).Scan(&postID))

	// The subscription shots need subscriptions: carol takes them, alice has
	// two subscribers, and dana follows carol.
	ctx := context.Background()
	_, err := app.DB.Exec(`UPDATE users SET email_subscriptions_enabled = true WHERE username = 'carol'`)
	require.NoError(t, err)
	alice, err := factory.GetUserByEmail(ctx, app.DB, "alice@example.test")
	require.NoError(t, err)
	carol, err := factory.GetUserByEmail(ctx, app.DB, "carol@example.test")
	require.NoError(t, err)
	for _, email := range []string{"dana@example.test", "erin@example.test"} {
		_, err := factory.EmailSubscription(ctx, app.DB, alice.ID, email)
		require.NoError(t, err)
	}
	_, err = factory.EmailSubscription(ctx, app.DB, carol.ID, "dana@example.test")
	require.NoError(t, err)
	dana, err := factory.GetEmailSubscriber(ctx, app.DB, "dana@example.test")
	require.NoError(t, err)
	_, err = factory.APIKey(ctx, app.DB, alice.ID, factory.WithAPIKey(screenshotAPIKey))
	require.NoError(t, err)
	noteID := addNotes(t, app)

	placeholders := strings.NewReplacer("{post}", postID, "{token}", dana.ManageToken, "{note}", noteID)

	for _, s := range shots {
		t.Run(s.file, func(t *testing.T) {
			viewport := playwright.Size{Width: 1280, Height: 800}
			if s.phone {
				viewport = playwright.Size{Width: 390, Height: 844}
			}
			opts := []browser.PageOption{browser.Configure(func(o *playwright.BrowserNewContextOptions) {
				o.Viewport = &viewport
				o.IsMobile = new(s.phone)
				o.HasTouch = new(s.phone)
				o.ColorScheme = playwright.ColorSchemeLight
			})}
			// signed in the way the browser tests are: several shots share a
			// seed user, more than the mail limit lets log in through the form
			if s.user != "" {
				user, err := factory.GetUserByEmail(context.Background(), app.DB, s.user+"@example.test")
				require.NoError(t, err)
				opts = append(opts, browser.As(user))
			}

			page := browser.Page(t, app, opts...)

			_, err := page.Goto(placeholders.Replace(s.path))
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

// openSubscribe opens the journal's subscribe panel.
func openSubscribe(t *testing.T, page playwright.Page) {
	t.Helper()

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Subscribe by email"}).Click())
}

// addNotes makes the notes the shots show, through the notes API as a phone
// would. It returns the id of the note whose edit page is shot.
func addNotes(t *testing.T, app *e2e.App) string {
	t.Helper()

	now := time.Now()
	ago := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

	postNote(t, app, url.Values{
		"kind":        {"quote"},
		"source":      {"Moby-Dick"},
		"locator":     {"p. 1"},
		"text":        {"Call me Ishmael. Some years ago, having little or no money in my purse, I thought I would sail about a little and see the watery part of the world."},
		"captured_at": {ago(72 * time.Hour)},
	})

	edit := postNote(t, app, url.Values{
		"text":        {"The light on the river before seven. Write it down while it lasts. #walk #essay"},
		"captured_at": {ago(2 * time.Hour)},
	})

	// three captures in one sitting make a stack
	for i, text := range []string{"Outline: three examples of slow mornings", "The bakery sign on Hale Street", "Ask Mara about the ferry timetable #essay"} {
		postNote(t, app, url.Values{"text": {text}, "captured_at": {ago(time.Duration(26-i*10) * time.Hour)}})
	}

	postNote(t, app, url.Values{"text": {"A post about carrying a notebook everywhere"}, "captured_at": {ago(5 * time.Hour)}})

	return edit
}

// postNote makes one note with the API and returns its id.
func postNote(t *testing.T, app *e2e.App, form url.Values) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, app.URL+"/api/v1/notes", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+screenshotAPIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Less(t, resp.StatusCode, http.StatusMultipleChoices)

	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))

	return out.Data.ID
}

// openNotesPanel writes a little, then shows the Notes tab of the editor's sidebar.
func openNotesPanel(t *testing.T, page playwright.Page) {
	t.Helper()

	fillEditor(t, page)
	require.NoError(t, page.GetByRole("tab", playwright.PageGetByRoleOptions{Name: "Notes", Exact: new(true)}).Click())
}

// createNotesKey makes a notes-only key through the form, then reloads, so
// the one-time key is not in the shot.
func createNotesKey(t *testing.T, page playwright.Page) {
	t.Helper()

	api := page.Locator("#api")
	require.NoError(t, api.GetByLabel("Key name").Fill("iPhone"))
	_, err := api.GetByLabel("Access").SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Notes only"}})
	require.NoError(t, err)
	require.NoError(t, api.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Create a key"}).Click())
	require.NoError(t, page.Locator("[data-testid=api-key]").First().WaitFor())

	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, page.Locator("#api").ScrollIntoViewIfNeeded())
}
