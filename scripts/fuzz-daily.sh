#!/usr/bin/env bash
# scripts/fuzz-daily.sh — the daily deep property fuzz, and its report.
#
#   scripts/fuzz-daily.sh [OUT_DIR]                (make fuzz-daily)
#   scripts/fuzz-daily.sh --report-only OUT_DIR    re-render OUT_DIR/report.md
#
# Runs the property fuzzers in test/go/langspec at the deep budget:
# TestPropertyRobustness (no generated program may hit an internal error)
# and TestCheckRunFalsePositive (programs the checker rejects that run
# clean — counted and sampled, never gated at this budget). The seeds move
# every day: BORU_FUZZ_SEED_BASE defaults to the UTC date × 100, so
# 2026-10-19 runs seeds 2026101901 … 2026101920 and no two days repeat a
# program; set it to replay a day. BORU_FUZZ_SEEDS and BORU_FUZZ_ITERS set
# the budget (default 20 × 5000 a fuzzer, a few minutes).
#
# Writes OUT_DIR (default fuzz-out/):
#   findings.jsonl  the tests' own records (BORU_FUZZ_FINDINGS): one line
#                   per internal error and one summary per fuzzer
#   go-test.log     the go test -v output
#   report.md       the run in Markdown: each finding with its shrunk
#                   program, the engine's message and the command that
#                   reproduces it
# and exits non-zero when a fuzzer failed or never reported. The daily
# build (.github/workflows/daily.yml) uploads the directory, puts report.md
# in the run's summary, and files every finding as an issue
# (scripts/scheduled-issues.sh findings).
set -uo pipefail
cd "$(dirname "$0")/.."

fuzzers="TestPropertyRobustness TestCheckRunFalsePositive"

report_only=0
if [ "${1:-}" = "--report-only" ]; then report_only=1; shift; fi
out=${1:-fuzz-out}
mkdir -p "$out" && out=$(cd "$out" && pwd)
findings=$out/findings.jsonl
log=$out/go-test.log
report=$out/report.md

# The command that regenerates a finding: the same seed's iterations up to
# and including the one that failed.
repro() { # repro <test> <seed> <iter>
  echo "cd test/go && BORU_FUZZ_SEED_BASE=$(($2 - 1)) BORU_FUZZ_SEEDS=1 BORU_FUZZ_ITERS=$(($3 + 1)) go test ./langspec/ -run '^$1\$' -v"
}

render() { # render <go test exit status> — writes $report, returns the verdict
  local rc=$1 verdict=0 n base seeds iters
  touch "$findings"
  base=$(jq -rs '[.[] | select(.kind == "summary")][0].seedBase // empty' "$findings")
  seeds=$(jq -rs '[.[] | select(.kind == "summary")][0].seeds // empty' "$findings")
  iters=$(jq -rs '[.[] | select(.kind == "summary")][0].iters // empty' "$findings")
  n=$(jq -s '[.[] | select(.kind == "internal_error")] | length' "$findings")
  {
    echo "## Daily property fuzz"
    echo
    if [ -n "$base" ]; then
      echo "Seeds $((base + 1)) … $((base + seeds)), $iters programs each (\`BORU_FUZZ_SEED_BASE=$base\`)."
    fi
    echo
    echo "| fuzzer | programs | result |"
    echo "|---|---:|---|"
    for t in $fuzzers; do
      local row
      row=$(jq -rs --arg t "$t" '
        [.[] | select(.kind == "summary" and .test == $t)][0]
        | if . == null then empty
          elif .test == "TestPropertyRobustness" then
            "| \(.test) | \(.programs) | " + (if .failures == 0 then "clean" else "**\(.failures) internal error(s)**" end) + " |"
          else
            "| \(.test) | \(.programs) | \(.rejected) check-rejected, \(.divergent) of them run clean (reported, not gated) |"
          end' "$findings")
      if [ -z "$row" ]; then
        row="| $t | — | **no summary: the fuzzer did not finish (see go-test.log)** |"
        verdict=1
      fi
      echo "$row"
    done
    echo
    if [ "$n" -gt 0 ]; then
      verdict=1
      echo "### Findings"
      echo
      echo "Each is filed as a \`fuzz\` issue; the same shrunk program found again is added to its issue."
      echo
      local i=0
      while IFS= read -r rec; do
        i=$((i + 1))
        local t seed iter
        t=$(jq -r .test <<< "$rec"); seed=$(jq -r .seed <<< "$rec"); iter=$(jq -r .iter <<< "$rec")
        echo "#### $i. $t — seed $seed, iteration $iter"
        echo
        echo "Engine message:"
        echo '~~~'; jq -r '.error // "(none recorded)"' <<< "$rec"; echo '~~~'
        echo "Shrunk program:"
        echo '~~~'; jq -r .min <<< "$rec"; echo '~~~'
        echo "Full program:"
        echo '~~~'; jq -r .program <<< "$rec"; echo '~~~'
        echo "Reproduce:"
        echo '~~~bash'; repro "$t" "$seed" "$iter"; echo '~~~'
        echo
      done < <(jq -c 'select(.kind == "internal_error")' "$findings")
    fi
    local samples
    # A program can hold backticks (a template string), so samples go in a
    # fenced block, one a line, never inline code.
    samples=$(jq -r 'select(.kind == "summary" and .test == "TestCheckRunFalsePositive") | (.samples // [])[]' "$findings")
    if [ -n "$samples" ]; then
      echo "### Check-rejected programs that run clean (samples)"
      echo
      echo "Each is a checker false positive or a sanctioned dead-branch shape (check_run_fp_test.go says which are sanctioned); triage a new shape like any finding."
      echo
      echo '~~~'; echo "$samples"; echo '~~~'
      echo
    fi
    if [ "$rc" -ne 0 ] && [ "$verdict" -eq 0 ]; then
      verdict=1
      echo "**go test failed without a finding** (exit $rc) — a build or harness failure; see go-test.log."
      echo
    fi
    echo "_The procedure for acting on a finding is in AGENTS.md (\"Daily fuzz findings\")._"
  } > "$report"
  return "$verdict"
}

if [ "$report_only" = 1 ]; then
  render 0
  exit $?
fi

base=${BORU_FUZZ_SEED_BASE:-$(( $(date -u +%Y%m%d) * 100 ))}
case $base in
  '' | *[!0-9]*) echo "BORU_FUZZ_SEED_BASE must be a non-negative integer, not '$base'" >&2; exit 2 ;;
esac
seeds=${BORU_FUZZ_SEEDS:-20}
iters=${BORU_FUZZ_ITERS:-5000}
rm -f "$findings"
echo "==> fuzz-daily: seeds $((base + 1))..$((base + seeds)), $iters programs each, into $out"
pattern="^($(echo $fuzzers | tr ' ' '|'))\$"
( cd test/go && BORU_FUZZ_SEED_BASE=$base BORU_FUZZ_SEEDS=$seeds BORU_FUZZ_ITERS=$iters \
    BORU_FUZZ_FINDINGS=$findings go test ./langspec/ -run "$pattern" -count=1 -v -timeout 25m ) > "$log" 2>&1
rc=$?
grep -E '^(--- |ok|FAIL)|fuzz: seeds' "$log" | sed 's/^ *//'
render "$rc"
verdict=$?
echo "==> fuzz-daily: report in $report ($([ "$verdict" = 0 ] && echo clean || echo 'FINDINGS or FAILURE'))"
exit "$verdict"
