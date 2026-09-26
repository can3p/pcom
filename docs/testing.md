# Testing

The one document a test-writing subagent reads besides `AGENTS.md` and its own prompt.

The test layers, from cheapest to most expensive:

| Layer | Where | Runs with | Checks |
|---|---|---|---|
| Unit | next to the code | `make test-short` | pure functions, goldens |
| Package (DB) | next to the code; after RS, the service tests | `make test` (Docker) | business rules and queries against a fresh database |
| E2E HTTP | `e2e/` | `make test` (Docker) | the real binary: statuses, redirects, htmx headers, HTML, resulting state. From R2, mail and S3 through tommy |
| Browser | `e2e/browser` (build tag `browser`) | `make test-ui` | the real binary with real assets in Chromium: JS, htmx swaps, dialogs, layout, no console or CSP errors |

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
row with sane defaults. Functional options override specific fields, e.g. `factory.WithPassword("secret")`,
`factory.Published()`, `factory.Visibility(core.PostVisibilityPublic)`. Readers, in `read.go`, fetch state
back through the same `exec`: `factory.GetUser`, `factory.GetPost`, `factory.ListPosts`, `factory.ListComments`,
`factory.ListOutgoingEmails`, `factory.ConnectionExists`.

The package imports no `testing`, so it also works from e2e's subprocess-backed database. Tests reach the ORM
only through it (ground rule 3); a missing builder or reader is requested from the coordinator, not
improvised inline. `pkg/feedops/testutil` is legacy: don't use it in new tests.

## Fakes and other helpers

- **`testutil.Must(t, v, err)`** (`pkg/testutil`) - returns `v`, or fails the test immediately if `err != nil`; for one-line fixture setup.
- **`fakesender.New()`** (`pkg/testutil/fakesender`) - a `sender.Sender` that records every `Send` instead of delivering it. `Sent()` returns what was recorded; `FailWith(err)` makes `Send` fail instead (`nil` resumes recording).
- **`fakestorage.New()`** (`pkg/testutil/fakestorage`) - an in-memory `pkg/media/server.MediaStorage`. `FailUploadWith`, `FailDownloadWith` and `FailExistsWith(err)` inject an error into the matching call.
- **`ginctx.New(t, method, target, body, opts...)`** (`pkg/testutil/ginctx`) - a `*gin.Context` wired like a real request (cookie session under `"sess"`), plus its `*httptest.ResponseRecorder`. Options: `ginctx.WithUser(t, db, userID)`, `ginctx.WithCSPNonces(style, script)`.
- **`golden.Assert(t, name, got)`** (`pkg/testutil/golden`) - compares `got` against `testdata/<name>.golden`. `UPDATE_GOLDEN=1 go test ./pkg/...` writes it instead of comparing; after regenerating, check `git diff --stat`, not the contents.

## End-to-end: e2e

Runs the real `cmd/web` binary against its own database and drives it over HTTP, so a test sees only what a
browser would (statuses, redirects, htmx headers, HTML). `-short` (`make test-short`) skips E2E entirely.
Every package that uses it needs `func TestMain(m *testing.M) { e2e.Main(m) }`.

`e2e.Start(t, opts...)` returns `*App{URL, DB}`. `app.Client(t)` gives a cookie-carrying `*Client` with `Get`,
`PostForm`, `PostJSON`, `LoginAs(email, password)` and `Do(req)` for anything else; each returns a `*Response`
with `RequireStatus(code)`, `Doc()` (goquery), `Location()`, `HXRedirect()`, `HXTrigger()`, `HXRetarget()`,
`HXReplaceURL()` and `HXRefresh()`. A feed a test creates must point at an `httptest.Server` the test owns,
never a real remote URL. Mail is asserted through the outgoing queue for now:
`factory.ListOutgoingEmails(ctx, app.DB, core.OutgoingEmailWhere.EmailType.EQ(...))`.

## Make targets

`make test-short` runs everything except E2E. `make cover` runs unit, package and E2E tests together under one
`GOCOVERDIR` and prints a merged per-package coverage table. The quiet targets `check-q`, `test-q`, `vet-q` and
`cover-q` (see `AGENTS.md`) are for agents and narrow with `PKG=./pkg/links/...`.

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
	user, err := factory.User(context.Background(), app.DB, factory.WithPassword("secret-pw"))
	require.NoError(t, err)

	client := app.Client(t)
	client.Get("/feed").RequireStatus(http.StatusFound)

	client.LoginAs(user.Email, "secret-pw")
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
