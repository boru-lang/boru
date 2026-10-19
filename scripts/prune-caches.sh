#!/usr/bin/env bash
# scripts/prune-caches.sh — keep the GitHub Actions cache one generation deep.
#
#   scripts/prune-caches.sh [--dry-run]            delete superseded entries
#   scripts/prune-caches.sh [--dry-run] --ref REF  delete every entry on REF
#
# Every green CI job saves a fresh build-cache entry keyed by its run id
# (.github/actions/setup), and a restore only ever takes the NEWEST entry
# of a key family, so each older one is dead weight. Once the repository's
# 10 GB store is full, GitHub evicts the least recently used entries —
# main's warm build caches among them, which is what turns a 40-second job
# into a cold two-minute one. The default mode deletes, per ref, every
# entry that a newer entry of the same family superseded. A family is the
# key without its run id and its content hash: go-build-Linux-test-cmd-<run
# id> and go-mod-Linux-<sha256> are one family each.
#
# --ref deletes every entry on one ref: a closed pull request's
# refs/pull/<n>/merge, which nothing restores from again. --dry-run lists
# what would go and deletes nothing.
#
# .github/workflows/cache-cleanup.yml runs it after every CI, Pages and
# coverage run, and when a pull request closes. Run by hand, it needs gh
# logged in with a token that may write the repository's Actions.
set -euo pipefail
cd "$(dirname "$0")/.."

dry=0
ref=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) dry=1; shift ;;
    --ref) ref=${2:?--ref needs a ref, e.g. refs/pull/123/merge}; shift 2 ;;
    *) sed -n '2,23p' "$0"; exit 2 ;;
  esac
done

gb() { awk -v b="$1" 'BEGIN { printf "%.2f GB", b / 1e9 }'; }

store=$(gh api 'repos/{owner}/{repo}/actions/cache/usage' \
  --jq '"\(.active_caches_count) entries, \(.active_caches_size_in_bytes)"')
echo "==> prune-caches: the store holds ${store%, *}, $(gb "${store##*, }")"

fields=id,key,ref,createdAt,sizeInBytes
if [ -n "$ref" ]; then
  why="every entry on $ref"
  doomed=$(gh cache list --ref "$ref" --limit 1000 --json "$fields" \
    --jq '.[] | [.id, .sizeInBytes, .ref, .key] | @tsv')
else
  why="every entry a newer one of its family superseded"
  # Strip the run id (the 6-plus-digit tail the setup action appends; a
  # shard number stays), then a content hash, to name the family; keep
  # the newest entry of each (ref, family) and list the rest.
  doomed=$(gh cache list --limit 1000 --json "$fields" --jq '
    map(. + {family: (.key | sub("-[0-9]{6,}$"; "") | sub("-[0-9a-f]{40,}$"; ""))})
    | group_by([.ref, .family])
    | map(sort_by(.createdAt) | reverse | .[1:])
    | flatten
    | .[]
    | [.id, .sizeInBytes, .ref, .key] | @tsv')
fi

n=0
bytes=0
while IFS=$'\t' read -r id size entry_ref key; do
  [ -n "$id" ] || continue
  n=$((n + 1))
  bytes=$((bytes + size))
  if [ "$dry" = 1 ]; then
    echo "    would delete  $key  ($entry_ref, $((size / 1000000)) MB)"
  else
    echo "    delete  $key  ($entry_ref, $((size / 1000000)) MB)"
    # Another prune, or GitHub's own eviction, may have got there first.
    gh cache delete "$id" > /dev/null 2>&1 || echo "      (already gone)"
  fi
done <<< "$doomed"

verb="deleted"
[ "$dry" = 0 ] || verb="would delete"
echo "==> prune-caches: $verb $n entries, $(gb "$bytes") — $why"
