# Open questions

Decisions the owner hasn't made yet. **Do not resolve one in code.** Once a
question is answered, move it to "Decided" with the date and the answer.

Raised by the 2026-09-21 modernization survey.

## Product and behavior

- **Q9. The RSS poller downloads feeds and images inside the DB transaction**
  that holds the feed row lock (`feeder.refreshFeeds`). With a 2-minute image
  budget per item, a transaction can stay open for minutes. Restructure when
  moving email and feed work to a general job queue?
- **Q10. Audit and idempotency for mutations (narrowed 2026-09-24).** The
  layering is decided (below): services take an actor, check authorization,
  and HTTP, API and CLI are transports over them. Still open: should every
  mutating service method also write an audit entry and accept an
  idempotency key?
- **Q13. Domain types at the repository boundary.** RS lets the generated
  `core` structs cross from repositories into services and templates, so RS
  doesn't touch templates. Should repositories return pcom-owned types
  instead? That decouples templates and services from the ORM, at the cost
  of mapping code. Deciding before R5 matters, because bob changes the
  generated types anyway (see `docs/plan/r5.md`).
- **Q17. Production log format and destination (R7).** JSON to stdout for
  the platform to collect, or something else? And should request logs carry
  the user ID, given the privacy rules for direct-only content?

Raised by planning F2–F7 on 2026-10-01. Each wave file states the default
it builds; confirm or change it before the wave starts.

