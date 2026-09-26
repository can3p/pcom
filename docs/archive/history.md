# Modernization history

Finished waves, newest last. Each entry records what was built, what turned
out wrong, what was left out on purpose, and what the wave cost in tokens.

## W0 — Test foundation (2026-09-26, branch `test/w0-foundation`)

**Built.**

- `pkg/testutil/testdb.New(t)` over gogo `v0.0.2`'s `testcontainers/postgres`
  (a template database, one copy per test). pcom's dockertest-based copy is deleted, and the
  existing DB tests use the new helper.
- `pkg/testutil/factory`: a builder per table, the graph relationships, and
  readers in `read.go`, with a self-test that builds every entity. It doesn't import
  `testing`, so the seed command can use it.
- `pkg/testutil`: `Must`, `fakesender`, `fakestorage`, `ginctx`, `golden`.
- `e2e/`: builds `./cmd/web` once with `-cover`, runs it per test against
  its own database with a stub asset manifest, and drives it with a
  cookie-jar client that sends the scraped CSRF token and exposes htmx
  headers. Two smoke tests (anonymous home, factory user logs in to `/feed`).
- `make test` writes `coverage.out`, `make test-short` skips E2E, `make cover`
  merges unit and E2E binary coverage into one per-package table;
  `codecov.yml`; `docs/testing.md` rewritten with worked examples.
- `task_prompt.py` now pastes a task's `###` section together with its table
  row, and takes ownership from "Owns …".

**Turned out wrong.**

- The plan assumed the E2E binary could simply be killed. It has no signal
  handling, and a `-cover` binary writes coverage only on a normal exit. A build
  overlay adds a SIGTERM handler. It can't go into `cmd/web`, because the cover tool ignores
  overlays on the packages it instruments, and it can't go into a dependency, because the module cache
  can't be overlaid. So it goes into `pkg/types`, a package without statements, which is left out of
  `-coverpkg`.
- `go test -cover` gives the test process its own `GOCOVERDIR`; the harness
  passes the binary the `-test.gocoverdir` value instead.
- Small shape changes against the plan: `Client.PostJSON` takes a path, not
  an action name; `ginctx.New` takes options (`WithUser`, `WithCSPNonces`)
  after the four planned arguments.

**Left out.** `pkg/feedops/testutil` stays, for W2.D8. (CI switched from
`make test` to `make cover` before merge, so Codecov sees E2E coverage: Q14.)

**Cost.** 5 sessions (coordinator plus 4 subagents: 2 sonnet builders, 1 haiku, 1 sonnet docs),
280 turns in total; the coordinator peaked at 148k context with 88k of tool
results, and the subagents at 43k–167k (the factories were the most expensive:
74 turns, 122k of results). 33 wasteful calls across all agents, nearly all
`cat` of whole files, plus 2 raw builds. Skills: wave-run and wave-close once
each. model-shape was used through `make model` (5 calls). LSP was loaded once
in total and test-failure never, because nothing failed. This is the first
recorded wave, so there is no earlier cost line to compare with.

## W6 — Browser tests (2026-09-26, branch `test/w6-browser`)

**Built.**

- B0 (coordinator): `e2e/browser` behind the `browser` build tag, playwright-go,
  `e2e.WithRealAssets()`, logged-in pages from the session cookie, guards on page
  errors, `console.error`, CSP violations and failed same-origin requests, a trace
  and screenshot on failure, `make ui-deps`/`test-ui`/`ui-trace`, and a `Browser`
  CI job. Smoke tests for login, boosted navigation, an action button and the
  error toast.
- B1–B7: one file per area (navigation, writing, comments, actions, settings,
  layout sweep, accounts). The suite runs in about 35 seconds, and passes
  `-count=3`.
- Every Stimulus controller except `selfsubmit` (used by no template) and
  `collapse` (only under the mobile-menu test skipped on #140) fails a test when
  its `connect()` is emptied. Removing `json-enc`, `head-support`, the
  `htmx:responseError` handler or the `htmx:sendError` handler each fails a test.
- Every mutating browser route is used successfully, except `signup` (skipped on
  #139) and `signup_waiting_list`, which is switched off in code (Q6). The API
  routes are W3's.
- Bugs filed: #139 (pages rendered from a bare map have no CSP nonce: `/signup`,
  `/confirm_signup`, `/articles`, `/confirm_waiting_list`), #140 (opening the
  mobile menu violates `style-src-attr`), #141 (a submit right after the post
  form re-renders itself can go out natively and get a 403).

**Wrong.**

- The plan said comments update "without a full reload". The comment form
  reloads the page on purpose (it keeps the scroll position), so a subagent
  pinned the intended behavior as a bug. The test now asserts the reload
  behavior, and no issue was filed.
- The share link has no copy button, so the planned "copy the share link"
  step had nothing to click. The clipboard controller is tested through the
  API key instead.
- The haiku layout sweep was vacuous (`window.scrollWidth` is undefined, so
  the overflow check could never fail) and allowed CSP violations. It was
  redone at sonnet.

**Left out.** `/confirm_signup` in the layout sweep (it needs a user with a
known confirmation seed, and #139 blocks the page anyway). The `Browser`
check becomes required on `master` right after this PR merges (not before, or
open PRs without the job would wait forever); that is the owner's step.

**Cost.** 2 coordinator sessions (B0, then B1–B7) and 8 subagents (7 sonnet,
1 haiku), 945 turns in total. The coordinators peaked at 133k and 140k
context with 92k and 108k of tool results; subagents at 79k–262k, with the
settings and writing tasks the most expensive (150–167 turns, 234k–271k of
results). 91 wasteful calls, almost all `cat` of whole files. Skills:
frontend-htmx by 4 agents, wave-run twice, wave-close once; model-shape and
test-failure never, and LSP barely (2k of results). Compared with W0, the
wave cost about three times the turns for twice the subagents: browser
tasks iterate far more than unit tests do.
