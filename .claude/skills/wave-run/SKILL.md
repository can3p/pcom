---
name: wave-run
description: Coordinate one wave of the pcom modernization plan (R-waves and F-waves) - what to read, how to build subagent prompts with task_prompt.py, dispatch at the right model tier, and verify each task cheaply. Use when starting, resuming or dispatching tasks of a wave, or when asked to "run R5" or similar.
---

# Running a wave

Most of the cost of the modernization is not writing code; it is **context that gets re-read every turn** and
**output nobody needed**. The rules for reading and verifying code cheaply are in `AGENTS.md`. This skill
adds what only a coordinator needs. When the wave is done, use the `wave-close` skill.

## 1. Read only what your job needs

| Role | Reads |
|---|---|
| Coordinator | `docs/implementation-plan.md` (the index), the one wave file `docs/plan/<id>.md`, `docs/open-questions.md` |
| Subagent | `docs/architecture.md`, `docs/testing.md`, its prompt (which carries the task excerpt), and the source files it owns or tests |

`AGENTS.md` is loaded automatically into every session and every subagent, as are the skill descriptions.
Never read another wave's file, `docs/archive/`, or `docs/gogo-extraction.md` unless the task says so.

## 2. Start

1. Branch as the index's "Branching" section says, and set the wave file's `Status` to `running`. Every
   session of this wave must run on the wave's branch: `tools/agent-stats.py --branch <branch>` measures the
   wave by it.
2. Tell yourself in one line what the previous session left ("L0 merged, L1 next"), not a summary.

## 3. Build prompts; don't write them

```bash
.claude/skills/wave-run/task_prompt.py r7 L1 L2 L3     # one prompt per task, separated by =====
```

It pastes the task's table row or `###` section into a preamble and prints the `model` to use on the first
line. Replace every `<FILL: ...>`, above all the owned files, before dispatching. If the excerpt names
something a subagent can't read (another wave's matrix, an issue by number, an open question), paste its
definition or a one-line summary into the prompt: subagents read no other wave files and have no GitHub
access. The preamble is chosen by wave:

- **R-waves**: a refactor preamble. Behavior is unchanged, `e2e/` is not edited, moved tests keep
  their assertions, and the layering rules apply.
- **F-waves**: a feature preamble. Behavior changes as the wave file decides; tests prove the feature
  (browser tests for what users do); an existing assertion changes only where the task changes it. A feature
  wave's last task is its docs task (guide, screenshot table, `make screenshots`, site build).

## 4. Dispatch; don't do

The coordinator's context is the expensive one: it lives for the whole wave. Bulk writing belongs in
subagents, whose context is thrown away.

- Every task with a tier goes to a subagent at that tier or cheaper, as the first line of its prompt says
  (`haiku` for cheap, `sonnet` for mid, `opus` for strong). The coordinator writes code itself only when
  the plan says so (contract-defining work), or after a task has failed twice.
  One more exception: a verbatim move of top-level blocks is a script, not a task. Cut by line range and
  prove it by diffing the old and new files' line multisets.
- Tasks that own disjoint files go out in **one message** with several `Agent` calls,
  `subagent_type: "general-purpose"`. Never `fork`: a fork drags the coordinator's context along.
- Tasks whose tests build the whole tree (E2E, the arch test) run with `isolation: "worktree"`, so one
  agent's half-written files don't break another's build. Worktrees start from `origin/master`, not the
  wave branch: push the branch first, or make the agent's first command `git merge --ff-only <wave
  branch>` (the only git command it may run). Merge each finished worktree by committing there and
  cherry-picking onto the wave branch; resolve shared files (a registry, an allowlist) by script, and
  conflicts in code by hand.
  A worktree has no `cmd/web/node_modules` and no built `cmd/web/dist` (both untracked), and the E2E tests
  serve `cmd/web/dist`: a task that doesn't change assets links the main checkout's
  (`ln -s /home/user/pcom/cmd/web/dist cmd/web/dist`); one that does links `node_modules` the same way and
  builds once. Parallel worktrees share golangci-lint's lock: "parallel golangci-lint is running" means
  retry, not a lint failure. Delete finished worktrees (`git worktree remove`) once merged; their copies
  of the tree show up in every repository-wide grep.
  With a worktree of its own, an agent may run `make test-ui` freely. After merging parallel tasks, run the
  full browser suite once: two tasks can each pass alone and break together through a shared controller.
- **Design references the subagents can't open** (a private claude.ai canvas, a mockup behind a login):
  read them once yourself, save the boards as files in your scratchpad and name that path in every prompt,
  with the decisions that override the board.
- Parallel tasks that write files in **one package** (`e2e`, `e2e/browser`) each iterate under
  their own build tag (`//go:build browser && b3`, run with `-tags browser,b3`) and switch to the shared
  tag before reporting, so one agent's half-written file doesn't break the others' compile. Shared build
  steps such as `yarn build` run once in the coordinator before dispatch, never in each agent.
  Don't have them prefix identifiers with the task id (`e1User`, `b2Anonymous`): the tag already isolates
  them, and the prefix is noise once merged. Name helpers for what they do; a name clash at merge time is
  the signal that a helper belongs in a shared `helpers_test.go`, which the coordinator writes once.
  Test names take the area, as in `TestActions_ShareLifecycle`.
  The same applies to ordinary packages shared by several tasks. There, a
  task's coverage target is for its own files or functions: tell it to measure them with
  `go tool cover -func` on a profile. When a shared package suddenly fails to build, check first for an
  agent that left its files untagged.
