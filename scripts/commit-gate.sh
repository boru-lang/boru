#!/usr/bin/env bash
# scripts/commit-gate.sh — the pre-commit gate: three minutes or less.
#
# `make commit-gate`. What runs is decided by what the change touched, so
# the gate stays under three minutes on a four-core machine while CI
# (.github/workflows/ci.yml, the same ceiling per job) runs everything in
# parallel and `make ci-local` runs everything in sequence:
#
#   always              gofmt on the changed Go files; vet and golangci-lint
#                       on the touched modules, in parallel
#                       (scripts/each-module.sh)
#   lang/go touched     the assertion lint
#   a Go module touched its unit tests — every package of a small module;
#                       for lang/go and cmd/go the changed packages only
#                       (their full suites are CI's, and lang/go's root
#                       package alone is 75 s)
#   an engine module,   the langspec gates over the SMOKE corpus: a fixed
#   test/go/langspec,   sample of spec files spanning the families, plus
#   test/specfix or a   every spec file the change touched, under
#   spec file touched   BORU_SPEC_FILES — every per-row verdict (a
#                       divergence, an island, an interpreter entry, a
#                       compile failure) asserts, and so does the per-file
#                       compile-failure ledger for every file in the run
#                       (test/go/langspec/compile_failures.tsv); the
#                       corpus-wide counts report
#   docs, Makefile,     the knowledge graph against the tree
#   scripts, kg, CI     (make -C kg verify, which needs the CLI built)
#   touched
#
# "Touched" is the working tree against HEAD (staged, unstaged and
# untracked files); on a clean tree, HEAD against its merge base with
# origin/main, so the gate also answers "is this branch ready to push".
# COMMIT_GATE_ALL=1 runs every lane whatever changed. Each lane prints its
# time; the total is checked against the ceiling and a breach is a
# warning, not a failure — the ceiling is the gate's own contract to keep,
# and the fix is in this script or in the tests it runs, never in
# skipping a lane.
set -uo pipefail
cd "$(dirname "$0")/.."

ceiling=${COMMIT_GATE_CEILING:-180}
engine_modules="core/go check/go compiler/go parser/go eng/go basic/go lang/go"
# The smoke corpus: about a tenth of the rows, one file per family that
# has bitten — callbacks and fn values, code bodies, each/fold, modules,
# user types, dispatch, generics, control, and the core corpus.
smoke_files="callbacks.tsv,code-bodies.tsv,each-variants.tsv,module-composition.tsv,fn-value.tsv,higher-order.tsv,control.tsv,user-types.tsv,edge-dispatch-1.tsv,generics.tsv,corpus-core.tsv"
# Corpus tests that do not walk the spec files (they generate, fuzz or
# transform their own programs, or read real programs) and are slow: the
# smoke skips them, the shards run them.
smoke_skip='TestPropertyDifferential|TestVariationDifferential|TestRealProgramsCompile|TestCheckRunFalsePositive'

modules=$(sed -n 's/^MODULES := //p' Makefile)
t0=$(date +%s)
failed=0
lanes=()

lane() { # lane <name> <command…> — runs it, records the time and the verdict
  local name=$1; shift
  local s=$(date +%s)
  echo "==> [commit-gate] $name"
  if "$@"; then
    lanes+=("$name: ok in $(( $(date +%s) - s ))s")
  else
    failed=1
    lanes+=("$name: FAILED in $(( $(date +%s) - s ))s")
  fi
}

# ---- what changed ---------------------------------------------------------
changed=$( { git diff --name-only HEAD -- 2>/dev/null; git ls-files --others --exclude-standard; } | sort -u )
scope="the working tree against HEAD"
if [ -z "$changed" ]; then
  base=$(git merge-base HEAD origin/main 2>/dev/null || git merge-base HEAD main 2>/dev/null || true)
  if [ -n "$base" ]; then
    changed=$(git diff --name-only "$base" HEAD)
    scope="HEAD against its merge base with main ($(git rev-parse --short "$base"))"
  fi
fi
if [ "${COMMIT_GATE_ALL:-}" = "1" ]; then
  changed=$(git ls-files)
  scope="everything (COMMIT_GATE_ALL=1)"
fi
echo "==> [commit-gate] scope: $scope — $(echo "$changed" | grep -c . || true) files"

touched_modules=""
for m in $modules; do
  if echo "$changed" | grep -q "^$m/"; then touched_modules="$touched_modules $m"; fi
done
go_files=$(echo "$changed" | grep '\.go$' || true)
spec_touched=$(echo "$changed" | sed -n 's#^lang/spec/\([^/]*\.tsv\)$#\1#p' | paste -sd, - || true)
smoke=0
for m in $engine_modules test/specfix; do
  case " $touched_modules " in *" $m "*) smoke=1 ;; esac
