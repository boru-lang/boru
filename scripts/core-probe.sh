#!/usr/bin/env bash
# core-probe — run candidate core/spec expressions through BOTH engines and
# report whether they agree, in the column format core/spec's files want.
#
# Usage:  scripts/core-probe.sh <file-of-expressions>
#
# One expression per line, in the corpus's own notation (`run addq 1 2`,
# `list 1 2`, `int 42`, …). Output per candidate:
#
#   AGREE  <expr>\t<render>          -> a candidate row
#   DIFFER <expr>\t<go>\t<ts>        -> a real divergence; adjudicate it
#
# AGREE is NOT a licence to paste. core/spec is a SPEC, not a differential:
# the expected column is the documented contract, and a row where both
# engines agree on the WRONG answer is exactly the defect class this corpus
# exists to catch. The probe tells you what the engines do; REFERENCE.md
# tells you what they should do.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src_file="${1:?usage: core-probe.sh <file-of-expressions>}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# A line the notation cannot read fails the probe's run but not its output:
# that line answers NOTATION and the rest are evaluated. Only a run that
# wrote nothing is a failure of the probe itself.
( cd "$repo/core/go" && CORE_PROBE_IN="$src_file" CORE_PROBE_OUT="$tmp/go.txt" \
    go test -run TestCoreProbe -count=1 >/dev/null 2>&1 ) || [ -s "$tmp/go.txt" ] || {
  echo "go probe failed (is TestCoreProbe present?)" >&2; exit 1; }

( cd "$repo/core/ts" && node --experimental-strip-types --no-warnings \
    "$repo/scripts/core-probe.ts" "$src_file" > "$tmp/ts.txt" ) || {
  echo "ts probe failed" >&2; exit 1; }

# awk, not `read`: a tab is IFS WHITESPACE to the shell, so `read` folds two
# adjacent tabs into one and an EMPTY answer shifts the columns — a row where
# one engine rendered nothing printed the other's answer in its place, and
# could read as AGREE.
paste -d'\t' "$src_file" "$tmp/go.txt" "$tmp/ts.txt" |
  awk -F'\t' '{
    if ($2 == $3) printf "AGREE\t%s\t%s\n", $1, $2
    else          printf "DIFFER\t%s\t%s\t%s\n", $1, $2, $3
  }'
