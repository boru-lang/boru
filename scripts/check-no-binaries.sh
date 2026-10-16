#!/usr/bin/env bash
# Refuse newly-added large or executable-binary blobs.
#
# Two compiled Go binaries (tools/piecetool/piecetool, test/go/piecetool/piecetool,
# 6.9MB each) were committed in the four-piece cut and deleted a commit later —
# but a delete does not reclaim anything: the blob stays in history forever and
# every clone keeps paying for it. test/solardemo/solardemo (8.6MB) sat tracked
# for far longer. .gitignore only helps once someone notices; this gate is what
# notices.
#
# Usage:
#   scripts/check-no-binaries.sh              # check working tree vs merge-base with main
#   scripts/check-no-binaries.sh <base-ref>   # check vs an explicit base
#
# Allow a genuinely-needed large file by adding its exact path to ALLOW below.
set -uo pipefail

MAX_BYTES=${MAX_BYTES:-1048576}   # 1MB

# Paths permitted to exceed MAX_BYTES. Keep this list short and justified.
ALLOW=(
  # Generated syntax-combination spec: the frozen contract syntax_matrix_test.go
  # checks the interpreter and checker against (see `make spec-gen`).
  "test/go/specgen/syntax-matrix.tsv"
  "test/go/specgen/syntax-matrix-passing.tsv"
  "test/go/specgen/syntax-matrix-len5-passing.tsv"
  "test/go/specgen/syntax-matrix-len5-fail-check.tsv"
)

allowed() {
  local p=$1 a
  for a in "${ALLOW[@]}"; do [ "$p" = "$a" ] && return 0; done
  return 1
}

base=${1:-}
if [ -z "$base" ]; then
  base=$(git merge-base HEAD origin/main 2>/dev/null || git merge-base HEAD main 2>/dev/null || echo "")
fi

if [ -n "$base" ]; then
  files=$(git diff --name-only --diff-filter=AM "$base"...HEAD 2>/dev/null)
else
  files=$(git ls-files)   # no base (e.g. shallow clone): check everything tracked
fi

fail=0
while IFS= read -r f; do
  [ -n "$f" ] || continue
  [ -f "$f" ] || continue
  allowed "$f" && continue

  size=$(git cat-file -s "$(git rev-parse "HEAD:$f" 2>/dev/null)" 2>/dev/null || stat -c%s "$f" 2>/dev/null || echo 0)

  # An executable binary is never acceptable, at any size.
  magic=$(head -c 4 "$f" 2>/dev/null | od -An -tx1 | tr -d ' \n')
  case "$magic" in
    7f454c46*|cffaedfe*|cefaedfe*|feedface*|feedfacf*|4d5a*)
      echo "::error file=$f::$f is a compiled binary ($size bytes). Build it, don't commit it — a deleted blob still lives in history forever."
      fail=1; continue;;
  esac

  if [ "$size" -gt "$MAX_BYTES" ]; then
    echo "::error file=$f::$f is $size bytes, over the ${MAX_BYTES}-byte limit. If it is genuinely required, add it to ALLOW in scripts/check-no-binaries.sh with a reason."
    fail=1
  fi
done <<< "$files"

if [ "$fail" -ne 0 ]; then
  echo
  echo "Committed binaries and large blobs cannot be undone by deleting them later:"
  echo "removing the file leaves the blob in history, so every clone still pays."
  exit 1
fi

echo "no new binaries or oversized files"
