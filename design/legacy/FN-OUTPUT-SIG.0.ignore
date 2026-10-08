# FN-OUTPUT-SIG.0 — the output slot is types, and only types

Status: LANDED.
Owner surfaces: core/go (`ParseFnDef`, `ParseFnReturns`,
`ResolveSigType`), lang/go (describe/inspect), FORMAL-SPEC.md,
REFERENCE.md.

## 1. The bug report

```
>> def si1 fn [s:String i:Integer [convert Integer s]]
>> si1 '1'
  error: [boru/type_error]: si1: expected 1 return value(s), got 2
```

The declaration reads exactly as intended: takes a String named `s`,
returns an Integer named `i`, body converts. Nothing about it is
obviously wrong, and the error points at `convert`.

## 2. What was happening

Pair syntax lowers a bare `i:Integer` inside a list to a single-entry
map flagged `Implicit` (parser/go/parse.go). `ParseFnParams` has always
understood that shape on the INPUT side — it is how `s:String` becomes a
named param. The output side did not: it ran the slot through
`OutputSigIsConcreteReturns` → `IsSigTypeValue`, whose branch list had no
Map arm, so `{i:Integer}` was classified a *concrete value*.

That triggered **return-by-value sugar**, specified at FORMAL-SPEC.md
§5.4 and implemented in `ParseFnDef`: an all-concrete output sig meant
"return these literals", so the values were appended to the body after an
`end` and every static return type was degraded to `Any`. The body then
produced its own result *plus* the appended map — two values against a
declared one.

```
fn f0 si1/1 (locals=1) [s]:
0000 PUSH_LOCAL  l0
0001 PUSH_TYPE   t0   ; Integer
0002 CALL_NATIVE s0   ; convert
0003 PUSH_CONST_FRESH        <-- {i:Integer}, appended by the sugar
0004 RET
```

Worse than the error: when the body's net stack effect was zero, the
program **succeeded with the wrong answer** and `boru check --pedantic`
reported no diagnostics at all.

```
>> def f fn [s:String i:Integer [convert Integer s drop]]
>> f '1'
{i:Integer}
```

## 3. Why patching the classifier was the wrong fix

`IsSigTypeValue` carried three separate comments, each recording an
earlier fix for the *identical* symptom — "misclassified as a concrete
return-by-value … spliced onto the body stack — surfacing as a spurious
'expected N return value(s)' error":

