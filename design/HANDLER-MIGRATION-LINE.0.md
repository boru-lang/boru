# The handler-migration line — a second session's brief

**Status:** brief, for a session that runs BESIDE the compiler line.
**Recorded:** 2026-09-17, from the velocity review
([FULL-COMPILATION-REVIEW.0.md](FULL-COMPILATION-REVIEW.0.md) §3.4 and §5,
step S2). **Owner:** unassigned — this document exists so that the line
can be started by anyone, in any session, without re-deriving it.

## Why a second line

Stage 6 of [FULL-COMPILATION.0.md](FULL-COMPILATION.0.md) (§6.8) is the
plan's most enumerable work and the only track that does not contend for
`compiler/go`: it lives in `basic/go` and `lang/go`, under each word's own
handler. It has had no owner and has not moved since it was measured on
2026-08-25 — 114 declaration-relevant signatures with no compile
declaration, of 172 relevant among 522 — while the compiler line retired
rows one family at a time. It accounts for 23 of the 113 corpus refusals
(code-body words 19, dyn-scope defs inside `do` bodies 3, a `for` body not
captured 1), for 14 interpreter-entering census rows of raw-token bodies
(`code-bodies.tsv` ×7, `bytecode-migrated.tsv` ×4, `control.tsv` ×2,
`word-splice.tsv` ×1), and for the eight `boru:test` quotation-body rows
that are the same defect one module over. The `do` note's own name for the
shipped answer — the dyn-body strategy, "the program compiles, the body is
interpreted" — is the defect stated.

## The worklist, and where it comes from

```
make handler-worklist
```

prints every undeclared signature as `<word>\t<why relevant>\t<shape>`,
from `TestDeclarationCensus` (`test/go/langspec/declaration_census_test.go`)
— the same census whose ceiling (`undeclaredHandlerCeiling`, 114) this
line lowers. The three reasons a signature is relevant, and what each one
owes:

