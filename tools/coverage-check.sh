#!/bin/bash
set -euo pipefail

# Read coverage floors from tools/coverage-floors.txt
# Compute coverage from .cover/ and coverage.out
# Exit 1 if any floor is not met

if [[ ! -d ".cover" ]]; then
	echo "ERROR: .cover/ directory not found. Run 'make cover' first."
	exit 1
fi

if [[ ! -f "coverage.out" ]]; then
	echo "ERROR: coverage.out not found. Run 'make cover' first."
	exit 1
fi

if [[ ! -f "tools/coverage-floors.txt" ]]; then
	echo "ERROR: tools/coverage-floors.txt not found."
	exit 1
fi

# Track whether any floor failed
failed=0

# Process floors file
while IFS= read -r line; do
	# Skip comments and blank lines
	[[ "$line" =~ ^# ]] && continue
	[[ -z "$line" ]] && continue

	# Parse the floor line: <name> <floor_percent>
	read -r name floor_percent <<<"$line"

	# Compute actual coverage
	if [[ "$name" == "total" ]]; then
		# Total coverage excluding pkg/model/core
		# Parse coverage.out: each line is <file>:<start>.<col>,<end>.<col> <num_stmts> <count>
		# Count statements and covered statements, excluding pkg/model/core
		actual=$(awk '
			NR == 1 { next }  # skip mode line
			$0 !~ /pkg\/model\/core/ {
				# Split on whitespace; last field is coverage count
				stmts = $(NF-1)
				count = $NF
				total += stmts
				if (count > 0) covered += stmts
			}
			END {
				if (total > 0) {
					pct = (covered / total) * 100
					printf "%.1f", pct
				} else {
					printf "0.0"
				}
			}
		' coverage.out)
	else
		# Per-package coverage using go tool covdata percent -pkg
		# The -pkg flag must match exactly the package name
		output=$(go tool covdata percent -i=.cover -pkg="$name" 2>&1)
		if echo "$output" | grep -q "$name"; then
			# Extract the percentage from line like: "\tgithub.com/can3p/pcom/pkg/web\t\tcoverage: 89.3% of statements"
			actual=$(echo "$output" | grep "$name" | sed 's/.*coverage: \([0-9.]*\)%.*/\1/')
		else
			echo "FAIL $name -- no coverage data found for package $name"
			failed=1
			continue
		fi
	fi

	# Compare with floor using awk
	status="ok"
	if awk -v actual="$actual" -v floor="$floor_percent" 'BEGIN { exit !(actual < floor) }'; then
		status="FAIL"
		failed=1
	fi

	echo "$status $name $actual% (floor $floor_percent%)"
done < <(grep -v '^\s*#' "tools/coverage-floors.txt" | grep -v '^\s*$')

exit $failed
