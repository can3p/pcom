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
