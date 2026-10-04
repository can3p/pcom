# Implementation plan

The index of planned work. Each planned wave has its own file, `docs/plan/<id>.md`, which exists while the
wave is planned or running and is deleted when it closes: the files in `docs/plan/` are the status. **Read
this file and the one wave file you work on; never the other wave files.**

Running a wave (prompts, dispatch, verification) is the `wave-run` skill; finishing one (docs, history, cost,
PR) is the `wave-close` skill. Both live in `.claude/skills/<name>/SKILL.md`, which agents without skill
support read directly. Rules for reading and verifying code cheaply are in `AGENTS.md` and apply to everyone.

Where the modernization is going:

- [bob](https://github.com/stephenafamo/bob) instead of sqlboiler (R5);
- declared, golden-tested mailers (R4);
- structured logging through one [zap](https://github.com/uber-go/zap) logger, passed explicitly (R7);
- no unmaintained or duplicate dependencies (R6);
- shared plumbing in [gogo](https://github.com/can3p/gogo) (`docs/gogo-extraction.md`).

F-waves are product features, planned from an issue; unlike R-waves they change behavior on purpose, as
their wave file decides.

Related documents:

- `docs/open-questions.md`: decisions that are still open and no wave owns.
- `docs/product.md`: how pcom behaves on purpose; `docs/architecture.md`: how it is built.
- `docs/archive/history/<id>.md`: one record per finished wave; `docs/archive/decisions.md`: the decision
  log up to 2026-10-04.

---

## Wave files

A wave is `R<n>` (refactor) or `F<n>` (feature), numbered on from the highest id in `docs/plan/` and
`docs/archive/history/`. Its file starts with one line of state:

```markdown
## R5 — bob ORM

Status: planned · Depends on: — · Branch: `refactor/r5-bob`
```

`Status` is `planned` or `running`; `Depends on` names only waves or outside events still pending. Then the
wave's goal and constraints, its task table, the `###` task sections (the table comes first:
`task_prompt.py` pastes a section up to the next heading), and an `### Open questions` section for what the
owner decides before or during the wave. A wave only ever edits its own file: nothing else lists the waves.

## Planning a wave

A wave file is a set of tasks a subagent can finish from its prompt alone. Before writing one:

- **Check what the plan names.** `grep -n` the code a task is told to copy, extend or reuse; a plan that
  points at a case or helper that doesn't exist costs a subagent's detour. A task that changes what a route
  or action does names the file its handler is in (`grep -rn '"<action name>"' pkg/web`), not the file
  its area suggests. Before a refactor, also grep the tests for ones that read source files by path: they
  break on every move and look like behavior changes.
- **Own everything a change reaches.** A task that changes a signature owns its callers (LSP
  `findReferences`); one that adds a field a page shows owns the page constructor in `pkg/web/pages_*.go`
  between the service and the template; one told to reuse markup owns the file it lives in; one that changes
  the test harness owns the routes the harness calls. One that changes shared markup (the header, a partial, a
  Stimulus controller several templates use) owns every test that selects it: grep `e2e/` for its classes,
  ids and texts, and every template for the controller's `data-controller` uses.
- **Tasks that share a file are not parallel.** Run them in sequence rather than plan a merge. Split a mixed
  file by area in a step 0 before parallel work, so later merges conflict only in registries. A breaking
  library bump is sequential too: order its tasks by import graph, each with the packages that compile at
  its point.
- **Name what parallel tasks share**: a constant, a limit, a helper, and which task defines it. Otherwise
  each invents its own.
- **A contract task leaves the tree runnable.** If it removes a fallback, the replacement's plumbing
  belongs in the same task.
- **Check a wrapper's fit before converting to it.** List which handlers match its exact shape; a wrapper
  that rebinds input can add an error where none was.
- **Deploy order is part of the plan.** fly's `release_command` runs every pending migration on deploy, so a
  migration that must wait for a deploy goes on a stacked branch with its own PR.
- **A dependency still in review** can be pinned by commit (a pseudo-version in `go.mod`) so the wave
  proceeds; before closing, move to the tag and check that its tree equals the pinned commit.

## Parallel waves

Waves that don't depend on each other run at the same time, each in its own session, on its own branch from
`origin/master`, merged in whatever order they finish. What keeps the merges small:

- **Records are per wave.** A wave writes its own wave file and, at close, its own history file. There is no
  shared status table, log or lessons list to append to.
- **Shared docs are edited in place.** A wave changes the section its finding concerns (`AGENTS.md`, a
  skill, `docs/product.md`, `docs/architecture.md`, `docs/testing.md`, the guide) and doesn't reflow or
  reorder text around it, so two waves' edits merge as two hunks.
- **Migrations and generated models.** Generated code is never merged by hand: the wave that merges second
  rebases, takes master's `pkg/model/core`, renames its migration to a timestamp after master's newest if
  needed, and reruns `make generate`.
- **`go.mod` and `go.sum`.** The wave that merges second rebases and reruns `go mod tidy`; within a wave the
  coordinator gives them to one task at a time.
- **Coverage floors.** On a conflict in `tools/coverage-floors.txt`, keep the higher floor of each line.
- **Route table.** Every new route needs a row in `TestGuards_RouteTableMatchesSource`; rows from parallel
  waves conflict only textually: keep both.
- **Contracts other waves use.** A wave that changes one (an E2E harness function, a factory signature, a
  shared template partial) says so in its wave file; a wave in flight adapts when it rebases, which shows up
  as a compile error rather than a silent change.
- **The guide.** A feature wave's last task is its docs task: it updates the pages in `docs/guide/` (a new
  page also goes into `docPages` in `website/site.go`), adds new screens to the table in
  `e2e/browser/screenshots/`, reruns `make screenshots` and builds the site; `wave-close` checks it.

## Branching

- **One wave, one branch, one reviewable PR.** Never put two waves on one branch.
- `git fetch origin` first. If the wave you depend on has merged, or nothing is in flight, branch from
  `origin/master`. If it is still open, branch from **that wave's branch** and pass the same base to
  `gh pr create --base <parent-branch>`. A wrong base makes the PR show the parent's commits as its own.
- When a parent merges, rebase the child onto `origin/master` and push with `--force-with-lease`.
- Merge wave PRs with a merge or rebase merge, **not a squash**. Squashing rewrites commits that stacked
  children already contain.
- A fix outside the wave's task goes on its own branch from `origin/master`, as its own PR ("Stay in scope"
  in `AGENTS.md`).

## Coordinating a wave

A coordinating session (strongest model) dispatches the wave's tasks as subagents; each task owns the files
listed for it and nothing else. How, and how each task is verified, is the `wave-run` skill.

### Model tiering

| Tier | `model:` value for the Agent tool | Use for |
|---|---|---|
| strong | `opus` | wave coordination and planning; contract tasks (a harness, a shared interface, step 0 of a refactor); permission and visibility rules; assertion audits |
| mid | `sonnet` | business logic and its tests, E2E and browser scenarios, area extractions in a refactor |
| cheap | `haiku` | pure-function unit tests, golden tests, factory-driven CRUD checks, mechanical conversions, CI config |

The rule of thumb: if a mistake would be caught by a test the task writes itself, the task can go cheap. If a
mistake would go unnoticed, because a permission test passes when it should fail, the task needs a stronger
model.

### Task prompt template

Built by `.claude/skills/wave-run/task_prompt.py <wave> <task>...`, which pastes the task's row or section
into the standard preamble. Change the preamble there.
