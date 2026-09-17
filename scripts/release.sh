#!/usr/bin/env bash
#
# release.sh — publish the boru modules so `go install …@latest` works.
#
# Releases core/go, check/go, compiler/go, eng/go, basic/go, lang/go and
# cmd/go in DEPENDENCY ORDER (the four-piece kernel split —
# design/legacy/ENG-FOUR-PIECE.0.ignore). For each
# module it:
#   1. auto-bumps the PATCH from the module's latest `<module>/vX.Y.Z` tag
#      (or v0.0.1 if the module has never been tagged);
#   2. strips the local `replace github.com/boru-lang/boru/… => ../…` directives
#      (a published go.mod MUST NOT contain replace directives, or
#      `go install …@version` refuses it);
#   3. pins the sibling `require` lines to the versions just released;
#   4. `go mod tidy` + commits the module's go.mod/go.sum (and cmd/go's
#      main.go version stamp);
#   5. tags `<module>/vX.Y.Z` and pushes main + the tag.
#
# The whole run is gated by a full `make test` up front — nothing is tagged
# unless the entire suite passes. Local development keeps building via the
# committed `go.work` (which supplies in-tree sibling source on top of the
# now-replace-free, real-version go.mods).
#
# Usage:  make release        # or: scripts/release.sh
#         DRY_RUN=1 make release   # print what would happen, tag/push nothing
#
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$(pwd)
DRY_RUN=${DRY_RUN:-0}

run() { if [ "$DRY_RUN" = 1 ]; then echo "  [dry-run] $*"; else eval "$*"; fi; }

# next_patch <module-path> — echo the next vMAJOR.MINOR.(PATCH+1) for the
# module's tag namespace, or 0.0.1 when it has never been released.
next_patch() {
  local m=$1 latest v
  latest=$(git tag -l "$m/v*" --sort=-version:refname | head -1)
  if [ -z "$latest" ]; then echo "0.0.1"; return; fi
  v=${latest#"$m"/v}
  echo "${v%%.*}.$(echo "$v" | cut -d. -f2).$(( $(echo "$v" | cut -d. -f3) + 1 ))"
}

# edit_module runs a go.mod-rewriting function only for a real release; in a
# dry run it prints the intent and leaves the working tree untouched.
edit_module() { # $1 = description, $2 = function name
  if [ "$DRY_RUN" = 1 ]; then echo "  [dry-run] $1"; else "$2"; fi
}

# ---- preflight ----
[ -z "$(git status --porcelain)" ] || { echo "ERROR: working tree not clean"; exit 1; }
[ "$(git rev-parse --abbrev-ref HEAD)" = main ] || { echo "ERROR: releases run from main"; exit 1; }
# Fetch tags so next_patch sees releases cut on another machine / in CI —
# otherwise a fresh or shallow checkout recomputes a stale (or duplicate) patch.
git fetch --tags --quiet origin || true

COREV=$(next_patch core/go); PARSERV=$(next_patch parser/go)
CHECKV=$(next_patch check/go); COMPILERV=$(next_patch compiler/go)
ENGV=$(next_patch eng/go); BASICV=$(next_patch basic/go); LANGV=$(next_patch lang/go); CMDV=$(next_patch cmd/go)
echo "==> Releasing  core/go v$COREV  ·  parser/go v$PARSERV  ·  check/go v$CHECKV  ·  compiler/go v$COMPILERV"
echo "==>            eng/go v$ENGV  ·  basic/go v$BASICV  ·  lang/go v$LANGV  ·  cmd/go v$CMDV"

# sibling_version maps a module path to the version just computed for it.
# Every pin below goes through this, so adding a module means adding ONE
# line here and its own stanza — never editing six argument lists.
sibling_version() {
  case "$1" in
    core/go)     echo "$COREV" ;;
    parser/go)   echo "$PARSERV" ;;
    check/go)    echo "$CHECKV" ;;
    compiler/go) echo "$COMPILERV" ;;
    eng/go)      echo "$ENGV" ;;
    basic/go)    echo "$BASICV" ;;
    lang/go)     echo "$LANGV" ;;
    cmd/go)      echo "$CMDV" ;;
    *) echo "ERROR: no release version for sibling $1" >&2; return 1 ;;
  esac
}

