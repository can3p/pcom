---
name: pcom-critic
description: Weigh a proposed change to pcom before any code is written - a plan, a task from a wave file, a design in a PR comment or an idea - against pcom's checklist (idiomatic use of existing mechanisms, no duplication, consistency with existing UI behaviour, critical gaps, the simplest way to the goal), and return a verdict with a simpler alternative. Use before implementing a feature or a non-trivial change, when planning, or when asked to critique a proposal.
---

# Weigh a proposal before writing code

The standard is `.claude/skills/pcom-review/checklist.md`; read it first. A review finds these problems
after the code exists; the critic finds them while they cost a sentence to fix.

## Steps

1. **Restate the goal** in one or two sentences: what the user can do afterwards that they can't now.
   Separate it from the proposed means. If the goal is unclear, ask before going on.
2. **Find what exists.** For each piece of the proposal, find the mechanism pcom already has for it: a
   `pkg/forms` form, `jsonAction` and the action controller, an htmx fragment, `link`, a service, a
   component, a setting. Read the nearest existing screen or flow it should behave like and name it.
3. **Weigh it against the four lenses:**
   - Idiomatic: which parts reuse an existing mechanism, which invent one, and why.
   - Consistency: where the new UI or behaviour would differ from the named existing one.
   - Gaps: authorization, CSRF, limits, data integrity (schema constraints, cleanup of files and jobs),
     tests at the right level, docs to update.
   - Simpler: the smallest change with the same user-visible result. Drop options, states, tables, client
     code or generality no requirement needs.
4. **Check the rules that block work:** an undecided question in `docs/open-questions.md` is not resolved
   in code; scope stays on the task (AGENTS.md "Stay in scope").

## Verdict

- **Go**, **Go with changes** (list them) or **Rethink** (the goal can be met another way; describe it).
- The simpler alternative, if there is one, with what it gives up.
- Open questions only the user can answer.

Keep it short: a page at most. Then implement the agreed version, and run skill `pcom-review` on the diff
before calling it done.
