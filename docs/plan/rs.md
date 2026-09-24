## RS — Repositories and services, thin handlers (planned)

After R1, every handler lives in `pkg/web/app`, but it still does its own
database work: about 50 inline `core.*` queries in the handlers, 150 in
`pkg/web/func.go`, more in `pkg/forms`, `pkg/auth`, `pkg/userops`,
`pkg/postops` and `pkg/feedops`, with authorization checks scattered between
them. RS puts every query behind a repository and every business rule into a
service, so a handler only translates HTTP to a service call and back.

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

- **Step 2** (mid): delete what is left empty (`pkg/userops`, the DB parts of
  `pkg/postops` and `pkg/feedops`, `pkg/web/func.go`), confirm the allowlist is
  empty, and update `docs/architecture.md` with anything the tasks learned.

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