# pin_siblings <module dir> — drop every local boru replace and pin every
# boru sibling the module actually REQUIRES, read from its own go.mod.
#
# Derived rather than listed because the hand-written lists went stale
# three times without anyone noticing: the four-piece cut, the parser
# extraction and the facade re-pointing each changed who requires whom,
# while the pin functions still named eng for modules that had stopped
# requiring it and no module at all for parser. A published go.mod that
# keeps a replace, or carries a v0.0.0 sibling, cannot be `go install`ed.
pin_siblings() {
  local dir=$1 sib ver
  ( cd "$dir"
    for sib in $(grep -oE 'github\.com/boru-lang/boru/[a-z]+/go' go.mod | sort -u); do
      go mod edit -dropreplace="$sib"
    done
    for sib in $(go mod edit -json | grep -oE '"Path": "github\.com/boru-lang/boru/[a-z]+/go"' \
                   | grep -oE 'boru/[a-z]+/go' | sed 's|^boru/||' | sort -u); do
      # `go mod edit -json` reports the module's OWN path alongside its
      # requires; pinning that would emit a self-require.
      [ "$sib" = "$dir" ] && continue
      ver=$(sibling_version "$sib") || exit 1
      go mod edit -require="github.com/boru-lang/boru/$sib@v$ver"
    done
    GOFLAGS=-mod=mod GOWORK=off go mod tidy
  )
}

# assert_publishable <module dir> — the invariant the release exists to
# uphold, checked instead of assumed. This is what a stale pin list
# actually costs: a tagged CLI whose advertised `go install` fails.
assert_publishable() {
  local dir=$1
  if grep -qE '^\s*replace\s+github\.com/boru-lang/boru' "$dir/go.mod"; then
    echo "ERROR: $dir/go.mod still has a boru replace directive — unpublishable" >&2
    grep -nE '^\s*replace\s+github\.com/boru-lang/boru' "$dir/go.mod" >&2
    exit 1
  fi
  if grep -qE 'github\.com/boru-lang/boru/[a-z]+/go v0\.0\.0' "$dir/go.mod"; then
    echo "ERROR: $dir/go.mod requires an unpublished v0.0.0 sibling" >&2
    grep -nE 'github\.com/boru-lang/boru/[a-z]+/go v0\.0\.0' "$dir/go.mod" >&2
    exit 1
  fi
}

if [ "$DRY_RUN" = 1 ]; then
  echo "==> [dry-run] skipping the full test gate"
else
  echo "==> Full test suite (release gate)"
  make test
fi

# The actual go.mod surgery, run only for a real release (see edit_module).
# NOTE: `go mod tidy` runs with GOWORK=off, where a DEPENDENCY module's own
# replace directives are ignored — so every sibling a module requires must
# already be pinned to a published version, which is why the kernel pieces
# release before eng.
check_pin()    { pin_siblings check/go; }
parser_pin()   { pin_siblings parser/go; }
compiler_pin() { pin_siblings compiler/go; }
eng_pin()      { pin_siblings eng/go; }
basic_pin()    { pin_siblings basic/go; }
lang_pin()     { pin_siblings lang/go; }
cmd_pin()      { ( pin_siblings cmd/go
  # Version lives in cmd/go/main.go (boru/main.go is a thin entrypoint).
  cd cmd/go && perl -i -pe 's{(^var Version = )"[^"]*"}{$1"'"$CMDV"'"}' main.go ); }

# ---- core/go (leaf: no sibling deps, no go.mod change) ----
echo "==> core/go v$COREV"
run "git tag core/go/v$COREV"
run "git push origin core/go/v$COREV"

