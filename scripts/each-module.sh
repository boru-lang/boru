#!/usr/bin/env bash
# scripts/each-module.sh — run one command in every Go module, in parallel.
#
#   scripts/each-module.sh [-j N] [-m "dir dir …"] <command…>
#
# The command runs with the module directory as its working directory;
# each module's output is captured and printed under a "==> <label> dir"
# header when it finishes, so a parallel run reads like a sequential one.
# The exit status is non-zero when any module's command failed, and every
# module still runs (a lint failure in lang/go must not hide one in
# cmd/go). -j defaults to the CPU count; -j 1 is the sequential form. -m
# names the modules (default: every module the root Makefile lists).
#
# make vet, make lint and make commit-gate run through it: golangci-lint
# over the thirteen modules took 127 s in sequence on CI and under 10 s
# with a warm cache, four at a time.
set -uo pipefail
cd "$(dirname "$0")/.."

jobs=$(nproc 2>/dev/null || echo 4)
modules=""
while [ $# -gt 0 ]; do
  case "$1" in
    -j) jobs=$2; shift 2 ;;
    -m) modules=$2; shift 2 ;;
    --) shift; break ;;
    *) break ;;
  esac
done
[ $# -gt 0 ] || { echo "usage: scripts/each-module.sh [-j N] [-m \"dirs\"] <command…>" >&2; exit 2; }
if [ -z "$modules" ]; then
  modules=$(sed -n 's/^MODULES := //p' Makefile)
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

run_one() {
  local dir=$1; shift
  local out="$tmp/$(echo "$dir" | tr / _).out"
  ( cd "$dir" && "$@" ) > "$out" 2>&1
  local rc=$?
  {
    echo "==> $* ($dir)"
    cat "$out"
  } >> "$tmp/log.$(echo "$dir" | tr / _)"
  cat "$tmp/log.$(echo "$dir" | tr / _)"
  return $rc
}
export -f run_one
export tmp

# A failing module exits 1, not 255, so xargs still runs every other
# module (it reports 123 at the end); the FAILED line names the module
# and its name is recorded for the retry below.
printf '%s\n' $modules | xargs -P "$jobs" -I{} bash -c 'run_one "$@" || { echo "==> FAILED: ${*:2} ($1)"; echo "$1" >> "$tmp/failed"; exit 1; }' _ {} "$@"
[ -f "$tmp/failed" ] || exit 0

# A module that failed in the parallel pass runs ONCE more on its own
# before the verdict: N tool processes over one workspace share the Go
# build cache and the tool's own cache, and a first pass after a wide
# source change has been seen to fail transiently for one module and
# pass alone a moment later. A failure that repeats alone is the real
# one and its output is above; a pass on the retry is reported as such.
rc=0
for dir in $(sort -u "$tmp/failed"); do
  echo "==> retrying alone: $* ($dir)"
  if run_one "$dir" "$@"; then
    echo "==> passed alone: $* ($dir) — the parallel failure was transient"
  else
    echo "==> FAILED again alone: $* ($dir)"
    rc=1
  fi
done
[ $rc -eq 0 ] || { echo "scripts/each-module.sh: a module failed (see the ==> FAILED lines above)" >&2; exit 1; }
