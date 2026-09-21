# Implementation plan: modernization

Forward-looking only. When a wave ships, **delete its section from this file**,
update the status table, and record what happened in `docs/archive/history.md`
(create that file with the first wave).

The end state:

- [go-flags](https://github.com/jessevdk/go-flags) configuration and a single binary with subcommands;
- [bob](https://github.com/stephenafamo/bob) instead of sqlboiler;
- declared, golden-tested mailers;
- a decomposed router;
- a docker-compose development stack (Postgres, S3-compatible object storage,
  and [tommy](https://github.com/can3p/tommy) as the mail sink) with every
  build tool in a container;
- shared plumbing in [gogo](https://github.com/can3p/gogo).

**None of that starts until the safety net exists.** Waves W0–W5 only add
tests, test infrastructure, a seed command and developer tooling.

Related documents:

- `docs/gogo-extraction.md`: what can move into the shared library, noted
  while surveying.
- `docs/open-questions.md`: decisions that are still open, and the ones
  already made.
- GitHub issues #108–#124: bugs and future work found during the survey.
  PR #118 fixes the API post-deletion hole.

---

## Status

| Wave | Name | Depends on | State | Branch |
|---|---|---|---|---|
| W0 | Test foundation | — (gogo#5 merged) | not started | `test/w0-foundation` |
| W1 | Unit tests, no database | W0 | not started | `test/w1-unit` |
| W2 | Package tests against Postgres | W0 | not started | `test/w2-db` |
| W3 | End-to-end HTTP tests | W0 | not started | `test/w3-e2e` |
| W4 | Local stack (Postgres, s3mock, tommy), dev tooling container, app in compose, seed | W0 | not started | `test/w4-local-stack` |
| W5 | Coverage ratchet | W1–W4 | not started | `test/w5-ratchet` |
| WB | Bug-fix wave (#108–#117, #119–#122) | W1–W3 | not started | `fix/wb-survey-bugs` |
| R1 | Router decomposition | W3, WB | planned | `refactor/r1-router` |
| R2 | go-flags config, single binary, tommy mail, object storage only | R1, R3 (mailjet BaseURL) | planned | `refactor/r2-config` |
| R3 | gogo convergence | W5 | planned | `refactor/r3-gogo` |
| R4 | Mailers | W1 (mail goldens), R2 | planned | `refactor/r4-mailers` |
| R5 | bob ORM | W2, W3, R3 | planned | `refactor/r5-bob` |
| R6 | Dependency hygiene | any time after W5 | planned | `chore/r6-deps` |

```
            ┌── W1 (12 tasks) ──┐
            ├── W2 (9 tasks)  ──┤
W0 ─────────┼── W3 (6 tasks)  ──┼── W5 ── WB ── R1 ── R2 ── R4
(1 session) └── W4 (5 tasks)  ──┘              │
                                               └─ R3 ── R5      R6: any time
```

**After W0 lands, W1, W2, W3 and W4 are independent of each other** and can
run at the same time: about 30 tasks in total, each owning disjoint files. Each
of those waves branches from `test/w0-foundation` (or `master` once W0 has
merged), not from each other.

Baseline, measured on 2026-09-21 on `master` at 091484d: every test passes, and
**17.2%** of statements are covered, excluding the generated `pkg/model/core`
(3.4% including it). At 0% are `cmd/web`, `pkg/auth`, `pkg/forms`,
`pkg/userops`, `pkg/web`, `pkg/mail`, `pkg/admin`, `pkg/links`, `pkg/pgsession`,
`pkg/postops/rss`, `pkg/media` (upload), `pkg/media/server/storage/*`,
`pkg/markdown/mdext/lazyload` and `pkg/util/ginhelpers/*`.

---

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
2. **Bugs are filed, not fixed.** If a test exposes a bug, file a GitHub issue
   with reproduction steps (label `bug`), and write the test for the *correct*
   behavior, skipped:

   ```go
   t.Skip("known bug: https://github.com/can3p/pcom/issues/NNN")
   ```

   WB removes the skips as it fixes the bugs. If the behavior is merely odd
   rather than wrong, write a characterization test that pins the current
   behavior, with a comment saying so. For a security bug, do not open a
   public issue. Report it to the coordinator, who asks the owner.
   Known issues are listed under "Known bugs" below. Check that list before
   filing, so the same bug isn't filed twice.
3. **Tests touch the ORM only through `pkg/testutil/factory`** to create
   fixtures and read state back. A test body that calls `core.Posts(...)`
   directly is a test R5 has to rewrite. This rule is what makes the bob
   migration cheap. The code under test obviously still uses `core`.
4. **Prefer black-box tests.** Use `package foo_test` and the public API unless
   an unexported function has logic worth pinning on its own, such as
   `ConstructComments` or `isURLMediaUpload`. Black-box tests survive
   refactors; tests of internals get rewritten by them.
5. **Assertions use `testify/require`** (and `assert` where continuing after a
   failure helps). Don't add new uses of `alecthomas/assert`, which R6 removes.
   Mocks use mockio v2 as documented in `AGENTS.md`, but prefer the fakes in
   `pkg/testutil`.
6. **Never assert on wall-clock time.** Nothing is injectable yet. Use
   `require.WithinDuration(t, time.Now(), got, 5*time.Second)`, or compare
   ordering.
7. **Every test gets its own database** (`testdb.New(t)`), and tests may run
   in parallel (`t.Parallel()` is encouraged for DB tests).
8. **Mail content is golden-tested** under `testdata/*.golden`, with the
   convention `UPDATE_GOLDEN=1 go test ./pkg/mail/...` to rewrite.
   These goldens are the safety net for R4.

## How waves are run

### Branching

- **One wave, one branch, one reviewable PR.** Never put two waves on one
  branch.
- `git fetch origin` first. If the wave you depend on has merged, or nothing
  is in flight, branch from `origin/master`. If it is still open, branch from
  **that wave's branch** and pass the same base to
  `gh pr create --base <parent-branch>`. A wrong base makes the PR show the
  parent's commits as its own.
- W1–W4 all depend only on W0, so each branches from W0's branch (or from
  `master` once W0 has merged). They are siblings, not a stack.
- When a parent merges, rebase the child onto `origin/master` and push with
  `--force-with-lease`.
- Merge wave PRs with a merge or rebase merge, **not a squash**. Squashing
  rewrites commits that stacked children already contain.

### Coordinating a wave

- A coordinating session (strongest model) dispatches the wave's tasks as
  subagents. Each task owns the files listed for it and nothing else.
- **Subagents run no git commands.** They don't touch `go.mod` (only W0 and W4.S2 do, each as a single task).
  They report gaps in `pkg/testutil` instead of patching around them. The
  coordinator adds missing factory helpers in one place, then re-dispatches.
  If two tasks independently ask for the same helper, it is real.
- The coordinator re-runs `make check` and `make cover` itself. Then it picks
  one test per task and breaks the code under it locally (and reverts) to prove
  the test catches something. A test that doesn't fail when you break the code
  under it isn't covering anything.
- The coordinator commits per task, puts docs last, opens the PR and watches CI
  to green.
- A wave is finished when the documents are accurate again, not when the code
  lands:
  1. Delete the wave's section here and update the status table. Edit later
     waves if what you learned changes them.
  2. Append to `docs/archive/history.md` what was built, what turned out
     wrong, and what was deliberately left out.
  3. Add generalizable lessons to `docs/lessons.md` (create both files on
     first use).
  4. Move answered items in `docs/open-questions.md` to "Decided".
  5. Update `AGENTS.md` if a rule, command or convention changed.
  6. Make logically split commits, with the docs commit last. Never a single
     "wave complete" commit.
  7. Push, open the PR, and watch CI with `gh pr checks --watch`. On a
     failure, read `gh run view --log-failed`. **A wave with red or pending CI
     is not finished.**
  8. Report back and stop. Merging is the owner's call.

### Model tiering

| Tier | Model | Use for |
|---|---|---|
| strong | Opus 5 (`claude-opus-5`) | W0; wave coordination; visibility/permission tests (W2.D2a, W3.E1); R1 skeleton; R5 planning |
| mid | Sonnet 5 (`claude-sonnet-5`) | business-logic tests (connections, forms, feed composition), E2E scenarios, WB fixes |
| cheap | Haiku 4.5 (`claude-haiku-4-5-20251001`) | pure-function unit tests, golden tests, factory-driven CRUD checks, docs, CI config |

The rule of thumb: if a mistake would be caught by a test the task writes
itself, the task can go cheap. If a mistake would go unnoticed,
because a permission test passes when it should fail, the task needs a stronger
model.

### Task prompt template

Every subagent prompt starts with this preamble, filled in:

```
You are adding tests to pcom (Go, gin, sqlboiler, Postgres). Read AGENTS.md,
docs/implementation-plan.md ("Ground rules for W0–W5"), and docs/testing.md.
Task: <id and title from the plan>.
You own exactly these files: <list>. Do not edit any other file.
Do not change production code. Do not run git. Do not edit go.mod.
Create fixtures with pkg/testutil/factory. If a helper is missing, stop and report
exactly what you need rather than writing ORM calls in your test.
If you find a bug: write the test for correct behavior, add
t.Skip("known bug: <describe>"), and include a reproduction in your report —
the coordinator files the issue and fills in the number.
Done when: `go test ./<your packages>/...` passes, `go vet` is clean, and the
coverage of <package> is at least <target>%. Report the coverage you reached,
the bugs you found, and anything you could not test without a code change.
```

---

## W0 — Test foundation

**One session, strong model, sequential.** Everything else depends on the
contracts written here. Write them first, commit, then build.

### T0.1 Test database

Owns `testcontainers/postgres/` and `pkg/testutil/testdb/`.

**Available in gogo:** the upgraded `testcontainers/postgres` was merged in
[can3p/gogo#5](https://github.com/can3p/gogo/pull/5) (`51eb4cb`). It isn't
tagged yet. Tag `v0.0.2` on gogo, or pin the pseudo-version with
`go get github.com/can3p/gogo@51eb4cb`. It provides:

- testcontainers-go instead of dockertest;
- migrations applied once into a template database, with each test database
  created by `CREATE DATABASE … TEMPLATE …`. There are 38 migrations today,
  and W2/W3 will create hundreds of databases;
- a per-run migration bookkeeping table (`WithMigrationsTable`, default
  `migrations`). pcom's copy today uses sql-migrate's Go default,
  `gorp_migrations`, while `dbconfig.yml` and the CLI use `migrations`;
- `New(t, WithMigrationsDir(dir), …)`, which returns
  `TestDB{DB *sqlx.DB, SQL *sql.DB, URL string}` and registers `t.Cleanup`,
  plus `Cleanup()` for `TestMain`. The old `NewTestDB(Options)` still works,
  but is deprecated.

The task:

- Bump gogo, **delete pcom's own `testcontainers/postgres`**, and point the
  existing tests at gogo's package.
- Add `pkg/testutil/testdb.New(t testing.TB) *postgres.TestDB`, a thin
  wrapper that resolves pcom's `migrations` directory relative to its own
  source file and calls `postgres.New(t, postgres.WithMigrationsDir(dir))`.
  The E2E harness hands `TestDB.URL` to the binary.
- Move the existing callers from `NewTestDB` to `testdb.New`: `pkg/feedops`,
  `pkg/feedops/feeder` and `pkg/web`. That's a test-only change.

### T0.2 Factories: `pkg/testutil/factory`

**Must not import `testing`**, because `cmd/seed` uses it. Style:

```go
func User(ctx context.Context, exec boil.ContextExecutor, opts ...UserOpt) (*core.User, error)
func WithPassword(pw string) UserOpt       // hashes via pgsession.HashUserPwd, sets EmailConfirmedAt
func WithVisibility(v core.ProfileVisibility) UserOpt
```

Defaults must satisfy every constraint and be unique through an atomic counter:
`user7@example.test`, username `user7`, timezone `UTC`, confirmed.

Required builders (one per table, plus the relationships the domain needs):

- Users and graph: `User`, `Connect(a, b)` (both directions, as
  `userops.CreateConnection` does), `Whitelist(who, allowsWho)`,
  `MediationRequest(who, target, …)`, `MediatorDecision(req, mediator, decision)`
- Posts: `Post` (draft by default, with `Published()`, `Visibility(v)` and
  `WithURL(u)` options), `Comment(post, author, …ReplyTo(c))`, `PostStat`,
  `PostShare`, `PostPrompt(asker, recipient, …)`, `NormalizedURL`
- Account: `Invitation(user, …Sent(email))`, `SignupRequest`, `APIKey(user)`,
  `UserStyle(user, css)`, `SetRegistrationOpen(bool)`
- Feeds: `RSSFeed`, `Subscription(user, feed)`, `RSSItem(feed, …)`,
  `UserFeedItem(user, item, …)`
- Media and mail: `MediaUpload`, `OutgoingEmail`
- Readers: `GetUser`, `GetPost`, `ListPosts(userID)`, `ListComments(postID)`,
  `ListOutgoingEmails(filter)`, `ConnectionExists(a, b)`,
  `GetMediationRequest(id)` and so on. Add readers as tasks report the need;
  keep them in `factory/read.go`.

Write a self-test that builds every entity once. It proves the defaults
satisfy the schema.

Leave `pkg/feedops/testutil` alone. Moving its callers over is optional
clean-up for W2.D8.

### T0.3 Fakes and helpers: `pkg/testutil`

- `fakesender`: implements `gogo/sender.Sender`. Records every `*sender.Mail`
  with its `uniqueID` and `emailType`. `FailWith(err)` makes `Send` return an
  error. Note that the code under test currently `log.Fatal`s on that error
  (#112), so failure-path tests stay skipped until WB. For unit and package
  tests this fake stays the right tool after R2. tommy (see R2) is for the E2E
  and development paths, where real delivery is what is being tested.
- `fakestorage`: an in-memory `server.MediaStorage`, with injectable errors.
- `ginctx`: `New(t, method, target, body) (*gin.Context, *httptest.ResponseRecorder)`.
  It installs a cookie session store under the name `sess`, as `main.go` does.
  It has an option to set a logged-in user (`pgsession.SetUser` needs the
  `*sqlx.DB`) and one to set CSP nonces.
- `golden`: `golden.Assert(t, name string, got []byte)` with `UPDATE_GOLDEN=1`.
- `testutil.Must[T](t, v T, err error) T`.

### T0.4 End-to-end harness: `e2e/`

A black-box harness that runs **the real binary**. That way W3's tests survive
R1 (router decomposition) and R2 (config) unchanged, and they are exactly what
proves those refactors safe. No production code changes are needed:

- `TestMain` builds `./cmd/web` once, with `go build -cover -o $TMP/web`.
- `e2e.Start(t) *App` does the following:
  - Creates a test database.
  - Makes a temp working directory with a symlink `client` → `cmd/web/client`,
    and a generated `dist/manifest.json`. The manifest maps every
    `static_asset` key used by templates to a stub file (`main.css`, `main.js`
    and the favicons; grep the templates). `static_asset` panics on unknown
    keys, so the harness greps the key list from the templates instead of
    hardcoding it.
  - Picks a free port, and starts the binary with `PORT`, `DATABASE_URL`,
    `SESSION_SALT=test`, `SITE_ROOT=http://127.0.0.1:<port>`, and
    `GOCOVERDIR` (when set). It does *not* set `FLY_APP_NAME`.
  - Waits for `GET /` to return 200, kills the process on cleanup, and dumps
    its stdout/stderr on failure.
- Note: the binary starts the email and RSS pollers. Until R2, emails go to
  the console sender, so tests assert on the `outgoing_emails` queue. R2 adds
  a tommy container to the harness and asserts on delivered mail through
  tommy's API. A feed
  created in a test must point at an `httptest.Server` that the test owns,
  never at the internet.
- `App.Client(t)` returns a cookie-jar client that does not follow redirects.
  It offers `Get`, `PostForm`, `PostJSON(action, v)` and
  `LoginAs(email, password)`. The CSRF token is scraped from
  `<body hx-headers='{"X-CSRFToken": …}'>` and sent as the `X-CSRFToken`
  header. Helpers assert on htmx response headers (`HX-Redirect`,
  `HX-Trigger`, `HX-Retarget`, `HX-Replace-Url`).
- HTML assertions use `github.com/PuerkitoBio/goquery` (new test-only
  dependency). CSS selectors keep cheap-model tests robust.
- W0 ships a smoke test: an anonymous `GET /` returns 200, and a user created
  by a factory can log in and reach `/feed`.
- CI needs no frontend build and no libvips for E2E beyond what is already
  installed.

### T0.5 Tooling and docs

- `Makefile`: `test` writes `coverage.out`. CI already uploads that file to
  Codecov, but nothing produces it today. `cover` runs unit and E2E tests,
  merges the binary coverage (`go tool covdata textfmt`), and prints a
  per-package table that excludes `pkg/model/core`. Add `test-short`, which
  skips E2E via `testing.Short()`.
- Add `codecov.yml` ignoring `pkg/model/core/**`.
- Write `docs/testing.md`: the ground rules above, how to use each helper, and
  one worked example each of a unit test, a DB test and an E2E test. Cheap
  models learn from the examples, so make them exemplary.
- Add a pointer from `AGENTS.md` to `docs/testing.md` and to this plan.

**Done when:** `make check` is green, the factory self-test and the E2E smoke
test pass in CI, and `make cover` prints the per-package table.

---

## W1 — Unit tests, no database

**12 parallel tasks.** No task needs Docker. Each owns only the new `_test.go`
files named below (plus `testdata/` in its package).

| Task | Package(s) | Focus | Tier | Target |
|---|---|---|---|---|
| U1 | `pkg/links`, `pkg/links/media` | Every `Link` name, including query strings and the comment fragment. `AbsLink` with and without the `USER_MEDIA_CDN`/`FLY_APP_NAME` env (use `t.Setenv`). YouTube transformer. `MediaReplacer`. | cheap | 90% |
| U2 | `pkg/util` (`tz.go`, `string.go`, `pointer.go`), `pkg/util/formhelpers` | `SplitLines`, TZ list sanity (every entry loads via `time.LoadLocation`), `FormatDuration`, the htmx header helpers `Retarget`/`Trigger`/`ReplaceHistory` (assert headers through `httptest`) | cheap | 80% |
| U3 | `pkg/forms/validation` (`email.go`, rest of `fields.go`) | Username and password rules, `ValidateMinMax`, `ValidateEnum`, `AttributionRE`. The disposable-email branch of `EmailOKToSignup` with a fake sender (it doesn't touch the DB unless it passes); keep the DB branch for W2. | cheap | 90% |
| U4 | `pkg/markdown`, `mdext/lazyload`, `mdext/blocktags`, `mdext/videoembed` | `extract.go` (image/link extraction), `ToEnrichedTemplate` for **every** `types.HTMLView` with one representative document (golden), lazyload renderer, the remaining blocktags and videoembed branches | cheap | 85% |
| U5 | `pkg/postops` (pure parts), `pkg/postops/rss` | `ConstructComments`: nesting, ordering, levels, orphans (a table test is worth a lot here). `CanSeePost` × `GetPostCapabilities` over the full radius × visibility matrix. `SerializePost` golden. `isURLMediaUpload`. `SerializeBlogSlice` with `fakestorage` (missing images, and #110's double close shows up as a logged error). `DeserializeArchive` edge cases. `rss.ToFeed`. | mid | 70% (pure parts) |
| U6 | `pkg/userops` (`profile.go`, radius methods) | `CanSeeProfile`/`CannotSeeProfileLite` over the full visibility × visitor × radius matrix | cheap | 100% of those files |
| U7 | `pkg/media` (`ValidateImageType`, `HandleUpload` argument checks) | Don't test `storage/local`: R2 deletes it. Don't test S3 here either: R2 adds S3 tests against the compose object storage. | cheap | 80% of `upload.go` |
| U8 | `pkg/util/ginhelpers`, `ginhelpers/csp`, `ginhelpers/csrf`, `pkg/pgsession` (`hash.go`) | `HTML`/`API` error→status mapping, with and without `FLY_APP_NAME`. CSP header shape and fresh nonces per request. CSRF: missing, wrong and right token, via header and via the `header_csrf` form field (sessions through `ginctx`). **`HashUserPwd` golden values**: a characterization test with fixed inputs → fixed hex. It guarantees no refactor silently logs every user out. | mid | 90% |
| U9 | `pkg/auth` (pure parts) | `HashValue`, `RedirectToLogin` (signed `return_url`), `EnforceReferer`, `EnforceAuth` (redirect), `AuthAPI` header parsing (the 400 paths only, since there's no DB), `Logout` headers, flashes, `GetUserData` CSRF token creation | mid | 50% (the rest is W2.D5) |
| U10 | `pkg/mail` (the functions that only format and send) | Golden subject/text/html for each mail, using `fakesender`: new_post, post_comment_author, post_comment_participants (including the "not to myself" early returns), post_prompt, post_prompt_answer, confirm_signup, confirm_waiting_list. Include awkward variants: a post with no subject, a linked URL, a subject with `<b>` and `"`. The HTML-escaping case is #120; write it as a skipped test. | cheap | 80% |
| U11 | `pkg/admin` | Notification goldens with `fakesender`, and `ClonedCustomRecovery` output shape (stack frames present, no panic) | cheap | 70% |
| U12 | `cmd/web` (`package main`, new `main_test.go` only) | **Template compile test**: parse `client/html/*.html` with `funcmap(stub)`; every template must parse. This is the most valuable single test for R1. `toMap` (even and odd args), the `markdown_*` funcs registered, `loadStaticManifest` (run from a temp dir with a fake manifest: CDN prefix only in cluster, panic on an unknown asset), `articlesRE`, `enforceEnvVars` panics | mid | as far as it goes without `main()` |

Also `pkg/feedops/reader` (`fetcher.go` with an `httptest.Server`: size cap,
timeout, MIME validation, redirects; and `fetch_time.go`) goes to **U7b**
(cheap, target 85%). It's listed separately because it's independent of media
storage.

**Done when:** every task's package meets its target, and `make check` passes
with no Docker running (`make test-short`).

---

## W2 — Package tests against Postgres

**9 parallel tasks**, all using `testdb.New(t)` and `factory`.

| Task | Package / files | What to pin down | Tier | Target |
|---|---|---|---|---|
| D1 | `pkg/userops/connections.go` | `CreateConnection` symmetry. `GetDirectAndSecondDegreeUserIDs` on a graph fixture with a triangle, a chain, an isolated user and several common friends, checking the `via` map. `GetConnectionRadius` for every case, including an empty user ID. `EstablishConnection` (whitelisted or not, and revoking a pending mediation). `DropConnection` clean-up of the whitelist, mediation and mediators. `RequestMediation` guard rails. `RevokeMediationRequest`. `DecideForwardMediationRequest` (only a common direct connection may decide). `DecideConnectionRequest` approve and dismiss. #117 as skipped tests. | mid | 85% |
| D2a | `pkg/web/func.go`: `SinglePost`, `UserHome`, `Explore`, `SharedPost` | **The privacy matrix.** For each of anon, registered-unrelated, second-degree, direct and author × each post visibility × each profile visibility × draft or published: can they see it? Which error (`ErrNeedsLogin` vs `ErrNotFound`)? Which capabilities? Comments visible? Share visible? Write it as one table-driven test that reads like a spec. This is the most important test in the repository. The product is "private social network". | **strong** | 90% of these functions |
| D2b | `pkg/web/func.go`: `Feed`, `getComments`, `Controls`, `Settings`, `Write`, `EditPost`, `Invite`, `Index`, `Login` | Feed composition: direct posts, second-degree posts filtered by visibility, `via` users, RSS items, comment items, ordering, `onlyPosts`, and a private RSS link when an API key exists. Controls: drafts, whitelist, the two mediation lists (#108 makes draft order non-deterministic, so assert as a set). Settings: invite arithmetic, API key, styles, feeds. | mid | 85% |
| D3 | `pkg/web/api.go` | `ApiGetPosts` pagination: limit clamping, cursor, `updated_since`, empty results. `ApiNewPost`/`ApiEditPost` with publish and draft, the resulting DB state, and notifications queued. `ApiUploadImage` with `fakestorage`. `ApiDeletePost` ownership is fixed in PR #118, which adds `pkg/web/api_test.go`. Extend that file rather than duplicating it. | mid | 85% |
| D4a | `pkg/forms`: post, comment, prompt | `PostForm` Validate and Save for every `SaveAction` × new or existing post × with or without prompt × with or without URL. Check the resulting redirect or htmx headers (invoke the returned `FormSaveAction` on a `ginctx`) and the notifications queued (`fakesender`). `NewCommentForm`: permissions, reply threading (`TopCommentID`), post-stat upsert increments, and the author and participant notifications. `PostPromptForm`: direct-only recipients and the rate limit (#108). | mid | 85% |
| D4b | `pkg/forms`: all other forms | signup (including attribution sanitising), accept_invite (creates user, connection and a fresh invite, and logs in), waiting list, login `Validate` (#114 as a skipped test), change_password, settings_general, user_styles, whitelist_connection, send_invite (invite accounting), add_feed | cheap | 85% |
| D5 | `pkg/auth` (DB parts), `pkg/pgsession` (`store.go`, `user.go`) | `Signup`, `AcceptInvite`, `CheckCredentials`, `Login` (session set), `Auth` middleware (session → user in context), `AuthAPI` (403 for an unknown key, sets the user for a known one), `pgstore` round-trip | mid | 90% |
| D6 | `pkg/mail` (DB parts), `pkg/mail/sender/dbsender` | `SendInvite` (uses an unused invite, fails without one, rejects an existing email), `Validate`. dbsender: `Send` idempotency on `(email_type, unique_id)`; `sendEmails` retry schedule (attempts 1→3 with the listed intervals, then `failed`), using a fake real sender that fails N times. The concurrency test for #113 is skipped. | mid | 85% |
| D7 | `pkg/postops` (DB parts), `pkg/media/upload.go`, `pkg/feedops` (`feedops.go`, `feeds.go`), rest of `pkg/feedops/feeder` | `DeletePost` cascade. `StoreURL` upsert returns the existing ID. `CanPromptNow` (#108 skipped where order matters). `GetPostPrompt`. `SerializeBlog`/`InjectPostsInDB` **round-trip**: export a user's blog, import it into another user, and compare. `HandleUpload` for a user and for a feed (the exclusivity check). `GetRssFeeds` last-imported map. `GetRssFeedItems`. Subscribe is idempotent. Unsubscribe is scoped to the user. `refreshFeeds` error path (#116 skipped). | mid | 80% |

`pkg/media/server` (vips resizing and caching, 42% today) is **D8**, cheap,
target 70%. It needs libvips, which CI already installs.

**Done when:** every package meets its target in `make cover`, the full suite
runs in under 3 minutes locally, and every bug found is filed or reported.

---

## W3 — End-to-end HTTP tests

**6 parallel tasks**, all on the W0 harness. Each owns one file,
`e2e/<area>_test.go`. They assert status codes, redirects, htmx headers, key
HTML (via goquery) and resulting DB state (via factory readers). Together they
are the specification R1 must preserve: **every route in `cmd/web/main.go`,
`actions.go` and `api.go` gets at least one test**. The coordinator checks this
against a route list extracted from the source.

| Task | Routes | Tier |
|---|---|---|
| E1 | Public and visibility: `/`, `/articles/:id` (valid, unknown, bad name), `/users/:username` and `/rss/public/:username` for each profile visibility, `/users/:u/user_styles` (referer enforcement, `@scope` wrapping for non-Firefox UAs), `/posts/:id` (the D2a matrix, at HTTP level, one case per row), `/shared/:id`, `/explore` anon vs logged-in, `/user-media/robots.txt`, `/user-media/favicon.ico`, `/static/*`, CSP and nosniff headers present | **strong** |
| E2 | Auth: `/login` (logged-in redirect, `return_url` signature kept or dropped), `POST /form/login` (CSRF required, bad credentials, success redirect, signed return), `POST /controls/action/logout`, `/signup` and `POST /form/signup` with registration open and closed, `/confirm_signup/:id`, `/invite/:id` and `POST /form/accept_invite/:id` (full flow, reused invite → 404), `/confirm_waiting_list/:id`, `POST /form/signup_waiting_list` (always 404 today), `EnforceAuth` redirects on every `/controls` and `/write` route. #109 as skipped tests. | mid |
| E3 | Posts: `/write` (with a `?prompt=`), `POST /controls/form/edit_post` (new draft, autosave, publish, make_draft, delete, someone else's post → 404), `/posts/:id/edit` (not the author → 403), `/posts/:id/md`, `/posts/:id/zip` (#110 skipped), `POST /controls/form/new_comment` (reply, permissions, emails queued), `/controls/action/delete_draft` | mid |
| E4 | Connections: whitelist form, `remove_from_whitelist`, `create_connection`, `drop_connection`, `request_mediation`, `revoke_mediation_request`, `sign_mediation`, `dismiss_mediation`, `accept_connection`, `reject_connection`, and `/controls` rendering each state. A full three-user story: A and B connected, B and C connected, A requests C, B signs, C accepts, and A sees C's direct-only post. | mid |
| E5 | Settings and misc: `/controls/settings`, save_settings, save_user_styles, change_password (then log in with the new one), generate_api_key, send_invite (email queued), prompt_post and dismiss_prompt, create_share and delete_share (then `/shared/:id` works and stops working), add_user_feed (feed at a test `httptest.Server`), remove_rss_subscription, `dissmiss_rss_item` (sic), upload_media (a PNG fixture), `settings/export` and `settings/import` round-trip | cheap |
| E6 | API v1 and private RSS: missing, malformed and unknown bearer (400/403), `GET /api/v1/posts` pagination, `POST /api/v1/posts` (new, edit), `DELETE /api/v1/posts/:id` (own post → 200, foreign post → 404; fixed in #118), `PUT /api/v1/image`, `/rss/private/:key` (valid, and unknown → #115 skipped) | cheap |

**Done when:** every route is covered, `make cover` shows the `cmd/web`
statement coverage coming from the binary, and the E2E suite runs in under two
minutes.

---

## W4 — Local stack, dev tooling container, seed

**Goal:** on a clean machine with only Docker installed, you can start pcom,
apply migrations, regenerate models, read captured mail and get a populated
database. You don't install Postgres, `sql-migrate`, `sqlboiler`, `psql`,
libvips or Node on the host. Two ways of working are both first-class:

- **Everything in compose:** `make dev` runs the app and the asset watcher in
  containers, with live reload.
- **App on the host:** the current `cd cmd/web && make watchexec` plus
  `yarn watch` keep working unchanged, against the same compose services.

**5 tasks.** S1 and S2 run in parallel right after W0, alongside W1–W3. S3
and S4 follow S2. S5 comes last.

W4 is the one test-phase wave that changes developer tooling. It may change
`docker-compose.yml`, `.env.example`, `tools/`, `dbconfig.yml`,
`sqlmigrate.sh`, `generate.sh`, `sqlboiler.toml`, `Makefile`, `.github/` and
`docs/`. It still doesn't touch Go code compiled into `cmd/web`. Pointing the
app at object storage and at tommy needs code changes, so those happen in R2.
Until then, the two services run idle and are ready.

### S1 `cmd/seed` (mid)

A new binary, `go run ./cmd/seed [--reset]`, that reads `DATABASE_URL` like
the app does. It is built on `pkg/testutil/factory`, so the seed and the tests
describe the world the same way. It creates a small, named world that
exercises every feature:

- Users: `alice`, `bob`, `carol`, `dave` and `eve`, all with the password
  `password`, and a mix of profile visibilities. Alice–Bob and Bob–Carol are
  direct connections, which makes Alice–Carol second-degree. Dave is
  unrelated. Eve has an unaccepted invite.
- Posts: every visibility, a draft, a URL post, a comment thread three levels
  deep, a share link, and an open prompt.
- An API key for Alice, with a fixed, documented value for `blg` development.
- One RSS feed with items inserted directly. Its URL is the `example.test`
  placeholder, so the poller fails harmlessly instead of hitting the internet.
- A pending mediation request. Registration is open in the seeded database.
  Production keeps it closed; see #123.
- It refuses to run when `FLY_APP_NAME` is set.
- `--reset` truncates every table except `migrations` and `system_settings`.
  Without it, the seed exits if users already exist.

### S2 Compose stack and the dev tooling container (strong)

This task defines the contracts for S3–S5.

- **`docker-compose.yml`**. No service pins a `container_name`, so a second
  checkout, such as a worktree for a stacked wave, can run next to the first
  with `docker compose -p <name>`. Every host port is chosen to avoid the
  usual defaults. The services:

  | Service | Image | Host port | Notes |
  |---|---|---|---|
  | `postgres` | `postgres:16-alpine` (the same image as the test harness) | **5442** | Healthcheck and named volume. 5442 avoids a native Postgres on 5432, which would otherwise take the app's connections without any error. |
  | `objectstore` | `adobe/s3mock:5.2.3`, pinned | 9090 | Picked because it needs the least configuration of any S3-compatible server: `COM_ADOBE_TESTING_S3MOCK_STORE_INITIAL_BUCKETS=pcom-media` creates the bucket, so there is no init container. It accepts any credentials, supports path-style addressing and ACL headers, and keeps files in a named volume at `/s3mockroot` when `COM_ADOBE_TESTING_S3MOCK_STORE_RETAIN_FILES_ON_EXIT=true`. The same image serves as the E2E and S3-storage test container in R2. MinIO no longer publishes maintained community images, and SeaweedFS, Garage and RustFS all need bucket or layout bootstrapping. |
  | `mail` | `can3p/tommy:v0.1.0`, pinned | 8811 (UI + API), 8822 (fake vendor ingress) | Mail sink. It answers Mailjet's v3.1 send API, so production's Mailjet sender works against it unmodified. From R2 on, all development and E2E mail goes here, and the console sender is removed. Read mail at `http://localhost:8811/ui/`, or from tests via `GET /api/v1/events?plugin=mail`. |
  | `tools` | built from `tools/Dockerfile` | — | Profile `tools`. See below. |
  | `app`, `assets` | built from `tools/Dockerfile` (target `dev`) | 8080 | Profile `dev`. See S4. |

- **`tools/Dockerfile`**, the dev tooling image:
  - Based on `golang:1.26-alpine`, plus `postgresql16-client` (for `psql`
    and `pg_dump`), `bash` and `gettext` (for `envsubst`). A `dev` target adds
    `vips-dev`, `gcc`, `musl-dev`, `nodejs`, `yarn` and `watchexec` for S4.
  - Pin tool versions with Go `tool` directives in `go.mod`:
    `go get -tool github.com/rubenv/sql-migrate/sql-migrate@v1.8.1`,
    `github.com/volatiletech/sqlboiler/v4@v4.19.1`, and
    `.../drivers/sqlboiler-psql@v4.19.1`. Run them as `go tool …`. That
    keeps versions in one place, lets dependabot bump them, and keeps
    sqlboiler exactly on the runtime version in `go.mod`, which matters
    because the generated code must match the library.
  - sqlboiler finds its driver by path. Pass `$(go tool -n sqlboiler-psql)`,
    or `go build` the driver into the image's `PATH`.
  - The `tools` service mounts the repository at `/src`, uses named volumes
    for the Go module and build caches, reaches the database in-network as
    `postgres:5432`, and runs as the host UID and GID, so generated files
    aren't owned by root.
- **Scripts run inside the container and never parse URLs by hand:**
  - `dbconfig.yml` uses `datasource: ${DATABASE_URL}` (sql-migrate expands
    env in that field). That replaces the `awk` URL splitting in
    `sqlmigrate.sh`.
  - `generate.sh` is the only place that splits `DATABASE_URL` into
    `POSTGRES_*`, because sqlboiler's psql driver has no DSN option.
  - Both scripts refuse to run outside the tools container, unless
    `PCOM_ALLOW_HOST_TOOLS=1` is set for the production flow below.
- **Make targets** are thin wrappers, so nobody types `docker compose run`
  by hand:

  | Target | Does |
  |---|---|
  | `make dev-up` / `make dev-down` | Starts or stops postgres, objectstore and mail. |
  | `make migrate` | `./sqlmigrate.sh up` in the tools container against the compose DB. |
  | `make migrate-status`, `make migrate-down` | The matching `sql-migrate` commands. |
  | `make migration name=add_foo` | `sql-migrate new`. |
  | `make generate` | See S3. |
  | `make psql` | A `psql` shell on the compose DB. |
  | `make db-reset` | Drops and recreates the dev database, then migrates it. |
  | `make seed` | Runs `cmd/seed` in the tools container. |
  | `make tools-shell` | A bash shell in the tools container. |
  | `make dev` | See S4. |

  **Production:** `make migrate-prod` runs the tools container against the
  `fly proxy` tunnel (`host.docker.internal:5433`), with `DATABASE_URL` taken
  from `./env.pl`. It asks for confirmation first. In R2 this becomes a
  `migrate` subcommand and a fly `release_command`.
- **`.env.example`** (copied to `cmd/web/.env`, where the app and scripts
  read it today) is written for host mode: `DATABASE_URL` on
  `localhost:5442`, plus `SESSION_SALT`, `SITE_ROOT=http://localhost:8080`
  and `PORT=8080`. The object storage and Mailjet/tommy settings are
  commented out until R2. The compose `app` service sets the in-network
  equivalents (`postgres:5432` and so on) as real environment variables.
  `godotenv` never overrides an existing variable, so one file serves both
  modes.

### S3 Model generation from a clean migrated database (mid)

Depends on S2.

- `make generate` creates a throwaway `pcom_codegen` database in the compose
  Postgres. It applies every migration there, runs sqlboiler against it, and
  drops it. **The models then depend only on the migrations, never on
  whatever state the developer's dev database is in.**
- Acceptance: `make generate` on a clean checkout produces **zero diff** in
  `pkg/model/core`. That proves the pinned tool versions reproduce the
  committed models.
- A CI job, "Generated models are current": a Postgres service, the same
  tools image, `make generate`, then `git diff --exit-code -- pkg/model`. It
  catches a migration committed without regenerated models, which otherwise
  shows up at runtime as a missing column. R5 later swaps sqlboiler for bob
  inside the same job.
- CI also builds the tools image, so a broken image fails the PR that broke
  it.

### S4 App in compose, with host mode kept working (mid)

Depends on S2.

- A `dev` profile with two services built from the `dev` image target:
  - `app` runs `watchexec -w /src --exts go,html,md --restart -- go run .`
    in `/src/cmd/web`, the container equivalent of `make watchexec`, and
    publishes 8080.
  - `assets` runs `yarn install --frozen-lockfile && yarn watch` in
    `/src/cmd/web`.

  `node_modules` lives in a **named volume** mounted over
  `cmd/web/node_modules`. The host's copy has macOS-native binaries (sass),
  which must not leak into Linux or the other way round. `dist/` stays on
  the bind mount, so host mode sees the same assets.
- `make dev` runs `docker compose --profile dev up`. `make dev-logs` follows
  the logs.
- Host mode is unchanged: `make dev-up`, then `cd cmd/web && make watchexec`
  and `yarn watch`, against the compose services on their host ports.
  libvips and Node are only needed on the host in this mode.
- Check on macOS that file watching through the bind mount triggers reloads.
  If it's unreliable, fall back to `watchexec --poll`, and document the
  choice.

### S5 Cold-start docs and the seed smoke test (cheap)

Depends on S1–S4.

- Write `docs/running.md`, the cold-start guide, covering both modes:
  1. `cp .env.example cmd/web/.env`.
  2. `make dev-up && make migrate && make seed`.
  3. Either `make dev`, or the host-mode commands.
  4. Where to read mail (tommy UI) and where uploads go (object storage,
     from R2 on).

  Trim the dev-setup part of `README.md` to point at the guide; the
  local-Postgres and `go install` steps go away. **Verify it by following
  it on a fresh clone** in a temp directory, once per mode, with no host
  tools on `PATH` for the compose mode. Don't write it from memory.
- Add an E2E test that runs the seed against the harness database, then logs
  in as each seeded user and `GET`s every navigable page (feed, explore,
  controls, settings, write, every seeded post, every profile). Each must
  return 2xx or 3xx and render without a template error. This is a cheap,
  broad net for template or data-shape regressions during R1–R5.

**Done when:** on a fresh clone with only Docker,
`cp .env.example cmd/web/.env && make dev-up && make migrate && make seed && make dev`
gives a working app at `http://localhost:8080` where you can log in as
`alice` and see every feature populated. The host-mode flow works too.
`make generate` produces no diff, and the CI generation job is green.

---

## W5 — Coverage ratchet

**1 task, cheap.** CI fails if total coverage (excluding `pkg/model/core`)
drops below the achieved level minus 1%. Critical packages get per-package
floors: `pkg/web`, `pkg/userops`, `pkg/auth`, `pkg/forms` and `pkg/postops`.
Record the numbers in the history file. Expected total after W1–W4: 70–80%.

---

## WB — Bug-fix wave

After the safety net is in place, and **before** any refactor, so that R1–R5
stay pure refactors. There is one task per issue, each mid or cheap, and they
are mostly parallel. #109 and #111 both touch `cmd/web/main.go`, so give them
to one task. #114 and #119 both change the password hash, so they go together
as well. Each fix removes the matching `t.Skip`, and that test is the proof.

| Issue | Summary | Notes |
|---|---|---|
| #108 | `ORDER BY ?` placeholder sorts nothing | 4 call sites |
| #109 | Handlers keep running after redirect | `cmd/web/main.go`, `actions.go` |
| #110 | Post zip export: anon panic, empty for non-authors, double close | decide: author-only? (Q7) |
| #111 | `-html` flag unusable; cleanup deferred before err check | superseded by R2 if R2 comes first |
| #112 | `log.Fatal` in the request path | return errors; touches every mail func |
| #113 | `FOR UPDATE` outside the transaction | dbsender double-send |
| #114 + #119 | Email case-sensitivity + argon2id with rehash-on-login | one task (strong): lowercase emails in a migration, verify legacy hashes against both the original and the lowercased email, then rehash to argon2id on login |
| #115 | RSS endpoints return the wrong status codes | |
| #116 | Feed poller swallows errors | |
| #117 | Requests can be decided twice | |
| #120 | Unescaped user content in HTML emails | quick `html.EscapeString` fix here; R4 makes it structural |
| #121 | Private RSS URL carries the read/write API key | separate read-only feed token; needs a migration |
| #122 | Session not rotated on login | small; with the auth work |

**Future, not part of WB:** #123 (re-enable signups with bot protection;
signups are off on purpose) and #124 (feed and explore pagination). Both come
after R1, because they touch routes R1 moves. #124 needs W3's feed E2E tests
in place first.

### Known bugs (for de-duplication)

Filed: #108–#117 and #119–#124. Already fixed: the foreign-post delete
through `DELETE /api/v1/posts/:id`, in PR #118 (merged). W2.D3 and W3.E6
test the ownership check as a normal, non-skipped test.

---

## R1 — Router decomposition (planned)

`cmd/web/main.go` is 988 lines. It contains configuration, wiring, 36 inline
route handlers and the template funcmap. `actions.go` adds 19 JSON actions, most of which
repeat the same BindJSON and reportError boilerplate.

**Target layout** (the binary's `main` package is the composition root only,
and `pkg/web/*` holds the transports):

```
cmd/web/main.go              → composition root only: config, deps, start (≈80 lines)
pkg/web/app/deps.go          → type Deps struct{ DB *sqlx.DB; Sender; MediaStorage; MediaServer; Config }
pkg/web/app/router.go        → func New(d *Deps) *gin.Engine — middleware + group mounting only
pkg/web/app/funcmap.go       → funcmap, staticAsset/manifest loader
pkg/web/app/routes_public.go → /, /articles, /explore, /users/*, /shared, /posts/* (read)
pkg/web/app/routes_auth.go   → login, signup, invite, confirm_*, logout, /form/* (non-controls)
pkg/web/app/routes_rss.go    → /rss/public, /rss/private
pkg/web/app/routes_media.go  → /user-media, /static
pkg/web/app/routes_controls.go → /controls, /controls/settings, /write, /controls/form/*
pkg/web/app/actions_*.go     → connections, mediation, posts, shares, prompts, rss, settings (import/export), media
pkg/web/app/api.go           → /api/v1
pkg/web/app/jsonaction.go    → func jsonAction[T any](d *Deps, fn func(c *gin.Context, u *core.User, in T) error) gin.HandlerFunc
```

- **Step 1** (strong, one task): `Deps`, `New()`, `jsonAction`, funcmap. Move
  the middleware and mount the groups, with the handlers still in one file.
  Once `main()` calls `app.New(deps).Run()`, the E2E suite must pass
  unchanged. From then on, add in-process `httptest` tests against `app.New`,
  which are faster than the binary.
- **Step 2** (parallel, cheap to mid): one task per `routes_*.go` or
  `actions_*.go` file, moving handlers verbatim. Each task owns its
  destination file. The coordinator removes the moved code from the source
  file, because tasks never edit the same file.
- **Step 3** (mid): convert the 19 actions to `jsonAction`. Delete
  `pkg/web/user_connections.go` (a dead duplicate of
  `userops.CreateConnection`) and `client/html/from--signup-waitlist.html`
  (an unused, misspelled copy).
- **Invariant:** W3's E2E tests and the S3 crawl are not edited during R1. If
  one has to change, the refactor changed behavior.

## R2 — go-flags configuration, single binary, compose-backed services (planned)

**Configuration rules:**

- Every setting is a `jessevdk/go-flags` option with paired `long:` and
  `env:` tags, declared once, so `--help` is the configuration reference.
- Required settings fail at startup and name the environment variable, never
  on the first request. An environment variable that exists but is empty
  counts as missing, which go-flags alone doesn't check.
- Credentials use a `Secret` type that doesn't print.
- There are no config files in a deployed environment, and no `if production`
  switches.

These helpers belong in gogo (see `docs/gogo-extraction.md`).

**Subcommands:** `serve`, `migrate`, `seed`, `admin invite --email --num`
(replaces `cmd/scripts/add_invite.go`), and
`admin registration --open/--close` (replaces
`cmd/scripts/toggle_open_registration`).

**Remove every scattered `os.Getenv`.** Today these are:

- `SESSION_SALT`, read in `auth.HashValue` on every call;
- `SENDER_ADDRESS`, at 13 call sites;
- `ADMIN_ADDRESS`, read into a package variable at init;
- `STATIC_CDN`, baked into the CSP string at init;
- `USER_MEDIA_*` and `SITE_ROOT`;
- `ENABLE_PPROF`;
- `FLY_APP_NAME`, which `util.InCluster()` uses as an `if production`
  switch.

Replace the switch with explicit settings, such as `--secure-cookies`,
`--hsts` and `--static-cdn`. This fixes #111 by construction.

**Mail goes through tommy; the console sender is dropped.**

- Prerequisite (gogo): `sender/mailjet/config.Config` gains a `BaseURL`
  option (`long:"api-base" env:"API_BASE"`). It is passed to
  `mailjet.NewMailjetClient(public, private, baseURL)`, which the SDK already
  supports.
- pcom always uses the Mailjet sender (still behind `dbsender`). Locally, the
  base URL points at tommy (`http://localhost:8822`, or `http://mail:8822`
  in compose), with any credentials. The console sender and
  `--force-real-sender` are deleted.
- The email poll interval becomes a setting, so E2E tests don't wait 10
  seconds.
- The E2E harness starts a tommy test container and adds
  `app.Mails(t, filter)`, which polls `GET /api/v1/events?plugin=mail` until
  the expected mail arrives. Tests that asserted on the `outgoing_emails`
  queue gain an assertion on delivered mail.
- Unit and package tests keep using `fakesender`.

**Object storage everywhere; local file storage is dropped.**

- The S3 storage gets a path-style addressing option (`UsePathStyle`). The
  compose object store needs it, and today's client only does virtual-host
  addressing.
- Local development points at the compose `objectstore` (s3mock, bucket
  `pcom-media`).
- Delete `pkg/media/server/storage/local`, the `user_media` directory, and
  the `cmd/web/.gitignore` entries for it.
- Add S3 storage tests against an s3mock test container: upload with
  `ACL: private`, download, `ObjectExists` true and false, and not-found
  mapping.
- The E2E harness starts that container and passes its settings to the
  binary.
- Unit tests keep using `fakestorage`.

**Migrations at deploy:** add a `migrate` subcommand and run it as the fly
`release_command`. `make migrate-prod` from W4 becomes a fallback, not the
deploy path.

## R3 — gogo convergence (planned)

See `docs/gogo-extraction.md`. The order:

1. The upgraded `testcontainers/postgres`. Done in gogo#5; W0 consumes it.
2. `sender/mailjet` `BaseURL`, needed by R2.
3. Switch pcom to the copies that already exist in gogo: `util/ginhelpers`
   (render, csrf) and `util.InCluster`, until R2 removes the latter.
4. go-flags config helpers, needed by R2.
5. The new extraction candidates, by payoff.

## R4 — Mailers (planned)

Every email becomes a declared definition:

- a typed input;
- a sample factory that touches no database, including the awkward variants
  (no subject, long subject, markup in user content, a linked URL);
- a template rendered through `html/template`;
- a golden file for the text and the HTML;
- a development preview page that lists every email rendered from its
  samples.

Call sites stop building mail bodies with `fmt.Sprintf`. W1.U10's goldens
define "unchanged output".

This fixes #120 by construction, and removes the per-function `log.Fatal` if
WB hasn't already. Consider replacing `dbsender`'s ad-hoc poller with a
general job queue, where the email is enqueued in the same transaction as the
state change. tommy is where a human checks what was actually delivered.

## R5 — bob ORM (planned)

Plan it in detail after W2/W3 land. Known constraints:

- gogo's `sender.Sender` and `forms.Form` interfaces take a sqlboiler
  `boil.ContextExecutor`. gogo needs an ORM-agnostic executor first, or it
  stays coupled to sqlboiler.
- Run side by side: generate bob models into `pkg/model/bob` and migrate
  package by package (userops, postops, feedops, web, forms, auth). Delete
  sqlboiler at the end. Because of ground rule 3, most test churn is inside
  `pkg/testutil/factory`, which switches to bob's generated factories.
- `make generate` and the CI freshness job from W4.S3 switch generator
  inside the same tools container.
- Consider `lib/pq` → `pgx` at the same time.
  - bob's psql error helpers must be generated for the driver actually used
    to connect.
  - Otherwise unique-violation detection silently never matches.

## R6 — Dependency hygiene (planned, any time after W5)

- Drop `alecthomas/assert` for testify.
- Drop both `pkg/errors` and `friendsofgo/errors` for stdlib `errors` and
  `fmt.Errorf("%w")`.
- `ory/dockertest` goes away with W0.T0.1, once pcom uses gogo's
  testcontainers.
- Check the unmaintained `antonlindstrom/pgstore` and `volatiletech/null`
  (the latter goes with bob).
- Remove `plan.txt` (its content can go to GitHub issues or `docs/`) and
  `.devin/`.