- **Q18. Comment edits (F2, #176).** Default: only the author, only while
  they may still comment on the post; no mail to the post's author or the
  participants on an edit; the marker shows the last edit time, no
  history. Should an edit notify anybody, or should the post's author be
  able to see the previous text?
- **Q19. Profile section (F3, #186).** Default: shown only on the journal
  page, to whoever may see the journal, under no heading of its own (a
  block with the class `us-profile-about`); stored in its own table; empty
  clears it. Should it also appear elsewhere (the per-user public RSS
  feed's description, a hover card, explore), and should it have a heading
  ("About")?
- **Q20. Pagination (F4, #124).** Default: cursor pagination merging the
  sources, 30 items a page, a "Load more" button (no infinite scroll), on
  the feed, explore, the anonymous index and the journal (the journal is
  not in the issue); RSS outputs capped at F1's 50 and not paged. Is the
  journal in scope, and is 30 right?
- **Q21. Magic links and prefetching (F5, #175).** Default: the mailed link
  opens a page with a "Log in" button (a POST); no JS auto-submit, because
  link scanners that run JS would log in; the link is not tied to the
  browser that asked for it; 15 minutes, single use, at most 3 unused links
  per 15 minutes. Alternatives: auto-submit with JS, or a code typed into
  the original tab (works across devices, survives every scanner, costs a
  step).
- **Q22. Removing passwords (F5).** Default: every password path goes in
  the wave; the `pwdhash` column is dropped by a later migration, deployed
  after the wave has run in production; a CLI command `web admin
  login-link --email` prints a link for an operator, as the way in when
  mail is down. Keep a password login as a fallback instead? Is the CLI
  escape hatch wanted?
- **Q23. Invitations and signup without passwords (F5).** The issue: the
  invite page asks to confirm, then mails a magic link. That's the default.
  The alternative: log in right after confirming, since the invitation
  link already proves the inbox, one step fewer. Which one? Open signup
  (`FORCE_SIGNUP`) follows the same answer.
- **Q27. Language detection and target (F7).** Default: a local Go
  library detects the language on publish, edit and RSS import, free and
  offline, less accurate on short texts; English is the only target, stored
  per user so others can come later. Alternative: the backend's own
  detection (Azure's `/detect`), which sends every published post that
  allows translation to the provider, translated or not. Which library: the
  owner checks the candidates' license and binary size at T0.

## Decided

- **2026-10-01. Q26, translation provider (F7, #144):** translation is
  common infrastructure (`pkg/translate`) with a pluggable backend chosen by
  configuration; Azure Translator is the first backend. Limits are in
  characters: per user per day, site-wide per month (default 2M, Azure
  F0's cap). Every translation is labelled as automatic, with the
  provider's name and "Show original". Integration and browser tests run
  against a mock Azure API built on a new universal API-mock primitive
  (`pkg/testutil/apimock`), never a real provider; tommy stays for mail and
  storage. Options considered, cheapest first (2026-10 prices):
  self-hosted LibreTranslate; the MT free tiers (Azure F0 2M chars a month,
  DeepL and Google 500k); Claude Haiku 4.5 (~$1.5 per million characters);
  Azure S1 $10, Google $20, DeepL Pro $25 per million characters; Claude
  Sonnet 5.5 and Opus 5.5. Each is a later backend.

- **2026-10-01. Q24, the public website (F6, #145):** copy tommy's website
  (`can3p/tommy/website`): its own Go module in `website/`, the page list in
  Go, a landing page made only of slices of existing files, links to
  unpublished files sent to GitHub and asserted by a test, deployed by
  `pages.yml` to `can3p.github.io/pcom`. Developer docs are published, as
  tommy's are; the wave files are not. pcom adds a user guide in
  `docs/guide/` and screenshots from `make screenshots`.

- **2026-10-01. Q25, translation and privacy (F7, #144):** the author
  decides. A post is sent to the model provider for translation only if its
  author allowed it with a per-post toggle (off by default, so no existing
  post is translated until its author opts in); RSS items are always
  translatable. Comments and profiles are out of scope.

- **2026-10-01. Public post feed (#146, wave F1):** for an anonymous
  visitor `/` lists the 50 newest posts that Q15 allows (no pagination
  yet), and `/rss/public` serves the same list. `/explore` stays for
  logged in users; an anonymous visitor is redirected to `/`. The anonymous menu is Home, Sign up, Login; the
  "Why" article goes, `/articles/:id` stays for the legal pages. The feed
  is advertised in the page head and by a visible RSS icon; the navbar gets
  a GitHub icon. RSS items are dated by publication, not creation.
- **2026-09-30. R2 production switches:** `FLY_APP_NAME` is replaced by one
  setting per behavior (`SECURE_COOKIES`, `HSTS`, `STATIC_CACHE`,
  `MEDIA_PERMA_CACHE`, `REPORT_PANICS`, `SHOW_ERRORS`, `LOG_LEVEL`) with
  production-safe defaults; development, compose and tests turn them off.
  `seed` keeps refusing to run when `FLY_APP_NAME` is set.
- **2026-09-30. Migrations at deploy (R2 D2):** no `migrate` subcommand. The
  production image carries the `sql-migrate` binary (same pinned version as
  `tools/Dockerfile`), `dbconfig.yml` and `migrations/`, and fly's
  `release_command` runs it. `go.mod` keeps no migration library.
- **2026-09-30. Source-scanning E2E test during refactors:**
  `TestGuards_RouteTableMatchesSource` reads route files by name, so a
  refactor that moves routes updates only its `prefixes` map; the route
  table it asserts stays unchanged (R1, and RS after it).
- **2026-09-29. Q7, post zip export (#110):** anyone who can see the post
  may export it; the export follows post visibility. Fixed in WB.
- **2026-09-29. Duplicate invitations (#168):** a second pending invitation
  to the same address (compared lowercased) is rejected with a form error,
  and a partial unique index enforces it. Fixed in WB.
- **2026-09-28. Generator and migration tools stay out of `go.mod`:**
  sql-migrate and sqlboiler (later bob) are installed in `tools/Dockerfile`,
  pinned by `ARG`s. `generate.sh` refuses to run when the generator and the
  runtime library in `go.mod` differ.
- **2026-09-28. macOS is the primary development platform.** Local tooling
  and docs target Docker Desktop on macOS first.
- **2026-09-28. Q16, delete on a post that was never saved:** nothing is
  stored. Today a new draft is saved; filed as #163, pinned by a skipped
  test, scheduled in WB.

- **2026-09-21. Q2, the API deletes any post:** fixed right away in PR #118,
  outside the wave order.
- **2026-09-21. Q1, Q3, Q4, Q5 (password hashing, unescaped HTML emails,
  private RSS URL carrying the API key, session rotation):** filed as #119,
  #120, #121 and #122, and scheduled in WB.
- **2026-09-21. Q6, signups:** they stay off on purpose, because bots were
  abusing the endpoints. Re-enabling them with bot protection is #123, for
  the future. Don't delete the waiting list in the meantime.
- **2026-09-21. Q8, feed pagination:** #124, after R1.
- **2026-09-21. Local development and tooling:** everything runs against
  docker-compose: Postgres, plus object storage replacing the local file
  storage. Migrations, sqlboiler (later bob) and psql run from a dev tooling
  container, so the host needs no build dependencies. Models are generated
  from a freshly migrated database, and CI checks they are current (W4, R2).
- **2026-09-21. Q11, running the app in compose:** yes, as a `dev` profile
  (W4.S4). The host-mode `make watchexec` flow must keep working alongside
  it.
- **2026-09-21. Q12, object storage:** `adobe/s3mock`, pinned. It is the
  least configuration: the bucket is created from one environment variable,
  there are no credentials and no init container, and it supports
  path-style addressing. MinIO no longer publishes maintained community
  images; SeaweedFS, Garage and RustFS all need bucket or layout
  bootstrapping. Superseded: for tests on 2026-09-24, and for development
  on 2026-09-26, by tommy's S3 listener (below).
- **2026-09-21. Mail in development and tests:**
  [tommy](https://github.com/can3p/tommy) runs in compose and in the E2E
  harness. The Mailjet sender points at it through a configurable base URL,
  and the console sender is dropped (R2).
- **2026-09-24. Layering:** database code is encapsulated in repositories
  (`pkg/repo`), business rules and authorization in services
  (`pkg/service/<area>`), and handlers stay thin. R1 moves the handlers and
  RS extracts the layers, enforced by an architecture test. R5 then swaps
  the ORM inside `pkg/repo` only.
- **2026-09-24. S3 in tests:** tommy (v0.2.0 or later) is the S3 target for
  the S3 storage tests and the E2E harness, in the same container that
  captures mail. Development keeps `adobe/s3mock` in compose (Q12), because
  tommy holds its S3 catalog in memory and development uploads should
  survive a restart. Once
  [can3p/tommy#40](https://github.com/can3p/tommy/issues/40) (persistence)
  and [#41](https://github.com/can3p/tommy/issues/41) (buckets from config)
  ship, compose drops s3mock for tommy.
- **2026-09-26. S3 in development:** both tommy issues shipped in v0.3.0
  (`TOMMY_PERSIST`, `TOMMY_S3_BUCKETS`), so compose runs tommy as the only
  object store and s3mock is dropped before W4 starts (W4.S2). Verified
  against `can3p/tommy:0.3.0`: the configured bucket exists at startup, and
  an object survives recreating the container on the same volume.
- **2026-09-24. Browser tests:** playwright-go in `e2e/browser`, behind a
  build tag, reusing the E2E harness and the factories (W6). No pixel
  snapshots until browsers run in the tools container.
- **2026-09-28. Q15, profile visibility and public posts:** a public post
  is visible to everybody at `/posts/:id`, whatever the author's profile
  visibility. Such posts must not appear in the public posts feed (#146),
  which respects profile visibility.
- **2026-09-26. Q14, E2E coverage in CI:** CI runs `make cover`, so Codecov
  gets the merged unit and E2E coverage, `cmd/web` included.
