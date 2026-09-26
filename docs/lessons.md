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
