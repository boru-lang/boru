# The Non-Uniformity Register's archive

**Split out of [NUR.md](../NUR.md) on 2026-09-27**, when that file neared the
repository's 1 MiB file limit (`scripts/check-no-binaries.sh`). These are
the full bodies of records that are FIXED or RESOLVED: the divergence as it
was measured, the trace, and the fix. Each keeps its number and its
`{#nurNNN}` anchor; NUR.md keeps the record's heading, anchor and status
line, with a link here, so every existing link to `NUR.md#nurNNN` still
lands on the record. The register's rules (numbers never reused, a record's
status) are NUR.md's; this file holds text only.

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