- A broad question ("where is X used across the handlers?") goes to an `Explore` subagent, which returns the
  answer rather than the files. The coordinator's own lookups use the LSP tool.
- **Splitting the work is yours.** The wave file plans the tasks; you decide who owns what, so that no
  two parallel tasks write the same file (`go.mod`, a registry, a shared helper) and no task waits on a
  file another is still writing. Whatever a task's prompt doesn't list as owned, it doesn't edit. A shared
  helper whose last caller one task removes fails that task's lint as unused: give that task the deletion,
  and delete the helpers left unused after merging the rest yourself.
- **Long prompts shared by several tasks** go into a file in your scratchpad, one per task, and the
  dispatch says only "read <path> and follow it": the coordinator's context doesn't carry six copies.
- **Work that stays uncommitted across tasks** (a plan that commits several tasks as one) is backed up
  without moving HEAD: commit the working tree through a temporary `GIT_INDEX_FILE` with
  `git commit-tree` and push it to a `wip/` branch. The proxy refuses to delete remote branches, so the
  owner deletes it after the real commit lands.
- **Subagents run no git commands** and delete no tracked files (the permission check refuses it): a
  deletion is yours, with `git rm`, before dispatch. They report gaps in the test factories instead of
  patching around them. Add missing helpers in one place, then re-dispatch. If two tasks independently ask
  for the same helper, it is real.
- **Resume, don't re-dispatch.** A subagent stopped by a rate limit keeps its context: continue it with
  `SendMessage` once the limit resets. When the safety classifier returns no verdict, retry once, then wait,
  and review yourself whatever a subagent finished during the outage.
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

1. The tests pass:
   - **R-waves:** `make test-q PKG=<task packages>` plus `git diff --stat -- e2e/`, which must be empty, and
     the `pkg/arch` allowlist is still empty.
   - **F-waves:** `make test-q PKG=<task packages>`; for browser tests, `make test-ui RUN=<the task's tests>`,
     then again with `COUNT=3` (a flaky test is sent back, not accepted).

   Lines marked `(cached)` did not re-run; after an edit to the code under test they must not be cached.
2. `git status --short`, to confirm only the owned files changed and no stray files appeared (`git diff
   --stat` misses untracked files, such as the `<file>-E` backups BSD `sed -i -E` leaves).
3. **One** mutation check: break the code the task changed where a failure would matter most (for a
   browser test, a Stimulus controller or an htmx attribute it covers), watch a test fail, and revert from a
   copy you made first under a unique name in your scratchpad (`cp` aside and back; never `git checkout`,
   which erases a subagent's uncommitted work and can't restore an untracked file). Break something the
   fixture actually sets: a field the factory leaves empty passes either way. Never while an audit agent is reading the same worktree: it will report your mutation
   as a regression. A test that doesn't fail when you break the code under it covers nothing.
4. **Audit the assertions** of every task that writes tests, because one mutation samples one test. Have a
   read-only `Explore` agent (strong tier) classify every new test as weak (passes whether or not the
   behavior works: only a status on a page that always answers 200, "body exists", `NotNil` on a returned
   action, asserting the setup), duplicated within the wave, duplicated by another suite (package, E2E,
   browser), a skip that can't fail, or OK, with a concrete fix for each. Run it per task as it reports, not
   once at the end, and send the fixes back before committing: tasks that pass their mutation check still
   ship weak, duplicated and unfailable tests that only this step finds. Before it, grep the task's files
   for `FIXME` and `// require`: a commented-out assertion is the commonest way a test stops testing.

A wave that adds Stimulus controllers ends with one more check: empty each new controller's `connect()` in
turn and run the browser suite. A controller that survives is untested, whatever the reports say.

Not part of the budget: reading every test file. Read a test only when the mutation check fails to fail.
Run `make check-q` and `make test-ui` once before each commit; `check-q` runs go fix and lint first, as CI
does. If go fix rewrote anything, the rewrite goes into that task's commit. Commit per task, with explicit
paths (`git commit -- <paths>`) or after `git diff --cached --stat`: a subagent may have staged something.

For each bug a subagent reports outside its task: `git fetch` and look at `git log <branch>..origin/master`,
since the owner may have fixed or decided it there; check the code's history (`git log -S`) and
`docs/product.md` for a deliberate choice; then check `gh issue list --label bug` and file an issue if it
is new (security bugs go to the owner, not a public issue). It is not fixed in the wave's diff. If the wave
can't proceed without the fix, make it on its own branch from `origin/master`, open it as a separate PR, and
rebase the wave onto it once it merges.

## 6. Session hygiene

- **One wave per session.** State lives in files, not in the conversation: the wave file and its
  `Status`, the commits on the wave branch. Start the next wave in a fresh session (or after `/clear`).
- After each commit, if the conversation is long, `/compact` with the instruction
  "keep: current wave, task statuses, open bugs filed, next step".
- Contract work the wave file gives the coordinator is its own; everything else is almost entirely dispatch:
  expect a few thousand coordinator tokens per task, not tens of thousands.
