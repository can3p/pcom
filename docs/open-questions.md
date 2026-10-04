# Open questions

Decisions the owner hasn't made yet that no planned wave owns. **Do not resolve one in code.** A question a
wave needs answered lives in that wave's file, under "Open questions", so it is read with the wave and
leaves with it.

When the owner answers, the answer goes where it applies (`docs/product.md` for how pcom behaves,
`docs/architecture.md` for how it is built, the wave file for what a wave builds) and the question is
deleted here. The reasoning goes into the history of the wave that acts on it.

- **Q9. The RSS poller downloads feeds and images inside the DB transaction**
  that holds the feed row lock (`refreshFeeds` in `pkg/service/feeds/poller.go`).
  With `GlobalImageDownloadTimeout` per item, a transaction can stay open for minutes. Restructure when
  moving email and feed work to a general job queue?
- **Q10. Audit and idempotency for mutations (narrowed 2026-09-24).** The
  layering is decided (`docs/architecture.md`): services take an actor, check authorization,
  and HTTP, API and CLI are transports over them. Still open: should every
  mutating service method also write an audit entry and accept an
  idempotency key?
