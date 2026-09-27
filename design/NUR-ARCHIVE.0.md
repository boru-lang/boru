# The Non-Uniformity Register's archive

**Split out of [NUR.md](../NUR.md) on 2026-09-27**, when that file neared the
repository's 1 MiB file limit (`scripts/check-no-binaries.sh`). These are
the full bodies of records that are FIXED or RESOLVED: the divergence as it
was measured, the trace, and the fix. Each keeps its number and its
`{#nurNNN}` anchor; NUR.md keeps the record's heading, anchor and status
line, with a link here, so every existing link to `NUR.md#nurNNN` still
lands on the record. The register's rules (numbers never reused, a record's
status) are NUR.md's; this file holds text only.

## NUR026 — Escape sets diverge between quoted strings and templates {#nur026}

**Status:** FIXED 2026-09-26 (one escape vocabulary, one malformed-escape
report — the handoff log's entry of that date; the vocabulary was narrowed
2026-08-15) · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo
uniformity review

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

**Rule:** one escape vocabulary across string literal forms.
**Divergence:** quoted strings (`"…"`/`'…'`) accept jsonic's full
escape set (`\x41` → `A`, plus `\b`, `\f`, …); backtick templates
process only `\n \t \r \\ \` \$` — `size "z\x41z"` → 3 while the same
text in a template → 6 (the escape survives literally).
**Evidence:** `eng/go/parser/parse.go:1681-1714`
(`processTemplateEscapes`); `eng/go/parser/grammar.go:103-104,340-367`.
**Documentation status:** REFERENCE documents the restricted template
set but never states quoted strings accept a superset — the asymmetry
is undocumented.

**Root cause (source investigation, 2026-07-31):** the divergence is
an **implementation accident, not a design choice**. `setupBaseTokens`
(grammar.go:97-104) deletes the backtick from jsonic's `StringChars`
and `MultiChars` so jsonic's built-in string matcher never consumes
templates — necessary because templates need `${…}` interpolation,
which the plain string matcher cannot provide. That forced a
hand-rolled template scanner (grammar.go:340-367), whose
`processTemplateEscapes` reimplements escapes from scratch as a
minimal six-case switch (`\n \t \r \\ \` \$`) with everything else
falling through to "keep literally" — while quoted strings still ride
jsonic's native escape handling and get the full set. Templates were
severed from jsonic purely to bolt on interpolation, and the
replacement escape handler was never brought to parity.

