# Architecture: layers

pcom has three layers. `pkg/arch/arch_test.go` enforces them; its allowlist
is empty and stays so. Shares
(`pkg/service/shares`, `pkg/repo/shares.go`, `pkg/web/app/actions_shares.go`,
`/shared/:id` in `routes_public.go`) is the worked example: copy it.

| Layer | Package | Does | Never does |
|---|---|---|---|
| Transport | `pkg/web/app` (HTML, actions, API), page builders in `pkg/web`, CLI commands | Bind and shape-check input, take the current user from the context, call **one** service method (a page may assemble several reads), render, redirect or set htmx headers, map service errors to responses | SQL or ORM calls, transactions, authorization beyond "is logged in", importing `pkg/repo` |
| Service | `pkg/service/<area>` | Business rules, authorization, visibility, transactions, sending mail in the same transaction as the change. Methods take `ctx`, the acting user (`*core.User`, nil for anonymous) and the input, and return a result or a service error | Importing gin or `net/http`; building queries |
| Repository | `pkg/repo` | Every query and write, as methods on `*repo.Store`, named for what they fetch (`PostByID`, `ShareByID`). `sql.ErrNoRows` becomes `repo.ErrNotFound` | Business rules or permission checks, in Go or in SQL: the service computes the filter and passes it in, and a query's expressions select, count and sum facts, they don't define rules (see "Logic in the database") |

Pure packages stay where they are (`pkg/markdown`, `pkg/links`,
`pkg/forms/validation`'s pure checks, `pkg/util`, mail formatting in
`pkg/mail`). `pkg/pgsession` owns its table and is exempt.

## Rules

- **Store and transactions.** `repo.New(db)` in the composition root;
  services hold the `*repo.Store`. A service opens a transaction with
  `s.store.Tx(ctx, func(tx *repo.Store) error { ... })` and passes `tx` on.
  `Tx` inside a transaction joins it. Repositories never call `Tx`.
- **Mail** is sent with `tx.SendMail(ctx, sender, ...)` inside the
  transaction, so it is queued only if the change commits. Services get the
  queue, `repo.MailQueue`, in their constructor; it delivers through gogo's
  `sender.Sender`. The queue drops a repeated (type, unique id), so a
  feature that notifies again about the same object needs its own key, and
  its test sends two events in a row.
- **Errors** (`pkg/service`, which re-exports gogo's `apperr`; code uses the
  `service` names): `ErrNotFound` (also for "exists, but you may not see
  it"), `ErrForbidden`, `ErrNeedsLogin`, `ErrConflict`, and
  `service.Invalid(field, message)` for input the user must fix; its message
  is shown verbatim, so write it as a sentence. Unexpected errors pass
  through unchanged. Responses, through gogo's `ginhelpers`, which
  `router.go` configures (login redirect; error text outside production):
  - pages: `ginhelpers.HTMLError(c, err)` (redirects to login on
    `ErrNeedsLogin`, otherwise `ginhelpers.Status(err)`: 404, 403, 409, 400);
  - API: `ginhelpers.API`, same statuses;
  - JSON actions: `jsonAction` answers 400 for every error with
    `actionMessage(err)` as the text, as the actions always did.
- **Connection graph.** `pkg/service/graph` (`RadiusBetween`,
  `DirectUserIDs`, `DirectAndSecondDegree`) is the one place that answers
  how two users are connected. Services call it with their store or `tx`.
- **Wiring.** `pkg/service/registry` builds every service; `app.New` fills
  `Deps.Services` from `Deps.DB` when it is nil. A new service adds one field
  to `registry.Services` and one line to `registry.New`. Handlers reach it
  as `d.Services.<Area>`.
- **Forms.** gogo's forms take no executor. `Save` calls a service (at most
  one that writes, as `DefaultHandler` opens no transaction); `Validate`
  keeps the field checks. Form constructors take the service they need.
  A form's field names, messages and checks stay in the form and its
  template: a service exposes the numbers a check needs (a `TextLimits()`
  accessor, exported minimums) and enforces them in its write methods, but
  has no `ValidateX` for forms to call.
