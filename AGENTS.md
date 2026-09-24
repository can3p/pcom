# Agent Context for PCOM Project

Go (gin, sqlboiler, Postgres) server rendering Go templates, with htmx, Stimulus.js and Bootstrap on the
client. This file is loaded into every session, so it holds only what every task needs. Area notes live next to
the code or in skills (`.claude/skills/<name>/SKILL.md`; agents without skill support read that file); read
the one for the area you touch:

| Area | Notes |
|---|---|
| Templates, JS, SCSS, htmx, action controller, dark mode, `renderHumanTime` | skill `frontend-htmx` |
| A model's fields, relationships, query helpers | skill `model-shape` |
| A failing test or build | skill `test-failure` |
| Running a modernization wave / finishing one | skills `wave-run` / `wave-close` |
| Markdown rendering, view types, custom renderers | `pkg/markdown/AGENTS.md` |
| RSS feed fetching and image budgets | `pkg/feedops/AGENTS.md` |
| Writing tests: libraries, test DB, factories, mocks, E2E and browser tests, ground rules | `docs/testing.md` |

## Modernization Plan

Ongoing work is planned in `docs/implementation-plan.md` (the index: status, ground rules, how waves run).
Each wave's tasks are in `docs/plan/<id>.md`. Coordinators use the `wave-run` and `wave-close` skills and
read the index, the one wave file they run and `docs/open-questions.md` (undecided questions; never resolve
one in code). Subagents read only what their prompt names. Test waves (W0–W6) must not change
production code; bugs are filed as GitHub issues and pinned with skipped tests.

Target layering (built by waves R1 and RS; new code follows it now): handlers only bind input, call a
service and render; services (`pkg/service/<area>`) hold business rules, authorization and transactions;
all SQL and ORM calls live in repositories (`pkg/repo`). Don't add queries to a handler.

## Reading code economically

- **Generated code is never read.** `pkg/model/core` is 1 MB of sqlboiler output. `make model` lists the
  models and `make model T=User` prints one model's fields, relationships, query helpers and methods in a few
  KB. Reach for `go doc ./pkg/model/core <Symbol>` only for a symbol that summary doesn't cover.
- **Navigate with the LSP tool (gopls)**, not by reading files. It is a deferred tool: load it once with
  `ToolSearch` query `select:LSP`.
  - `documentSymbol` gives a file's outline; then `Read` only the lines you need (`offset`/`limit`). This is how
    to approach large files such as `cmd/web/main.go`.
  - `workspaceSymbol` finds a definition by name. It also searches the module cache, so use distinctive names.
  - `hover` and `goToDefinition` give a type or signature; `findReferences`, `incomingCalls` and
    `goToImplementation` answer "who uses this". Positions are 1-based line and column; get them from
    `documentSymbol` or `grep -n`.
  - Don't run `findReferences` on the core model types (`core.User`, `core.Post`): the result is huge.
- Don't re-read a file you just wrote or edited.

## Verifying economically

- **Compile errors come from gopls for free, in the main session only.** After an edit, its diagnostics
  arrive with the next tool result; fix those instead of running `go build` or `go vet`. **Subagents don't
  receive diagnostics** (they are delivered to the parent session), so a subagent runs `make vet-q PKG=...`
  after editing, before any test run.
- Use the quiet targets. They print one line on success and, on failure, the failing tests, the panic location
  and a log path; the log is kept only on failure.

  ```bash
  make check-q                       # build + vet + tests: the same steps as `make check` (CI)
  make test-q PKG=./pkg/links/...    # one package tree
  make cover-q PKG=./pkg/links/...   # "ok <pkg> ... coverage: 91.2%"
  make vet-q PKG=./pkg/links/...
  go test ./pkg/x/ -run TestName -count=1   # one test while iterating
  ```

- Iterate on one package or one test; run `make check-q` once per commit.
- Never `go test -v ./...`, never `-json`, never `cat` a log. Dig into a failure with `grep -n` on the log path
  the report prints, or re-run the single test with `-v`.
- Golden files: after `UPDATE_GOLDEN=1`, check `git diff --stat`, not the contents.
- Pipe unavoidable noisy commands (`docker compose`, `yarn`, `gh run view`) through `tail -n 40` or `grep`.
- Don't paste code or logs into reports to a coordinator.