**Verdict (maintainer, 2026-07-31 — resolve by fix,
`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):** boru shall **not** use the
jsonic JSON string lexer as-is for strings. Instead: a **custom
unified string lexer** — a vendored copy of jsonic's string lexer,
extended to also handle backtick templates (i.e. `${…}`
interpolation). One lexer then (1) preserves the full escape set
across every string-literal form (the rule this record seeks), (2)
makes string processing uniform — one escape vocabulary in exactly one
place — and (3) parses templates, interpolation included, correctly.
This retires the hand-rolled `processTemplateEscapes` path and its
minimal escape set. Stays Pending until the unified lexer lands.

**Verdict (maintainer, 2026-08-15 — resolve by fix):** **templates take
the full quoted-string escape set**, so one string syntax means one
escape vocabulary and `size "z\x41z"` and its template spelling agree.

Documenting the asymmetry was the cheaper option and was not taken: a
reader should not have to know which quoting form they are in to know
what `\x41` means. The narrowing direction (cutting quoted strings down
to the template set) was rejected outright — it removes working
spellings. The cost to watch and pin: a template containing a backslash
sequence that is inert today would start escaping, so the fix wants
rows for each newly-live escape and for the sequences that must stay
literal. Stays **Pending** until it lands.

### Resolved: the vocabulary (2026-08-15)

`writeStringEscape` (Go) / `readStringEscape` (TS) is now the single
escape vocabulary, and it is the quoted-string one, measured against
jsonic rather than assumed:

```
\n \t \r \b \f \v   the control characters
\xNN                one byte, two hex digits
\uNNNN              one rune, four hex digits
anything else       the character itself, backslash DROPPED
```

`size "z\x41z"` and its template spelling now agree. The last rule is
the behaviour change the verdict asked to pin: `\z` is `z` and `\0` is
`0` in a template exactly as in a quoted string, where both were
previously literal. The template-only spellings need no case of their
own — `\``` and `\$` fall into the default arm and yield the bare
character, which is what they always meant.

Pinned in `parser/spec/parse.tsv` (both ports, no shared code) for the
newly-live escapes and the flipped unknown-escape row, and in the two
ports' unit tests for `\b` / `\f` / `\v`, whose canon carries a raw
control byte no single TSV line can hold.

**The migration cost is real, and REGEX is where it lands.** A regex
written in a template is the common case of "a backslash sequence that
was inert": `\s`, `\[`, `\(`, `\?`, `\]` all used to survive to the
regex engine and now lose their backslash at parse time. Every
backslash a regex needs must be written DOUBLED in a template — `\\s`,
not `\s` — exactly as it already had to be in a quoted string.

The tree's own sources were the proof: a repo-wide scan for templates
whose meaning changes found **two lines, both in
`lang/go/modules/sift.boru`** — the size-suffix matcher (`\s`) and the
pattern tokenizer (`\[`, `\(`, `\?`, `\]`) — and both broke loudly
(`TestSiftBoruCoverage`: 13 failures, `error parsing regexp`) rather
than silently. Both are migrated in the same commit. The loudness is
the mitigating fact: a mangled regex fails to compile, so this is not
the class of change that quietly returns wrong answers.

### What REMAINS open — malformed input

A malformed `\x` / `\u` (too few digits, or a non-hex digit) is
reported differently by the two forms:

```
"a\xZZb"    ERROR: the escape sequence … does not encode a valid ASCII character
`a\xZZb`    'axZZb'   — the literal reading, no error
```

The VOCABULARY is uniform; what a well-formed escape means no longer
depends on the quoting form, which is the divergence this record was
opened for. What is left is error REPORTING, and closing it needs an
error channel the call site does not have: the template path is a
jsonic `LexMatcher` returning a `*Token`, so raising means changing the
lexer seam — the unified-lexer work the 2026-07-31 verdict sketched and
the 2026-08-15 verdict did not ask for. Recorded rather than silently
accepted, with a spec row pinning the residual.

### The fix (2026-09-26)

The seam was never closed: a tabnas `LexMatcher` may return a BAD token
(`#BD`, its `Why` the code), which is how jsonic's own string lexer
reports. So boru owns the escape check in both ports, with ONE definition
(`escapeFault`): `\x` needs two hex digits; `\u` four, or the braced
`\u{…}` form of 1–6 digits up to `10FFFF`; the reported span is the escape,
clipped at the form's closing delimiter.

- A template's literal matcher refuses a malformed escape with it
  (`` `a\xZZb` `` → `invalid ascii escape: \xZZ`).
- A new `string_escape` matcher runs ahead of jsonic's string lexer and
  refuses a malformed escape in `"…"` / `'…'` the same way; a well-formed
  or unterminated string, or a raw control character, stays jsonic's. That
  closed a cross-port split in the quoted form too (NUR229).
- Measuring the vocabulary across forms found it NOT fully unified: the
  braced `\u{…}` form was live in a quoted string only, and Go's shared
  writer decoded a surrogate pair split across two escapes as two U+FFFD
  where TS joined it (NUR230). `writeStringEscape` / `readStringEscape`
  now read both, as jsonic does.

Pinned: 34 `parse.tsv` rows (the escape matrix over all three forms,
positive and malformed, in both runners) and the residual row flipped;
`TestTemplateWave3Escapes`, `TestTemplateWave3MalformedEscapes`, the
direct `processTemplateEscapes` cases. REFERENCE.md §"String escapes"
states the one vocabulary and the report.

---

## NUR079 — Gated words inside an imported file-module body escape the policy that governs the same call at top level {#nur079}

**Status:** FIXED 2026-09-26 · **Recorded:** 2026-08-18 · **Surfaced by:**
the Roc comparison study (`design/legacy/roc-in-boru-report.0.ignore` §7.1),
while checking Roc's claim that `roc check`/`roc build` perform no
dependency I/O

**The fix (half (ii), 2026-09-26).** `loadFileModule` applies the natives
path's checks (`checkFileModuleImport`): `modules.import` with `{module:
<ref>, kind: "file"}` — Check's own first step refuses an uninstalled
modules scope — and the module's own subscope `install:false`, keyed on
the ref it is loaded under, the key its NUR045 per-export gates already
carry. The native path now supplies `kind: "native"` — a where-predicate on
an ABSENT arg passes vacuously, so without it a `kind: ["file"]` admission
would have admitted every native module (caught in the first measurement:
`boru:net` imported under `sandbox`). The restrictive built-ins (`sandbox`
and what extends it, `compute`, `gen`) admit file modules with `{ allow:
["import"], where: { kind: ["file"] } }`: a body runs under the importer's
profile (half (i)), so the import widens nothing, and reading the file
stays the `fileops` scope's call — so multi-file programs run under
`read-only` and `client` exactly as before. Both paths' refusals are CODED
(`PolicyRefusal`: `permission_denied` / `capability_not_installed`), so `do
[import …] error [dot code]` tells a refused import from a broken one.
`boru check` takes the permission flags (and BORU_POLICY), the run's
pre-flight resolves the policy FIRST and checks under it (the double
execution of (b) is now gated), `boru build`'s pre-flight likewise, and
`boru describe` and the language server honour the environment policy.
The check pass no longer degrades a refused top-level import to an opaque
module: it records the refusal as the top-level trap the run raises and
reports it as an error-severity mirror, so the compiled program raises the
same coded error at the same caret. (A refused import whose module the
program goes on to use still declines to compile — the check reads the
names after the trap as undefined, the limit `raise "x" foo` shares — but
loudly.) Pinned: lang `TestNUR079FileModuleImportPolicy`,
`TestNUR079CheckReportsRefusedImport`; cmd `TestCheckHonoursPermissionFlags`,
`TestPreflightRunsUnderPolicy`, `TestDescribeHonoursEnvironmentPolicy`,
`TestComputeDiagnosticsEnvPolicyFailure`. Documented in CLI.md
§Per-command policy flags and design/PERMISSIONS.10.md.

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

**Rule:** one policy decision per gated operation, wherever it occurs.
`CLI.md` §Permissions and `EXPLANATION.md` §Capabilities both present a
profile as governing *the program*, and `boru policy explain` is
documented as answering "why was this denied?" for any call.

**Divergence:** the same gated call is refused at top level and permitted
one file deeper. With `lib.boru` containing `import "boru:net"` plus a
`Net.fetch`, and `main.boru` containing `import "./lib.boru"`, measured
against a local listener:

```
boru run --no-check --perms read-only direct.boru   exit 1  requests 0
boru run --no-check --perms read-only main.boru     exit 0  requests 1
boru run            --perms read-only main.boru     exit 0  requests 2
```

Three separate divergences sit in that table. (a) `modules.import` is
enforced for a top-level `import "boru:net"` and not for the identical
import inside a file-module body, so a profile is bypassed by moving the
import one file deeper; `--deny=network.connect` behaves the same way,
refusing the top-level fetch and permitting the in-body one. (b) The
pre-flight check pass executes the body a *second* time, so even a
correctly-gating profile would already have leaked. (c) The analysis
commands are unreachable by policy at all: `boru check` registers no
`--perms` flag (`cmd/go/internal/check/check.go` contains the string
`perms` zero times) and ignores `BORU_POLICY`, which `REFERENCE.md`
documents as an environment fallback and which works on `boru do`. The
same execution occurs under `boru describe ./lib.boru`, under a piped
`boru repl`, and on LSP `didOpen`.

**Evidence:** the `module_body_executed_in_check` info advisory already
records half of it — "a network send and a stdin read are not modelled and
still do" — so the *execution* is designed and known; what is not designed
is that no policy can reach it. `design/MODULE-SECURITY.0.md`'s per-edge
attenuation argument is written about boru-source dependencies, for which
there is no per-import gate today. The natives path does gate: compare
`modules.Resolve`'s `Installed` / `Check("modules","import",…)` /
per-module `install:false` sequence with `loadFileModule`, which applies
none of them.

**Mechanism** (identified in the PR #384 review, then confirmed in the
tree): `runModuleBodyCover` (`lang/go/native/native_module_module.go`)
builds a fresh sub-registry for the body and deliberately inherits
`Output`, `ErrOutput`, `Input`, the effect ledger, observe hooks, runtime
stamping, `HostFileOps`, `CapMemFileOps`, every `ModuleInheritedCaps` seam,
host formats and extensions, `ParseFunc`, `BaseDir`/`BaseFile` and the TCO
switch — **and no policy**. Every gate then resolves `HostPolicy(r)` on the
registry it is running on and treats a nil policy as allow
(`checkFetchPolicy`: `if pol == nil { return nil }`), so the body executes
ungated no matter what the parent's profile says. This is why gating the
import alone would not discharge the record.

**Verdict:** resolve by fix, in two halves. (i) The module sub-registry must
carry the parent's policy — attenuated, never widened, the way
`Vm.run-sandbox` already attenuates — or a gated word inside a body stays
ungated. (ii) The file-module import path should apply the same three checks
the natives path applies, keyed on the declared ref
(`modules.scopes."./lib.boru"` is already a live policy key, as the NUR045
per-export gate's blame string shows), and the analysis commands should
accept the permission flags they already document. Ship the gate together
with updated built-in profiles, since `sandbox`, `read-only` and `compute`
all set `modules.words.default: deny` and would otherwise refuse every
multi-file program. This record retires when a profile denies an in-body
gated call and `boru check --perms <profile>` is honoured.

**Progress (2026-08-18): half (i) is landed.** `runModuleBodyCover` now
carries the parent's policy into the module sub-registry
(`SetHostPolicy(modReg, HostPolicy(parent))`), placed AFTER the `SetHostX`
inheritance because those hooks auto-wrap with `HostPolicy(r)` and
installing it first would wrap the parent's already-permissioned backend a
second time. Same policy, not a widened one, which is what the verdict's
"attenuated, never widened" requires here — the body declares no policy of
its own, so there is nothing to compose against. Measured against a local
listener, `read-only` before and after:

```
before   nested import "boru:net" + Net.fetch   exit 0, server logged the request
after    nested import "boru:net" + Net.fetch   exit 1, permission denied: modules.import, 0 requests
```

Regression test: `lang/go/native/module_policy_nur079_test.go` — a positive
(the body's sub-registry carries the policy AND a policy-resolving gate
refuses through it) and a negative (an unconfigured parent leaves the body
unrestricted, so inheriting never manufactures a policy). Verified to FAIL
with the fix removed.

**Correction to the Mechanism paragraph above.** "The body executes ungated
no matter what the parent's profile says" is too broad, and the narrower
truth is why this was easy to miss. The capability-wrapping gates were
never in the hole: `HostFileOps` is inherited by pointer and the parent
already wrapped it (`SetHostFileOps` applies `NewPermissionedFileOps` when
a policy is present), so a file write inside a module body was refused
correctly all along. The hole was confined to gates that resolve
`HostPolicy(r)` at dispatch — `modules.import` and the network words among
them. Measured on the pre-fix binary under `read-only`: an in-body
`IO.write` was denied, while an in-body `Net.fetch` succeeded.

**Still open:** half (ii) — the file-module import path applying the
natives path's three checks, and the analysis commands accepting the
permission flags they document. The record stays Pending until both land.

## NUR100 — ADR-016 forbids arity-keyed exceptions; two live sites use them {#nur100}

**Status:** FIXED 2026-09-26 (the predicate as a one-value application —
the handoff log's entry of that date) · **Recorded:** 2026-08-25 ·
**Surfaced by:** the maintainer's ruling, 2026-08-25, that ADR-016's rule is
absolute — "everything everywhere every time and always"

**The fix.** Each site now keys on the thing its count stood in for, and
neither reads an arity.

*§1 — the predicate role is a ONE-VALUE APPLICATION.* The replacement
contract the record asked for is the one every call already has: the
candidate is matched against the predicate's signatures by `MatchFnSig`
(types in signature order, then value patterns), and the signature that
takes it runs. A candidate no signature takes is not a member — without
running a body, which is what the old input-type gate did for the first
signature alone. Three consequences, each the uniform answer:

- the WHOLE overload set is consulted, first match first, where only the
  first overload used to be read — `fnpred [[n:Integer] […] [s:String] […]]`
  admits `"ab"` through its second overload on `is`, a typed def and a typed
  parameter alike (`PredicateInputType`, the typed slot's pre-filter and the
  node's parent, is now the overloads' COMMON input, nil when they differ);
- a value pattern selects as for any call — `fnpred [[0] [true]]` admits 0
  and refuses 1, where the type-only gate ran the body over 1 and admitted it;
- a predicate none of whose signatures can take one value is a type no value
  inhabits: `def K fnpred [[a:Any b:Any] [a]]  def v:K 5` answers `does not
  satisfy predicate type K` (statically, too — the check pass's run of a
  concrete candidate reaches the same no-match), and `4 is K` false, where
  the use raised `RunPredicate: predicate must take exactly one argument`.

The declaration is NOT refused: a refusal would be a count again, and a
predicate no value satisfies is already expressible (`fnpred n:Integer
[false]`) and accepted.

*§2 — the poly decline keys on a declared re-step.* `smallerArityOverload`
is gone. The hazard it guarded was never the count: the VM's poly re-match
already retries narrower windows over the same stack top (NUR147), and what
it cannot reproduce is a dispatch whose result RE-STEPS on the tape — the
poly op pushes the handler's results, where `apply`'s `[Function]` overload
marks the fn and steps it over the values beneath. tryRecordPoly now
declines when an overload DECLARING `CompileResteps` is reachable over the
dynamic operands (`restepOverloadReachable`: each operand the overload reads
is an Any carrier or gradually matches its slot); the corpus's declines are
the same `apply` windows (measured: every one had a dynamic Any lead), and
a window whose lead cannot be a Function no longer declines on arity alone.

The aritygate tightened with both: `core/go/registry.go` 3→2 and
`compiler/go/compiler_dispatch_record.go` 2→1, their "NAMED DIVERGENCE"
blocks retired. (It also caught an arity read of my own in the same run —
`lower.go`'s landing skip compared an apply's `NArgs` it already had as its
operand count — removed.) Found on the way and fixed: NUR272 and NUR273.
Pinned: lang
`TestNUR100PredicateIsAOneValueApplication` (seven exact rows on both lanes
and four refusals, none an arity error), compiler
`TestRestepOverloadReachable`, `lang/spec/fnpred.tsv` §8.

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

**2026-09-25, after NUR099's close:** the arity route INTO the predicate
branch is gone — only `fnpred` declares a predicate — so §1's count now
judges only a DECLARED predicate of the wrong arity: `def K fnpred [[a:Any
b:Any] [a]] end def z:K 5` still raises `RunPredicate: predicate must take
exactly one argument` at the use, and `4 is K` answers false. Still no
verdict on whether that declaration should be refused where it is written.

**Rule:** ADR-016 — *"Every function behaves the same way whatever its arity
and wherever it came from … this record forbids exceptions keyed on arity or
origin."* Accepted 2026-08-15. The two exceptions it named as defects were
fixed; the rule itself is unconditional.

**Divergence:** two live sites decide behaviour by counting parameters.

1. **`core/go/registry.go` `RunPredicate`** — whether a function may act as a
   predicate at all is decided by its parameter count:

   ```
   error: predicate type K: RunPredicate: predicate must take exactly one argument
   ```

   A semantic exception: two functions that both express a membership test
   are admitted or refused on arity alone.

2. **`compiler/go/compiler_dispatch_record.go` `smallerArityOverload`** — a
   poly window over dynamic operands is refused compilation when the word
   registers an overload consuming FEWER operands. Lower stakes only in that
   no answer changes — the refusal is a compile-coverage defect, absorbed
   meanwhile by the interpreter re-running the program — but the same shape,
   and introduced recently in PR #401.

**A THIRD SITE existed and was not on this list; it is now gone (2026-08-28).**
The compiler's `lambdaCallbackInputs` admitted a list `each` callback and
refused list `fold`/`scan`, under a rule its own note called THE ARITY-1
BOUNDARY: "at one input no convention can disagree, at two they can". Arity
deciding what compiles is the same shape as sites 1 and 2, in the admission
lane rather than the dispatch lane, and it survived unrecorded because it read
as a compile-coverage limit rather than a semantic exception.

It was also wrong on the facts. The two containers disagree because the MAP
path binds POSITIONALLY (`CallBoruFn`, args in sig order) and the LIST path
runs its inputs as a STACK (`InvokeBody` → `RunResolved`, `MatchSignature`
filling top-down) — a per-path convention, not an arity threshold. One
per-word permutation at the closure bind (`ClosureInStackPair`) reconciles
them, and the rows compile at both arities with no boundary anywhere. Five
ledger rows graduated with it (design/FULL-COMPILATION.0.md §6.3).

This site was found while fixing something else, not by looking for
arity-keyed rules, which is the register's own weakness rather than an
accident. **`test/go/aritygate` now counts them** (2026-08-28): it flags every
comparison against the arity of a function being INSPECTED — `len(sig.Params)`,
`x.Arity`, `sig.TotalArgs()` — and pins the census per file, so a new site
fails the build and a retired one must be tightened away.

Two design notes on it, because a gate that cries wolf gets disabled. It does
NOT flag a handler bounds-checking the args IT received (`len(args) < 2`): a
function reading its own declared positions is not an exception to anything,
and an earlier draft that counted those found 298 sites across 100 files. And a
PIN IS NOT AN ACCUSATION — most of the 148 pinned sites are the matcher and its
machinery reading arities in order to MATCH a signature, which IS the argument
rule. The pins exist so a CHANGE forces someone to say which kind it is. Sites
1 and 2 above are marked in the table as the named divergences they are.

Verified against a deliberate violation before landing: adding
`len(s.Params) == 1` to a pinned file fails the gate, and removing it restores
green.

**No verdict on sites 1 and 2** (at recording; the fix above is the verdict the run took). The predicate role does need to test ONE
value, so removing site 1's gate needs a replacement contract, not a deletion
— and naming that contract is a design call the register should not pre-empt.
Recorded so the divergence between an accepted ADR and the code is not lost;
the fix is the maintainer's to direct.

---

## NUR101 — BROAD places a REFERENCED fn but still dispatches a COMPUTED one {#nur101}

**Status:** FIXED 2026-09-25 (the handoff log's entry of that date): the
verdict of 2026-08-27 — resolve by fix, compiler-side only — LANDED that
day (the `ParenPlacedFnIDs` / `ParenReSteppedFnIDs` pair read at the
collapse, below), and the four shapes it left refusing graduated on
2026-09-22 with the curried chain (the inner paren records its
re-stepped lead's apply at the collapse, so `[((mk 1) 2)]` assembles one
element and is `[[3]]` on both lanes). Measured today: `(mk 1) 2` places
(`fn (Integer) 2`) and `((mk 1) 2)` re-steps (3) on both lanes, `def h
(mk 1) end  h 2` is 3, and the standing measurement
(`lang/go/nur101_paren_restep_test.go`: `TestParenReStepRule`,
`TestParenReStepPlacedLayoutCompiles`,
`TestParenReStepListElementCompileFailure` — a parity pin since its
graduation) passes. Nothing of this record is open; it kept a Pending
status only because the register was not updated when the last shape
graduated. **Recorded:** 2026-08-25 · **Surfaced by:** re-measuring
`design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` §5.4 against the post-#402 tree; the
diagnosis below is the maintainer's correction of this record's first version

**Rule:** ADR-011's 2026-08-24 amendment, implementing NUR073's BROAD verdict
— *"a paren places its collapsed Function value, **reference and inline
literal alike**, so the inline-application idiom … is removed and application
is explicit — a bare name, `apply`, or a member read."*

**Divergence:** it holds for a reference and for an inline literal. It does
NOT hold when the group COMPUTES the function.

```
$ boru do 'def inc fn b:Integer Integer [add 1 b] end
           def mk fn a:Integer Function [(fn b:Integer Integer [add a b])] end
           …'

(fn Integer [Integer] [10 add]) 7      → fn (Integer) 7      ← inline literal: places ✓
(inc/v) 2                              → fn inc(Integer) 2   ← reference: places ✓
(valof inc) 2                          → fn inc(Integer) 2   ← reference: places ✓
(mk 1) 2                               → 3                   ← COMPUTED: dispatches ✗
(if true [inc/v] [inc/v]) 2            → 3                   ← COMPUTED: dispatches ✗
```

Three of the five obey the rule; the two whose group computes a Function
apply it inline — exactly the idiom BROAD removed. `(fn …) 7` is ADR-011's
own worked example and answers correctly, which is what makes the computed
case a gap in the fix rather than a disagreement about the rule.

**This is the unfixed remainder of NUR073**, whose record is deleted (it was
Resolved 2026-08-24, and this file does not keep Resolved records). That fix
closed the reference and literal halves; the computed half is this record,
and it is why the number is worth citing rather than forgetting.

**One known downstream symptom, and it is where this was found.** Inside a
list literal the two lanes then disagree, silently:

```
def mk fn a:Integer Function [(fn b:Integer Integer [add a b])] end [((mk 1) 2)]

interpreted → [3]                  ← carries the bug into container evaluation
compiled    → [fn (Integer) 2]     ← BROAD-correct
```

Exit 0 both ways, no warning, `boru check` clean. The COMPILED answer is the
specified one here; fixing the placement rule fixes the divergence with it,
and no compiler change is wanted. (This record's first version had it exactly
backwards — it named the compiled lane defective and a refusal gate was
written against that reading. Both were reverted. The argument that misled it
was `[inc 2]` → `[3]`, which proves nothing: ADR-011 explicitly carves out
*"a bare WORD inside a group, which dispatches during the group's own
evaluation"*. Raised on PR #403.)

**Re-measured 2026-08-26 (against `df0edb5`), and the divergence has
NARROWED.** The transcript above is stale: every one of its five placement
cases now places, including the two it calls defective —

```
(mk 1) 2                     → fn (Integer) 2      this record says: 3
(if true [inc/v] [inc/v]) 2  → fn inc(Integer) 2   this record says: 3
```

— and the guard still holds (`def h (mk 1) end  h 2` → `3`). So the
top-level half of this record is CLOSED.

What survives is narrower, and it is not about list literals: placement now
depends on whether the application sits inside an ENCLOSING GROUP.

```
(mk 1) 2      → fn (Integer) 2     places
((mk 1) 2)    → 3                  dispatches      ← the whole remaining divergence
```

The list literal merely inherits it, because its contents evaluate as a
sub-program in which the inner form IS `((mk 1) 2)`; that is why
`[((mk 1) 2)]` is `[3]` interpreted and `[fn (Integer) 2]` compiled. The map
form agrees on both lanes because the compiled lane falls back there.

Note also that the compiled lane is NOT uniformly at the placing answer:
`lang/go/bytecode_curried_test.go:17-24` pins compiled `((mk 1) 2)` as `[3]`.
So the enclosing-group case diverges from the unwrapped case on BOTH lanes,
and closing it is not an interpreter-only change.

The open question is therefore single: **does a COMPUTED function applied
inside an enclosing group place, or dispatch?** ADR-011's carve-out is
written for *"a bare WORD inside a group"*, and `(mk 1)` is not a bare word,
which is how this register came to hold two contradictory readings (see
below). Options, costs and a recommendation:
[design/legacy/O1-RELITIGATION.0.ignore](legacy/O1-RELITIGATION.0.ignore).

**RULED 2026-08-26 — place uniformly.** A computed function applied inside
an enclosing group PLACES, exactly as its unwrapped twin does. `((mk 1) 2)`
becomes `fn (Integer) 2`, and `[((mk 1) 2)]` becomes `[fn (Integer) 2]` on
both lanes. There is no enclosing-context exception: ADR-011's carve-out
stays what it says, a bare WORD inside a group, and a computed group result
is not one.

Both lanes move. The compiled lane is NOT already at this answer for the
enclosing-group case — `lang/go/bytecode_curried_test.go:17-24` pins compiled
`((mk 1) 2)` as `[3]` — so the fix is interpreter AND compiler, and that
fixture is rewritten with it. `design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` §5.4's
`((mk 1) 2)` → `3` transcripts and its `def h (mk 1)` / `2 h/v apply`
workaround are re-spelled, as every §1 program was when BROAD's first half
landed. `def h (mk 1) end  h 2` → `3` must keep working: a bare NAME bound to
a function calls by rule, and that rule is untouched.

The superseded second record under this number — "a paren-computed fn inside
a list literal is applied interpreted, baked compiled", which read the
compiled lane as the defect — is DELETED as of this ruling, per the
register's own discipline for superseded records. Its content is subsumed:
the list literal was never the subject, it merely inherits the wrapped form.

**Verdict:** resolve by fix — extend BROAD's placement to a computed group
result, so `(mk 1) 2` is `fn 2` like its reference and literal twins. Two
things to check when it lands: the top-level `((mk 1) 2)` → `3` transcripts
in §5.4 and its `def h (mk 1)` / `2 h/v apply` workaround were written against
the unfixed behaviour and will need re-spelling, exactly as every §1 program
was when BROAD's first half landed; and `def h (mk 1)  h 2` → `3` must keep
working, since a bare NAME bound to a function calls by rule.

---

**THE RULING'S PREMISE IS FALSIFIED — measured 2026-08-27.** The ruling
above stands as a statement of intent about the LANGUAGE; what it got wrong
is the claim that the interpreter needed to change to reach it.

The first implementation deleted `fnReturnPark`'s survivor-count clause
(`closeIdx != idx+2`), on the reasoning that "the survivor count was never
the right question". It is. Deleting it turned
`(x:Integer => [x mul 2] 5)` from `10` into `fn (Integer) 5` and broke seven
suites. The clause is reinstated.

The rule, as MEASURED against `RunInterp` rather than assumed:

> A Function a paren PLACED is re-stepped into a CALL exactly when it leads
> **two or more survivors** of an enclosing group that closes with a paren
> rewind — a user paren, an fn frame, or an `if` / `for` / `do` body. The
> program top level, list literals and map literals do not rewind.

Placement is the one-survivor case; the enclosing group is a SECOND decision
taken one paren out, not a context that modifies the first. That predicts
every measured row, including the ones this record never tried
(`for 2 [(mk 1) 2]` → `3 3`, `case 1 [1] [(mk 1) 2]` → places).

**The defect was the COMPILER'S, in both directions, and there were five of
them** — not the one this record names. The paren structure is erased before
the residual lowering, so `resolveDynamicApply` sees the same
`[carrier, 2]` for `(mk 1) 2` and for `((mk 1) 2)` and guesses:

| program | interpreted | compiled (before) |
|---|---|---|
| `(mk 1) 2` | `fn (Integer) 2` | `3` — applied a placement |
| `(mk2 5) 10` | `fn (Integer) 10` | `11` — applied a placement |
| `[((mk 1) 2)]` | `[3]` | `[fn (Integer) 2]` — placed an apply |
| `if true [(mk 1) 2]` | `3` | `fn (Integer) 2` — placed an apply |
| `if true [((mk 1) 2)]` | `3` | `fn (Integer) 2` — placed an apply |

`((mk 1) 2)` compiling to `3` at the top level was RIGHT BY ACCIDENT: the
outer paren collapses, the pair reaches the program residual, and the
carrier arm applies it there. One level down, inside a list or an arm, the
same paren reaches no residual lowering and produced the opposite answer.

**How they survived a 100%-covered suite.** Stage J flipped `lang.Run` from
the tree-walking interpreter to the COMPILED path. **75 parity assertions
across five files still read `gotI, _ := ….Run(src)`** and compare it to
`RunCompiled` — the compiled lane against itself, passing unconditionally.
`TestFactoryApplyCompiles` asserts `(mk2 5) 10` is `[11]` "on both lanes";
the interpreter has never answered `11` for that program. This is the
finding that matters most in this record, and it generalises: any flip of a
Run-like entry point needs a mechanical sweep of its oracle uses.

**Verdict (2026-08-27): resolve by fix, COMPILER-side only. LANDED.** The
interpreter is already correct and is unchanged.

The mechanism is a matched PAIR of records taken at the collapse, because
that is the last moment the two spellings are distinguishable:
`ParenPlacedFnIDs` (the park returned 1 — one survivor, placed) and the new
`ParenReSteppedFnIDs` (the park returned 0 over more than one survivor — the
rewind lands on the lead and re-steps it). The residual lowering, the branch
arm merge and the list-literal assembly all read the pair instead of
guessing from the value's shape, which is what lets `((mk 1) 2)` keep
compiling natively while `(mk 1) 2` — byte-identical at that point — is
refused rather than applied.

**Value divergences on the 16-shape probe: five → zero.** Four shapes refuse
where the interpreter answers; they are one shape in four positions (a
paren-bounded carrier apply consumed where the residual lowering does not
reach), and they graduate together with Stage 3's universal fn values and
Apply kernel. Standing measurement:
`lang/go/nur101_paren_restep_test.go`. Full account, with the measurement
tables and the harness finding:
[design/PAREN-RESTEP-RULE.0.md](PAREN-RESTEP-RULE.0.md).

---

## NUR103 — the checker's answer depends on who is asking {#nur103}

**Status:** RESOLVED 2026-09-25 by the diagnostic-surface gate ·
**Recorded:** 2026-08-26 · **Surfaced by:** the
server-concurrency corpus (`test/go/servercorpus`), measuring why a real
protocol server does not compile

**Resolution 2026-09-25 (the reverse-order NUR run).** The rule — one
analysis, one answer — is now a GATE: `TestDiagnosticSurfaceParity`
(`test/go/langspec/diag_surface_test.go`) sweeps every corpus row through
both surfaces and fails on any diagnostic class the compile surface emits
that the plain `check` surface does not, unless the class is adjudicated in
`diagSurfaceLedger` with its graduation condition (`undefined_word` is
ledgered: the Stage 1 `/v` hold's residue, re-diagnosed 2026-09-19; the
`fn_body_error` entry graduated today, no row shows it any more). The
mini-redis instance still reproduces in the FULL module context (measured
today: the driver's plain check is clean, the compile-armed check reports
`undefined_word: h2` and refuses), and every reduction tried — the handler
body over plain maps, over `Any` params with paren-read keys, registered
through `service … add` — checks clean on both surfaces, so the ingredient
is the enclosing module fn's context, not the def-and-read shape. It is
the ledgered class's open instance, owed the class's graduation (the
`undefined_word` entry) and a servercorpus row that carries it into the
gate's sweep; no verdict of this record changes.

**Rule:** one analysis, one answer. The static pass is a single
abstracted interpreter over carriers; its verdict on a program is a
property of the program, not of what the caller intends to do with the
verdict.

**Divergence:** the same program yields a clean check and a refusing
compile.

```
$ boru check redis_run.boru
check: 0 error(s), 0 warning(s), 1 info

$ boru run -force-compile redis_run.boru
error: force-compile: check diagnostics: [undefined_word] undefined word: h2

$ boru run -no-compile redis_run.boru      # runs fine, returns "hello"
```

where `redis_run.boru` imports `design/examples/apps/mini-redis.boru` and
issues a SET then a GET. The offending name is at `mini-redis.boru:210`
— `def h2 (h set (f) v) (hashes set (k) h2) drop 1`, inside a lambda
registered as a service handler. Checking the module **on its own** is
also clean (0 errors, 2 unrelated unused-def warnings), so the diagnostic
is produced by neither the module nor the plain check of the importing
program: it appears only when the analysis runs with the recorder armed.

**Why it matters.** This is the mechanism that refuses realistic servers.
The plain echo server compiles to zero interpreter runs; mini-redis does
not compile at all, and this diagnostic is the sole reason. A user cannot
diagnose it either, because the tool they would reach for — `boru check`
— reports the program clean.

**Mechanism (not yet isolated).** `design/FULL-COMPILATION.0.md` §6.9(3)
names the shape: numerous analysis models fork on `!Compiling` — static-if
reduction, loop spread, closure surfacing, and the emission of
`no_signature` itself. One of those forks is presumably reached here, and
the compile-armed path surfaces a diagnostic the plain path does not.

Hypotheses tested and REJECTED, recorded so they are not re-explored:

- *The fn-analysis quota fork.* `check/go/carrier.go:2757` skips the
  quota short-circuit while `Compiling`, on the stated grounds that the
  compiler must see real body events rather than a fabricated
  `declaredReturnBail`. That would make the compile pass analyse bodies
  plain check skips — an exact fit for the symptom. **Ruled out:** the
  bail emits an `analysis_truncated` diagnostic at quota+1, and the plain
  check of this program reports no such diagnostic, so the quota is never
  reached.
- *A general "def and use in one statement" defect.* Two reductions —
  at top level, and inside a one-parameter lambda — both check clean
  under compilation. The trigger is narrower than the surface shape.

**MINIMAL REPRO (2026-08-26).** Found by the diagnostic-parity gate
(`test/go/langspec/diagnostic_parity_test.go`), which enumerates every
corpus row whose findings differ between the passes. One line, and the
code is entirely ordinary — a record-typed parameter read through dot
access:

```
$ cat rec.boru
def f fn [[o:{pretty:Boolean}][Boolean][o.pretty]] f {pretty:true}

$ boru check rec.boru
check: 0 error(s), 0 warning(s), 0 info
check: Boolean

$ boru run -no-compile rec.boru
true

$ boru run -force-compile rec.boru
error: force-compile: check diagnostics: [undefined_word] undefined word:
```

Note the diagnostic names **no word at all** — an empty `Word` field —
which is itself a defect: whatever emits it has lost the identifier it is
complaining about. The corpus row is `edge-dispatch-3.tsv:L56`.

**Traced to the emitter.** An earlier note here claimed the check-mode
constructor `undefinedWordCheckDiag` (`check/go/check_recovery.go`) had no
production callers and that the live emitter was
`Engine.undefinedWordError`. That was wrong, and instrumenting
`undefinedWordError` proved it: for this program it never fires. The live
emitter IS `undefinedWordCheckDiag`, reached through the `CheckBraid` seam
from the analysis arm of `stepWord` (`core/go/engine.go`) — with `w.Name`
empty and no source position. So **something with no name is being stepped
as a word**.

**Trigger isolated (2026-08-26), and it is NOT the reach lowering.** The
first hypothesis — that dot access produced a nameless Word — was tested
and falsified. The trigger needs two conditions together:

| param type | body | result |
|---|---|---|
| `{pretty:Boolean}` | `o.pretty` | **refuses** |
| `Map` | `o.pretty` | compiles |
| `{pretty:Boolean}` | `o get "pretty"` | **refuses** |
| `{pretty:Boolean}` | `true` (no field read) | compiles |
| `{pretty:Boolean}` | `o.pretty`, other names | **refuses** |

So: **a field read from a parameter whose declared type is a STRUCTURAL
RECORD type**. Neither half alone reproduces — the same dot access over a
plain `Map` parameter compiles and returns `true`, and the same record
parameter with no field read compiles. It is not dot-specific either:
an explicit `get` fails identically, which is what rules the reach
lowering out.

An instrumented run printed the sub-engine's whole tape at that moment, and
the answer was in one line:

```
[step] ({__OP} dynamic(record{pretty:word(Boolean)}){Map} pretty{Atom} >word(dot){Word} ){__CP}
[step] ({__OP} >dynamic(word()){Word} ){__CP}
=== EMPTY WORD  data=<nil> parent=Word carrier=true dynamic=true
```

`o.pretty` lowers to `( o dot pretty )` and reduces correctly. What it
reduces TO is the defect: a **carrier whose type is `Word`**. The step loop
classifies by type alone (`IsWord` is `v.Parent.Equal(TWord)`), so the next
step dispatches that carrier as a token; it has no `WordInfo` payload, so
the name is `""`, and every name arm falls through to the undefined-word
tail. Hence a diagnostic naming no word.

**Why the carrier's type is `Word`.** The param's declared record schema is
`{pretty: word(Boolean)}` — the field type is still the unevaluated Word
TOKEN `Boolean`, not the type. `recordSchemaFieldReturns`
(`lang/go/native/native_storage.go`) then does the obviously right thing
with the wrong input: `ft := ValueType(fv)` on a Word token is `Word`, so
the field read narrows to `dynamic(Word)`.

The field type is unresolved because of an asymmetry between the two ways
of spelling a record parameter:

| spelling | how the field map is built | field value |
|---|---|---|
| `type R record {pretty:Boolean}` then `o:R` | `record` DISPATCHES, evaluating its map | type value |
| inline `o:{pretty:Boolean}` | rides inside the fn-spec LIST — inert data, never evaluated | Word token |

`ResolveSigType`'s map arm returned the inline map as the pattern verbatim.
The dispatcher tolerated it (`Unify` resolves the name at match time, which
is why `f {pretty:1}` was correctly REJECTED all along), but the
schema-bearing param carrier — `recordSchemaCarrier`, `check/go/check_fnmodel.go`
— copies the pattern's fields verbatim into a `RecordTypeInfo`, so the
unresolved word reached the read. The typed-container child of `xs:[:Foo]`
has had exactly this resolution since generics landed
(`ResolveSigChildParam`); the structural-record field simply never got its
twin.

**Why only the compile path.** Nothing in the fork logic — the plain pass
builds the COARSE param carrier (`{:Any}`, no schema), so it never reads the
field schema and never mints the bad type. Only the compile pass builds the
precise schema-bearing carrier. The `!Compiling` forks named as suspects
above (`nur068ReturnCarrier`, the fn-analysis quota) are not implicated;
neither is the reach lowering, which the truth table had already cleared.

**Fixed (2026-08-26).** Two changes, one for each half:

1. `ResolveSigRecordFields` (`core/go/generics_unify.go`) resolves an inline
   record pattern's field type words at sig install, following
   `ResolveSigChildParam`'s cascade arm for arm — type-param node, user type
   body, builtin name — and leaving literal-value patterns (`{status:'ok'}`,
   ADR-010) and words naming no type untouched. Nested inline records
   resolve recursively. Wired into `ResolveSigType`'s map arm, so both
   spellings of a record parameter now produce the same pattern.
2. `stepWord` (`core/go/engine.go`) collects a word-TYPED value with no
   `WordInfo` as data instead of dispatching it. A carrier is an abstract
   value, not a token: there is no name to look up and no arguments to
   collect. This is reachable for any word-typed carrier that is genuinely
   word-typed — a `w:Word` record field read, a declared `Word` return — and
   is what keeps the class of defect from re-appearing as another nameless
   diagnostic.

With both in place the minimal repro checks clean, interprets to `true`, and
COMPILES to `true` under `-force-compile`.

The second half was not compile-path-only, which the record-schema instance
had disguised. A fn with a DECLARED `Word` return produces the same carrier
on the plain pass, and the same nameless diagnostic came out of `boru check`:

```
$ cat w.boru
def g fn [[][Word][quote foo]]
def h (g)
1

$ boru check w.boru                     # before
check: 1:20: [error] type_error: g: return value 1: expected Word, got Atom
check: [error] undefined_word: undefined word:            <- spurious, positionless
check: 2 error(s), 1 warning(s), 0 info

$ boru check w.boru                     # after
check: 1:20: [error] type_error: g: return value 1: expected Word, got Atom
check: 1 error(s), 1 warning(s), 0 info
```

One real diagnostic, one phantom beside it, with no position and no name.
That is the shape to watch for: a positionless diagnostic is a diagnostic
whose subject the emitter never had.

**The `h2` site is a DIFFERENT defect.** `MiniRedis.serve` still refuses
with `undefined_word: h2` after this fix. That one names its word, so it is
not the nameless-carrier family at all; the two shapes shared only the
armed-only symptom. It is diagnosed separately below, and the general
Discharge stands either way: the class defect is that a diagnostic can
exist in one pass and not the other, which no single root-cause fix
retires.

Worth recording how this one was found: four manual reductions of the
mini-redis shape had failed, because they were reducing the wrong program.
The parity gate found an unrelated, far smaller instance in one run — which
is the argument for building the measurement before hunting the bug.

**The other four armed-only rows**, for whoever picks this up:
`case.tsv:L76` (`case_not_exhaustive`), `case.tsv:L97`
(`undefined_word/zed`), `edge-quote-1.tsv:L103` (`undefined_word/nosuch`,
emitted TWICE), `generics-fn.tsv:L55` (`undefined_word/value`).

**The `h2` half, diagnosed 2026-08-26.** Found by delta-debugging the real
program rather than by writing reductions: keep one handler at a time and
ask which still refuses. Thirteen of the fourteen drop out; `HDEL` alone
reproduces, and `HSET` — the site the diagnostic NAMES — does not. Reducing
that block in place, then lifting it out of the module, gives twelve
self-contained lines:

```
import "boru:net"
def serve fn opts:Map Any [
  def svc (service {kv:{}}) add {cmd:"HDEL"}
    ([req:Map state:Any] =>
        [def hashes {}
          def cur (hashes get "k")
          def h2 (cur set "f" None) h2
        ]
    )
    svc Net.listen {tcp:opts.port codec:Net.lines} svc
]
def ln (serve {port:0})
1
```

`boru check`: clean. `boru run -no-compile`: `1`. `boru run
-force-compile`: `undefined_word: h2`. Three faults compose, each of them
§6.9(3)'s family:

1. **`boru check` does not analyse a service-handler body at all.** Put a
   bare `noSuchWordHere` in that lambda and `boru check` still reports the
   program clean while the compiler refuses it. This is not a parity
   nicety — it is a COVERAGE HOLE: a typo inside a request handler ships.
   The compile pass must analyse the body, because it has to record it into
   a compiled callback unit; the plain pass never does, and every
   divergence below follows from that one asymmetry. Bounded by
   experiment: a lambda called directly, and a lambda never called at all,
   are both analysed by both passes. It is registration through `add` that
   loses the body. (A lambda inside a LIST literal diverges the other way —
   plain check flags it, the compile pass refuses before reaching it.)
2. **A call the checker models as divergent silently unbinds its `def`.**
   `hashes` is the literal `{}`, so `cur` is statically `None`, so `cur set
   "f" None` matches no signature and is modelled as producing no residual.
   `def h2 <nothing>` therefore binds nothing, and the following read of
   `h2` is an undefined word. At run time none of this happens: `cur` is a
   real map and the handler works, which is why the interpreter answers
   `1`.
3. **The suppression hides the cause and leaves the symptom.** The
   `no_signature` that would NAME the failing `set` call is suppressed
   while compiling (the documented fork, `check_recovery.go`). Removing
   just the `h2` read makes the program compile silently — the bad call
   raises nothing on either pass. So the one diagnostic that does escape is
   the downstream consequence of a cause the user is never shown, at a
   different line, about a different name. That is exactly why the site the
   message names (`HSET`) is not the site that reproduces (`HDEL`).

Read together: the checker is analysing code the user's own tool never
looks at, reaching a false conclusion about it (the run-time `cur` is not
`None`), and reporting that conclusion through its least informative
symptom. The four earlier manual reductions all failed because they
reproduced the `def`-then-read SHAPE, which is fine on its own — what
matters is a `def` whose value the checker proves divergent, inside a body
only one pass ever reads.

**Discharge for this half.** Fault 1 is **fixed** — NUR105, which turned
out to be far wider than service handlers and is recorded separately. With
it, `boru check` on mini-redis reports what the compiler was refusing on,
at :230 in the HDEL handler, with the causal `no_signature` immediately
before the `h2` read: the refusal is diagnosable by the tool a user would
reach for, which was the point.

Fault 2 is **decided, not yet implemented** (2026-08-26). The question was
whether a provably-divergent value expression should bind a `Never` carrier
rather than nothing. The answer is the same one from a different angle:
after a provably-divergent expression the rest of the region is
UNREACHABLE, so the divergence is reported ONCE at its own site and its
downstream consequences are suppressed rather than re-derived as findings
about names. That is what an empty residual already means, and it is what
the compiled lane will do — the trap raises and nothing after it executes.
A checker reporting consequences the compiled program can never reach is
describing a program that does not exist.
`design/FULL-COMPILATION.0.md` §6.9(1) carries the rule and its two limits:
it does not suppress the divergence itself, and it does not extend past the
region.

Fault 3 is §6.9(3)'s named fork, and this record is the argument that
suppressing a diagnostic does not make its consequences go away; it makes
them unattributable.


**Discharge.** Either collapse the forks so the two passes cannot
disagree, or — where a fork is genuinely required — prove it
diagnostic-neutral, which is what §6.9(3) asks for. Note that fixing the
underlying binding bug is NOT sufficient on its own: the general defect
is that a diagnostic can exist in one pass and not the other, and the
next instance would be just as invisible to `boru check`.

## NUR105 — `boru check` does not analyse a callback body passed as an argument {#nur105}

**Status:** FIXED 2026-09-25 (the folded map member — the handoff log's
entry of that date; the three defect rows were discharged 2026-08-26, the
last position below) · **Recorded:** 2026-08-26 · **Surfaced by:** bounding
NUR103's mini-redis half, whose first fault was "check does not analyse a
service-handler body". The bound turned out to be far wider than service
handlers.

**Rule:** one analysis, one answer — and the analysis is over the whole
program. A body the checker is demonstrably able to analyse is not skipped
because of where the body was written.

**Divergence.** `boru check` reports clean, and the program dies:

```
$ cat cb.boru
each ([e:Any] => [nosuchw e]) [1 2 3]
1

$ boru check cb.boru
check: 0 error(s), 0 warning(s), 0 info
check: List Integer

$ boru run cb.boru
error: each: element 0: [boru/undefined_word]: undefined word: nosuchw
```

This is not a parity nicety and not a subtlety of gradual typing. It is a
plain FALSE NEGATIVE on the most common callback idiom in the language, for
an error — an undefined word — that the checker catches without difficulty
when the identical body is written one line up as a `def`.

**Measured, and the boundary is sharp.** Two columns: whether `boru check`
emits a finding naming the undefined word, and whether the program raises
when actually run (`-no-check -no-compile`, so the pre-flight verdict does
not stand in for the runtime's).

| callback spelling and position | check names it | runs |
|---|---|---|
| `each [nosuchw] …` — code BLOCK as arg | yes | raises |
| `each ([e:Any] => [nosuchw e]) …` — **`=>` lambda as arg** | **no** | **raises** |
| `each (fn [[e:Any][Any][nosuchw e]]) …` — **`fn` value as arg** | **no** | **raises** |
| `each ([e:Any] afn [nosuchw e]) …` — **`afn` value as arg** | **no** | **raises** |
| `each f/v …` — named fn REFERENCE as arg | yes | raises |
| `def g ([x:Any] => [nosuchw 1])` — def-bound lambda | yes | not called |
| `def g (fn [[x:Any][Any][nosuchw 1]])` — def-bound fn value | yes | not called |
| `def g ([x:Any] afn [nosuchw 1])` — def-bound afn value | yes | not called |
| `def xs [([x:Any] => [nosuchw 1])]` — lambda in a list | no | not called |
| `def m {k:([x:Any] => [nosuchw 1])}` — lambda in a map | no | not called |
| `([x:Any] => [nosuchw 1])` — bare lambda statement | no | not called |

Three rows are outright defects — the bolded ones, where the checker is
silent and the program then raises. The rest are consistent: either the
body is analysed, or it is never reached at run time so there is nothing to
miss. (Whether an unreached broken body deserves a warning is a separate
question, and not this record's.)

The boundary is not "anonymous", and not "which lambda spelling". All three
anonymous spellings — `=>`, `fn`, `afn` — are analysed when `def`-bound and
missed when passed as an argument; a code BLOCK argument is analysed, and
so is a named fn REFERENCE. What decides it is the POSITION: a function
VALUE constructed as an argument has its body analysed by nobody. The same
gap covers the bare-statement position and the stored service handler.

**Why it matters more than its NUR number suggests.** `each`, `filter`,
`fold`, `map`, every `Net`/`service` handler, every sort comparator: the
callback-as-argument shape is how boru does higher-order work, and it is
the shape §12 and §13 of `design/FULL-COMPILATION.0.md` build the whole
server story on. A user writing a typo inside one gets no warning from the
tool that exists to warn them. It also means the frontier measurements
that read `boru check` diagnostics have been reading a pass that skipped
this code — the diagnostic-parity gate's own numbers included.

**Relationship to NUR103.** Same family, wider blast radius. The stored
service handler is the same gap in a different position, and NUR103's fault
chain there (unanalysed body → a divergent call silently unbinding its
`def` → the `no_signature` suppression hiding the cause) begins with
exactly this. Fixing this is the first of NUR103's three faults, and closes
the handler case along with the three defect rows above.

**Discharged for the three defect rows (2026-08-26).** Construction now
QUEUES the body (`CheckState.PendingFnBodies`) and the queue DRAINS at end
of pass, immediately before `RescueForwardRefDiagnostics` and
`EmitUnusedDefDiagnostics` — which sit there for exactly the same reason.
The analysis itself is the SAME pass `InstallFnDef` already ran, reached
through the analysis seam (`RunFnConstructionPass`): one route, not three,
because a second construction-time analysis with its own rules would be a
new way for two spellings of one callback to disagree, which is the defect
rather than the fix.

The queue matters, and the first version did without it. Analysing a body
at its CONSTRUCTION site analyses it too early — every forward reference is
still unbound there, a recursive self-call most of all — so

```
def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]
```

reported `mul: got (Integer, Atom)`, because `fact` resolved to the
undefined-word placeholder. The `undefined_word` behind it would have been
rescued at end of pass; its CONSEQUENCE would not. Draining at end of pass
fixes that and disposes of a second problem at the same time: a fn that
reaches `InstallFnDef` before the drain is analysed THERE, under its name,
so the named path keeps its precision and the drain gets exactly the set of
fn values nothing ever named.

**Three things implementing it discovered, and they are the part worth
keeping.**

FIRST, the dynamic-scope rescue is NAME-KEYED. `RescueForwardRefDiagnostics` asks
whether a binder of the name can reach the READING fn through the call
graph, and the reader is a NAME; `DynamicScopeReachable` answers false
outright when it is empty. So a nameless analysis silently loses the rescue,
and the pinned recursion idiom

```
def f fn [[m:Map n:Integer] [Integer]
  [if (n lte 0) [acc2] [def acc2 n … f m (n sub 1)]]]
```

drew a false `undefined_word: acc2` — a false POSITIVE traded for the false
negative, which is not a fix.

An anonymous body has no call-graph identity, so the sound reachability
question cannot be ASKED of it. The first fix answered it optimistically —
rescue when SOME fn binds the name — a rule deliberately weaker than the
named one, justified only by the alternative being a false positive on a
legal idiom.

**The drain removed the need for it, and the merged cover-gate is what
noticed.** The gate came back one statement short and the statement was
that arm: with the analysis deferred to end of pass, every name is bound by
then, so the plain `r.Defs.Top(d.Word)` rescue directly above catches these
diagnostics and the optimistic arm is never reached. Deleting it changes
nothing — the pinned recursion-with-dynamic-scope shape still checks clean
and the matrix is unchanged. The redesign that made the analysis CORRECT
also made a soundness-weakening special case redundant; those two usually
travel together, and a 100% floor is what surfaces the second half, because
nothing else in a suite reports an arm that has quietly stopped being
reachable.

SECOND, a body must be analysed in the SCOPE IT WAS WRITTEN IN. Draining
every queued body against the top-level registry reported every
module-scope name in mini-redis's handler lambdas as undefined —
`arg-at`, `kv-read`, `now-ms` — because those words are bound in the
module's sub-registry. Each queue entry therefore carries its own registry.
Caught by the two mini-redis false-positive tests, which is what they are
for.

THIRD, a SPECULATIVE analysis that cannot run must report nothing. The
drain analyses bodies nobody asked about, in ISOLATION, with no enclosing
stack — so a body that reads the caller's stack cannot be run at all. The
Church-numeral row in `frontier-hof-audit.tsv` raised the
strict-forward-barrier error whose own text says why ("`apply` reads its
receiver from the enclosing stack") on a program that answers 5.
`fn_body_error` is the analyser reporting that IT could not proceed — a
fact about the analysis, not about the code — so the drain drops it. A
NAMED fn keeps it: defining a fn is asking for its body to be analysed.

Two smaller notes. Every named fn's body diagnostics came out TWICE until a
pass-scoped body-position set was added — a fn reaches the check at
construction AND at install, and the two differ only in the name, which
`FnSummaries`' key does not collapse. And the CheckState lifecycle gate had
a blind spot of its own, found the moment the first struct-keyed field
arrived: its clone assertion could not build a second key for anything but
string- and pointer-keyed maps, so the mutation silently did not happen and
the assertion passed for a map `Clone` was in fact sharing.

**The last position (closed 2026-09-25).** A lambda written as a MAP-literal
value was still unanalysed, where the list-literal twin was analysed — a
different route: a map member is a check-mode CONST FOLD
(`constFoldContainerVal`, a concrete sub-run with the pass OFF), so the fn
value it built was queued by nobody, while a list literal never folds and
its lambda queued at construction. The fold now queues every fn value inside
the folded constant (`noteFoldedFnBodies` → `NoteFnBodyPending`) and the
end-of-pass drain analyses it like the twin: `def m {k:([x:Any] => [nosuchw
1])}` reports `undefined_word: nosuchw` at 1:23, the bare statement and the
list member alike. The position no longer decides the answer.

## NUR110 — a def in an untaken branch binds anyway {#nur110}

**Status:** Resolved (2026-09-22, the branch-carried def —
`compiler/go/branch_carried.go`; see the closing note at the end of this
record) · **Recorded:** 2026-08-28 · **Surfaced by:** checking what
NUR109's compiled answer rested on

**Rule:** a `def` runs when its branch runs. A name defined only inside a
branch that was not taken is not bound afterwards — which is exactly what the
interpreter does, and what both lanes already do for a zero-iteration loop.

**Divergence:** the compiled lane binds it anyway.

```
if false [def op 1] [0]  end  op
  compiled     [0 1]                 <- op is 1
  interpreted  undefined_word        <- op was never defined

if false [def op 1] []   end  op          → [1]           vs undefined_word
if false [def op 1] [0]  end  typeof op   → [0 Integer]   vs undefined_word
```

**Why the existing gate misses it.** Family L already refuses the SHADOW case:
a fn redefined inside a conditional body overlap-removes the enclosing overload
in place, the depth-based rollback cannot revert it, and `installDef` marks the
program uncompilable (`core/go/core_helpers.go`, gated on
`analysisInCondBody`). But that refusal is reached only when the overlap filter
actually DROPS an entry — `changed` is true. **A fresh `def` drops nothing**, so
`changed` stays false and no refusal fires. The gate covers redefinition and not
definition.

Measured boundaries, so the fix knows its own edges:

| shape | compiled | interpreted | |
| --- | --- | --- | --- |
| fresh def, untaken `if` branch | binds | unbound | **miscompile** |
| shadowing a VALUE binding | refused (residual provenance) | correct | not wrong — the refusal is a defect of its own |
| shadowing a FN overload | refused (family L, by name) | correct | not wrong — the refusal is a defect of its own |
| zero-iteration `for` body | unbound | unbound | correct |
| taken branch | binds | binds | correct |

The zero-iteration loop row is the useful one: the same question is already
answered correctly there, so the machinery to get this right exists.

**Verdict: resolve by fix, compiler-side — and refusing is containment, not
the fix.** Its two siblings above refuse, and those refusals are defects owed
their own fix; what makes them less urgent than this row is only that a
refusal is contained (the interpreter still runs the program correctly) where
a silent wrong binding is not. Full graduation is the same one
family L already names — "a runtime dispatch respecting the conditional
binding" — which is Stage 4/5's def-twin work, not Stage 3's. Pinned as
measured meanwhile by `lang/go`'s `TestCondBodyFreshDefBindsCompiledOnly`
(since 2026-09-22 `TestCondBodyFreshDefRaisesLikeInterpreter`, asserting
parity — see the closing note).

**SHARPENED 2026-08-30, and the diagnosis above is not the one that matters.**
Three things the record did not have.

1. **THE CHECK PASS AND THE COMPILE PASS DISAGREE WITH EACH OTHER**, which is a
   larger finding than the compiled/interpreted split this record was filed
   under. Same program, same front end:

   ```
   def c false  if c [def op 1] [0] end op

   boru check          check: 1:38: [error] undefined_word: undefined word: op
   boru check --emit   (no diagnostics at all)
   ```

   The checker's model rolls the branch binding back correctly and says `op` is
   unbound — agreeing with the interpreter. The compile pass keeps it, reports
   nothing, and the disassembly shows why:

   ```
   0000 PUSH_CONST  k0   ; false        0003 PUSH_CONST k1 ; 0
   0001 JMP_IF_FALSE -> 0003            0004 PUSH_CONST k2 ; 1   <- `op`
   0002 JMP -> 0004
   ```

   `op` is CONSTANT-FOLDED to the value the untaken branch installed. So this is
   not "a gate that fires on `changed` and should also fire on fresh" — the
   rollback the checker performs is already right, and the recorder's view of
   the same pass survives it. Fixing the recorder's rollback is the repair;
   refusing would only contain the miscompile — a defect in its own right,
   owed a fix — if the repair proves unreachable.

2. **THE BOUNDARY TABLE ABOVE IS INCOMPLETE.** Re-measured at `-no-check`,
   `if` is not the only shape:

   | shape | compiled | interpreted | |
   | --- | --- | --- | --- |
   | `if false [def op 1] [0] end op` | `0 1` | undefined_word | **miscompile** |
   | `[] each [def op 1] end op` | `[] 1` | undefined_word | **miscompile** |
   | `0 [] fold [def op 1] end op` | `0 1` | undefined_word | **miscompile** |
   | `case 9 [1] [def op 1] [0] end op` | undefined_word | undefined_word | correct |
   | `for 0 [def op 1] end op` | undefined_word | undefined_word | correct |
   | `[] filter [def op 1 true] end op` | undefined_word | undefined_word | correct |

   Two higher-order bodies diverge and a third does not, which is the useful
   asymmetry: `each` and `fold` leak the binding, `filter` does not, and all
   three raise CondBodyDepth through the same `native_array.go` seam. Whatever
   `filter` does differently is the mechanism to copy.

3. **CORRECTION, same day: the default CLI path is NOT safe.** The first
   version of this note said `boru run`'s check pre-flight catches it. That was
   measured only on the record's own CONSTANT-condition shape, where the
   `unreachable_branch` analysis happens to also emit `undefined_word` — the
   benign case, which masked the general one. With a condition the checker
   cannot fold, nothing fires anywhere:

   ```
   def f fn [[n:Integer][Boolean][n gt 5]]  if (f 1) [def op 1] [0] end op

   boru check             0 error(s), 0 warning(s), 0 info
   boru run               0 1                                    <- compiled
   boru run -no-compile   [boru/undefined_word]: undefined word: op
   ```

   So this is a SILENT WRONG ANSWER on the default path, with no diagnostic on
   any lane — the class this project ranks above every refusal. It is not an
   `-no-check` curiosity.

   Worth keeping as a method note: the record supplied `if false`, and
   measuring only what a record hands you reproduces its blind spot. The
   constant-condition shapes are the ones where a SECOND analysis (the
   unreachable-branch pass) independently notices the unbound name; they say
   nothing about whether the binding leak is caught.

4. **THE MECHANISM, located exactly.** `InstallJoinedDefs`
   (`core/go/carrier_join.go`) folds each arm's net def additions back after the
   branch. A name bound in ONE arm with no pre-branch binding to join against
   takes the bare `r.Defs.Push(k, tv)` arm — a DEFINITE binding. Instrumented,
   that arm is reached by `if` and by nothing else: `case`, `for`, `each`,
   `fold` and `filter` never touch it, which is why they were measured correct
   and why `each`/`fold` (which have no `r.Defs` snapshot in
   `analyseHigherOrderBodyVals` at all) are a SECOND, separate leak.

   The push is not simply wrong. Dropping it would report `undefined_word` on
   every legitimate post-branch read — the false positive the join exists to
   prevent. What the push cannot express is that the binding is CONDITIONAL,
   and the model has no third state between bound and unbound.

**A REFUSAL AT THE JOIN WAS BUILT, MEASURED AND REJECTED — 131 corpus rows.**
Marking the program uncompilable at that fresh-push arm (family L's pattern,
which is what this record's verdict proposes) restores parity on every shape
above and costs far too much:

	compiled coverage: 7644 rows — 7139 compiled, 131 refused   (gate: 0)
	  85  `r` …defined only inside a conditional branch
	  38  `i1` …
	   8  others

Sweeping the refusing rows says why, and it is not a tuning problem:

- **The name is never read after the branch.** `def mk fn [[i:Integer] [Map]
  [if (i lte 0) [do {…}] [def more (mk (i sub 1)) do {…more…}]]]` defines
  `more` in an arm and reads it INSIDE that same arm. No divergence exists;
  the refusal is a pure false positive.
- **The shape is inside an imported MODULE.** The 85 `r` and 38 `i1` rows are
  `import "boru:cli"` rows whose own source contains such an `if`. A user
  program that never writes the shape loses compilation because a library
  does.

So the join site is the wrong place: it knows a name was bound conditionally,
and cannot know whether anything will READ it afterwards.

**THE READ SITE WAS THEN BUILT TOO, AND REJECTED FOR A SHARPER REASON.** Marking
the name at the join (with its DefTable generation, so a later unconditional
`def` clears it) and refusing at `resolveOperand` — after first trying
`dynScopeRescue`, which repairs the in-fn case outright — took the corpus from
131 refusals to **0**, closed every shape in the table above, and looked done.

`lang/go`'s unit tests said otherwise, and the reason is structural rather than
tunable. Three tests failed; one was NUR110's own pin graduating, and two were
genuine false positives of a class the marking cannot see:

```
def out (if (3 gt 1) [ def t2 {a: 1}  (m set "k" t2) drop  t2 ] [ {a: 0} ])  out
                          ^^ refused on `t2`
```

`t2` is the arm's OWN LOCAL. It is bound conditionally — that much is true — and
it escapes as the arm's RESULT, seated by the branch exactly as the compiler
already models. A post-branch read of the NAME is unsound; the arm's result
VALUE flowing onward is sound; and both arrive at `resolveOperand` as "a value
whose `defReads` name is conditionally bound". The signal cannot separate them,
and neither can ordering: the arm's fragment is lowered at Finalize, after the
join has marked.

Nor is constant-folding the discriminator. `3 gt 1` is not literal-folded, so
the shape takes the general branch path; splitting `InstallJoinedDefs` into
definite and conditional variants was tried and changed nothing.

**A THIRD ATTEMPT, AND THE RULE THIS RECORD HAD WRONG.** The `each` / `fold`
half was filed above as a SECOND, separate leak — `analyseHigherOrderBodyVals`
raises CondBodyDepth and pushes a spec baseline but never snapshots `r.Defs`,
where every branch/loop body has always rolled back (`runCarrierBodyDefsAdds`,
keep=false). Adding that rollback fixes `[] each [def op 1] end op` and BREAKS
the non-empty case, because the leak is the interpreter's actual semantics:

```
def j 999  def _ ([1] each [def j (5 add 1) j])  j   →  6      not 999
[1] each [def j 6 j] end j                           →  [6] 6
[1 2] each [def op 1] end op                         →  [1 2] 1
[]    each [def op 1] end op                         →  undefined_word
```

A body-local `def` LEAKS, like `do`, and SHADOWS an enclosing binding of the
same name. It exists after the construct if and only if the body actually RAN.

So `each`/`fold` is not a smaller sibling of the `if` half — it is the SAME
question, and the rule both need is a THIRD binding state: *bound iff that body
ran*. Every fix that picks one of the two existing states is right for one case
and wrong for the other. There is no rollback-shaped fix, and no refusal-shaped
fix that is not either unsound or over-broad.

`filter` stays correct under all three attempts, and it is worth naming why
without over-reading it: `filterReturnsFn` NEVER RUNS THE BODY — it types the
result only, and the predicate goes through the declared Callable spec's closure
path. That is not a mechanism `each` can copy; `each` needs the body's
diagnostics.

**So the verdict stands as resolve-by-fix, and the fix is the def twins.** What
is needed is per-read provenance distinguishing a name read after the construct
from a value produced inside it, plus a binding state that survives as
conditional — which is exactly "a runtime dispatch respecting the conditional
binding", family L's stated full graduation, and Stage 5 work. Three attempts
are now recorded as rejected, each with its measurement, so the next one does
not re-derive them.

**THE DEF TWINS LANDED AND FLIPPED (2026-09-01/02), AND THIS RECORD IS STILL
OPEN — with a NEW mechanism.** The sentence above ("the fix is the def twins")
is now overtaken: rollback-and-replay is the only regime, and the divergence
survived it. Recording the change, because the symptom is identical and the
mechanism is not, and a reader who checks only the symptom will conclude
nothing moved.

BEFORE the flip, the compiled lane KEPT the check pass's installs, so the
arm's `def` simply survived the pass into the run. AFTER it, the pass's
installs are rolled back and each transition is REPLAYED from a placed op —
and the arm's twin is placed OUTSIDE the branch. Measured
(`boru check --emit -e 'if false [def op 1] [0] end op'`):

```
0000 BIND_TWIN   w0   ; bind twin def op @depth 1 (replay)   <- unconditional
0001 PUSH_CONST  k0   ; false (Boolean)
0002 JMP_IF_FALSE -> 0004
0003 JMP         -> 0005
0004 PUSH_CONST  k1   ; 0 (Integer)
0005 PUSH_CONST  k2   ; 1 (Integer)                          <- the read, folded
```

So the record now has TWO independent wrongs where it had one, and either
alone reproduces it: the twin replays unconditionally because it sits before
`JMP_IF_FALSE`, and the read of `op` const-folds at check time to the arm's
value. A conditional PLACEMENT fixes the first and leaves the second.

The measured boundaries in the table above all still hold — the value-shadow
row still refuses (`residual value of unknown provenance`), family L still
refuses the fn-shadow, the zero-iteration loop is still correct. The
divergence is reachable only with the checker bypassed (`-no-check`): both
lanes report `undefined_word` for the bare shape under a normal run, which is
the third disagreement this record already carries — check and compile
disagreeing with each other.

**The graduation path is re-filed.** Not "the def twins", which are done:
§6.9's `OpDispatchGeneric` for the read half, plus a BINDER half that makes a
conditionally-bound name registry-visible at VM time and a placement that puts
the twin INSIDE the arm's region. That is the same re-filing the four
payoff-list gates took on 2026-09-02 (design/FULL-COMPILATION-HANDOFF.0.md,
"The payoff gates, measured"), and for the same reason: the twins fix where
the registry is, not what is in the bytecode.

---

**CLOSED 2026-09-22 — by fix, compiler-side, on the loop's own mechanism.**
The record's verdict named the repair: the machinery that answers the same
question for a zero-iteration loop. It is the LOOP-CARRIED def
(NoteLoopCarried: a frame slot per name, a store at each rebind site, the
pre-loop value seeded before the loop), and the BRANCH-CARRIED def
(`compiler/go/branch_carried.go`) is the same mechanism seated at
`RecordBranch`: one frame slot per name per unit — a loop and a branch
carrying the same name share the cell, so nesting composes — every arm def
of the name a store into it at its own site, the pre-branch binding seeded
before the branch when one stands, and the joined carrier's identity
aliased to the slot so a read after the merge loads whichever arm ran. The
slot means "bound since this frame started" (a frame's locals begin as the
zero Value; measured first: `for 2 [if (i eq 0) [def z 9] [] end z]` is
`9 9` interpreted, so an arm's binding from one iteration is read in the
next and a seed must never re-run per branch), and a name with NO pre
binding, bound in one arm only, is read through a BOUND-CHECKED load
(`OpPushLocalBound`): the zero slot is the arm that did not run, and the
read raises the interpreter's own `undefined_word`, at the read's own
position, with the frame's locals among the did-you-mean candidates. Every
shape in the tables above now agrees on both lanes — `if false [def op 1]
[0] end op` raises `undefined_word` compiled — and the two shapes the
withdrawn read-site screen could not separate (`t2` escaping as the arm's
RESULT) are separated by construction: the arm's own reads keep resolving to
the arm's value; only the JOINED binding is aliased. `lang/go`'s
`TestCondBodyFreshDefRaisesLikeInterpreter` asserts parity where the old
fence pinned the divergence, `TestBranchCarriedDefParity` holds
twenty-four shapes to account, and `lang/spec/fn-locals-scope.tsv` §6b/§6c
carry the witnesses.

**What this record's history bought.** Both rejected attempts stand as the
reasons the fix has the shape it has: a refusal at the JOIN was 131 corpus
rows because the join cannot know whether anything reads the name (the
slot is allocated at the join but costs nothing unread — a store and a
cell); a refusal at the READ could not tell a read of the NAME from the
arm's VALUE flowing out as its result (the alias is by the joined carrier's
identity, which the arm's own reads never carry). And InstallJoinedDefs'
push for the no-pre one-arm case is now a payload-less CARRIER under a
compile pass (`condBoundCarrier`), so nothing can bake the arm's value even
where the slot cannot be seated: a `_`-prefixed name (whose def the recorder
never records) declines "unknown provenance" instead of answering 9 — a
contained compile failure, still owed its seat.

`each` / `fold`'s leak (the third attempt above) is the interpreter's own
semantics and is untouched by this.

## NUR122 — a compiled fn-value apply has no name and no named-dispatch semantics {#nur122}

**Status:** FIXED 2026-09-25 (the read's own token — the handoff log's
entry of that date). **Found:** 2026-09-05, measuring the closure-capture
family's blocker (a).

**Rule:** the two lanes agree on errors (NUR108: the message, the code,
and the position are all part of the error a user meets).

**Divergence, measured on the default lane:**

```
def f fn [[g:Function x:Integer][Integer][g x]]  f (z:String => [z]) 5
    interpreted  signature_error: cannot call `g` — no signature matches the arguments   (1:43)
    compiled     type_error: f: expected 1 return value(s), got 2 — [fn (String) 5]         (1:43)
def f fn [[g:Function x:Integer][Integer][g x]]  f ([] => [42]) 5
    interpreted  type_error: f: expected 1 return value(s), got 2 — [42 5]
    compiled     type_error: f: expected 1 return value(s), got 2 — [fn 5]
def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h (app (z:String => [z]))  (h 5)
    interpreted  signature_error: cannot call `g` — no signature matches the arguments   (1:64)
    compiled     signature_error: cannot call `` — no signature matches the arguments    (1:80)
```

**Re-measured 2026-09-06, and two of the three have MOVED** — the NUR123 deopt
increments (a fn-valued frame read is handed to the interpreter at its
statement) fixed them without this record being touched:

```
f (z:String => [z]) 5     AGREES now, message and position: `cannot call `g`` at 1:43
f ([] => [42]) 5          message agrees (`[42 5]`, was `[fn 5]`); POSITION still differs
                              interpreted 1:50    compiled 1:43
the returned-lambda (g x) UNCHANGED: `g` at 1:64 vs `` at 1:80
```

So what is left of NUR122 is narrower than the record above: one POSITION-only
divergence, and the lambda-VALUE body's empty dispatch name with its position.
The rule counts a position as part of the error (NUR108), so row two still
diverges — but its value list, which was the substantive half, now matches.
The remaining name/position pair belongs to the lambda body's own dispatch,
which is the shape NUR123 also leaves last — and the disassembly names the
mechanism, so the next increment does not start from scratch:

```
fn f1 fnval$body/2 (locals=2) [x g]:
0000 PUSH_LOCAL  l0
0001 PUSH_LOCAL  l1
0002 CALL_DYN_TRAIL_TOP      <- no BIND_DYN_SCOPE, no DEOPT_IF_FN
0003 RET
```

The fourteenth and fifteenth increments made a lambda body's BARE READ of a
fn-holding capture deopt (planDeopts over rec.wordReadNames, with
emitDynParamBinds binding params and captures registry-visibly). `(g x)` is a
paren APPLY, not a read: no word read is recorded for `g`, so no point is
planned, and the lowering emits CALL_DYN_TRAIL_TOP — which dispatches the
runtime value under ANONYMOUS semantics, hence the empty name, and stamps the
unit's position rather than the call site's. The interpreter reads `g` as a
WORD bound in the frame and dispatches it NAMED.

So the increment is to give that apply the binding NAME its dispatch lacks.

**Measured further 2026-09-06, and the family is NOT lambda-specific.** A paren
apply of a fn-typed PARAM loses the name in a plain fn body too, and there the
position is lost outright rather than merely misplaced — a simpler witness than
the one above:

```
def app3 fn [[g:Function][Integer][(g 5)]]  app3 (z:String => [z])
    interpreted  signature_error: cannot call `g` … (1:37)
    compiled     signature_error: cannot call `` …  (source position unknown)
```

Two boundaries, both measured: the divergence is on the ERROR path only — the
succeeding twin `app3 (z:Integer => [z mul 2])` is 10 on both lanes — and the
no-paren spelling `[g x]` inside a lambda REFUSES instead ("fn app2: body result
of unknown provenance") — a refusal the interpreter absorbs, and a defect in
its own right.

That narrows the fix below a deopt. The values already agree; only the dispatch's
NAME and POSITION are wrong. The model is `callDynFrameWords`
(eng/go/vm_dyn_words.go), which the NUR123 work built for the BARE READ case: it
carries a `DynFrameWord` table of name+position, installs each live fn under its
binding name (InstallFrameBinding, the interpreter's own param install) and
re-steps it through the interpreter's dispatch — which is precisely what makes
that path answer `cannot call \`g\``. `CALL_DYN_TRAIL_TOP` has no equivalent
table, so its head dispatches anonymously. The increment is to give it one: the
recorder knows the apply's head resolved to a named local, and the VM already
knows what to do with that name.

The interpreter reads `g` as a WORD bound in the frame: the value is
re-labelled under the binding name (NUR119) and dispatched as a NAMED fn,
so a no-match raises and a 0-arg lambda fires. The compiled lane holds the
raw runtime value: the whole-frame replay's island re-steps it under
VALUE semantics (an anonymous fn that matches nothing is data — it parks,
and the RET's count check reports the parked pair), and the paren
window's `CALL_DYN_TRAIL_TOP` dispatches it under its own empty name at
the unit's position. Every program in the family that does NOT error
agrees on both lanes; the divergence is the error lane only. This is the
error contract Stage 3's Apply kernel owes (design/FULL-COMPILATION.0.md
§6.4) and NUR119's re-label is its value half: a compiled frame local
read as a fn carries its binding name, and the apply of a NAMED value
dispatches as the interpreter's word does.

**A third witness, one layer out (measured 2026-09-05 on main, after the
eleventh increment).** The same paren spelling inside a RETURNED closure
over a CAPTURED fn — a row that COMPILES today, so the divergence is
live:

```
def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h (app add/v)  (h 5)
    interpreted  signature_error: cannot call `g`    (1:64)
    compiled     signature_error: cannot call `add`  (1:80)
```

Here the compiled lane names the CAPTURED value's own name rather than an
empty one, because the capture is a named native; the interpreter names
the param it is read through. The successful twins agree (`app (z:Integer
=> [mul 3 z])` is 15 on both lanes), so this too is the error lane only.

**Why the value replay cannot close this (measured 2026-09-05, the
attempted twelfth increment).** Admitting a lambda-value unit to the
whole-frame replay makes the bare `[g x]` family compile and answer
alike, but its FAILING dispatches still differ in their notes: the
interpreter's forward token is the word `x` and it reports `takes 1
argument, but none were supplied`, while the island re-steps the VALUE
region `[g, 5]` and reports `the argument was 5 (an Integer)`. The
replay islands values; the interpreter steps tokens. The mechanism that
reproduces the tokens is the ninth increment's DEOPT
(`CompiledFn.Body`) — see the handoff's twelfth-increment note.

**Fixed 2026-09-06 (the nineteenth increment) — the dispatch NAME and its
position.** The apply's head now carries the pair the slot could not supply.
`CompiledFn.DynApplyName` maps the pc of an `OpCallDynTrailTop` /
`OpCallDynTrailKeepQ` to the binding NAME its head was read bare under and that
READ's position, seated by both lowering routes — the body-tail apply
(`[(g 5)]`, emit.go) and the event apply (`[1 add (g 5)]`, lower.go) — from the
same `wordReadNames` / `wordReadPos` tables NUR123's replay reads. The VM builds
the no-match through `NoMatchDiag` with the APPLIED fn itself rather than
`RuntimeNoMatch`'s registry lookup, because a frame binding is a slot on this
lane and `Registry.Lookup` finds nothing. One reconciliation was needed beyond
the name: a boru fn authored without a `|` boundary carries
`BarrierPos == BarrierAllForward` (-1), which registration resolves to the arg
count (`upsertFnDef`) and the diagnostic reads via `HasForwardSigs` to decide
the "group the call in parens" help — so the interpreter printed that line and
the raw const did not. `installedSigView` resolves the sentinel for the
diagnostic only; the shared predicate also picks the DISPATCH mode for a raw fn
value at the pointer (engine.go), which this must not move.

Measured after the fix, all three witnesses:

```
def app3 fn [[g:Function][Integer][(g 5)]]  app3 (z:String => [z])
    BYTE-IDENTICAL on both lanes: `cannot call `g`` at 1:37, caret, both notes, the help
def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h (app (z:String => [z]))  (h 5)
    name and position AGREE (`g` at 1:64, was `` at 1:80); the written tuple still differs
… def h (app add/v) …
    name and position AGREE (`g` at 1:64, was `add` at 1:80); likewise
```

The boundary is the WRITTEN TUPLE, and it is the same one the attempted twelfth
increment measured: parity is byte-for-byte when the argument is a LITERAL or a
paren-computed value (`(g 5)`, `(g (1 add 1))`), and stops at the notes when it
is a WORD (`(g y)`, `(g j)`) — the pointer substitutes a word onto the value
stack before the head dispatches, so the interpreter's forward window consumed
no token and reports `takes 1 argument, but none were supplied` where the
compiled lane names the value it applied. Both lanes raise the same
signature_error under the same name at the same place, and the SUCCEEDING twin
agrees exactly (`app5 (z:Integer => [z mul 2]) 7` is 14 on both), so what is
left is a note-only residue. Closing it needs the recorder to record, per
argument, whether the interpreter's window would have WRITTEN it — a separate
seam from the head's name. Pinned in lang/go/dyn_apply_head_name_test.go,
declines included.

**The frame binding's RENAME (fixed 2026-09-07, the twenty-sixth increment).**
The family had a VALUE half after all, found measuring the closure-capture
family's blocker (b): the interpreter's frame binding renames the fn value it
binds for a named param (`installDef`: `fnDef.Name = name` for a
Function-family body), and the VM bound the caller's value verbatim, so a
row that COMPILES rendered differently on the default lane:

```
def f fn [[g:Function][Function][g/v]]  (f (z:Integer => [z])) 3
    interpreted  fn g(Integer) 3
    compiled     fn (Integer) 3
def app fn [[g:Function][Function][( fn [[x:Integer][Function][g/v]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)
    interpreted  fn g(Integer)                (the returned closure's /v read of its capture)
    compiled     refused before the increment; renders fn (Integer) without the rename
```

`nameFrameFns` (eng/go/vm.go) names a fn bound for a NAMED param at every
frame entry — `bindUnitLocals`, CALL_USER, the tail call, OpCallUserPoly and
the dyn-apply entry — for an `FnDefInfo` of THIS registry, the payload the
interpreter's rule names; a value already so named, an unnamed slot and a
compiled closure are left alone. Both rows above agree, a named fn takes the
param's name (`(f g/v) 3` → `fn h(Integer) 3`), and the nameless no-match
builder of the `/v` delivery `(g/v 5)` prints ``cannot call `g` `` on both
lanes — through the value's name, not a seated head.

Still open in this class, measured on this tree:

- **A module wrapper.** `import "boru:math-util"  def f fn
  [[g:Function][Function][g/v]]  (f MathUtil.sqrt/v) 16.0` renders `fn
  sqrt(Number) 16.0` compiled for the interpreter's `fn g(BigDecimal) or
  (BigInteger) or (Float) or (Integer) 16.0`: installDef's REBINDING path
  installs the inner native's overloads under the param's name, a
  registry-visible install the payload rename does not mirror. The wrapper
  still dispatches (`(g 16.0)` is 4.0 on both lanes); only the render of the
  value read back differs. Pinned as measured in
  lang/go/closure_capture_test.go (`TestClosureCaptureOpenShapes`).
- **The position-only row** `f ([] => [42]) 5` (interp 1:50, compiled
  1:43), unchanged.
- **A DEF-bound rename** inside a body (`def k g/v  k 2`) agrees today only
  because the compiled lane falls back on that shape.

**Narrowed 2026-09-05 (the sixth increment, NUR123).** The whole-frame
replay now re-steps a BARE-READ lead as the WORD the interpreter
dispatches (`CompiledFn.DynFrameWords`: the VM installs the fn-valued read
as a frame binding under its name and runs the region through the
interpreter's own word dispatch), so the first two witnesses above agree
on both lanes — `f (z:String => [z]) 5` raises `cannot call `g`` compiled
too, and `f ([] => [42]) 5` reports `[42 5]`. What remains is the PAREN
WINDOW spelling: `(g x)` lowers to `CALL_DYN_TRAIL_TOP` under value
semantics, and its no-match still carries the empty name at the unit's
position (the third witness). That lowering is credited as an accepted
read of `g` (NUR123's accounting), so the record stays open for it.

**Retired (2026-09-25).** Every witness agrees on both lanes now, message
and position: `f (z:String => [z]) 5` and `f ([] => [42]) 5` (the
position-only row: 1:50 on both), the returned-lambda `(g x)` (`cannot
call \`g\`` at 1:64 on both — the paren-window spelling names its head
and anchors at the read), and the module wrapper renders as NUR119's
open shape, pinned there. The last piece was the paren-window spelling's
ANCHOR: the recorded dyn-apply event and the residual trailing apply take
the lead's READ position the unit noted (`wordReadPos`, from
noteWordRead) when the lead value carries none — the check pass's carrier
for a bare fn-typed binding read — so the raise stamps at the `g` the
interpreter blames: `def ap fn [[g:Function][Any][(g 5)]]  ap
([x:Integer] => [x 1])` reads 1:31 on both lanes (NUR118's fn-value seam,
closed the same day). (Stamping the position onto the carrier itself was
tried first and reverted: the residual layout's statement-boundary test
read it and applied a no-return lambda's leaked body residual at the main
level.) Pinned by lang `TestFnValueSeamAnchorsAtTheRead`.

## NUR123 — a bare read of a fn-valued frame binding is a word dispatch the compiled lane never made {#nur123}

**Status:** FIXED 2026-09-25 (the guarded gradual read — the handoff log's
entry of that date; "The fix" below). The road, as it was logged: FIXED for
the fn-body residual and for every fn-typed
read (2026-09-05, the sixth increment), and for a GRADUAL body-local's
read wherever its statement can be handed to the interpreter (2026-09-05,
the eighth and ninth increments) and for a CODE BODY's read of the
captured local (2026-09-05, the tenth increment: `[1] each [j]`, `do
[j]`), and for a LAMBDA VALUE's own body (2026-09-06, the fourteenth
increment: `def h fn [[m:Map][Function][def j (m get "f")  ( fn
[[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)` is 42 on
both lanes, where it answered `fn`; the escaping unit seats its body
tokens after all and plans from its OWN frame — its captures ride in slots
for the whole apply — instead of the enclosing one), and inside a BRANCH
ARM or a nested `each` body of that lambda body (2026-09-06, the
fifteenth increment: `[if true [j] [0]]` is 42 where it answered `fn`,
`[[1] each [j]]` is [42] where it raised ``did you mean `h` or `q`?``); and wherever a `do` body's result is the captured local
itself — the bare `( fn [[x:Integer][Any][do [j]]] )`, its paren twin, and
the OPERAND form `[do [j] typeof]` (2026-09-06, the sixteenth increment: a
RESIDUAL-IDENTITY defect, not a missing deopt — the `do$body` unit does
carry its point, but the do's result carrier IS the captured local's, so
resolveOperand's capture-override sent the value back to the slot and the
lowering DROPped the call). The operand form was first recorded as still
open, on the reading that it took a resolution path the guard did not
reach; that was WRONG and is corrected here — the guard's own frame-floor
index was off by one (`es.frames` opens with a root frame carrying no
floor, so frame n's floor is `fragFloors[n-1]`), which put every
recording-time resolution out of range and let only the finish-time arm
fire. The declined point (a literal or a read pushed before the
statement and still pending at the test: `5  j typeof`) is GUARDED since
2026-09-25 (below) and the `/v` render closed with NUR119 (the wrapper
renders under its own name on both lanes, `WrapperUnderName`). A read inside a BRANCH ARM of a lambda body,
and an `each` body nested in one, were closed by the fifteenth increment
(2026-09-06). **Found:** 2026-09-05, measuring the closure-capture
family's blocker (a) on the tree after NUR120/NUR121 landed.

**The fix.** The last shape was a GRADUAL captured read whose statement no
island can take over — `5 j typeof` inside `( fn [[x:Integer][Any][5 j
typeof]] )` over `def j (m get "f")`, the literal pending beneath the read,
which the deferred-operand accounting declines (`deoptDeferred`) — and the
read kept its slot push SILENTLY: `typeof` over the fn value, `[5 Function]`
for the interpreter's `[5 Integer]` (`(q 7)` over `def q (h {f: ([] =>
[42])})`), a wrong VALUE the frame's return count happened to catch. A point
no island can serve is now a GUARD (`deoptPoint.bail`, `bailPoint`,
`DeoptSpec.Bail`): the read keeps its slot push and the VM tests the value
where the statement begins (`OpDeoptIfFn`'s guard arm), raising a designed
defer when it holds an appliable fn — the word dispatch the interpreter makes
there — and passing the slot push through when it does not (`{f: 3}` answers
the same on both lanes). Loud, in the bail ledger, where it was silent; a
compile-time decline of the same shape is the residual optimisation. Points
an island environment cannot bind demote to guards the same way. Pinned:
`TestGradualReadWithPendingValueGuards` (`lang/go/register_tail_test.go`).
Measured with it, the record's other open shapes agree on both lanes: `def f
fn [[g:Function][Any][g]]  f ([] => [42]) 5` is `[42 5]`, a body's `def k g/v
k 2` raises the frame's count error on both, the wrapper renders alike.

**Rule:** a program matches or refuses; the two lanes agree on values and
errors.

**Observed.** The interpreter's `stepWord` does not substitute a binding
whose value is a fn: a FnDefInfo entry "goes through normal Lookup"
(`core/go/engine.go`), i.e. a bare read of a frame binding — a param, a
capture, a body-local `def` — whose RUNTIME value is a fn is a WORD dispatch
under the binding name (`installDef` re-labels the value `Name = name` at
bind time): a 0-arg fn FIRES, an n-arg fn collects forward from the tokens
after it and from the frame's stack below it, a no-match raises
`signature_error: cannot call `g``; only a pending forward whose next slot
expects a Function takes the value as data. The check model binds a CARRIER
for the same param (a `Function` carrier through the fn-carrier side table,
an `Any` carrier through `Defs`), which no signature can dispatch, so the
read was recorded as a value and the emitter lowered it as a slot push; the
apply lowerings that later reached the value ran VALUE semantics (NUR122's
park), and a value nothing applied was RETURNED. Default lane, exit 0:

| program | interpreted | compiled, before |
|---|---|---|
| `def f fn [[g:Function][Any][g]]  f ([] => [42])` | `42` | `fn` |
| `def f fn [[g:Function][Any][g]]  f (z:Integer => [z])` | `signature_error: cannot call `g`` | `fn (Integer)` |
| `def f fn [[g:Function][Any][g]]  f add/v` | `signature_error: cannot call `g`` | `fn add(…)` |
| `def f fn [[g:Function][Any][(g)]]  f (z:Integer => [z])` | `signature_error: cannot call `g`` | `fn (Integer)` |
| `def f fn [[g:Function][Any][def y 1  g]]  f ([] => [42])` | `42` | `fn` |
| `def id fn [[x:Any][Any][x]]  id ([] => [42])` | `42` | `fn` |
| `def f fn [[g:Function][Any][g]]  def g0 ([] => [42])  f g0/v` | `42` | `fn g0` |
| `def f fn [[g:Function][Any][g]]  f (quote ([] => [42]))` | `42` | `fn` |
| `def f fn [[g:Function][Integer][g]]  f ([] => [42])` | `42` | `type_error: … expected Integer, got Function` |
| `def f fn [[g:Function][Any][[g]]]  f ([] => [42])` | `[42]` | `[fn]` |
| `def f fn [[g:Function][Any][if true [g] [0]]]  f ([] => [42])` | `42` | `fn` |
| `def f fn [[g:Function][Any][g typeof]]  f ([] => [42])` | `Integer` | `Function` |
| `def h fn [[k:Any][Any][k]]  def f fn [[g:Function][Any][h g]]  f ([] => [42])` | strict-barrier `signature_error` | `42` |

**Fixed (the sixth increment).** The ENGINE classifies the read where it
happens (`Engine.noteWordRead` → `EmitRecorder.NoteWordRead`: a fn-typed or
gradual carrier read bare with no Function-expecting forward pending; a
`/v` read is `NoteValRead`), the emitter counts the reads per unit, and at
the unit's finish a residual carrying such a read arms the whole-frame
replay whatever the count (`noteWordReadReplay`, `fnUnitRec.dynFrameWords`
→ `CompiledFn.DynFrameWords`, keyed by the op's unit-local pc). The VM's
`callDynFrame` then installs each fn-valued word-read entry as a frame
binding under its name (`InstallFrameBinding`, the interpreter's own
param install; a compiled closure is first bridged to a handler-bearing
FnDefInfo, `closureAsWord`) and re-steps the region with the WORD in the
value's place — the interpreter's own dispatch, errors and positions
included — popping the bindings afterwards; a region whose reads hold
plain data at run time is the residual already and skips the island. Every
fn-typed read must then be accounted for — seated in the window, or
consumed by a fn-value apply lowering (`creditWordRead`: the paren window,
the trailing apply) — else the unit refuses (`wordReadAccounting`): a
container member, an if-arm residual, a stack-collected argument, and a
binding read both bare and by `/v` (one value ID, two dispatch semantics)
all refuse rather than diverge — four unimplemented cases, each owed a
fix. Every row above bar the last three now agrees on both lanes
(`lang/go/word_read_dispatch_test.go`), the container and arm rows
refuse, and `[x g]`, `g 3`, the quoted arg, the returned closure and the
named 0-arg lambda agree too. A word-read lead over plain data that
matches NO prefix of the tokens after it raises the no-match natively
(`NoMatchDiag`, the interpreter's own builder — no island); the Apply
kernel's frame push still runs first, so a matching lead never islands.
A compiled closure is bridged under the lambda's OWN declared param
contract (`lamParamContract` → `CompiledFn.Params`), so `g "s"` over a
`z:Integer` lambda no-matches on both lanes (the first bridge declared
Any and ran the closure over the String — `[0]` for the interpreter's
error, caught before landing).
The two `frontier-fnparam-deref.tsv` rows (ruled 2026-08-15: a bare name
is a call) graduated into `lang/spec/fn-value.tsv` §7 and the file is
retired; the interp-entry census stays at its ceiling.

**Open.** A GRADUAL read is arming best-effort only: consumed outside the
residual it keeps the slot push, because refusing it would refuse every
`m k get` over an Any param. Which reads are gradual at the unit's finish
is the pass's doing, and it is narrower than this record's first draft
said. A gradual PARAM (`k:Any`) is re-run under the argument's RUNTIME
type — a `Function` carrier when the call passed a fn — so its reads are
accounted strictly and REFUSE, never diverge: `def h fn [[k:Any][Any][k
typeof]]  h ([] => [42])`, `[[k]]`, `[{a: k}]` all refuse ("bare read of
`k` is consumed where the interpreter dispatches it") and answer the
interpreter's Integer, `[42]`, `{a:42}` (re-measured 2026-09-05 after the
word path's nil-handler guard; the `{a: k}` spelling was NUR125's panic).
What stays gradual is a value the pass never types as a fn — a body-local
bound to a CONTAINER ELEMENT, which the Map carrier types as
`dynamic(Any)` on every run:

```
def h fn [[m:Map][Any][def j (m get "f")  j]]  h {f: ([] => [42])}
    interpreted  42          compiled  fn
