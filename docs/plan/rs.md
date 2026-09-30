## RS — Repositories and services, thin handlers (planned)

After R1, every handler lives in `pkg/web/app`, but it still does its own
database work: about 50 inline `core.*` queries in the handlers, 150 in
`pkg/web/func.go`, more in `pkg/forms`, `pkg/auth`, `pkg/userops`,
`pkg/postops` and `pkg/feedops`, with authorization checks scattered between
them. RS puts every query behind a repository and every business rule into a
service, so a handler only translates HTTP to a service call and back.

**What R1 left** (2026-09-30): the routes are in `pkg/web/app`, one mount
function per `routes_*.go` and `actions_*.go` file; `routes.go` only calls
them. 15 JSON actions go through `jsonAction(d, fn)`, whose `fn` returns an
error whose text the user sees verbatim (`userError` for sentences). Five
don't fit it and still reply by hand: `generate_api_key` and
`regenerate_feed_token` (no body), `upload_media` (its own JSON),
`settings/export` (file stream), `settings/import` (form upload).
`TestGuards_RouteTableMatchesSource` in `e2e/guards_test.go` finds routes by
scanning those files by name, with each file's router-variable names: a task
that adds, renames or deletes a route file updates its `prefixes` map (only
the map; the coordinator does it, as in R1).

This is also the seam that makes R5 cheap: once all SQL lives in `pkg/repo`,
the bob migration rewrites one package, not the whole application.

### The layers

