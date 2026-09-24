---
name: test-failure
description: Triage a failing Go test or build in pcom cheaply. Use when `make check-q`, `make test-q`, `make cover-q` or a `go test` run reports FAIL, a panic, or a compile error, before reading any log or test file.
---

# Triage a failing test

Goal: find the cause while reading as little as possible. Stop at the first step that explains the failure.

1. **Read the report you already have.** The quiet targets print the failing test names, the assertion
   messages, the panic and the first stack frame in your code, then a log path. Most failures are explained here.
2. **Compile errors:** in a main session, gopls already reported them in the diagnostics after your last
   edit; fix those without rebuilding. A subagent doesn't receive diagnostics: run `make vet-q PKG=<pkg>`.
3. **One test, verbose:** `go test ./pkg/x/ -run '^TestName$' -count=1 -v 2>&1 | tail -n 40`.
   Add `/subtest_name` to the `-run` pattern for table tests.
4. **Search the log, don't print it:** `grep -n -A5 'TestName' <log path>`. Never `cat` a log.
5. **Go to the code under test with the LSP tool,** not by reading the file: `documentSymbol` for the
   outline, `goToDefinition` from the failing line, then `Read` with `offset`/`limit` around that function.
6. **Decide what kind of failure it is:**
   - The test is wrong: fix the test.
   - The code is wrong and you are in a test wave (W0–W5): don't fix it. Write the test for the correct
     behavior, add `t.Skip("known bug: <describe>")`, and report a one-line repro. Check
     `gh issue list --label bug --search '<keyword>'` first; it may be known.
   - Flaky (it passes with `-count=5` sometimes): look for wall-clock asserts, map ordering, shared DB
     state or missing `t.Parallel()` isolation. Report it; don't paper over it with retries.
   - Environment (Docker not running, port taken): say so in one line and stop.
7. **Two attempts, then report.** If two fixes didn't work, stop and report what you tried in three lines,
   rather than looping.
