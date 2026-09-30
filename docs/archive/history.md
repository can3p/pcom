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
