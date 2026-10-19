#!/usr/bin/env bash
# scripts/ci-steps.sh — the ONE definition of the CI steps.
#
# .github/workflows/ci.yml calls this script for every step it runs, and
# `make ci-local` runs the same steps in the same order on a developer's
# machine, so the two cannot drift: a step that exists in CI exists here,
# and running `make ci-local` before a push is running CI. (The pre-commit
# checklist in CLAUDE.md used to miss CI steps — kg verify,
# lint-assertions, parser-parity, cover-gate-core, the race gates — and
# two red builds in one batch came from that gap.)
#
# Usage: scripts/ci-steps.sh <step> [arg…]
#   build-cli            build cmd/go/bin/boru
#   kg-verify            the committed knowledge graph matches the tree
#   vet | lint-assertions | lint
#   vet-copies           the no-copy ratchet: every module's copies of a
#                        core.Value against tools/vetcopies/ceilings.tsv
#   test-module D [ONLY] one module's tests (ONLY=root: its root package
#                        alone; ONLY=rest: every other package); test/go
#                        never includes the langspec corpus
#   test-modules-core    the small modules in one go (everything but
#                        lang/go, cmd/go, test/go)
#   test-modules         every module's tests except test/go/langspec
#   test-langspec N      langspec shard N of $(make langspec-shard-count)
#   test-langspec-all    every langspec shard in sequence, refreshing
#                        test/go/langspec/GATE_STATUS.md
#   gate-table F…        render the gate table from the rows the shards
#                        appended (BORU_GATE_SUMMARY files)
#   parser-parity | cover-gate-core | race-gates | wasm
#   cover-check-ci       the merged ADR-008 gate over the profiles the test
#                        jobs wrote under COVER=1 (coverage/ci/*.xout)
#   vuln                 govulncheck (advisory: never fails the run)
#   direction-gates      the direction lane as a job of its own — the
#                        local form; CI renders the same table from the
#                        shards (gate-table) and has no such job
#   all                  the whole sequence, as `make ci-local`
#
# The daily build's steps (.github/workflows/daily.yml), too long for a PR:
#   race-modules         make test-race: every RACE_MODULES package under -race
#   syntax-matrix        make spec-test with SPECGEN_FULL=1: every row of
#                        every generated syntax-matrix file
#   ts-suites            the TypeScript twins no PR runs (core, basic, the
#                        engine port) and the cross-engine differential
#   full-suite           every module's tests on this host, langspec too —
#                        plain go test, as the Windows runner has no make
set -euo pipefail
cd "$(dirname "$0")/.."

step=${1:-}
[ -n "$step" ] || { sed -n '2,45p' "$0"; exit 2; }
shift || true

run() { echo "==> [ci] $*"; "$@"; }

gate_status=test/go/langspec/GATE_STATUS.md

case "$step" in
  build-cli)        run make -C cmd/go build ;;
  # RE-ACTIVATED 2026-09-28 after the failures that had taken it off on
  # 2026-09-20 were fixed; the pipeline runs on the interpreter.
  kg-verify)        run make -C kg verify ;;
  vet)              run make vet ;;
  lint-assertions)  run make -C lang/go lint-assertions ;;
  lint)             run make lint ;;
  vet-copies)       run make vet-copies ;;
  test-module)      run make test-module M="${1:?module dir}" ONLY="${2:-}" ;;
  test-modules-core)
    for m in core/go check/go parser/go basic/go calc/go wpg test/specfix test/solardemo; do
      run make test-module M="$m"
    done ;;
  test-modules)     run make test-modules ;;
  test-langspec)    run make test-langspec SHARD="${1:?shard number}" ;;
  test-langspec-all)
    n=$(make -s langspec-shard-count)
    rm -f "$gate_status"
    for i in $(seq 1 "$n"); do BORU_GATE_SUMMARY="$PWD/$gate_status" run make test-langspec SHARD="$i"; done
    sort -o "$gate_status" "$gate_status"
    "$0" gate-table "$gate_status" ;;
  gate-table)
    # The table the direction job used to render: every gate's live value
    # against both of its numbers, from the rows every shard appended.
    # Sorted by gate name so the table (and the committed GATE_STATUS.md)
    # reads the same whichever shard, worker or test wrote a row first.
    rows=$(cat "$@" 2>/dev/null | sort -u || true)
    echo "## Langspec gates"
    echo
    echo "Every gate's live value against its END STATE (the design's number) and its REGRESSION ceiling (the last merged value, which the langspec shards assert). An **open** row is debt the design still carries, not this change's regression; a **REGRESSION** row failed its shard. This is the direction lane's table, rendered from the regression shards themselves (\`make test-direction\` is the local form of the lane; see test/go/langspec/lanes_test.go)."
    echo
    echo "| gate | live | end state | regression ceiling | status | measures |"
    echo "|---|---:|---:|---:|---|---|"
    if [ -n "$rows" ]; then echo "$rows"; else echo "| (no gate ran) | | | | | |"; fi
    echo
    echo "$(echo "$rows" | grep -c '| open' || true) open, $(echo "$rows" | grep -c '| at end state' || true) at end state, $(echo "$rows" | grep -c '| ratchet tightened' || true) tightened, $(echo "$rows" | grep -c '| REGRESSION' || true) regressions" ;;
  parser-parity)    run make parser-parity ;;
  cover-gate-core)  run make cover-gate-core ;;
  cover-check-ci)   run make cover-check-ci ;;
  race-gates)
    # `make test` does not run under -race. Two registries unifying on two
    # goroutines: the registry-armed unify once kept a package-global
    # stack (the test walks' parallel rows found it).
    ( cd core/go && run go test . -run 'TestUnifyRegistryArmedConcurrentNoRace' -race ) ;;
  wasm)             run make -C wpg wasm ;;
  vuln)            run make vuln || echo "==> [ci] vuln is advisory (see STATIC_ANALYSIS_REPORT.md); continuing" ;;
  direction-gates)  run make test-direction || echo "==> [ci] the direction lane is red by design until the debt is paid; its table is the output" ;;
  race-modules)     run make test-race ;;
  syntax-matrix)    SPECGEN_FULL=1 run make spec-test ;;
  ts-suites)
    # The packages link each other with file: dependencies that resolve
    # through dist/, so install all four, then build parser/ts — its
    # prebuild builds core/ts first. Every suite runs whatever an earlier
    # one did; the step fails if any did. BORU_CROSSDIFF_REQUIRED turns the
    # differential's skip (no TS engine) into a failure: here it must run.
    for d in core/ts parser/ts basic/ts eng/ts; do run npm ci --prefix "$d" --no-audit --no-fund; done
    ( cd parser/ts && run npm run build )
    rc=0
    for t in test-ts-core test-ts-basic test-ts; do run make "$t" || rc=1; done
    BORU_CROSSDIFF_REQUIRED=1 run make crossdiff || rc=1
    exit $rc ;;
  full-suite)
    rc=0
    for m in $(sed -n 's/^MODULES := //p' Makefile); do
      echo "==> [ci] go test $m"
      ( cd "$m" && go test -timeout 40m ./... ) || rc=1
    done
    exit $rc ;;
  all)
    for s in build-cli kg-verify vet vet-copies lint-assertions lint test-modules test-langspec-all parser-parity cover-gate-core race-gates wasm vuln; do
      "$0" "$s"
    done
    echo "==> [ci] every CI step ran" ;;
  *) echo "scripts/ci-steps.sh: unknown step '$step'" >&2; exit 2 ;;
esac
