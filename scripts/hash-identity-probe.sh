#!/usr/bin/env bash
# hash-identity-probe — is `canon` a sound basis for content hashing?
#
# Usage:  scripts/hash-identity-probe.sh
#
# `design/legacy/unison-in-boru-report.0.ignore` proposes deriving a content hash from
# the ADR-015 canon contract and using it as a definition's identity. This
# wrapper runs the in-language battery (scripts/hash-identity-probe.boru)
# and adds the two checks a single boru process cannot make for itself:
#
#   P2v cross-process re-parse — canon a function, feed the rendering back
#       to a fresh process, and check the result still behaves.
#   P9  cross-process determinism — the same expression hashed in two
#       separate processes must produce the same digest.
#   P10 cross-port canon parity — Go and TS must render a value to the
#       same bytes, or the two engines disagree about identity.
#
# Findings are written up in `design/legacy/unison-hash-identity-probe.0.ignore`.
# A FAIL is not a bug report against canon: canon meets its own contract.
# It records a property that content hashing needs and ADR-015 does not
# supply.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
boru="$repo/cmd/go/bin/boru"
[ -x "$boru" ] || { echo "build the binary first: (cd cmd/go && make build)" >&2; exit 1; }

# --- the in-language battery (P1-P8) ------------------------------------
"$boru" --no-compile "$repo/scripts/hash-identity-probe.boru"

# --- P2v: does a fn's canon really re-parse to the same function? --------
# The in-language battery can print the rendering but cannot eval text, so
# the round-trip is derived here: render, re-parse, apply, compare.
echo
echo "== P2v: fn canon re-parse — render, re-parse, apply =="
orig=$("$boru" do --no-compile 'def g (fn [[x:Number][Number][mul x x]]) g 5' 2>/dev/null | tail -1 || true)
rendered=$("$boru" do --no-compile 'canon (fn [[x:Number][Number][mul x x]])' 2>/dev/null | tail -1 || true)
echo "   original applied to 5 = $orig"
echo "   canon                 = $rendered"
rt=$("$boru" do --no-compile "def f ($rendered) f 5" 2>/dev/null | tail -1 || true)
if [ -n "$orig" ] && [ "$rt" = "$orig" ]; then
  echo "PASS  re-parsed fn behaves identically ($rt)"
else
  echo "FAIL  re-parsed fn does not behave identically (got '${rt:-<error>}', want '$orig')"
fi
again=$("$boru" do --no-compile "canon ($rendered)" 2>/dev/null | tail -1 || true)
if [ "$again" = "$rendered" ]; then
  echo "PASS  canon reaches a textual fixpoint"
else
  echo "FAIL  canon does not even reach a fixpoint — each pass adds a layer:"
  echo "   pass 2 = $again"
fi

# --- P9: cross-process determinism --------------------------------------
echo
echo "== P9: cross-process determinism — one expression, two processes =="
digest() {
  "$boru" do --no-compile "import \"boru:bin-util\" BinUtil.fnv64 (canon $1)" 2>/dev/null | tail -1
}
for expr in '{a:1 b:2}' '[1 2 3]' "'text'" '(context)'; do
  a="$(digest "$expr")"; b="$(digest "$expr")"
  if [ "$a" = "$b" ]; then printf 'PASS  %-12s %s\n' "$expr" "$a"
  else printf 'FAIL  %-12s %s vs %s\n' "$expr" "$a" "$b"; fi
done
echo "   a Store carries live pointer addresses into its canon, so its digest"
echo "   is not stable across processes — it is not stable across runs at all"

# --- P10: cross-port canon parity ---------------------------------------
echo
echo "== P10: cross-port canon parity — Go and TS must agree byte for byte =="
if ! command -v node >/dev/null 2>&1; then
  echo "SKIP  node not on PATH"
  exit 0
fi
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# Floats only: they are where shortest-round-trip formatting differs between
# runtimes, and a hash turns a cosmetic difference into a different identity.
# Every case is written with a decimal point so it parses as a Float on
# the Go side: a bare 1e6 is an Integer, and coercing with `add 0.0` would
# silently turn -0.0 into 0.0 and measure the coercion, not canon.
cases='1.0 0.1 1.0e21 1.5e-7 0.30000000000000004 -0.0 1.7976931348623157e308 5.0e-324 100.0 1000000.0 0.5 123456789.123456789'

"$boru" do --no-compile "canon [$cases]" 2>/dev/null \
  | tail -1 | tr -d '[]' | tr ' ' '\n' > "$tmp/go.txt"

cat > "$tmp/ts.ts" <<EOF
import { canonValue } from "$repo/core/ts/src/canon.ts";
import { newFloat } from "$repo/core/ts/src/value.ts";
for (const c of [$(echo "$cases" | tr ' ' ',')]) console.log(canonValue(newFloat(c)));
EOF
node --experimental-strip-types --no-warnings "$tmp/ts.ts" > "$tmp/ts.txt" 2>/dev/null

if diff -q "$tmp/go.txt" "$tmp/ts.txt" >/dev/null; then
  echo "PASS  all $(wc -l < "$tmp/go.txt" | tr -d ' ') float renderings agree across ports"
else
  echo "FAIL  canon diverges across ports:"
  diff "$tmp/go.txt" "$tmp/ts.txt" | sed 's/^/   /'
fi
echo "   NOTE: every case is a decimal literal so both sides hold a Float."
echo "   Comparing unlike types here measures the test setup, not canon."

# Strings: quoting, unicode and escapes are the other place two runtimes
# drift. Floats alone do not license a claim about these. Non-ASCII is
# written as a literal character on BOTH sides — an escape form the two
# languages spell differently would measure the escape, not canon.
"$boru" do --no-compile "canon ['a\"b' 'é🔥' '' 'back\\\\slash' 'tab\tx' 'nl\nx']" 2>/dev/null \
  | tail -1 > "$tmp/go-s.txt"
cat > "$tmp/ts-s.ts" <<'TSEOF'
import { canonValue } from "REPOPATH/core/ts/src/canon.ts";
import { newString, newList } from "REPOPATH/core/ts/src/value.ts";
const ss = ['a"b', 'LITERAL', '', 'back\\slash', 'tab\tx', 'nl\nx'];
console.log(canonValue(newList(ss.map((s) => newString(s)))));
TSEOF
sed -i "s|REPOPATH|$repo|g; s|LITERAL|é🔥|" "$tmp/ts-s.ts"
node --experimental-strip-types --no-warnings "$tmp/ts-s.ts" > "$tmp/ts-s.txt" 2>/dev/null || true
if [ -s "$tmp/ts-s.txt" ] && diff -q "$tmp/go-s.txt" "$tmp/ts-s.txt" >/dev/null; then
  echo "PASS  string/unicode/escape renderings agree across ports"
  echo "   $(cat "$tmp/go-s.txt")"
else
  echo "FAIL  string canon diverges across ports (or the TS build failed):"
  echo "   go = $(cat "$tmp/go-s.txt")"
  echo "   ts = $(cat "$tmp/ts-s.txt" 2>/dev/null)"
fi
