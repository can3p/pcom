# Implementation plan: modernization

Forward-looking only. This file is the **index**: status, ground rules and how
waves are run. Each wave's tasks live in their own file, `docs/plan/<wave>.md`
(`w0.md`, `w1.md`, … `wb.md`, `r1.md`, …). **Read this file and the one wave file
you are working on; never the other wave files.**

Running a wave (prompts, dispatch, verification) is the `wave-run` skill;
finishing one (history, cost, PR) is the `wave-close` skill. Both live in
`.claude/skills/<name>/SKILL.md`, which agents without skill support read
directly. Rules for reading and verifying code cheaply are in `AGENTS.md` and
apply to everyone.

The end state:

- [go-flags](https://github.com/jessevdk/go-flags) configuration and a single binary with subcommands;
- [bob](https://github.com/stephenafamo/bob) instead of sqlboiler;
- declared, golden-tested mailers;
- a decomposed router with **thin handlers**: every query lives in a
  repository (`pkg/repo`), every business rule and authorization check in a
  service (`pkg/service/<area>`), and handlers and CLI subcommands only
  translate to and from service calls. An architecture test enforces it;
- a browser test suite, so frontend changes can be iterated on safely;
- a docker-compose development stack (Postgres, S3-compatible object storage,
  and [tommy](https://github.com/can3p/tommy) as the mail sink) with every
  build tool in a container;
- shared plumbing in [gogo](https://github.com/can3p/gogo).

**None of that starts until the safety net exists.** Waves W0–W6 only add
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
| W6 | Browser tests (playwright-go) | W0 | not started | `test/w6-browser` |
| WB | Bug-fix wave (#108–#117, #119–#122) | W1–W3 | not started | `fix/wb-survey-bugs` |
| R1 | Router decomposition (move handlers) | W3, W6, WB | planned | `refactor/r1-router` |
| RS | Repositories and services, thin handlers | R1 | planned | `refactor/rs-layers` |
| R2 | go-flags config, single binary, tommy mail and S3 in tests, object storage only | RS, R3 (mailjet BaseURL) | planned | `refactor/r2-config` |
| R3 | gogo convergence | W5 | planned | `refactor/r3-gogo` |
| R4 | Mailers | W1 (mail goldens), R2 | planned | `refactor/r4-mailers` |
| R5 | bob ORM, one repository at a time | RS, R3 | planned | `refactor/r5-bob` |
| R6 | Dependency hygiene | any time after W5 | planned | `chore/r6-deps` |

```
            ┌── W1 (12 tasks) ──┐
            ├── W2 (9 tasks)  ──┤
W0 ─────────┼── W3 (6 tasks)  ──┼── W5 ── WB ── R1 ── RS ── R2 ── R4
(1 session) ├── W4 (5 tasks)  ──┘              │     │
            └── W6 (7 tasks) ──────────────────┘     └─ R5 (also after R3)
                                                  R3: after W5   R6: any time
```

Each wave's tasks are in `docs/plan/<id>.md` (lowercase: `w0.md`, `wb.md`, `r1.md`).

**After W0 lands, W1, W2, W3, W4 and W6 are independent of each other** and
can run at the same time: about 40 tasks in total, each owning disjoint files.
Each of those waves branches from `test/w0-foundation` (or `master` once W0
has merged), not from each other.

**Layering is two waves on purpose.** R1 moves handlers verbatim, so its diff
is reviewable as a move. RS then extracts repositories and services area by
area, reusing R1's per-area files. R5 comes after RS, so the ORM swap touches
only `pkg/repo`.

Baseline, measured on 2026-09-21 on `master` at 091484d: every test passes, and
**17.2%** of statements are covered, excluding the generated `pkg/model/core`
(3.4% including it). At 0% are `cmd/web`, `pkg/auth`, `pkg/forms`,
`pkg/userops`, `pkg/web`, `pkg/mail`, `pkg/admin`, `pkg/links`, `pkg/pgsession`,
`pkg/postops/rss`, `pkg/media` (upload), `pkg/media/server/storage/*`,
`pkg/markdown/mdext/lazyload` and `pkg/util/ginhelpers/*`.

---

## Ground rules for W0–W6

They are in `docs/testing.md`, because that is the file every test-writing
subagent reads. In short: no production code changes, bugs are pinned with
skipped tests and filed, fixtures only through `pkg/testutil/factory`.

## How waves are run

### Branching

- **One wave, one branch, one reviewable PR.** Never put two waves on one
  branch.
- `git fetch origin` first. If the wave you depend on has merged, or nothing
  is in flight, branch from `origin/master`. If it is still open, branch from
  **that wave's branch** and pass the same base to
  `gh pr create --base <parent-branch>`. A wrong base makes the PR show the
  parent's commits as its own.
- W1–W4 and W6 all depend only on W0, so each branches from W0's branch (or from
  `master` once W0 has merged). They are siblings, not a stack.
- When a parent merges, rebase the child onto `origin/master` and push with
  `--force-with-lease`.
- Merge wave PRs with a merge or rebase merge, **not a squash**. Squashing
  rewrites commits that stacked children already contain.

### Coordinating a wave

A coordinating session (strongest model) dispatches the wave's tasks as
subagents; each task owns the files listed for it and nothing else. How, and
how each task is verified, is the `wave-run` skill. When the wave is done, the
`wave-close` skill updates this file and the history, records the wave's token
cost, and opens the PR.

### Model tiering

| Tier | `model:` value for the Agent tool | Use for |
|---|---|---|
| strong | `opus` | W0; wave coordination; visibility/permission tests (W2.D2a, W3.E1); W6.B0 browser harness; R1 skeleton; RS contracts (step 0) and the visibility service (L1); R5 planning |
| mid | `sonnet` | business-logic tests (connections, forms, feed composition), E2E and browser scenarios, WB fixes, RS area extractions |
| cheap | `haiku` | pure-function unit tests, golden tests, factory-driven CRUD checks, docs, CI config |

The rule of thumb: if a mistake would be caught by a test the task writes
itself, the task can go cheap. If a mistake would go unnoticed,
because a permission test passes when it should fail, the task needs a stronger
model.

### Task prompt template

Built by `.claude/skills/wave-run/task_prompt.py <wave> <task>...`, which
pastes the task's row or section into the standard preamble. Change the
preamble there.
