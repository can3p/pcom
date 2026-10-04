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
- The owner's review of the PR found tests that could not fail: the open and
  closed `/signup` tests were identical and checked only that a body existed,
  one test repeated another as a logged-in user, and others duplicated browser
  tests. An audit of all four files confirmed about a dozen such cases (weak
  assertions, duplicates of W6 tests, a case missing: a foreign edit through
  the API). All were fixed on the branch, each checked with a mutation. The
  per-task check had been "tests pass plus one mutation", which samples one
  test per task; `wave-run` now adds an assertion audit.

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

## W4 — Local stack, dev tooling container, seed (2026-09-28, branch `test/w4-local-stack`, PR #164)

**Built.**

- `cmd/seed` (logic in `pkg/testutil/seed`): alice, bob, carol, dave and eve
  (password `password`), connections, every post visibility, a draft, a URL
  post, a three-level thread, a share, a prompt, Alice's fixed API key, an
  `example.test` feed and a pending mediation request. `--reset` keeps
  `migrations` and `system_settings`; `FLY_APP_NAME` makes it refuse. It
  prints the logins when done.
- `docker-compose.yml`: `postgres` on 5442 and `tommy` on 8811/8822 with S3 on
  9555 (moved from the planned 9090, which `make pprof_tunnel` uses). There is
  a `tools` profile, and a `dev` profile (`make dev`: the app under watchexec
  plus `yarn watch`, `node_modules` in a volume, `PCOM_WATCH_POLL` for
  polling). Every port can be overridden with `PCOM_*_PORT`.
- `tools/Dockerfile`: `golang:1.26-alpine` with psql, and sql-migrate,
  sqlboiler and sqlboiler-psql installed with `go install` at versions pinned
  by `ARG`s. A `dev` target adds vips, gcc, Node, yarn and watchexec.
- Scripts no longer split URLs by hand (`dbconfig.yml` reads `DATABASE_URL`)
  and refuse to run outside the container. `make generate` builds the models
  from a throwaway `pcom_codegen` database, and refuses when the image's
  sqlboiler differs from the library in `go.mod`. The Make targets are
  `dev-up/down`, `migrate*`, `migration`, `generate`, `psql`, `db-reset`,
  `seed`, `seed-reset`, `tools-shell`, `dev`, `dev-logs` and `migrate-prod`.
- CI jobs "Generated models are current" and "Dev image".
- `docs/running.md` (macOS first); README points at it.
- `TestSeed_EveryPageRenders` logs in as every seeded user and loads 16 pages
  each; a broken template fails it.

**Wrong.**

- The committed models had been generated by sqlboiler 4.16.2 against the
  4.19.1 runtime. The first `make generate` changed 27 files; they were
  regenerated once, in their own commit.
- The plan pinned the generators with `go tool` directives. That put about 40
  modules (sqlboiler's CLI dependencies, including MySQL and Oracle drivers)
  into `go.mod`. The owner rejected it in review, and they moved into the
  Dockerfile. `generate.sh`'s version check replaces what the `go.mod`
  pinning had given for free.
- The sandbox's network policy blocks the Alpine mirror, so neither image
  could be built for real there. S2 tested with a stand-in base image, and
  CI's first run was the real check; it passed.
- The permission check denied reading the generated-model diff, even after
  the owner allowed it in chat. The cause was found from the version header,
  which the owner pasted.
- A subagent ran `git mv` against its instructions. The staged renames went
  into the coordinator's next docs commit, and that commit didn't build; it
  was split and force-pushed.
- Docker died during S3's image tests. Re-running the SessionStart hook by
  hand failed on an unset `CLAUDE_PROJECT_DIR`; fixed in this close-out.

**Left out.** The app doesn't use tommy yet: it uses S3 only in production,
has no path-style option and no Mailjet base URL. That is R2, and
`.env.example` and `docs/running.md` say so. Not done: the fresh-clone
walkthrough of `docs/running.md`, `make dev` in a container, and macOS file
watching, all for the owner's machine.

**Cost.** 1 coordinator session (139 turns, peak 276k context, 217k of tool
results) and 6 subagents (S1, S2 opus, S3, S4, S5-docs, S5-test; 5 to 40
turns each, peaks 53k–96k), about 236 turns in total. 21 `cat` flags from the
coordinator, mostly reading short scripts and prompt files. Skills: wave-run
and wave-close once each; model-shape and test-failure not reported as used
by any subagent. Compared with W3, about half the turns: tooling tasks need
fewer iterations than test tasks, but the coordinator's peak was higher
because of CI log reads (`get_job_logs` fed 46k) and the model-diff standoff.

## W2 — Package tests against Postgres (2026-09-28, branch `test/w2-db`)

**Built.**

- 10 tasks, each owning its own test files: D1 the connection graph and
  mediation (`pkg/userops`), D2a the privacy matrix for `SinglePost`,
  `UserHome`, `Explore` and `SharedPost` (`pkg/web/privacy_test.go`, one
  table over viewer × post visibility × profile visibility × draft, with
  capabilities, comments and share), D2b feed composition, controls and
  settings, D3 the API, D4a and D4b every form in `pkg/forms` and the
  database branches of the email validation, D5 `pkg/auth` and
  `pkg/pgsession`, D6 `pkg/mail` and the email queue (`dbsender`), D7
  `pkg/postops`, `pkg/media` uploads and the feed code, D8 image resizing
  and caching in `pkg/media/server`.
- Factory helpers: `GetPostStat`, `GetMediaUploadByFname`, `GetUserByEmail`,
  `GetUserStyle`, `GetRSSItem`, `ListRSSItems`, `PromptCreatedAt`, and
  `NextFetchAt` and `WithoutTitle` options for `RSSFeed`.
- Total coverage without the generated models went from 67.7% (after W3) to
  81.5%. Every W2 package meets its target: `auth` 91.8%, `pgsession`
  94.4%, `userops` 89.0%, `web` 87.9% (the four privacy-matrix functions at
  100%), `forms` 85.3%, `mail` 87.2%, `dbsender` 86.7%, `postops` 88.3%,
  `media` 85.2%, `feedops` 84.9%, `feeder` 83.2%, `media/server` 83.5%.
  `make cover` runs in about 35 seconds.
- Bugs filed: #156 (API `updated_since` off by the server's UTC offset),
  #157 (invalid `save_action` reported under `visibility`), #158 (waiting
  list stores the email as typed), #159 (user styles over the limit are
  saved) and #160 (`auth.Login` panics on a database error). Skipped tests
  also pin #108, #113, #114, #116, #117, #147 and #148. Q16 (delete on a
  never-saved post) went to the open questions.
