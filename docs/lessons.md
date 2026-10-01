# Lessons

Generalizable lessons from running the waves. Wave-specific notes go in
`docs/archive/history.md`.

- **Coverage from a subprocess.** A binary built with `go build -cover` writes
  its data only when it exits normally (main returns, or `os.Exit`). Stop it
  with a signal it handles, not a kill. Under `go test -cover`, pass the child the
  test's `-test.gocoverdir` value: the test process's own `GOCOVERDIR` is
  not the directory the caller asked for.
- **Build overlays and coverage.** `go build -overlay` files are ignored by
  the cover tool for instrumented packages, and files under the module cache
  can't be overlaid at all. To inject test-only code, overlay a package that is
  excluded from `-coverpkg`.
- **Subagents cat files.** Even with AGENTS.md's reading rules, W0's
  subagents printed whole files 33 times and loaded LSP once. The task
  preamble now says it explicitly.
- **Parallel subagents in one Go package.** One half-written file breaks the
  compile for every agent in the package. Give each agent its own build tag
  while it iterates (`//go:build browser && b3`, run with `-tags browser,b3`)
  and have it switch to the shared tag before reporting. Shared build steps
  (`yarn build`) run once in the coordinator, not in every agent, or they race
  on the output directory.
- **A cheap tier needs a self-checking task.** A layout sweep asserts things
  that pass whether or not the check works (`undefined > n` is false). Such a
  task can't catch its own mistakes, so it is not cheap. Ask for a proof that
  each check can fail.
- **Verify bug reports against intent.** A subagent told the wrong expected
  behavior reports the real one as a bug. Before filing, check the code and
  its history (`git log -S`) for a deliberate choice.
- **Mutation-sweep the controllers.** Emptying each Stimulus controller's
  `connect()` in turn and running the suite found the one controller whose
  tests only ever took the happy path (confirm: every test accepted the
  dialog). The agents' own "covers" lists overstated coverage.
- **Check that "no database" is true in the code, not the plan.** Three W1
  targets assumed a pure branch that sits behind a query. Before setting a
  unit-test target, read the function's first statements. Report
  unreachable code, don't pad for it.
- **A skipped test can hide a test that never runs.** `testdb` skips under
  `-short`, so a unit test that quietly used the database passed the
  Docker-free check by not running. Grep a no-database wave's files for
  `testdb` and `factory.` before accepting them.
- **Goldens must not depend on shared sequences.** Factory emails and
  usernames come from a counter shared by parallel tests; a golden that
  contains one depends on test order. Build golden fixtures with fixed
  values.
- **Cheap agents characterize bugs.** Told to pin bugs with skipped tests,
  haiku agents still wrote tests asserting today's wrong output, and
  placeholder tests with commented-out bodies. Read every `t.Skip` and every
  "known bug" comment in their files; the preamble now forbids both.
- **Run go fix before pushing.** CI's Go Fix job commits its rewrites onto
  the branch, and on W1 that commit failed Lint: go fix inlined a
  `//go:fix inline` helper everywhere and left it unused. `make check-q` now
  runs `fix-q` and `lint-q` first, and every task prompt runs them on the
  task's packages before reporting.
- **Prompts can't point at things subagents can't read.** A task row that
  names another wave's matrix ("the D2a matrix") or an issue number gives a
  subagent nothing: it may not read other wave files and has no GitHub
  access. Paste the definition and a one-line summary of each issue into the
  prompt.
- **An envelope looks like a missing field.** A cheap agent that decoded
  API responses without the `{"data": ...}` wrapper concluded the handlers
  were broken and asked for production changes. A "needs: fix the handler"
  from a test wave is a reason to check the agent's reading first.
- **Mutation-check commented-out assertions.** A `// require...` behind a
  FIXME survived the preamble's ban again. `grep -n "FIXME\|// require" `
  over a cheap agent's files before the mutation check.
- **Make the cloud container reproducible first.** The first E2E run in a
  fresh cloud session failed for four environment reasons, none in the
  code. The SessionStart hook now fixes them; if a later wave meets a new
  one, extend the hook rather than working around it by hand.
