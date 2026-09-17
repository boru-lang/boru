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
# Usage: scripts/ci-steps.sh <step> [arg]
#   build-cli          build cmd/go/bin/boru
#   kg-verify          the committed knowledge graph matches the tree
#   vet | lint-assertions | lint
#   test-modules       every module's tests except test/go/langspec
#   test-langspec N    langspec shard N of $(make langspec-shard-count)
#   test-langspec-all  every langspec shard, in sequence
#   parser-parity | cover-gate-core | race-gates | borudebug-gates | wasm
#   vuln               govulncheck (advisory: never fails the run)
#   direction-gates    the direction lane (red until the debt is paid; never
#                      fails the run — its table is the output)
#   all                the whole sequence, as `make ci-local`
set -euo pipefail
cd "$(dirname "$0")/.."

step=${1:-}
[ -n "$step" ] || { sed -n '2,24p' "$0"; exit 2; }
shift || true

run() { echo "==> [ci] $*"; "$@"; }

case "$step" in
  build-cli)        run make -C cmd/go build ;;
  kg-verify)        run make -C kg verify ;;
  vet)              run make vet ;;
  lint-assertions)  run make -C lang/go lint-assertions ;;
  lint)             run make lint ;;
  test-modules)     run make test-modules ;;
  test-langspec)    run make test-langspec SHARD="${1:?shard number}" ;;
  test-langspec-all)
    n=$(make -s langspec-shard-count)
    for i in $(seq 1 "$n"); do run make test-langspec SHARD="$i"; done ;;
  parser-parity)    run make parser-parity ;;
  cover-gate-core)  run make cover-gate-core ;;
  race-gates)
    # A shared immutable Program driven across forks, the island sub-engine
    # reuse, and the concurrent spec rows in compiled mode must be
    # data-race-free; `make test` does not run under -race.
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
    for s in build-cli kg-verify vet lint-assertions lint test-modules test-langspec-all parser-parity cover-gate-core race-gates borudebug-gates wasm vuln direction-gates; do
      "$0" "$s"
    done
    echo "==> [ci] every CI step ran" ;;
  *) echo "scripts/ci-steps.sh: unknown step '$step'" >&2; exit 2 ;;
esac
