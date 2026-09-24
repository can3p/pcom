---
name: wave-close
description: Finish a pcom modernization wave (W0-W6, WB, R1-R6, RS) - update the plan and history, record the wave's token statistics, make split commits, open the PR and watch CI. Use when all tasks of a wave are done, or when asked to wrap up, close or ship a wave.
---

# Closing a wave

A wave is finished when the documents are accurate again and CI is green, not when the code lands.

1. **Plan.** Delete `docs/plan/<id>.md` and set the wave's row in the status table of
   `docs/implementation-plan.md` to done. Edit later wave files if what you learned changes them.
2. **Measure.** Run `tools/agent-stats.py --branch <wave branch> --no-agents` for the summary, and without
   `--no-agents` if a subagent looks expensive. Keep the output out of your context beyond what you record.
3. **History.** Append to `docs/archive/history.md` (create it on first use), under a heading for the wave:
   - what was built, what turned out wrong, and what was deliberately left out;
   - a **Cost** line from step 2: sessions, subagents, total turns, the coordinator's peak context,
     tool-result volume (`res`), wasteful-call flags, and which skills were used how often. Compare with the
     previous wave's line in one sentence. This is how the economy rules and skills are judged; if a skill
     was never used or didn't help, say so.
4. **Lessons.** Add generalizable lessons to `docs/lessons.md` (create it on first use). If a rule in
   `AGENTS.md` or a skill proved wrong or missing, fix it there, not only in the lessons.
5. **Questions.** Move answered items in `docs/open-questions.md` to "Decided".
6. **Commits.** Logically split commits, with the docs commit last. Never a single "wave complete" commit.
7. **PR and CI.** Push, open the PR (with `--base <parent branch>` if the parent wave hasn't merged), and
   watch CI with `gh pr checks --watch`. On a failure, read `gh run view --log-failed | tail -n 60` and follow
   the `test-failure` skill. From W6 on, that includes the `browser` job. **A wave with red or pending CI
   is not finished.**
8. **Report and stop.** Four lines at most: PR link, coverage change, bugs filed, cost line. Merging is the
   owner's call.