done
if echo "$changed" | grep -q '^test/go/langspec/\|^lang/spec/'; then smoke=1; fi
docs=0
if echo "$changed" | grep -q '\.md$\|^Makefile$\|^kg/\|^scripts/\|^\.github/\|^go\.work'; then docs=1; fi

# ---- fmt, vet, lint ---------------------------------------------------------
if [ -n "$go_files" ]; then
  fmt_check() {
    local bad
    bad=$(echo "$go_files" | while read -r f; do [ -f "$f" ] && gofmt -l "$f"; done)
    [ -z "$bad" ] || { echo "not gofmt-clean (run make fmt):"; echo "$bad"; return 1; }
  }
  lane "gofmt on $(echo "$go_files" | grep -c .) changed Go files" fmt_check
fi
if [ -n "$touched_modules" ]; then
  lane "vet ($touched_modules )" scripts/each-module.sh -m "$touched_modules" go vet ./...
  if command -v golangci-lint > /dev/null; then
    lane "golangci-lint ($touched_modules )" scripts/each-module.sh -m "$touched_modules" golangci-lint run ./...
  else
    failed=1; lanes+=("golangci-lint: MISSING — go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0")
  fi
  case " $touched_modules " in *" lang/go "*) lane "lint-assertions" make -s -C lang/go lint-assertions ;; esac
fi

# ---- unit tests of what changed, and the smoke corpus, side by side ----------
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

unit_tests() {
  local rc=0
  for m in $touched_modules; do
    local pkgs="./..."
    case "$m" in
      lang/go|cmd/go)
        if ! echo "$changed" | grep -q "^$m/go\.mod$"; then
          pkgs=$(echo "$changed" | grep "^$m/.*\.go$" | xargs -r -n1 dirname | sort -u | sed "s#^$m#.#" | paste -sd' ' -)
          [ -n "$pkgs" ] || continue
        fi ;;
    esac
    echo "==> [commit-gate] go test $m: $pkgs"
    if [ "$m" = "test/go" ]; then
      ( cd "$m" && go test -timeout 10m $(go list ./... | grep -v '/langspec$') ) || rc=1
    else
      ( cd "$m" && go test -timeout 10m $pkgs ) || rc=1
    fi
  done
  return $rc
}

smoke_run() {
  local files=$smoke_files
  [ -z "$spec_touched" ] || files="$files,$spec_touched"
  echo "==> [commit-gate] langspec smoke over $files (skipping $smoke_skip)"
  ( cd test/go && BORU_SPEC_FILES="$files" go test -timeout 10m ./langspec/ -skip "$smoke_skip" )
}

if [ -n "$touched_modules" ]; then
  ( unit_tests > "$tmp/unit.log" 2>&1; echo $? > "$tmp/unit.rc" ) &
fi
if [ "$smoke" = 1 ]; then
  ( smoke_run > "$tmp/smoke.log" 2>&1; echo $? > "$tmp/smoke.rc" ) &
fi
s=$(date +%s)
wait
if [ -f "$tmp/unit.rc" ]; then
  cat "$tmp/unit.log"
  if [ "$(cat "$tmp/unit.rc")" = 0 ]; then lanes+=("unit tests: ok in $(( $(date +%s) - s ))s"); else failed=1; lanes+=("unit tests: FAILED in $(( $(date +%s) - s ))s"); fi
fi
if [ -f "$tmp/smoke.rc" ]; then
  cat "$tmp/smoke.log"
  if [ "$(cat "$tmp/smoke.rc")" = 0 ]; then lanes+=("langspec smoke: ok in $(( $(date +%s) - s ))s"); else failed=1; lanes+=("langspec smoke: FAILED in $(( $(date +%s) - s ))s"); fi
fi

# ---- the knowledge graph ------------------------------------------------------
# DEACTIVATED 2026-09-20 with the CI step of the same name: the kg generator
# cannot run (the generic lane's evaluating host — scripts/ci-steps.sh carries
# the full reason and the re-activation instruction), so a doc edit could fail
# this lane with no way to fix it. Re-activate here and there together.
if false && [ "$docs" = 1 ]; then
  kg_verify() { make -s -C cmd/go build && make -s -C kg verify; }
  lane "kg verify" kg_verify
fi

# ---- the verdict ---------------------------------------------------------------
total=$(( $(date +%s) - t0 ))
echo
echo "==> [commit-gate] lanes:"
for l in "${lanes[@]:-}"; do [ -n "$l" ] && echo "    $l"; done
if [ $failed -ne 0 ]; then
  echo "==> [commit-gate] FAILED in ${total}s"
  exit 1
fi
if [ "$total" -gt "$ceiling" ]; then
  echo "==> [commit-gate] passed in ${total}s — OVER the ${ceiling}s ceiling: the gate's contract is three minutes; find what grew (the lane times above) and fix it there, not by skipping a lane"
else
  echo "==> [commit-gate] passed in ${total}s (ceiling ${ceiling}s)"
fi