… [def j (m get "f")  j typeof]]  …
    interpreted  Integer     compiled  Function
… [def j (m get "f")  {a: j}]]  …
    interpreted  {a:42}      compiled  {a:fn}
```

— default lane, exit 0, compiled without a refusal. The engine notes each
such read (`noteWordRead`: Dynamic, admits a fn); the emitter's
`NoteWordRead` did not, because the value is a producing EVENT of the unit
(resolveOperand's events-first rule), not a slot in `localByID`, and the
gate asked only for a local. FIXED for the residual spelling (the eighth
increment): a body-local producer of the unit counts as its word read
(named, gradual — never strict), the tail test anchors a word read at its
READ rather than at the value's producer (`replayIsBodyTailAnchored`: an
event between the producer and the read ran before the dispatch on both
lanes — `def j (m get "f")  def y 1  j`, `j drop  j` seat), and the VM's
word-read no-match reads the INSTALLED binding (the bridge carries the
interpreter's barrier), so the typed no-match rows agree byte for byte,
help line included. `def j (m get "f")  j` is 42 on both lanes now; `j 3`,
`x j`, `m.f`, the list element, `(j)` and a returned closure bound to a
body-local likewise (`lang/go/word_read_dispatch_test.go`,
TestBodyLocalWordReadParity). The rest DEOPTS (the ninth increment,
OpDeoptIfFn): the read's statement — at the read's push when its consumer
follows it on the stack (at the read itself when an event runs between
them), at the forward word, def, `if` or `for` that collects it, at the
literal or paren it sits in (`(j typeof)`, `(j) typeof`, `[(j typeof)]`,
`def k (j)`) — tests the binding's value at run time and hands the rest
of the body to the interpreter when it is a fn: `j typeof` Integer, `{a:
j}` {a:42}, `(j typeof)` Integer, `if true [j] [0]` 42, `for 1 [j typeof]`
Integer, `j (m get "g") add` 43, `j  def y 1` 42, `j j add` 84, `def n (m
size)  def j (m get "f")  j  n drop` 42, `def k j  k` the interpreter's
error, a fn's effects once (TestBodyLocalDeoptParity, 61 rows). A CODE
BODY's read of the captured local deopts too (the tenth increment): the
closure unit's point keys on its capture slot and the enclosing unit
binds the name registry-visibly (seedParentDeopt) — `[1] each [j]` [42],
`[1 2] each [j]` over a 1-arg lambda [3 6], `do [j]` 42, `do [j typeof]`
Integer, `if true [do [j]] [0]` 42, `5  do [j]  add` 47 (eleven more
rows; `for 2 [j]` and `[1 2] fold [j] 0` agree on value and message and
differ in the count error's position, NUR118; a lambda value's body,
`[1] each [x:Integer => [j]]`, refuses — itself a defect). STILL OPEN: a
lambda value's own body (`([] => [j])` renders `fn` on both lanes and escapes
the frame its binding lives in); a point whose statement the compiled stack
cannot match declines and keeps the slot push (`5  j typeof`, `def y 5  y
j typeof`: a literal or a read pushed before the statement and still
pending at the test — both lanes raise the count error, `[5 Function]`
for `[5 Integer]`; `x {a: j} size add`: a param read before the
statement, consumed after it; an island that would spell a body-local FN
def, `def w fn […]  (j typeof)  w`, which no bind can make
registry-visible); `j/v` renders `fn` for `fn j` (NUR119). (The interpreter's no-match NOTES used to list the frame's
DefCleanup marker as a stray argument — `… and __dc (a __DC)` — because
the written-tuple walk did not stop at engine markers; fixed with this
increment, `isEngineMarker`, so the two lanes' notes agree byte for byte.)

## NUR124 — a shuffled fn re-steps AT the shuffle on the interpreter, and a produced closure not at all on the compiled lane {#nur124}

**Status:** FIXED 2026-09-25 (the value-delivered window parks — the handoff
log's entry of that date). **Found:** 2026-09-05, measuring the closure
bridge of NUR123.

**The fix.** Both axes closed on 2026-09-07 (the twenty-fourth and
twenty-fifth increments below). What stayed open was the FIFTH witness,
`def appv fn [[g:Function][Integer][(g/v 5)]]  appv (z:String => [z])`: a
`/v` delivery inside a paren is a VALUE — the interpreter applies it when an
overload takes the window (`(g/v 5)` over an Integer g is 5) and otherwise
leaves it as data beside the window (`[fn g(String) 5]`, the frame's count
error) — while the VM stripped the stored value's quote and raised `cannot
call `g`` through the nameless no-match builder. `callDynTrailTop` now parks
a value-delivered head (the one head the recorder seats no name for; an
event-produced lead never reaches the op) that matches nothing, in the
window's WRITTEN order (`DynApplyHead.Leading` / `WrittenFirst`, seated for a
nameless head too): both lanes raise `expected 1 return value(s), got 2 — [fn
g(String) 5]`. Measured the same day, every other witness of this record
agrees on both lanes — `[(mk 3)] each [5 swap]` [15], `[5 over]` [45], `[5
swap drop]` the each_error, `[dup drop 5]` [5], `5 (mk 3)` `5 fn`, `[g/v]
each [5 swap drop]` the each_error — and `[g/v] fold [5 swap drop] 0` and
`m get "f" drop 7` DECLINE loudly (a Stage 3 residual, the NUR124 re-step
gate) where the interpreter answers. Pinned: `TestDynApplyHeadNameNamelessArm`
(`lang/go/dyn_apply_head_name_test.go`), a parity pin now.

**Rule:** a program matches or refuses; the two lanes agree on values.

**Divergence, measured on the default lane, exit 0:**

```
def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  [(mk 3)] each [5 swap]
    interpreted  [15]              compiled  [fn (Integer)]
