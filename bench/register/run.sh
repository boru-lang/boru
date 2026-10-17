#!/usr/bin/env bash
# The performance register harness (bench/register/README.md).
#
# Runs the benchmark suites, derives the host id, and APPENDS rows to
# bench/register/measurements.jsonl (and a hosts.jsonl row the first time
# it sees a host). It RECORDS; it never gates — execution time is too
# noisy to fail CI on, so the deterministic alloc ceilings in `make test`
# remain the only performance GATES and this is the memory.
#
# Rows are never edited or deleted. A measurement discovered to be wrong
# is superseded by a NEW row naming it, the same discipline the frontier
# ledger uses, so the series stays readable years later.
#
# Absolute values compare only WITHIN one host id. Across hosts only
# ratios travel (check-cost/exec-cost, before/after a change), which is
# why rows carry absolutes and reports derive ratios.
#
# Usage: BORU=cmd/go/bin/boru bench/register/run.sh [benchtime]
set -u
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
BORU="${BORU:-$root/cmd/go/bin/boru}"
benchtime="${1:-1s}"
hosts="$here/hosts.jsonl"
meas="$here/measurements.jsonl"

commit=$(cd "$root" && git rev-parse --short HEAD 2>/dev/null || echo unknown)
ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
gover=$(go version 2>/dev/null | awk '{print $3}')
osver=$(uname -r)

host_row=$(BORU="$BORU" "$here/hostid.sh") || exit 1
host=$(printf '%s' "$host_row" | sed -n 's/^{"host":"\([^"]*\)".*/\1/p')
[ -n "$host" ] || { echo "run.sh: could not derive host id" >&2; exit 1; }

touch "$hosts" "$meas"
grep -q "\"host\":\"$host\"" "$hosts" || printf '%s\n' "$host_row" >> "$hosts"

# emit surface workload metric value unit n
emit() {
  printf '{"ts":"%s","commit":"%s","host":"%s","surface":"%s","workload":"%s","metric":"%s","value":%s,"unit":"%s","n":%s,"benchtime":"%s","go":"%s","os_version":"%s"}\n' \
    "$ts" "$commit" "$host" "$1" "$2" "$3" "$4" "$5" "$6" "$benchtime" "$gover" "$osver" >> "$meas"
}

# Parse `go test -bench` output into rows. Each Benchmark line reads
#   BenchmarkName-8   <iters>   <ns/op> ns/op   <B/op> B/op   <allocs/op> allocs/op
capture() {
  local surface="$1" dir="$2" pattern="$3"
  echo "==> $surface: $pattern in $dir"
  ( cd "$root/$dir" && go test -run '^$' -bench "$pattern" -benchmem -benchtime "$benchtime" . 2>/dev/null ) |
  while read -r name iters nsop _ bop _ allocs _; do
    case "$name" in Benchmark*) ;; *) continue ;; esac
    local w="${name%-*}"
    emit "$surface" "$w" "ns_per_op" "$nsop" "ns" "$iters"
    [ -n "${bop:-}" ] && emit "$surface" "$w" "bytes_per_op" "$bop" "B" "$iters"
    [ -n "${allocs:-}" ] && emit "$surface" "$w" "allocs_per_op" "$allocs" "count" "$iters"
  done
}

capture check   lang/go   'BenchmarkPerfCheck'
capture interp  lang/go   'BenchmarkInterpBaseline'
capture interp  core/go   'BenchmarkKernel|BenchmarkTape|BenchmarkFlatProgramRecording'
capture parse   parser/go 'BenchmarkParse'

echo "register: appended to $meas (host $host, commit $commit)"
