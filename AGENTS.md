# Agent Context for PCOM Project

Go (gin, bun over pgx, Postgres) server rendering Go templates, with htmx, Stimulus.js and pcom's own design
system (SCSS tokens and components, no CSS framework) on the client. This file is loaded into every session, so it holds only what every task needs. Area notes live next to
the code or in skills (`.claude/skills/<name>/SKILL.md`; agents without skill support read that file); read
the one for the area you touch:

| Area | Notes |
|---|---|
| Templates, JS, SCSS, design system and dark mode, htmx, action controller, `renderHumanTime` | skill `frontend-htmx` |
| A model's fields and relations; writing queries | `pkg/model` (one file per area), "Writing bun queries" in `docs/architecture.md` |
| A failing test or build | skill `test-failure` |
| Running a modernization wave / finishing one | skills `wave-run` / `wave-close` |
| How pcom behaves on purpose: accounts, login, visibility, feeds, comments | `docs/product.md` |
| Layers: handlers, services, repositories, service errors, the arch test; configuration and tooling | `docs/architecture.md` |
| Markdown rendering, view types, custom renderers | `pkg/markdown/AGENTS.md` |
| Adding or changing a mail: inputs, samples, templates, goldens | package doc of `pkg/mail/mail.go`, item 6 in `docs/testing.md` |
| RSS feed fetching and image budgets | `pkg/feedops/AGENTS.md` |
| Writing tests: libraries, test DB, factories, mocks, E2E and browser tests, ground rules | `docs/testing.md` |
| User guide, website, screenshots | `website/`, `docs/guide/` |

## Modernization Plan

Ongoing work is planned in `docs/implementation-plan.md` (the index: wave files, parallel waves, branching).
Each wave's tasks are in `docs/plan/<id>.md`. Coordinators use the `wave-run` and `wave-close` skills and
read the index, the one wave file they run and `docs/open-questions.md` (undecided questions; never resolve
one in code). Subagents read only what their prompt names.

Layering: handlers only bind input, call a service and render; services (`pkg/service/<area>`) hold
business rules, authorization and transactions; all SQL and ORM calls live in repositories (`pkg/repo`).
Don't add queries to a handler.

**Stay in scope.** A diff holds the task it was made for, so it stays reviewable. A bug, a cleanup or a
refactor you notice outside that task is not fixed in it: check `gh issue list` and file an issue
(`--label bug` for a bug; a security bug goes to the owner, not a public issue). If the work can't wait,
fix it on its own branch from `origin/master`, as its own PR. Subagents don't file or branch: they report
it under `bugs:` and the coordinator does.

**The docs describe the present.** A change that makes a doc, a skill or an area `AGENTS.md` untrue updates
it in the same diff; that is in scope. A constraint or decision you learn goes where the next person doing
that work will read it (the table above), stated once as the current rule with its reason. Delete what no
longer holds rather than annotating it. Wave names, dates and incident stories belong in
`docs/archive/history/`, not in the rules; the `wave-close` skill does this for every wave.

**No magic numbers.** A limit or tunable value (a length cap, a page size, a timeout) is a setting in
`pkg/config` with its default there, handed to the service as an option; the service exports the same default
for callers built without configuration, and tests use that constant, not the literal. Pattern:
`Limits.ProfileAboutMaxLength` → `accounts.WithProfileAboutMaxLength`. A new setting also appears in
`web serve --help`, whose golden the same diff rewrites (`UPDATE_GOLDEN=1 go test ./cmd/web -run TestHelp`).

## Reading code economically

- **Navigate with the LSP tool (gopls)**, not by reading files. It is a deferred tool: load it once with
  `ToolSearch` query `select:LSP`.
  - `documentSymbol` gives a file's outline; then `Read` only the lines you need (`offset`/`limit`). This is how
    to approach large files such as `cmd/web/main.go`.
  - `workspaceSymbol` finds a definition by name. It also searches the module cache, so use distinctive names.
  - `hover` and `goToDefinition` give a type or signature; `findReferences`, `incomingCalls` and
    `goToImplementation` answer "who uses this". Positions are 1-based line and column; get them from
    `documentSymbol` or `grep -n`.
  - Don't run `findReferences` on the central model types (`model.User`, `model.Post`): the result is huge.
- Don't re-read a file you just wrote or edited.

## Verifying economically

- **Cloud sessions** are prepared by the SessionStart hook (Docker for the test database, libvips, covdata, CI's
  golangci-lint, Chromium). If a test can't reach Docker or a build can't find `vips`, the hook failed: re-run
  `CLAUDE_CODE_REMOTE=true .claude/hooks/session-start.sh` rather than troubleshooting by hand. Details and
  known sandbox-only browser failures: "Claude Code on the web" in `docs/testing.md`.

- **Compile errors come from gopls for free, in the main session only.** After an edit, its diagnostics
  arrive with the next tool result; fix those instead of running `go build` or `go vet`. **Subagents don't
  receive diagnostics** (they are delivered to the parent session), so a subagent runs `make vet-q PKG=...`
  after editing, before any test run.
- Use the quiet targets. They print one line on success and, on failure, the failing tests, the panic location
  and a log path; the log is kept only on failure.

  ```bash
  make check-q                       # go fix + lint + build + vet + tests: what CI runs
  make fix-q PKG=./pkg/links/...     # go fix, rewriting files in place
  make lint-q PKG=./pkg/links/...    # golangci-lint
  make test-q PKG=./pkg/links/...    # one package tree
  make cover-q PKG=./pkg/links/...   # "ok <pkg> ... coverage: 91.2%"
  make vet-q PKG=./pkg/links/...
  go test ./pkg/x/ -run TestName -count=1   # one test while iterating
  ```

- Iterate on one package or one test; run `make check-q` once per commit.
- **Run go fix on your changes before you finish** (`make fix-q PKG=...`, then `make lint-q PKG=...` and the
  tests). CI's Go Fix job commits whatever go fix rewrites, straight onto the branch, and that commit must
  still pass Lint: a helper go fix inlines everywhere is left unused, for example.
- Never `go test -v ./...`, never `-json`, never `cat` a log. Dig into a failure with `grep -n` on the log path
  the report prints, or re-run the single test with `-v`.
- Golden files: after `UPDATE_GOLDEN=1`, check `git diff --stat`, not the contents.
- Pipe unavoidable noisy commands (`docker compose`, `yarn`, `gh run view`) through `tail -n 40` or `grep`.
  Never pipe a quiet target: the pipe takes `tail`'s exit status, so a failure no longer stops a `&&` chain.
- Don't paste code or logs into reports to a coordinator.
