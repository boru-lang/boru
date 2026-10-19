#!/usr/bin/env bash
# scripts/scheduled-issues.sh — turn a scheduled build's results into issues.
#
#   scripts/scheduled-issues.sh findings FINDINGS_JSONL
#   scripts/scheduled-issues.sh health TITLE NEEDS_JSON
#   scripts/scheduled-issues.sh selftest
#
# A scheduled run fails where nobody is watching, so its results become
# issues: the merged coverage gate once sat red four nights running and
# nothing said so.
#
#   findings  files each internal error in a fuzz findings file
#             (scripts/fuzz-daily.sh) as an issue labelled `fuzz`, one per
#             distinct shrunk program. A program already open in an issue
#             (its "Finding key" line) gets a comment instead. A fuzz issue
#             never closes itself: tomorrow's seeds generate different
#             programs, so a green day proves nothing; the PR that fixes
#             the engine closes it.
#   health    keeps ONE issue, titled TITLE and labelled `ci-health`, per
#             scheduled workflow: opened (or commented on) when a job in
#             NEEDS_JSON — the workflow's `toJSON(needs)` — failed or was
#             cancelled, and commented on and closed once they all pass.
#   selftest  runs both lifecycles against the live repository under the
#             `ci-selftest` label (file, find again, recover, close) and
#             fails unless each step did what it should; it leaves no issue
#             open. The daily build runs it on demand (its selftest input).
#
# RUN_URL names the run in every issue and comment, WORKFLOW the workflow.
# DRY_RUN=1 prints each write instead of making it; reads still happen.
# Needs gh and jq, and a token that may write issues.
set -euo pipefail
cd "$(dirname "$0")/.."

run_url=${RUN_URL:-}
workflow=${WORKFLOW:-the scheduled build}
today=$(date -u +%Y-%m-%d)
run_link=${run_url:+" ([run]($run_url))"}

dry() { [ "${DRY_RUN:-0}" = 1 ]; }
sha16() { if command -v sha256sum > /dev/null; then sha256sum; else shasum -a 256; fi | cut -c1-16; }

# The command that regenerates a finding (the same as scripts/fuzz-daily.sh's).
repro() { # repro <test> <seed> <iter>
  echo "cd test/go && BORU_FUZZ_SEED_BASE=$(($2 - 1)) BORU_FUZZ_SEEDS=1 BORU_FUZZ_ITERS=$(($3 + 1)) go test ./langspec/ -run '^$1\$' -v"
}

ensure_label() { # ensure_label <name> <colour> <description>
  dry && return 0
  gh label create "$1" --color "$2" --description "$3" --force > /dev/null
}

create_issue() { # create_issue <label> <title> <body file> — prints the new issue's number
  if dry; then
    echo "    [dry-run] open [$1] $2" >&2
    echo "(dry-run)"
    return
  fi
  local url
  url=$(gh issue create --label "$1" --title "$2" --body-file "$3")
  echo "${url##*/}"
}

comment() { # comment <number> <body>
  if dry; then echo "    [dry-run] comment #$1: $2" >&2; return; fi
  gh issue comment "$1" --body "$2" > /dev/null
}

close_issue() { # close_issue <number> <closing comment>
  if dry; then echo "    [dry-run] close #$1: $2" >&2; return; fi
  gh issue close "$1" --comment "$2" > /dev/null
}