… [(mk 3)] each [5 over]
    interpreted  [45]              compiled  [fn (Integer)]
… [(mk 3)] each [5 swap drop]
    interpreted  each_error: body produced no result     compiled  [5]
… [(mk 3)] each [dup drop 5]
    interpreted  [5]               compiled  [5]
```

**A fourth witness, measured 2026-09-06, running the OPPOSITE way** — here
the compiled lane APPLIES where the interpreter parks:

```
def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  5 (mk 3)
    interpreted  5 fn (Integer)    compiled  15
```

Pre-existing: identical on `6f583f6` (before the fourteenth increment) and on
the current head, so it is this record's, not a regression from the NUR123
line. It also narrows the "top-level spellings refuse" note above — that holds
for `5 (mk 3) swap` and `5 g/v swap`, but the bare `5 (mk 3)`, with no shuffle
word at all, compiles and applies. The paren bounds ONE survivor and the `5`
was already on the stack, so the park rule leaves both values
(design/PAREN-RESTEP-RULE.0.md); the compiled residual applies the lead
anyway. The `(mk 3) 5` twin agrees on both lanes (`fn (Integer) 5`), which is
what places the fault on the value's ARRIVAL above an existing residual rather
than on the apply itself.

**A fifth witness, measured 2026-09-06 while covering the nineteenth
increment's nameless arm** — same direction (the compiled lane applies where
the interpreter parks), reached by a DIFFERENT mechanism:

```
def appv fn [[g:Function][Integer][(g/v 5)]]  appv (z:String => [z])
    interpreted  type_error: appv: expected 1 return value(s), got 2 — [fn g(String) 5]
    compiled     signature_error: cannot call `` — no signature matches the arguments
