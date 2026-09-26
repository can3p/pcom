# Modernization history

Finished waves, newest last. Each entry records what was built, what turned
out wrong, what was left out on purpose, and what the wave cost in tokens.

## W0 — Test foundation (2026-09-26, branch `test/w0-foundation`)

**Built.**

- `pkg/testutil/testdb.New(t)` over gogo `v0.0.2`'s `testcontainers/postgres`
  (a template database, one copy per test). pcom's dockertest-based copy is deleted, and the
  existing DB tests use the new helper.
- `pkg/testutil/factory`: a builder per table, the graph relationships, and
  readers in `read.go`, with a self-test that builds every entity. It doesn't import
  `testing`, so the seed command can use it.
- `pkg/testutil`: `Must`, `fakesender`, `fakestorage`, `ginctx`, `golden`.
- `e2e/`: builds `./cmd/web` once with `-cover`, runs it per test against
  its own database with a stub asset manifest, and drives it with a
  cookie-jar client that sends the scraped CSRF token and exposes htmx
  headers. Two smoke tests (anonymous home, factory user logs in to `/feed`).
- `make test` writes `coverage.out`, `make test-short` skips E2E, `make cover`
  merges unit and E2E binary coverage into one per-package table;
  `codecov.yml`; `docs/testing.md` rewritten with worked examples.
- `task_prompt.py` now pastes a task's `###` section together with its table
  row, and takes ownership from "Owns …".

**Turned out wrong.**

- The plan assumed the E2E binary could simply be killed. It has no signal
  handling, and a `-cover` binary writes coverage only on a normal exit. A build
  overlay adds a SIGTERM handler. It can't go into `cmd/web`, because the cover tool ignores
  overlays on the packages it instruments, and it can't go into a dependency, because the module cache
  can't be overlaid. So it goes into `pkg/types`, a package without statements, which is left out of
  `-coverpkg`.
- `go test -cover` gives the test process its own `GOCOVERDIR`; the harness
  passes the binary the `-test.gocoverdir` value instead.
- Small shape changes against the plan: `Client.PostJSON` takes a path, not
  an action name; `ginctx.New` takes options (`WithUser`, `WithCSPNonces`)
  after the four planned arguments.

**Left out.** `pkg/feedops/testutil` stays, for W2.D8. (CI switched from
`make test` to `make cover` before merge, so Codecov sees E2E coverage: Q14.)

**Cost.** 5 sessions (coordinator plus 4 subagents: 2 sonnet builders, 1 haiku, 1 sonnet docs),
280 turns in total; the coordinator peaked at 148k context with 88k of tool
results, and the subagents at 43k–167k (the factories were the most expensive:
74 turns, 122k of results). 33 wasteful calls across all agents, nearly all
`cat` of whole files, plus 2 raw builds. Skills: wave-run and wave-close once
each. model-shape was used through `make model` (5 calls). LSP was loaded once
in total and test-failure never, because nothing failed. This is the first
recorded wave, so there is no earlier cost line to compare with.
