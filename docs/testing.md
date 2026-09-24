# Testing

The one document a test-writing subagent reads besides `AGENTS.md` and its own prompt. W0 (task T0.5) extends
it with the new helpers and one worked example each of a unit, a DB and an E2E test.

## Ground rules for W0–W5

1. **No production code changes.** No file that is compiled into `cmd/web`
   changes, apart from the three additive exceptions below. A wave that finds it
   cannot test something without changing code stops and reports it; that is
   input for R1, not a reason to refactor. Allowed:
   - `_test.go` files, and `testdata/` directories.
   - New packages that production code does not import:
     `pkg/testutil/...`, `e2e/`, and `cmd/seed` (W4).
   - The `testcontainers/postgres` package, `Makefile`, `.github/`,
     `docker-compose.yml`, `.env.example`, `tools/` and `docs/`. The
     migration and codegen scripts (`dbconfig.yml`, `sqlmigrate.sh`,
     `generate.sh`, `sqlboiler.toml`) may change **in W4 only**.
   - `go.mod`/`go.sum`, **only in W0** (test dependencies) and **W4.S2**
     (the `tool` directives).
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
- **testcontainers/postgres** - PostgreSQL test container helper

## Test container (before W0)

Located in `testcontainers/postgres`:
- Provides `NewTestDB()` function that returns a `*TestDB` with a clean database instance
- Each test gets its own isolated database
- Migrations are automatically applied from `migrations`
- Container is shared across tests in a package for efficiency
- Container cleanup happens automatically after tests complete (with 5-minute expiration as fallback)
- Use `defer testDB.Close()` to clean up the database after each test

## Factories (before W0)

Located in `pkg/feedops/testutil/factory.go`:
- Factory functions for creating test entities: `CreateUser`, `CreateRSSFeed`, `CreateRSSItem`, etc.
- Helper functions for retrieving entities: `GetRSSFeed`, `GetRSSItemsByFeed`, `GetUserFeedItemsByUser`
- All factory functions accept `context.Context` and `boil.ContextExecutor` for transaction support

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
