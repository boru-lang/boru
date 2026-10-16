#!/usr/bin/env bash
# scripts/vet-copies.sh — the no-copy ratchet (`make vet-copies`;
# design/VALUE-NO-COPY.0.md, phase 0).
#
# Builds tools/vetcopies and runs it as go vet's -vettool in every module
# go.work lists, counting the copies of a core.Value in NON-TEST code:
#
#   value   copylocks findings through the zero-size noCopy marker in a
#           value's content (core's unexported content struct; no-copy
#           phase 4): every by-value copy of content, or of a struct that
#           holds it by value. A core.Value is a holder handle over the
#           content, and copying it copies a cell record, not content
#   bulk    bulkcopy findings: copy() and variadic append() over a
#           []core.Value, the runs of holders copylocks cannot see
#
# A module fails when either count rises above its ceiling in
# tools/vetcopies/ceilings.tsv, or when copylocks reports a copy of any
# other lock (a sync.Mutex, say): there must be none, and this is the only
# lane that would see one, because the marker makes the ordinary vet lanes
# run with -copylocks=false. A module with no row has ceilings of zero. It
# also fails on a function VARIABLE aliasing a function that takes a
# *core.Value (fnvaralias): calls through it are indirect, so every &v handed
# to them escapes to the heap — re-export such a function as a wrapper func.
#
# The ceilings only go down. UPDATE=1 lowers each ceiling to its module's
# current count and never raises one; a change that adds a copy the policy
# sanctions (design/VALUE-NO-COPY-EXCEPTIONS.0.md) names the exceptions row
# in its commit and raises the ceiling by hand.
#
#   scripts/vet-copies.sh [-m "dir dir …"]   default: every go.work module
#   UPDATE=1 scripts/vet-copies.sh           lower the ceilings to the counts
#   VERBOSE=1 scripts/vet-copies.sh          print every non-test finding
set -uo pipefail
cd "$(dirname "$0")/.."

ceilings=tools/vetcopies/ceilings.tsv
modules=""
while [ $# -gt 0 ]; do
  case "$1" in
    -m) modules=$2; shift 2 ;;
    *) echo "usage: scripts/vet-copies.sh [-m \"dir dir …\"]" >&2; exit 2 ;;
  esac
done
all_modules=$(awk '/^use \(/{f=1; next} /^\)/{f=0} f {sub(/^[ \t]*\.\//, ""); print}' go.work)
[ -n "$modules" ] || modules=$all_modules
if [ "${UPDATE:-}" = "1" ] && [ "$(echo $modules)" != "$(echo $all_modules)" ]; then
  echo "vet-copies: UPDATE=1 takes every module (drop -m)" >&2; exit 2
fi

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
tool="$tmp/vetcopies"
if ! ( cd tools/vetcopies && go build -o "$tool" . ); then
  echo "vet-copies: cannot build tools/vetcopies" >&2; exit 1
fi

# vet_one <module dir> — every line go vet prints for the module, in
# $tmp/<label>.out; go vet exits non-zero whenever it reports, so the exit
# status says nothing and the lines are classified below instead.
vet_one() {
  local dir=$1 label
  label=$(echo "$dir" | tr / _)
  ( cd "$dir" && go vet -vettool="$tool" ./... ) > "$tmp/$label.out" 2>&1
  if [ "$dir" = wpg ]; then # the wasm entry point builds only for js/wasm
    ( cd "$dir" && GOOS=js GOARCH=wasm go vet -vettool="$tool" ./wasm/ ) >> "$tmp/$label.out" 2>&1
  fi
  return 0
}
export -f vet_one
export tmp tool
printf '%s\n' $modules | xargs -P "$(nproc 2>/dev/null || echo 4)" -I{} bash -c 'vet_one "$1"' _ {}

