# Open questions

Decisions the owner hasn't made yet. **Do not resolve one in code.** Once a
question is answered, move it to "Decided" with the date and the answer.

Raised by the 2026-09-21 modernization survey.

## Product and behavior

- **Q7. Post zip export (#110).** Should `/posts/:id/zip` be author-only, or
  should it export the post for anyone who can see it?
- **Q9. The RSS poller downloads feeds and images inside the DB transaction**
  that holds the feed row lock (`feeder.refreshFeeds`). With a 2-minute image
  budget per item, a transaction can stay open for minutes. Restructure when
  moving email and feed work to a general job queue?
- **Q10. "Every mutation is a command".** Should every state change go
  through a named command with an actor, an authorization check, an audit
  entry and an idempotency key? HTTP, API and CLI would then all be
  transports over the same commands. That's large; the alternative is to
  converge only on shared plumbing.

## Decided

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
  bootstrapping. See W4.S2.
- **2026-09-21. Mail in development and tests:**
  [tommy](https://github.com/can3p/tommy) runs in compose and in the E2E
  harness. The Mailjet sender points at it through a configurable base URL,
  and the console sender is dropped (R2).