| reason | count (2026-08-25) | what the word owes |
|---|---:|---|
| code-body (a `NoEvalArgs` operand) | 59 | a declaration that the body is a UNIT the recorder compiles (`CompileStoresBody` / `CompileStoresBodyList` / a `CallableSpec` with the body's inputs), and a handler that runs the unit through the VM invoker (`InvokeCallbackFn` / `enterBodyUnit`) instead of re-stepping tokens (`InvokeBody` → `RunResolved`) |
| quoted operand (`QuoteArgs`) | 44 | `CompileQuoteInert` when the operand is data the handler stores or reads (the timer words, `get`/`set` over atom keys), or a structured lowering when the handler re-steps it (`usurp`'s permutation, `while`'s loop) |
| fn-valued operand | 11 | `CompileReadsFn` when the handler only reads the value, `CallableSpec` when it applies it — through the Apply kernel, never a private convention |

A declaration is one increment (`for-each` took one); a handler that
returns tokens for the engine to re-step is a rewrite. The declaration
census's `relevant` / `declared` predicates say which a signature is.

## The procedure, per word

1. `make handler-worklist | grep '^<word>'` — the signature and its reason.
2. `grep -rn "<word>" lang/spec/*.tsv | head` — the corpus rows that
   exercise it; `BORU_SPEC_FILES=<those files>` then runs the ten gates
   over them in seconds (`make test-langspec` is the whole corpus).
3. Read the handler. Decide declaration vs. rewrite by the table above.
4. Write the declaration, or the unit-taking handler, with the word's
   check-mode half (`RunInCheck` / the ReturnsFn) kept in step.
5. Gates, in order: `BORU_SPEC_FILES=… go test ./langspec -run
   'TestSpecProd|TestSpecCompiledDifferential|TestSpecCompiledOrFallback|TestInterpEntryCensus'`
   (parity and the census on the word's rows); then `make test-langspec
   SHARD=n` for every shard, or `make ci-local`; then the merged coverage
   gate (`make cover-gate`, cached per module) — every new statement is
   covered or carries a proof-carrying `//covergate:allow`.
6. Lower `undeclaredHandlerCeiling` by the signatures declared, in the
   same change, with the word named in the constant's comment. Lower the
   census and refusal ceilings the rows moved (each gate tells you).

## What done looks like

- `undeclaredHandlerCeiling` 114 → 0; the census's `render` prints `none`.
- The code-body refusal buckets in `TestCompiledCoverage` (19 + 3 + 1) → 0.
- The `RunResolved` seam and the `boru:test` `CallBoru` rows in
  `TestInterpEntryCensus` → 0 (the latter needs runtime compilation of a
  quotation body, §10.1 S3 — take the declaration first, the unit second).
- `design/DO-STRUCTURE-COMPILATION.0.md` §8's "the body is interpreted"
  sentence is no longer true, and the note says so with a date.

## Rules this line inherits

- A new shape lands on the generic lane first and takes a typed lowering
  later by proof (FULL-COMPILATION.0.md §10.1): a callback convention that
  skips the kernel's match is where NUR155 came from.
- Never a handler that returns tokens; never an island. A declaration the
  recorder cannot honour yet is a REFUSAL at that word — a named defect —
  not a fallback.
- The interpreter is the oracle: `RunInterp`, never `Run` (NUR106).
- Every non-uniformity found on the way goes to `NUR.md` (answer
  divergences only); every refusal to `design/COMPILABLE-SUBSET.md` §5.

## Progress

**2026-09-18 — the fn-operand class, as the pilot (11 signatures).**
Ceiling `undeclaredHandlerCeiling` 114 → 110. Declared, all
`CompileStoresFn | CompileFnHandlerStrict` (`lang/go/native/native_valof.go`):
`usurp [Function]`, `stack-args [Function]`, `forward-args [Function]`,
`force-arity [Integer Function]`. Each handler reads the fn's signatures
to build a wrapper and STORES the original (`FnDefInfo.Wraps`) for the
wrapper's later re-dispatch — never an invocation at the word — and
validates the operand as an `FnDefInfo`, which is exactly the strict
store-fn contract. The VM dispatches the wrapper through
`UnwrapModifierChain` with no tape, so the value form is not the
"re-stepping result the VM cannot reproduce" the `CompileQuoteInert`
comment warns about; that warning is about the by-name Atom forms, which
are the quoted class and untouched here. The declaration found a live
MISCOMPILE on the way: these words run in check mode, so their only
recorder seat is the gradual poly record, and it delivered a capturing
closure to the native as a `ClosurePayload` the validation rejects —
compiled `illegal_ref` against the interpreter's value. The word's
check-mode half (`recordGradualWrap`) now honours the declaration by
declining the poly record for a typed `Function` carrier, so that shape
refuses with parity; the dynamic-Any `m.a` form of the same defect is
NUR158, owed to the poly seat (compiler/VM), not to the word. Gates over
the class's rows (usurp, path-modifier, apply, fn-value, modifiers,
module-minilang/parselang/emitlang/parse, callbacks, forward-barrier,
valof): 749 rows, 666 compiled, 25 refused, 0 divergences, 10 census
rows — unchanged before and after (a first, wider decline regressed
path-modifier.tsv:52-55, the composed wrapper chains, and was narrowed
to the typed carrier).

Left undeclared, with the reason recorded in `COMPILABLE-SUBSET.md` §5:
`apply [Function]` — its handler marks the value and the RE-STEP applies
it; the recorder owns the word by name (the fn-value elision, the
pending-apply window, `OpCallDynTrailTop`), no flag says "applies through
the Apply kernel", and `CompileReadsFn` would let a top-level fn-typed
carrier bake a `CALL_NATIVE` that leaves the marked fn as data — the S1
line's word. `mini` ×2, `parse` ×2, `emit` ×2 value forms — the handler
returns a SPLICE that applies the fn on the tape (`<fn> <src> <opts>
end`): a token-returning macro, this brief's rewrite class; the rewrite
(apply the transducer through the kernel from Go, with a `ReturnsFn`
twin) is not small — it changes the expansion's `end`-terminated forward
collection and the filter-shaped partials — so the honest state is
"undeclared, refused/modelled at the residual", not a permissive flag.
No declaration in `core/go/value.go` means "refuse here, by name": the
refusal is `CompileDefault`'s zero value, which the census counts as
undeclared — a refusing declaration (a tri-state `tapeBound: Yes`) is
the triple's C1 and is not yet a flag.