- **Repositories return pcom's own types.** The structs in `pkg/model` are
  the rows, written by hand, and they are also the bun models: their `bun`
  tags mean nothing outside `pkg/repo`. Services, handlers and templates use
  them directly; only `pkg/repo`, `pkg/model` and the test factories import
  bun (`pkg/arch` enforces it), so there is no mapping layer.
- **Page builders** in `pkg/web` (`pages_<area>.go`) take service results,
  not a database: `web.SharedPost(c, userData, shared)`.
- **Executors.** Code that still takes an executor gets `store.Exec()` only in tests
  and factories (`repo.Using(exec)` on their side).
- **Panics stay panics.** Where the old code panicked (a confirmation mail
  that cannot be queued, the waiting list write), the accounts service returns
  `accounts.FatalError` and the transport panics, so the recovery middleware
  still answers 500 and mails the admin.
- **Prompts.** `SendPrompt` checks in its transaction that the recipient is a
  direct connection (`graph.RadiusBetween`) and answers with the form's
  wording; the form's own check only shapes the choice list.
- **Uploads.** `media.StoreUpload` (`pkg/service/media`) stores an image owned
  by exactly one of a user or an RSS feed on the caller's `tx`; posts (import)
  and feeds (feed images) call it, so no service writes `media_uploads` itself.
- **Exemptions.** `pkg/pgsession` and the `dbsender` mail queue own their
  tables and are exempt from the layering (see `pkg/arch`).
- **One query, one method.** Repository files are per aggregate; a lookup
  another area needs is called, not copied (`UsersByIDs`, `OpenGrantExists`).
- **Know a value's readers before changing it.** Before a migration rewrites
  a column, or a change alters what a column, hash, key or function returns,
  find everything derived from it (a hash salted with it, a signature, a
  cache key) and change or name each reader. Lowercasing stored emails, for
  example, would have broken the password hashes salted with them.

## Configuration and tooling

- **One setting per behavior.** Production behavior (secure cookies, HSTS, static caching, panic reports,
  error pages) is each its own setting with a production-safe default; development, compose and tests
  turn them off. There is no "production mode" switch: `FLY_APP_NAME` only makes `seed` refuse to run.
  Limits and tunables are settings too ("No magic numbers" in `AGENTS.md`).
- **Migrations run at deploy, outside the app.** There is no `migrate` subcommand and no migration library
  in `go.mod`: the production image carries the `sql-migrate` binary, `dbconfig.yml` and `migrations/`, and
  fly's `release_command` runs it before the new version takes traffic.
- **Migration tools stay out of `go.mod`.** sql-migrate is installed in `tools/Dockerfile` and the production
  `Dockerfile`, pinned by an `ARG` that CI's lint job keeps equal in both, because `go tool` directives pull
  a tool's whole dependency tree into the module graph.