- The privacy matrix found that `UserHome` loaded direct-only and
  second-degree posts for a logged-in visitor unrelated to the author. The
  journal template never rendered them, so only a package test could see
  it; fixed in a separate PR stacked on this branch, which turns the two
  skipped rows on.

**Wrong.**

- Every task passed its mutation check, and the assertion audit at the end
  still found 35 weak tests, 18 duplicates (within W2 and of W3's E2E
  tests), 3 that pinned suspect behavior and 5 skips that could not fail.
  The commonest weak test asserted `NotNil` on the action a form's `Save`
  returns; the bad skips depended on the host's time zone or a lucky race.
  Four fix agents reworked them, which cost almost as much as the first
  round. The audit now runs per task for every test wave, and the preamble
  bans those patterns.
- Two "privacy leaks" were escalated to the owner as security bugs although
  master had already fixed one (drafts at `/posts/:id`) and decided the
  other (Q15). The branch had not been rebased since it was cut.
- The coordinator had the parallel agents prefix identifiers with task
  names, which master's new naming rule forbids; they were renamed after
  the rebase.
- D4b on the cheap tier left broken, untagged files in `pkg/forms` and
  reported BLOCKED; the retry on the mid tier finished it. D8's first
  version survived a resize mutation and was sent back. A session rate limit
  stopped three agents mid-task; they were resumed with their context.

**Left out.** The package matrix duplicates part of W3's
`TestVisibility_SinglePostMatrix` and `TestVisibility_Explore` (status
codes only); cutting the E2E side is left to the owner. Some constructor and
panic-recovery branches stay uncovered where only a fault in the middle of
a transaction reaches them.

**Cost.** 1 coordinator session and 16 subagent runs (10 tasks, of which
D2a on opus and D4b and D8 on haiku; a sonnet retry of D4b; an opus Explore
audit; 4 sonnet audit-fix agents), about 1,500 turns in total. The
coordinator peaked at 266k context with 149k of tool results; subagents at
93k–250k. 78 wasteful-call flags (62 `cat`, 15 full reads, 1 raw build),
almost all in subagents. Skills: wave-run and wave-close once each,
model-shape by 1 agent, test-failure never. Compared with W3, three times
the turns for 2.5 times the tasks: the late audit and its rework made up
about a quarter of them.

## W5 — Coverage ratchet (2026-09-29, branch `test/w5-ratchet`)

Built `make cover-check`: `tools/coverage-check.sh` compares the merged
coverage `make cover` leaves in `.cover/` and `coverage.out` with the floors
in `tools/coverage-floors.txt`, and CI runs it right after the tests. The
Codecov upload runs even when the check fails. Floors are the achieved level
minus 1%:

| Scope | Achieved | Floor |
|---|---|---|
| total, excluding `pkg/model/core` | 81.1% | 80.1% |
| `pkg/auth` | 92.6% | 91.6% |
| `pkg/web` | 89.3% | 88.3% |
| `pkg/userops` | 89.0% | 88.0% |
| `pkg/postops` | 88.4% | 87.4% |
| `pkg/forms` | 85.3% | 84.3% |

The total is up from the 17.2% baseline and above the plan's 70–80%
estimate. The floors use the merged numbers (unit, package and E2E tests
under one `GOCOVERDIR`), not a package's own `ok` line, which is lower for
the packages E2E exercises (`pkg/web` 87.9% on its own). A package listed
with no data fails, so a misspelled path can't pass. The check was proven by
raising a package floor, raising the total floor and misspelling a path:
each made it fail.

Left out: browser tests aren't counted, because `make cover` doesn't run
them; `pkg/postops/rss` (93.3%) has no floor of its own; floors are raised by
hand, not automatically.

**Cost.** 1 coordinator session (23 turns, peak 70k context, 21k of tool
results, 3 `cat` flags) and 1 haiku subagent (48 turns, peak 50k, 26k of
tool results). Skills: wave-run and wave-close once each. The smallest wave
so far by far: one task, and the coordinator measured the baseline once
before dispatch so the subagent never re-ran `make cover`.

## WB — Bug-fix wave (2026-09-29, branch `fix/wb-survey-bugs`)

Fixed 31 issues in 12 tasks over three rounds, one commit per task: #108–#117,
#119–#122, #139–#143, #147, #148, #151, #152, #154, #156–#160, #163, #167 and
#168. The wave took the filed bugs #139–#143, #147 and #148 as well, and
the owner decided Q7 (the zip export follows post visibility) and #168
(a second pending invitation to an address is rejected). Rounds existed
because `cmd/web/main.go` held the routes, the RSS and zip handlers and
several `log.Fatal`s, so one task per round owned it; JS/SCSS tasks never
shared a round, because assets are built once per round.

What changed for users and operators:

- Emails are stored lowercased and trimmed in `users`, `user_invitations`
  and `user_signup_requests`, and CHECK constraints enforce it (the owner
  checked the production tables by hand first). Login is one exact lookup
  on the normalized input; a legacy sha256 hash is replaced by argon2id on
  login. Login and logout rotate the session.
- The private RSS feed has its own read-only token (`user_feed_tokens`),
  regenerated from settings; old feed URLs carrying an API key are plain
  404s, as the owner didn't want to keep compatibility. `pkg/repo` exists now, holding the feed token queries.
- Invitation mails are keyed by the invitation, a partial unique index
  allows one pending invitation per address, and the waiting list, signup
  and invitation checks compare addresses case-insensitively.