```

Pre-existing: byte-identical on `120fb37`, the commit before that increment.
The mechanism is not the park rule but the QUOTE: `callDynTrailTop` strips the
applied copy's construction-time quote to mirror a read-substituted arrival
(the strip is deliberate and documented at its site — a still-quoted fn
islanded as inert miscompiled `[1 2] each [(1 2 c)]`), while a `/v` delivery
hands the SLOT's stored value over still quoted, which the interpreter leaves
as data inside the paren. `RecordDynApply` declines an inline-quoted fn for
exactly this reason; a `/v` read reaches the op with the carrier unquoted at
record time and quoted at run time, so the decline does not catch it. Pinned
as a decline (not a parity row) in lang/go/dyn_apply_head_name_test.go, where
it is also what keeps the nameless no-match arm reachable: `g/v` is a value
delivery, not the word dispatch the interpreter would name.

**The written-tuple half is FIXED** (2026-09-07, the twenty-first increment).
Both ops now hand `NoMatchDiag` the leading run rather than every applied
argument, and all eight witnesses below reach parity — the three
`OpCallDynTrailTop` rows (`(g y)` over a param, over a const-folded body-local,
over a computed one) and the five `OpCallDynFrame` rows (`(g 5 y)`, `(g y 6)`,
`(g 5 6 y)`, `(g 5 y 6)`, `(g y 5 6)`).

The signal is a READ of a binding on the unit, bare (`fnUnitRec.localReads`,
which `NoteLocalRead` already populated ungated) or through a value reference
(`valReads`). Both are SUBSTITUTIONS — the pointer replaces the token with the
binding's value before the head dispatches — so neither reaches the written
tuple: `def f fn [[g:Function y:Integer][Integer][(g y/v)]]` reports `takes 1
argument, but none were supplied` exactly as the bare spelling does. The first
cut consulted only the bare half and counted the `/v` one as written, a
divergence that predates this work (identical on `f0d208c`) and that the fix
would otherwise have carried forward unclosed; found by a review bot on #441,
whose finding named the right incompleteness with the direction reversed.
`(g y/v y)` and `(g y y/v)` are parity either way — the bare read of the same
id stops the run regardless — so only the lone `(g y/v)` separates the two
readings. The count itself: `writtenRun` counts the leading arguments with no
recorded read, the recorder seats that count on the trailing apply's site
(`DynApplyHead.NWritten`) and marks each replay region entry
(`DynFrameWord.Read`), and the two VM diagnostics slice their arg window to it.
`noteWordRead`'s fn-admitting gate is untouched — it answers NUR123's "does
this read DISPATCH", a different question from "was there a read at all".

One contract had to be preserved on the way: `dynFrameWordsFor` returning nil
means "this window carries no word read", and callers arm the replay on it.
Marking reads on a table allocated for a read ALONE widened that signal and
refused three module rows that used to compile (measured, not predicted —
TestFnUnitLoopApply*, TestModuleReadNoRebindStillCompiles). The marks now ride
only on a table that already exists for a NAME.

**What the twentieth increment measured, and why the rule needed correcting**
(2026-09-07). What the nineteenth recorded — "a literal or
paren-computed argument is written, a word read is not", read as a per-argument
FILTER over three rows — is wrong in three ways. Everything below is
pre-existing, byte-identical on `120fb37`.

It is a PREFIX, not a filter. Forward collection walks the tokens after the
head LEFT TO RIGHT and stops at the first WORD read, which the pointer had
already substituted onto the value stack:

```
(g 5 6 y)   interpreter's written tuple  [5 6]
(g 5 y 6)                                [5]     <- a FILTER would say [5 6]
(g y 5 6)                                []
```

It spans TWO ops. Those multi-argument rows lower to `OpCallDynFrame`, the
whole-frame replay, whose diagnostic hands its whole arg window to
`NoMatchDiag` exactly as the trailing apply's did. The one-argument family
could not have shown either fact: over one argument a prefix and a filter are
the same function, and multi-argument applies never reach
`OpCallDynTrailTop` at all.

And the discriminator is SYNTACTIC, not an operand kind. A body-local bound to
a literal folds its read to `PUSH_CONST` — the same operand a written literal
produces — and is still not written:

```
def f fn [[g:Function][Integer][def y 7  (g y)]]   PUSH_CONST 7, written []
def f fn [[g:Function][Integer][(g 5)]]            PUSH_CONST 5, written [5]
```

So the fix keyed on operand kind that the nineteenth increment proposed would
have answered that row wrong while looking right on every literal row. What it
needs instead is the SYNTACTIC fact — was there a bare read? — and that signal
already exists, ungated: both engine bare-read sites call `NoteLocalRead(id,
pos)` unconditionally beside the fn-gated `noteWordRead`, recording every bare
read's value ID on the innermost open unit (`rec.localReads`). Probed over this
family it discriminates exactly, the const-folded row included — `(g 5)` and
`(g (1 add 1))` read 0, while the param, the folded body-local and the computed
body-local all read 1. So `noteWordRead`'s gate stays as NUR123's fn-dispatch
signal and is NOT relaxed; the count of leading zero-read arguments is what the
two diagnostics need. Pinned in lang/go/dyn_apply_head_name_test.go
(TestWrittenTuplePrefixRule, TestWrittenTupleConstFoldedLocalDeclines), which
fail if any of the three findings moves.

**The trailing-apply half is FIXED** (2026-09-07, the twenty-second
increment). The withdrawn fix below needed one signal, and the signal exists:
`apply` records through `RecordCall` as an IDENTITY — the check engine returns
the fn concrete and re-steps it, so `args[0].ID == outs[0].ID` — and that ID is
exactly the one `trailingApply` meets as its `fnv`. `EmitState.appliedByWord`
marks it PROGRAM-wide, which is what the unit-scoped `pendingApply`
structurally could not (it returns false outright when no fn unit is open, the
program residual's case). `trailingApply` now declines a parked result unless a
trailing `apply` word claimed it.

Measured: `10 (mk2 5) apply` — the corpus row the previous attempt refused —
still compiles and answers 11; `5 (mk 3)` no longer answers 15 but REFUSES
("residual shape beyond Stage 1 (call result above a literal)", an EXISTING
site, so the refusal-site census does not rise), and the default lane answers
`5 fn (Integer)` on the interpreter. `(mk 3) 5`, `5 (mk 3) 7` and `1 2 (mk 3)`
are unchanged. Corpus differential and refusal ceiling both pass. Pinned by
TestApplyWordClaimsParkedResult in lang/go/returned_closure_park_test.go.

What is left of NUR124 is the SHUFFLE family, and it is TWO defects rather than
the one this record described (measured 2026-09-07, the twenty-third
increment; every row pre-existing).

**Axis 1 — TIMING, and it is not about the closure at all.** The record framed
this family as a produced CLOSURE the compiled lane parks. But a plain
`FnDefInfo` diverges too, as soon as anything follows the shuffle:

```
def g fn [[x:Integer][Integer][x mul 3]]  [g/v] each [5 swap drop]
    interpreted  each_error: each: element 0: body produced no result
    compiled     [5]