- **One mutation per task samples one test.** It proved each task's most
  important test, and said nothing about the rest: W3 shipped identical tests
  and assertions that could not fail, found only by the owner. Audit every
  test's assertions (weak, duplicated, or already covered by the other suite)
  before committing a test task.
- **Check the index before committing.** A subagent told not to use git ran
  `git mv` anyway, and its staged renames went into an unrelated commit. Run
  `git diff --cached --stat` before every commit made while subagents are
  running, or commit with explicit paths (`git commit -- <paths>`).
- **Generated code must match its runtime library.** Pin the generator to
  the library version in `go.mod`, and have the generate script refuse on a
  mismatch, so a dependency bump fails CI instead of quietly shipping
  mismatched code.
- **Keep tools out of `go.mod`.** `go tool` directives bring a tool's whole
  dependency tree into the module graph. Install CLI tools in the tools image
  instead.
- **A job log is expensive.** `get_job_logs` with a large `tail_lines`
  returned 46k characters to prove one build step ran. Ask for a narrow tail
  first, and widen it only if that isn't enough.
- **A mutation check per task is not an audit.** Every W2 task passed its
  mutation check, and the assertion audit at the end still found 35 weak
  tests, 18 duplicates and 5 skips that could not fail. The commonest weak
  pattern in package tests is `require.NotNil` on the action a form's `Save`
  returns; the commonest bad skip depends on the host's time zone or on a
  race going the unlucky way. The audit now runs for every test task, as each
  one reports.
- **Fetch master before escalating a bug.** The owner fixes and decides
  things on master while a wave runs. Two W2 "privacy leaks" had already been
  fixed or decided there when they were reported. `git fetch` and
  `git log <branch>..origin/master` come before any bug report or question,
  and the wave branch is rebased when master has moved.
- **A leak the template hides still needs a package test.** `UserHome`
  loaded direct-only posts for unrelated visitors, but the journal template
  shows them no posts at all, so no HTTP or browser test could see it. Only
  the package-level privacy matrix caught it. Keep visibility rules tested at
  the function that queries, not only at the page.
- **Rate limits end parallel agents mid-task.** Three W2 agents stopped when
  the session limit hit; `SendMessage` to each resumed it with its context
  intact once the limit reset. Resume rather than re-dispatch.
- **`git checkout` can't restore an untracked file.** During W5's
  mutation check, restoring a new floors file with `git checkout --` failed,
  and the fallback copied an unrelated backup of the same name from the shared
  temp directory over it. Back up an untracked file under a unique name in the
  session's scratchpad, or break it with an edit you revert with another edit.
- **Ask what else a column feeds before normalizing it.** WB's migration
  lowercased `users.email`, which a legacy password hash was salted with, and
  would have locked users out. Before a data migration rewrites a column,
  grep for everything derived from it: hashes, signatures, cache keys.
- **An ordering test must insert against the grain.** Without a working
  `ORDER BY`, Postgres returns rows in insertion order, so a test that
  inserts in the expected order passes either way. UUID primary keys don't
  order by time either. Now a ground rule in `docs/testing.md`.