# ---- parser/go (requires core/go; a leaf over the kernel) ----
# Must precede eng, basic, lang and cmd — all four require it. It was
# absent from this script entirely until the module existed, which would
# have tagged a CLI whose `go install` could not resolve parser/go.
echo "==> parser/go v$PARSERV"
edit_module "cd parser/go: drop core replace, pin core@v$COREV, go mod tidy" parser_pin
[ "$DRY_RUN" = 1 ] || assert_publishable parser/go
run "git add parser/go/go.mod parser/go/go.sum"
run "git commit -m 'parser/go: v$PARSERV (core/go v$COREV)'"
run "git tag parser/go/v$PARSERV"
run "git push origin main parser/go/v$PARSERV"

# ---- check/go (requires core/go) ----
echo "==> check/go v$CHECKV"
edit_module "cd check/go: drop core replace, pin core@v$COREV, go mod tidy" check_pin
[ "$DRY_RUN" = 1 ] || assert_publishable check/go
run "git add check/go/go.mod check/go/go.sum"
run "git commit -m 'check/go: v$CHECKV (core/go v$COREV)'"
run "git tag check/go/v$CHECKV"
run "git push origin main check/go/v$CHECKV"

# ---- compiler/go (requires core/go + check/go) ----
echo "==> compiler/go v$COMPILERV"
edit_module "cd compiler/go: drop core+check replaces, pin versions, go mod tidy" compiler_pin
[ "$DRY_RUN" = 1 ] || assert_publishable compiler/go
run "git add compiler/go/go.mod compiler/go/go.sum"
run "git commit -m 'compiler/go: v$COMPILERV (core/go v$COREV, check/go v$CHECKV)'"
run "git tag compiler/go/v$COMPILERV"
run "git push origin main compiler/go/v$COMPILERV"

# ---- eng/go (requires core/go + check/go + compiler/go + parser/go) ----
echo "==> eng/go v$ENGV"
edit_module "cd eng/go: drop kernel replaces, pin core/check/compiler, go mod tidy" eng_pin
[ "$DRY_RUN" = 1 ] || assert_publishable eng/go
run "git add eng/go/go.mod eng/go/go.sum"
run "git commit -m 'eng/go: v$ENGV (core/go v$COREV, check/go v$CHECKV, compiler/go v$COMPILERV)'"
run "git tag eng/go/v$ENGV"
run "git push origin main eng/go/v$ENGV"

# ---- basic/go (requires core/go + check/go + parser/go; NOT compiler) ----
echo "==> basic/go v$BASICV"
edit_module "cd basic/go: drop eng replace, pin eng@v$ENGV, go mod tidy" basic_pin
[ "$DRY_RUN" = 1 ] || assert_publishable basic/go
run "git add basic/go/go.mod basic/go/go.sum"
run "git commit -m 'basic/go: v$BASICV (eng/go v$ENGV)'"
run "git tag basic/go/v$BASICV"
run "git push origin main basic/go/v$BASICV"

# ---- lang/go (requires basic/go + eng/go + the four kernel pieces) ----
echo "==> lang/go v$LANGV"
edit_module "cd lang/go: drop eng replace, pin eng@v$ENGV, go mod tidy" lang_pin
[ "$DRY_RUN" = 1 ] || assert_publishable lang/go
run "git add lang/go/go.mod lang/go/go.sum"
run "git commit -m 'lang/go: v$LANGV (eng/go v$ENGV, basic/go v$BASICV)'"
run "git tag lang/go/v$LANGV"
run "git push origin main lang/go/v$LANGV"

# ---- cmd/go (requires lang/go + basic/go + the kernel pieces) ----
echo "==> cmd/go v$CMDV"
edit_module "cd cmd/go: drop eng+lang replaces, pin versions, stamp Version=$CMDV, go mod tidy" cmd_pin
[ "$DRY_RUN" = 1 ] || assert_publishable cmd/go
run "git add cmd/go/go.mod cmd/go/go.sum cmd/go/main.go"
run "git commit -m 'cmd/go: v$CMDV (eng/go v$ENGV, lang/go v$LANGV)'"
run "git tag cmd/go/v$CMDV"
run "git push origin main cmd/go/v$CMDV"

echo "==> Done. Install with:"
echo "    go install github.com/boru-lang/boru/cmd/go/boru@v$CMDV"
