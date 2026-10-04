# What can move into gogo

Generic plumbing that could move into [github.com/can3p/gogo](https://github.com/can3p/gogo), so that every
gogo-based project shares one copy. **Extract only code that has tests**, and move the tests along with it.

pcom depends on `github.com/can3p/gogo v0.1.0` and uses `forms`, `sender` (mailjet), `links.ArgBuilder`,
`util/transact`, `testcontainers/postgres`, `apperr`, `settings`, `util/ginhelpers` and
`util/ginhelpers/csrf`. gogo takes no executor: forms call services and the mail queue is pcom's
`repo.MailQueue`, so the ORM never reaches gogo. What has already moved is in the history files under
`docs/archive/history/`.

## Candidates

Roughly ordered by payoff:

| Candidate | pcom location | Notes |
|---|---|---|
| Image media server | `pkg/media/server` (+ `storage/s3`) | libvips resizing by named class, caching server, concurrency limiter, S3 storage behind `MediaStorage`. Self-contained apart from `media.ErrNotFound`. **Highest value.** |
| Goldmark extensions | `pkg/markdown/mdext/*` | linkrenderer (target/rel by view), lazyload, headershift, videoembed (YouTube), blocktags (gallery). They depend on `types.HTMLView` and `types.Replacer`, which would need to become generic options. |
| CSP nonce middleware | `pkg/util/ginhelpers/csp` | Parametrize the source lists (today they're built from env at init). |
| Mail definitions and preview | R4 (planned) | Typed definitions with sample factories, an `html/template` layout, golden helpers and a preview handler. Design it so it imports nothing from pcom, and extract it right after R4. |
| Webpack/asset manifest loader | `LoadStaticManifest` in `pkg/web/app/funcmap.go` | `static_asset` funcmap with CDN prefix. |
| Template helpers | `toMap` in `pkg/web/app/funcmap.go`, `renderHumanTime` (`pkg/util/date`) | A small `gogo/tmplfuncs`. |
| Postgres session store | `pkg/pgsession` | Wraps `antonlindstrom/pgstore` (stale) for gin-contrib/sessions, plus user-in-context. Replace pgstore when extracting. |
| Signed return URLs, referer guard, flashes | `pkg/auth` (`HashValue`, `RedirectToLogin`, `EnforceReferer`, `AddFlash`/`GetFlashes`) | Tiny, but every gogo app reimplements them. Should use HMAC rather than salted SHA-256. |
| Form validators | `pkg/forms/validation` | Email (with the anti-disposable check), username, URL, min/max, enum. Pairs with `gogo/forms`. |
| Panic reporter | `pkg/admin/panics.go` | `ClonedCustomRecovery` (a copy of gin's recovery output) + email to the admin. |
| URL normalization | `pkg/util/url.go` (`NormalizeURL`) | Pure function, tested. |
| Timezone list | `pkg/util/tz.go` | IANA zones for settings dropdowns. |
| JSON action helper | `pkg/web/app` | `jsonAction[T]`: bind, call, `{explanation}` on error. Pairs with gogo's `API` renderer. |
| RSS fetcher | `pkg/feedops/reader` | Size and time-limited fetch with MIME validation. Maybe; it's pcom-specific today. |
| DB-backed mail queue | `pkg/mail/sender/dbsender` | **Don't extract as is.** R4 considers replacing it with a general job queue with a transactional outbox. Extract that instead, if it happens. |
| Boolean switch setting | `pkg/config.Switch` | A go-flags boolean that can default to true (`--x`, `--x=false`, `X=false`; empty is an error). Belongs next to `settings.Secret` as `settings.Switch`. |
| tommy test container | `pkg/testutil/tommy` | One tommy container per test binary (Mailjet API, S3 bucket, read-back API URLs). Belongs next to `testcontainers/postgres` as `testcontainers/tommy`. |