- Request handling no longer calls `log.Fatal`; HTML emails escape user
  content. Every route with a UUID path param, the API's included, goes
  through the `requireUUIDParam` middleware.

What turned out wrong along the way:

- **B's first design locked users out.** Lowercasing `users.email` in a
  migration destroyed the spelling a legacy hash was salted with, so any
  invited user with a capital letter could never log in, and accounts that
  differed only in case were skipped. It was first reworked to look up
  case-insensitively; once the owner had checked the production tables, it
  became a plain normalization with database constraints instead.
- **B broke change password.** Login rehashed to argon2id while the change
  password form still compared sha256 hashes. Caught from B's own report,
  not by a test; the form now uses the same check.
- **#108's first fix sorted by a UUID.** `ORDER BY id DESC` on UUID ids is
  random, and the test inserted rows in the order it expected, so it passed
  either way. The coordinator's own correction first inverted the insertion
  order the wrong way; on a +02:00 host the test also couldn't fail because
  of #171. The comments query's order can't be observed through
  `SinglePost`, which re-sorts, so it has no test.
- **#141's first test couldn't fail.** The race was made deterministic by
  clicking Delete inside `htmx:afterSwap`.
- **`make cover-check` passed on stale data.** A fresh `make cover` showed
  `pkg/auth` below its floor (89.0% against 91.6%), because the auth rework
  added error branches. Tests for the reachable ones brought it to 91.1%;
  the rest are session-save, session-rotation and database-error paths
  that need a fault injector, so the owner lowered the floor to 90.1%, the
  one exception so far to "floors only ever rise".

