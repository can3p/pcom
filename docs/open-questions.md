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

## Decided

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