- **One driver, no code generation.** Everything connects through pgx's `database/sql` driver: the server,
  the CLI commands and the test databases (`testdb` reopens gogo's lib/pq connection with pgx). The models
  are not generated; `TestModels_MatchTheSchema` (`pkg/model`) checks them against the migrated database,
  so a migration that adds or changes a column fails it until the struct follows.

## Logic in the database

Business rules live in Go services, where they are read, tested and changed
together. Postgres stores facts and protects their integrity. A rule moves
into the database only for a strong reason, which the migration or
repository comment names and review checks.

The database does:

- **Integrity backstops.** NOT NULL, foreign keys, unique and partial unique
  indexes, CHECKs on a column's own shape. The service checks first and
  returns a form error; the constraint catches races and code paths that
  forgot (the pending-invitation index, #168; `users.email_canonical NOT
  NULL`, so an insert that skips the canonical address fails).
- **Concurrency control.** Transactions, row locks (`LockLoginAttempt`),
  scoped advisory locks (`repo.LockUser`, `repo.LockMailbox`), and
  conditional writes that re-check the state they act on
  (`DeleteUnconfirmedUser … where email_confirmed_at is null`).
- **Set work it does far better.** Filtering, counting, summing, ordering,
  paging and graph recursion over stored facts, with every parameter (times,
  limits, ids) computed by the service and passed in.
- **Cascades only for rows a parent wholly owns** (`login_attempts` on
  `users`).
- **One-off data migrations.** A backfill may restate a Go rule in SQL,
  commented as a frozen copy that matches the Go rule when written; the live
  rule stays in Go (`users_email_canonical.sql` and
  `pgsession.CanonicalEmail`).

The database does not:

- **Compute business values** in functions, triggers, generated columns,
  views or defaults. The mailbox rule, for example, is
  `pgsession.CanonicalEmail`, with a table test, and its result is stored on
  every insert.
- **Decide time.** No `now()` in a rule's predicate: the service passes its
  clock's time in, so tests move it (`accounts.WithClock`). `created_at`
  defaults are fine.
- **Define a rule in an expression.** When an aggregate's arithmetic would be
  the rule, store the fact the rule needs and let SQL only add it up: the
  service counts a wrong code in `login_attempts.wrong_tries`, and
  `WrongLoginTriesSince` sums that column.
- **Decide through an error.** A behavior that depends on a constraint
  failing is stated in the query or the service instead, and a constraint
  error stays an error: `UnconfirmedUserIDsCreatedBefore` leaves out the
  accounts holding invitations rather than relying on the foreign key to
  refuse their delete.
- **Hold a second copy of a rule.** A query narrows by stored facts and the
  service applies its predicate to the rows: `UnexpiredLoginAttempts`
  returns the candidates and `LatestLoginAttempt` picks the first that
  `open()` accepts.

## Writing bun queries

Repositories query through [bun](https://bun.uptrace.dev). The models are the structs in `pkg/model`; their
conventions are in its package comment. Code that takes an executor takes a `repo.Executor` (the database or
a transaction). Each rule below is pinned by a test in `pkg/repo/bun_test.go`.

- **Every query runs on the Store's executor.** Start it with `s.query()` in a Store method, or
  `repo.Query(exec)` in a test factory. Both point bun at the executor with `.Conn`, so a query inside a
  transaction stays in it, and so do the relations bun loads with a query of their own (has-many).
- **Locking with a joined relation names the table.** bun joins a belongs-to or has-one relation with a
  `LEFT JOIN`, and Postgres refuses to lock the nullable side of an outer join: write
  `For("UPDATE OF ?TableAlias")` (and `... SKIP LOCKED`). A has-many relation is a separate query; lock its
  rows with `For` inside the relation's apply function.
- **`bun.List` needs no empty-slice guard.** `Where("user_id IN (?)", bun.List(ids))` with no ids is valid SQL
  and matches nothing.
- **A defaulted column has a `default:` tag, never `nullzero`.** With `default:`, a zero value inserts
  `DEFAULT` and the value comes back through `RETURNING`. `nullzero` would also write NULL for a zero
  value on update (a `false` bool).
- **Timestamps are stamped by the model.** Each model's `BeforeAppendModel` sets `created_at` and
  `updated_at` on insert when they are zero and `updated_at` on every update. An update with `.Column(...)`
  writes only those columns, so list `updated_at` when the update should bump it. To bun an upsert is an
  insert: set `UpdatedAt` before it, and name in its `DO UPDATE` every column the conflict should change,
  `updated_at` included if it should move.
- **A limit on a has-many relation is shared.** `Relation("X", func(q) { return q.Limit(1) })` limits the
  one query that loads the rows of every parent, not each parent's rows.
- **Paging.** A paged list ends with `page.apply(q, KindPost, "?TableAlias.published_at", "?TableAlias.id")`
  (`pkg/repo/paging.go`), which sorts newest first and keeps the rows after the page's item.
- **Errors.** A single-row `Scan` that finds nothing returns `sql.ErrNoRows`, which `notFound` turns into
  `ErrNotFound`. A unique violation is a `*pgconn.PgError` with code `23505`.

## Tests

- Services are tested against `testdb.New(t)` with the factories: no
  interfaces or mocks for the store.
- Repositories are tested through their services, except queries with
  logic of their own (the graph, pagination) and `Store` itself.
- A query that keeps a database-side rule for a strong reason, or a filter
  a service rule depends on, is asserted directly against `testdb`
  (`UnconfirmedUserIDsCreatedBefore` in the pruning test).
- A moved test keeps its assertions. If an assertion must change, the
  behavior changed.
