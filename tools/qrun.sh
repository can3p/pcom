#!/usr/bin/env bash
# Run a command quietly: one line on success, a trimmed failure report otherwise.
# The full output always goes to a log file, so nothing is lost, only not printed.
#
# Usage: tools/qrun.sh <label> <command...>
# Env:   QRUN_SHOW_OK=1  on success, print the "ok ..."/coverage lines instead of a count
#        QRUN_MAX=60     maximum lines of failure output to print
#        QRUN_LOG=path   log file (default: $TMPDIR/pcom-qrun-<label>.log)
set -u

label=$1
shift
log=${QRUN_LOG:-${TMPDIR:-/tmp}/pcom-qrun-$label.log}
max=${QRUN_MAX:-60}

"$@" >"$log" 2>&1
rc=$?

if [ "$rc" -eq 0 ]; then
	if [ "${QRUN_SHOW_OK:-}" = 1 ]; then
		grep -E '^ok |coverage:' "$log" || echo "ok: $label"
	else
		n=$(grep -c '^ok ' "$log")
		if [ "$n" -gt 0 ]; then echo "ok: $label ($n packages)"; else echo "ok: $label"; fi
	fi
	exit 0
fi

# Failure: drop the lines that only say "fine" or "nothing here", keep the rest, cap it.
filtered=$(grep -Ev '^ok |^\?|no test files|^=== (RUN|PAUSE|CONT|NAME)' "$log")
total=$(printf '%s\n' "$filtered" | wc -l | tr -d ' ')
echo "FAIL: $label (exit $rc)"
printf '%s\n' "$filtered" | head -n "$max"
if [ "$total" -gt "$max" ]; then
	echo "... $((total - max)) more lines; full log: $log"
else
	echo "(full log: $log)"
fi
exit "$rc"
