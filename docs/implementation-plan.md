# Implementation plan: modernization

Forward-looking only. This file is the **index**: status, ground rules and how
waves are run. Each wave's tasks live in their own file, `docs/plan/<wave>.md`
(`w0.md`, `w1.md`, … `wb.md`, `r1.md`, …). **Read this file and the one wave file
you are working on; never the other wave files.** When a wave ships, **delete its
file**, update the status table, and record what happened in
`docs/archive/history.md` (create that file with the first wave).

**Token discipline, delegation and reporting rules are in
`docs/plan/README.md`. Read it before dispatching anything.**

The end state:

- [go-flags](https://github.com/jessevdk/go-flags) configuration and a single binary with subcommands;
- [bob](https://github.com/stephenafamo/bob) instead of sqlboiler;
- declared, golden-tested mailers;
- a decomposed router;
- a docker-compose development stack (Postgres, S3-compatible object storage,
  and [tommy](https://github.com/can3p/tommy) as the mail sink) with every
  build tool in a container;
- shared plumbing in [gogo](https://github.com/can3p/gogo).

**None of that starts until the safety net exists.** Waves W0–W5 only add
tests, test infrastructure, a seed command and developer tooling.

Related documents:

- `docs/gogo-extraction.md`: what can move into the shared library, noted
  while surveying.
- `docs/open-questions.md`: decisions that are still open, and the ones
  already made.
- GitHub issues #108–#124: bugs and future work found during the survey.
  PR #118 fixes the API post-deletion hole.

---

## Status

| Wave | Name | Depends on | State | Branch |
|---|---|---|---|---|
| W0 | Test foundation | — (gogo `v0.0.2` released) | not started | `test/w0-foundation` |
| W1 | Unit tests, no database | W0 | not started | `test/w1-unit` |
| W2 | Package tests against Postgres | W0 | not started | `test/w2-db` |
| W3 | End-to-end HTTP tests | W0 | not started | `test/w3-e2e` |
| W4 | Local stack (Postgres, s3mock, tommy), dev tooling container, app in compose, seed | W0 | not started | `test/w4-local-stack` |
| W5 | Coverage ratchet | W1–W4 | not started | `test/w5-ratchet` |
| WB | Bug-fix wave (#108–#117, #119–#122) | W1–W3 | not started | `fix/wb-survey-bugs` |
| R1 | Router decomposition | W3, WB | planned | `refactor/r1-router` |
| R2 | go-flags config, single binary, tommy mail, object storage only | R1, R3 (mailjet BaseURL) | planned | `refactor/r2-config` |
| R3 | gogo convergence | W5 | planned | `refactor/r3-gogo` |
| R4 | Mailers | W1 (mail goldens), R2 | planned | `refactor/r4-mailers` |
| R5 | bob ORM | W2, W3, R3 | planned | `refactor/r5-bob` |
| R6 | Dependency hygiene | any time after W5 | planned | `chore/r6-deps` |

```
            ┌── W1 (12 tasks) ──┐
            ├── W2 (9 tasks)  ──┤
W0 ─────────┼── W3 (6 tasks)  ──┼── W5 ── WB ── R1 ── R2 ── R4
(1 session) └── W4 (5 tasks)  ──┘              │
                                               └─ R3 ── R5      R6: any time
```

Each wave's tasks are in `docs/plan/<id>.md` (lowercase: `w0.md`, `wb.md`, `r1.md`).

**After W0 lands, W1, W2, W3 and W4 are independent of each other** and can
run at the same time: about 30 tasks in total, each owning disjoint files. Each
of those waves branches from `test/w0-foundation` (or `master` once W0 has
merged), not from each other.

Baseline, measured on 2026-09-21 on `master` at 091484d: every test passes, and
**17.2%** of statements are covered, excluding the generated `pkg/model/core`
(3.4% including it). At 0% are `cmd/web`, `pkg/auth`, `pkg/forms`,
`pkg/userops`, `pkg/web`, `pkg/mail`, `pkg/admin`, `pkg/links`, `pkg/pgsession`,
`pkg/postops/rss`, `pkg/media` (upload), `pkg/media/server/storage/*`,
`pkg/markdown/mdext/lazyload` and `pkg/util/ginhelpers/*`.

---

## Ground rules for W0–W5

1. **No production code changes.** No file that is compiled into `cmd/web`
   changes, apart from the three additive exceptions below. A wave that finds it
   cannot test something without changing code stops and reports it; that is
   input for R1, not a reason to refactor. Allowed:
   - `_test.go` files, and `testdata/` directories.
   - New packages that production code does not import:
     `pkg/testutil/...`, `e2e/`, and `cmd/seed` (W4).
   - The `testcontainers/postgres` package, `Makefile`, `.github/`,
     `docker-compose.yml`, `.env.example`, `tools/` and `docs/`. The
     migration and codegen scripts (`dbconfig.yml`, `sqlmigrate.sh`,
     `generate.sh`, `sqlboiler.toml`) may change **in W4 only**.
   - `go.mod`/`go.sum`, **only in W0** (test dependencies) and **W4.S2**
     (the `tool` directives).
2. **Bugs are filed, not fixed.** If a test exposes a bug, file a GitHub issue
   with reproduction steps (label `bug`), and write the test for the *correct*
   behavior, skipped:

   ```go
   t.Skip("known bug: https://github.com/can3p/pcom/issues/NNN")
   ```

   WB removes the skips as it fixes the bugs. If the behavior is merely odd
   rather than wrong, write a characterization test that pins the current
   behavior, with a comment saying so. For a security bug, do not open a
   public issue. Report it to the coordinator, who asks the owner.
   Known issues are listed under "Known bugs" below. Check that list before
   filing, so the same bug isn't filed twice.
3. **Tests touch the ORM only through `pkg/testutil/factory`** to create
   fixtures and read state back. A test body that calls `core.Posts(...)`
   directly is a test R5 has to rewrite. This rule is what makes the bob
   migration cheap. The code under test obviously still uses `core`.
4. **Prefer black-box tests.** Use `package foo_test` and the public API unless
   an unexported function has logic worth pinning on its own, such as
   `ConstructComments` or `isURLMediaUpload`. Black-box tests survive
   refactors; tests of internals get rewritten by them.
5. **Assertions use `testify/require`** (and `assert` where continuing after a
   failure helps). Don't add new uses of `alecthomas/assert`, which R6 removes.
   Mocks use mockio v2 as documented in `AGENTS.md`, but prefer the fakes in
   `pkg/testutil`.
6. **Never assert on wall-clock time.** Nothing is injectable yet. Use
   `require.WithinDuration(t, time.Now(), got, 5*time.Second)`, or compare
   ordering.
7. **Every test gets its own database** (`testdb.New(t)`), and tests may run
   in parallel (`t.Parallel()` is encouraged for DB tests).
8. **Mail content is golden-tested** under `testdata/*.golden`, with the
   convention `UPDATE_GOLDEN=1 go test ./pkg/mail/...` to rewrite.
   These goldens are the safety net for R4.

## How waves are run

### Branching

- **One wave, one branch, one reviewable PR.** Never put two waves on one
  branch.
- `git fetch origin` first. If the wave you depend on has merged, or nothing
  is in flight, branch from `origin/master`. If it is still open, branch from
  **that wave's branch** and pass the same base to
  `gh pr create --base <parent-branch>`. A wrong base makes the PR show the
  parent's commits as its own.
- W1–W4 all depend only on W0, so each branches from W0's branch (or from
  `master` once W0 has merged). They are siblings, not a stack.
- When a parent merges, rebase the child onto `origin/master` and push with
  `--force-with-lease`.
- Merge wave PRs with a merge or rebase merge, **not a squash**. Squashing
  rewrites commits that stacked children already contain.

### Coordinating a wave

- A coordinating session (strongest model) dispatches the wave's tasks as
  subagents. Each task owns the files listed for it and nothing else.
- **Subagents run no git commands.** They don't touch `go.mod` (only W0 and W4.S2 do, each as a single task).
  They report gaps in `pkg/testutil` instead of patching around them. The
  coordinator adds missing factory helpers in one place, then re-dispatches.
  If two tasks independently ask for the same helper, it is real.
- The coordinator re-runs `make check` and `make cover` itself. Then it picks
  one test per task and breaks the code under it locally (and reverts) to prove
  the test catches something. A test that doesn't fail when you break the code
  under it isn't covering anything.
- The coordinator commits per task, puts docs last, opens the PR and watches CI
  to green.
- A wave is finished when the documents are accurate again, not when the code
  lands:
  1. Delete the wave's section here and update the status table. Edit later
     waves if what you learned changes them.
  2. Append to `docs/archive/history.md` what was built, what turned out
     wrong, and what was deliberately left out.
  3. Add generalizable lessons to `docs/lessons.md` (create both files on
     first use).
  4. Move answered items in `docs/open-questions.md` to "Decided".
  5. Update `AGENTS.md` if a rule, command or convention changed.
  6. Make logically split commits, with the docs commit last. Never a single
     "wave complete" commit.
  7. Push, open the PR, and watch CI with `gh pr checks --watch`. On a
     failure, read `gh run view --log-failed`. **A wave with red or pending CI
     is not finished.**
  8. Report back and stop. Merging is the owner's call.

### Model tiering

| Tier | Model | Use for |
|---|---|---|
| strong | Opus 5 (`claude-opus-5`) | W0; wave coordination; visibility/permission tests (W2.D2a, W3.E1); R1 skeleton; R5 planning |
| mid | Sonnet 5 (`claude-sonnet-5`) | business-logic tests (connections, forms, feed composition), E2E scenarios, WB fixes |
| cheap | Haiku 4.5 (`claude-haiku-4-5-20251001`) | pure-function unit tests, golden tests, factory-driven CRUD checks, docs, CI config |

The rule of thumb: if a mistake would be caught by a test the task writes
itself, the task can go cheap. If a mistake would go unnoticed,
because a permission test passes when it should fail, the task needs a stronger
model.

### Task prompt template

Every subagent prompt starts with this preamble, filled in. The economy rules
in `docs/plan/README.md` (quiet `make` targets, 12-line report) are part of it:

```
You are adding tests to pcom (Go, gin, sqlboiler, Postgres). Read AGENTS.md,
docs/implementation-plan.md ("Ground rules for W0–W5"), and docs/testing.md.
Task: <id and title from the plan>.
You own exactly these files: <list>. Do not edit any other file.
Do not change production code. Do not run git. Do not edit go.mod.
Create fixtures with pkg/testutil/factory. If a helper is missing, stop and report
exactly what you need rather than writing ORM calls in your test.
If you find a bug: write the test for correct behavior, add
t.Skip("known bug: <describe>"), and include a reproduction in your report —
the coordinator files the issue and fills in the number.
Verify only with `make test-q PKG=./<pkg>/...`, `make vet-q PKG=...` and
`make cover-q PKG=...`; never `go test -v`, never paste code or logs.
Done when: tests pass, vet is clean, and the coverage of <package> is at least
<target>%. Reply in the report format of docs/plan/README.md (at most 12 lines:
coverage reached, bugs found, anything untestable without a code change).
```
