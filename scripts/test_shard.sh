#!/usr/bin/env bash
# Splits the top-level tests of one package into shards for CI.
#
#   scripts/test_shard.sh PKG SHARD SHARDS   prints a -run pattern for shard SHARD (1..SHARDS)
#   scripts/test_shard.sh PKG list           prints every top-level test name, one per line
#
# Tests are read from the package's own test files (the testtools tag set), sorted by
# name and dealt out round-robin, so every test lands in exactly one shard and one
# family of tests (TestSettings*, TestMemberReq*) is spread over all of them.
set -euo pipefail

pkg=${1:?package}
mode=${2:?shard number or "list"}

dir=$(go list -tags testtools -f '{{.Dir}}' "$pkg")
files=$(go list -tags testtools -f '{{range .TestGoFiles}}{{$.Dir}}/{{.}} {{end}}{{range .XTestGoFiles}}{{$.Dir}}/{{.}} {{end}}' "$pkg")
[ -n "$files" ] || { echo "no test files in $dir" >&2; exit 1; }

# shellcheck disable=SC2086 # the file list is space-separated paths without spaces
names=$(grep -hoE '^func Test[A-Za-z0-9_]*\([A-Za-z0-9_]+ \*testing\.T\)' $files \
	| sed -E 's/^func (Test[A-Za-z0-9_]*)\(.*/\1/' | LC_ALL=C sort -u)

if [ "$mode" = list ]; then
	printf '%s\n' "$names"
	exit 0
fi

shards=${3:?number of shards}
if ! [[ "$mode" =~ ^[0-9]+$ && "$shards" =~ ^[0-9]+$ ]] || [ "$mode" -lt 1 ] || [ "$mode" -gt "$shards" ]; then
	echo "shard must be 1..$shards" >&2
	exit 1
fi

picked=$(printf '%s\n' "$names" | awk -v s="$mode" -v n="$shards" '(NR - 1) % n == s - 1' | paste -sd'|')
[ -n "$picked" ] || { echo "shard $mode of $shards is empty" >&2; exit 1; }
printf '^(%s)$\n' "$picked"
