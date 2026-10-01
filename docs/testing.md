# Testing

The one document a test-writing subagent reads besides `AGENTS.md` and its own prompt.

The test layers, from cheapest to most expensive:

| Layer | Where | Runs with | Checks |
|---|---|---|---|
| Unit | next to the code | `make test-short` | pure functions, goldens |
| Package (DB) | next to the code; after RS, the service tests | `make test` (Docker) | business rules and queries against a fresh database |
| E2E HTTP | `e2e/` | `make test` (Docker) | server rules that don't depend on the frontend: access control and visibility, statuses, CSRF and auth guards, API, RSS, security headers, resulting state. From R2, mail and S3 through tommy |
| Browser | `e2e/browser` (build tag `browser`) | `make test-ui` | **everything a user does in a page**, in Chromium with the real assets: forms, buttons, htmx swaps and redirects, Stimulus controllers, dialogs, layout, no console or CSP errors |

The split matters: an HTTP test that imitates htmx (sending its headers, pinning `HX-*` response headers)
keeps passing when an htmx upgrade breaks every page. So a behavior that needs the page's JavaScript to
happen is tested in the browser, never over plain HTTP.

## Ground rules for W0–W6

1. **No production code changes.** No file that is compiled into `cmd/web`
   changes, and neither do its templates, JS or SCSS, apart from the three additive exceptions below. A wave that finds it
   cannot test something without changing code stops and reports it; that is
   input for R1, not a reason to refactor. Allowed:
   - `_test.go` files, and `testdata/` directories.
   - New packages that production code does not import:
     `pkg/testutil/...`, `e2e/`, and `cmd/seed` (W4).
   - `Makefile`, `.github/`, `docker-compose.yml`, `.env.example`, `tools/`
     and `docs/`. The migration and codegen scripts (`dbconfig.yml`,
     `sqlmigrate.sh`, `generate.sh`, `sqlboiler.toml`) may change **in W4
     only**.
   - `go.mod`/`go.sum`, **only in W0** (test dependencies), **W4.S2**
     (the `tool` directives) and **W6.B0** (playwright-go).
2. **Bugs are filed, not fixed.** If a test exposes a bug, write the test for
   the *correct* behavior, skipped, and report it; the coordinator files the
   GitHub issue (label `bug`) and fills in the number:

   ```go
   t.Skip("known bug: https://github.com/can3p/pcom/issues/NNN")
   ```

   WB removes the skips as it fixes the bugs. If the behavior is merely odd
   rather than wrong, write a characterization test that pins the current
   behavior, with a comment saying so. For a security bug, do not open a
   public issue. Report it to the coordinator, who asks the owner.
   Check `gh issue list --label bug` before filing, so the same bug isn't
   filed twice.
3. **Tests touch the ORM only through `pkg/testutil/factory`** to create
   fixtures and read state back. A test body that calls `core.Posts(...)`
   directly is a test R5 has to rewrite. This rule is what makes the bob
   migration cheap. The code under test obviously still uses `core`. To learn
   a model's shape, run `make model T=<Model>`; never read `pkg/model/core`.
4. **Prefer black-box tests.** Use `package foo_test` and the public API unless
   an unexported function has logic worth pinning on its own, such as
   `ConstructComments` or `isURLMediaUpload`. Black-box tests survive
   refactors; tests of internals get rewritten by them.
5. **Assertions use `testify/require`** (and `assert` where continuing after a
   failure helps). Don't add new uses of `alecthomas/assert`, which R6 removes.
   Mocks use mockio v2 as shown below, but prefer the fakes in `pkg/testutil`.
6. **Never assert on wall-clock time.** Nothing is injectable yet. Use
   `require.WithinDuration(t, time.Now(), got, 5*time.Second)`, or compare
   ordering.
7. **Every test gets its own database** (`testdb.New(t)`), and tests may run
   in parallel (`t.Parallel()` is encouraged for DB tests).
8. **Mail content is golden-tested** under `testdata/*.golden`, with the
   convention `UPDATE_GOLDEN=1 go test ./pkg/mail/...` to rewrite.
   These goldens are the safety net for R4.
9. **Keep tests compact.** Cases that differ only in inputs and expectations
   are rows of one table-driven test with `t.Run`, not one function each. A
   table test opens one database at the top, and each row makes its own
   users with the factory, so rows can run in parallel. Create fixtures in
   one line with `testutil.Must(factory.User(ctx, db))(t)`. Setup that
   several tests of a file repeat goes into one small helper in that file.
   No comments that restate the test name, and no separate tests for trivial
   variations.
