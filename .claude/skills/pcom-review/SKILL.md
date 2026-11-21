---
name: pcom-review
description: Review a pcom diff (a branch, a commit range or a PR) against pcom's own rules - idiomatic use of forms, jsonAction, links and layers; duplication; UI consistency with existing screens; critical gaps (authorization, CSRF, data loss, limits, tests, docs); and a simpler way to the same result. Use when asked to review code or a PR in this repository, or to run a review subagent; the built-in code-review skill hunts bugs, this one checks pcom's conventions.
---

# Review a pcom diff

Read `checklist.md` in this directory first; it is the whole standard. This file is the procedure.

## Scope

1. Find the diff: `git log --oneline origin/master..HEAD` for a branch, or the commits or PR named. Review
   only those changes, but read the surrounding code they touch and the nearest existing screen or flow,
   because lens 2 compares with it.
2. `git diff --stat` first; then read file by file. Navigate with gopls (AGENTS.md), not by reading whole
   files.

## Pass

For each lens of the checklist, in order 3 (gaps), 1 (idiomatic, duplication), 2 (consistency),
4 (simpler):

- Look for the red flags listed. Grep for the pattern across the diff (`c.PostForm` in handlers, a literal
  `/controls/` path in a template, a second helper doing what an existing one does).
- For lens 2, name the existing screen you compare with and the difference.
- Verify every candidate in the code before keeping it: a finding that turns out wrong costs the reader
  more than a missed nit.

## Report

- Findings ranked as the checklist says, each with lens, `path:line`, the problem, the fix and its cost.
- Then "checked, fine": the lenses or areas you looked at and found nothing, in one line each, so the
  reader knows what was covered.
- No code, no logs. Don't edit files unless asked to fix; when asked, fix in one commit per finding group,
  with the repo's checks (AGENTS.md "Verifying economically").

A review subagent's prompt names the diff and points it at this skill; it reports back rather than
posting to GitHub, unless the user asked for PR comments.