Left out: #171 (timestamps without a time zone; filed during WB, not
scheduled), a test for #115's database-error 500, a test for #111 (R2
replaces the flags), and the session-save error branches in `pkg/auth`.
Still for later, after R1: #123 (signups with bot protection) and #124
(feed pagination, which needs W6.B5's feed browser tests).

**Cost.** 1 coordinator session (144 turns, peak 255k context, 146k of tool
results; flags: `cat`×9) and 17 subagents: 13 task runs (one a rework at a
higher tier) and 4 assertion audits, about 600 turns together, medians
29 turns and 63k peak. The busiest subagents were B (strong: 109 turns,
87k peak) and D (60 turns, 125k peak). Across all agents, tool results fed
710k through Bash and 277k through Read; wasteful calls: `cat`×31,
verbose×1, full-read×1, raw-build×1. Skills: wave-run and wave-close once,
frontend-htmx by 2 subagents; model-shape and test-failure were never used.
The coordinator peak is close to W4's (276k) and far above W5's one-task
70k: most of it came from reviewing diffs of production code, which a
bug-fix wave needs and test waves didn't, plus about ten minutes of
safety-classifier outage retries.

## R1 — Router decomposition (2026-09-30, branch `refactor/r1-router`)

`cmd/web/main.go` went from 1058 lines to 195 (`main()` is 85, ending in
`app.New(deps).Run()`). Everything else moved to `pkg/web/app`: `Deps`,
`New()` (middleware, session store, recovery, group mounting), the funcmap
and static manifest loader, `jsonAction`, and the routes, split into
`routes_{media,public,auth,rss,controls}.go` and
`actions_{connections,mediation,posts,shares,prompts,rss,settings,media}.go`,
one mount function per file. `api.go` moved as is. New in-process `httptest`
tests run against `app.New`. Deleted: `pkg/web/user_connections.go` and
`client/html/from--signup-waitlist.html`.

Three steps, one commit each. Step 1 (strong subagent) built the skeleton
with the handlers still in one file. Step 2 was planned as parallel
agents, but the coordinator did it with a script instead: every handler was
a top-level block, so cutting by line range and comparing old and new line
multisets proved the move verbatim, which agents copying code can't. Step 3
(mid subagent) converted the actions.

What turned out differently from the plan:

- **The E2E suite read the source.** `TestGuards_RouteTableMatchesSource`
  scans named source files for route registrations, so every step broke it
  without a behavior change. The owner allowed one exception to "e2e/ is not
  edited": its `prefixes` map (files and router-variable names) is updated
  per step; `guardRoutes` and `excludedRoutes` are unchanged. RS inherits
  the same rule (noted in `docs/plan/rs.md`).
- **15 of the 19 actions fit `jsonAction`, not 19.** `generate_api_key`
  and `regenerate_feed_token` take no body (binding would add a "Bad input"
  error), `upload_media` replies with its own JSON, `settings/export`
  streams a file and `settings/import` reads a form upload. Error texts
  are byte-identical; lint's ST1005 rejects capitalized error strings, so
  the user-facing sentences go through `userError`.
- **Handlers had to move in Step 1,** not Step 2: `app.New` can't reference
  code in package `main`.
- One behavior change, accepted: `prompt_post` queries with the request
  context instead of `main`'s background context.

Left out: `TestFuncmapMarkdownFuncsRegistered` is weak (all five markdown
modes give the same output for its input) but moved unchanged, because R1
doesn't touch moved assertions. `task_prompt.py` now builds prompts for
step-style waves (a bullet per step) and gives non-RS refactor waves a
"move, don't restructure" layering line.

**Cost.** 1 coordinator session (69 turns, peak 145k context, 70k of tool
results; flags: `cat`×8) and 3 subagents: Step 1 (strong: 46 turns, 101k
peak, including one round of audit fixes), Step 3 (mid: 10 turns, 52k peak)
and one assertion audit (8 turns, 30k peak), about 64 turns together.
Skills: wave-run and wave-close once each; model-shape, test-failure and
frontend-htmx were not needed. Against WB (144 coordinator turns, 255k
peak, 17 subagents, about 600 turns), R1 cost about a tenth in agent turns,
mostly because the scripted Step 2 replaced a dozen task runs.

## RS — Repositories and services, thin handlers (done 2026-09-30)

Built: every query now lives in `pkg/repo` (methods on `*repo.Store`, with
`Tx` joining an open transaction, `Using(exec)` for legacy callers and
factories, `SendMail` so mail is queued in the change's transaction), every
rule in `pkg/service/{accounts,connections,feeds,graph,media,posts,reading,shares}`,
wired through `pkg/service/registry`. Handlers bind, call one service method
and render; the service errors map to responses in one place each
(`ginhelpers.Status`/`HTMLError`, `actionMessage`). `pkg/arch` type-checks
every package and flags queries, ORM imports and database handles passed to
other code outside `pkg/repo`; its allowlist went from 64 files to none.
`pkg/userops`, `feedops.go`, `pkg/web/func.go` and `media.HandleUpload` are
gone. `docs/architecture.md` describes the result.

How it ran: step 0 (coordinator) built the contracts and the shares slice,
extracted the connection graph that three areas share, and split the mixed
handler and page files by area with a line-multiset-checked script, so the
six step 1 tasks owned disjoint files. Each task ran in its own git
worktree; the coordinator merged by cherry-pick. Step 2 (one mid task)
removed the bridges between areas and merged the per-area repository files.

What turned out wrong:
- Agent worktrees start from `origin/master`, not the unpushed wave branch:
  the first dispatch of all six tasks came back BLOCKED with no edits.
- Audits found something in five of seven tasks: deleted tests and changed
  upload texts (L6), weak new tests (L1, L5), no service-level authorization
  tests (L3: removing the owner filter from delete failed only e2e), and
  failures that no longer panicked, so no 500 and no admin mail (L4). All
  were fixed before merging.
- One audit reported the #118 hole reopened. It was the coordinator's
  mutation check, read during the minutes the owner filter was removed.
- A line-union resolver for shared files is safe for imports and struct
  fields, not for code: it deduplicated `}` in `func_test.go`.
- The E2E visibility tests don't catch direct-only posts leaking to
  second-degree viewers on a user's home page; only the privacy matrix
  does.
- `make cover-check` passed on stale `.cover/` data until the close. A
  fresh `make cover` showed the total at 79.4% and `pkg/repo` at 9%.
  Per-package instrumentation didn't count code a service test reaches in
  another package. `make cover` now uses `-coverpkg` over every pcom
  package (total 85.7%), and the floors follow. One `make cover` run
  failed on a test that three reruns didn't reproduce; watch for it in CI.

Behavior changes, accepted: a database failure on the public RSS feed after
the user lookup answers 500 (was 404); API validation errors answer 400
(was 500); dismissing an unknown RSS item and commenting on a missing post
say "not found" instead of the raw "sql: no rows" text; an API key whose
user fails to load now fails the request instead of continuing without a
user; form saves run the service's own transaction instead of joining
gogo's, so an accepted invite stays if starting the session fails. The
prompt service now checks the direct connection itself (the form did).

Left out: moving the page builders from `pkg/web` next to their handlers in
`pkg/web/app` (a package merge touching the privacy matrix; not needed for
the layering, left for a later wave). Factory users still log in through
the legacy password hash (`HashUserPwd`), which real users no longer take.
The dbsender mail queue is exempt from the arch test, like pgsession, until
R3/R5 change gogo's `sender.Sender`.

**Cost.** 1 coordinator session (207 turns, peak 391k context, 208k of tool
results; flags: `cat`×11, raw `go build`/`go test`×27) and 20 subagents
(about 760 turns): six area tasks (L1 strong, L2–L5 mid, L6 cheap), five
fix rounds, step 2 (mid, resumed once after a rate limit), six strong
assertion audits and one re-audit, plus six first dispatches that stopped at
once on the worktree base. Skills: wave-run and wave-close once each; the
subagents' model-shape and test-failure use doesn't show in the stats. Against R1 (69
coordinator turns, 145k peak, about 64 subagent turns), RS cost about ten
times as much, as expected for a wave that rewrote every area rather than
moving it; the coordinator's peak came from merging six worktrees in one
session.

## R3 — gogo convergence (done 2026-09-30, branch `refactor/r3-gogo`)

**Built.** gogo v0.1.0 (can3p/gogo#6), a breaking release written by the
coordinator: `settings` (go-flags `Parse` that treats an empty required
setting as missing and names flag and variable; `Secret`), the Mailjet
`BaseURL`, `apperr`, pcom's `render.go` as gogo's `ginhelpers` configured by
a `Configure` middleware, a constant-time CSRF compare, and no executor in
`sender` or `forms`, so gogo no longer depends on sqlboiler. In pcom, three
sequential mid tasks: G1 moved the mail queue interface into
`repo.MailQueue`, G2 dropped the executor from forms (every `Save` makes at
most one writing service call, so losing the handler's transaction is
safe), G3 replaced pcom's `render.go` and `csrf` with gogo's and made
`pkg/service`'s errors aliases of `apperr`. go.mod pinned the PR commit
while the tasks ran and moved to the tag at the end; the tag's tree is that
commit. Behavior is unchanged; `e2e/` assertions are untouched.

**Turned out wrong.** The planned "ORM-agnostic executor" was the wrong
fix: after RS nothing in gogo needed a database handle, so it was removed
instead of abstracted. Step 3 of the old plan (switch pcom to gogo's
`util.InCluster`) was pointless churn; gogo dropped it. The e2e suite
replayed cached passes after production edits (the server is a separate
binary), so subagents' "e2e pass" reports could be stale; found by a
mutation that disabled CSRF, fixed in the harness. CI's first run hit a
harness flake: `freePort` releases the port before the server binds it,
and another process took it ("address already in use"); `Start` now
retries on a new port. G1 weakened a dbsender
assertion to `NotNil`; G3 used `apperr` names where pcom uses `service`
ones and left a stale `pkg/forms` exception in the arch test. All fixed by
the coordinator.

**Left out.** Moving pcom code into gogo (the media server, goldmark
extensions, CSP, ...): most of it reads settings R2 rewrites, so it waits
until after R2. go-flags and the Mailjet base URL in pcom are R2's.

**Cost.** 1 coordinator session (129 turns, peak 253k context, 200k of tool
results, most of it the gogo work and its survey; flags: `cat`×17, raw
`go build`/`go test`×8) and 3 mid subagents (G1–G3, about 16 turns and 54k
peak each, none resumed or escalated); no separate audit agents: the
coordinator reviewed each diff and ran one mutation per task. Skills:
wave-run×1, wave-close×1; the subagents' skills weren't needed for
signature changes. Far cheaper than RS (207 coordinator turns, 20
subagents), because the tasks were mechanical and the contract (gogo) was
written before dispatch.

## R2 — go-flags config, single binary, tommy for mail and S3 (done 2026-09-30, branch `refactor/r2-config`)

**Built.** Every setting is a go-flags option in `pkg/config` with paired
`long:`/`env:` tags, parsed with gogo's `settings.Parse`; `web serve --help`
(pinned by goldens in `cmd/web/testdata`) is the configuration reference.
All 14 production secrets and the variables fly and the Dockerfile set keep
their names. `FLY_APP_NAME` is gone from the web binary: `SECURE_COOKIES`,
`HSTS`, `STATIC_CACHE`, `MEDIA_PERMA_CACHE`, `SHOW_ERRORS`, `REPORT_PANICS`
and `LOG_LEVEL` replace it, defaulting to production values (D1), turned off
in compose, `.env.example` and the E2E harness. New: `MJ_API_BASE`,
`EMAIL_POLL_INTERVAL`, `USER_MEDIA_PATH_STYLE`, `HTML_DIR` (fixes #111).
Mail always goes through gogo's Mailjet sender (console sender deleted), to
tommy in development and tests; user media always go to S3 (local storage
deleted), tommy's bucket locally; the S3 client no longer reads `AWS_*` or
`~/.aws`. `cmd/seed` and `cmd/scripts` became `web seed`, `web admin invite`,
`web admin registration`, `web debug feed`; `seed` keeps its `FLY_APP_NAME`
guard (D3). Migrations run at deploy from `sql-migrate` in the production
image as fly's `release_command` (D2; no migration library in go.mod). E2E
and browser tests assert on mail tommy received and objects in its bucket
(`pkg/testutil/tommy`, one container per test binary, capacity raised to
100000 events). Tests: one row per variable (value, default, required),
secrets don't print, an effect test per setting, serve's settings-to-config
mapping one setting at a time, and an arch rule that no package outside
`cmd/web` reads the environment or imports the AWS config loader.

**Wrong or surprising.** go-flags refuses a default on a `bool`, so D1's
default-on settings needed `config.Switch` (`--x=false`, `X=false`; empty is
an error), which also changes `ENABLE_PPROF`: `1` enables it, empty fails
startup. C0 had to take the tommy container, harness env and compose from
later tasks, because dropping the console sender broke E2E otherwise. The
first test audit found 12 weak tests and 12 settings tested only for
parsing; the second found E's negative mail checks relied on a blind 1s
wait and shared addresses, fixed with `App.WaitMailSent` (the one place e2e
knows about the queue) and unique addresses. `ObjectExists` returned an
error for a missing key on real S3 (HeadObject gives `NotFound`, not
`NoSuchKey`); fixed. The production image can't be built on an arm Mac
(Sass under amd64 emulation); the sql-migrate layer was proven with a
native image of the same lines. The coordinator lost a subagent's
uncommitted file by reverting a mutation with `git checkout`; the agent
recreated it.

**Left out.** Bugs found and filed, behavior kept: #182 (the panic reporter
itself panics outside the auth group), #183 (a /static 404 gets the
immutable cache header, pinned by a skipped test). Local `cmd/web/user_media`
files from before R2 stay ignored, not migrated. `config.Switch` and the
tommy test container are gogo candidates (`docs/gogo-extraction.md`); a
`TOMMY_CAPACITY` env var and a Go read-back client are optional tommy
improvements.

**Cost.** 1 coordinator session (175 turns, peak 297k context, 188k of tool
results; flags: `cat`×14, raw `go build`/`go test`×5) and 10 subagents
(A, B, S, X mid; E strong, resumed once for audit fixes; D cheap; T1, T2 mid
for audit findings; 2 strong audits), median 37 turns and 78k peak; S was
resumed once to redo the file the coordinator lost. Skills: wave-run×1,
wave-close×1. Dearer than R3 (129 turns, 3 subagents) because the wave had
real contract work, two audit rounds and a harness rewrite; the audits paid
for themselves (24 weak tests and 2 real bugs).

## F1 — Public post feed on the index page (done 2026-10-01, branch `feat/f1-public-feed`, #146)

**Built.** The first feature wave. For an anonymous visitor, `/` lists the
50 newest posts of the public feed (Q15: published, public, by a public
profile) under a "Public posts" heading with an RSS icon, in place of the
landing page (hero, demo video, selling points). `/rss/public` serves the
same posts; both use `reading.PublicPosts` and one `publicPostsLimit`. The
page head advertises the feed. The anonymous menu is Home · Sign up · Login;
anonymous `/explore` redirects to `/`, logged in users keep it; the navbar
links to GitHub; `why.md` is gone, `/articles/:id` stays for the legal
pages. The feed's post item is a partial shared by the feed and the index.
RSS items in every feed are dated by publication, not creation;
`rss.ToFeed` lost its unused author parameter; `web.ProjectName` replaces
the hardcoded name. `task_prompt.py` got a feature preamble.

**Wrong or surprising.** The plan first dropped `/explore` for everyone;
the owner kept it for logged in users, who also see registered-users
profiles there. P1 copied the feed's post markup into the index because
`feed.html` wasn't in its owned files; sent back to extract a partial. P1
and P2 each defined the 50-post limit. The audit found the E2E index and
feed tests repeating the service test's Q15 matrix, a pubDate check that
couldn't fail (the factory sets creation and publication to the same
instant) and a router assertion that matched either RSS link; all fixed.

**Left out.** No pagination ("load more" is a later issue). No test pins
the 50 on the routes, or that `/` and `/rss/public` list the same posts.

**Cost.** 1 coordinator session (76 turns, peak 163k context, 132k of tool
results; flags: `cat`×6 in the coordinator, 16 and raw builds×2 across the
wave) and 5 subagents (P0 strong; P1 mid, resumed once; P2 mid; audit fixes
mid; 1 strong audit), median 13 turns and 55k peak. Skills: wave-run×1,
wave-close×1, frontend-htmx×1 (P1). Much cheaper than R2 (175 turns, 10
subagents): three small tasks, one audit round, and the planning happened
in the same session.

## F2 — Comment editing, with notifications (done 2026-10-01, branch `feat/f2-comment-edit`, #176)

**Built.** `posts.EditComment`: the comment's author edits it while they
may still comment on the post; anybody else and a missing id get
`ErrNotFound`, anonymous `ErrNeedsLogin`. A changed (trimmed) body is
stored with the new `post_comments.edited_at` and mails the post's author
and the other participants an "edited" variant of the two comment mails
(the editor none); an unchanged body writes and sends nothing, and an edit
doesn't bump the new-comment counter. `addComment`'s notification code is
now `notifyComment`, shared by both paths. `POST
/controls/form/edit_comment/:id` reloads the page; `form--comment.html` has
an edit mode opened inline by an "Edit" button on the viewer's own
comments; "(edited <time>)" shows on the post page and in the feed's
comment item.

**Wrong or surprising.** The plan missed the mail queue's dedup on (type,
unique id): reusing `comment.ID + recipient.ID` would have dropped every
edit mail, so edited mails add `edited_at` to the id (caught while filling
the prompt, pinned by a two-edit test and a mutation). The hidden,
prefilled edit form put each comment's text on the page twice, so two
existing browser assertions now look inside `.post-user-home`. The mail
functions' new `edited` flag forced a compile fix in `pkg/mail/escape_test.go`,
outside C0's owned files. Docker Hub rate-limited the sandbox (429):
`make generate` ran with the pinned sqlboiler/sql-migrate on the host, and
the tommy image came from `mirror.gcr.io`. Both audits found gaps after the
mutation checks passed (a no-op edit tested only on an already edited
comment, a mail body never checked, a form test duplicating the service
test, a browser text check satisfied by the hidden textarea, no feed marker
check); all fixed in one round each.