# normalise <file> — one line per diagnostic: copylocks quotes a func
# literal's source text into its message, so a finding can span lines, and
# those lines are joined onto the diagnostic they continue; the go command's
# own "go: downloading" notes are dropped.
normalise() {
  awk '
    /^go: downloading / { next }
    /^#/ { if (cur != "") print cur; cur = ""; print; next }
    /^[^ \t#][^ \t]*\.go:[0-9]+:[0-9]+: / { if (cur != "") print cur; cur = $0; next }
    { if (cur != "") { sub(/^[ \t]+/, ""); cur = cur " " $0 } else print }
    END { if (cur != "") print cur }
  ' "$1"
}

# changed_lines <module> — "file first last" for every run of lines added or
# changed against HEAD in the module's Go files, untracked files whole.
changed_lines() {
  git -C "$1" diff -U0 --relative HEAD -- . 2>/dev/null | awk '
    /^\+\+\+ / { f = substr($2, 3); next }
    /^@@ / { split($3, a, ","); s = substr(a[1], 2); n = (a[2] == "" ? 1 : a[2]); if (n > 0) print f, s, s + n - 1 }'
  git -C "$1" ls-files --others --exclude-standard 2>/dev/null | grep '\.go$' | sed 's/$/ 1 999999999/' || true
}

ceiling() { # ceiling <module> <column 2|3> — the recorded ceiling, 0 without a row
  [ -f "$ceilings" ] || { echo 0; return; }
  awk -F'\t' -v m="$1" -v c="$2" '$1 == m { print $c; found = 1 } END { if (!found) print 0 }' "$ceilings"
}

failed=0
rows=""
printf '%-16s %8s %8s %8s %8s\n' module value ceiling bulk ceiling
for m in $modules; do
  out="$tmp/$(echo "$m" | tr / _).norm"
  normalise "$tmp/$(echo "$m" | tr / _).out" > "$out"
  # A diagnostic is "path.go:line:col: message"; "#" lines name packages.
  diags=$(grep -E '^[^ #][^ ]*\.go:[0-9]+:[0-9]+: ' "$out" | sort -u || true)
  nontest=$(echo "$diags" | grep -v '_test\.go:' | grep . || true)
  value=$(echo "$nontest" | grep -c 'core/go\.noCopy' || true)
  bulk=$(echo "$nontest" | grep -c 'bulk copy of \[\]core\.Value' || true)
  locks=$(echo "$diags" | grep -v 'core/go\.noCopy\|function variable aliases' | grep 'lock' || true)
  aliases=$(echo "$nontest" | grep 'function variable aliases' || true)
  other=$(echo "$diags" | grep -v 'core/go\.noCopy\|bulk copy of \[\]core\.Value\|lock\|function variable aliases' || true)
  stray=$(grep -vE '^[^ #][^ ]*\.go:[0-9]+:[0-9]+: |^#|^$' "$out" || true)
  vc=$(ceiling "$m" 2); bc=$(ceiling "$m" 3)
  verdict=""
  [ "$value" -le "$vc" ] || verdict="$verdict value $value > $vc;"
  [ "$bulk" -le "$bc" ] || verdict="$verdict bulk $bulk > $bc;"
  shown=$verdict; [ "${UPDATE:-}" != "1" ] || shown=""
  printf '%-16s %8d %8d %8d %8d %s\n' "$m" "$value" "$vc" "$bulk" "$bc" "${shown:+FAILED:$shown}"
  if [ -n "$verdict" ] && [ "${UPDATE:-}" != "1" ]; then
    failed=1
    # The new copies are on the lines that changed: list the findings on
    # the module's lines added or changed against HEAD (VERBOSE=1 lists
    # every finding).
    changed_lines "$m" > "$tmp/ranges"
    hits=$(echo "$nontest" | grep . | awk 'NR == FNR { lo[$1] = lo[$1] " " $2; hi[$1] = hi[$1] " " $3; next }
      { split($0, p, ":"); f = p[1]; sub(/^\.\//, "", f); n = split(lo[f], l, " "); split(hi[f], h, " ")
        for (i = 1; i <= n; i++) if (p[2] + 0 >= l[i] + 0 && p[2] + 0 <= h[i] + 0) { print "    " $0; break } }' "$tmp/ranges" - || true)
    if [ -n "$hits" ]; then
      echo "  findings on lines changed against HEAD:"; echo "$hits"
    else
      echo "  no finding is on a line changed against HEAD; VERBOSE=1 lists every finding"
    fi
  fi
  if [ -n "$locks" ]; then
    failed=1
    echo "  FAILED: copylocks reports a copy of a lock other than core.Value's marker:"
    echo "$locks" | sed 's/^/    /'
  fi
  if [ -n "$aliases" ]; then
    failed=1
    echo "  FAILED: a function variable aliases a function that takes a *core.Value; calls through it"
    echo "  are indirect, so every &v handed to them escapes to the heap. Declare a wrapper func:"
    echo "$aliases" | sed 's/^/    /'
  fi
  if [ -n "$other$stray" ]; then
    failed=1
    echo "  FAILED: go vet printed lines that are neither a counted copy nor a package header:"
    printf '%s\n%s\n' "$other" "$stray" | grep . | sed 's/^/    /' | head -40
  fi
  if [ "${VERBOSE:-}" = "1" ]; then echo "$nontest" | grep . | sed 's/^/    /'; fi
  rows="$rows$m	$value	$bulk
"
done

if [ "${UPDATE:-}" = "1" ]; then
  if [ "$failed" = "1" ]; then echo "vet-copies: not updating the ceilings while a module fails" >&2; exit 1; fi
  {
    echo "# The no-copy ratchet's ceilings (scripts/vet-copies.sh): per go.work module,"
    echo "# the NON-TEST copies of a core.Value that copylocks reports (value) and the"
    echo "# copy() and variadic append() calls over a []core.Value (bulk). They only go"
    echo "# down: UPDATE=1 lowers them to the counts; a sanctioned new copy raises one"
    echo "# by hand, naming its row in design/VALUE-NO-COPY-EXCEPTIONS.0.md."
    printf 'module\tvalue\tbulk\n'
    printf '%s' "$rows" | while IFS=$'\t' read -r m value bulk; do
      [ -n "$m" ] || continue
      vc=$(ceiling "$m" 2); bc=$(ceiling "$m" 3)
      if [ -f "$ceilings" ] && grep -q "^$m	" "$ceilings"; then
        [ "$value" -lt "$vc" ] || value=$vc
        [ "$bulk" -lt "$bc" ] || bulk=$bc
      fi
      printf '%s\t%s\t%s\n' "$m" "$value" "$bulk"
    done
  } > "$tmp/ceilings.new"
  mv "$tmp/ceilings.new" "$ceilings"
  echo "vet-copies: ceilings written to $ceilings"
  exit 0
fi
[ "$failed" = "0" ] || { echo "vet-copies: FAILED (see above)" >&2; exit 1; }
echo "vet-copies: every module at or under its ceilings"
