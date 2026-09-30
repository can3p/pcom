# What can move into gogo

Notes from the 2026-09-21 survey. The goal is to make pcom lighter by moving
generic plumbing into [github.com/can3p/gogo](https://github.com/can3p/gogo),
so that every gogo-based project shares one copy. **Extract only after the
code has tests** (W1/W2), and move the tests along with the code.

pcom depends on `github.com/can3p/gogo v0.1.0`. From
it, pcom uses `forms`, `sender` (+console, mailjet), `links.ArgBuilder`,
`util/transact`, `testcontainers/postgres`, `apperr`, `util/ginhelpers` and
`util/ginhelpers/csrf`; from R2 on also `settings`.

## 1. Done

- **`testcontainers/postgres` upgrade**, merged in
  [can3p/gogo#5](https://github.com/can3p/gogo/pull/5) released as
  `v0.0.2`:
  - testcontainers-go instead of dockertest;
  - migrations applied once into a template database, and every test
    database copied from it;
  - a per-run migration bookkeeping table, defaulting to `migrations`;
  - `New(t, …)` with `t.Cleanup`.

  W0.T0.1 switches pcom to it and deletes pcom's copy.

## 2. Done: gogo v0.1.0, convergence (R3)

Released as `v0.1.0` (can3p/gogo#6); R3 switched pcom to it. In short:

- pcom's `render.go` moved to gogo's `util/ginhelpers`, configured per
  router instead of by `util.InCluster`, and pcom's service errors to
  `apperr`. The CSRF check was already the same.
- `util.InCluster` is deleted from gogo. R2 removes pcom's copy.
- `sender/mailjet` has the configurable API base URL (`BaseURL`,
  `--api-base`/`API_BASE`), which R2 needs for tommy.
- The go-flags helpers are `settings.Parse` (an empty required setting is
  missing; the error names flag and variable) and `settings.Secret`. R2
  uses them.
- **The executor is gone rather than abstracted.** `sender.Sender` is
  `Send(ctx, mail)` and `forms` take none: a form calls services, services
  own transactions, and queueing mail in a transaction is pcom's
  `repo.MailQueue`. So bob (R5) never meets gogo.
- gin 1.12, goldmark 1.8; gogo no longer depends on sqlboiler.

## 4. New candidates from pcom

Roughly ordered by payoff:

| Candidate | pcom location | Notes |
|---|---|---|
| Image media server | `pkg/media/server` (+ `storage/s3`) | libvips resizing by named class, caching server, concurrency limiter, S3 storage behind `MediaStorage`. Self-contained apart from `media.ErrNotFound`. Local storage is being deleted in R2, so it doesn't move. **Highest value.** |
| Goldmark extensions | `pkg/markdown/mdext/*` | linkrenderer (target/rel by view), lazyload, headershift, videoembed (YouTube), blocktags (gallery). They depend on `types.HTMLView` and `types.Replacer`, which would need to become generic options. |
| CSP nonce middleware | `pkg/util/ginhelpers/csp` | Parametrize the source lists (today they're built from env at init). |
| Mail definitions and preview | written in pcom's R4 | Typed definitions with sample factories, an `html/template` layout, golden helpers and a preview handler. Design it so it imports nothing from pcom, and extract it right after R4. |
| Webpack/asset manifest loader | `loadStaticManifest` in `cmd/web/main.go` | `static_asset` funcmap with CDN prefix. |
| Template helpers | `toMap` in `cmd/web/main.go`, `renderHumanTime` (`pkg/util/date`) | A small `gogo/tmplfuncs`. |
| Postgres session store | `pkg/pgsession` | Wraps `antonlindstrom/pgstore` (stale) for gin-contrib/sessions, plus user-in-context. Replace pgstore when extracting. |
| Signed return URLs, referer guard, flashes | `pkg/auth` (`HashValue`, `RedirectToLogin`, `EnforceReferer`, `AddFlash`/`GetFlashes`) | Tiny, but every gogo app reimplements them. Should use HMAC rather than salted SHA-256. |
| Form validators | `pkg/forms/validation` | Email (with the anti-disposable check), username, password, URL, min/max, enum. Pairs with `gogo/forms`. |
| Panic reporter | `pkg/admin/panics.go` | `ClonedCustomRecovery` (a copy of gin's recovery output) + email to the admin. |
| URL normalization | `pkg/util/url.go` (`NormalizeURL`) | Pure function, tested. |
| Timezone list | `pkg/util/tz.go` | IANA zones for settings dropdowns. |
| JSON action helper | written in pcom's R1 | `jsonAction[T]`: bind, call, `{explanation}` on error. Pairs with gogo's `API` renderer. |
| RSS fetcher | `pkg/feedops/reader` | Size and time-limited fetch with MIME validation. Maybe; it's pcom-specific today. |
| DB-backed mail queue | `pkg/mail/sender/dbsender` | **Don't extract as is.** R4 considers replacing it with a general job queue with a transactional outbox. Extract that instead, if it happens. |

## Found in R2

- `pkg/config.Switch`: a boolean go-flags setting that can default to true
  (`--x`, `--x=false`, `X=false`; empty is an error). Belongs next to
  `settings.Secret` as `settings.Switch`.
- `pkg/testutil/tommy`: one tommy container per test binary (Mailjet API, S3
  bucket, read-back API URLs). Belongs next to `testcontainers/postgres` as
  `testcontainers/tommy`.
