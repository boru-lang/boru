#!/bin/bash
# Usage (from a library root): interp-count.sh test/<suite>.aql [more files...]
# Runs each file on the instrumented boru build (boru main @ 64c5ab2 + trace prints) and
# reports run-time interpreter use from compiled code: INTERP = unattributed, non-check
# interpreter entries (an island counts twice: its vm:island* seam + the Engine.Run it does);
# ISLAND = fallback islands, with source row:col and first tokens. Program stdout is discarded.
T=${T:-/path/to/boru-trace}   # the binary built from interp-trace-build.patch
for f in "$@"; do
  out=$(BORU_INTERP_TRACE=1 timeout 1800 "$T" "$f" 2>&1 >/dev/null)
  rc=$?
  i=$(printf '%s\n' "$out" | grep -c '^INTERP')
  s=$(printf '%s\n' "$out" | grep -c '^ISLAND')
  echo "== $f exit=$rc INTERP=$i ISLAND=$s"
  printf '%s\n' "$out" | awk -F'\t' '$1=="INTERP"{c[$2"\t"$3]++} END{for(k in c) print "   "c[k]"\t"k}' | sort -t$'\t' -k1,1nr | head -6 | cut -c1-260
  printf '%s\n' "$out" | awk -F'\t' '$1=="ISLAND"{c[$2"\t"$3"\t"substr($4,1,120)]++} END{for(k in c) print "   ISLAND "c[k]"\t"k}' | sort -t$'\t' -k1,1nr | head -6
  printf '%s\n' "$out" | grep -v '^INTERP\|^ISLAND' | head -5 | sed 's/^/   stderr: /'
done
