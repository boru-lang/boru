#!/usr/bin/env bash
# parity-probe — run candidate sources through BOTH parsers and report
# whether they agree, in the exact column format parser/spec/parse.tsv wants.
#
# Usage:  scripts/parity-probe.sh <file-of-sources>
#
# One source per line, using the corpus's own escapes (\n, \t, \\) so a
# multi-line source stays one line. Output per candidate:
#
#   AGREE  <src>\t<render>          -> paste into parse.tsv
#   DIFFER <src>\t<go>\t<ts>        -> a real divergence; adjudicate it
#
# The point is that a row is only ever added to the corpus after both
# engines have been asked, rather than by writing down what one of them
# happens to do.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src_file="${1:?usage: parity-probe.sh <file-of-sources>}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

( cd "$repo/parser/go" && PARITY_PROBE_IN="$src_file" PARITY_PROBE_OUT="$tmp/go.txt" \
    go test -run TestParityProbe -count=1 >/dev/null 2>&1 ) || {
  echo "go probe failed (is TestParityProbe present?)" >&2; exit 1; }

( cd "$repo/parser/ts" && node --experimental-strip-types --no-warnings \
    "$repo/scripts/parity-probe.ts" "$src_file" > "$tmp/ts.txt" ) || {
  echo "ts probe failed" >&2; exit 1; }

paste -d'\t' "$src_file" "$tmp/go.txt" "$tmp/ts.txt" |
  while IFS=$'\t' read -r src go ts; do
    if [ "$go" = "$ts" ]; then
      printf 'AGREE\t%s\t%s\n' "$src" "$go"
    else
      printf 'DIFFER\t%s\t%s\t%s\n' "$src" "$go" "$ts"
    fi
  done