```

Trace it — `[g] 5 → [g,5] swap → [5,g]`. The interpreter applies g THERE,
giving [15], and `drop` then empties the stack, which is the each_error. The
compiled body leaves g in place, `drop` removes it, and the body ends [5]. So
the interpreter re-steps a shuffled fn AT THE SHUFFLE and the compiled body at
BODY END, if at all.

`[g/v] each [5 swap]` and `[g/v] each [5 over]` PASS on both lanes — and they
are trap rows, agreeing only because nothing follows the shuffle, so the two
timings coincide. A family assembled from them would report this fixed. Note
the `/v` prefix carefully: the CLOSURE spellings of the same two bodies,
`[(mk 3)] each [5 swap]` and `[(mk 3)] each [5 over]`, are the Axis 2
divergences above — the body alone does not say which row you are looking at.

**Axis 2 — a compiled closure is not re-stepped even at body end.**
`[(mk 3)] each [5 swap]` answers `[fn (Integer)]` where the FnDefInfo twin
answers [15]: same body, and only the element's payload differs
(`core.ClosurePayload` against `FnDefInfo`).

**Where both come from.** `eachHandler` is the SAME handler on both lanes —
the island runs the same `each` word token through the sub-engine and reaches
the same `InvokeBody`. What differs is what it is handed: the compiled body
arrives as a LIST VALUE whose elements happen to include Word values
(`[5, word(swap)]`, probed), where the interpreter's body is a CODE block off
the tape. Running the former is not stepping the latter, and that is what
loses both the shuffle-time re-step and the ClosurePayload dispatch.

`[(mk 3)] each [dup drop 5]` is the control: a body that never puts the fn on
top agrees on both lanes.

**The site was located, and one fix was tried and WITHDRAWN before it**
(2026-09-06). The
boundary is exact: the residual must be exactly `[literal, 1-arg closure]` —
`(mk 3) 5`, `5 (mk 3) 7` and `1 2 (mk 3)` all agree, and the rest of the family
refuses. That is the shape `resolveDynamicApply`'s `trailingApply` helper
accepts: an event-produced single-output fn on the sim top over a plain static
arg. Every SIBLING arm of `resolveDynamicApply` consults the park rule
(`callResultPlaced` / `placedNotReStepped`); `trailingApply` checks only the
shape, so the arity-1 apply fires on a value the paren placed with one
survivor.

Adding `callResultPlaced` there does remove the miscompile — the row refuses
("residual shape beyond Stage 1 (call result above a literal)"), trading a
wrong answer for a refusal, which is a defect of its own — and it costs a
corpus row against a refusal ceiling of 0, so it was withdrawn:

```
recursion.tsv:L48  def mk2 fn [[x:Integer] [Function] [([x:Integer] => [x add 1])]] 10 (mk2 5) apply
```

That row parks the result exactly as `5 (mk 3)` does and then APPLIES it,
because the trailing `apply` word dispatches the parked value on purpose. So
the guard needs a signal separating "a later word dispatches this parked value"
from "nothing does". **`applyPending` is NOT that signal** — measured: the
`apply` row still refuses with it excluded. The code says why, so the next
attempt need not re-measure it: `applyPending` reads
`es.units[len(es.units)-1].pendingApply`, a UNIT-scoped list, and returns false
outright when no fn unit is open — which is the program residual's case. The
registration site is explicit about the same boundary: "Top-level applies
(`len(units) == 1`) keep today's refusal: the program residual has no
equivalent single-consumer window." So the mark is not merely absent here, it
is structurally unavailable by design. Two things follow for the next attempt:
the fix site is `trailingApply`, and the missing discriminator has to be
program-level provenance for the `apply` word — the pending-apply list cannot
supply it.

Two shapes the fix must also leave alone, both already passing: a member or
dynamic read (`5 m.f`, `[..] r.one-of`) is no call result and must keep the
arm; and the paren-bounded `(a b comp)` shape collapses at the paren
(engine.go's trailing fn-value apply) and never reaches here.

A native word's returned fn is RE-STEPPED where it lands
(design/PAREN-RESTEP-RULE.0.md: the park applies to a USER fn's single
result only), and a stack-shuffle word that moves a closure to the top
returns it: the interpreter applies it to the value now beneath it
(`5 swap` → 15, `5 over` → 45). The compiled lane trusts the shuffle over a
dynamically-shaped stack (`DynStackShuffleWords`) and lays the closure out
as data. The top-level spellings (`5 (mk 3) swap`, `5 g/v swap`) refuse
("computed closure at a word's argument slot", "function value reaches
swap (Stage 3)"); the code-body spelling over a produced closure is the
one that compiles. A module-scope named lambda (`[g/v] each [5 swap]`)
agrees on both lanes (`[15]`) — the re-step rule is keyed on the value's
origin, which is itself the ADR-016 hazard.

**Axis 1 (TIMING) is FIXED** (2026-09-07, the twenty-fourth increment). The
rule the interpreter applies is the one `spliceMatchResults` encodes: a
native word's results go back onto the tape at the call's position and the
main loop steps them, so an unquoted fn among them dispatches WHERE IT LANDS
— collecting forward over the tokens written after the call, then from the
stack beneath it (`[5 swap 7]` is 21, `[5 tuck]` is 15, `[5 swap drop]`
applies g to 5 before drop runs). The check pass cannot make that dispatch: a
fn-typed CARRIER has no signatures, so `stepLiteral` stepped it past as data,
and the unit compiled from that model kept the value inert where the runtime
applies it. Three pieces close it:

- the pass NOTES each such result at the call (`Engine.noteFnResultReSteps` →
  `EmitRecorder.NoteFnResultReStep`): a fn-typed or fn-admitting gradual
  carrier, unquoted, not a user fn's (parked) result, not a code-body word's
  (the body's own residual, which the closure lowering models — `do [mk 7]
  add 1` keeps NUR121's refusal), not a result a pending forward collects,
  and only when a PLAIN body token follows — the group's close, a `/v`
  modifier, a marker, a placed value or the end of the tape leave the point to
  the residual arms and the park rule, as before;
- the recorder plans a RE-STEP point on the producing event (`planReStepDeopts`,
  emitted by `emitReStepAfter` right after the event's op) when the resume
  position is a token of the unit's body and the compiled stack at the point
  holds what the interpreter's holds — the deferred-operand accounting of
  NUR123's points, run from the RESUME token for a point that sits AFTER its
  event; a fn-TYPED note no point serves REFUSES at the lowering (a lowerer
  decline, not a new `MarkUncompilable` site), a gradual one keeps the model;
- the VM re-steps: `DEOPT_IF_FN` with `Results` tests the results on top with
  the main loop's own predicate (`core.FnValueDispatchesAtPointer`) and, on a
  fn, runs the island over `[results…] ++ Body[Token:]` as TOKENS — a fn value
  stepped at the pointer dispatches exactly as on the interpreter — above the
  frame region, with the unit's UNPUSHED UNNAMED inputs seated beneath it
  (`DeoptSpec.Prefix`): the interpreter's frame holds those on its stack
  bottom, the compiled unit in slots, and the first island over `[5] each
  [{f: g/v} get "f" drop]` ran `g drop` over an empty region and raised
  `uncalled_function` for the interpreter's `each_error`. The prefix rides on
  NUR123's points too.

Measured: the eleven witnesses in `lang/go/restep_deopt_test.go` agree and
compile; `[5 swap]`, `[5 swap 7]`, `[5 tuck]` and `[dup drop 5]` keep their
answers (the residual arms own a trailing fn; the each islands stay islands);
`[[g/v] get 0]` still dispatches the concrete element in the pass. Corpus
differential, refusal ceiling, refusal-site census (93, unchanged) and the
frontier ledger pass.

**Axis 2 (the PAYLOAD) is FIXED** (2026-09-07, the twenty-fifth increment).
`[(mk 3)] each [5 swap drop]` answered `[5]` compiled for the interpreter's
`each_error`, `[5 swap]` `[fn (Integer)]` for `[15]`: a produced closure is a
`ClosurePayload`, not an `FnDefInfo`, so wherever the interpreter met one —
an `each` island's sub-engine re-stepping the shuffled element, or the
compiled unit's re-step deopt testing swap's results — it stepped past what
it would have dispatched. Three pieces, and a fourth the fix uncovered:

- `execFnDefLiteral` asks the VM piece for the fn a closure stands in for
  (`CompiledRuntime.ClosureAsFnDef`, the value-path twin of NUR123's
  `closureAsWord`): one signature over the unit's declared param contract,
  applied through the registry's body-closure invoker, ANONYMOUS as the
  source lambda (`CompiledFn.Lambda`) so a 0-arg lambda VALUE nothing calls
  parks exactly as the interpreter's does (`[(mk0)] each [dup drop]` is `[fn]`
  on both lanes). The bridged value takes the closure's place on the tape.
- `compileFnDef` keeps a Go handler on an anonymous fn: it attached the
  boru body-runner to EVERY anonymous sig, and the bridge's empty body ran
  an empty frame that returned the argument itself (`[15]` came back `[5]`).
- The re-step deopt's runtime test (`core.FnValueDispatchesAtPointer`) admits
  an unquoted closure, since the island it hands the results to now
  dispatches one.
- The bridge exposed a PRE-EXISTING miscompile of the residual WINDOW
  islands: `OpCallDynamicMixed` re-steps its window verbatim, so a PARKED
  user-fn result — placed data the interpreter never re-steps — was applied
  live: `5 (mkf 3) 7` answered `5 21` and `1 2 (mkf 3)` `1 6` on main for
  the interpreter's `5 fn g(Integer) 7` / `1 2 fn g(Integer)` (a NAMED fn
  value; the closure twins agreed only because the sub-engine could not
  dispatch a closure). Both window arms now decline a `callResultPlaced`
  lead, and the residual lays the parked pair out as data on both lanes.

**Still open in NUR124, each measured on this tree:**

- **The FOLD.** `def f fn [[h:Function][Any][[5 h/v] get 1 drop 7]]  f g/v`
  answers 7 for the interpreter's `uncalled_function`: `tryFoldStaticIndex`
  hands the param's own carrier back as the fold's result, so there is no
  event to test after and nothing is noted; the map twin `{a: h/v} get "a"
  drop 7` (a poly `get`, an event) now raises `call to 'h'` on BOTH lanes (the
  twenty-sixth increment's frame rename, `nameFrameFns`), with only its
  position differing — the interpreter's at the read (1:75), the compiled
  lane's at the call site (1:100).
- **The MAIN program.** `def m {f: g/v}  m get "f" drop 7` answers 7: the
  main code seats no body to resume into, and the gradual note (the map's
  value type is unknown) keeps the optimistic model, as NUR123's declined
  points do; `[g/v] fold [5 swap drop] 0` answers `fn g(Integer) 0` for the
  same reason one level up — the fold ISLAND's result is a strict `Any`
  carrier the interpreter re-steps into `g 0`. A main-level re-step needs the
  rest-of-program island the design's Stage 5 regions describe.

## NUR173 — a reach-lowered group's lone survivor is re-stepped interpreted and pushed as data compiled {#nur173}

**Status:** FIXED 2026-09-20 for the reach-group family (`OpReStepLanding`).
What it does not yet reach is named at the end.
**Supersedes the diagnosis of** [NUR169](../NUR.md#nur169), whose MECHANISM was right
and whose SEAT was one function away.

**Rule:** a compiled program answers as the interpreter does.

**Divergence** (reachable on `main` before this fix, no gate lifted, no
instrument — `boru run` alone):

```
def h fn [[] [Integer] [42]] end
def mk fn [[] [Map] [{f: h/v}]] end
def m (mk) end
m.f
  interp:    42
  compiled:  fn h        <- silent