- a user-defined (`def`'d) type name, fixed by a registry lookup;
- a `Disjunct` value, fixed by an `IsDisjunct` arm;
- a dotted `Pkg.Type` reach, fixed by an `IsReach` arm.

The implicit pair-map was the fourth. The pattern is the tell: a
classifier that must recognise *every* shape a type can take, and fails
open into a silent semantic change, cannot be made correct by adding
arms. Each arm is one more shape someone has already been bitten by.

The language also disagreed with itself about the specific value:
`boru -e "{i:Integer} Type is"` answers `true` via `IsRecordShape`, a
predicate `IsSigTypeValue` never consulted.

## 4. The rule now

**An output sig is always the types the returns must match.** Never
values to splice.

A value IS a type (ADR-010), so a literal in the output slot is a
*literal type* — it admits exactly itself:

```
def f fn x:String 22 [22]     f 'a'   # => 22
def f fn x:String 22 [33]     f 'a'   # => type_error: return value 1: expected 22, got Integer
```

This needed almost no new machinery. `ParseFnReturns` already returned
`([]*Type, []*Value)` and `FnSig.ReturnPatterns` was already enforced by
all three engines — the sugar was intercepting the sig before any of it
ran. Deleting the interception is most of the change.

Consequences:

- `name:Type` works in the output slot, because `fn [x:A y:B [body]]`
  IS `fn [[x:A] [y:B] [body]]` and the two slots must read one shape one
  way. The name is documentation: `FnSig` has no named-return concept
  and nothing downstream reads it. Both spellings of the pair are
  accepted, because both slots must serve both tokenizers: the implicit
  map the production parser lowers to, and the single Word
  (`NewWord("i:Integer")`) a whitespace-only lexer produces — the
  borueng and checker spec runners, for which `ParseFnParams` has always
  carried the same arm.
- **The value side is reduced before it is resolved**, for BOTH
  spellings, by one `EvalSigTypeExpr` shared with `ParseFnParams`.
  `resolveReturnSlot` is where each declared return passes through the
  unwrap and the reduction together, so a parenthesised annotation is
  run whether or not the return carries a name. Reducing only the
  *named* one — the first attempt — left `[(Integer tor String)]`
  falling to the `TAny` tail while `[y:(Integer tor String)]` was
  enforced: the same two-spellings-one-meaning split this note exists to
  remove, reintroduced one slot down, and caught by the spec row written
  to assert the two agree. A sugar marker expands
  and a parenthesised annotation is RUN, so `y:(Integer tor String)`
  reaches `ResolveSigType` as the disjunct it denotes. Skipping the step
  is a fresh instance of this note's own failure mode: the raw
  `ParenExpr` falls to the `TAny` tail, and a declared union return
  enforces nothing while the identical annotation on a *param* is
  enforced. A failed or multi-valued annotation is a declaration-time
  error, never a silent wildcard.
- An explicit map (`{i: Integer}`) still declares a Map-typed return —
  the same `Implicit` split `ParseFnParams` draws for a Map-typed param.
- `OutputSigIsConcreteReturns`, `IsSigTypeValue`, `isSigTypeName` and
  `OutputSigValues` are gone. Nothing classifies the slot any more, so
  there is nothing left to fail open.

### The one behaviour genuinely lost

The sugar let a base case be written without a body:

```
def fact fn [[0] 1 []  [n:Integer] [Integer] [n mul (n sub 1 fact)]]
```

That `[0] 1 []` triple now needs its return written down — `[0] 1 [1]`,
which declares the literal return type AND produces it. Explicit, and
one token longer.

### A String literal is a literal type

The name an Atom is resolved by comes from `AsAtom`. An Atom carries
`AtomPayload`, not `StrPayload`, so the shared `AsString` extraction
answered `""` and every lookup below missed — invisible while a missed
lookup was a hard error either way, but the fallback below turns a miss
into a literal, which would have made `Integer/q` an Atom pattern
instead of `TInteger`.

`ResolveSigType` resolved a String or Atom as a type NAME and errored if
it named none — reachable only because the sugar caught non-type strings
first. With the sugar gone, `['ok']` would have become
`type part "ok" must start with an uppercase letter`. It now falls back
to a literal pattern, so `'ok'` behaves like `42`, and the pre-existing
split where unnamed `['ok']` worked but named `[x:'ok']` errored is
closed. A WORD still must name a type: a bare lowercase word in a type
slot is a typo, not a literal.

The fallback is gated on the type-name SHAPE, not on mere unboundness,
and that distinction is load-bearing. `'Integr'` is a misspelled
`Integer`; if any unresolvable name became a literal, that typo would
silently turn into a constraint matching one specific string, and the
report would move from `unknown type "Integr"` at the declaration to
`no signature matches; nearest [String]` at the call. So the fallback
applies only when a `/`-separated part fails to start with an uppercase
letter — when the string could not have been a type name at all. The
cost is that an uppercase literal (`['OK']`) reads as a misspelled type
and is rejected; a loud wrong guess beats a silent one.

## 5. Two diagnostics fixed alongside

**The declaration span went missing for one spelling.** `ParseFnDef`
took `DeclSite{Pos: outputSig.Pos()}`, and `attachDeclSpan` drops the
span when `Row <= 0`. The parser stamps positions only on nodes its
`valof` rule wraps in a `sited`, which a list-interior pair never is — so
the pair form silently lost the "the declaration of `f` expects N return
value(s)" span that the list form got. The pair's own VALUE keeps its
position (`Integer` at 1:22), so `sigDeclPos` descends to the first
positioned value inside and anchors on the declared type itself.

**`describe` and `inspect` under-reported, three ways.** `BuildFuncInfo`
discarded the declared `sig.Returns` and asked `inferReturns`, whose tables cover
builtins only — so every user-defined fn rendered a blank return column
while the generated example line right below it printed the answer.
`inspect` reported no returns at all, for user fns and natives alike,
and no `type` key for a word (by ADR-011 there is exactly one Function
type for it to have). Both now go through one `SigReturnNames` helper,
so the two introspection surfaces cannot disagree.

A declared return wins there. The builtin table is keyed on the NAME
alone, so it also answered for a user fn that happens to SHADOW a
builtin — `def upper fn x:Integer [Integer] [x]` reported
`Scalar/String`, a contract the function does not have. `sigIsDefined`
draws the line on the signature: a body — or the `Fallback` flag, set
only on the catch-all injected for a boru word — means boru source, and
a declaration is not a guess. Inference stays first for genuine
natives, where it knows more than the declared slot type does (`add`
declares `Number` and infers `Integer` for two Integers).

And where a return carries a PATTERN, the pattern is the contract and
the `*Type` is the weaker half. Reporting the type alone named a
contract wider than the one enforced — exactly the literal return types
this change introduced: `def f fn x:String 22 [22]` returns `22`, not
`Integer`, and a declared union degrades its type to `Any` while
keeping the whole domain in the pattern.

## 6. Open: an engine divergence this uncovered

A spec row for the empty-body case — `def f fn [[x:Atom] 42 []]  f a/q`,
a declared literal return against a body that produces nothing — fails
`TestSpecCompiledOrFallback`: the compiled unit and the interpreter
report the same fault with different taxonomy.

```
compiled    = "f: expected 1 return value(s), got 0"
interpreted = "f: return value 1: expected Integer, got Atom"
```

The two disagree about whether the frame's residual holds the argument:
the interpreter finds one value to type-check, the compiled unit finds
none to count. Run standalone from the CLI both engines say `got 0`, so
what varies is the corpus runner's frame conditions, not the shape
itself.

It is NOT a regression from this change — the divergence lives in the
unnamed/declared-return frame model, and the shape simply had no corpus
row before, because under the sugar `[[x:Atom] 42 []]` meant "return 42"
and never reached a return check at all. Deleting the sugar is what made
the shape reachable, and writing the row is what found it.

The row is withdrawn rather than pinned to either engine's answer —
pinning would bless one taxonomy before deciding which is right. The
empty-body contract is covered meanwhile by
`TestFnArgCleanup_LiteralReturnType_EmptyBodyIsAnError`. Fixing the
divergence is its own change: decide whether an unconsumed argument is
part of the frame residual, make both engines agree, then restore the
row.

## 7. Blast radius

Small, which is itself evidence the sugar was not load-bearing: no
`.boru` file in the repo used a literal output sig, and no spec TSV row
pinned one. Ten Go tests failed, all of them pinning the sugar directly
(`lang/go/test/fn_arg_cleanup_test.go`'s `ConcreteReturn_*` family) or
the classifiers that served it (`core/go/fndef_stage5_test.go`).
