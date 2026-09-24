#!/usr/bin/env bash
# Run a command quietly: one line on success, a trimmed failure report otherwise.
# The full output always goes to a log file, so nothing is lost, only not printed.
#
# Usage: tools/qrun.sh <label> <command...>
# Env:   QRUN_SHOW_OK=1  on success, print the "ok ..."/coverage lines instead of a count
#        QRUN_MAX=60     maximum lines of failure output to print
#        QRUN_LOG=path   log file (default: a fresh $TMPDIR/pcom-qrun-<label>.XXXXXX,
#                        so parallel runs never overwrite each other's logs)
set -u

label=$1
shift
max=${QRUN_MAX:-60}
if [ -n "${QRUN_LOG:-}" ]; then
	log=$QRUN_LOG
else
	log=$(mktemp "${TMPDIR:-/tmp}/pcom-qrun-$label.XXXXXX")
fi

"$@" >"$log" 2>&1
rc=$?

if [ "$rc" -eq 0 ]; then
	if [ "${QRUN_SHOW_OK:-}" = 1 ]; then
		grep -E '^ok |coverage:' "$log" || echo "ok: $label"
	else
		n=$(grep -c '^ok ' "$log")
		if [ "$n" -gt 0 ]; then echo "ok: $label ($n packages)"; else echo "ok: $label"; fi
	fi
	rm -f "$log"
	exit 0
fi

# Failure: drop the lines that only say "fine" or "nothing here", and goroutine
# dumps past the first (panicking) goroutine, whose stack says where it broke.
echo "FAIL: $label (exit $rc)"
filtered=$(awk '/^goroutine [0-9]+ \[/ { if (++g > 1) exit } { print }' "$log" |
	grep -Ev '^ok |^\?|no test files|^=== (RUN|PAUSE|CONT|NAME)|^--- PASS' |
	grep -Ev '^(testing\.|panic\(|created by testing)|^	.*/src/(testing|runtime)/')
total=$(printf '%s\n' "$filtered" | wc -l | tr -d ' ')
budget=$max

# Too long to print whole: put the verdict lines first (failing tests and
# packages, panics, compile errors) so they cannot fall outside the budget.
if [ "$total" -gt "$max" ]; then
	verdict=$(printf '%s\n' "$filtered" | grep -E '^--- FAIL|^FAIL|^panic:|^# |\.go:[0-9]+:[0-9]+: ' | head -n 20)
	printf '%s\n--- first lines\n' "$verdict"
	budget=$((max - $(printf '%s\n' "$verdict" | wc -l) - 1))
fi
printf '%s\n' "$filtered" | head -n "$budget"
if [ "$total" -gt "$budget" ]; then
	echo "... $((total - budget)) more lines; full log: $log"
else
	echo "(full log: $log)"
fi
exit "$rc"
