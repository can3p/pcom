# What can move into gogo

Notes from the 2026-09-21 survey. The goal is to make pcom lighter by moving
generic plumbing into [github.com/can3p/gogo](https://github.com/can3p/gogo),
so that every gogo-based project shares one copy. **Extract only after the
code has tests** (W1/W2), and move the tests along with the code.

pcom depends on `github.com/can3p/gogo v0.0.1`. From
it, pcom only uses `forms`, `sender` (+console, mailjet), `links.ArgBuilder`
and `util/transact`.

## 1. Done

- **`testcontainers/postgres` upgrade**, merged in
  [can3p/gogo#5](https://github.com/can3p/gogo/pull/5) (`51eb4cb`, not
  tagged yet):
  - testcontainers-go instead of dockertest;
  - migrations applied once into a template database, and every test
    database copied from it;
  - a per-run migration bookkeeping table, defaulting to `migrations`;
  - `New(t, …)` with `t.Cleanup`.

  W0.T0.1 switches pcom to it and deletes pcom's copy.

## 2. Already in gogo; pcom keeps its own copy (switch in R3)

| pcom | gogo | Difference |
|---|---|---|
| `pkg/util/ginhelpers/render.go` | `util/ginhelpers/render.go` | gogo's `HTML` takes a `Redirector` instead of importing `auth`. Otherwise the same. |
| `pkg/util/ginhelpers/csrf/csrf.go` | `util/ginhelpers/csrf/csrf.go` | gogo takes a `getUserCSRFToken` func. Otherwise the same. |
| `pkg/util/cluster.go` | `util/cluster.go` | gogo has `SetCluster(test)` + `IsFlyCluster`. R2 removes the concept from pcom entirely, in favor of explicit settings. |
| `testcontainers/postgres` | `testcontainers/postgres` | gogo's is now ahead (section 1). Switch in W0, not R3. |

## 3. Small gogo changes the modernization needs

- **`sender/mailjet`: a configurable API base URL.** Add `BaseURL` to
  `config.Config` (`long:"api-base" env:"API_BASE"`) and pass it to
  `mailjet.NewMailjetClient(public, private, baseURL)`, which the SDK already
  accepts. With that, the same sender talks to
  [tommy](https://github.com/can3p/tommy) in development and tests, and to
  Mailjet in production. It's a prerequisite for R2, which drops the console
  sender.
- **go-flags configuration helpers:**
  - `CheckRequired`, which treats an empty environment variable as missing;
    go-flags alone considers it set;
  - a `Secret` string type that redacts itself when printed;
  - `ExplainMissing`, which rewrites "required flag `--database-url`" into a
    message that also names `DATABASE_URL`.

  Every gogo app needs these, and pcom needs them for R2.
- **An ORM-agnostic executor.** `sender.Sender.Send` and `forms.Form` take a
  sqlboiler `boil.ContextExecutor`, and `forms.DefaultHandler` takes
  `*sqlx.DB`. Before pcom moves to bob (R5), gogo needs an executor interface
  that doesn't tie consumers to sqlboiler.
- gogo pins old versions (gin 1.10, goldmark 1.7). Bump them with the first
  extraction.

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