```

**The seat.** `m.f` is not a dot operator at the tape level: it lowers to the
REACH GROUP `( m dot f )`. That collapse never parks — an unmarked dot-read of
a function is a CALL (NUR038), so `fnReturnPark` declines a reach group by kind
— and the rewind therefore lands ON the one value it leaves and `stepLiteral`
RE-STEPS it. The interpreter holds a concrete value there and its own step
settles the question. An analysis pass holds a CARRIER and steps past it as
data, and nothing downstream recovers the fact:

- `recordParenReStep` excludes reach groups by name, because its contract is
  the MORE-than-one-survivor case (a user paren with one survivor parks, so a
  re-step there means the park declined);
- every fn-value-call arm of `resolveDynamicApply` tests `len(residual) >= 2`
  — each needs an argument for the lead to take — so a lone survivor reached
  no arm at all.

That second half is exactly NUR169's "**no case for `count == 1`**". Its
mechanism was right. It named `stepCloseParen`'s recorder switch, one function
from `recordParenReStep`, where the live half of the exclusion is.

**The correction to this page's own first draft (2026-09-20, earlier the same
day).** The superseding note said *"not the paren — bare and parenthesised
agree in every row"*. Both spellings lower to the SAME paren, so that control
varied nothing. **A control that cannot vary its variable proves nothing**, and
this is the second measurement bug of the same week — the first was a counting
regex blind to its own subject (PR #478). Vary the axis at the level the
MACHINE works at, not the level the source is written at.

**What was measured, and what it narrows the defect to.** Only the **0-arg
landing** diverges. A member that takes arguments already compiled correctly,
forward and from the stack:

| probe | interp | compiled (before) |
|---|---|---|
| `m.f` (0-arg member, event-result map) | 42 | `fn h` |
| `(m.f)`, `(mk).f`, an `if`-arm map, inside a fn body | 42 | `fn h` / `[]` |
| `m.f add 1` | 43 | `[]` |
| `5 m.f` | `5 42` | `42 5` |
| `m.g 21` (1-arg member) | 22 | 22 |
| `m.k 3 4` (2-arg member) | 7 | 7 |
| `5 m.g` (1-arg, stack arg) | 6 | 6 |
| `m.g` alone (1-arg, nothing to collect) | `fn a1(Integer)` | `fn a1(Integer)` |
| `def m {f: h/v} end m.f` (literal map) | 42 | 42 |

The literal-map row compiles because the member is PINPOINTED: a concrete
receiver plus a concrete key lets `tryMemberFnArrivalDispatch` claim the arity
and emit a guarded `OpCallDynMethod`. An event-result map resolves no member at
all, so no model can claim anything.

**Why the obvious fix is not available.** Declining the read instead — a
carrier receiver whose member type still admits a Function — was built and
measured: the corpus went **53 -> 282** compile failures. Nearly every computed
container is `Map`-of-`Any` to the check pass, so "could this member be a fn?"
is statically almost always yes. There is no static answer here; the decision
belongs to the runtime value.

**The fix.** Three parts, all additive:

1. The collapse records the fact it alone knows — `CheckState.
   ReachReSteppedFnIDs`, the third sibling of `ParenPlacedFnIDs` /
   `ParenReSteppedFnIDs` (`core/go/engine.go`, `recordReachGroupReStep`).
   **SUPERSEDED the next day by [NUR174](../NUR.md#nur174)**, which found the fact is a
   property of the STEP rather than of the producer, closed the `get`-word twin
   with it, and deleted this apparatus. The rest of this page stands.
2. `check`'s `noteReStepLanding` — the LAST model in `stepLiteral`'s chain,
   after the three that can resolve a member and claim an arity — NOTES the
   producing event as owing a landing. It consumes nothing, splices nothing and
   declines nothing, which is the whole reason it is safe to put last: the pass
   keeps stepping the same value, and every model keyed on its id sees exactly
   what it saw before.
3. `OpReStepLanding` (`eng/go/vm.go`), emitted right after the producing
   event's own op, islands an unquoted appliable value ALONE — `Run` over one
   token IS `stepLiteral`'s re-step — and leaves anything else exactly where it
   is. A signature that matches runs; one that does not leaves the fn as data,
   with no no-match raised. That last clause is why `OpCallDynTrailTop` over
   zero args could not be reused: it raises where the interpreter answers
   `fn a1(Integer)`.

An ordinary non-fn container read pays one type test and nothing else.

**Four ways the note stands aside**, each measured into place:

- A **collectable token** written after the survivor. The island runs the value
  ALONE; the interpreter's re-step does not — `execFnDefLiteral` matches over
  the live tape, so a fn with parameters collects what follows.
  `Cli.parse {name:"x" flags:{}} ["x"]` is that shape, and the alone-island ran
  the export's body with its parameters unbound. A boundary or a WORD leaves
  nothing to take (`MatchSignature`'s forward phase stops at a function word),
  so those still land.
- A **MULTI-result event**: the landing tests ONE value, and which of several
  the rewind lands on is not this model's to guess.
- A **variadic producer** (a loop, a variadic branch), whose result is a
  runtime-variable REGION where the landing tests one value.
- A value with **no producing event** to hang the op on.

A `def`-BOUND read is NOT among them, and that is why the op is emitted at the
top of `seatCallResults` rather than after the event's own ops: that is the one
moment the result is on the stack on every path, promoted to a frame slot or
not. Emitting it after the seating skipped every `def x m.y` — most of them.

**None of the four declines**, deliberately. Declining them was built and
measured at **53 -> 181** corpus compile failures, because `def x m.y` is the
commonest shape in the language. So the ledgers do not move and no compile
failure site is added; what the landing cannot seat keeps today's paths and is
named below.

**Measured cost, and what proves the fix.** Every gate is unchanged: the four
corpus ledgers (53 / 284 / 32 / 111 / 52), `MarkUncompilable` sites at 92, and
the generated sweep diffed cell by cell against a clean-worktree baseline —
zero regressions, zero movement, DIVERGED 18 and call-form failures 200 in
both. The sweep does not move because it has no statement-tail member read off
an EVENT-RESULT container; its own `container` seeds all read a LITERAL map.

What proves the fix is **twelve new rows in `lang/spec/fn-value.tsv` §8**,
every one of which answered wrongly before it — the witnesses above plus the
must-not-regress arities, run on both lanes by `TestSpecProd` and the compiled
differential.

**One more trade the gates named, and it is worth keeping.** The landing's first
VM draft islanded every applied value — the interpreter's own one-token re-step,
semantically exact — and the censuses counted 21 extra interpreter entries for
it (engine entries 422 -> 442, interp-entry rows 78 -> 100). An island IS an
interpreter entry, and this project counts every one. The op now takes
`callDynTrailTop`'s ladder instead — native apply, then `dynApplyEnter` into the
matched compiled unit, island only as a last resort — and both gates return to
their ceilings. The last row to come back was `module-rand.tsv:L16`
(`[10 20 30] r.one-of`), which already compiled correctly and was paying an
island for nothing: a module DELEGATION wrapper is asked `MatchFnSig(v, nil)`
here, unlike in `noMatchIfSigged`, because a wrong "no" costs a landing this
model would have skipped anyway where a wrong "yes" costs an interpreter entry
on every read of one.

**What remains.**

- ~~**The `get`-WORD twin.**~~ CLOSED 2026-09-20 by [NUR174](../NUR.md#nur174) — and
  not in the way this line predicted. It guessed "a different recording site";
  the answer was that there should be no recording site at all, because the
  model already stands where the interpreter decides.
- **A collectable token written after the survivor** — the first stand-aside
  above. Closing it means islanding the window the interpreter's forward
  collection would take, not just the value: `OpCallDynamicMixed`'s shape,
  over a window the landing would have to claim.
- **A VARIADIC producer's region top**, whose re-step belongs to the
  mark-window machinery (`OpCallDynMixedFromMark`).
- **The sweep's two `def container` CRASH cells** (`CALL_DYNAMIC underflow`
  under `fn-body`, `SWAP underflow` under `module-body`) are still open. Their
  seed reads a LITERAL map whose member is an ANONYMOUS lambda, and the first
  draft of this fix closed both, so the shape is reachable from here — but
  which stand-aside holds them has not been measured, and this page has
  already been wrong once today about a cause it had not measured.

## NUR190 — a dynamic fn value under a function word its `/q` or Any-typed overload claims {#nur190}

**Status:** FIXED 2026-09-26, twice over, composed at the merge of main's
#513 (2026-09-26): the branch's landing island and skip (the handoff log's
entry of that date — **The fix** below) and main's sealed claim target
(main's #513 — **The claim** below). The merged landing tries main's claim
FIRST (it stays compiled), then the island, then the skip; a landing with
none of the three defers loudly. Main's #513 recorded its `/q` half fixed
and the Function-typed reference contained; on the merged tree that half
is gone with the run's NUR078 (below). Before: CONTAINED 2026-09-24
(NUR190's open halves deferred — the handoff log's entry of that date), the
maintainer's call: the `/q`
capture and the Function-typed reference DEFER loudly at the landing's
walk and the corpus keeps them on the runtime-defers ledger
(runtime_defers.tsv). The Function-typed half is GONE with NUR078
(2026-09-26): a bare `z` calls at every slot, so `m.g z` is the named
no-match on both lanes, and the reference `m.g z/v` is a collected value
the residual arms apply (7 on both lanes); the landing's `vm:landing-claim`
arm was retired as unreachable. The `/q` capture stays contained. PARTLY FIXED 2026-09-23 (the landing's overload
walk — the handoff log's entry of that date). RECORDED and pinned
pending earlier the same day (the named fn value's candidates), found
when that increment's word-after rule turned fn-value.tsv's L317/L318
(`m.f z`, `m get 'f' z`) into `[fn h(Atom) z]`: the rows pass on `main`
and here by COINCIDENCE. Present on `main` (a worktree at 90d557b),
silent, default lane, exit 0.

**The fix (2026-09-26).** The `/q` claim needs the word's compiled call
skipped and every op after it re-planned, and the landing now does both.
Where the word is in the body at the landing's own depth — a token of the
program, of a user paren (re-opened), of a fn or closure body in an island
environment — the landing carries an ISLAND (`LandingWord.Deopt`,
`landingIsland`): on the claim the VM hands the interpreter the value and
the body from the word on, over the frame region beneath (empty — the walk
runs only then), and continues at the unit's RET or the program's end with
the island's residual (`landingDeopt`; the program's tokens ride on the
recording, `SetRootBody`). Inside a branch arm, a loop body or a literal's
member no body can be rebuilt, but every such landing that compiles is
followed at once by the word's single call and the paren apply that
consumes the value and its result: the landing carries a SKIP past both
(`LandingWord.SkipTo`, `seatLandingSkip`), and on the claim the capture
runs over the value and the word alone and seats the results the apply
claimed (`landingSkipCapture`, `landingSkip` until the merge of main's #513; a different count defers). Every placement probed
agrees and compiles — `m.f y` `[y]`, `(m.f y)`, `((m.f y) 3)`, `if true
[(m.f y)] [0]`, `for 1 [(m.f y) drop]`, `[(m.f y)]`, `{a: (m.f y)}`, a fn
body, a callback body — and fn-value.tsv L317/L318 left the runtime-defers
ledger (the defer census 7 -> 5; the two rows run one landing island each,
engine entries 183 -> 185, interp-entry rows 36 -> 38). Probing the class
found NUR271 (a dyn body's settled lead re-applied at the program's
residual), fixed with it. Pinned: lang `TestNamedFnCandidatesOpenShapes`
(fifteen rows, every placement), the lang ledger's bail line 40 -> 37.

**The claim (the `/q` half, 2026-09-26).** The word's compiled call and
the residual apply CAN be skipped where the lowering proves their layout:
when the residual apply the program emits is `OpCallDynamic /1` whose fn
operand is the landed event's one result and whose argument is the word's
own call — an argument-free, one-result `CALL_USER` of the word's unit or
`CALL_NATIVE`/`CALL_NATIVE_POLY` under the word — and the three ops are
contiguous (landing, call, apply), `sealLandingSkip` (compiler/go/lower.go)
seats the pc past the apply on the landing's word (`LandingWord.Skip`).
The walk's `/q` arm then ENTERS the plan's own overload over the word as an
atom at the word's position — the interpreter's arrival converts it so
(`CollectArrival`) — through the Apply kernel (`dynApplyEnterSig`, the
frame push the residual apply itself uses, with the declared return
contract), and the run loop resumes at the sealed pc, so the word never
runs (`landingQuoteClaim`, eng/go/vm.go). No seal, or an overload with no
unit of this program, keeps the defer. Measured: `m.f z`, `m get 'f' z`,
`m.f y`, `m.q z` and a capture under a word that raises (`m.f w`, w
raising) answer as the interpreter does; the return contract, a raising
body and a two-result body hold parity; `m.f typeof` (typeof collects the
value) and `m.f y 5` (a wider residual) still defer. Every probe that
changed moved from the loud defer to the interpreter's answer (a
45c3bdb build side by side). The Function-typed reference is NOT claimed:
the stamped stored-fn unit it would enter reads a bare Function param as
data (NUR220, `[fn f]` for `[0]`), so enabling it would trade the loud
defer for a silent wrong answer. Pinned in `TestNamedFnCandidatesOpenShapes`
(lang), `TestReStepLandingQuoteClaim` / `TestReStepLandingWalk` (eng),
`TestSealLandingSkip` (compiler).

**The merge (main's #513, 2026-09-26).** The two closes compose; neither is
dropped. The landing tries main's sealed claim FIRST, because it stays
compiled; then the island, then the skip (`landingWalk`). Main's `/q` rows
answer through its claim as they do on main. The two captures main's claim
left deferred take the island and answer on both lanes: `m.f typeof`
(typeof collects the value) is `[typeof]`, and `m.f y 5` (a wider
residual) is `[y 5]`. Both are pinned as parity rows in
`TestNamedFnCandidatesOpenShapes`. The Function-typed reference main kept
deferred is gone with the run's NUR078: main's `m.g z` row expected 7
interpreted, and here it is the named no-match on both lanes. The claim's
entered frame anchors its contract error at the landed value. Main's
return-contract row caught this branch's NUR118 anchoring that frame at
the op instead (NUR274, fixed at the merge). Main's `landingSkip` run-loop
field and this branch's `landingSkip` method shared a name, so the method
is `landingSkipCapture` now.

**The deferral (the contained half, 2026-09-24).** The walk's `/q` arm
stood aside and the residual apply took the word's RESULT (`m.q z` was
`[42 0]` for `[z]`, `m.f y` `[42 42]` for `[y]`); it now defers at the
landing (`vm:landing-quote-claim`) exactly as the Function-typed
reference does (`vm:landing-claim`), and the program dies loudly with
the compiler-defect note. A compile-time decline was the alternative,
and over-wide: no static model tells a `/q` slot from a typed slot's
barrier, so a decline would have to fire on every landing whose
candidate overloads carry either slot, declining typed-slot rows that
run right today. The deferral is precise — only the walk that actually
claims the slot dies — and the maintainer asked that such deferrals be
KEPT ON A LEDGER: runtime_defers.tsv, the per-file twin of
compile_failures.tsv (runtime_defer_ledger_test.go), names the rows that
compile and then bail, both ways per file, and fn-value.tsv's L317/L318
are its first rows booked by choice (the corpus-wide runtime defers
8 -> 10, the lang unit ledger's bail line 34 -> 37: `m.f y`, `m.f z`,
`m.q z`). Pinned in `TestNamedFnCandidatesOpenShapes` (lang) and the
eng landing test's `/q` arm.

**The walk (the closed half).** The landing op carries the function word
the check pass found after the value (`LandingWords`, by the landing's
pc; `landingArg`'s bit 1), and with that word and nothing beneath the
VM's landing runs the interpreter's OWN plan (`core.PlanMatch`, through
the region host) over a two-token window — the value, the word — and
answers as the interpreter's re-step does (`landingWalk`): no overload
takes the word or matches over nothing → a named fn raises
`uncalled_function`, an anonymous or macro value PARKS inert, so the
later residual apply leaves it (`m.l z` is `[fn lam(Integer) 0]`, was
1); the zero-argument fallback → it FIRES (`m.f z` is `[42 0]`, was 1;
`m.f typeof` Integer, was Function); an Any-typed slot's speculative
claim → the strict barrier's stranded-forward `signature_error`, the
interpreter's own (`m.a z`; the wordless landing raised a false
`uncalled_function`). Pinned in `TestNamedFnCandidatesWalk` (lang), the
eng landing tests, the compiler's word-table pins.

**The claims the lowering cannot honour (open until the deferral above).** A `/q` slot
CAPTURES the word (`m.q z` is `[42 0]` compiled for the interpreter's
`[z]`: the walk stands aside and the residual apply takes z's result;
L317/L318 answer right because z's result is its own atom) and a
Function-typed slot takes the word's REFERENCE (`m.g z` is 7
interpreted: the run BAILS loudly now, where it raised). Both need the
word's compiled call SKIPPED and every later op re-planned — the
lowering decided the program's shape on the model that the word runs and
the residual arm applies the value over its result — so the faithful
compiled treatment is a decline or a bail, and either moves a corpus
ceiling (compile failures 21, runtime defers 8) by the two coincidental
rows: the maintainer's call, not this increment's.

**Rule:** the interpreter's re-step of a fn value matches over the live
tape (execFnDefLiteral; CollectCandidateScan's word arm): a `/q` slot
CAPTURES the following word as an atom, an Any-typed slot takes the word's
RESULT (the word dispatches at collection), a Function-typed slot takes its
REFERENCE, and a typed slot STOPS at it — and with the next token a word, a
`/q`-at-0 signature is preferred (preferWordSig).

| witness (`h` is `[[] [Integer] [42]]` and `[[x:Atom/q] [Atom] [x]]`, `m` is `(mk)` — a Map a fn returned, so `m.f` is dynamic; `y` is a 0-arg fn returning 42, `z` one returning its own atom) | interpreted | compiled |
|---|---|---|
| `m.f y` | `[y]` | `[42 42]` |
| `m.f z` (fn-value.tsv:L317) | `[z]` | `[z]` — by coincidence |
| `7 m.f y` | `[7 y]` | `[7 42 42]` on main; declines here ("dynamic value precedes residual args") |

**The defect, in one sentence.** The compiled lane has no static knowledge
of the run-time fn's overloads, so with a FUNCTION word after a dynamic fn
value and nothing beneath it, the landing (OpReStepLanding) settles only a
fn that fires its zero-argument overload or raises as a named fn matching
nothing; a fn with an arg-taking overload that could claim the word stands
aside (NUR175's rule), and the lead arm then applies it over the word's
RESULT — right for an Any-typed slot, wrong for a `/q` one (the word was
never to run), a Function-typed one (the reference, not the result) and a
typed one (a barrier: the fn takes nothing and raises or parks). Seating
the value as DATA instead (the draft that found this) is wrong for the same
run-time shapes in the other direction, so the lead arm keeps the apply
it always had and the shape is pinned as measured
(`TestNamedFnCandidatesOpenShapes`); the islands' word-after rule
(`crossesBoundary`) still declines the mixed and trailing twins, where the
same apply was a witnessed miscompile (NUR187's `7 m.f three`).

**The fix (open).** The landing takes the following WORD into the op —
its name, and a skip target past the word's compiled call — and walks the
fn's overloads at run time in the interpreter's order: a `/q` slot captures
the atom and dispatches, skipping the word; a Function-typed slot takes
the word's reference and dispatches, skipping the word; an Any-typed slot
lets the word run and applies over its result (today's lead arm); a
zero-argument overload fires; a typed slot or no match stands aside for
the raise or the park. Until then a decline of the lead shape would put
L317/L318 and every `7 m.z typeof`-like row (a 0-arg member the landing
fires under a word) on the compile-failure ledger for a shape the
landing settles at run time, so the pending pin is the measured state.

## NUR207 — a def-bound fn value that arrives as a gradual carrier is read as data {#nur207}

**Status:** FIXED 2026-09-26, twice — on main's #514 (its fix, below) and on
the reverse-order NUR run (the root's gradual def read, next) — and
composed at the merge of main's #514 (the handoff log's entries of that
date). On the merged tree main's claim (check `tryShapedFnReadArrival`)
runs first and the branch's root plan takes the reads it stands aside for:
`7 j typeof`, `5 [j]` and `(j)` compile with parity where the branch's
guard deferred, and `j j`, `r 'x' 3` and `def k j` decline at main's claim
(NUR216's collected read, the unfit written token) where the branch's
island answered or its guard deferred. The family's residue, all present
on main at 45c3bdb, is recorded as NUR216 (a claimed read the model does
not reach), NUR217 (a fn laundered through a producer the pass cannot see
into) and NUR218 (a member lambda at an `Any` parameter). Recorded
2026-09-26 on main (#510).

**The branch's fix (2026-09-26).** The defect was the program ROOT's alone. A fn unit already
planned such a read (NUR123: a deopt at its statement's start, or the
whole-frame replay), and every witness answered correctly inside a fn. The
root noted nothing (`NoteWordRead` returned at no open unit), so the read
lowered to a data push. Now the root records its gradual def reads
(`noteRootWordRead`) and `Finalize` plans them (`planRootWordReads`):

- A read the program RESIDUAL holds is tested once the residual is laid
  out. When every read of the value is a residual entry, the first read's
  token is a token of the program, no root event is written after it, and
  the residual keeps written order, the point is an ISLAND
  (`DeoptSpec.Beneath`): a fn at run time hands the interpreter the program
  from the read's token on, over the values beneath it, and the island's
  residual ends the run (`Program.Deopts`, `Program.Body`). `j`, `5 j`,
  `j j`, `r 5 3` and `r 'x' 3` answer the interpreter's results, the
  no-match's notes included. A read that LEADS the residual's
  leading-form dynamic apply deopts only on a no-match
  (`DeoptSpec.NoMatchOnly`): over a window the value matches, that apply
  is the word dispatch's own answer and stays native (`h {x:3 y:4}` over a
  `find` result, patrun.tsv L41).
- A read a root event CONSUMES takes the fn units' placement
  (`deoptStatementStart`, `deoptDeferred`): an island from its statement's
  start where the compiled stack there is the interpreter's. `j typeof`,
  `[j]`, `{a: j}`, `if true [j] [0]` and `5 j add` answer the interpreter's
  results.
- The root's def WRITES its value (`bindGlobal`) where the interpreter's
  `def` installs a fn. So the island installs the read's value under its
  name for its run, in place of the write (`bindRootRead`), which it puts
  back after.
- Anything else is a GUARD: the value a fn at run time raises a designed
  defer, loud where it answered wrong silently. That covers a value written
  before the statement that the root lays out at the program's end
  (`7 j typeof`, `5 [j]`), a read in a paren group (`(j)`), and a def made
  through the read (`def k j`). The guards are bails on the ledger, by the
  maintainer's rule.

Pinned by lang `TestNUR207RootGradualDefRead`.

The record below is the finding as it was made.
**Found:** the generated sweep's last cells, on `main` at ae17688 —
measuring why the `if` × container cell's member read could not simply
stand aside (the anonymous 0-arg member parks at the landing, but the
carrier it leaves hides the fn from every later reader).

**The witnesses.**

```
def mk fn [[][Any][([] => [42])]] end def j (mk) end j
  interpreted   42       `j` names a fn definition: the word dispatches
  compiled      [fn]     the def-bound Any carrier is pushed as data

def m {s: ([a:Integer b:Integer] => [a sub b])} end def r (m.s) end r 'x' 3
  interpreted   [boru/signature_error]: cannot call `r`
  compiled      [fn (Integer, Integer) x 3]
```

**Where it sits.** A `def` of a fn value makes the name a fn DEFINITION on
the interpreter (ADR-011: a bare name always calls), whatever produced the
value. The compiled lane knows that only when the bound carrier says so — a
Function-typed carrier with a claimed shape takes the def-bound read model
(check's `tryShapedFnReadArrival`, NUR194's window), a produced closure its
claim. A carrier typed `Any` (a fn's declared `Any` result) or a gradual
member read (`m.s`) carries no such fact, so the read is a plain value push
and the program residual lays it out as data — or, over a written window,
the dynamic apply parks a non-matching value where the name would raise.

**The direction.** The read of a def-bound gradual carrier is a runtime
question — fn or data — so it wants the guarded re-step the landing models
use (a `RESTEP_LANDING` of the NAME, dispatched as the word), or a decline;
never a data push.

**Refined 2026-09-26 (the merge of main's #511).** A guard over a read that
LEADS the residual's dynamic apply bailed on any fn value. That guard is
what a root event after the read leaves, since no island can take the
program over. But the apply over a window the value matches is the word
dispatch's own answer, as the island's `NoMatchOnly` rule already says. The
sweep's `def` × container · suffix-def and · splice (`… def f m.f end f 5
def zzvpost 8`) bailed where both lanes answer 6. Such a guard now bails on
a no-match alone, and reads the stack entry, where its window is static. A
window that `MatchFnSig` does not match exactly still bails, loud: `f 5 def
q 1 end q` lays out a two-value window over a one-param fn. Pinned in
`TestNUR207RootGradualDefRead`.

**Main's fix (#514).** Five pieces, each measured against main over a
battery of fn sources (a lambda literal, a map member, a list member, both
factories, a branch) in fifty-odd reading contexts:

- **The parking member folds** (compiler `tryFoldParkedMemberFn`, the map
  twin of `tryFoldStaticIndex`'s list fold). A get / dot read over a
  concrete container and key of a member every value landing PARKS — an
  anonymous, capture-free, unapplied, non-macro fn whose signatures are all
  real zero-argument ones — is the member value itself: the lambda literal
  on both passes, so `def j (m get "f")  j` is 42 and `if true m.f [2]` is
  the `if` × lambda cell. The list member (`l.0`) always folded this way.
- **The fold's escape fence** (compiler `foldedEscape`, riding the
  placement-gate poison `armReadCompileFailure` — no new compile-failure
  site). The fold is measured against a def of the member, a branch arm,
  `apply` of the member itself, `typeof` and the program residual; the
  folded value reaching anything else — a native word's operand (`set`,
  `eq`, a stack shuffle), a user fn's argument, a list or map literal, a
  baked container, a `/v` read of a def of it, an `if` condition, a code
  body's or a lambda's result — declines, as the member's 0-arg landing did
  on main. The value takes its own identity at the fold (`foldedMembers`),
  every copy of it is known by its body's backing array (`foldedBodies`: a
  def's install re-normalises the signatures but shares the body), a
  branch merge over a folded arm and a named fn's call result returning it
  carry the taint on (`carryFoldedTaint`, `returnsFolded`), and a
  def-bound read of a folded carrier the read model stands aside for, or
  that sits inside a fn or code body, declines too. A list twin (`l.0`)
  diverging on main is no licence: the map member's program declined there
  and must not compile to a wrong answer.
- **The gradual claims** (compiler `noteClosureShapeBind`,
  `memberLambdaShape`; check `tryShapedFnReadArrival`). A def of a carrier
  not typed Function claims a shape when the pass can prove one — an
  `Any`-returning factory's closure (the producer's out-op), a pinpointed
  member lambda (one signature of plain typed params) — and the read model
  dispatches the name over its whole window (witness 1 is 42). It declines
  only what the program-level paths got wrong — a bare read with nothing
  beneath it, a written token the parameter does not take (witness 2), a
  function word the forward phase stops at — and stands aside to the paths
  it had everywhere else (a frame holding values beneath, a computed
  token, any read in a fn, closure or nested body), so `10 3 r` keeps its
  trailing-window island. A gradual claim is the def-read model's alone
  (`gradualClaims`: no other reader of the shape table sees it), it answers
  only the read the pass is stepping (`pendingGradualRead`), and a read of
  it the model never sees — a pending word collected it: `def k j`,
  `typeof j`, `j eq j` — declines at the next recorded event
  (`flushGradualRead`, NUR216's collected reads).
- **The branch def-read declines** (compiler `armLeavesFn`,
  `noteMayBeFnRead`). A def of a branch result an arm of which may leave a
  fn value the merge's landing does not fire — a parked lambda, an
  arg-taking fn, a fn carrier, a nested branch or `Any` result proven to
  hold one — declines at the read (`def c true end def g (if c ([] => [42])
  [2]) end g` answered `[fn]` for 42, its `g/v` `fn` for `fn g`).
- **`apply` over a gradual lead** (compiler `recordCallElided`). A lead not
  typed Function whose identity result carries a structured producer's id
  was elided silently — `(mk) apply` answered `[fn]` for 42 and `[5]` for
  the interpreter's `signature_error` over data. It is the pending apply
  now: OpCallDynApplyTop applies a fn and raises apply's no-match over
  anything else, or the program declines.
- **A proven fn at an `Any` parameter** (compiler `RecordUserCall`). A
  carrier the pass proves holds a fn (`gradualHoldsFn`) handed to a user fn
  parameter not typed Function declines: the callee is analysed over the
  gradual carrier and reads the param as data (`def g fn [[h:Any][Any][h]]
  end g (mk)` answered `[fn h]` for 42).

## NUR210 — a computed `do` body: a value beneath it re-seated, a rebinding it leaks unmodelled {#nur210}

**Status:** FIXED 2026-09-26, composed at the merge of main's #514 (the
entries of that date in design/NUR-RUN-HANDOFF.0.md). Main's close stands
whole — the region, the runtime-checked single value and the kept-defs
latch (its fix and follow-up, below) — and the branch's prefix island
still compiles what main declined or deferred: a run with inert values
beneath it, or collected by a list literal, at the program level (`9 do
(mk)` over `[1 2]` is [9 1 2]; `[9 do (mk)]` over `[g/v]` is [[10]], main's
loud defer). The branch's rebinding steps — the bare root read seated live,
NUR266's statement boundary and the root's generalisation (below) — are
superseded: a run whose tokens are not proven plain data may leave a NAMED
0-arg fn value, which fires across the `end` (`do (mk) end x` over
`[g/v]` is the interpreter's [7 5] and was the branch's [fn g 5], silent),
so main's rule declines every entry above such a run, and the latch every
later observer. What the branch answered that the composed tree declines
is NUR282. Recorded 2026-09-26 on main's #512.
**Found:** measuring the `do` analogues of the hosted splice's declines
(the 2026-09-26 handoff entry), on `main` at b4fad6c.

**The witnesses.**

```
def mk fn [[][List][quote [1 2]]] end 9 do (mk)
  interpreted   [9 1 2]
  compiled      [1 9 2]            silent: the 9 is seated after the body's run

