#!/usr/bin/env bash
# scripts/release-gate.sh — choose the commit a release cuts from, and prove
# it may be released.
#
#   scripts/release-gate.sh [COMMIT]
#
# A release is cut only from a commit on main that passed BOTH CI (its push
# run) and the daily build (macOS, Windows, the deep fuzz, the long suites).
# With no COMMIT the candidate is the head of the most recent successful
# daily build on main, so the second condition holds by construction; to
# release something newer, run the daily build on main first
# (`gh workflow run daily.yml`). A third condition keeps the release branch
# a snapshot of main: the last release must already be merged into the
# candidate (scripts/release.sh merges each release back into main).
#
# Prints the commit, and writes commit=<sha> to $GITHUB_OUTPUT when set.
# Needs gh (a token that may read the repository's Actions) and a checkout
# with main and the release branch fetched.
set -euo pipefail
cd "$(dirname "$0")/.."

repo=${GH_REPO:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}
fail() { echo "release-gate: $*" >&2; exit 1; }

# runs <workflow file> <event> — the head SHAs of its successful runs on main
runs() {
  gh api "repos/$repo/actions/workflows/$1/runs?branch=main&event=$2&status=success&per_page=100" \
    --jq '.workflow_runs[].head_sha'
}

git fetch --quiet origin main
git fetch --quiet origin release 2>/dev/null || true

daily_ok=$( { runs daily.yml schedule; runs daily.yml workflow_dispatch; } | sort -u)
if [ -n "${1:-}" ]; then
  commit=$(git rev-parse --verify "$1^{commit}") || fail "no such commit: $1"
else
  # The newest successful daily run, whichever event started it.
  commit=$(gh api "repos/$repo/actions/workflows/daily.yml/runs?branch=main&status=success&per_page=20" \
    --jq '[.workflow_runs[] | select(.event == "schedule" or .event == "workflow_dispatch")][0].head_sha // empty')
  [ -n "$commit" ] || fail "no successful daily build on main yet; run one: gh workflow run daily.yml"
fi
short=${commit:0:9}

git merge-base --is-ancestor "$commit" origin/main || fail "$short is not on main"
grep -qx "$commit" <<< "$daily_ok" \
  || fail "the daily build has not passed $short; run it on main (gh workflow run daily.yml) and release once it is green"
ci_ok=$(runs ci.yml push)
grep -qx "$commit" <<< "$ci_ok" \
  || fail "CI has not passed $short on main"
if git rev-parse --quiet --verify origin/release > /dev/null; then
  git merge-base --is-ancestor origin/release "$commit" \
    || fail "the last release (origin/release) is not merged into $short; merge release into main first"
fi

echo "release-gate: $short passed CI and the daily build, and contains the last release"
echo "$commit"
if [ -n "${GITHUB_OUTPUT:-}" ]; then echo "commit=$commit" >> "$GITHUB_OUTPUT"; fi
