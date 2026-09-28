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
