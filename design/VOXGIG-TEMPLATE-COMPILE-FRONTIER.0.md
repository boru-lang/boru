# voxgig `Template` library — bytecode-compile frontier

> **RESOLVED (2026-07).** The module-scope-binding-read blocker documented
> below is fixed. An enclosing-scope binding read (the module-scope
> `mustache-acc` flex accumulator; any computed module `def`) inside a
> fn/closure unit now routes to a dynamic-scope lookup (`OpLookupDynScope`)
> instead of an unreachable in-frame event operand — see
> `boru-bytecode-next-stages.0.md` §"Stage C — Update (2026-07)". The Template
> library `template.boru` and 5 of 6 test files now `-force-compile` clean and
> stay byte-identical `compile == interpret` across all four engines
> (mustache/handlebars/liquid/jinja), including the mutating lexers. The lone
> remaining refusal — `test/template_prop_test.boru`: "code-body word `each`
> (Stage 2)" — is a SEPARATE frontier (a higher-order `each` in code-body
> position in the property-test harness), unrelated to the module-scope read,
> and is an open defect: the refused program is silently re-run on the
> interpreter, which hides the failure rather than fixing it.

Diagnosis of every bytecode-compilation refusal the **voxgig `Template`**
library (`voxgig-boru/template`) and its test suites trigger against `boru`
`main` (`203ea2f`), and the one fix landed here. Method: build `boru`, run each
suite under `-force-compile` / `-compile-report`, reduce every refusal to a
minimal reproducer, and map it to its emit/lower site. This is the Template
corpus's analogue of `VOXGIG-COMPILE-COMPLETION-PLAN.0.md` (which covers the
bloom/stats/decision/trie/sort corpus).

## The one hard rule held

`compile == interpret` (byte-identical stdout). Confirmed for every Template
suite: `diff <(boru -compile f) <(boru -no-compile f)` is identical on stdout for
all eight files, and `make verify-bytecode` stays green with the fix below.

## Refusal frontier (whole-program, after the do-map fix)

