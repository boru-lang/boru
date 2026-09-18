#!/usr/bin/env bash
# scripts/cover-key.sh <module dir> — the cache key of a module's coverage
# profile: a digest of every tracked file's blob hash under the module and
# under every in-repo module it depends on (its go.mod `replace ../…`
# directives, followed transitively), plus go.work and lang/spec, plus the
# working-tree content of anything modified or untracked there. Two trees
# with the same key would produce the same profile, so `make cover-profile`
# reuses the stored .xout when the key matches (COVER_FRESH=1 forces).
set -euo pipefail
cd "$(dirname "$0")/.."
mod=${1:?module dir, e.g. core/go}

# The dependency closure over replace directives.
closure() {
  local seen=" " queue=("$1")
  while [ ${#queue[@]} -gt 0 ]; do
    local m=${queue[0]}; queue=("${queue[@]:1}")
    case "$seen" in *" $m "*) continue ;; esac
    seen="$seen$m "
    echo "$m"
    [ -f "$m/go.mod" ] || continue
    # Both forms of the directive: `replace X => ../y` on one line, and
    # the block `replace (` … `)` with one `X => ../y` per line. (The
    # block form was missed at first, so compiler/go's key ignored core/go
    # and check/go and a stale profile poisoned the merged view with
    # phantom uncovered blocks after the unify threading.)
    while read -r dep; do
      dep=$(cd "$m" && cd "$dep" 2>/dev/null && pwd -P) || continue
      dep=${dep#"$PWD"/}
      queue+=("$dep")
    done < <(awk '
      /^replace[ \t]*\($/ { block = 1; next }
      block && /^\)/      { block = 0; next }
      (block || /^replace /) && /=> *\.\.?\// { print $NF }
    ' "$m/go.mod")
  done
}

paths=$(closure "$mod" | sort -u)
{
  echo "go.work"; cat go.work go.work.sum 2>/dev/null
  for p in $paths lang/spec; do
    [ -e "$p" ] || continue
    git ls-files -s -- "$p"
    # Working-tree state the index does not carry: modified and untracked.
    { git diff --name-only -- "$p"; git ls-files --others --exclude-standard -- "$p"; } | sort -u | while read -r f; do
      [ -f "$f" ] && { printf '%s ' "$f"; git hash-object "$f"; }
    done
  done
} | sha256sum | cut -c1-16
