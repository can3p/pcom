//go:build browser

// Package browser drives the real app in headless Chromium, for everything a
// user does in a page: forms, action buttons, htmx swaps and redirects,
// Stimulus controllers, toasts. It builds on the e2e harness, so each test
// gets its own server and database, and fixtures come from the factories.
//
// The package is behind the build tag `browser`; run it with `make test-ui`.
// Every test package needs a TestMain that calls Main:
//
//	func TestMain(m *testing.M) { browser.Main(m) }
package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
)

// Password is the login password of every user made by NewUser.
const Password = "browser-pw"

// Expect holds Playwright's auto-waiting assertions. Wait for a state with
// these, never with a sleep.
var Expect = playwright.NewPlaywrightAssertions(5000)

var (
	repoRoot = func() string {
		_, file, _, _ := runtime.Caller(0)
		return filepath.Dir(filepath.Dir(filepath.Dir(file)))
	}()

	chromium playwright.Browser
)

// Main starts one Chromium for the test package, builds the web binary
// through e2e.Run and runs the tests. HEADED=1 shows the browser and
// SLOWMO=<ms> slows every action down, for watching a test locally.
func Main(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	pw, err := playwright.Run(&playwright.RunOptions{Verbose: false})
	if err != nil {
		fmt.Fprintln(os.Stderr, "browser: starting Playwright (run `make ui-deps` once):", err)
		return 1
	}
	defer func() { _ = pw.Stop() }()

	opts := playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(os.Getenv("HEADED") == "")}
	if ms, err := strconv.ParseFloat(os.Getenv("SLOWMO"), 64); err == nil {
		opts.SlowMo = playwright.Float(ms)
	}

	chromium, err = pw.Chromium.Launch(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "browser: launching Chromium (run `make ui-deps` once):", err)
		return 1
	}
	defer func() { _ = chromium.Close() }()

	return e2e.Run(m)
}