finding_body() { # finding_body <record> <key>
  local rec=$1 key=$2 t seed iter
  t=$(jq -r .test <<< "$rec"); seed=$(jq -r .seed <<< "$rec"); iter=$(jq -r .iter <<< "$rec")
  cat <<EOF
The daily build's property fuzz found a program that makes the engine fail internally: an \`internal_error\`, the engine's own failure, not the program's.

Finding key: \`$key\` (the fuzzer and the shrunk program). A later run that shrinks to the same program comments here rather than opening a new issue.

| | |
|---|---|
| fuzzer | \`$t\` |
| seed, iteration | $seed, $iter |
| found | $today$run_link |

**Engine message**
~~~
$(jq -r '.error // "(none recorded)"' <<< "$rec")
~~~

**Shrunk program**
~~~
$(jq -r .min <<< "$rec")
~~~

**Full program**
~~~
$(jq -r .program <<< "$rec")
~~~

### Acting on it

1. Reproduce. The same seed and iteration regenerate the same program:
   ~~~bash
   $(repro "$t" "$seed" "$iter")
   ~~~
2. Pin the shrunk program as a regression test that fails today, with its negative pair: a \`lang/spec/*.tsv\` row in the program's family, or a Go test beside the construct.
3. Fix the engine. The PR that makes the pin pass closes this issue (\`Fixes #<this issue>\`).

This issue stays open until that fix lands. A green daily build proves nothing here, because tomorrow's seeds generate different programs.
EOF
}

cmd_findings() { # cmd_findings <findings.jsonl>
  local file=$1 label=${FUZZ_LABEL:-fuzz} filed=0 again=0 handled=" "
  if ! jq -e 'select(.kind == "internal_error")' "$file" > /dev/null 2>&1; then
    echo "==> findings: none in ${file}"
    return 0
  fi
  ensure_label "$label" d93f0b "An internal error the daily property fuzz found"
  local open
  open=$(gh issue list --label "$label" --state open --limit 500 --json number,body)
  while IFS= read -r rec; do
    local t seed iter min key num body
    t=$(jq -r .test <<< "$rec"); seed=$(jq -r .seed <<< "$rec"); iter=$(jq -r .iter <<< "$rec")
    min=$(jq -r .min <<< "$rec")
    key=$(printf '%s\n%s' "$t" "$min" | sha16)
    # The same shrunk program from two seeds of one run is one finding.
    case "$handled" in *" $key "*) continue ;; esac
    handled="$handled$key "
    num=$(jq -r --arg k "Finding key: \`$key\`" '[.[] | select(.body | contains($k))][0].number // empty' <<< "$open")
    if [ -n "$num" ]; then
      comment "$num" "Found again on $today$run_link: seed $seed, iteration $iter. Reproduce: \`$(repro "$t" "$seed" "$iter")\`"
      echo "==> findings: $key found again, commented on #$num"
      again=$((again + 1))
    else
      body=$(mktemp)
      finding_body "$rec" "$key" > "$body"
      num=$(create_issue "$label" "Fuzz: internal error in $(printf '%s' "$min" | cut -c1-70)" "$body")
      rm -f "$body"
      echo "==> findings: $key filed as #$num"
      filed=$((filed + 1))
    fi
  done < <(jq -c 'select(.kind == "internal_error")' "$file")
  echo "==> findings: $filed filed, $again found again"
}

cmd_health() { # cmd_health <title> <needs json>
  local title=$1 needs=$2 label=${HEALTH_LABEL:-ci-health} failed num body
  failed=$(jq -r 'to_entries[] | select(.value.result == "failure" or .value.result == "cancelled")
    | "\(.key) (\(.value.result))"' <<< "$needs" | paste -sd ',' - | sed 's/,/, /g')
  ensure_label "$label" e11d21 "A scheduled workflow is failing; closes itself when it passes again"
  num=$(gh issue list --label "$label" --state open --limit 200 --json number,title \
    | jq -r --arg t "$title" '[.[] | select(.title == $t)][0].number // empty')
  if [ -n "$failed" ]; then
    if [ -n "$num" ]; then
      comment "$num" "Failed again on $today$run_link: $failed."
      echo "==> health: still failing, commented on #$num"
    else
      body=$(mktemp)
      cat > "$body" <<EOF
**$workflow** failed on $today$run_link.

Failed: $failed.

Look at the run's jobs and fix main like any red build. A fuzz finding also has its own \`fuzz\` issue with the program and how to reproduce it. This is the workflow's one health issue: a later failure adds a comment here, and the next green run closes it.
EOF
      num=$(create_issue "$label" "$title" "$body")
      rm -f "$body"
      echo "==> health: failing, opened #$num"
    fi
  elif [ -n "$num" ]; then
    close_issue "$num" "Green again on $today$run_link. Closing."
    echo "==> health: green again, closed #$num"
  else
    echo "==> health: green"
  fi
}

# selftest checks each step against the live repository. A check reads
# the issue back rather than trusting the step's own output.
cmd_selftest() {
  if dry; then echo "selftest makes real issues; it does not run under DRY_RUN" >&2; exit 2; fi
  local label=ci-selftest fail=0 tmp n
  tmp=$(mktemp -d)
  check() { # check <description> <want> <got>
    if [ "$2" = "$3" ]; then echo "    ok    $1"; else echo "    FAIL  $1: want $2, got $3"; fail=1; fi
  }
  open_count() { gh issue list --label "$label" --state open --json number --jq length; }
  ensure_label "$label" c5def5 "Made and closed by scripts/scheduled-issues.sh selftest"
  check "no self-test issue open at the start" 0 "$(open_count)"

  printf '%s\n' '{"kind":"internal_error","test":"TestPropertyRobustness","seed":1,"iter":0,"program":"[self-test] 1 add 1","min":"[self-test] 1 add 1","error":"[self-test] not a real finding"}' > "$tmp/findings.jsonl"
  FUZZ_LABEL=$label cmd_findings "$tmp/findings.jsonl"
  check "a finding opens one issue" 1 "$(open_count)"
  FUZZ_LABEL=$label cmd_findings "$tmp/findings.jsonl"
  check "the same finding again opens nothing new" 1 "$(open_count)"
  n=$(gh issue list --label "$label" --state open --json number --jq '.[0].number')
  check "the same finding again is a comment on its issue" 1 "$(gh issue view "$n" --json comments --jq '.comments | length')"

  local title="[self-test] Scheduled build is failing"
  HEALTH_LABEL=$label cmd_health "$title" '{"selftest":{"result":"failure"}}'
  check "a failure opens the health issue" 2 "$(open_count)"
  HEALTH_LABEL=$label cmd_health "$title" '{"selftest":{"result":"failure"}}'
  check "a second failure opens nothing new" 2 "$(open_count)"
  HEALTH_LABEL=$label cmd_health "$title" '{"selftest":{"result":"success"}}'
  check "a green run closes the health issue" 1 "$(open_count)"
  close_issue "$n" "Self-test complete. Closing."
  check "no self-test issue left open" 0 "$(open_count)"
  rm -rf "$tmp"
  [ "$fail" = 0 ] || { echo "==> selftest: FAILED"; exit 1; }
  echo "==> selftest: every step behaved"
}

case "${1:-}" in
  findings) cmd_findings "${2:?findings file}" ;;
  health)   cmd_health "${2:?issue title}" "${3:?needs json}" ;;
  selftest) cmd_selftest ;;
  *) sed -n '2,30p' "$0"; exit 2 ;;
esac
