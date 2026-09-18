#!/usr/bin/env bash
# scripts/ci-steps.sh — the ONE definition of the CI steps.
#
# .github/workflows/ci.yml calls this script for every step it runs, and
# `make ci-local` runs the same steps in the same order on a developer's
# machine, so the two cannot drift: a step that exists in CI exists here,
# and running `make ci-local` before a push is running CI. (The pre-commit
# checklist in CLAUDE.md used to miss six CI steps — kg verify,
# lint-assertions, parser-parity, cover-gate-core, the race gates, the
# borudebug gates — and two red builds in one batch came from that gap.)
#
# Usage: scripts/ci-steps.sh <step> [arg…]
#   build-cli            build cmd/go/bin/boru
#   kg-verify            the committed knowledge graph matches the tree
#   vet | lint-assertions | lint
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
#   parser-parity | cover-gate-core | race-gates | borudebug-gates | wasm
#   vuln                 govulncheck (advisory: never fails the run)
#   direction-gates      the direction lane as a job of its own — the
#                        local form; CI renders the same table from the
#                        shards (gate-table) and has no such job
#   all                  the whole sequence, as `make ci-local`
set -euo pipefail
cd "$(dirname "$0")/.."

step=${1:-}
[ -n "$step" ] || { sed -n '2,31p' "$0"; exit 2; }
shift || true

run() { echo "==> [ci] $*"; "$@"; }

gate_status=test/go/langspec/GATE_STATUS.md

case "$step" in
  build-cli)        run make -C cmd/go build ;;
  kg-verify)        run make -C kg verify ;;
  vet)              run make vet ;;
  lint-assertions)  run make -C lang/go lint-assertions ;;
  lint)             run make lint ;;
  test-module)      run make test-module M="${1:?module dir}" ONLY="${2:-}" ;;
  test-modules-core)
    for m in core/go check/go compiler/go parser/go eng/go basic/go calc/go wpg test/specfix test/solardemo; do
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
    echo "## Full-compilation gates"
    echo
    echo "Every gate's live value against its END STATE (the design's number) and its REGRESSION ceiling (the last merged value, which the langspec shards assert). An **open** row is the programme's debt, not this change's regression; a **REGRESSION** row failed its shard. This is the direction lane's table, rendered from the regression shards themselves (\`make test-direction\` is the local form of the lane; see test/go/langspec/lanes_test.go)."
    echo
    echo "| gate | live | end state | regression ceiling | status | measures |"
    echo "|---|---:|---:|---:|---|---|"
    if [ -n "$rows" ]; then echo "$rows"; else echo "| (no gate ran) | | | | | |"; fi
    echo
    echo "$(echo "$rows" | grep -c '| open' || true) open, $(echo "$rows" | grep -c '| at end state' || true) at end state, $(echo "$rows" | grep -c '| ratchet tightened' || true) tightened, $(echo "$rows" | grep -c '| REGRESSION' || true) regressions" ;;
  parser-parity)    run make parser-parity ;;
  cover-gate-core)  run make cover-gate-core ;;
  race-gates)
    # A shared immutable Program driven across forks, the island sub-engine
    # reuse, and the concurrent spec rows in compiled mode must be
    # data-race-free; `make test` does not run under -race.
    # Two registries unifying on two goroutines: the registry-armed unify
    # once kept a package-global stack (the test walks' parallel rows
    # found it).
    ( cd core/go && run go test . -run 'TestUnifyRegistryArmedConcurrentNoRace' -race )
    ( cd lang/go && run go test . -run 'TestCompiledConcurrencyRaceFree|TestCompiledIslandReuseNoStateLeak' -race )
    ( cd test/go && run go test ./langspec/ -run 'TestSpecCompiledConcurrentRowsRaceFree' -race ) ;;
  borudebug-gates)
    # -tags borudebug allocates the VM's args buffer per call so a native
    # that retains its args slice diverges cleanly instead of corrupting a
    # later dispatch.
    ( cd lang/go && run go test -tags borudebug . -run 'Bytecode|Compiled|Emit' )
    ( cd test/go && run go test -tags borudebug ./langspec/ -run 'TestSpecCompiledDifferential|TestSpecCompiledOrFallback|TestCompiledCombinationParity' ) ;;
  wasm)             run make -C wpg wasm ;;
  vuln)             run make vuln || echo "==> [ci] vuln is advisory (see STATIC_ANALYSIS_REPORT.md); continuing" ;;
  direction-gates)  run make test-direction || echo "==> [ci] the direction lane is red by design until the debt is paid; its table is the output" ;;
  all)
    for s in build-cli kg-verify vet lint-assertions lint test-modules test-langspec-all parser-parity cover-gate-core race-gates borudebug-gates wasm vuln; do
      "$0" "$s"
    done
    echo "==> [ci] every CI step ran" ;;
  *) echo "scripts/ci-steps.sh: unknown step '$step'" >&2; exit 2 ;;
esac
