#!/usr/bin/env bash
# brand-audit.sh — inventory every place the product name is baked in, so a
# rename is a checklist instead of an archaeology dig.
#
# The name is a codename and will change. This script classifies each
# occurrence by what breaks if it changes underneath existing data:
#
#   FROZEN      already in files / OS credential stores. Renaming ORPHANS
#               data. These must live in cmd/go/internal/wire and be
#               brand-free; the wire test suite enforces that.
#   MIGRATABLE  user-visible surfaces (paths, env vars, file extensions,
#               the import namespace). Renaming needs a dual-read window
#               and a deprecation notice, not a find-and-replace.
#   COSMETIC    text a human reads and nothing parses back. Rename freely.
#
# Exit status is always 0: this is a report, not a gate. The hard gates are
# the golden fixtures and the brand-containment test in package wire.
set -uo pipefail
cd "$(dirname "$0")/.."

GO_SRC=(cmd lang core eng basic parser check compiler)
say() { printf '\n\033[1m%s\033[0m\n' "$1"; }
count() { grep -rn --include='*.go' "$1" "${GO_SRC[@]}" 2>/dev/null | grep -v '_test.go' | wc -l | tr -d ' '; }

say "FROZEN — must be brand-free (enforced by cmd/go/internal/wire tests)"
if [ -d cmd/go/internal/wire ]; then
  grep -nE '^\t(Keyring|Export|Exec|Keychain)[A-Za-z]* +=' cmd/go/internal/wire/wire.go \
    | sed 's/^/  /'
  printf '  golden fixtures: %s\n' "$(ls cmd/go/internal/wire/testdata 2>/dev/null | wc -l | tr -d ' ')"
else
  echo "  MISSING — package wire not found"
fi

say "MIGRATABLE — needs a compatibility window before it changes"
printf '  %-34s %s\n' "import \"<name>:...\" namespace"  "$(count '"boru:')"
printf '  %-34s %s\n' "*.<name> source extension"        "$(count '\.boru"')"
printf '  %-34s %s\n' "<NAME>_* environment variables"   "$(count 'BORU_')"
printf '  %-34s %s\n' "~/.<name> and .<name>/ paths"     "$(count '\.boru/\|"\.boru"')"
printf '  %-34s %s\n' "<name>.jsonic manifest"           "$(count '"boru\.jsonic"')"

say "COSMETIC — safe to rename with the product"
printf '  %-34s %s\n' "help text, banners, labels"       "$(count 'boru' )"
printf '  %-34s %s\n' "markdown docs"                    "$(grep -rl 'boru' --include='*.md' . 2>/dev/null | grep -v '\.git' | wc -l | tr -d ' ')"

say "Before renaming"
cat <<'NOTE'
  1. Change only cmd/go/internal/brand. Nothing in wire should move.
  2. If a FROZEN identifier genuinely must change: add the old value to the
     legacy list, generate a golden fixture for it, and update the pins in
     cmd/go/internal/wire/wire_test.go — in one commit.
  3. For each MIGRATABLE surface, ship the new spelling reading BOTH, and
     keep the old one working for at least one release.
  4. Run: make test && go test ./cmd/go/internal/wire/...
NOTE
