#!/usr/bin/env bash
# CPU-profile share of the collection loops — the instrument Stage 2's gate
# was re-specified onto (design/FULL-COMPILATION.0.md §11, falsifier F1b).
#
# Why share and not wall clock: F1b was TESTED on this class of host and the
# measurement established that wall clock cannot resolve it — two runs of
# byte-identical code differed by 5.4% geomean. Profile SHARE is comparative
# within a single run, so the machine's noise divides out of it. This records
# the share; like everything else in the register it never gates.
#
# It profiles the DISPATCH-DENSE interpreter benchmarks (BenchmarkParens and
# BenchmarkInterpBaseline, both of which drive `RunInterp`), not the
# collection-WORD suite: in BenchmarkPerfWords the 500-element inner work
# swamps dispatch and the collection loops sit near 3-5% of samples, where a
# regression would hide inside the run-to-run spread. In the dispatch-dense
# set the same loops are ~15% and ~9% of samples — about four times the
# resolution for the same machine noise.
#
# Two bases are recorded per anchor, because they answer different questions:
#
#   cum_pct_total  — share of ALL samples. Moves when anything outside the
#                    interpreter moves too (GC, parse), so it is the honest
#                    absolute but a noisy comparator.
#   cum_pct_interp — share of Engine.Run's cumulative time. Divides out any
#                    uniform shift in how much of the process is interpreter
#                    at all, which is what makes it the comparator F1b wants:
#                    "did the collection loops get more expensive RELATIVE TO
#                    the interpreter they live in".
#
# Anchors are the three functions present on BOTH sides of a Stage-2-style
# comparison — the seam functions themselves (collectForward,
# collectCandidateScan, collectArrival) exist only after the extraction, so
# they cannot be compared to anything. They are called BY these anchors, so
# whatever they cost shows up here. Cum% does not sum across a call chain;
# read each anchor on its own.
#
# Usage: bench/register/cpushare.sh [benchtime] [repeats]
set -u
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
benchtime="${1:-3s}"
repeats="${2:-3}"
bench="${CPUSHARE_BENCH:-BenchmarkInterpBaseline|BenchmarkParens}"
hosts="$here/hosts.jsonl"
meas="$here/measurements.jsonl"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

commit=$(cd "$root" && git rev-parse --short HEAD 2>/dev/null || echo unknown)
# A row names a commit, so it has to say when the tree it measured was not
# that commit. Only TRACKED modifications count — this script is untracked
# the first time it runs, and that says nothing about what was measured.
dirty=""
if ! (cd "$root" && git diff --quiet HEAD -- 2>/dev/null); then
  dirty=";dirty"
  echo "cpushare.sh: WARNING — tracked files differ from $commit;" \
       "rows will be flagged dirty" >&2
fi
ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
gover=$(go version 2>/dev/null | awk '{print $3}')
osver=$(uname -r)

host_row=$(BORU="${BORU:-$root/cmd/go/bin/boru}" "$here/hostid.sh") || exit 1
host=$(printf '%s' "$host_row" | sed -n 's/^{"host":"\([^"]*\)".*/\1/p')
[ -n "$host" ] || { echo "cpushare.sh: could not derive host id" >&2; exit 1; }
touch "$hosts" "$meas"
grep -q "\"host\":\"$host\"" "$hosts" || printf '%s\n' "$host_row" >> "$hosts"

echo "==> building the interpreter benchmark binary"
( cd "$root/lang/go" && go test -c -o "$work/lang.test" . ) || exit 1

# One profile per repeat; the anchors' cum% is read out of each.
for r in $(seq 1 "$repeats"); do
  echo "==> profile $r/$repeats (benchtime $benchtime)"
  ( cd "$root/lang/go" && "$work/lang.test" -test.run '^XXX$' \
      -test.bench "$bench" -test.benchtime "$benchtime" \
      -test.cpuprofile "$work/cpu.$r.out" ) >/dev/null 2>&1 || {
        echo "cpushare.sh: benchmark run $r failed" >&2; exit 1; }
  go tool pprof -top -cum -nodecount=800 "$work/lang.test" "$work/cpu.$r.out" \
    2>/dev/null > "$work/top.$r.txt"
done

# emit <surface> <workload> <metric> <value> <unit> <n> <spread>
emit() {
  printf '{"ts":"%s","commit":"%s","host":"%s","surface":"%s","workload":"%s","metric":"%s","value":%s,"unit":"%s","n":%s,"spread":%s,"benchtime":"%s","go":"%s","os_version":"%s","flags":"cpuprofile;bench=%s"}\n' \
    "$ts" "$commit" "$host" "$1" "$2" "$3" "$4" "$5" "$6" "$7" "$benchtime" "$gover" "$osver" "$bench$dirty" >> "$meas"
}

# Mean and spread of one anchor's cum% across the repeats, on either basis.
# One profile file at a time so the anchor and its Engine.Run denominator are
# always read from the SAME run — pairing them across runs would divide a
# quiet run's numerator by a busy run's denominator.
cum_pct() {  # <top-file> <extended-regexp>
  grep -E "$2" "$1" | head -1 | awk '{v=$5; gsub(/%/,"",v); print v+0}'
}

summarise() {  # <extended-regexp> <total|interp> -> "mean spread n"
  local pattern="$1" basis="$2" f v run
  local -a xs=()
  for f in "$work"/top.*.txt; do
    v=$(cum_pct "$f" "$pattern")
    [ -n "$v" ] && [ "$v" != 0 ] || continue
    if [ "$basis" = interp ]; then
      run=$(cum_pct "$f" '\(\*Engine\)\.Run$')
      [ -n "$run" ] && [ "$run" != 0 ] || continue
      v=$(awk -v a="$v" -v b="$run" 'BEGIN{printf "%.3f", 100*a/b}')
    fi
    xs+=("$v")
  done
  [ "${#xs[@]}" -gt 0 ] || return 1
  printf '%s\n' "${xs[@]}" | awk '
    {n++; s+=$1; if(n==1||$1<lo)lo=$1; if(n==1||$1>hi)hi=$1}
    END {printf "%.3f %.3f %d\n", s/n, hi-lo, n}'
}

echo "==> anchors (n=$repeats, benchtime=$benchtime, commit=$commit)"
printf '%-22s %10s %10s\n' anchor "%total" "%interp"
for spec in \
  'Run|\(\*Engine\)\.Run$' \
  'stepWord|\(\*Engine\)\.stepWord$' \
  'MatchSignature|\(\*Engine\)\.MatchSignature$' \
  'resolveForwardArgs|resolveForwardArgs( \(inline\))?$' \
  'stepLiteral|\(\*Engine\)\.stepLiteral$' ; do
  anchor="${spec%%|*}"; pattern="${spec#*|}"
  read -r mt st nt <<< "$(summarise "$pattern" total)" || true
  [ -n "${mt:-}" ] || { echo "cpushare.sh: anchor $anchor not found in profile" >&2; exit 1; }
  emit interp "cpushare/$anchor" cum_pct_total "$mt" pct "$nt" "$st"
  mi=""
  if [ "$anchor" != Run ]; then
    read -r mi si ni <<< "$(summarise "$pattern" interp)" || true
    [ -n "${mi:-}" ] || { echo "cpushare.sh: anchor $anchor has no interp basis" >&2; exit 1; }
    emit interp "cpushare/$anchor" cum_pct_interp "$mi" pct "$ni" "$si"
  fi
  printf '%-22s %9s%% %9s\n' "$anchor" "$mt" "${mi:+$mi%}"
done

echo "register: appended cpushare rows to $meas (host $host, commit $commit)"