def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x
  interpreted   [5]                `do` keeps its body's defs
  compiled      internal_error: CALL_DYNAMIC underflow

def x 99 end def mk fn [[][List][quote [undef x]]] end do (mk) end x
  interpreted   undefined_word: x
  compiled      internal_error: CALL_DYNAMIC underflow
```

**Where it sits.** The dyn-body backstop (compiler `recordDynBodyCall`)
lowers a computed `do` body to a CALL_NATIVE whose result is marked
variadic; the program residual's const `9` beneath it is pushed by the
residual reconciliation AFTER the call's values instead of before, and a
keep-defs body's def of a name the program reads afterwards is invisible to
the compiled read. These are exactly two of the four shapes the review of
#508 measured against the withdrawn per-iteration `for` host; the hosted
`for` splice that closed code-bodies.tsv L141 declines both by position
(the program's last statement over an empty residual), `do` does not.

**The direction.** The variadic call's residual beneath must be seated
before the call (the region-prefix mark), or the site declined; a
keep-defs dyn body followed by a compiled read of any name it could bind
must read live or decline.

**Main's fix (#514).** Two halves, neither a new MarkUncompilable site.

- *The run is a region.* `recordDynBodyCall` records a computed
  whole-residual body (`do`'s, CallableSpec BodyOutResidual) as a
  variadic REGION — the check pass models one dynamic(Any) out where the
  run leaves 0-or-more values — so every region rule applies: a value
  beneath it seats through the mark or declines ("call result above a
  literal"), a fixed-count consumer declines (`[9 do (mk)]`, `9 do (mk)
  drop`, `do (mk) add 9`, which raised CALL_NATIVE_POLY underflow over an
  empty run), and the fn-value apply arms no longer read the run as one
  value (the CALL_DYNAMIC underflow). The run may leave a CALLABLE the
  interpreter re-steps over what follows it (`do (mk) 5` over `[g/v]` is
  6), so unless its tokens are proven plain data (a factory's const
  `quote [1 2]`) it seats only as the residual's LAST entries (lower.go
  `dynRegionNotLast`; the program residual otherwise admits a region with
  inert values above it).
- *The kept-defs latch* (compiler kept_defs.go / kept_defs_scan.go). A
  keep-defs word (`do`, and `each`/`fold`/`scan`/… — BodyMultiRunKeepsDefs
  — whose computed List bodies leak the same way) over a computed body
  arms a latch where the body RUNS: at the program's top level for good;
  inside a unit, handed to the unit at its finish and re-armed after
  whatever invokes it (a CALL_USER of it, the native a code-body closure
  is handed to, any possible indirect invocation once such a unit
  exists). While armed, the first observer of a binding — a def read, a
  user fn call, a fn-value apply — poisons `armReadCompileFailure`, the
  arm-read seam's Finalize decline. A body the recorder can PROVE binds
  nothing does not arm it: a factory call returning a const quoted list,
  read directly or through a promoted value-def, whose tokens pass a
  binder scan (no def/undef/var/unpack at its top level, no undef /
  behave / usurp anywhere, no check-mode native, no unresolved name, no
  reach or splice, and the same of every user fn or bound value it
  names); a proven fn callback keeps nothing either. The same latch closes
  NUR203.

**The follow-up (2026-09-26, the same day): a single-value seat takes one
runtime-checked value.** Recorded as a region, the run declined at EVERY
fixed-count consumer, and that took out shapes that compiled correctly
whenever the body left exactly one value — the mini-s3 handler `def ok (do
b error [drop false])  if ok [1] [0]` in a fn body among them, so its
units stopped stamping (lang/go/test `TestStampDynEnvLateArmDrift`; the
first landing was reverted for it). Three changes:

- *A run proven to be ONE plain value is no region* (`recordDynBodyCall`
  over `bodyPlainCount`): `9 do (mk)` over `quote [5]` seats as the one
  value the check pass models. A proven plain run of any other count keeps
  the region and its declines — a runtime check could only ever defer.
- *A region that may leave a callable, consumed by a single-value seat, is
  DEMOTED to one runtime-checked value* (compiler dyn_body_one.go): a call
  operand (`(do b) add 1`, `do b error […]`), a list element (`[9 do b]`),
  a branch condition, a loop bound, a store or def source (Finalize's
  pre-pass, `demoteConsumedDynRegions`), and a promoted or dead value-def
  (`def ok (do b)` — `dynBodyOneAt` in the lowering). The call carries
  SigRef/PolyRef.DynBodyOne, and the VM seats the run only when it left
  EXACTLY ONE value the interpreter's tape would not re-step; zero values,
  two or more, or a fn value / class / reach / modifier is the designed
  defer `vm:dyn-body-one` at the call's position — loud, and the compiled
  lane's own. A run seated as a RESIDUAL's entries keeps the region rules
  (a value beneath it and entries above it still decline).
- *The kept-defs latch lets a FRESH read through* (`noteKeptDefsFreshBind`
  / `keptDefsFreshRead`): a def made after the run, at the latch's own
  depth, binds a value the body never saw, so a read of exactly that
  binding (the same value ID) is no stale observer. A read that resolves a
  conditional def's join, a binding from before the run, or anything after
  a second run, a user call or a fn-value apply stays one.

Measured against the parent (0bfc2fe, main + NUR190 + NUR212) over a
generated sweep of 1024 `do`-body programs (8 bodies — one value, two, none,
a 1-arg and a 0-arg fn value, a binding body, a string, a name — across 21
consumer / residual contexts, each as a proven factory, an unproven factory,
and a List param under no contract and under `[Any]`, plus 22 loop /
branch / fresh-read contexts): no program the parent answered correctly now
answers wrongly and no new silent divergence; the ten silent divergences
left are NUR213's residual shapes, identical on the parent. Against 1cafe3f
the follow-up only moves declines to matches or to the loud defer. Where it
turned a parent MATCH into the loud defer, the run is of the wrong count
for its seat (`(do b) add 1` over `[5 6]`, where the parent's fixed seat
happened to line up) or leaves a fn value at a single-value seat, which
the tape parks or fires by context (`[do (mk)]` over `[g/v]` parks it,
`def h (do b)` over a 0-arg one fires it).

Pinned by lang `computed_body_region_test.go`
(`TestComputedDoBodyRegionDeclines`, `TestComputedDoBodyRegionCompiles`,
and the follow-up's `TestComputedDoBodyCheckedOneCompiles`,
`TestComputedDoBodyCheckedOneDefers`, `TestKeptDefsFreshReadStaysNarrow`),
`keep_defs_leak_test.go` (`TestDynamicKeepDefsBodyLeakDeclines`, since
NUR282's latch half `TestDynamicKeepDefsBodyLeaksToTheFn`), lang/go/test
`TestStampDynEnvLateArmDrift` (the mini-s3 units stamp again), compiler
`kept_defs_test.go` and `dyn_body_one_test.go`, and eng
`TestCheckDynBodyOne`. A run that may leave a callable seated ALONE (or
last) is still data on the compiled lane: NUR213.


**The branch's silent half, fixed (the prefix island — it stands on the merged tree).** The interpreter's `do` splices a computed body's
results back and RE-STEPS them: each value lands above what lies beneath,
and a trailing fn value applies over it (`9 do (mk)` over `[([x:Integer] =>
[x add 1])]` is `[10]`). The compiled lane seated the run as its one
recorded value. The trailing apply's rotation gave `[1 9 2]`, a list over
the run assembled `[1 [9 2]]` for `[9 do (mk)]` and `[1 [2]]` for
`[do (mk)]`, and a lambda the body placed last stayed data. The prefix island
(`compiler/go/prefix_island.go`) re-steps the run as the interpreter does. A
mark opens before the run's chain, which is the dyn-body event and the calls
producing its operands, contiguous. The constants beneath the run are pushed
at the mark. `CALL_DYN_MIXED_FROM_MARK` then re-steps the window through the
interpreter's own machinery. A list literal opens an outer mark first and
collects the island's results. A list over a run the island cannot seat, in a
fn body or through a branch, declines. The trailing apply no longer rotates a
run, and NUR067's prefix seating, which does not re-step, stands aside for
one. A run is a dyn-body event over a COMPUTED body whose results ARE its
body's residual (`eventFlags.dynBodyRun`, `do`'s list form). `each` and
`fold` over a dyn body answer one value and keep their layouts. A literal
body the backstop took was modelled exactly by the pass and keeps its
layout too (fn-value.tsv L332 and L333 stay native). Pinned by lang
`TestNUR210ComputedDoRunBeneathAndCollected` and compiler's
`prefix_island_test.go`.

**Still open then: the rebinding half.** `def x 99 end def mk fn [[][List][quote
[def x 5]]] end do (mk) end x` bakes the read of `x` as the check pass's 99,
and the run's leading apply then underflows (`internal_error`, loud). A read
after a keep-defs dyn body of a name the body could bind must read live.

**The branch's rebinding half (superseded at the merge of main's #514).** A computed body at the root runs in the
root's scope, so a later read of a name sees its defs and undefs. Three
things stood between the witnesses and that answer:
- **The read was baked** as the check pass's binding. After such a body
  (`noteDynKeepDefsLeak` at the top frame now latches `rootDynLeak`) a root
  read of a value binding seats live on the registry the body installed
  into (`NoteLiveRead`).
- **The residual's leading apply** took the body's run as a lead over the
  read, across the statement's `end`, and underflowed when the run netted
  nothing. The statement-boundary rule could not place a def read; each
  read's position is kept now (NUR266).
- **A body that unbinds** was stamped as a detached fn unit that lost the
  unbind (NUR267); it runs on the interpreter.

The three witnesses answer `[5]`, `[99]` and `undefined word: x` on both
lanes. Pinned by lang `TestNUR210ComputedBodyRebindsTheRoot`. The shapes
the run's count cannot seat under a later dynamic apply stay loud, and are
NUR266's open half.

**Still open then (silent, pre-existing).** The live read covers a bare root
read at the pointer. A read the check pass resolves any other way still
bakes the value it saw before the body, over `[def x 5 7]` after `def x 99`:

```
do (mk) end [x]           interp [7 [5]]       compiled [7 [99]]
do (mk) end {a: x}        interp [7 {a:5}]     compiled [7 {a:99}]
do (mk) end def y x end y interp [7 5]         compiled [7 99]
```

The list literal even seats the live read, then folds the literal to a
constant over the value the pass holds. A body the pass cannot see may
rebind any name, a fn's included, so the sound rule is that the pass stops
KNOWING root values after such a body: generalise them in place, as a
placed speculative undef does (`core.GeneraliseSpecUndef`), so that no later
read can bake one, and a read with no live home declines. That is a
check-pass change with diagnostic-parity reach, and it is this record's
next cut.

**The branch's generalisation, landed (2026-09-26; on the merged tree it stands only where main's latch does not arm — a body proven to bind nothing).** `do`'s check half now
generalises every root value binding IN PLACE when it meets a computed body
at the root, before it returns the body's `dynamic(Any)` hatch
(`generaliseRootValues`, basic). This is the speculative undef's own
transition (`core.GeneraliseSpecUndef`): a fresh carrier of each binding's
type, so no later read can fold the value the pass held before the body.
- **Which reads go live.** A list or map literal's element, a def's
  operand, a repeated read and a fn unit's read at its later call site all
  read live now, as the bare read already did: `do (mk) end [x]` is
  `[7 [5]]`, `{a: x}` is `[7 {a:5}]`, `def y x end y` is `[7 5]`, and
  `def f fn [[][Any][[x]]] end do (mk) end f` is `[7 [5]]`, on both
  lanes.
- **What stays.** The bindings themselves, so nothing is reported. A body
  that leaves a name alone reads its value live (`[7 [99]]`); one that
  unbinds it raises `undefined_word` on both lanes.
- **Where it applies.** At the root only: inside a fn body the body's defs
  land in the frame, and the unit's leak already seats those reads live
  (NUR203). Fn values, types and frame bindings keep their models, which
  are the transition's own exclusions. So a computed body that rebinds a
  FN is not covered here.

Measured against the tree before it: the probes that changed all moved
from a silent wrong answer to the interpreter's. Its neighbours that
decline (`do (mk) end s size`) declined before it too, and are NUR276.
Pinned by lang `TestNUR210ComputedBodyGeneralisesTheRoot`. NUR210 is
FIXED; NUR266's open half keeps its loud shapes.

**Composed at the merge of main's #514 (2026-09-26).** Both closes are
on the merged tree, and three seams decide between them. (1) The prefix
island's run is seated whole: the lowering drops the single-value check
a list literal's demotion armed on the island's own region
(`lowerCall`), so `[9 do (mk 5)]` over `[n n]` is [[9 5 5]] and not the
defer. (2) A run a single-value seat demoted is one runtime-checked value,
not a run the list literal must decline (`runOperand`), so a fn body's
`[9 do b]` over `[5]` compiles as main's does. (3) A member read's own
`/v` spelling (`m.f/v`, noted with no name) does not trip the fold's
escape fence, which is for a def of the folded value read by `/v`. The
branch's live seat of a unit's own def after a computed keep-defs body
(NUR203's close) was measured against the latch and lost: exempting the
reads it seats live re-opened a literal over the name baking the pre-body
value (`[t]`, `{a: t}` — silent), so the latch stands whole. Measured over
199 probe programs from both sides' tests and the merge's own probes: the
merged tree answers every one the interpreter's way or declines loudly,
except NUR213's four residual shapes (silent on both sides) and NUR281
(silent on both sides, found here). Pinned by lang
`TestComputedDoBodyIslandCompiles` and the adapted
`TestNUR210ComputedBodyRebindsTheRoot` / `…GeneralisesTheRoot`.

## NUR231 — a refinement over a computed bound: the compile pass baked a bound it did not know {#nur231}

**Status:** FIXED 2026-09-26 (the value half: Bytes a refinement base, a computed bound the run's; the type half: the run-time type install — the handoff log's entries of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** closing NUR009 — every Bytes bound is computed, so pinning
Bytes refinements on both lanes met it first.

**Rule:** one refinement, one membership question, on both lanes.

**Divergence** (pre-existing; measured before the fix):

```
3 is (Integer gt (size "abc"))                          interp: false            compiled: true
7 is (between 1 (size "abcdefghij") Integer)            interp: true             compiled: false
def T (Integer gte (size "abcd")) def v:T 3 v           interp: type_error       compiled: [3]
def T (Integer lte (size "abcd")) def v:T 3 v           interp: [3]              compiled + boru check: type_error
def x:(Integer gt (size "abc")) 2 x                     interp: type_error       compiled: [2]
def g fn [[n:(Integer gt (size "abc"))] [Any] [n]] g 2  interp: signature_error  compiled: [2]
```

The comparison words' refinement constructor runs in the check pass
(RunInCheck), where a computed bound is a carrier; the pass built
`(Integer gt Integer)` and the recorder baked it as a const, carrier and
all. A carrier orders below every value (the type-literal-first rule), so
every verdict over it was the lattice's, not the bound's: a lower bound
admitted everything, an upper bound refused everything, `between` over one
was Never, and a type over one checked nothing the run would.

**The fix.**

- *A value is built by the run.* The constructors (`MakeDepScalarSig`'s
  handler, `BetweenHandler`) note a bound the pass does not know
  (`NoteRuntimeConstruct`), and the engine's post-handler hook records the
  dispatch as the call it is (`RecordRuntimeDispatch` — the run-time bind
  latch's generalisation, its outs registered for later operands). A
  refinement bakes as a const only over const bounds (`IsInertConst`), and
  `between` decides an empty interval only over known ones. Value uses —
  `is`, a residual, a factory's result, type algebra over one — compile
  and agree.
- *The pass decides nothing over an unknown bound.* `depBoundCheck`
  admits (gradually), so `boru check` raises no diagnostic of its own.
- *A type over one is installed by the run* (the type half, the same
  day). The pass cannot check a value against a bound it does not know,
  and the compiled lane replayed the pass's install and verdict; the first
  cut declined those sites (`DeclineUnknownRefinement`, one new census
  site, 91 → 92). They compile now:
  - *The run installs the type.* The type installer notes a body holding
    a refinement over an unknown bound — directly, or in a union,
    negation or typed container's child (`HasUnknownRefinement`) — and
    the def's dispatch records the body operand and `OpBindTypeRun` in the
    def's place. At run time `core.RunTypeInstall` runs the interpreter's
    own `InstallType` over the body the run computed — a mint, or the
    adopted Never of an empty interval — and forwards the node the pass
    minted to the run's (`RunForward`). Every compiled reference names
    the pass's node: a type operand pushes the run's node
    (`ForwardedType`), and a signature slot or typed-bind spec decides
    membership, unification, rendering and equality through it
    (`forwardingBehavior`). The def's type twin is written back, so its
    replay installs nothing. Only at the root: a fn body's per-call
    install would share the forwarded node across calls, a loop body's
    across iterations, so there the def declines as the compile-time word
    it is.
  - *A typed def records the run's check.* `OpBindTyped` over
    `TypedBindRunMembership` runs the registry-armed Unify the
    interpreter's typed def runs, against the named node or against the
    inline constraint the run computed, which sits beneath the value
    (`ConsOperand`). A concrete value records too: the pass's verdict over
    an unknown bound is no verdict.
  - *An overload set re-matches at run time.* The pass's match over an
    unknown bound admits every value, the leniency the fn-predicate
    overload hazard already routes to `OpCallUserPoly`; committing to the
    refinement's overload answered a signature_error where the
    interpreter fell through to `[n:Integer]`.
  - *The pass decides nothing over an unknown bound, anywhere.* An
    intersection keeps the unknown bound and proves no emptiness over it
    (`(Integer lt (size s)) tand (Integer gt 5)` was Never at compile
    time), and a complement admits (`tnot` over an admitting refinement
    refused everything).
  - *An inline signature type forwards too.* An inline parameter or
    return type over such a bound (`[n:(Integer gt (size s))]`, a union
    holding one) is resolved when the fn is built. In a compile pass its
    pattern is an ANONYMOUS node minted over the placeholder
    (`runSigPattern`), which the compiled unit and the replayed signature
    both carry. The building word's dispatch records an unnamed
    `OpBindTypeRun`, which mints a node from the refinement the run
    computed and forwards the anonymous one to it, renamed as the run
    renders it — so the no-match's "declared pattern" and the RET's
    "expected" read `(Integer gt 3)`, as the interpreter's do.
  - *What still declines.* Three shapes decline, through the existing
    compile-time-word site (`NoteRuntimeDependent`):
    - an inline INTERVAL over such a bound: the run may find it empty,
      and then the interpreter's slot is `Never` itself, where the
      compiled slot is the base with a pattern;
    - a typed container's child, which is no node a forward can stand in
      for;
    - a fn body's per-call type def.
    `DeclineUnknownRefinement` is retired, and both censuses are back to
    91.
  - *A known side decides nothing either.* A refinement with any bound
    the pass does not know admits whatever its known side says
    (`depScalarCheck`). A verdict the known side gives alone was baked
    with the placeholder rendered: `def g fn [[n:(between 5 (size s)
    Integer)] …] g 3` compiled to a trap naming `(Integer gte 5 lte
    Integer)`. The run checks every bound, as the interpreter does. A
    named type the run makes an alias of `Never` also renders `Never`,
    because the pass's node takes the bound node's name.

Measured while compiling the type half, beyond the table above (each now
agrees): `def T ((Integer gt (size "abc")) tor String) 2 is T` (interp
false, compiled true — the union's install had no decline);
`def T ((Integer lt (size "abcdefghij")) tand (Integer gt 5)) T` (interp
`T`, compiled Never); `def T (Integer gt (size "abc"))` with `f [n:T]` and
`f [n:Integer]`, `f 2` (interp 1, compiled signature_error). Found on the
way: NUR233 (a make field's refusal was a plain error, a compiler defect
when compiled) and NUR234 (a direct call's contract no-match notes).

Pinned by core's `TestRefinementConstructorsNoteUnknownBounds`,
`TestUnknownBoundDecidesNothing`, `TestRefinementConstOnlyOverKnownBounds`,
`TestUnknownRefinementIsTheRuns`, `TestRunTypeInstallForwards`,
`TestForwardingBehaviorOperands`, `TestHasUnknownRefinementWalks`,
`TestCombineOverUnknownBounds`, `TestNegationOverUnknownAdmits`,
`TestTypedBindRunMembership` and `TestWrittenBackTypeTwinInstallsNothing`;
compiler's `TestRecordTypeRun`, `TestPlacedTypeTwin`,
`TestRuntimeLatchesNeedALiveRecorder` and `TestRecordTypedBindRun`; eng's
`TestBindTypeRunRefusal`; and lang's `TestNUR231ComputedBoundValuesCompile`,
`TestNUR231ComputedBoundTypesCompile` (with their known-bound twins),
`TestNUR231TypeRunDisassembles` and `TestNUR231RunBuiltSignaturesDecline`.
