---
name: wave-close
description: Finish a pcom modernization wave (R-waves and F-waves) - fold what the wave learned and decided into the docs and skills, prune what it made stale, write its history and token statistics, make split commits, open the PR and watch CI. Use when all tasks of a wave are done, or when asked to wrap up, close or ship a wave.
---

# Closing a wave

A wave is finished when the docs describe the code as it now is and CI is green, not when the code lands.
The docs are how the next session learns: what this wave found out goes into them, and what it made untrue
comes out.

1. **Plan.** Delete `docs/plan/<id>.md`; nothing else lists the waves. Edit a later wave's file only where
   what you learned changes it, and drop its `Depends on` for this wave.
2. **Decisions.** Write each decision the wave made or relied on where it applies: `docs/product.md` for how
   pcom behaves, `docs/architecture.md` for how it is built, `docs/testing.md` or `docs/running.md`, an
   area's `AGENTS.md`. State it as the current rule, with its reason. Delete answered questions from
   `docs/open-questions.md`.
3. **Learn.** Go through what went wrong or surprised you (subagent reports, audits, review findings,
   your own detours). For each finding, ask whether it would have helped most sessions doing that kind of
   work, or only this one:
   - **General:** add it as a rule, with its reason, where that work's reader looks: `AGENTS.md` (everyone),
     a skill (`wave-run` for coordination, `test-failure`, `frontend-htmx`, `model-shape`), the "Planning
     a wave" section of `docs/implementation-plan.md`, `docs/testing.md`, `docs/architecture.md`, or the
     subagent preambles in `task_prompt.py`. Say it once, in the place it applies; if a rule there already
     covers it, sharpen that rule instead of adding a second.
   - **One-off:** it goes in the history only.
4. **Prune.** Remove what the wave made stale, so the docs carry no history:
   - `grep -rn '\b<ID>\b'` over `AGENTS.md`, `.claude/skills`, `docs/*.md`, `docs/plan`, the area
     `AGENTS.md` files and `task_prompt.py`. A rule that only applied while the wave ran is deleted; one
     that still holds is restated without the wave's name ("before R5" / "from R5 on" becomes the rule).
   - Check the paths, symbols, commands and numbers the docs name in the areas the wave touched against
     the code (`grep`, LSP); fix what moved.
   - A rule the wave proved wrong is rewritten, not annotated.
5. **History.** Write `docs/archive/history/<id>.md`, headed like the others (`# F7 — Translation (done
   <date>, branch ..., #issue)`):
   - what was built, what turned out wrong, what was deliberately left out, the decisions taken and the
     options weighed, and which docs and skills steps 2–4 changed;
   - a **Cost** line from `tools/agent-stats.py --branch <wave branch> --no-agents` (without `--no-agents`
     if a subagent looks expensive; keep the output out of your context beyond what you record): sessions,
     subagents, total turns, the coordinator's peak context, tool-result volume (`res`), wasteful-call flags,
     and which skills were used how often. Compare with the most recent history file's line in one sentence.
     This is how the economy rules and skills are judged; if a skill was never used or didn't help, say so.
6. **Documentation (F-waves only).** Check that the guide pages the wave changed are updated, that new
   screens are in the screenshot table, that `make screenshots` was rerun and its PNGs committed, and that
   the site builds and its tests pass (`cd website && go test ./... && go run . -out ../site`). Open the
   screenshots: a PNG that exists can still show an empty page. Have a strong read-only agent fact-check
   new prose against the code, citing file and line for each claim; drafts read well and generalize wrongly.
7. **Commits.** Logically split commits, with the docs commit last. Never a single "wave complete" commit.
8. **PR and CI.** Before pushing, run `make cover` and then `make cover-check`: the check reads whatever
   `.cover/` the last `make cover` left, so on its own it can pass on stale data. Then push, open the PR
   (with `--base <parent branch>` if the parent wave hasn't merged), and watch CI with
   `gh pr checks --watch`. On a failure, read `gh run view --log-failed | tail -n 60` and follow the
   `test-failure` skill. That includes the `browser` job. **A wave with red or pending CI is not finished.**
9. **Report and stop.** Report the PR link, coverage change, bugs filed, the docs and skills you changed in
   steps 2–4, and the cost line. Merging is the owner's call.