**Left out.** No edit history, no time limit, no edit link in the feed.
The "(edited …)" time text itself is not pinned, only the word.

**Cost.** 1 coordinator session (66 turns, peak 143k context, 84k of tool
results; flags: `cat`×4 in the coordinator, `cat`×12 and raw builds×2
across the wave) and 4 subagents (C0 mid, resumed once, 29 turns, 91k
peak; C1 mid, resumed once, 19 turns, 77k peak; 2 strong audits, 5–6
turns). Skills: wave-run×1, wave-close×1, frontend-htmx×1 (C1). A little
cheaper than F1 (76 turns, 5 subagents): two sequential tasks, but about a
third of the coordinator's turns went to the Docker Hub outage and to
re-running the sandbox-only browser failures against the parent commit.

## F3 — Public profile section on the blog page (done 2026-10-01, branch `feat/f3-profile`, #186)

**Built.** A user writes a free-form "About" text in a new "Public profile"
card in the settings (the comment editor's toolbar and image upload),
saved by `POST /controls/form/save_profile`. The journal shows it rendered
under an "About" heading in a `us-profile-about` block, only when it is not
empty. It lives in a new `user_profiles` table; `accounts.SaveProfile`
validates it (`ValidateProfileAbout`, at most 6,000 characters), trims it
and deletes the row when it is empty; `reading.Journal.About` is loaded
after the visibility checks. A new view type `ViewProfile` and template
function `markdown_profile` render it like a comment.

**Wrong or surprising.** The plan expected a `ViewComment` case in
`pkg/markdown/html.go` to copy; there is none (comments take the default
branch), so only the view table test changed. A1's owned files missed
`pkg/web/pages_reading.go`, the page struct between `reading.Journal` and
`user_home.html`; A1 stopped and the coordinator added the field.
`make generate` can't build the tools image in the sandbox; A0 ran
`generate.sh` on the host. `factory.WithProfileAbout` parks the text in a
package-level map until the user row exists, because a `UserOpt` only sees
the user before insert. The per-user RSS feed reuses `reading.Journal`, so
it loads the text without showing it (one extra query).

**Left out.** The text appears nowhere but the journal (not in the RSS
feeds, the feed or explore). No translation (F7 scopes profiles out).

**Cost.** 1 coordinator session (41 turns, peak 128k context, 72k of tool
results; flags: `cat`×5 in the coordinator, 13 and a raw build across the
wave) and 2 mid subagents (A0: 21 turns, 85k peak; A1: 11 turns, 71k peak),
no audit agent: the coordinator read the two short test diffs itself and
ran one mutation per task. Skills: wave-run×1, wave-close×1,
frontend-htmx×1 (A1). Cheaper than F1 (76 turns, 5 subagents): two
sequential tasks, no rework beyond one missing file.

## F4 — Pagination of the feed, explore, index and journal (done 2026-10-01, branch `feat/f4-pagination`, #124)

**Built.** Cursor pagination everywhere a list was unbounded. The five
repository list methods take a `repo.Page` ordered by (sort time, kind, id)
descending, with `limit + 1` rows; `reading.Feed` merges posts, RSS items
and comments and cuts at `reading.PageSize` (30). The cursor is opaque
(`reading.Cursor`: microsecond time, kind, id, base64url); a bad one is
`service.Invalid`. The feed's first page alone carries connections,
prompts and the feed token; the posts-only feed mode is gone. RSS outputs
(`/rss/private/:token`, `/rss/public/:username`, `/rss/public`) list the
newest `reading.RSSLimit` (50, F1's `publicPostsLimit` moved) posts.
`/feed`, `/explore`, `/` (anonymous) and `/users/:username` get a shared
`partial--load-more.html` link: htmx replaces it with the next items and a
new link; without JS it renders the full page from the cursor. The feed's
item loop is `partial--feed-items.html`; the journal's posts render through
`partial--post-list.html`. No index migration: `explain` on the seeded
database showed the journal query already using `posts_user_id_idx`.

**Wrong or surprising.** R0's owned files missed four callers of the
changed signatures (`service/feeds`, `web/pages_reading.go`, two web
tests); it returned patched copies under `needs:`. The audit of R0's tests
found no bug in the paging code but large gaps: second-precision test times
(a millisecond cursor would pass), an order check that sorted with the
code's own comparator, no exactly-full last page (limit+1 untested), no
source large enough for the per-source limit to bite, one bad-cursor
variant. All fixed; a millisecond cursor now fails three tests. R1's first
browser tests survived two mutations (no `hx-get`; handler always returns
the full page) because body `hx-boost` fetches in place; sent back with a
structure check and an HTTP fragment/full-page/400 test, and R2 was told up
front. Two browser tests (`TestWriting_PostVisibility`,
`TestActions_ShareLifecycle`) fail in the sandbox on inline-style CSP
reports only, as `docs/testing.md` documents.

**Left out.** No factory options for sort times (`PublishedAt`, comment and
feed item `CreatedAt`): tests use inline option closures. `/rss/public`
and the index still aren't pinned to list the same posts. No assertion
audit agent for R1 and R2: their tests are few and the coordinator's own
mutations (both caught after the fix) stood in.

**Cost.** 1 coordinator session (60 turns, peak 148k context, 78k of tool
results; flags: `cat`×3 in the coordinator, 18 and a raw build across the
wave) and 4 subagents (R0 strong, resumed once for the audit fixes; R1 mid,
resumed once; R2 mid; 1 strong audit), median 19 turns and 81k peak.
Skills: wave-run×1, wave-close×1. A little cheaper than F1 (76 turns, peak
163k) for a larger change: prompts came straight from `task_prompt.py`, and
R2 got R1's lesson in its prompt, so it needed no second round.
## F6 — Public website on GitHub Pages (done 2026-10-01, branch `feat/f6-website`, #145)

**Built.** A static site for `can3p.github.io/pcom`, ported from tommy's
website: its own Go module in `website/` (goldmark only), the page list in
Go (`docPages`), a landing page made only of slices of `README.md` and the
guide pages, links rewritten through one map, a missing page or image
failing the build, and every internal link crawled by a test. `pages.yml`
deploys on push to master; a CI job vets, tests and builds `website/` on
PRs. A user guide in `docs/guide/` (eight pages, each opening with its
landing-card text) and `make screenshots`, a runner behind the
`browser,screenshots` tags that seeds the E2E app and writes seven PNGs.
The README links the site, drops the dead `why.md` link and names its
"Quick start". The wave skills, the feature preamble and `AGENTS.md` now
require every F-wave to end with its docs task.

**Wrong or surprising.** Before F4's paging data, the seed world had no
visible public posts (Alice's profile is connections-only; Carol, the
public profile, had none), and its comment threads still showed "No
Comments yet": the factory doesn't maintain `post_stats`. The seed now sets
the counts; both were caught only by looking at the screenshots. F2–F4
merged while F6 was open, so the wave rebased and documented them too. The editor has no inline preview (it's a separate page), so
`editor.png` shows typed markdown. A strong fact check of the guide against
the code found 15 errors in the mid-tier draft, mostly plausible
generalizations (every account gets an invitation, everyone sees a
journal's public posts, unsubscribe from the feed); all corrected. The S0
audit found the "a card per guide page" test circular (it read the
hand-kept list it was meant to check) and links above the repo root
silently clamped. Bugs filed: #191 (publishing a published post re-notifies),
#192 (a logged-in stranger sees fewer journal posts than an anonymous
visitor), #193 (the import text in Settings is wrong).

**Left out.** No pixel diffing of screenshots. F5 and F7 document
themselves. `/posts/<id>/md` and `/zip`
have no UI link; the guide gives the addresses.

**Cost.** 1 coordinator session (78 turns, peak 169k context, 447k of tool
results, much of it the seven PNGs read to check them; flags: `cat`×2,
raw build×1 in the coordinator) and 7 subagents (S0, S1, S2 mid, S0 and S2
resumed once for audit fixes; S3, S4 cheap; 2 strong read-only audits),
median 17 turns and 93k peak. Skills: wave-run×1, wave-close×1. Close to
F1 in coordinator turns, with more subagents: the fact check of prose was
new and paid off more than the test audit did.

## F5 — Login with an emailed code, passwords removed (done 2026-10-01, branch `feat/f5-login-codes`, #175)

**Built.** Login, invitations and signup work without passwords. The login
page takes an email and starts a login attempt (`login_attempts`, its id in
the session); a confirmed user under the limit is mailed a 6-digit code
(HMAC-SHA256 keyed with `SESSION_SALT`, 15 minutes, one use, 5 tries, 3
mails per user per 15 minutes); an unknown or unconfirmed address gets the
same page and no mail. The code form posts to `/form/login/code`, which
checks the code against the session's attempt only, rotates the session and
follows the attempt's return URL. Accepting an invitation creates a
confirmed user and mails a code in the same transaction; an open signup
mails the code in the confirmation mail, and the first good code confirms
the address (and sends the existing admin notice). `/confirm_signup/:id`,
the "Change Password" card, `Authenticate`, `ChangePassword`, the argon2
hashing, `factory.WithPassword` and the seeded passwords are gone;
`golang.org/x/crypto` is indirect. `web admin login-code --email` prints a
fresh code for the user's open attempt when mail is down (Q22). The E2E
`LoginAs(email)` issues the code through the accounts service instead of
waiting for mail. M4, the migration dropping `users.pwdhash`, is on the
stacked branch `feat/f5-drop-pwdhash`, to merge after F5 has run in
production.

**Wrong or surprising.** The plan put `LoginAs(email)` in M0, before M1 had
the routes it logs in through, and factory passwords are hashed with the
email, so M0 couldn't have changed it: moved to M1. M1 and M2 were planned
in parallel but share `routes_auth.go`, the browser accounts tests and M1's
code form: run in sequence. The plan said M4 merges with the wave but
deploys later; fly's `release_command` migrates on every deploy, so it
ships as a separate PR. Three audits found weak tests in every task: a
cross-session check that used an already dead code, wrong-code tests typing
"000000" (sometimes the real code), E2E tests repeating the service's tries
and lowercasing rules, no proof of atomicity or of the admin notice, and a
CLI test that a wrong key couldn't fail. M0's own test found that Postgres
rounds times to microseconds (an attempt could outlive its expiry by one):
the service clock truncates. The tools image couldn't build in the sandbox
(Docker Hub 429, apk TLS), so M0 and M4 generated models on the host with
the pinned sql-migrate and sqlboiler. The permission check refused a
subagent's `rm` of the files M3 deletes; the owner approved and the
coordinator removed them with `git rm`.

**Left out.** Per-IP limits (bot protection is #123). No browser test of
"Use a different email" (an E2E guard checks its link). The
`email_confirm_seed` column stays unused. The guide is not updated (F6 has
not merged).

**Cost.** 1 coordinator session (123 turns, peak 196k context, 105k of tool
results; flags: `cat`×4 in the coordinator, `cat`×37, full reads×2 and raw
builds×14 across the wave) and 9 subagents (M0 strong, resumed once; M1,
M2 and M3 mid, M1 and M2 resumed once; M3 dispatched twice after the
deletion block; M4 cheap; 3 strong audits), median 32 turns and 78k peak.
Skills: wave-run×1, wave-close×1; the subagents used none by name. Costlier
than F1 (76 turns, 5 subagents): four sequential tasks, three audit rounds,
and two plan corrections made mid-wave.

## F7 — Translating posts and RSS items to English (code done 2026-10-01, guide page pending; branch `feat/f7-translation`, #144)

Stacked on F4 (`feat/f4-pagination`, #194), which had not merged.

**Built.** A reader translates a post or an RSS item into English in place,
sees "Automatically translated from <language> by <provider> · Show
original" above it (with `lang="en"`; originals carry their detected
language), and can pick languages to always translate in a new settings
card. A post is translatable only if its author ticked "Allow readers to
translate this post" (`posts.allow_translation`, default off; the API's
optional `allow_translation` keeps the stored value when omitted); RSS items
always are. Pieces:
- `pkg/translate`: the `Backend` interface, a registry chosen by
  `TRANSLATION_PROVIDER` (empty turns the feature off everywhere), and the
  shared layer every backend gets: chunking under the backend's limits,
  retries on 429/5xx honouring `Retry-After`, and shape checks (segment
  count, `notranslate` spans by `data-i`) that reject rather than return a
  partial translation. `pkg/translate/azure` is Text API v3.0 only.
  Settings live under `TRANSLATION_*`, the backend's under its own
  namespace; `Validate` requires only the selected backend's.
- Language detection with `whatlanggo` (Q27), stored on every post save and
  RSS import.
- `pkg/markdown/translate.go`: `Segments`/`Apply` turn each block into one
  HTML fragment (emphasis as tags, links as `<a data-i>` without the URL,
  code, URLs and @handles as `notranslate` spans) and put translations back
  by index, escaping provider text so it cannot inject markdown.
- `pkg/service/translations`: `Translate` under the reading rules (drafts and
  posts the reader can't see are not found; RSS items need a
  subscription), a cache keyed on a hash of language, subject and body,
  per-user daily and site-wide monthly character budgets
  (`translation_usage`), one backend call per source for concurrent misses
  (singleflight); `Cached` for pages under the same rules; `Slot` for the
  fragment routes; `RetranslateStale`/`Forget` called from the post save
  transaction through `posts.Translations`; a worker that claims jobs on a
  lease, calls the provider outside any transaction and stores each job in
  its own.
- Pages build one `TranslationView` (one languages and one cache lookup per
  kind); the Translate button is server-rendered, and only always-translate
  languages auto-translate: cached ones inline, the rest with
  `hx-trigger=load`.
- `pkg/testutil/wiremock`: one WireMock container per test binary with JSON
  stubs next to the code they serve, `Watch` (unmatched calls fail the test,
  filtered by the test's own content) and `Verify`; `e2e.WithWireMock()`.
- The privacy policy describes what is sent, to whom and when.

**Wrong or surprising.** The plan said RSS items are sanitized HTML; the
poller stores them as markdown, so T1's HTML segmenter was built, found
unused by T4 and deleted, and RSS items go through the markdown path. T3
could not call `RetranslateStale`/`Forget` directly while they were T0
stubs returning errors, so it calls them through an interface T2 wired.
The audits found real bugs every time: T3 never forgot translations of a
post turned into a draft with the toggle off, nor re-translated a
republished one; T1 translated RSS `@mention` links and dropped autolink
brackets, and its identity round trip skipped every unchanged segment;
T2's `Cached` applied no read rules (any logged-in user could read a
translation of a post they couldn't see — never reachable, nothing called
it yet) and its worker held one transaction across all provider calls;
T4 first fired one request per item just to show a button, put rules in
handlers and panicked on one error path. learn.microsoft.com is blocked in
the sandbox; Azure's contract was checked against the
`MicrosoftDocs/azure-ai-docs` repository on GitHub instead, which showed the
region header is optional for global resources. Docker stopped once during
the close; the session-start hook restarted it.

**Left out.** The user guide page and screenshot: F6 merged while F7 was
open, so under its rule they are still owed before F7 is done. The
titles of `{cut}`/`{spoiler}` blocks are not translated, only their content.
No test provokes one worker job's SQL failure or two concurrent cache
misses; both are covered by reading the code. Only Azure is implemented.

**Cost.** 1 coordinator session (164 turns, peak 293k context, 202k of tool
results; flags: `cat`×7) and 11 subagents (TM, T1, T3 and T4 mid; T0 and T2
strong; 5 strong audits), median 18 turns and 86k peak; across the wave
`cat`×60, raw builds×14, full reads×2. T4 was resumed four times (RSS
format, per-page view, audit, coverage) and T0–T3 once each for audit fixes.
Skills: wave-run×1, wave-close×1. Far heavier than F4 (60 turns, peak 148k):
six tasks instead of three, every audit sending work back, and one
`make cover` output piped through a grep that matched its `-coverpkg` list
added a large result to the coordinator's context.