- **Host time zone can make a test unable to fail.** Timestamp columns have
  no time zone (#171); on a host ahead of UTC every stored time reads back in
  the future. Run mutation checks of time-comparing tests with `TZ=UTC`.
- **A race test that passes three times proves nothing.** #141's test passed
  with the fix removed. Make the race deterministic by acting inside the
  exact window (an `htmx:afterSwap` listener), then check it fails.
- **A fix that changes a shared format must list its readers.** Rehashing
  on login broke change password, which read the same column. A bug-fix
  task's report should name every other reader and writer of what it
  changed; the WB preamble should ask for it.
- **Subagents' shared-fixture edits break every package at once.** One
  agent's half-written factory option stopped every other task's build and
  the coordinator's mutation checks. Factory additions stay coordinator
  work, or the task that needs one runs alone.
- **Stop on a classifier outage.** When the safety check returns no verdict,
  retry once, then wait. Work a subagent finished during the outage gets the
  coordinator's own review before it is committed.
- **A verbatim move is a script, not a task.** When the code to move is a
  set of top-level blocks, cut them by line range and prove the move by
  comparing the old and new files' line multisets: only the scaffolding may
  differ. It is exact and costs a few coordinator turns; agents copying code
  are neither (R1 step 2).
- **Before a refactor wave, grep the tests for source reads.** A test that
  opens `.go` files by path (R1's route-table guard) breaks on every move
  and looks like a behavior change. Find them first and agree with the
  owner what may be updated.
- **A wrapper that rebinds input can change behavior silently.** Routing an
  action with no request body through `BindJSON` would have added a new
  error. List which handlers fit the wrapper's exact shape before
  converting, and leave the rest.
- **BSD `sed -i -E` leaves `<file>-E` backups.** On macOS it's `sed -i ''
  -E`. Check `git status` for strays after a subagent, not only
  `git diff --stat`, which doesn't show untracked files.
- **Agent worktrees start from `origin/master`.** Push the wave branch, or
  make a worktree agent's first command `git merge --ff-only <wave branch>`
  and check that a step 0 file exists (RS).
- **Split shared files before parallel work, not after.** RS step 0 split
  the mixed handler and page files by area and gave each task its own
  `<table>_<area>.go` repository files; the six merges then conflicted only
  in the registry and the allowlist, which scripts resolved.
- **Audit every task, even when its mutation check passes.** In RS the
  mutation checks passed and the audits still found deleted tests, changed
  texts and lost panics in five of seven tasks.
- **Don't mutate a worktree an audit is reading.** Run the mutation check
  before or after the audit, never during it: an audit that reads the
  mutated file reports a regression that doesn't exist.
- **Resolve conflicts in code by hand.** A resolver that unions lines works
  for import blocks and struct fields; for function bodies it drops
  repeated lines like `}`.
- **A refactor must keep failure paths, not only results.** Where the old
  code panicked (500 plus the admin mail), the new code must still panic;
  returning an error that a form shows as text is a behavior change.
- **Moving code between packages changes what per-package coverage sees.**
  Measure with `-coverpkg` over the module, or a refactor that moves
  queries out of their tests' package looks like lost coverage (RS).
- **Don't trust a cached e2e pass.** The e2e harness runs the server as a
  separate binary, which go test's cache can't see; until the harness
  stat'ed the sources (R3), `make test-q PKG=./e2e/...` reported a cached
  "ok" after production edits. Any test that execs a binary built from the
  repo needs the same treatment.
- **A mutation that passes may be testing an empty fixture.** Delivering
  the wrong mail passed a "delivered equals queued" check because the
  factory's payload is `{}`; mutate a field the fixture actually sets.
- **A breaking library bump is sequential work.** Every task needs the new
  go.mod and the tree compiles only after the last one, so order the tasks
  by import graph, run them on the wave branch one at a time, and give each
  the package list that can compile at its point.
- **Pin a library PR by commit to keep going.** go.mod on the PR's
  pseudo-version lets the wave run while the library release waits; before
  closing, move to the tag and check that the tag's tree equals the pinned
  commit.
- **Remove a coupling rather than abstract it when nothing needs it.**
  gogo's executor parameter only served one app's mail queue; after RS the
  app's forms ignored it. Deleting it beat designing an ORM-agnostic
  interface.
- **Revert a mutation from a backup, never with git, in a subagent's
  worktree.** Its work is uncommitted, so `git checkout <file>` erases it.
  Copy the file aside before breaking it and copy it back.
- **"Every setting tested" means tested where it takes effect.** A table
  that parses each variable catches renames, not a setting that is parsed
  and ignored or wired into the wrong field. Test each setting's on and off
  at its point of use, and the composition root's mapping one setting at a
  time.
- **A negative assertion on asynchronous work needs a barrier, not a
  sleep.** "No mail arrived" after a fixed wait passes whenever the mail is
  merely late. Wait for the producer to finish (the queue drained), then
  assert, and use addresses no parallel test shares.
- **A contract task must leave the tree runnable.** When a contract removes
  a fallback (the console sender), the replacement's plumbing (the tommy
  container, harness env) belongs in the same task, even if the plan put it
  later.
- **In a feature wave, say which suite owns a rule.** F1's E2E index and
  feed tests re-seeded the whole visibility matrix the service test already
  owned. The service test owns the rule; a route test proves the wiring with
  one case the rule admits and one it rejects.
- **"Reuse X" needs X's file in the owned list.** Told to render the feed's
  post item but not given `feed.html`, a subagent copied the markup rather
  than extract a partial. If a task should share code, own the file it lives
  in.
- **Parallel tasks in one package invent the same constant.** F1's index
  and RSS tasks each defined a 50-post limit and one hardcoded the project
  name. Name shared constants, and their owner, in the plan; otherwise the
  coordinator reconciles them at merge.
- **Put a wave file's task table before its task sections.**
  `task_prompt.py` pastes a `###` section up to the next heading, so the last
  section also carried the table and closing notes into its prompt.
- **A feature that resends a notification must check the queue's dedup
  key.** The mail queue drops a repeated (type, unique id); F2's plan reused
  the new-comment mails for edits without saying so, which would have sent
  the first edit's mail at most. Look at how the existing sender keys
  messages before planning a "notify again" feature, and test two events in
  a row.
- **A hidden prefilled form duplicates text on the page.** Browser
  assertions that search the whole page or card for a comment's text pass
  from the textarea alone; scope them to the rendered body.
- **When Docker Hub rate-limits the sandbox (429), pull from
  `mirror.gcr.io/<image>` and `docker tag` it back** rather than waiting or
  skipping the suite.
- **A field that reaches a template crosses a page struct.** F3's plan
  owned the service (`reading.Journal`) and the template (`user_home.html`)
  but not `pkg/web/pages_*.go`, which copies one into the other, so the
  template task stopped. When a service view gains a field a page shows,
  own the page constructor too.
- **Check that the code a plan says to copy exists.** F3 told A0 to add a
  `ViewProfile` case wherever `ViewComment` has one; there was none. A
  `grep -n` while writing the plan is cheaper than a subagent's detour.

- **A signature change's callers belong in the owned list.** F4's R0
  changed five repository methods and the reading service; four files
  outside its list (another service, a page builder, two web tests) had to
  change to compile. The subagent handed patched copies back under `needs:`,
  which worked but cost a round trip. When a task changes a signature, list
  its callers (LSP `findReferences`) in the plan.
- **Mutate the server, not only the markup, for htmx swaps.** Removing
  `hx-get` from the "Load more" link and making the handler return the full
  page both left F4.R1's first browser tests green: body `hx-boost` fetched in
  place and the pasted page still contained the expected items. The rule
  now lives in the `frontend-htmx` skill.
- **Fact-check prose against the code with a strong reader.** F6's mid-tier
  guide draft read well and was wrong in 15 places, mostly by generalizing
  ("every account gets an invitation"). A read-only audit citing file and
  line for each claim caught them; the same applies to any docs task.
- **Look at generated artifacts, not just their existence.** `make
  screenshots` "wrote every PNG", but two showed empty pages and one a
  contradictory comment count; both were seed gaps no test noticed.
- **A task that changes the test harness owns the routes the harness
  calls.** F5 planned `LoginAs(email)` in the contract task, before the
  code routes existed; it could only move to the task that adds them.
- **Tasks that share a file are not parallel.** Run them in sequence rather
  than plan a merge; F5's M1 and M2 shared a route file, a test file and a
  form.
- **"Merged now, deployed later" doesn't hold where deploys migrate.** fly's
  `release_command` runs every pending migration, so a deferred migration
  goes on a stacked branch with its own PR.
- **A test of a printed secret uses it.** F5's CLI test checked that six
  digits were printed; a wrong HMAC key passed it until the test logged in
  with the code.
- **Plan file deletions for the coordinator.** The permission check refuses
  a subagent command that deletes tracked files; the coordinator removes
  them with `git rm` (with the owner's approval) before dispatching.