| Refusal | Where it fires | Shape in the library | Status |
|---|---|---|---|
| `code-body word each (Stage 2)` | `template_prop_test` top level | `Test.results each [ var … ]` (the test file's own result loop) | open — higher-order body reads an enclosing binding (leaf-1 class) |
| `code-body word test-test (Stage 2)` | every `*_unit_test` | `Test.test "name" [ … ]` framework word | open — the check-prop/test-body class (leaf-6 class) |
| `operand … at size` / `branch reads enclosing computation` | `lex-mustache`, `compile-tagged-seq`, … | `mustache-acc size` where `mustache-acc` is a module-level `def x (flex [])` | open — a mutable **module reference** read through dynamic scope; see below |
| `fn gen-program: consumes loop results` | `gen-program` | `(compile-tagged-seq … ) get "code"` — a recursive fn returning a `do {…}` map, result consumed | **FIXED here** |

`vm-run-with` (the sandbox runner) and the `size`/`each`/`test-test` rows share
the chain: `-force-compile` aborts at the first, so the frontier is a *chain*
per file — the `flex` read gates the large majority. Because the `flex` read
gates first, the `do {…}` fix below flips no Template file on its own until it is
resolved; it is landed as a langspec-gated correctness step (it also fixes the
shape for any non-Template program), the same discipline
`VOXGIG-COMPILE-COMPLETION-PLAN.0.md` uses for its flips-0 leaves.

## Fix landed: `do {…}` value-eval map is a single (non-variadic) result

`tryRecordDynBody` (`carrier.go`) flagged **every** `do`-body event
`variadicResult`. That is correct for the code-body (`do [list]`) overload — a
sub-program residual is runtime-variable — but wrong for the value-eval
(`do {map}`) overload, which produces **exactly one** value (the map) whatever
its members compute. As a whole fn body the over-mark was rescued by
`RecordUserCall`'s declared-return exception; **inside an `if` arm** it was not,
so the branch merge (and any fn whose body is that `if`) became variadic and a
fixed-arity consumer of the fn result — `(mk …) get "code"` — refused
`consumes loop results`. The fix marks the concrete-map value-eval overload
non-variadic at the source (`!body.Dynamic && len(outs)==1 && body Is Map`); the
List overload and the dynamic-body poly case stay variadic. Regression:
`TestDoMapVariadicArmCompiles` (`RunCompiledStrict == Run`, byte-identical, which
is also the no-FALLBACK-island proof since force mode does not fall back).
`make test` + `make verify-bytecode` green.

**Note on `make cover-gate`:** main (`203ea2f`) already leaves two rare
interpreter-only branches uncovered (`engine.go`'s
`isPendingResidualContainer` non-container fall-through and
`unwindLiveFrames`'s `PopFrameArgs`-error arm), so the 100% gate is red before
this change. This change is cover-gate-**neutral** — the uncovered set is
byte-for-byte identical with and without it (verified by running the gate on
both trees). It adds no uncovered statement; the pre-existing gap is unrelated.

## The `flex` module-reference read (open — do NOT paper over)

`def mustache-acc (flex [])` is a **mutable module-level reference**; the lexer
and `lex-*` fns read it (`mustache-acc size`, `mustache-acc push …`). A concrete
`flex` value cannot bake as a const (baking would capture the compile-time
instance and diverge from the runtime one), so the sound lowering is a runtime
dynamic-scope read (`OpLookupDynScope`, `curReg.Defs.Top(name)`, identical to the
interpreter's substitution). `dynScopeRescue` already emits exactly that — but
declines a module-level name because the call-graph reachability gate
(`dynamicScopeReachable`, `FnBinders`) only admits **fn-bound** names, and a
module def is bound by no fn.

**A naive widening is UNSOUND and was reverted.** Admitting module-scope names
by any of the tried signals — `Depth(name) <= baseline[name]`, the
`ModuleBinders`-at-`FnBodyDepth==0` set, `FnBinders[name] empty`, `!v.Dynamic`,
`suspended==0` — each mis-classified a **`fold`/`each` body-local** (`liquid`'s
`split-args` binds `q` inside a `fold` body) as module scope during the
higher-order-body **sub-run compile**, where the fn baseline, `FnNameStack`, and
`FnBinders` are all detached from the enclosing fn. That emitted an
`OpLookupDynScope` for `q`, which **misses at run time inside the fold callback**
and corrupts the fold result (`interpreter fail-count 0`, compiled `1` — a
silent miscompile the langspec differential is blind to, caught only by running
the Template suites compiled).

The sound fix needs a module-scope signal that survives the higher-order-body
sub-run. **This was found and works** (see next section) — but it unmasks a
deeper, separate cross-registry defect, so the `flex` read still **refuses**
and the program is silently re-run on the interpreter pending that Stage-C
work — an open defect, owed a fix, not a resting place.

## Update — the read-site classification is the sound module-scope signal

The mis-classification above is entirely an artifact of asking the question in
the **emit recorder** (a higher-order-body sub-run, where the baseline is
detached). Asking it at the **READ site** (`engine.go` stepWord, where the live
baseline is the reading fn's own) is exact: instrumented over the whole Template
suite, `ModuleScopeBinding` returns `true` for every `*-acc` flex-cell read and
`false` for every `fold` body-local `q` read — 100% reliable, no exceptions.

The mechanism (prototyped, then reverted — see below):

- `CheckState.ModuleScopeReads` — a per-pass set. `engine.go`'s def-read site
  records `w.Name` when `ModuleScopeBinding(e.registry, w.Name)` (the *reading
  fn's* baseline). A concrete module reference is also `SetDynFrom`-tagged so
  `dynScopeRescue` can recover its name (a `flex` value carries no `Value.ID`).
- `dynScopeRescue` admits `name` when `c.ModuleScopeReads[name]` **or** the
  existing `dynamicScopeReachable` gate — the module case needs no call-graph
  proof (a module def is unconditionally live in the registry). A fold/each
  body-local is never in `ModuleScopeReads`, so it keeps the reachability gate
  and is never mis-admitted. This flipped **7 of 8** Template files to fully
  compiled with byte-identical output.

## Why it is NOT landed: a do-map residual mis-hoist under `dynEnv` (Stage C)

Landing the read-site fix flips `liquid_unit_test` to a miscompile — but **not**
through the flex read. The root cause, from the disassembly of the compiled
`split-args` unit (`f26`) at the run-time miss:

```
f26 split-args (locals=[s …]):
  0007 CALL_NATIVE  do (Map)          ; initial acc {cur:[""] q:[""]}
  0011 CALL_NATIVE  iota              ; the range
  0012 LOOKUP_DYN_SCOPE k92 ; 'q'     ; <-- MISS: q unbound in this unit
  0013 MAKE_LIST    n1                ; assemble [q]
  0014 DROP                          ; …and discard it (dead)
  0015 LOOKUP_DYN_SCOPE k92 ; 'q'     ; (a second dead [q])
  0016 MAKE_LIST    n1
  0017 DROP
  …
  0024 PUSH_CLOSURE f27 ; fold$body   ; the REAL fold body is a closure
  0025 CALL_NATIVE  fold
```

The fold-body do-map `do {… q:[q]}`'s `[q]` list member — where `q` is a **fold
body-local** bound per-iteration INSIDE the `fold$body` closure (`f27`) — is
residual-**promoted into the enclosing `split-args` unit** (`f26`) as a *dead*
`LOOKUP_DYN_SCOPE q; MAKE_LIST; DROP` sitting BEFORE the fold call, where `q` is
never bound → run-time miss (interp `fail-count 0`, compiled `1`).

Bisected precisely:

- The mis-hoist is triggered by the **do-map non-variadic fix** (`e82ca4d`,
  landed): with `mapValueEval=false` (the old always-variadic behaviour) the
  member stays contained in the closure and `liquid` is byte-identical; with the
  fix a fixed-arity do-map reconciliation residual-promotes the `[q]` member out
  of the loop body.
- It is **independent of the module-scope read** — `split-args` has no
  module-scope read; the read-site fix merely compiles enough of the surrounding
  module to *reach* `split-args`'s compilation, exposing the latent promotion.
- The identical `split-args` shape compiles byte-identically **standalone** and
  **through a 2-file import**; only the *full* Template module (deeper
  compilation) reaches it.

**Critical soundness note.** `make verify-bytecode` passed `e82ca4d` cleanly
despite this latent mis-hoist — the langspec differential corpus has no
fold-body-do-map-over-a-loop-local shape, so it is *blind* to the defect (exactly
the hazard `VOXGIG-COMPILE-COMPLETION-PLAN.0.md` warns of). The only reliable
check for this class is the **voxgig `--compile==interpret` sweep** — i.e. the
corpus re-baseline Stage C is defined around. A fix (either stop the loop-body
member promotion, or keep the do-map variadic when a member references a
loop-body-local) must be gated on that sweep, not verify-bytecode alone.

That is exactly Stage C in `boru-bytecode-next-stages.0.md` — "sound module-body
compilation (cross-registry EmitState) … the one stage that is a *project*, not a
commit," gated on a corpus re-baseline. The read-site module-scope mechanism is
the correct Stage-E/F piece and should land **together with** the Stage-C
cross-registry `dynEnv` fix, so it never exposes the latent miss. Until then
the flex read refuses and the program is silently re-run on the interpreter —
scaffolding absorbing an open defect, not an outcome the design accepts.

### Attempt log — the record-time variadic heuristic is insufficient

A targeted fix was prototyped and **verified against the full Template corpus**
(the reliable gate), then reverted as insufficient — the findings narrow the real
fix:

1. **Closure gate (`!es.inClosureUnit()`).** Marking a do-map non-variadic only
   when NOT inside a higher-order body closure correctly makes the fold-BODY
   do-maps (`split-args` lines 413–419, recorded in `fold$body`) variadic — no
   hoist — while keeping the gen-program / `compile-tagged-seq` fn-return do-maps
   non-variadic (they compile). d2/d4/d5 + `TestDoMapVariadicArmCompiles` pass.
2. **But it is insufficient.** `liquid` still misses. The residual driver is the
   fold's **initial-accumulator** do-map (`do {cur:[""] q:[""]}`, line 407) —
   recorded in `split-args`'s OWN unit (`inClosure=false`), so still non-variadic —
   whose fixed-arity reconciliation promotes the loop-carried accumulator's
   member assemblies (`[q]`) into the enclosing unit.
3. **The real discriminator is the CONSUMER, not the record-time context.** A
   do-map consumed by `get` at fixed arity (gen-program) needs single-value
   modeling; one consumed by `fold` as an accumulator tolerates a variadic. That
   is **not knowable at `tryRecordDynBody`** (the consumer is chosen later), so no
   record-time flag can be right for both.

**Therefore the clean fix is at CONSUMPTION, not the variadic flag:** keep do-maps
variadic (revert the `e82ca4d` flag change), and relax `layoutOperands`
(`lower.go:1236`, the "consumes loop results" refusal) to accept a single-valued
do-map operand — `lw.es.eventInfo[op.idx].dynBodyResult && nout==1` — seating it
as one value. That touches the sim-stack count model on a corpus-wide hot path, so
it must be gated on the voxgig `--compile==interpret` sweep (a do-map producing
!=1 value, and a genuinely variadic loop result, are outside this relaxation
and stay open defects until their own fix lands). This is the concrete Stage-C
task; it is a residual/consumption-model change, not a heuristic.

## Langspec Stage D/E/F status (for reference)

At the internal langspec surface these stages are **already cleared** (prior
work): `reach.tsv:38` (Stage D, `getpath ∘ setpath` over a dynamic receiver) →
`7`; `flex` push/drop alias reference semantics (Stage E) → `[1 2 3]`;
`recursion.tsv:72` (Stage F, a callee reading the caller's `n` by dynamic scope)
→ `42` — all compile natively under `-force-compile`. The 9 residual langspec
refusals are all documented **Stage-H** "unmatched dispatch recovered" ERROR
rows (`knownRefusals`), where the program raises and a static guess would
diverge. They are open defects, owed a fix and tracked to closure — not
sanctioned outcomes, and not D/E/F gaps.
