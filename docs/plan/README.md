# Working economically

The modernization is many hundreds of lines of plan and thousands of lines of
tests. Most of the cost is not writing code; it is **context that gets re-read
every turn** and **output nobody needed**. These rules cut both. They apply to
the coordinator and to every subagent.

## 1. Read only what your job needs

| Role | Reads |
|---|---|
| Coordinator | `AGENTS.md`, `docs/implementation-plan.md` (the index), the one wave file, `docs/open-questions.md` |
| Subagent | `AGENTS.md`, `docs/plan/<wave>.md` (only its task row and the sections it references), `docs/testing.md` once W0 has written it, and the source files it owns or tests |

- Never read another wave's file, `docs/archive/`, or `gogo-extraction.md`
  unless the task says so.
- Read source with `grep -n` first and `Read` with `offset`/`limit` after.
  `cmd/web/main.go` alone is ~1000 lines.
- **Generated code is not read.** `pkg/model/core/*.go` is huge. Get the shape
  with `go doc ./pkg/model/core User` (or `go doc -all` for one type), or
  `grep -n 'type User struct' -A30`.
- Don't re-read a file you just wrote or edited.

## 2. Print only what decides the next step

Use the quiet targets, which print one line on success and a trimmed report on
failure (the full log is kept, and the path is printed):

```bash
make check-q                       # build + vet + tests, quiet
make test-q PKG=./pkg/links/...    # tests for one package tree
make cover-q PKG=./pkg/links/...   # "ok <pkg> ... coverage: 91.2%": the number a task is judged on
make vet-q PKG=./pkg/links/...
```

- While iterating, run one package, or one test:
  `go test ./pkg/x/ -run TestName -count=1`. Run the whole suite once per
  commit, not per edit.
- Never `go test -v ./...`, never `-json`, never `cat` a log. To dig into a
  failure, `grep -n` the log path the failure report gives you, or re-run the
  single failing test with `-v`.
- Golden files: after `UPDATE_GOLDEN=1`, look at `git diff --stat`, not the
  contents. Open a golden only when its test is the subject of the review.
- Pipe unavoidable noisy commands (`docker compose`, `yarn`, `gh run view`)
  through `tail -n 40` or `grep`.

## 3. The coordinator plans, dispatches and verifies; it does not write bulk code

The coordinator's context is the expensive one: it lives for the whole wave.
Bulk writing belongs in subagents, whose context is thrown away.

- **Dispatch, don't do.** Every task in a wave file that names a tier goes to a
  subagent at that tier or cheaper. The coordinator writes code itself only
  when the plan says the coordinator does (contract-defining work), or after a
  task has failed twice.
- **Parallel:** tasks that own disjoint files go out in **one message** with
  several `Agent` calls. Use `model: "haiku"` for cheap, `"sonnet"` for mid and
  `"opus"` for strong. Use `subagent_type: "general-purpose"`; never `fork`
  (a fork drags the coordinator's context along, which is the cost being
  avoided).
- **Prompts are self-contained and short.** Use the template in
  `docs/implementation-plan.md`, with three additions:
  - the exact `make` commands to verify with (the quiet ones above);
  - "print nothing you don't need; do not paste code or logs into the report";
  - the report format below.
- **Escalate once, don't loop.** A task that fails its "done when" twice is
  bumped one tier, once, with the failure summary. If it fails there, the
  coordinator stops and reports rather than burning more tokens.
- **Tier down when in doubt about difficulty, not about consequences.** The
  plan's rule of thumb stands: if a mistake would be caught by a test the task
  writes itself, go cheap. If it would go unnoticed (a permission test that
  passes when it should fail), go strong.

### Report format (subagent → coordinator, at most 12 lines)

```
DONE | BLOCKED  <task id>
files: <paths created or changed>
coverage: <pkg> <n>%
bugs: <test name> — <one-line repro>      (one line each, or "none")
needs: <missing factory helper / code change>   (or "none")
```

No code, no logs, no narrative. If the coordinator needs a detail it asks with
`SendMessage`, which keeps the subagent's context.

### Coordinator verification budget, per task

1. `make cover-q PKG=<task packages>`: the number is right and the tests pass.
2. `git diff --stat`, to confirm only the owned files changed.
3. **One** mutation check per task: break the code under one test, watch it
   fail through `make test-q`, revert. Choose the test whose failure would matter
   most.

Not part of the budget: reading every test file. Read a test only when the
mutation check fails to fail.

## 4. Session hygiene

- **One wave per session.** State lives in files, not in the conversation: the
  status table, `docs/archive/history.md`, and the wave file's own checklist.
  Start the next wave in a fresh session (or after `/clear`) and re-read only
  the files in section 1.
- After each commit, if the conversation is long, `/compact` with the
  instruction "keep: current wave, task statuses, open bugs filed, next step".
- Tell the coordinator in one line what the previous session left
  ("W0 merged, W1 next"), not a summary of what was done.

## 5. Which wave tasks are cheap to hand off

Everything the plan tags cheap or mid is a candidate for a subagent, and the
wave files say which. Two structural points:

- **W0 is the only wave with real coordinator work** (contracts). Its
  delegable parts are marked in `docs/plan/w0.md`.
- **W1–W4 are almost entirely dispatch.** Each task already has disjoint files,
  a target coverage and a tier. The coordinator's job is: build prompts from the
  table row, dispatch a batch in one message, run the verification budget,
  commit, next batch. Expect the coordinator to spend a few thousand tokens per
  task, not tens of thousands.