| Layer | Package | Does | Never does |
|---|---|---|---|
| Transport | `pkg/web/app` (HTML, actions, API), R2's CLI subcommands | Bind and shape-check input, take the current user from the context, call **one** service method (a page may assemble several read calls), render, redirect or set htmx headers, map service errors to statuses | SQL or ORM calls, transactions, authorization beyond "is logged in", importing `pkg/repo` |
| Service | `pkg/service/<area>` | Business rules, authorization (who may do what, visibility), transactions, enqueueing notifications in the same transaction as the state change. Methods take `ctx`, the acting user (nil for anonymous) and a typed input, and return a typed result or a `service` error. | Importing gin or `net/http`; building queries |
| Repository | `pkg/repo` | Every query and write, one file per aggregate (`users.go`, `connections.go`, `posts.go`, `comments.go`, `shares.go`, `prompts.go`, `invites.go`, `feeds.go`, `media.go`, `mail_queue.go`, `settings.go`). Methods are named for what they fetch (`PostsByAuthors(ctx, ids, visibilities, page)`); `sql.ErrNoRows` becomes `repo.ErrNotFound`. | Business rules or permission checks: the service computes the filter (the viewer's radius, the visible visibilities) and passes it in |

Pure packages stay where they are: `pkg/markdown`, `pkg/links`,
`pkg/forms/validation`, `pkg/util`, mail formatting in `pkg/mail`, the media
server. Infrastructure that owns its own tables stays as well: `pkg/pgsession`
(session store) and the `dbsender` poller, which reads the queue through
`repo`.

Decisions made up front, so the parallel tasks agree:

- **Transactions:** `repo.Store` wraps the executor. A service runs
  `s.store.Tx(ctx, func(tx *repo.Store) error { … })` (on top of gogo's
  `util/transact`); repositories never open transactions themselves. Mail is
  enqueued through the transaction's executor, so it is sent only if the
  change commits.
- **Concrete types, no interfaces for mocking.** Services take `*repo.Store`
  and are tested against `testdb.New(t)`, which is cheap with the template
  database. Interfaces appear only where a second implementation exists
  (sender, media storage).
- **Errors:** `service.ErrNotFound`, `ErrForbidden`, `ErrNeedsLogin`,
  `ErrConflict` and a `ValidationError{Field, Message}`. One function in
  `pkg/web/app` maps them to HTML, htmx and API responses. It replaces the ad
  hoc `ErrNeedsLogin`/`ErrNotFound` in `pkg/web` and the per-handler
  `reportError` calls. Status codes must stay what W3 pins.
- **Forms:** `gogo/forms` passes a `boil.ContextExecutor` to `Save`. Until
  gogo has an ORM-agnostic executor (R3/R5), a form's `Save` ignores it and
  calls a service; its `Validate` keeps the field checks. Form logic that
  touches the database moves into the service.
- **Model types cross the boundary for now.** Repositories and services
  return the generated `core` structs (or small view structs built from them)
  so templates don't change in RS. Whether to introduce pcom-owned domain
  types is Q13; R5 is the natural moment, because bob changes the generated
  types anyway.
- **Enforced by a test,** `pkg/arch/arch_test.go`. It runs `go list -deps`
  and fails when a package outside `pkg/repo`, `pkg/testutil/...`,
  `pkg/pgsession`, `cmd/seed` and the composition root imports
  `sqlboiler/v4/queries`, `queries/qm`, `sqlx` or `database/sql`, or calls
  the `core` query builders. `pkg/web/app` must not import `pkg/repo`.
  `boil` is allowed in `pkg/forms` only, for the `Save` signature. The test
  starts with an allowlist of every current offender, and each task deletes
  its entries. RS is done when the allowlist is empty.

### Steps

- **Step 0** (strong, coordinator): the contracts. `repo.Store` and `Tx`,
  `repo.ErrNotFound`, the `service` errors and their HTTP mapping, the
  architecture test with its allowlist, and `docs/architecture.md` (the table
  above and the rules, one page; add a row for it to the `AGENTS.md` area
  table). Convert one vertical slice end to end as the worked example that
  every later task copies: **shares** (`create_share`, `delete_share`,
  `/shared/:id`). It is small and has an authorization rule.
- **Step 1** (parallel). One task per area. Each task owns its repository
  file(s), its `pkg/service/<area>/` package, and the handler files R1 created
  for that area. The handler files line up with R1's `routes_*.go` and
  `actions_*.go`, so the tasks don't collide.

| Task | Area | Moves from | Tier |
|---|---|---|---|
| L1 | Visibility and reading: single post, user home, explore, feed, shared post, comments, public and private RSS | `pkg/web/func.go` read paths, `pkg/userops/profile.go`, `postops.CanSeePost`, the RSS handlers | **strong** (the privacy matrix) |
| L2 | Connections: whitelist, mediation, connection requests | `pkg/userops/connections.go`, `actions_connections.go`, whitelist form | mid |
| L3 | Posts: write, edit, autosave, publish, delete, drafts, comments, prompts, export and import, API v1 | `pkg/postops`, post/comment/prompt forms, `pkg/web/api.go`, `func.go` write paths | mid |
| L4 | Accounts: signup, waiting list, invites, login credentials, password, settings, user styles, API keys, registration toggle | DB parts of `pkg/auth`, the account forms, the inline queries in the auth routes, `pkg/mail/invite.go` | mid |
| L5 | Feeds: subscriptions, items, dismiss, the poller's reads and writes | `pkg/feedops`, `pkg/feedops/feeder` | mid |
| L6 | Media and mail queue: upload records, `dbsender` queue access | `pkg/media/upload.go`, `pkg/mail/sender/dbsender` | cheap |

**Step 0 outcome** (2026-09-30). Everything above step 1 exists; see
`docs/architecture.md`. Added beyond the list, so step 1's tasks don't
collide:

- `pkg/service/graph` + `pkg/repo/graph.go`: the read-only connection graph
  (radius, direct and second-degree IDs) that L1, L2 and L3 all need.
  `userops`' graph functions are one-line wrappers over it, so their callers
  and W2 tests are unchanged.
- `repo.Using(exec)` (a store over an existing executor: legacy code,
  factories, tests) and `Store.SendMail` (gogo's sender takes an executor).
- Mixed handler files split by area (verbatim moves, checked by line
  multiset): `routes_controls.go` became `routes_connections.go`,
  `routes_posts.go`, `routes_settings.go`, `routes_feeds.go`;
  `/feed` moved into `routes_public.go`; export and import moved from
  `actions_settings.go` to `actions_export.go`; `pkg/web/func.go` became
  `pages.go` plus `pages_<area>.go`. The guards test's `prefixes` map
  follows.
- The arch test type-checks every package, so it also flags passing a
  database handle into a legacy helper. Its allowlist tags each file with
  the task that owns it: **a task's files are exactly its allowlist
  entries** plus the new ones below.

**Ownership in step 1.** Each task owns:

- its allowlist entries in `pkg/arch/arch_test.go` (it deletes them, and
  touches no others);
- `pkg/service/<area>/` (areas: L1 `reading`, L2 `connections`, L3 `posts`,
  L4 `accounts`, L5 `feeds`, L6 `media`), plus one field and one line in
  `pkg/service/registry` (the only shared file; add, don't reorder);
- in `pkg/repo`, the files for its aggregates: L1 none of its own, L2
  `connections.go`, `whitelist.go`, `mediation.go`; L3 `posts.go`,
  `comments.go`, `prompts.go`, `shares.go`; L4 `users.go`, `invites.go`,
  `settings.go`, `api_keys.go`, `feed_token.go`; L5 `feeds.go`; L6
  `media.go`, `mail_queue.go`. A query on another area's aggregate goes into
  `<aggregate>_<area>.go` (L1's post reads: `posts_reading.go`). Step 2
  merges these and removes duplicates. Nobody edits `graph.go` or
  `store.go`; if a task needs one changed, it asks.
- the tests of the code it moves. `pkg/web/privacy_test.go` is L1's;
  `pkg/web/func_test.go` is shared, and each task edits only the tests of
  its own page builders.

**Factories and repositories.** Inspected on 2026-09-30:

- `factory/read.go` has 25 lookup helpers (`GetPost`, `ListPosts`,
  `ConnectionExists`, `ShareExists`, ...), about 145 call sites, half of
  them in `e2e/`. They duplicate repository reads. In step 1, when a task's
  repository gains an equivalent method, the factory helper's body becomes
  `return repo.Using(exec).X(ctx, ...)`, with its signature unchanged
  (`e2e/` is frozen). After RS, call sites switch to the repository and
  `read.go` keeps only what tests alone need (`ListOutgoingEmails` with
  arbitrary filters).
- The creators stay. They insert, in one step, states that no business path
  creates directly (backdated timestamps, dismissed prompts, mediator
  decisions, a share on a draft). Routing them through services would make
  fixtures depend on the rules under test. They should delegate only where
  they copy an invariant: `factory.Connect` inserts both directed rows by
  calling `userops.CreateConnection`, so L2 points it at the repository
  method that replaces it.
- Found in passing, not resolved: `factory.WithPassword` hashes with
  `pgsession.HashUserPwd(email, pw)`, while signup, invite acceptance and
  password change use `pgsession.HashPassword`. So factory users log in only
  through the legacy-hash branch. L4 checks which is intended (#119).

- **Step 2** (mid): delete what is left empty (`pkg/userops`, the DB parts of
  `pkg/postops` and `pkg/feedops`, `userops`' graph wrappers), merge the
  `<aggregate>_<area>.go` repository files into one file per aggregate and
  drop duplicate queries, move the page builders from `pkg/web` next to
  their handlers, confirm the allowlist is empty, and update
  `docs/architecture.md` with anything the tasks learned.

**Tests.** W2's package tests move with the code they test. Only the call
site changes (`userops.CreateConnection(ctx, db, …)` becomes
`connections.Create(ctx, actor, …)`); **an assertion that changes means the
behavior changed**, and that is a bug in the task. The D2a privacy matrix is
re-pointed at L1's service. New service methods get service tests against
`testdb`; repositories are tested through their services, apart from queries
with their own logic (pagination cursors, the second-degree graph query).

**Invariant:** W3's E2E tests, W6's browser tests and the W4.S5 seed crawl are
not edited during RS. Coverage stays at or above the W5 floors.

**Done when:** the allowlist in `pkg/arch/arch_test.go` is empty, no handler
in `pkg/web/app` is longer than about 30 lines, `make check` and the browser
suite are green, and `docs/architecture.md` describes what exists.
