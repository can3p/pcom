---
name: wave-run
description: Coordinate one wave of the pcom modernization plan (W0-W6, WB, R1-R6, RS) - what to read, how to build subagent prompts with task_prompt.py, dispatch at the right model tier, and verify each task cheaply. Use when starting, resuming or dispatching tasks of a wave, or when asked to "run W1" or similar.
---

# Running a wave

Most of the cost of the modernization is not writing code; it is **context that gets re-read every turn** and
**output nobody needed**. The rules for reading and verifying code cheaply are in `AGENTS.md`. This skill
adds what only a coordinator needs. When the wave is done, use the `wave-close` skill.

## 1. Read only what your job needs

| Role | Reads |
|---|---|
| Coordinator | `docs/implementation-plan.md` (the index), the one wave file `docs/plan/<id>.md`, `docs/open-questions.md` |
| Subagent | `docs/testing.md`, its prompt (which carries the task excerpt), and the source files it owns or tests. In RS and later refactor waves, also `docs/architecture.md` |

`AGENTS.md` is loaded automatically into every session and every subagent, as are the skill descriptions.
Never read another wave's file, `docs/archive/`, or `docs/gogo-extraction.md` unless the task says so.

## 2. Start

1. Branch as the index's "Branching" section says. Every session of this wave must run on the wave's
   branch: `tools/agent-stats.py --branch <branch>` measures the wave by it.
2. Tell yourself in one line what the previous session left ("W0 merged, W1 next"), not a summary.

## 3. Build prompts; don't write them

```bash
.claude/skills/wave-run/task_prompt.py w1 U1 U2 U3     # one prompt per task, separated by =====
```

It pastes the task's table row or `###` section into a preamble and prints the `model` to use on the first
line. Replace every `<FILL: ...>`, above all the owned files, before dispatching. There are three preambles,
chosen by wave:

- **Test waves** (W0–W5): read only `docs/testing.md`; LSP, the `model-shape` and `test-failure` skills;
  quiet `make` targets; the 12-line report.
- **W6**: the same, but tests run through `make test-ui` and compile with `TAGS=browser`.
- **R-waves, RS and WB**: a refactor preamble. Behavior is unchanged, `e2e/` is not edited, moved tests keep
  their assertions, and the layering rules apply.

W4's tooling tasks use the test preamble with its first line and "Test only" line adjusted by hand.

## 4. Dispatch; don't do

The coordinator's context is the expensive one: it lives for the whole wave. Bulk writing belongs in
subagents, whose context is thrown away.

- Every task with a tier goes to a subagent at that tier or cheaper, as the first line of its prompt says
  (`haiku` for cheap, `sonnet` for mid, `opus` for strong). The coordinator writes code itself only when
  the plan says so (contract-defining work), or after a task has failed twice.
- Tasks that own disjoint files go out in **one message** with several `Agent` calls,
  `subagent_type: "general-purpose"`. Never `fork`: a fork drags the coordinator's context along.
- Parallel tasks that write files in **one package** (W3's `e2e`, W6's `e2e/browser`) each iterate under
  their own build tag (`//go:build browser && b3`, run with `-tags browser,b3`) and switch to the shared
  tag before reporting, so one agent's half-written file doesn't break the others' compile. Shared build
  steps such as `yarn build` run once in the coordinator before dispatch, never in each agent.
- A broad question ("where is X used across the handlers?") goes to an `Explore` subagent, which returns the
  answer rather than the files. The coordinator's own lookups use the LSP tool.
- **Subagents run no git commands** and don't touch `go.mod` (only W0 and W4.S2 do, each as a single task).
  They report gaps in the test factories instead of patching around them. Add missing helpers in one place,
  then re-dispatch. If two tasks independently ask for the same helper, it is real.
- **Escalate once, don't loop.** A task that fails its "done when" twice is bumped one tier, once, with the
  failure summary. If it fails there, stop and report rather than burning more tokens.
- **Tier down when in doubt about difficulty, not about consequences.** If a mistake would be caught by a test
  the task writes itself, go cheap. If it would go unnoticed (a permission test that passes when it should
  fail), go strong.

**Diagnostics from subagents land in your context.** gopls reports every broken intermediate state of every
running subagent's files to the coordinator, not to the subagent. Ignore diagnostics for files a running
subagent owns; act only on those still present once it reports DONE. Subagents check compilation themselves
with `make vet-q` (their prompt says so).

**Name the skills in the prompt.** Tested: a cheap subagent given a plain question used neither the skills
nor LSP and read generated code in full, while the same model told which skill to use followed it. The
preamble from `task_prompt.py` names them; keep it that way in hand-written prompts too.

Subagents reply in at most 12 lines: `DONE | BLOCKED <id>`, `files:`, `coverage:`, `bugs:`, `needs:`. No code,
no logs. If you need a detail, ask with `SendMessage`, which keeps the subagent's context.

## 5. Verify each task on a budget

1. `make cover-q PKG=<task packages>`: the number is right and the tests pass. Lines marked `(cached)` did
   not re-run; after an edit to the code under test they must not be cached.
2. `git diff --stat`, to confirm only the owned files changed.
3. **One** mutation check: break the code under the test whose failure would matter most, watch it fail
   through `make test-q`, revert. A test that doesn't fail when you break the code under it covers nothing.

Variations by wave:

- **W6 (browser):** step 1 is `make test-ui RUN=<the task's tests>`, then again with `COUNT=3` (a flaky
  test is sent back, not accepted). The mutation check breaks a Stimulus
  controller or an htmx attribute the task covers. At the end of the wave, empty each controller's
  `connect()` in turn and run the suite: a controller that survives is untested, whatever the reports say.
- **R-waves and RS (refactors):** replace step 1 with `make test-q PKG=<task packages>` plus
  `git diff --stat -- e2e/`, which must be empty. Also check that the task shrank the `pkg/arch` allowlist
  and didn't grow it. The mutation check becomes: break the service method the task extracted and watch an
  E2E or service test fail. Run `make test-ui` once per commit, next to `make check-q`.

Not part of the budget: reading every test file. Read a test only when the mutation check fails to fail.
Run `make check-q` and `make lint` (CI lints; `check-q` does not) once before each commit; commit per task.

For each bug a subagent reports: check `gh issue list --label bug` and the "Known bugs" list in
`docs/plan/wb.md`, file an issue if it is new (security bugs go to the owner, not a public issue), and put
the number into the test's `t.Skip`.

## 6. Session hygiene

- **One wave per session.** State lives in files, not in the conversation: the status table, the wave file,
  `docs/archive/history.md`. Start the next wave in a fresh session (or after `/clear`).
- After each commit, if the conversation is long, `/compact` with the instruction
  "keep: current wave, task statuses, open bugs filed, next step".
- W0, W6.B0 and RS step 0 are the coordinator's own contract work; their delegable parts are marked in
  their wave files. Everything else is almost entirely dispatch: expect a few thousand coordinator tokens per
  task, not tens of thousands.