// NewUser creates a user who can log in with Password.
func NewUser(t testing.TB, app *e2e.App, opts ...factory.UserOpt) *core.User {
	t.Helper()

	u, err := factory.User(context.Background(), app.DB, append([]factory.UserOpt{factory.WithPassword(Password)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}

	return u
}

// PageOption configures Page.
type PageOption func(*pageConfig)

type pageConfig struct {
	user    *core.User
	allow   []*regexp.Regexp
	context playwright.BrowserNewContextOptions
}

// As logs the page in as user, who must have been made by NewUser. It reuses
// the session of an HTTP login rather than filling in the login form.
func As(user *core.User) PageOption {
	return func(c *pageConfig) { c.user = user }
}

// Allow exempts guard violations matching the regular expression, for a
// test that causes an error on purpose, such as a server error that must
// show the error toast.
func Allow(pattern string) PageOption {
	return func(c *pageConfig) { c.allow = append(c.allow, regexp.MustCompile(pattern)) }
}

// Configure changes the browser context options, for example the viewport
// or the emulated color scheme.
func Configure(fn func(*playwright.BrowserNewContextOptions)) PageOption {
	return func(c *pageConfig) { fn(&c.context) }
}

// cspReporter turns a CSP violation into a console error, which the guards
// catch like any other.
const cspReporter = `document.addEventListener('securitypolicyviolation', (e) => {
  console.error('CSP violation: ' + e.violatedDirective + ' blocked ' + (e.blockedURI || 'inline code'));
});`

// Page opens a blank page in a fresh browser context, with the app's URL as
// the base URL, so tests call page.Goto("/feed").
//
// The page is guarded: the test fails on an uncaught page error, a
// console.error, a CSP violation, a failed request to the app, or an app
// response of 404 or 5xx, unless Allow exempts it. On failure a trace and a
// full-page screenshot are saved and their paths logged.
func Page(t testing.TB, app *e2e.App, opts ...PageOption) playwright.Page {
	t.Helper()

	cfg := pageConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	cfg.context.BaseURL = playwright.String(app.URL)

	ctx, err := chromium.NewContext(cfg.context)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.user != nil {
		login(t, app, ctx, cfg.user)
	}

	if err := ctx.AddInitScript(playwright.Script{Content: playwright.String(cspReporter)}); err != nil {
		t.Fatal(err)
	}

	if err := ctx.Tracing().Start(playwright.TracingStartOptions{
		Screenshots: playwright.Bool(true),
		Snapshots:   playwright.Bool(true),
	}); err != nil {
		t.Fatal(err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatal(err)
	}

	g := &guard{app: app.URL, allow: cfg.allow}
	g.watch(page)

	t.Cleanup(func() {
		for _, v := range g.violations() {
			t.Errorf("browser: %s", v)
		}

		if t.Failed() {
			saveArtifacts(t, ctx, page)
		} else {
			_ = ctx.Tracing().Stop()
		}

		_ = ctx.Close()
	})

	return page
}

func login(t testing.TB, app *e2e.App, ctx playwright.BrowserContext, user *core.User) {
	t.Helper()

	client := app.Client(t)
	client.LoginAs(user.Email, Password)

	var cookies []playwright.OptionalCookie
	for _, c := range client.Cookies() {
		cookies = append(cookies, playwright.OptionalCookie{Name: c.Name, Value: c.Value, URL: playwright.String(app.URL)})
	}

	if err := ctx.AddCookies(cookies); err != nil {
		t.Fatal(err)
	}
}

// guard collects what a page did wrong while the test ran. Playwright calls
// the handlers from its own goroutine.
type guard struct {
	app   string
	allow []*regexp.Regexp

	mu   sync.Mutex
	errs []string
}

func (g *guard) watch(page playwright.Page) {
	page.OnPageError(func(err error) { g.add("uncaught page error: " + err.Error()) })

	page.OnConsole(func(m playwright.ConsoleMessage) {
		if m.Type() == "error" {
			g.add("console.error: " + m.Text())
		}
	})

	page.OnRequestFailed(func(r playwright.Request) {
		if !strings.HasPrefix(r.URL(), g.app) {
			return
		}

		reason := ""
		if f := r.Failure(); f != nil {
			reason = f.Error()
		}

		// A navigation or swap that supersedes a pending request aborts it;
		// that is the browser working, not the app failing.
		if strings.Contains(reason, "ERR_ABORTED") {
			return
		}

		g.add(fmt.Sprintf("request failed: %s %s: %s", r.Method(), r.URL(), reason))
	})

	page.OnResponse(func(r playwright.Response) {
		if s := r.Status(); strings.HasPrefix(r.URL(), g.app) && (s == 404 || s >= 500) {
			g.add(fmt.Sprintf("response %d: %s %s", s, r.Request().Method(), r.URL()))
		}
	})
}

func (g *guard) add(msg string) {
	for _, re := range g.allow {
		if re.MatchString(msg) {
			return
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.errs = append(g.errs, msg)
}

func (g *guard) violations() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.errs...)
}

// artifactsDir is where failed tests leave traces and screenshots:
// UI_ARTIFACTS if set, else .ui-artifacts in the repository root.
func artifactsDir() string {
	if dir := os.Getenv("UI_ARTIFACTS"); dir != "" {
		return dir
	}

	return filepath.Join(repoRoot, ".ui-artifacts")
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func saveArtifacts(t testing.TB, ctx playwright.BrowserContext, page playwright.Page) {
	dir := artifactsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("browser: saving artifacts: %v", err)
		return
	}

	base := filepath.Join(dir, unsafeName.ReplaceAllString(t.Name(), "_"))
	trace, shot := base+".trace.zip", base+".png"

	if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(shot), FullPage: playwright.Bool(true)}); err != nil {
		shot = "none (" + err.Error() + ")"
	}

	if err := ctx.Tracing().Stop(trace); err != nil {
		trace = "none (" + err.Error() + ")"
	}

	t.Logf("browser: screenshot %s, trace %s (make ui-trace F=%s)", shot, trace, trace)
}
