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

## W1 — Unit tests, no database (2026-09-28, branch `test/w1-unit`)

**Built.**

- 13 tasks (U1–U12 and U7b), one commit each, only new `_test.go` files and
  `testdata/`. Total coverage without the generated models went from 35.1%
  (`master` after W0 and W6) to 49.0%.
- Characterization tests that guard later refactors: every
  `client/html/*.html` parses with the real `funcmap` (U12; a broken
  template or an unregistered function fails it), `HashUserPwd` pinned to
  fixed hex (U8), golden output of `ToEnrichedTemplate` for every HTML view
  (U4), golden subject, text and HTML for every user mail (U10) and admin
  notification (U11).
- Permission matrices: `CanSeeProfile` (U6) and `CanSeePost` ×
  `GetPostCapabilities` (U5), CSRF (U8), referer and auth enforcement (U9).
- Bugs filed: #147 (`HandleUpload` panics when reading the upload fails),
  #148 (the signup attribution regex is unanchored). #110's double close and
  #120's unescaped subjects got skipped tests.

**Wrong.**

- The plan assumed DB-free branches that don't exist. `EmailOKToSignup`
  queries users right after the format check, so its disposable-domain branch
  is unreachable without Postgres (U3 at 65%). `HandleUpload` inserts a row
  before storing the file (U7 at 44%), and the mail and admin wrappers update
  rows before sending (U10 at 72%). Everything DB-free in those packages is
  covered; the rest was added to W2's D4b, D6 and D7.
- Cheap agents pinned bugs as expected behavior: an unanchored regex's
  matches listed as "valid", a double close asserted as "logs an error", a
  skipped test with a commented-out body, a W2 placeholder test. The
  coordinator rewrote each to assert the correct behavior under `t.Skip`.
- U7b's streaming size-cap test served data under the cap, so raising the cap
  1000× passed. The mutation check caught it.
- U11 used `testdb` and the factories in a no-database wave. Its goldens took
  emails from the factory sequence, which parallel tests share, so they
  failed in the full run. Worse, `testdb` skips under `-short`, so the
  Docker-free check passed by not running them. Rewritten with in-memory
  structs.

**Left out.** Per-function targets instead of package percentages for the
three packages above; the numbers are what's reachable without a database.

**Cost.** 1 coordinator session and 13 subagents (9 haiku, 4 sonnet),
about 900 turns in total. The coordinator peaked at 182k context with 134k of
tool results; subagents at 51k–122k, the most expensive being U10 (haiku,
125 turns, sent back once) and U4 (haiku, 102 turns). 40 wasteful calls
(27 `cat`, 12 verbose). Skills: model-shape by 5 agents, wave-run and
wave-close once each; test-failure never. Compared with W6, about the same
turns for 13 tasks instead of 8: unit tests iterate less than browser tests,
but three tasks were sent back and five were patched by the coordinator.

## W3 — End-to-end HTTP tests: server rules (2026-09-28, branch `test/w3-e2e`)

**Built.**

- 4 tasks, one file each in `e2e/`: E1 visibility and read access
  (`visibility_test.go`, the post visibility matrix over viewer × profile
  visibility × post visibility × draft), E2 guards on every mutating route
  (`guards_test.go`, a table checked against the routes parsed from
  `cmd/web`), E3 one-shot links and account pages (`accounts_test.go`), E4
  API v1, private RSS and black-box uploads (`api_test.go`). Total coverage
  without the generated models went from 49.0% (after W1) to 67.7%;
  `cmd/web` is at 65.4%, all of it from the binary under test. The E2E suite
  runs in about 25 seconds.
- Factory options (unconfirmed users with a confirm seed, used invitations,
  confirmed waiting-list requests) and readers for shares, subscriptions,
  feed items, prompts, whitelist, mediator decisions and signup requests, so
  refused mutations are asserted against the database.
- E0, not in the original plan: a SessionStart hook for Claude Code on the
  web (`.claude/hooks/session-start.sh`). Fresh cloud containers had no
  running Docker, no libvips, a golangci-lint too old for go.mod, no
  `go tool covdata`, and no way to download Playwright's Chromium; the
  browser harness gained `CHROMIUM_PATH` to use the image's copy.
- Route union with W6 checked: every route is covered by W3, by W6, or (for
  `POST /form/signup` and `/form/signup_waiting_list`, off on purpose) by
  skipped tests for #139 and W3.E3's 404 pin.
- Bugs: drafts served to non-authors at `/posts/:id` (a direct connection
  saw every draft, anyone saw a public draft), fixed in PR #153 on top of
  this wave at the owner's request. Filed #151 (`/user-media` special
  files), #152 (malformed invite id → 500), #154 (unknown media name → 500);
  #109, #110 and #115 got skipped tests.
- Decided with the owner (Q15): a public post is visible to everybody
  whatever the profile visibility, but stays out of the public feed (#146).

**Wrong.**

- E4 (haiku) skipped three tests as "response not returning ID" and asked
  for handler changes: it never found that `ginhelpers.API` wraps every
  response in `{"data": ...}`. It also left its build tag on and skipped
  pagination. Re-dispatched once at sonnet with the root cause; that passed.
- E3 (haiku) left a DB assertion commented out behind a FIXME even after the
  reader existed; the mutation check caught it and the coordinator fixed it.
- The task table referred to W2's D2a matrix and to issues by number; the
  subagents can read neither, so the coordinator pasted both into the
  prompts. `task_prompt.py` doesn't do this.
- The first push failed with a 403 until GitHub access was reconnected.

**Left out.** Three browser tests fail only in the cloud sandbox (older
Chromium, no outbound network); CI's `browser` job is the reference for them.

**Cost.** 1 coordinator session and 7 subagent runs (2 opus, 2 haiku, 1
sonnet retry, 1 sonnet Explore for the route union; E2 and E3 were resumed
once each with new factory helpers), about 510 turns in total. The
coordinator peaked at 217k context with 142k of tool results, much of it the
environment troubleshooting and the draft fix; subagents at 90k–154k. 31
wasteful-call flags (25 `cat`, 4 raw builds, 2 full reads). Skills: wave-run,
wave-close and session-start-hook once each, model-shape by 2 agents,
test-failure never. Compared with W1, about half the turns for a third of
the tasks: E2E tasks are larger, and one was sent back.