10. **An ordering test inserts rows in the opposite order to the one it
    expects.** A query without a working `ORDER BY` returns rows in insertion
    order, so rows inserted in the expected order pass whether or not the
    query sorts (#108 hid this way twice).
11. **A test that compares stored times must fail under UTC too.** Timestamp
    columns have no time zone (#171), so on a host ahead of UTC every stored
    time reads back in the future, and a rate-limit or "older than" test can
    pass for the wrong reason. Run such a test's mutation check with `TZ=UTC`.

## Libraries

- **github.com/ovechkin-dm/mockio/v2** - Mock library for Go without code generation
- **github.com/stretchr/testify** - Assertion and testing utilities (require, assert)
- **github.com/can3p/gogo/testcontainers/postgres** - PostgreSQL test container helper, wrapped by `pkg/testutil/testdb`

## Database: pkg/testutil/testdb

`db := testdb.New(t)` returns a fresh, fully migrated Postgres database, dropped when the test ends. `db.DB`
is a `*sqlx.DB` and also a `boil.ContextExecutor`, so it goes directly into sqlboiler and factory calls.
`db.URL` is its connection string, for handing to a subprocess (this is how `e2e.Start` gives the real binary
its own database). Every test gets its own database, so `t.Parallel()` is safe and encouraged.

## Fixtures: pkg/testutil/factory

Builders such as `factory.User(ctx, exec, opts...)`, `factory.Post(ctx, exec, authorID, opts...)`,
`factory.Comment(ctx, exec, postID, authorID, opts...)` and `factory.RSSFeed(ctx, exec, opts...)` insert one
row with sane defaults. Functional options override specific fields, e.g. `factory.WithEmail("a@b.test")`,
`factory.Published()`, `factory.Visibility(core.PostVisibilityPublic)`. Readers, in `read.go`, fetch state
back through the same `exec`: `factory.GetUser`, `factory.GetPost`, `factory.ListPosts`, `factory.ListComments`,
`factory.ListOutgoingEmails`, `factory.ConnectionExists`.

The package imports no `testing`, so it also works from e2e's subprocess-backed database. Tests reach the ORM
only through it (ground rule 3); a missing builder or reader is requested from the coordinator, not
improvised inline. `pkg/feedops/testutil` is legacy: don't use it in new tests.

## Fakes and other helpers

- **`testutil.Must(v, err)(t)`** (`pkg/testutil`) - returns `v`, or fails the test immediately if `err != nil`; for one-line fixture setup such as `testutil.Must(factory.User(ctx, db))(t)`.
- **`fakesender.New()`** (`pkg/testutil/fakesender`) - a `sender.Sender` that records every `Send` instead of delivering it. `Sent()` returns what was recorded; `FailWith(err)` makes `Send` fail instead (`nil` resumes recording).
- **`fakestorage.New()`** (`pkg/testutil/fakestorage`) - an in-memory `pkg/media/server.MediaStorage`. `FailUploadWith`, `FailDownloadWith` and `FailExistsWith(err)` inject an error into the matching call.
- **`ginctx.New(t, method, target, body, opts...)`** (`pkg/testutil/ginctx`) - a `*gin.Context` wired like a real request (cookie session under `"sess"`), plus its `*httptest.ResponseRecorder`. Options: `ginctx.WithUser(t, db, userID)`, `ginctx.WithCSPNonces(style, script)`.
- **`golden.Assert(t, name, got)`** (`pkg/testutil/golden`) - compares `got` against `testdata/<name>.golden`. `UPDATE_GOLDEN=1 go test ./pkg/...` writes it instead of comparing; after regenerating, check `git diff --stat`, not the contents.

## End-to-end: e2e

Runs the real `cmd/web` binary against its own database and drives it with plain HTTP: statuses, redirects,
headers, HTML and the resulting database state. It is for server rules that don't depend on the frontend
(see the layers table); the client does not imitate htmx, and user flows belong in the browser suite.
`-short` (`make test-short`) skips E2E entirely.
Every package that uses it needs `func TestMain(m *testing.M) { e2e.Main(m) }`.

`e2e.Start(t, opts...)` returns `*App{URL, DB}`. `app.Client(t)` gives a cookie-carrying `*Client` with `Get`,
`PostForm`, `PostJSON`, `LoginAs(email)` (it posts the email, issues the code through the accounts service and posts it) and `Do(req)` for anything else; each returns a `*Response`
with `RequireStatus(code)`, `Doc()` (goquery) and `Location()`; `Header` and `Body` are plain fields. A feed a test creates must point at an `httptest.Server` the test owns,
never a real remote URL.

Mail and user media go to one tommy container per test binary (`pkg/testutil/tommy`), which the harness
starts and points the binary at. Assert what was delivered, never the `outgoing_emails` queue:
`app.Mails(t, to, match)` waits for mail to that exact recipient (match may be nil), `app.NoMails(t, to,
match)` asserts none arrived, and `app.SentMails` returns the final set for counting; the last two first wait
until the app has sent everything it queued (`app.WaitMailSent`), so an absence or a count is final. The
container is shared and never cleared: use addresses unique to the test and match on text only your app
produces (`app.URL+"/..."`). `app.S3Object(t, key)`, `app.S3Objects(t,
prefix)` and `app.ResizedVariant(t, fname, class)` read the bucket; `tommy.S3Events` gives the raw request
headers (e.g. `X-Amz-Acl`). Package tests that need neither keep `fakesender` and `fakestorage`.

## Browser tests: e2e/browser

Every user flow that needs the page's JavaScript is tested here, in headless Chromium through
[playwright-go](https://github.com/mxschmitt/playwright-go): forms, action buttons, htmx swaps and
redirects, Stimulus controllers, confirmations, toasts, dark mode. The package is behind the build tag
`browser`, so `make test` and `make check` need no browser. Run it with `make test-ui`, which builds the
frontend first (`RUN=<regex>` narrows it, `COUNT=<n>` repeats it, `HEADED=1 SLOWMO=250` shows the browser);
install Chromium once with `make ui-deps`. Compile it with `make vet-q PKG=./e2e/browser/... TAGS=browser`.

What a run does:

1. `make test-ui` runs `yarn build`, so the suite tests the current frontend source, then
   `go test -tags browser ./e2e/browser/...` through the quiet runner. A passing run prints one line.
2. `TestMain` starts the Playwright driver bundled with playwright-go and launches **one Chromium for
   the package**, headless unless `HEADED` is set (`SLOWMO=<ms>` delays every action). It uses the
   browsers `make ui-deps` installed in Playwright's cache (`~/Library/Caches/ms-playwright` on macOS).
3. Each test starts its own app (the real web binary against a fresh test database) and gets a fresh
   browser context in that Chromium: separate cookies and storage, so tests run in parallel.
4. On failure the screenshot and trace paths are printed. `make ui-trace F=<path>` opens Playwright's trace
   viewer, a timeline of every action with a DOM snapshot, console and network log at each step.

To watch a test, run one at a time: `make test-ui HEADED=1 SLOWMO=250 RUN=<TestName>`. Without `RUN`,
every parallel test opens its own window.

The harness API:

- `e2e.Start(t, e2e.WithRealAssets())` serves the real `cmd/web/dist`. Every test starts its own app.
- `browser.NewUser(t, app, opts...)` is `factory.User` for a confirmed user, and
  `browser.Page(t, app, browser.As(user))` returns a page in a fresh browser context, already logged in
  (it reuses an HTTP login's session cookie). The base URL is the app's, so `page.Goto("/feed")`.
- **Guards:** the page fails the test on an uncaught error, a `console.error`, a CSP violation, a failed
  request to the app or an app response of 404 or 5xx. A test that causes an error on purpose exempts it
  with `browser.Allow(regexp)`, matched against the guard message. `browser.Configure(fn)` changes the
  context options (viewport, `ColorScheme`).
- **On failure** a full-page screenshot and a trace are saved to `.ui-artifacts/` (or `$UI_ARTIFACTS`) and
  both paths are logged in one line; open the trace with `make ui-trace F=<path>`. CI uploads the directory.

Reliability rules, because a red run must mean a real regression:

1. **Assert outcomes, not mechanisms:** what the user sees (text, roles, visibility, URL, title) and the
   database state through the factory readers. Never wait for htmx events, read `hx-*` attributes, inspect
   request headers or call `window.htmx`, so the tests survive an htmx upgrade and catch one that breaks a page.
2. **Wait by assertion only:** `browser.Expect` (Playwright's auto-waiting assertions), never a sleep or
   `WaitForTimeout`. A "nothing happened" check asserts on a state the action would have changed.
3. **Locate by role, label and text**, then by existing CSS classes. Test waves don't add `data-testid`.
4. **Isolation:** no shared fixtures; every test may call `t.Parallel()`.
5. **No retries:** a flaky test is fixed, or skipped with an issue number. A new test must pass
   `make test-ui RUN=<it> COUNT=3` before it is accepted.

A browser test:

```go
//go:build browser

package browser_test

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
```

## Make targets

`make test-short` runs everything except E2E. `make cover` runs unit, package and E2E tests together under one
`GOCOVERDIR` and prints a merged per-package coverage table. The quiet targets `check-q`, `test-q`, `vet-q`,
`cover-q`, `fix-q` and `lint-q` (see `AGENTS.md`) are for agents, narrow with `PKG=./pkg/links/...` and take build tags with
`TAGS=browser`. The browser targets are `ui-deps`, `test-ui` and `ui-trace` (above).

CI enforces a coverage ratchet: after `make cover`, `make cover-check` fails if total coverage (excluding
`pkg/model/core`) or one of the critical packages drops below its floor in `tools/coverage-floors.txt`. Floors
are the merged numbers minus 1% and only ever rise: after raising coverage, run `make cover` and set the new
percentage minus 1.

## Claude Code on the web

Cloud sessions get a fresh container that lacks some of what the tests need. The SessionStart hook
`.claude/hooks/session-start.sh` fixes that before the session starts, and is safe to re-run by hand if a
step fails:

- installs `libvips-dev` (cgo headers for `pkg/media`; without them `cmd/web` and `e2e` don't compile);
- starts `dockerd` (testcontainers Postgres for `testdb` and E2E; log in `/tmp/dockerd.log`);
- installs the frontend's `node_modules` and downloads Go modules;
- builds `go tool covdata`, which the image's Go install lacks and `make cover` needs;
- installs golangci-lint at CI's `GOLANGCI_LINT_VERSION` (the image's copy is built with an older Go and
  refuses to lint this module);
- installs the playwright-go driver and sets `CHROMIUM_PATH=/opt/pw-browsers/chromium`, because the
  network policy blocks Playwright's CDN, so `make ui-deps` can't download Chromium.

`make generate` builds the tools image, which needs the network, so it fails in the sandbox. Run
`generate.sh` on the host instead, with `sqlboiler` and `sql-migrate` installed by `go install` at the versions
the tools image pins (F3 did).

The image's Chromium is older than the one CI installs, and the sandbox has no general outbound network, so a
few browser tests fail only there (an embedded external resource, inline-style CSP reports):
`TestWriting_RenderedPostFeatures`, `TestWriting_PostVisibility` and `TestActions_ShareLifecycle` (2026-10-01). Trust CI's
`browser` job for those; W6 and refactor waves run the full suite there.

The tools image may not build in the sandbox (Docker Hub rate limits, package mirrors blocked). Then run
`PCOM_ALLOW_HOST_TOOLS=1 ./generate.sh` on the host with the sql-migrate and sqlboiler versions `tools/Dockerfile`
pins, against a throwaway `postgres:16-alpine` container (F5 did, twice).

## Worked examples

A unit test, pinning an unexported function's logic (ground rule 4):

```go
package postops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsURLMediaUpload(t *testing.T) {
	t.Parallel()

	require.True(t, isURLMediaUpload("3fa85f64-5717-4562-b3fc-2c963f66afa6.png"))
	require.False(t, isURLMediaUpload("not-a-uuid.png"))
}
```

A DB test, using `testdb`, `factory` and a reader alongside the function under test:

```go
package userops_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/stretchr/testify/require"
)

func TestGetDirectUserIDs(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	alice, err := factory.User(ctx, db)
	require.NoError(t, err)
	bob, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, alice.ID, bob.ID)
	require.NoError(t, err)

	ids, err := userops.GetDirectUserIDs(ctx, db, alice.ID)
	require.NoError(t, err)
	require.Contains(t, ids, bob.ID)

	got, err := factory.GetUser(ctx, db, bob.ID)
	require.NoError(t, err)
	require.Equal(t, bob.Email, got.Email)
}
```

An E2E test:

```go
package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

func TestFeedRequiresLogin(t *testing.T) {
	app := e2e.Start(t)
	user, err := factory.User(context.Background(), app.DB)
	require.NoError(t, err)

	client := app.Client(t)
	client.Get("/feed").RequireStatus(http.StatusFound)

	client.LoginAs(user.Email)
	client.Get("/feed").RequireStatus(http.StatusOK)
}
```

## Mockio v2

```go
import . "github.com/ovechkin-dm/mockio/v2/mock"

func TestExample(t *testing.T) {
    ctrl := NewMockController(t)
    mockObj := Mock[MyInterface](ctrl)

    // Single return value
    WhenSingle(mockObj.Method(Any[string]())).ThenReturn("result")

    // Multiple return values
    WhenDouble(mockObj.Method(Any[string]())).ThenReturn("result", nil)

    // Dynamic answers
    WhenSingle(mockObj.Method(Any[string]())).ThenAnswer(func(args []any) string {
        return "dynamic result"
    })
}
```
