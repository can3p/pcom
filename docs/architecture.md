# Architecture: layers

pcom has three layers. `pkg/arch/arch_test.go` enforces them, and its
allowlist names every file that doesn't follow them yet. Shares
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
  `sender.Sender`.
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
- **Model types cross the boundary for now.** Repositories and services
  return the generated `core` structs, or small structs built from them, so
  templates don't change (open question Q13).
- **Page builders** in `pkg/web` (`pages_<area>.go`) take service results,
  not a database: `web.SharedPost(c, userData, shared)`.
- **Legacy code.** RS is done: the arch test's allowlist is empty and stays
  so. Code that still takes an executor gets `store.Exec()` only in tests
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
  views or defaults. F5 first defined the mailbox rule as a Postgres
  function only the app called; it is now `pgsession.CanonicalEmail`, with a
  table test, stored on every insert.
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
