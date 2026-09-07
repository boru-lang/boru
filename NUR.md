# Non-Uniformity Register (NUR)

A running register of every place where boru — the language or its
implementation — deviates from one of its own uniform rules. A
non-uniformity is any special case: a type treated differently from its
siblings, one member of a word family with an exception, a path that
bypasses a single-source-of-truth mechanism. Uniformity is a core design
value of boru (one parser, one argument-positioning convention, one
binding store, one total order, one truthiness rule); this register is
where every deviation from that value is made visible, argued, and
either eliminated or explicitly accepted.

Records are short, numbered (`NUR000`, `NUR001`, …), and dated, in the
style of [ADR.md](ADR.md). Numbers are **never reused**. A **Resolved**
record is **deleted** from this file — the fix and its rationale live
in the resolving commit, which names the `NURnnn` it closes — and its
number is retired, never reassigned. A gap in the sequence is itself
the record that something was found and fixed, and any external
reference to a deleted `NURnnn` stays unambiguous forever.

> **Recording a non-uniformity is mandatory; a Pending record does NOT
> block the PR.** When a non-uniformity surfaces — in code review, in a
> design note, or during coding and debugging — it is recorded here
> immediately with status **Pending**. That recording is not optional and
> not subject to maintainer instruction (unlike an ADR entry); what
> requires the maintainer is the **Allowed** verdict — the same reviewed
> discipline as a `//covergate:allow` entry
> (`design/COVERAGE-ALLOWLIST.10.md`). A record is discharged by becoming
> **Resolved** (the divergence is removed) or **Allowed** (an explicit,
> argued acceptance), and it may stay Pending across many merges: the
> register's job is that a divergence is never lost or silently
> baselined, not that work stops until it is settled.

**Statuses:**

- **Pending** — not yet discharged. Either not yet argued to a verdict
  at all, or argued to a **verdict of "resolve by fix"** whose fix has
  not landed: a record directed at a fix stays Pending until the
  divergence is actually gone, because only Resolved and Allowed
  discharge it. Does not hold up a merge. Every Pending record must also
  appear in the open list below.
- **Allowed** — a deliberate divergence, kept. The record states the
  uniform rule, the divergence, the rationale, and the evidence that
  pins it (docs and tests), so the acceptance cannot silently rot.
- **Resolved** — the divergence was removed. The record is **deleted**
  and its number retired (see above), so a record only ever appears in
  this file as Pending or Allowed; `git log -S NURnnn` recovers a
  retired number's history.

> **Spelling note (2026-08-19).** The modifier `/r` was renamed **`/v`**
> and the word `ref` became **`valof`** (see [ADR.md](ADR.md), ADR-011).
> Records dated before that day quote the old spellings verbatim, because
> that is what was observed and argued at the time; `/r` no longer parses,
> so read `/r` as `/v` and `ref` as `valof` when replaying their
> transcripts. The behaviour those records describe is unchanged by the
> rename — except where a record says otherwise.

---

## Pending non-uniformities (the open list)

The live list of records whose status is **Pending** — the standing
inventory of known, argued-or-unargued divergences. An entry leaves this
list only by becoming **Resolved** or **Allowed** in its record below —
keep the two in sync in the same commit.

| # | Title | Surfaced by / provenance |
|---|-------|--------------------------|
| [NUR112](#nur112) | The checker's residual for a parked native word applied after its name was EXTENDED does not match what runs: `def Pos (refine Integer)  def m {a:size/v}  def size fn [[n:Pos] [Integer] [200]] end  def v:Pos 3  m.a v` is checked `[dynamic(Any) Pos]` — two values, one of them the argument left behind — and actually leaves `[Integer]`. Both ENGINES agree on the answer (3); it is the static model that differs, so no differential can see it — TestCheckTypeSoundness can, and did | writing a corpus row for the parked-native apply gate, 2026-08-29 |
| [NUR009](#nur009) | Bytes excluded from the DepScalar refinement bases — VERDICT 2026-08-15: WAIT for the ADR-012 `types/go` consolidation to close this through the refinement-base capability; no narrow fix meanwhile | 2026-07-22 uniformity review |
| [NUR026](#nur026) | Escape sets diverge between quoted strings and templates — NARROWED 2026-08-15: the escape VOCABULARY is resolved by fix (templates take the quoted-string set: \b \f \v \xNN \uNNNN, and an unknown escape drops its backslash); what remains is the malformed-input REPORTING difference, which needs an error channel the template lexer seam does not have | 2026-07-22 uniformity review |
| [NUR072](#nur072) | Three sugar kinds (mini, type-bound, lambda) still canon in DEBUG form after NUR059 — withdrawn there because the renders do not round-trip: SugarInfo does not retain the mini delimiter, and type-bound renders its Items rather than the bound's text; also carries the undecided bare-word question (`word(foo)` vs `foo`, 175 corpus rows) | NUR059's fix, 2026-08-15 |
| [NUR075](#nur075) | `deq` is extensible per type (`DeepEqualer`), `eq` is not — the one part of the retired NUR031's verdict its fix did not take: the divergences closed by adding kernel arms rather than by routing through `Behavior`, so a type can define its own deep equality but not its own identity | NUR031's fix, 2026-08-16 |
| [NUR076](#nur076) | A `behave`-installed capability is invisible to check mode, because `behave` does not run there — for `make` that turns a working program into a check FAILURE: a type whose Maker ignores the schema still has the schema's unknown/missing-field rules applied statically | NUR056's fix, 2026-08-17 (flagged by the PR #379 review, Codex P1) |
| [NUR060](#nur060) | The parser twins disagree on open-input sources beyond the corpus | PR #337 parity-probe sweep (flagged for NUR by Codex P1) |
| [NUR063](#nur063) | Seven self-knowledge words are proposed to dispatch from two module surfaces (`boru:debug` and `boru:scry`) — VERDICT 2026-08-15: `boru:scry` canonical, the `boru:debug` copies frozen behind shared handlers and deprecated on a stated timeline | design/BORU-SCRY.0.md §6 (flagged for NUR by PR #344 Codex P1) |
| [NUR064](#nur064) | Pattern clauses route-and-bind in `receive` but route-only in `add` — VERDICT 2026-08-15: defer to the processes/services design line, to be decided when those modules are built | `design/STATE-MACHINES.0.md` §8 (flagged for NUR by the PR #345 review, Codex P1) |
| [NUR065](#nur065) | Two spellings of the classifier role get different static guarantees: `classes:` is alphabet-closed and diagnosed, `classify:` is neither — VERDICT 2026-08-15: defer to the state-machine design line (its open question #7) | `design/STATE-MACHINES.0.md` §3.6 (flagged for NUR by the PR #352 review, Codex P1) |
| [NUR074](#nur074) | `canon` renders a function's PARAMETER names, so alpha-equivalent functions render — and digest — differently; NUR031's planned fix (render the anonymous fn literal) does not reach this | `design/unison-hash-identity-probe.0.md` P4 (flagged for NUR by the PR #376 review, Codex P1) |
| [NUR077](#nur077) | `StackForm`'s op vocabulary can CALL a word by name but cannot APPLY a function value, so an inline lambda or a fn read out of a container has no faithful representation — `Call{Name, Arity}` re-invokes by name and does not consume a receiver. `Eval` now refuses those forms (`ErrUnnamedApply`) rather than replaying them to a different answer — VERDICT 2026-08-17: resolve by fix, a NEW dedicated Apply Op (arity-carrying, consumes the value, seamed at `execFnDefLiteral`; `DoEval` stays reserved), after the three prerequisite recorder/gate defects are fixed | the `OnCall` frame-skeleton over-count fix, 2026-08-16 |
| [NUR078](#nur078) | A bare fn name before a `Function`-typed slot still resolves as a reference, against amended ADR-011 — the engine's TFunction intercept implements the exception the 2026-08-17 amendment struck (`h zero` ≡ `h zero/r` when the slot is `Function`-typed; a call/barrier before any other slot) — VERDICT 2026-08-17: resolve by fix, open-work item B (all four sites retire together, re-opening the NUR038 call-head question in the implementing PR) | the ADR-011 amendment, 2026-08-17 (flagged by the PR #381 review, Codex P1) |
| [NUR079](#nur079) | Gated words inside an imported file-module body escape the policy that governs the same call at top level — the module sub-registry inherited every capability seam except policy, so gates resolving `HostPolicy(r)` read nil as allow — VERDICT: resolve by fix in two halves; half (i) landed 2026-08-18 (the body now carries the parent's policy), half (ii) open | the Roc comparison study, 2026-08-18 |
| [NUR080](#nur080) | A typed def over an Integer literal loses its newtype brand under the compiler, and the bare literal gains one — VERDICT: resolve by fix | the Roc comparison study, 2026-08-18 |
| [NUR081](#nur081) | `Test.skip` is documented as a drop-in for `Test.check-prop`, but the two disagree on argument validity: `check-prop` now rejects `runs < 1` / `max-shrinks < 0` while `skip` accepts them silently | the Quint follow-up Wave 1, 2026-08-18 |
| [NUR082](#nur082) | Three tree-walking subcommands, two rules for `.boru/`: `fmt` and (now) `check` skip the package directory, `boru test`'s `discover()` walks it — VERDICT 2026-08-18: resolve by fix, one shared walk helper carrying the skip | giving `boru check` directory targets, 2026-08-18 (W-CLI-CHECK) |
| [NUR083](#nur083) | `check` and `build` anchor relative imports to the FILE's directory, `run` and `debug` to the process cwd, so `boru check sub/m.boru` now accepts a program `boru run sub/m.boru` refuses from the same cwd — VERDICT 2026-08-18: resolve by fix, `run`/`debug` adopt the file anchor (the multi-target `check` cannot use cwd at all) | multi-file `boru check`, 2026-08-18 (W-CLI-CHECK) |
| [NUR084](#nur084) | `-h` is not a uniform surface: FlagSet commands print their flags to stderr, `fmt` reads `-h` as a filename, and none exits 0 — though `boru help <cmd>` tells users to run it — VERDICT 2026-08-18: resolve by fix, `fmt` gains a FlagSet and `flag.ErrHelp` exits 0 | `boru check -h` failing as a missing file, 2026-08-18 (W-CLI-CHECK) |
| [NUR088](#nur088) | One signature has six valid spellings; `boru fmt` collapses only ONE of them to the short form, so four survive the formatter untouched and a `fmt`-clean file still carries several spellings of one signature | writing `STYLE-GUIDE.md` §S1, 2026-08-19 (`design/HIGHER-ORDER-FUNCTIONS.0.md` §4.2) |
| [NUR089](#nur089) | An inline `=>` lambda argument and a named `/v` reference to the SAME function are not equally checkable: the reference passes the check, the lambda draws `no_signature: cannot call g … got (Integer)`, and both run to the identical answer | the function-type prototype, 2026-08-19 (`design/HIGHER-ORDER-FUNCTIONS.0.md` §1.1) |
| [NUR091](#nur091) | A malformed `fn` declaration fails LOUDLY or SILENTLY depending on its output slot: `fn List [Integer] [size]` raises signature_error, `fn List Any [1]` strands its operands and binds nothing, exit 0 | the function-type prototype, 2026-08-19 |
| [NUR096](#nur096) | The check pass did not move with NUR095: a fn stored through a fn-SHAPE-typed member is APPLIED by both engines but still modelled by the checker as the inert fn it was before that retirement, so `TestCheckTypeSoundness` fails on the two multi-return `class.tsv` rows that pin it | adding the NUR095 retirement rows to `lang/spec/class.tsv`, 2026-08-20 |
| [NUR092](#nur092) | `varyRefusalLedger`'s stale arm is corpus-sensitive: adding an UNRELATED spec row can displace a seed from the hash-ordered 32-seed sample, empty a bucket, and instruct the author to delete a ledger entry whose refusal class is still live at larger breadth | adding NUR091's spec rows, 2026-08-19 |
| [NUR110](#nur110) | A FRESH `def` inside a branch that DID NOT RUN binds the name anyway in the compiled lane: `if false [def op 1] [0] end op` answers `[0 1]` compiled and `undefined_word` interpreted. The family-L CondBodyDepth gate only fires when a redefinition DROPS an existing overload, so shadowing is refused and fresh definition is not | measuring NUR109's premise, 2026-08-28 |
| [NUR122](#nur122) | A bare-name dispatch of a fn-typed frame binding whose runtime value does not match is a NAMED no-match on the interpreter and something else on the compiled lane: `def f fn [[g:Function x:Integer][Integer][g x]]  f (z:String => [z]) 5` raises `signature_error: cannot call `g`` interpreted and `type_error: f: expected 1 return value(s), got 2 — [fn (String) 5]` compiled (the frame replay parks the value); the paren spelling `(g x)` raises the no-match with an EMPTY name (`cannot call `` `) at the body's position; a 0-arg `g` fires interpreted (`[42 5]`) and parks compiled (`[fn 5]`); and inside a RETURNED closure the same paren spelling over a captured NAMED fn names that fn instead of the param — `def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h (app add/v)  (h 5)` raises `cannot call `g`` at 1:64 interpreted and `cannot call `add`` at 1:80 compiled, both exit 1 (measured 2026-09-05 on main, a COMPILING row). Same shape family as NUR119: the compiled value has no binding name and no named-dispatch semantics RE-MEASURED 2026-09-06: the NUR123 deopt increments moved two of the three without this record being touched — `f (z:String => [z]) 5` now AGREES in message and position (`cannot call `g`` at 1:43), and `f ([] => [42]) 5` agrees in message (`[42 5]`, was `[fn 5]`) with only its POSITION still differing (interp 1:50, compiled 1:43); the returned-lambda `(g x)` row is unchanged. What remains is one position-only divergence plus the lambda-VALUE body's empty dispatch name. FIXED 2026-09-06 (the nineteenth increment) for the DISPATCH NAME and its position: the trailing apply's head carries the binding name and read position it was recorded under (`CompiledFn.DynApplyName`), so `(g 5)` raises `cannot call `g`` at the read on both lanes — message, code, caret, candidate notes and forward-args help alike — for a plain fn body, a lambda VALUE's body and a capture (`app add/v` named `add` at 1:80 and now names `g` at 1:64). What remains is the position-only row (`f ([] => [42]) 5`, interp 2:1 vs compiled 1:43) and the WRITTEN-TUPLE half already recorded below: a WORD argument is substituted onto the value stack before the head dispatches, so the interpreter's tuple is empty (`takes 1 argument, but none were supplied`) where the compiled lane names the applied value. RE-MEASURED 2026-09-07: that rule is wrong three ways — the tuple is a PREFIX not a filter (`(g 5 y 6)` writes `[5]`, not `[5 6]`), the gap spans OpCallDynFrame as well as the trailing apply (multi-arg applies lower there), and the discriminator is SYNTACTIC not an operand kind (a body-local bound to a literal folds to PUSH_CONST and is still not written) — so closing it needs the SYNTACTIC fact rather than the lowered operand — which `NoteLocalRead` already records ungated for every bare read (`rec.localReads`), so no kernel change and no relaxation of noteWordRead's fn gate. | measuring the closure-capture family's blocker (a), 2026-09-05 |
| [NUR123](#nur123) | A BARE READ of a frame binding holding a fn is a WORD dispatch on the interpreter (stepWord routes a bound FnDefInfo through Registry.Lookup: a 0-arg fn fires, a no-match raises `cannot call `g``) and was a slot PUSH on the compiled lane: `def f fn [[g:Function][Any][g]]  f ([] => [42])` answered `fn` for 42, `def id fn [[x:Any][Any][x]]  id ([] => [42])` the same, `f (z:Integer => [z])` answered `fn (Integer)` for the interpreter's error — default lane, exit 0. Fixed for the fn-body residual (the replay re-steps the read as the word; fn-typed reads elsewhere refuse); FIXED for a gradual body-local's read (`def h fn [[m:Map][Any][def j (m get "f")  j]]  h {f: ([] => [42])}` is 42 on both lanes: its residual read seats the replay, and any other consumption — `j typeof`, `{a: j}`, `(j typeof)`, `(j) typeof`, `if true [j] [0]`, `for 1 [j typeof]`, `j (m get "g") add`, `j  def y 1`, `def k j`, `def k (j)` — deopts to the interpreter at its statement when the binding holds a fn; a point whose statement the compiled stack cannot match declines to the slot push: `5  j typeof` raises `[5 Function]` for the interpreter's `[5 Integer]`); FIXED inside a CODE BODY a native runs in the frame (`[1] each [j]` [42], `do [j]` 42 — the closure unit deopts on its capture slot and the enclosing unit binds the name); FIXED for a lambda VALUE's own body (2026-09-06, the fourteenth increment: `def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)` is 42 on both lanes where it answered `fn`, and the `j typeof` twin `Integer` where it answered `Function` — the escaping unit seats its body tokens and plans from its OWN frame, whose captures ride in slots for the whole apply; a read inside a BRANCH ARM of such a body still keeps its slot push); OPEN for the declined points and NUR119's render (a gradual PARAM's read refuses instead: the pass re-runs it under the argument's runtime type; re-measured 2026-09-05) | measuring the closure-capture family's blocker (a), 2026-09-05 |
| [NUR126](#nur126) | RESOLVED (2026-09-05, the twelfth and thirteenth increments). A RETURNED lambda's captured COMPUTED value was baked as an unrelated CONSTANT: `def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)` answered `7` — the caller's own argument — for the interpreter's `42`, and the factory's bytecode DROPped the computed value and pushed `PUSH_CONST` over the producing event's SEQ read as a const index (the disassembler panics on it: index out of range). The promotion planner never counted a RESIDUAL closure's captures, so their producers got no frame slot. The thirteenth increment closed the nested-fragment half: a multi-value branch ARM's residual is now RECORDED whole (`captureArmResidual`), so the planner may walk into the arm and force-promote its residual events — which also fixed the arm's own count-only reconciliation (`if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]` answered `7778 [7778 7779]` for the interpreter's `99 [7779 7780]`). An arm whose residual LEADS with a parked Function still declines the capture (the auto-apply screen) — the corpus's two vault-tui sites | measuring the closure-capture family after the eleventh increment, 2026-09-05 |
| [NUR124](#nur124) | A stack-shuffle word's result puts a produced closure on TOP and the interpreter RE-STEPS it there: `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  [(mk 3)] each [5 swap]` is `[15]` interpreted and `[fn (Integer)]` compiled; `[5 over]` is `[45]` / `[fn (Integer)]`; `[5 swap drop]` raises each_error interpreted and answers `[5]` compiled; and the OPPOSITE direction, measured 2026-09-06 and pre-existing on `6f583f6`, where the compiled lane APPLIES where the interpreter parks — `5 (mk 3)` is `5 fn (Integer)` interpreted and `15` compiled, with the `(mk 3) 5` twin agreeing, which places the fault on the value's ARRIVAL above an existing residual — default lane, exit 0. The top-level spellings refuse ("function value reaches swap (Stage 3)"); inside a code body over a produced closure the shuffle fold compiles. A FIFTH witness (2026-09-06, likewise pre-existing) runs the same way through the QUOTE rather than the park rule — `def appv fn [[g:Function][Integer][(g/v 5)]]  appv (z:String => [z])` parks `[fn g(String) 5]` interpreted and applies compiled, because callDynTrailTop strips the applied copy's quote to mirror a read-substituted arrival while a `/v` delivery hands the stored value over still quoted | measuring NUR123's closure bridge, 2026-09-05 |
| [NUR119](#nur119) | A fn value read through a PARAM's `/v` renders under the PARAM's name on the interpreter and under its own name on the compiled lane: `def app fn [[g:Function][Function][g/v]]  (app (z:Integer => [mul 3 z]))` renders `fn g(Integer)` interpreted and `fn (Integer)` compiled, and `def sq (z:Integer => [mul z z])  … app sq/v` renders `fn g(Integer)` against `fn sq(Integer)`. Same value, one render — the interpreter's frame binding re-labels the fn under the name it is read through, and a compiled unit pushes the raw runtime value. Pre-existing for the paren-placed spelling; the bare and args-following spellings refuse (`unconsumed fn-value carrier in residual (closure render)`, callResultRenderKnown) rather than diverge | measured 2026-09-05 while fixing the returned-closure park |
| [NUR118](#nur118) | A compiled fn's RETURN-CONTRACT error blames the body's first token where the interpreter blames the CALL SITE: `def f fn [[m:Map] [Integer] [m get "a"]]  f {a:"s"}` raises the same `type_error: f: return value 1: expected Integer, got ProperString` on both lanes, at `1:43` (the call) interpreted and `1:30` (the body) compiled. The compiled RET check is stamped with the unit's own position because one unit serves every call site (compiler StartFnCompile's fnPos) — a documented limitation that became reachable from a corpus-style row when the binding-sensitive unit memo let `def k 5  def f fn [[] [Integer] [k add 2]]  f  def k "x"  f` compile (Stage 4b); the row pins code and message and excludes the position | Stage 4b's cross-family rebind row, 2026-09-04 |
| [NUR114](#nur114) | A compiled diagnostic's caret is always ONE character wide where the interpreter underlines the whole token: the compiler's debug table is `[]core.SrcPos` carrying only row and column, so `stampAt` has no token text to set `BoruError.Src` from and the renderer's `caretCount = len(sub)` falls to its minimum of 1. Found while closing NUR108 — the positions now match exactly and the underline still does not | closing NUR108, 2026-08-30 |
| [NUR109](#nur109) | `parse` over an unbound def-scoped parser name raises `parse_error: the parser is not a usable function value` compiled and `parse_unknown_lang: no parser "op" is registered` interpreted. `TestParseFnDispatchMissParity`'s own header argues the compiled answer is the right one and asserted both lanes gave it — reading the interp side from `Run` | completing the NUR106 oracle sweep, 2026-08-27 |
| [NUR101](#nur101) | BROAD's placement depended on ENCLOSING CONTEXT: `(mk 1) 2` places (`fn (Integer) 2`) while `((mk 1) 2)` dispatches (`3`). **RULED 2026-08-26 "place uniformly"; the ruling's PREMISE was then FALSIFIED 2026-08-27** — the survivor count IS the question, and the enclosing group is a SECOND decision, not a modifier of the first. The interpreter was right all along; the COMPILER carried five silent miscompiles in both directions, hidden by 75 parity assertions that use post-Stage-J `Run` (the compiled path) as their interpreter oracle. See [design/PAREN-RESTEP-RULE.0.md](design/PAREN-RESTEP-RULE.0.md) | re-measuring §5.4 after #402, 2026-08-25; ruled 2026-08-26; ruling's premise falsified by measurement 2026-08-27 |
| [NUR099](#nur099) | `def <Capitalised> <fn>` is the ONLY door to an arbitrary predicate type, so it must stay ambiguous: the same fn body means a callable function under a lowercase name and a membership test under a capitalised one, and `def K fn [[a:Any b:Any][Any][a]] end K 1 2` therefore binds an uninhabitable type in silence — VERDICT 2026-08-25: resolve by fix, a `fnpred` word analogous to `fnsig` — **HALF LANDED 2026-08-25**: `fnpred` ships and the explicit route is live; what remains is migrating the 150 corpus sites off the capitalised-fn form and deleting the arity route behind it | reviewing the §5.1 diagnostic, 2026-08-25 |
| [NUR100](#nur100) | ADR-016 ("arity and origin never change function behaviour") is contradicted by live code: `RunPredicate` admits or refuses a function as a predicate purely on its parameter count, and `smallerArityOverload` gates a compile refusal the same way | the maintainer's ruling that the ADR-016 rule is absolute, 2026-08-25 |
| [NUR097](#nur097) | One syntax, two binding regimes: a closure CAPTURES parameters and fn-locals but resolves module-scope names LATE through the def stack, so a later `def` silently changes an existing closure's answer — verdict proposed: Allowed (top-level liveness) plus an in-file check hint | the higher-order capability audit's §5.6, re-assessed 2026-08-21 (`design/HIGHER-ORDER-FUNCTIONS.0.md`) |
| [NUR102](#nur102) | A predicate body runs a different number of times in each lane — overload pruning evaluates it 4× interpreted and 2× compiled, an effect-count divergence no differential gate can see because both lanes return the same value | the Stage-2 collection-kernel feasibility probe, 2026-08-25 |
| [NUR103](#nur103) | The checker's answer depends on who is asking: the same program yields a clean `boru check` and a refusing compile, so the tool a user would reach for reports the program fine — **one instance fixed 2026-08-26** (a nameless `undefined_word` from a Word-typed carrier); the mini-redis instance is diagnosed and OPEN, and its first fault is a coverage hole — `boru check` does not analyse a service-handler body at all, so a bare undefined word inside one ships clean | the server-concurrency corpus, 2026-08-26 |
| [NUR105](#nur105) | `boru check` does not analyse the body of a function VALUE constructed in ARGUMENT position — all three anonymous spellings (`=>`, `fn`, `afn`) — so `each ([e:Any] => [nosuchw e]) [1 2 3]` checks clean and then raises `undefined_word` at run time, an error the checker catches without difficulty when the identical body is `def`-bound. A code BLOCK argument and a named fn REFERENCE are both analysed, so position decides it, not spelling; measured matrix in the record | bounding NUR103's mini-redis half, 2026-08-26 |
| [NUR104](#nur104) | A record type means two different things depending on how it is spelt: the named spelling DISPATCHES and so evaluates its field map, the inline `o:{…}` rides inside the inert fn-spec list and never does — **the two field shapes found so far are fixed 2026-08-26**, the construction asymmetry is not | tracing NUR103, 2026-08-26 |
Pending records normally use a compact form (rule / divergence /
evidence / documentation status, plus a proposed verdict where one is
obvious). A record argued to a **resolve-by-fix** verdict keeps the
compact form and appends the verdict, since the fix — not prose — is
what will close it; a record **re-opened from Allowed** keeps the
argued form it already had, with the superseded allowance retained as
data. Expansion to the full argued form is what an **Allowed** verdict
requires.

---

## NUR099 — a fn body means a different thing under a capitalised name, and that is the only door to a predicate type {#nur099}

**Status:** Pending (verdict: resolve by fix) · **Recorded:** 2026-08-25 ·
**Surfaced by:** the maintainer's review of the §5.1 `stranded_type_call`
diagnostic

**Rule:** boru refuses loudly, and at the declaration. A declaration the
engine can already prove unusable is not accepted and left to fail later —
NUR091 is the same rule applied to a malformed `fn` triple.

**Divergence:** the declaration is accepted and nothing reports it.

```
$ boru do 'def K fn [[a:Any b:Any][Any][a]] end  K 1 2'
K 1 2
$ echo $?
0
$ boru do 'def K fn [[a:Any b:Any][Any][a]] end  4 is K   "s" is K   true is K'
false false false                     ← the bound type admits nothing at all
$ boru do 'def K fn [[a:Any b:Any][Any][a]] end  def z:K 5 end z'
error: def z: predicate type K: RunPredicate: predicate must take exactly one argument
```

The engine reaches a definite conclusion about `K` — but only when someone
uses it AS A TYPE. It never reports at the declaration, and never at all for
the spelling that actually happens: calling it.

**Not the return type.** `def <Capitalised> <fn>` is the predicate-type
declaration form and a non-Boolean body is legal — the **None-on-failure**
convention returns the value for a member and `None` for a non-member
(`lang/spec/record.tsv` §177: `def Positive fn [n:Integer Integer [if (n gt
0) [n] [None]]]`, return type Integer). So `def I x:Integer => [add 1 x]` is
a well-formed predicate admitting every Integer, and the return type cannot
tell a mistake from a predicate.

**The root cause is that one spelling carries two jobs.** The same fn body
means different things according to the CASE of the name it is bound to:

```
def even fn n:Integer Boolean [eq 0 (mod 2 n)]   → a callable function
def Even fn n:Integer Boolean [eq 0 (mod 2 n)]   → a predicate type
```

and the capitalised form cannot be refused, because it is the ONLY way to
declare an arbitrary predicate type. The type-constructing vocabulary —
`convert typeof inspect make refine class surface exposes gen of extends
default const base tor tand tany tall teq is as tis istype behave fnsig tnot
pathof` — has no predicate constructor. `refine` declines the job (*"base
must be Record, Table, or a class type, got Integer"*), and the comparison
predicates have their own door (`def Big (Integer gt 10)`), but an arbitrary
membership test has none. The capital is not something predicates want; it is
the only entrance available.

**Verdict (2026-08-25, maintainer): resolve by fix — add `fnpred`, the word
analogous to `fnsig`.** `boru describe fnsig` reads *"a function TYPE — a
function minus its body"*; `fnpred` is its mirror, a predicate TYPE — a
function kept FOR its body. One drops the body and keeps the shape, the other
drops the shape and keeps the body, and both bound to a capitalised name
produce a type: `fnsig`'s enforces a function's shape, `fnpred`'s enforces a
value's membership.

Once predicates have their own spelling, `def <Capitalised> <fn-with-body>`
denotes nothing legitimate and can be refused AT THE DECLARATION — the
outcome the maintainer asked for originally, reached without counting
parameters (compare NUR100, and ADR-016's absolute ban on arity-keyed
exceptions). It also retires the case-keyed meaning: a fn body would mean one
thing whatever the name's case.

**Half landed, 2026-08-25.** `fnpred` ships with both of `fnsig`'s forms —
the pair form `fnpred n:Integer [eq 0 (mod 2 n)]` and the spec-list form
`fnpred [[n:Integer] [eq 0 (mod 2 n)]]` — and the output slot is supplied
implicitly as `Any`, which is the honest declaration: both membership
conventions are supported and they disagree on the return type, so pinning
it to Boolean would refuse the None-on-failure form. `InstallType` now routes
on the DECLARATION (`IsDeclaredPredicateFn`) before falling back to the
parameter count, and `PredicateInputType` believes a declaration whatever the
fn's shape. Pinned in `lang/spec/fnpred.tsv` (18 rows) and
`core/go/fnpred_declared_test.go`.

What remains, and why it is not in the same change: the arity route
(`isPredicateFnValue`, and `PredicateInputType`'s count test) is still live,
because ~150 sites across the corpus declare predicates the old way. Deleting
it is a BREAKING change that needs those migrated to `fnpred` first. Both are
marked DEPRECATED in place with a pointer here so neither is extended
meanwhile. Only when they are gone does `def <Capitalised> <fn-with-body>`
become refusable at the declaration — the outcome this record exists for.

**Related.** The 1-argument cases (`def I …`, and the capitalised-constructor
convention `def New fn opts:Map Service […]` used by `design/examples/`) stay
reachable only at the USE site until this lands, which is what
`stranded_type_call` reports (`design/HIGHER-ORDER-FUNCTIONS.0.md` §5.1).

---

## NUR100 — ADR-016 forbids arity-keyed exceptions; two live sites use them {#nur100}

**Status:** Pending (no verdict) · **Recorded:** 2026-08-25 · **Surfaced
by:** the maintainer's ruling, 2026-08-25, that ADR-016's rule is absolute —
"everything everywhere every time and always"

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
   registers an overload consuming FEWER operands. Lower stakes (a
   compile-coverage conservatism, not an answer change — the lane falls back
   and the results agree), but the same shape, and introduced recently in
   PR #401.

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

**No verdict on sites 1 and 2.** The predicate role does need to test ONE
value, so removing site 1's gate needs a replacement contract, not a deletion
— and naming that contract is a design call the register should not pre-empt.
Recorded so the divergence between an accepted ADR and the code is not lost;
the fix is the maintainer's to direct.

---

## NUR126 — a returned lambda's computed capture was baked as an unrelated constant {#nur126}

**Status:** RESOLVED 2026-09-05 (the twelfth increment); the number stays
recorded here until the register's next sweep because its witness is the
clearest statement of the invariant it restores. **Found:** 2026-09-05,
measuring the closure-capture family after the eleventh increment.

**Rule:** a program matches or refuses; the two lanes agree on values.

**Divergence, measured on the default lane, exit 0:**

```
def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)
    interpreted  42        compiled  7      ← the caller's own argument
… (q 1)                                     ← and 1 for that call
```

**The bytecode said it plainly.** The factory computed the capture and
threw it away:

```
fn h/1 (locals=1) [m]:
  0002 CALL_NATIVE_POLY p0   ; get/2
  0003 DROP                  ; the computed value, discarded
  0004 PUSH_CONST  k3        ; 1 (Integer) — an unrelated constant
  0005 PUSH_CLOSURE f1
```

`pushOperand` materialises only const, local and type operands — "event
operands are already on the stack", as its own comment says — and its
switch ends in a `default` that treats anything else as a CONST. A
capture that reached it still naming its producing EVENT was therefore
emitted as `PUSH_CONST <event seq>`, the seq read as a const index. When
that index happened to be in range the closure captured whatever constant
lived there (the caller's argument, here); when it did not, the
disassembler panicked with `index out of range`, which is how the shape
was caught. Both are the same defect.

**Root cause.** `planValueDefLocals` promotes a computed producer to a
frame local when something still references it — and `forEachOperand`
deliberately surfaces a closure operand's captures so that a def used
ONLY as a capture is counted. A RETURNED lambda never passes through
that walk: its closure operand is the unit's RESIDUAL, and the planner's
residual input was a flat list of the out-ops' own event seqs, with
captures invisible. So the producer was never promoted, the capture
stayed an opEvent, and the push baked a constant.

**Fixed for a unit's RESIDUAL closure** by `appendResidualSeqs`
(compiler/go/emit.go), which walks a residual operand list INCLUDING each
closure's captures (recursing through nested closures) and feeds the
planner, so the producer is promoted and the capture lowers as
`STORE_LOCAL` + `PUSH_LOCAL`. Pinned in lang
(`closure_capture_promotion_test.go`: every value kind a capture can
hold, a capture used with the lambda's own param, two computed captures,
and a second instance of the same factory keeping its own value) and in
the compiler (TestAppendResidualSeqs over the operand kinds).

**Was open: a capture produced inside a NESTED FRAGMENT** — a def made in a
branch or case ARM, captured by a lambda built in that same arm, reached the
push as an opEvent still, because `collectPromotableEvents` deliberately
stopped at a multi-value arm:

```go
// Only recurse into a SINGLE-result fragment … A MULTI-value arm
// (residualN>1 …) leaves several residual values on the sim stack, and
// promotion / dead-drop would wrongly store or drop one of them
if frag != nil && frag.residualN <= 1 {
```

Three hypotheses were eliminated on the way to that line and need not be
retried: the body operand is NOT kept outside `ev.call.ops`
(`RecordClosureCall` appends an ordinary `evCall` whose `ops[bodyPos]` IS the
closure operand); the planner does NOT see the capture and decline it (an
instrumented run shows the walk never reaches it); and it is NOT the unit's
residual closure (seeding `captured` from `rec.outOps` changes nothing). A
capture-only walk over the whole fragment tree was also measured and is NOT
sufficient — the promotion LOOP iterates the same restricted set.

**Fixed 2026-09-05 (the thirteenth increment) for an arm whose residual is
recordable.** The blocker was that the planner could not tell an arm's
residual values from its intermediates: `fragmentResultSeqs` marks only the
single out operands and a multi-value arm's residual appeared in no recorded
list. It does now — `captureArmResidual` (compiler/go/emit.go, the branch
twin of `RecordLoop`'s all-inert capture) records the whole residual as
resolved operands, EVENT entries included, because an arm runs once per taken
path. `armRepushableResidual` then opens the walk into such an arm,
`armResidualForceSeqs` force-promotes each residual event to a frame slot
(exempting it from `fragResultStaysOnSim`), and `RewritePromotedRefs` rewrites
`frag.residualOps` so `lowerFragment`'s re-push arm reads the slots. The arm
now lowers exactly as the PROGRAM residual always has: store each computed
value, then push the whole residual in the interpreter's order.

That closed a second, larger divergence the same measurement exposed — the
arm's residual was only COUNTED, so a stray leftover made the count match
while the values did not:

```
if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]
    interpreted  99 [7779 7780]        compiled  7778 [7778 7779]
def m {a: 3}  if true [ def c (m get "a")  99 (each [ … ]) ] [ 7 8 ]
    interpreted  99 [4 5]              compiled  3 [1 2]
```

Four such exit-0 wrong answers are fixed and five arms that refused ("branch
leaves extra values") now compile; pinned in lang
(`multivalue_arm_residual_test.go`, parity plus the declines) and in the
compiler (`TestCaptureArmResidualArms`, `TestArmRepushableResidual`).

**Still open, and now named exactly: an arm whose residual LEADS with a
parked Function.** `captureArmResidual` keeps the loop side's screen — a
Function value in the region auto-applies in the interpreter when a later
value lands above it — so such an arm declines the capture and keeps the
count-only path. That is precisely the corpus's two remaining sites:
`lang/go/modules/vault_tui.boru:1105` and :1108 (`def cols (vt-rtable-cols
scr.name)` captured by `(rec:Map => [ vt-rrow cols rec ])`), whose `case` arm
leaves four values led by the deferred `Tui.table` member — measured
`ARMCAP decline fn n=4 at 0`. They are reached by `module-vault-tui.tsv` L21
and L24 and pass today only because the closures they build are never invoked
(L21 reads the map's KEYS; L24 errors on a missing terminal backend), so the
wrong capture is unobserved — which is also why a refusing latch was tried
here and withdrawn: it cost those two rows against a corpus refusal ceiling of
0 while fixing nothing observable. Admitting a parked Function to the re-push
is the parked-fn auto-apply family (NUR121, NUR124), not this record.

**Not fixed, and now correctly attributed** (it was hidden behind the
bogus constant): the same shape with a FN-valued capture read bare as the
lambda's whole body answers `fn` where the interpreter dispatches the
binding as a word and answers `42`. That is NUR123's open lambda-body
case, whose record claimed such a body "refuses soundly" — it compiles
and diverges. Its fix is the deopt inside a lambda unit (the handoff's
twelfth-increment note).

## NUR124 — a stack-shuffle word's result re-steps a produced closure on the interpreter only {#nur124}

**Status:** Pending. **Found:** 2026-09-05, measuring the closure bridge of
NUR123.

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

**The written-tuple half is RE-MEASURED and its rule CORRECTED** (2026-09-07,
the twentieth increment). What the nineteenth recorded — "a literal or
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

**The site is located, and one fix was tried and WITHDRAWN** (2026-09-06). The
boundary is exact: the residual must be exactly `[literal, 1-arg closure]` —
`(mk 3) 5`, `5 (mk 3) 7` and `1 2 (mk 3)` all agree, and the rest of the family
refuses. That is the shape `resolveDynamicApply`'s `trailingApply` helper
accepts: an event-produced single-output fn on the sim top over a plain static
arg. Every SIBLING arm of `resolveDynamicApply` consults the park rule
(`callResultPlaced` / `placedNotReStepped`); `trailingApply` checks only the
shape, so the arity-1 apply fires on a value the paren placed with one
survivor.

Adding `callResultPlaced` there does remove the miscompile — the row refuses
("residual shape beyond Stage 1 (call result above a literal)"), a sound
fallback rather than a wrong answer — but it costs a corpus row against a
refusal ceiling of 0, so it was withdrawn:

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

## NUR123 — a bare read of a fn-valued frame binding is a word dispatch the compiled lane never made {#nur123}

**Status:** Pending — FIXED for the fn-body residual and for every fn-typed
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
fire. OPEN for a declined point (a literal or a read pushed before the
statement and still pending at the test: `5  j typeof`) and for
the `/v` render (NUR119). A read inside a BRANCH ARM of a lambda body,
and an `each` body nested in one, were closed by the fifteenth increment
(2026-09-06). **Found:** 2026-09-05, measuring the closure-capture
family's blocker (a) on the tree after NUR120/NUR121 landed.

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
all refuse soundly. Every row above bar the last three now agrees on both
lanes (`lang/go/word_read_dispatch_test.go`), the container and arm rows
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
`[1] each [x:Integer => [j]]`, refuses soundly). STILL OPEN: a lambda
value's own body (`([] => [j])` renders `fn` on both lanes and escapes
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

## NUR122 — a compiled fn-value apply has no name and no named-dispatch semantics {#nur122}

**Status:** Pending. **Found:** 2026-09-05, measuring the closure-capture
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
of unknown provenance"), a sound fallback.

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

## NUR119 — a fn value read through a param's `/v` renders under the param's name on one lane only {#nur119}

**Status:** Pending. **Found:** 2026-09-05, measuring the returned-closure
park (`design/PAREN-RESTEP-RULE.0.md` §2.1).

**Observed.** A user fn that returns a fn it was HANDED, read through the
param's `/v`:

```
def app fn [[g:Function][Function][g/v]]  (app (z:Integer => [mul 3 z]))
    interpreted  fn g(Integer)        compiled  fn (Integer)
def sq (z:Integer => [mul z z])  def app fn [[g:Function][Function][g/v]]  app sq/v
    interpreted  fn g(Integer)        compiled  refused (closure render)
```

The VALUE is the same function on both lanes (apply it and both answer
alike); only its render differs. The interpreter's frame binding re-labels
the fn under the name it is read through — `sq` becomes `g` inside `app` —
and a compiled unit pushes the raw runtime value, which carries its own
name (or none, for a compiled closure).

**Pre-existing** for the paren-placed spelling (`(app …)`, one survivor,
placed by the paren) on every binary measured. The bare and
args-following spellings (`app (…)`, `app (…) 7`) used to REFUSE the
residual ("unconsumed fn-value carrier in residual (closure render)") or
APPLY it (`21` — the returned-closure park miscompile, fixed the same day);
they now refuse under the render gate: `callResultRenderKnown` admits a
parked call result only when the callee returns a compiled anonymous
closure whose render string the compiler carries. The `Any`-typed twin
(`[[g:Function][Any][g/v]]`) is not a fn-typed carrier and slips the gate:
`app (…)` renders `fn (Integer)` against `fn g(Integer)` on the default
lane, exit 0 — the one live divergence this record leaves open.

**Proposed verdict.** Resolve by fix, on the compiled side: the VM's
`/v` read of a fn-typed frame local (or the frame entry's param binding)
re-labels the value under the binding name exactly as the interpreter
does, so a compiled unit returns the same render. Until then the render
gate holds the fn-typed shapes to the interpreter and this record names
the `Any`-typed one.

## NUR118 — a compiled return-contract error blames the definition where the interpreter blames the call {#nur118}

**Status:** Pending · **Recorded:** 2026-09-04 · **Surfaced by:** the
binding-sensitive unit memo (Stage 4b) letting a cross-family rebind row
compile

**Rule:** the two lanes agree on errors, and — NUR108 established — the
position is part of the error a user meets.

**Divergence:** same code, same message, same secondary span (the
declaration); a different PRIMARY row and column. Measured on the tree
before Stage 4b touched it, so it is pre-existing and reachable from any
compiled fn whose declared return fails at run time:

```
def f fn [[m:Map] [Integer] [m get "a"]]  f {a:"s"}

interp   --> 1:43   (the call `f`)
compiled --> 1:30   (the body's first token, `m`)
```

**A second flavour, measured 2026-09-05 (closing NUR120):** where the
contract is applied to a fn VALUE at invoke time rather than at a unit's
RET — a lambda read from a map member (`def m {f: ([x:Integer] => [x
1])}  m.f 5`) or handed to a Function param and called inside a user fn
(`def ap fn [[g:Function][Any][(g 5)]]  ap ([x:Integer] => [x 1])`) —
the compiled count error carries NO position at all (`--> source position
unknown`) where the interpreter blames the call (`1:36`, `1:31`). Same
code, same message; the closure value's RetPos is the fn's own position,
empty for an inline lambda at those two seams.

**Where it comes from, traced not guessed.** The interpreter's return
check (`__RC`) raises at the CALL, whose token position it has. The
compiled RET check is stamped from the unit's debug table, and a unit is
compiled ONCE for every call site, so it cannot carry any one site's
column: `StartFnCompile` takes the body's first-token position as the
unit's `fnPos` precisely so the error is not reported at an unknown
position (its own comment says "It cannot equal the interpreter's
call-site column (one unit serves every call site)").

**Why it surfaced now.** `def k 5  def f fn [[] [Integer] [k add 2]]  f
def k "x"  f` used to REFUSE under the frozen-read latch and run on the
interpreter; under the memo the second call re-records `f` against the
String binding, the unit compiles, and its RET raises the interpreter's
own type_error — at the definition. lang/go's
`TestModuleReadCrossFamilyRebindCompiles` pins code and message and
excludes the position, naming this record.

**Not a NUR114 duplicate.** NUR114 is the caret's WIDTH at a position the
lanes agree on; this is the position itself, for one error class.

**Proposed verdict:** resolve by fix — the RET check needs the CALL
SITE's position, which the VM has (the caller's pc and its debug entry)
at the moment the callee's RET runs; the stamp should read it from the
frame it returns to rather than from the unit. Until then the row
excludes the position, and every other compiled return-contract error
divergence of this shape is this record's.

---

## NUR114 — a compiled caret underlines one character where the interpreter underlines the token {#nur114}

**Status:** Pending · **Recorded:** 2026-08-30 · **Surfaced by:** closing
NUR108

**Rule:** the two lanes agree on errors, and the RENDERED diagnostic is the
error as a user meets it — NUR108 established that the position is part of
it, and the underline is the same claim about "where do I look".

**Divergence:** same code, same message, same row and column; different
underline.

```
def x:(Integer gt 10) 5 x

compiled → 1 | def x:(Integer gt 10) 5 x
               ^   def x: value 5 does not unify with declared type (Integer gt 10)
interp   → 1 | def x:(Integer gt 10) 5 x
               ^^^ def x: value 5 does not unify with declared type (Integer gt 10)
```

**Where it comes from, traced not guessed.** `renderSite` sets
`caretCount = len(sub)` from `BoruError.Src` — the TOKEN TEXT — with a floor
of 1. The interpreter's `stampErrPos` copies that text out of the `SrcPos` it
stamps with (`pos.Src`, e.g. `"def"`). The VM's `stampAt` cannot: the
compiler's debug table is `Debug []core.SrcPos` built from event positions,
and although `core.SrcPos` HAS a `Src` field, nothing on the compile path
fills it — every entry is row/col only.

**So it is not a one-line mirror of `stampErrPos`.** Adding the `Src` copy to
`stampAt` was written and measured: it changes nothing, because
`debug[pc].Src` is empty at every site. Inert code, reverted rather than
shipped. The work is upstream — carrying the token text through the recorder
into the debug table — and it is worth costing against how much of the table
it widens before it is done.

**Not urgent, and the reason is worth stating.** Unlike NUR108, this cannot
send a user to the wrong place; it under-marks the right one. It is recorded
because "the diagnostics match" is a claim this project makes, and after
NUR108 the caret is the last thing about these two renderings that does not.

---

## NUR112 — a parked native applied after its name was extended is checked as two values {#nur112}

**Status:** Pending · **Recorded:** 2026-08-29 · **Surfaced by:** writing a
corpus row to pin the VM's parked-native apply gate; the row never landed,
the observation did.

**Rule:** the checker's residual is a claim about what the program leaves.
When it names more values than run, every consumer downstream of it — the
compiler's stack model included — is planning against a shape that does not
occur.

**Divergence.** The program answers `3` on both engines. The checker says it
leaves two values:

```
def Pos (refine Integer)
def m {a:size/v}
def size fn [[n:Pos] [Integer] [200]] end
def v:Pos 3
m.a v

  checked  [dynamic(Any) Pos]
  actual   [Integer]
```

**Why it is not a differential finding.** The two ENGINES agree — both run
the parked value's own native sigs and answer `3`. What differs is the
static model, so `TestSpecCompiledDifferential` and the compile-or-fallback
walk are both blind to it by construction. `TestCheckTypeSoundness`
(`test/go/langspec/check_accuracy_test.go`) is the gate that compares the
checked residual against the real one, and it is what caught this.

**The shape.** A fn value parked from a name (`size/v`) while that name was
purely native, then applied after a boru overload EXTENDED the name — the
one rebinding `extend_owner` permits. The checker appears to model the
dispatch as not consuming its argument (`Pos` survives in the residual) and
to type the result dynamically, where at run time the native sig consumes
the argument and yields one Integer.

**Scope.** Stage 8 (checker totality). Related to but distinct from the
retired NUR111 (resolved 2026-08-30, record deleted per this file's
lifecycle rule): that one was the checker declining to enforce a
declaration it can see, this one is the checker's residual disagreeing
with the runtime's. Both are the static model, neither is an engine
divergence.

**Not pinned in the corpus.** A row for this shape would trip the
type-soundness pin (0 violations) on landing, which is the correct
behaviour for a pin and the wrong way to record a finding. The VM gate the
row was written for is pinned by a handler test instead
(`core/go/value_classify_stage5_test.go`, `TestParkedNativeApplyGate`).

---

## NUR110 — a def in an untaken branch binds anyway {#nur110}

**Status:** Pending · **Recorded:** 2026-08-28 · **Surfaced by:** checking what
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
| shadowing a VALUE binding | refused (residual provenance) | correct | sound |
| shadowing a FN overload | refused (family L, by name) | correct | sound |
| zero-iteration `for` body | unbound | unbound | correct |
| taken branch | binds | binds | correct |

The zero-iteration loop row is the useful one: the same question is already
answered correctly there, so the machinery to get this right exists.

**Verdict: resolve by fix, compiler-side — and refusing is a fix.** Its two
siblings above refuse, and a refusal is sound (the interpreter runs the program
correctly); a silent wrong binding is not. Full graduation is the same one
family L already names — "a runtime dispatch respecting the conditional
binding" — which is Stage 4/5's def-twin work, not Stage 3's. Pinned as
measured meanwhile by `lang/go`'s `TestCondBodyFreshDefBindsCompiledOnly`.

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
   the same pass survives it. Fixing the recorder's rollback is a repair;
   refusing is the fallback if it is not reachable.

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

## NUR109 — an unbound parser name is two different errors {#nur109}

**Status:** Pending · **Recorded:** 2026-08-27 · **Surfaced by:** completing
the NUR106 oracle sweep

**Rule:** the two lanes raise the same taxonomy for the same program.

**Divergence:** `parse op 'inc'`, where `op` was bound only inside a branch
that did not run —

```
compiled → [boru/parse_error]: parse: the parser is not a usable function value
interp   → [boru/parse_unknown_lang]: parse: no parser "op" is registered
```

**The compiled answer is the specified one**, and the test that hid this says
so in its own header: *"a def-scoped parser value has no 'missing kind' — only
an unusable value, and the two engines must agree on it."* The interpreter
still falls back to the kind-name miss, because an unbound def-scoped name is
indistinguishable to it from an unregistered kind.

`TestParseFnDispatchMissParity` asserted both lanes raised `parse_error` and
read its interp side from `Run` — the compiled lane (NUR106) — so it compared
`parse_error` to itself.

**Verdict: resolve by fix, interpreter-side.** The interpreter needs the same
distinction the compiled dispatch already makes: a name that resolved to a
value which is not a usable parser is not a missing kind. Pinned as measured
meanwhile.

**THE VERDICT IS SUSPENDED, 2026-08-28 — its premise does not hold.** It rests
on "the compiled answer is the specified one", and the compiled answer rests in
turn on `op` being BOUND at the parse site. Measured, that binding is itself a
miscompile: `op` is defined only inside a branch that does not run, and the
compiled lane binds it anyway ([NUR110](#nur110)). The interpreter's
`parse_unknown_lang` is the CONSISTENT answer for a name that is not bound —
which is what both lanes say when the same `op` is read bare on the interpreter.

So this is a SYMPTOM, not an independent divergence, and "fix the interpreter to
agree" would have taught it to agree with a wrong binding. Re-derive the verdict
once NUR110 is closed: with `op` correctly unbound, both lanes should reach the
unregistered-kind path and the question may not survive at all.

This is the third time in this stage that a record's verdict named the wrong
lane — §6.4's "the interpreter is wrong" and NUR101's ruling premise were the
others. The common cause each time was reasoning from the compiled lane's answer
without checking what that answer rested on.

---

## NUR101 — BROAD places a REFERENCED fn but still dispatches a COMPUTED one {#nur101}

**Status:** Pending · **Recorded:** 2026-08-25 · **Surfaced by:** re-measuring
`design/HIGHER-ORDER-FUNCTIONS.0.md` §5.4 against the post-#402 tree; the
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
[design/O1-RELITIGATION.0.md](design/O1-RELITIGATION.0.md).

**RULED 2026-08-26 — place uniformly.** A computed function applied inside
an enclosing group PLACES, exactly as its unwrapped twin does. `((mk 1) 2)`
becomes `fn (Integer) 2`, and `[((mk 1) 2)]` becomes `[fn (Integer) 2]` on
both lanes. There is no enclosing-context exception: ADR-011's carve-out
stays what it says, a bare WORD inside a group, and a computed group result
is not one.

Both lanes move. The compiled lane is NOT already at this answer for the
enclosing-group case — `lang/go/bytecode_curried_test.go:17-24` pins compiled
`((mk 1) 2)` as `[3]` — so the fix is interpreter AND compiler, and that
fixture is rewritten with it. `design/HIGHER-ORDER-FUNCTIONS.0.md` §5.4's
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
[design/PAREN-RESTEP-RULE.0.md](design/PAREN-RESTEP-RULE.0.md).

---

## NUR000 — Boolean arithmetic is a defined error {#nur000}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The six arithmetic words (`add`/`sub`/`mul`/`div`/`mod`/`pow`) are
**total within every scalar type and every Micron kind**
(REFERENCE.md §"Within-type operations"): numbers compute, `String` and
`Atom` carry the occurrence package, `Bytes` mirrors it over byte
subsequences, Microns fall to the field-wise default.

### The divergence

`Boolean` is the single scalar family excluded: `add true false` raises
`[boru/type_error]: add: arithmetic is not defined on Boolean` — for all
six ops.

### Why allowed

Boolean deliberately carries the logical words (`and`/`or`/`xor`/`not`)
instead of arithmetic; every candidate arithmetic semantics (C-style
integer promotion, GF(2)) is arbitrary, and an arbitrary choice would
be silently accepted where a loud error teaches the logical vocabulary.
The exception is implemented as a **registered** `[Boolean Boolean]`
signature that raises with a pinned message (the `setMicron` precedent)
rather than by signature absence, so the failure is specific instead of
an opaque dispatch error; a check-mode mirror (`booleanArithReturns`)
flags concrete Boolean arithmetic statically; and the signatures are
CoreDefault, so a user's `refine Boolean` overload can still extend an
arithmetic word by specificity (the refinement escape).

### Evidence

- `lang/go/native/native_scalar_ops.go` — `booleanArithHandler` /
  `booleanArithError` / `booleanArithReturns`; the six erroring
  `[Boolean Boolean]` signatures.
- REFERENCE.md §"Within-type operations" — "**`Boolean`** arithmetic is
  a **defined error**".
- `lang/spec/scalar-micron-ops.tsv` (all six ops pinned as errors);
  `lang/spec/open-words.tsv` (the refine-extension escape and its
  negative twins).

---

## NUR001 — `convert Boolean` coerces by presence, not content {#nur001}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

`convert <ScalarType> <String>` parses the string's **content**:
`convert Integer "42"` → `42`, `convert Float "1.5"` → `1.5`.

### The divergence

`convert Boolean "false"` → `true`. Boolean conversion applies the
truthiness rule — `false`, numeric zero in any leaf
(`0`/`0.0`/`0d0`) and `""` are false; a String's characters are never
inspected.

One neighbouring defect is recorded separately and does not disturb
this allowance: the language's falsy set also contains `none`, `[]`
and `{}`, but `convert`'s source slot is Scalar-only and refuses all
three with a `signature_error` (NUR053).

### Why allowed

`convert Boolean` shares **one** coercion rule with `if`-condition
truthiness and `make Boolean` (presence, not content); making the
conversion path parse content would fork truthiness into two rules —
a worse non-uniformity than the one it fixes. The three consumers apply
the same rule but do not accept the same *domain*; that separate
divergence is NUR053 and does not disturb this allowance, which is
about content-vs-presence. Content parsing exists as
an explicit opt-in: `convert Boolean {truthy: true}` parses the YAML
tokens (`y`/`yes`/`true`/`on` and `n`/`no`/`false`/`off`,
case-insensitive) and
falls back to presence for anything else; the option is inert for
non-Boolean targets.

### Evidence

- `lang/go/native/native_type.go` — `coerceBooleanTruthy` and the
  `truthy` option plumbing on `convert`.
- REFERENCE.md — "**`convert Boolean` is presence coercion; `{truthy:
  true}` opts into YAML parsing**" and §`if` ("coerces its condition …
  the exact same rule as `convert Boolean` and `make Boolean`").
- `lang/go/native/native_type_convert_seam9_test.go` (both modes,
  positive and negative); `lang/go/native/integration_coverage_test.go`
  (`'false' convert Boolean` → `true` pinned explicitly).

**Review (2026-07-31):** re-affirmed by the maintainer
(`design/NUR-RESOLUTION-PLAN.0.md`). The single coercion rule this
record leans on is now specified once — with every consuming construct
enumerated — in `design/TRUTHINESS.0.md` (the One Truthiness Model);
an ADR stating the model as a language principle is a recorded
candidate there.

---

## NUR002 — Value enumeration exhausts finite domains; Boolean is the built-in instance {#nur002}

**Status:** Allowed · **Date:** 2026-07-22 · **Rewritten:** 2026-07-31
(maintainer — the pre-rewrite record framed this as a Boolean special
case; the rewrite states the general rule instead)

### The uniform rule (as rewritten)

**Exhaustive coverage of any finite domain does not require a default
branch.** For a scalar scrutinee, a default-less `case` proves
exhaustiveness through type clauses, `[is T]` predicates,
comparison-predicate / refinement interval unions, or — when the
scrutinee's domain is **finite** — by enumerating its values. An
infinite scalar can never be covered by literal enumeration
(`case n [1 … 2 …]` can not cover `Integer`).

### Where Boolean sits

Boolean is not a special case: it is the built-in two-value
pseudo-enum. `true` + `false` cover a `Boolean` scrutinee exactly as
enum members cover an enum: `case b [true 1 false 0]` is statically
exhaustive with no default, and `case b [true 1]` is a
`case_not_exhaustive` check error (`uncovered: false`).

### Why this is the rule, not a divergence

Coverage-by-enumeration follows from cardinality, not special
pleading: a domain is enumerable iff it is finite, so the checker's
coverage proof stays in the sound direction throughout. The mechanism
is the one value/type coverage channel enums use (`def Color (red/q
tor …)` covered member by member) — and enums are themselves
specialisations of disjunct types, so the general principle is
**finite disjunct exhaustiveness**, of which Boolean is the built-in
instance. Documentation should present it that way rather than
presenting Boolean as special.

### Follow-on design work (recorded 2026-07-31, not yet scheduled)

Finite **dependent scalar types** also define finite domains, and
should eventually enter the same coverage channel. Two items to
investigate (`design/NUR-RESOLUTION-PLAN.0.md`):

- **Ergonomics** — allow a finite dependent type to be declared by
  enumerating its values (a `{2,3,4}`-style literal domain) rather
  than forcing range predicates (`Integer >=2 and <=4`).
- **Implementation** — when a finite dependent set is statically
  known, avoid materialising large sets; prefer symbolic/range
  representations. Where free variables remain, a symbolic
  representation is necessary regardless.

### Evidence

- REFERENCE.md §"`case` — dispatch and exhaustiveness" — "**Boolean, by
  `true` and `false`** (or the `Boolean` literal)", including the
  negative example.
- `lang/spec/case.tsv` §6 ("true+false cover Boolean", alongside the
  union and enum coverage rows that show the shared mechanism).

---

## NUR003 — `and`/`or` select an operand; the rest of the boolean family returns strict Boolean {#nur003}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The boolean word family returns strict `Boolean`: `not`, `xor`, `any`,
`all`, and the `boru:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/
`implies`) all coerce their inputs by truthiness and yield `true` or
`false`.

### The divergence

`and` and `or` are value-selecting short-circuit connectives: they
return whichever **operand** decided the result, of whatever type —
`1 and 2` → `2`, `false 5 and` → `false`, `0 9 or` → `9`.

### Why allowed

Deliberate Lisp/Python semantics: the operand form composes directly
(`x or default`, with `otherwise` as the None-aware variant), and a
strict Boolean is one `not not` (or a comparison) away. The divergence
is loudly documented at the word table itself, and check mode types the
result precisely — `foldOrJoin` concrete-folds statically-decided
selections and otherwise narrows to the join of the operand types, so
the non-uniform return type never degrades static analysis to `Any`.

### Evidence

- `lang/go/native/native_boolean.go` — `andHandler`/`orHandler` (operand
  return) vs `notHandler`/`boolBinaryNative`/`anyHandler`/`allHandler`
  (strict Boolean); `foldOrJoin` for the check-mode typing.
- REFERENCE.md §Boolean — "**`and` / `or` return an operand, not a
  coerced boolean.**"

**Review (2026-07-31):** re-affirmed by the maintainer
(`design/NUR-RESOLUTION-PLAN.0.md`). The operand-return semantics —
short-circuit behaviour, evaluation order, which operand is returned,
and the interaction with static typing — are specified in
`design/TRUTHINESS.0.md` §"The connectives", which this record now
leans on.

---

## NUR004 — Boolean, Atom and Bytes have no lattice subtypes {#nur004}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The scalar branch families carry structural leaves: `String` has
`EmptyString`/`ProperString`, `Number` has `Integer`/`Float`/
`BigInteger`/`BigDecimal`, `Micron` has its twelve kinds.

### The divergence

`Scalar/Boolean` and `Scalar/Atom` are leaf-less — direct children of
`Scalar` with no builtin subtypes (no `True`/`False` lattice nodes).
`Scalar/Bytes` is a third leaf-less child, registered from the language
layer (`native_bytes.go`) rather than declared in `builtinDecls`. The
same reasoning covers it with one caveat: of the two value-level
substitutes named below, `case` literal coverage does not reach Bytes
(the domain is infinite) and DepScalar refinement construction is not
available for it either (it is not a supported refinement base —
NUR009). The nominal-split route, `refine Bytes`, does work, and is
what stands in for subtypes here.

### Why allowed

Vacuous rather than divergent: no kernel mechanism requires a scalar
family to have leaves, and nothing dispatches on their presence. There
is no useful structural split of Boolean — `True`/`False` subtypes would
duplicate what value-level machinery already provides uniformly (`case`
literal coverage per NUR002, and DepScalar refinements: `(Boolean gte
true)` *is* the true-only subset, since Boolean is one of the supported
refinement bases — `canonicalBaseType` admits Integer, Float, Number,
String, Boolean and Atom). Users who want a
nominal split can mint it (`refine Boolean`), which participates in
dispatch by specificity like any refinement.

**Clarified (2026-07-31, maintainer):** the two layers this record
separates are the **lattice subtype hierarchy** (structural leaves the
kernel dispatches on — `EmptyString`/`ProperString`, the Number
leaves) and **value-level finite sets** (the inhabitants of a finite
domain — what `case` coverage and DepScalar refinements operate on).
`true`/`false` belong at the value layer: they are the two members of
a finite domain (NUR002, as rewritten), not structural variants of the
type, so minting `True`/`False` lattice leaves would put value
distinctions into the structural layer — the wrong home for them.

### Evidence

- `core/go/typetable.go::builtinDecls` — the Scalar branch layout.
- `core/go/depscalar.go::canonicalBaseType` — Boolean listed among the
  supported DepScalar bases (`Boolean gte true` constructs).
- `lang/spec/case.tsv:75` (true+false cover Boolean) and
  `lang/spec/edge-types-1.tsv:82-85` / `lang/spec/open-words.tsv:26-29`
  (`refine Boolean` mints a nominal split that dispatches) — the
  value-level machinery that stands in for subtypes.
- `lang/go/native/native_bytes.go:23` — the `Scalar/Bytes`
  registration, the third leaf-less child.

---

## NUR005 — String `add` is the sole cross-type exception to same-type arithmetic {#nur005}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict and rewritten wording: maintainer, via
`design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

Scalar arithmetic is **same-type arithmetic**: the six words are
"applied within a type, never across it" — REFERENCE.md §"Within-type
operations". A cross-type pair has no signature and raises
`[boru/signature_error]`. Where a signature is *deliberately registered
to refuse*, the failure is instead a coded error with a specific
message — `[boru/type_error]` for `Big`⊕`Float`, for Boolean
arithmetic (NUR000), for a cross-KIND Micron pair, and for several
within-kind Micron restrictions (`mul` on two Qions); `[boru/arith_error]`
for a Qion currency mismatch.

### The divergence

`add` carries `[String Scalar]` / `[Scalar String]` overloads that
stringify the non-String operand (`add "x" 5` → `'5x'`), while Atom
`add` is `[Atom Atom]`-only and Bytes `add` is `[Bytes Bytes]`-only.

### Why allowed

**String `add` is the sole language-level exception to same-type
arithmetic, and it is deliberate.** Concatenation-with-coercion is the
overwhelmingly common string operation, the coercion is total and
canonical (every Scalar has one string render), and the overloads
require **at least one** String operand, so they never manufacture a
concatenation of two NON-String operands: `add true 1` raises
`[boru/signature_error]` — no concat overload matches without a String,
and no within-type arm matches a Boolean/Integer pair either, so the
refusal is a dispatch miss rather than the registered `type_error`
NUR000 installs for Boolean arithmetic. ("String-or-bust" governs the
concat overloads only; two non-String scalars of the SAME type still
have their within-type arm — `add 1 2` → 3, `add a/q b/q` → 'ba'.)

The Pending record's framing — that Atom and Bytes "do not mirror it" —
treated the trio as an
architectural grouping obliged to move together; the verdict is that
the String/Atom/Bytes occurrence-package parallel is a **documentary
comparison, not an architectural grouping**. Nothing requires Atom or
Bytes to adopt a cross-type overload because their within-type
packages mirror String's, and neither has String's coercion case: an
Atom is a name and Bytes are raw octets, so a silent stringify would
manufacture bugs, not ergonomics.

### Evidence

- REFERENCE.md §"Within-type operations" — now states the exception
  **at the rule**: "The **sole language-level exception** is `String`
  `add` … no other word, and no other type — `Atom` and `Bytes`
  included — crosses scalar types" (doc fix landed with this verdict,
  closing the 60-lines-apart contradiction the Pending record flagged).
- `lang/go/native/native_math.go` — the `[TString TScalar]` /
  `[TScalar TString]` overloads and the "string-or-bust" comment;
  `native_scalar_ops.go` / `native_bytes.go` — the within-type
  `[Atom Atom]` / `[Bytes Bytes]` signatures.
- `lang/spec/arithmetic.tsv` §3 — the concat battery, including the
  `add true 1` and `add true false` negatives.

---

## NUR009 — Bytes excluded from the DepScalar refinement bases {#nur009}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the comparison words double as refinement constructors over
the well-known ordered scalar bases (`Integer gte 0`, `String lt "z"`,
`Boolean gte true` — one shared resolver, `canonicalBaseType`).
**Divergence:** `canonicalBaseType` admits Integer/Float/Number/String/
Boolean/Atom and omits Bytes — the one ordered scalar leaf (full
byte-lexicographic Comparer, Sizer, complete occurrence package) denied
refinement construction.
**Evidence:** `core/go/depscalar.go:157-175`;
`lang/go/native/native_bytes.go:13,138-163`.
**Documentation status:** not found in REFERENCE/ADR/design — either an
unstated deliberate scoping or an omission; needs a verdict.

**Verdict direction (maintainer, 2026-07-31 — architectural
remediation, `design/NUR-RESOLUTION-PLAN.0.md`):** the Bytes omission
exposes a deeper **ownership** problem in the kernel type hierarchy,
and the fix is architectural rather than a one-line addition to
`canonicalBaseType`. The proposed rule: **all globally visible
descendants of `Node` or `Scalar` belong in `eng`**; core modules
(`boru:*`) may define module-owned descendants; the `lang` layer must
not define additional *global* Node/Scalar descendants except through
an explicit NUR. Likely migrations into eng: `Bytes`, `Time`, `Date`,
`DateTime`, `Instant`; the remaining scalar descendants are to be
reviewed individually. A new ADR describing ownership of the kernel
type hierarchy is required before the migration lands (recorded as ADR
candidate 2 in the resolution plan). This record stays Pending until
that remediation (or a narrower argued verdict) closes it.
**ADR-012 (2026-08-03) retargets the remediation's destination:** the
ownership rule is recorded, but the migrations consolidate in the new
`types/go` component with capability opt-ins (the refinement-base
capability closes this record), not in eng — the kernel stays
content-free.

**Verdict (maintainer, 2026-08-15 — WAIT, no narrow fix):** do **not**
add `Bytes` to `canonicalBaseType` as a one-line patch. This record
closes through the **ADR-012 `types/go` consolidation** and its
refinement-base capability, as the retargeted direction describes.

The reasoning is that a special case added now is a special case the
consolidation would have to unwind: the point of the capability is that
a type DECLARES its refinement-base participation, and hand-listing one
more leaf in the resolver is the mechanism the remediation replaces.
The gap is real and stays visible in the meantime — that is what a
Pending record is for. Stays **Pending** until the consolidation lands.

---

## NUR011 — `eq` is identity for compounds, value for scalars {#nur011}

**Status:** Allowed · **Date:** 2026-07-23

### The uniform rule

One word, one equality principle.

### The divergence

`eq` compares scalars by value but lists/maps/XML/instances by
container identity (`["a"] eq ["a"]` → false); `deq` is deep value
equality throughout. Consequence: `eq` disagrees with `cmp`-equality
on compounds (structurally-equal lists are `cmp`-equal but not `eq`).

### Why allowed

The maintainer's rule (2026-07-23, resolving NUR015 in the same
stroke): **for Scalars, `eq` and `deq` are the same and based on
values; for Nodes and Ideals, `eq` is by reference, `deq` is by
value.**

The Ideal half carries argued carve-outs, settled by the equality work
of the retired NUR031 (`git log -S NUR031` for its reasoning) and
recorded here now that it owns them. Two kinds have no second level to
offer, so their `eq` and `deq` coincide from opposite directions:

- **By VALUE, both** — a *value-like* Ideal with no handle behind it.
  `Error` (two independently raised errors with equal code, message and
  payload are `eq`) and `Word` (a name plus its `/`-modifiers), joined
  by the declared **type values** — a `class` and refinements of one, a
  disjunction/`enum`, a `fnsig`/`surface`, an uninstantiated `gen`
  schema — which are immutable declarations compared nominally.
- **By REFERENCE, both** — an opaque handle whose identity IS its value:
  `Timeout`/`Interval`, the `Module` descriptor (2026-08-02), and a
  sealed host `ExtensionPayload` (an `IO.open` file handle, a lock, a
  watcher, an mmap), which the kernel compares as a box and never reads
  into.

`Store` and `Function` are the two that DO have both levels, and they
take the rule as written: `eq` is reference identity — a Store's
`*StoreInstanceInfo`, a function's identity token — and `deq` is deep
value: a Store's own entry projection, a function's content as canon.

All of these are the rule applied, not departures from it — but the rule
as quoted above does not say so, and this record is where a reader looks
first.

Two equality levels are deliberate — reference identity
answers "is this the same container?" (cheap, aliasing-aware), deep
equality answers "do these hold the same values?" — the Scheme
`eq?`/`equal?` trichotomy collapsed to two levels because scalar
value-identity makes the levels coincide there. Every value-oriented
word keys on `deq` (the collection words since the NUR015 fix); `eq`
remains the aliasing probe.

### Evidence

- `eng/go/compare.go` — `ExactEqual` (scalar arm shared with
  `DeepEqual` via `scalarFamilyEqual`, so eq and deq can never drift
  on a scalar; `sameContainer` identity arms for compounds).
- REFERENCE.md §Comparison ("**`eq` is identity for compounds; `deq`
  is structural — by design**"); EXPLANATION.md §"Type ordering", the "**Two equalities, one rule.**"
  lead-in (added with this verdict); `design/LISP-ANALYSIS.5.md` (the original
  argument).
- `core/go/compare.go` — the carve-out arms themselves
  (`opaqueIdealExactEqual` / `opaqueIdealDeepEqual`, `storeDeepEqual`,
  `errorInfoEqual`, `hostPayloadIdentity`, `sameFnIdentity` /
  `fnStructurallyEqual`), and `core/go/compare_nur031_test.go`, which
  pins each of them.
- `lang/spec/compare-restrict.tsv` — the per-kind rows, including the
  code and type values; `lang/spec/module-array.tsv` — the collection
  words' `deq`-basis battery pins the value side of the rule.

**Modification recorded (maintainer, 2026-07-31,
`design/NUR-RESOLUTION-PLAN.0.md`):** the two-level model is to grow
into a complete equality family with a third word — **`req`**,
reference equality (pointer identity only, uniformly for compounds
and scalars) — separating three notions many languages conflate:
convenience equality (`eq`), deep structural equality (`deq`), and
reference identity (`req`). Performance note: Bytes `deq` may be
O(n); `req` gives a constant-time identity probe. Documentation
should compare the model with JavaScript, Python, Ruby, and the
Lisp family. The `req` design travelled with the equality work of the
retired NUR031 and is now unowned: it is a third WORD, not a
non-uniformity, so no record tracks it. This record's allowance is
unchanged.

---

## NUR013 — Two ordering regimes: a lawful total order and IEEE relationals {#nur013}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; the 2026-07-31 investigation verdict discharged below;
verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

One ordering answer per value pair within one word family.

### The divergence

`cmp`/`tcmp`/`sort` give NaN a defined slot (sorts greatest; two NaNs
tie) while `lt`/`lte`/`gt`/`gte` apply the IEEE unordered rule (always
false); `nan eq nan` is false while `nan cmp nan` is 0. Signed zeros
now add a mirror-image case in the other direction: `-0.0 cmp 0.0` is
-1 while `-0.0 eq 0.0` is true and `-0.0 lt 0.0` is false.

### The `totalOrder` comparison (the 2026-07-31 verdict, discharged)

IEEE-754 §5.10 `totalOrder` requires
`−qNaN < −inf < negative finite < −0 < +0 < positive finite < +inf <
+qNaN`, with NaNs further ordered by sign and payload. boru's order
was compared against it point by point:

- **NaN slotting — conforming, for boru's observable NaN.** boru
  exposes exactly one quiet NaN: there is a single `nan` literal, sign
  is not observable (`nan -1.0 mul` renders `nan`), and no payload is
  reachable. For a single positive qNaN, `totalOrder` demands exactly
  what boru does — greatest, above `+inf`, tying with itself.
- **NaN sign/payload ordering — impractical, and accepted.** Ordering
  negative NaNs below `−inf` and ordering by payload would require
  making NaN sign and payload observable values in the language, which
  nothing else in boru does and no boru program can produce. The
  divergence is therefore vacuous at the language level; per the
  verdict's own terms it is folded into this record's acceptance.
- **Signed zeros — was nonconforming, now FIXED.** `-0.0 tcmp 0.0`
  answered 0; `totalOrder` requires −0 before +0. The total order now
  slots negative zero first (`sort [0.0 -0.0]` → `[-0.0 0.0]`), with
  Integer `0` and BigDecimal `0d0` slotting as +0 so the cross-leaf
  triangle stays transitive.

### Why allowed

The two regimes are deliberate and are the standard resolution of an
unsatisfiable constraint set — the same architecture NUR024 records as
**semantic** vs **deterministic** ordering:

- **The relationals** (`lt`/`lte`/`gt`/`gte`) answer a *mathematical*
  question and therefore obey IEEE-754: NaN comparisons are false
  (§5.11), and ±0 compare equal. A language that silently ordered NaN
  in `lt` would be wrong by the numeric standard its floats implement.
- **The total order** (`cmp`/`tcmp`/`sort`) answers "give me a lawful,
  deterministic arrangement of these values". It must be total and
  antisymmetric or `sort` is not a function; that requires a slot for
  NaN and a decision on ±0, which is precisely what `totalOrder`
  specifies and what boru now implements.

Because the relationals must keep IEEE ±0 equality while the total
order separates the zeros, the relational path carries an explicit
signed-zero carve-out beside the NaN one. That carve-out is part of
this acceptance, not a new divergence: it is the same
semantic-vs-deterministic split applied to the other special value.

### Evidence

- `eng/go/compare_scalar_behaviors.go` — the NaN slot and the
  Signbit tiebreak (float projection and big-rat paths, keeping
  Integer/BigDecimal zeros at +0).
- `eng/go/compare.go` — the relational unordered/signed-zero guards
  that keep `lt`/`lte`/`gt`/`gte` IEEE-conforming.
- `eng/go/compare_nan_test.go`, `eng/go/compare_zero_test.go` — both
  regimes, positive and negative.
- `lang/spec/float-special.tsv` (signed-zero and NaN sections),
  `lang/spec/edge-scalars-2.tsv` (the cmp/sort rows).
- `design/IEEE-754-COMPLIANCE.8.md` §5.10 — the conformance record
  above; `design/TYPE-ORDERING.10.md` §"NaN in the total order".

---

## NUR014 — Cross-leaf numeric magnitude equality depends on the leaf pair AND the value {#nur014}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

Leaves of the same family compare by magnitude: `1 cmp 1.0` → 0,
`1 eq 1.0` → true.

### The divergence

Whether the collapse holds is decided by the leaf pair **and** by the
value, so it is not a family invariant either way.

- **Value-dependent within one pair.** Float↔BigDecimal collapses for
  every binary-exact (dyadic) magnitude — `0d0.5 eq 0.5` → true — and
  fails for every other — `0.1 eq 0d0.1` → false (an exact big.Rat
  compare of the float's true binary value against the exact decimal).
- **Pair-dependent at one value.** The same magnitudes answer
  differently purely because of the leaves: `9007199254740993 eq
  9007199254740992.0` → true (Integer↔Float compares through a float64
  projection) while `0d9007199254740993 eq 9007199254740992.0` → false
  (BigInteger↔Float compares exactly).

### Why allowed

The divergence is **mathematically honest**: the Float written `0.1`
IS NOT one-tenth — it is the nearest binary64 value,
0.1000000000000000055511151231257827…, and the exact big.Rat compare
reports that truthfully. Every collapse that *can* hold exactly does
hold (`1 eq 1.0`, `1 eq 0d1`, `0d0.5 eq 0.5` — dyadic values convert
exactly), so the family invariant fails only where the mathematics
itself fails. The alternative — rounding BigDecimal through float64 to
force the collapse — would silently equate distinct values, defeating
the reason BigDecimal exists; it would also contradict the
exactness-preserving design that already makes mixed Big⊕Float
arithmetic a defined error. The behaviour is Python's
(`Decimal('0.1') == 0.1` → False), for the same reason.

### Evidence

- `eng/go/compare_scalar_behaviors.go` — `numberCompareBehavior.
  Compare` and `toRatExact` (the in-code rationale comments cite the
  Python precedent).
- REFERENCE.md:195-200 — the user-facing statement of the honest
  result, with the exact-value explanation.
- `lang/spec/bignum.tsv:47-63` — pins both directions: the collapses
  that hold (`0d5 eq 5`, `1 cmp 0d1.0` → 0, `0d0.5 eq 0.5`) and the
  one that must not (`0.1 eq 0d0.1` → false).
- `lang/spec/edge-scalars-1.tsv:24-25` — both `cmp` directions of the
  non-collapse.

---

## NUR018 — Store and Error are excluded from `make` {#nur018}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

`make` instantiates the structural type-kinds; the kernel guide groups
Record, Options, Table, Class, Store, Error and the Micron family
together as the `make`/`record`/`class` structural set
(eng/go/CLAUDE.md §"Where a Type Lives" rule 4).

### The divergence

`make Store {}` and `make Error {message:"x"}` raise
`[boru/unsupported]: make: unsupported target type` while
Record/Options/Table/Class/Micron are `make` targets — Store and Error
construct only through their dedicated words.

### Why allowed

`make` targets are the **schema-bearing** structural kinds: a
Record/Options/Table/Class/Micron declares a shape, and `make`
instantiates a value against that shape. Store and Error carry no
user-declared schema and their constructors are semantically loaded in
ways a bare `make` cannot honour: a Store IS its position in the
context machinery (`StoreInstanceInfo` carries the parent-chain and
COW-layer state that `eng/go/registry.go`'s context words establish —
a detached `make Store {}` would have to invent an answer to "whose
child is it?"), and an Error's identity is its passage through
`raise`/`trap` (`describe raise`: "construct an Ideal/Error"), so
error construction always flows through the raising path that stamps
code and context. The kernel-guide grouping this record measured
against is about **kernel residence** (where the types live), not
about `make`-constructibility — clarified at the rule itself with this
verdict. The exclusion is loud (a coded `unsupported` error, not a
dispatch miss), and the dedicated constructors are the documented
route.

### Evidence

- `core/go/core_make.go` — `registerKernelIdeals` (:815) is where the
  omission lives: it registers Ideals for Object, Resource, Record,
  Micron and Table and registers none for Store or Error, so
  `reg.Ideals.For`/`Match` return nil for those two and the target
  falls through to `MakeConvert` (:1066), whose default arm (:1112)
  raises the covered `unsupported target type`. (`isTypeLike` (:31) is
  NOT the gate — it short-circuits on `IsBareTypeNode` and answers true
  for Store and Error exactly as it does for Record.)
- `eng/spec/make.tsv` — negative rows pinning both exclusions
  (`make Store {}` and `make Error {message:'x'}` → ERROR).
- eng/go/CLAUDE.md §"Where a Type Lives" rule 4 — the
  kernel-residence clarification landed with this verdict.
- REFERENCE.md — the `make` documentation states the exclusion and
  names the dedicated constructors.

---

## NUR019 — `slice` is a core sequence word, not a String straggler {#nur019}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22 as "the String family's core straggler"; verdict:
maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

The string vocabulary moved to `boru:string-util`; moved words are not
available unqualified (lang/go/CLAUDE.md §"Package layout").

### The divergence (as recorded)

`slice` alone stayed core — REFERENCE's string table listed it
unqualified between two `StringUtil.*` rows, and `boru describe` files
it under `list`, not `string`, with the reason stated nowhere.

### Why allowed

The move rule does not apply because **`slice` is not a String-family
word**: it is a core *sequence* word, polymorphic over String, List,
and Bytes (nine unqualified signatures spanning all three), kin of
`size`/`take`/`reverse`, which also stayed core for the same reason.
Relocating it to `StringUtil` would force splitting one polymorphic
word — the List and Bytes overloads cannot live in a string namespace
— which is a semantically worse outcome than the filing confusion this
record flagged. What WAS wrong was the filing: REFERENCE's string
table presented `slice` as if it were an unqualified string word, and
the describe categories did not say where to find it. Both filings are
fixed with this verdict; the `list` category placement stands, because
that is the sequence home.

### Evidence

- `lang/go/native/natives.go:372-385` and
  `lang/go/native/native_bytes.go` — the String+List signature pairs
  plus the Bytes overloads: one polymorphic word.
- `boru describe slice` — all nine signatures, unqualified.
- REFERENCE.md:1160 — the string-table row now carries the "core
  *sequence* word, no import — also slices List and Bytes; filed under
  the `list` describe category, see NUR019" parenthetical (fixed with
  this verdict).
- `lang/go/native/help/help_categories.go` — the string category's
  description now points at core `slice` (fixed with this verdict).
- `lang/spec/edge-scalars-3.tsv:45-53`, `corpus-core.tsv:119`,
  `corpus-structures.tsv:14` — both string and list behaviour pinned;
  the two-argument negative-start form is pinned at
  `edge-scalars-3.tsv:47,52`. NUR039's actual divergence — a negative
  start in the THREE-argument form discarding `end` — is pinned by no
  spec row.

---

## NUR020 — `print` stays in core; every other IO word is namespaced {#nur020}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict: maintainer, via `design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

The IO vocabulary lives in `boru:io` (`IO.printstr`, `IO.read`,
`IO.write`, …); moved words are not available unqualified.

### The divergence

`print` alone stays in core, unqualified — one IO word outside the
namespace the rest of its family lives in.

### Why allowed

The argument, now written down rather than asserted: **`print` in core
is what makes the expected "Hello World" learning experience work.**
`print "Hello, World"` must be a complete first program — no `import`,
no namespace, no explanation of the module system before the first
line of output — and that matches the expectation practically every
mainstream language sets (`print`/`println`/`puts`/`console.log`
reachable from the first line). The pedagogical entry point outweighs
family symmetry for exactly one word; everything programmatic
(`printstr`, streams, `read`/`write`, `trace`) correctly demands the
`boru:io` import, so the capability surface of real programs is
unchanged. The boundary is one word wide and this record is its
argument; a second unqualified IO word would need its own NUR.

### Evidence

- `lang/go/native/native_print.go` and `register.go` — `print` is the
  single core IO registration; `io_module.go` — everything else.
- `lang/go/CLAUDE.md` §"Package layout" — "only `print` stays in core";
  ADR-004 §Consequences argues print's *forwardness* (a distinct
  question, deliberately not revisited here).
- Bare `print` works in a one-line program with no import
  (`boru -e 'print "Hello, World"'`) — the experience this record
  protects; HOWTO.md's recipes use it unqualified throughout.

---

## NUR022 — `del` covers a fraction of `set`'s containers {#nur022}

**Status:** Allowed · **Date:** 2026-08-14 (the container gap was
RESOLVED BY FIX 2026-08-02; the surviving slot asymmetry is allowed —
see the verdict at the end) · **Recorded:** 2026-07-22 ·
**Surfaced by:** full-repo uniformity review

**Rule (as restated by the 2026-08-14 verdict):** the storage-column
words cover the same **keys** — a key that `set` can write, `del` can
remove. **Slots are out of scope**: a declared Class field and a List
index are positions, not keys, and the inverse of writing a position is
writing a different value, not removing the position.
**Divergence (as recorded, now FIXED — see below):** `set` dispatched
over Class, Store, FlexXml, WeakFlexXml, FlexMap, WeakFlexMap, Map,
List, FlexList, WeakFlexList (and carried a registered `type_error`
refusal for the immutable Microns); `del` covered Map and FlexMap
only. The List exclusion was documented (pointing at
pop/shift/remove-at); the Store, Class, FlexList/WeakFlexList and
FlexXml/WeakFlexXml absences were not. `boru describe set` listed 19
signatures, `boru describe del` four.
**Documentation status:** documented — `lang/spec/flex.tsv` §12 now
states the per-container contract, and every refusal carries its own
message.

**Note on the rule (2026-08-02 review):** the rule above was
originally phrased "paired reader/writer words cover the same
containers", which mis-describes the pair: `set` and `del` are both
WRITERS. The reader, `get`,
covers a third and wider set again (Module, Class, Store, Error,
Resource, Xml, Node, Micron, None) — so container coverage is not
uniform across the storage column at all. That wider spread is
context for the verdict below, not a separate record: bringing `del`
into line with `set` is the step that was directed.

**Verdict (maintainer, 2026-07-31 — resolve by fix,
`design/NUR-RESOLUTION-PLAN.0.md`):** bring `del` into symmetry with
`set` across the container set. **First investigation step:** confirm
that boru distinguishes an *absent key* from a *present key bound to
`none`* — the deletion semantics hang on that distinction being real
and observable. Separately, a **sentinel-values design programme** is
opened (globally unique singletons, user- and system-defined
sentinels, their interaction with containers, equality, and
option-like APIs) — it needs its own design document because it
potentially touches many language facilities, but **NUR022 must not
wait on it**: the del/set symmetry fix proceeds independently. Stays
Pending until the fix lands.

### Investigation step (2026-08-02): the distinction is real, with one hole

An absent key and a key bound to `none` are distinguishable, so
deletion is not expressible as `set key none` and the word earns its
place:

| probe | `{a:1}` | `{a:1 b:none}` |
| --- | --- | --- |
| `has b/q` | `false` | `true` |
| `size` | `1` | `2` |
| `keys` | `["a"]` | `["a", "b"]` |

and the two containers are not `eq`: `{a:1} eq {a:1 b:none}` → `false`.

`del` and `set … none` therefore produce different containers:
`({a:1 b:2} del b) eq ({a:1 b:2} set b none)` → `false`.

The hole is **`get`**. Reading an absent key and reading a
present-none key both yield something whose `typeof` is `None` and
which answers `eq none` → true, `deq none` → true, `eq None` → true.
They *render* differently (`None` for the miss, `none` for the
binding — type literal vs value), but no comparison operator
separates them. So the distinction is observable through `has` /
`size` / `keys` / `eq`, and invisible through the reader. That is
the shape the **sentinel-values programme** the verdict opened has to
settle (a distinct miss sentinel would close it); it is recorded here
as context, not as a separate divergence.

### Fix (2026-08-02): the container sets are now identical

`del` dispatches over exactly the eleven containers `set` does, with
the same key shapes (String and Atom for keyed containers, Integer
for indexed) — 19 signatures each. Each container either removes the
slot or refuses with its own message:

| container | `del` |
| --- | --- |
| Map | copy-returning — a new map without the key |
| FlexMap, WeakFlexMap | in place, returns the node |
| FlexXml, WeakFlexXml | removes an **attribute** — the slot `set` writes |
| Store | copy-on-write, via a tombstone layer (`CowDel`) |
| Class | refused — a declared field is sealed |
| Micron | refused — immutable, mirroring `set`'s own refusal |
| List, FlexList, WeakFlexList | refused — names pop / shift / `ArrayUtil.remove-at` |

The refusals are **registered signatures**, not sig-absence, for the
reason `set`'s Micron form is: an absent signature raises an opaque
`signature_error`, a present one raises the specific message, and
negative spec rows can pin it.

The Store form needed new kernel machinery. `CowSet` layers a binding
over the old store because that store may be shared with an enclosing
scope; removal cannot work by subtraction, since there is nothing in
the new layer to leave out. So `CowDel` writes a **tombstone** and
`StoreInstanceInfo.Get` stops there — the key reads absent from the
deleting layer down while the layer that owns it is untouched. Own
`Data` beats a tombstone, so a `set` after a `del` re-binds; clones
carry tombstones, or a cloned prototype chain would resurrect every
deleted key.

Gate: `lang/go/native/native_del_symmetry_test.go` asserts the two
words carry the **same** container set and the same key shapes, and
fails in both directions — so a container added to `set` cannot
silently reopen the gap, and a `del`-only container is caught too.
Behaviour: `lang/spec/flex.tsv` §12; kernel:
`eng/go/store_tombstone_test.go`.

### Verdict (maintainer, 2026-08-14): Allowed — slots are not keys

One asymmetry survives on purpose: **`set` can write a declared Class
field and `del` cannot remove it.** Under the rule as originally
worded that was still a divergence; the verdict is that the WORDING
was wrong, not the behaviour.

A class field is a **slot**, not a key. The inverse of writing a value
to a slot is writing a different value, not deleting the slot — an
instance missing a declared field would no longer satisfy its own
type. The same reading is what makes the List refusal correct (`set`
replaces at an index; removal shifts the tail, which is a different
operation), so the line is not a special case for Class: it is the
same line drawn twice. The rule at the top of this record is
therefore restated as "a **key** that `set` can write, `del` can
remove", with slots explicitly out of scope, and the record is
**Allowed**.

What remains true and is deliberately NOT closed here: the `get` hole
recorded in the investigation step above — an absent key and a
present-`none` key are indistinguishable through the reader — belongs
to the **sentinel-values programme**, which the 2026-07-31 verdict
opened as its own design line. That programme decides whether a
distinct miss sentinel exists; it does not reopen this record.

---

## NUR024 — Two orderings by design: semantic (`cmp`) and deterministic (`tcmp`) {#nur024}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict: maintainer, via `design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

One comparison vocabulary, one totality regime.

### The divergence

`cmp`/`lt`/`lte`/`gt`/`gte` raise `[boru/incomparable]` across
families (`cmp true 1` errors) while `eq`/`neq`/`deq` are total
(`1 eq "1"` → false) and `tcmp` is an unrestricted total order — two
totality regimes inside one family, with `cmp` and `tcmp` answering
differently for the same pair.

### Why allowed

The language deliberately carries **two distinct orderings**, and the
divergence is that architecture made visible:

- **Semantic ordering** — `cmp`, `lt`, `lte`, `gt`, `gte`. These
  answer "which is greater, *as values in one domain*?" and therefore
  **reject meaningless comparisons**: `cmp true 1` has no semantic
  answer, and a silent cross-family verdict would hide a real type
  error at exactly the moment it is cheapest to catch.
- **Deterministic ordering** — `tcmp`. This answers "give me *some*
  stable, lawful total order over everything" and exists for
  implementation purposes: deterministic signature ordering,
  deterministic map-key walks, reproducible sorts of heterogeneous
  data. It never rejects, because its job is determinism, not meaning.

Equality (`eq`/`neq`/`deq`) is total in both regimes because "are
these the same value?" has an answer across families (no), while
"which is greater?" does not. The two-ordering separation should also
be stated at the architecture level — recorded as ADR candidate 5 in
the resolution plan (semantic vs deterministic ordering).

### Evidence

- REFERENCE.md §Comparison — both regimes documented with
  the rationale: the ordering words are "**family-restricted**" and
  raise `[boru/incomparable]` across families (:1202-1206), `tcmp` is
  "the **unrestricted** total order" (:1208), and the callout at
  :1214-1216 states "different types are simply *not equal* … Only the
  **ordering** words restrict".
- `eng/go/compare.go` (family restriction raising `incomparable`);
  `eng/go/compare_types.go` (tcmp's Rank-based total order);
  `lang/spec/compare.tsv` and `lang/spec/compare-restrict.tsv` — the
  positive/negative batteries pinning both regimes.

---

## NUR026 — Escape sets diverge between quoted strings and templates {#nur026}

**Status:** Pending (NARROWED — the escape VOCABULARY is resolved by
fix, 2026-08-15; the malformed-input REPORTING difference remains) ·
**Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

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
`design/NUR-RESOLUTION-PLAN.0.md`):** boru shall **not** use the
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

---

## NUR039 — `slice` with a negative start silently ignores its end argument {#nur039}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

### The uniform rule

An argument is honoured or refused, never ignored. Out-of-domain
indices elsewhere in the String family clamp predictably
(`slice 5 6 "abc"` → `''`, `slice 0 5 "-"` → `'-'`).

### The divergence

A NEGATIVE start silently collapses `slice start end s` to the
two-argument "drop N from the end" form, discarding `end` entirely:

```
slice -3 -1 'abcde'   →  ab
slice -3  2 'abcde'   →  ab
slice -3  5 'abcde'   →  ab
slice  1  3 'abcde'   →  bc     (the positive form honours end)
```

Three different `end` values, one answer. The negative-index
convention is documented as "count from the end"; that an `end`
argument is then dropped is not.

### Why allowed

The affected spelling is a negative start, which every caller in this
repository can avoid by clamping — and clamping is what a caller wants
anyway, since a negative index is a bug at the call site more often
than an intent to count from the end. The alternative fixes (honour
`end` for a negative start, or refuse the combination) are both
behavioural changes to a core sequence word, which is a larger edit
than the confusion it removes.

The acceptance rests on callers not reaching the spelling, so the
guard that matters is an *upstream* one:

- `utils/cut.boru`'s `cut-span` (:180) and `cut-point` (:165) reject
  `lo < 1` before any range reaches the slicing helpers
  (`cut-err-rng "fields and characters are numbered from 1"`), so no
  negative start is constructed in the first place.
- `utils/tests/cut_test.boru` pins that rejection and the clamped
  behaviour at both ends.

**Correction (2026-08-02 review).** This record previously claimed the
pin was that `cut-chars-rng` "clamps the start explicitly". It does
not: `cut-chars-rng` (utils/cut.boru:329-335) computes
`def a ((rg get 0) sub 1)` with no start clamp and clamps only the
END (`def b (if (hi gt n) [n] [hi])`); its `if (a gte b)` guard is an
empty-range test that a negative `a` against a positive `b` passes
straight through. Replaying its body with `lo = 0` reproduces this
record's own divergence inside the function that was cited as its pin.
The register inherited the error from the source comment above that
function, which mis-described its own body until this review corrected
it (the comment now runs utils/cut.boru:322-328 and says the opposite).
The acceptance survives — the real guard is the upstream `lo < 1`
rejection above, so `cut` is correct today — but the local fragility
is now recorded rather than mis-pinned.

**Correction (2026-08-02 review).** The motivating example previously
given here — `slice (ep add 1) (size tok) tok` where `ep` is `-1` from
a failed `indexof` — does not exhibit this divergence: `(ep add 1)` is
`0`, a NON-NEGATIVE start, and `end` is honoured normally. It illustrates
an off-by-one, not the negative-start collapse. The spelling that does
trigger it is the same call without the `add 1`.

### Evidence

- The four `slice` calls above, verified on the current binary.
- `utils/cut.boru:165,180` (the upstream `lo < 1` rejection) and
  `utils/tests/cut_test.boru`.
- NUR019 records the separate question of where `slice` belongs, and
  its 2026-08-02 verdict is that `slice` is a core **sequence** word,
  not a String-family straggler; this record is an independent defect
  in the same word and takes no position on filing.

---

## NUR040 — `set` quotes a bare computed key where `get` refuses it {#nur040}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

### The uniform rule

Sibling accessors treat their key argument the same way, and a program
that means a variable's VALUE does not silently get its NAME.
lang/go/CLAUDE.md states the split it intends: `dot`/`dotr` quote a
bare word as a literal field name, `get`/`getr` evaluate it.

### The divergence

`set` carries the quoting `Atom/q` slot that `get` does not, so the
same bare-word spelling means opposite things:

```
def k "aa"   {} set k 1     →  {k:1}      # the NAME was stored
def k "aa"   {} set (k) 1   →  {aa:1}     # the VALUE
def k "aa"   {aa:1} get k   →  1          # get EVALUATES k
```

`boru check` reports no ERROR for the first line. It does emit
`[warning] unused_def: def k is never used` — which is the tell, since
that warning appears for neither alternative spelling — but nothing
names the actual hazard. The failure mode in real code is a map built
entirely under one literal key: every iteration of a loop overwrites
`{k:…}`, and only an unused-binding warning hints at it.

### Why allowed

The asymmetry leaks from a distinction that is deliberate and
load-bearing elsewhere — `dot`/`dotr` quote a bare key, `get`/`getr`
evaluate one (lang/go/CLAUDE.md, "dot / dotr vs get / getr"). Making
`set` match `get` is a behavioural change to a core word, which is a
larger and riskier edit than the confusion it removes. The quoting slot
has a real purpose (`set name value store` reads well).

The standing improvement, not required by this allowance: a check-mode
advisory when a bare word passed to a quoting slot is ALSO a live
binding — the one case where the two readings differ and the author
almost certainly meant the value. The `unused_def` warning above is an
accidental partial signal of exactly that condition.

### Evidence

- The three calls above, and the three `boru check` runs behind the
  warning claim.
- **The two files this record was scouted from never pass a bare word
  to `set`'s key slot**, so neither depends on which way the ambiguity
  resolves: `utils/` spells every LITERAL key `(quote k)` (117 sites, 0
  exceptions), and `lang/go/modules/cli.boru` uses `(quote …)` at its
  75 literal-key sites (42 distinct names) and the parenthesised value
  form (`set (nm) …`) at its 8 computed ones. Its house rule at
  cli.boru:53-54 states the convention: "a computed map key is always
  parenthesised (`m set (k) v`) — a bare `k` stores the literal name
  \"k\", with no diagnostic at all."
- **Elsewhere the repo does rely on the quoting reading**, which is the
  real reason the fix is riskier than the confusion:
  `lang/go/modules/vault_tui.boru` — shipped, `//go:embed`-ed — has 77 bare-word key
  sites (`grep -oE '\bset +[a-z][a-zA-Z0-9_-]*'`; 75 excluding the two
  that follow a `-`-suffixed word) (`state set screens …`, `state set status …`),
  and `kg/report.boru:334`, `design/examples/apps/todo-tui.boru:52` and
  the linguist samples do the same. Making `set` evaluate its key would
  change all of them.
- lang/go/CLAUDE.md:303-316 — the "**`dot` / `dotr` vs `get` / `getr`
  (CRITICAL)**" bullet inside §"Parser Customization" (a bolded
  lead-in, not a section) — the deliberate split this record's
  divergence leaks from.

> **Family note (2026-08-21).** `has` — historically on the quoting side
> with `set` — was moved to the evaluating side by maintainer direction:
> its key now evaluates exactly as `get`'s (QuoteArgs stripped from every
> `has` sig, core and the `boru:net` extension alike; pinned in
> `lang/spec/corpus-core.tsv` incl. the `ERROR:undefined_word` negative).
> The divergence this record accepts now covers `set` alone.

---

## NUR046 — `boru fmt` is not idempotent: one pass is not a fixed point {#nur046}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** the C3 utils
suite (`utils/`)

### The uniform rule

A formatter is idempotent. `fmt(fmt(x)) == fmt(x)`, so "formatted"
is a property a file either has or does not, a `make fmt` target converges,
and a formatting check can be a single-pass diff. `make fmt-docs` and
`kg/Makefile`'s restored `fmt` target both rely on this.

### The divergence

On a `def name fn [[params] [Returns] [body]]` whose header
does not fit the width, the FIRST pass and the SECOND pass produce different
layouts. It converges at pass 2 — passes 2..n are identical — so the fixed
point exists; one application simply does not reach it.

```boru
# m.boru, as hand-written:
def cat-format fn [[line:String k:Integer numbered:Boolean ends:Boolean] [String] [
  def body (if ends [(join "" [line "$"])] [line])
  join "" [body "\n"]
]]
```

```
$ boru fmt m.boru && cat m.boru          # pass 1
def cat-format fn
  [[line:String k:Integer numbered:Boolean ends:Boolean] [String] [
  def body (if ends [(join "" [line "$"])] [line]) join "" [body "\n"]
]]

$ boru fmt m.boru && cat m.boru          # pass 2 — different, and stable
def cat-format fn
[[line:String k:Integer numbered:Boolean ends:Boolean] [String]
      [def body (if ends [(join "" [line "$"])] [line]) join ""
          [body "\n"]
      ]
  ]
```

The blast radius is in §Evidence below; program output is unchanged in
every affected file, and every one still passes `boru check`.

**Why it matters:** three ways.

1. A `fmt` target inside an `all:` target never converges in one run, so
   `make all` always leaves a dirty tree — which is why `utils/Makefile`
   deliberately keeps `fmt` OUT of `all` and says so, the same posture
   `kg/Makefile` held while NUR028 was open.
2. Pass 1 joins two statements onto one line (`… [line]) join "" [body …`)
   and pass 2 re-indents a statement as though it continued the previous
   one. Both are legal — boru is whitespace-insensitive — but a reader
   cannot tell statement boundaries by eye any more, which is most of what
   a formatter is for.
3. It is a fixed-point bug in the same component as the resolved
   superlinear blow-up, in a shape that blow-up's gate would not have
   caught: that gate compared old-binary and new-binary output on the
   *repo's already-canonical* corpus, where pass 1 is already the fixed
   point. Non-canonical input is the untested axis.

### Documentation status

`kg/Makefile:25-28` claims idempotence in so
many words — "the formatter is idempotent, so once they are canonical
this is a no-op on the tree and `make all` leaves nothing to commit" —
with `fmt` inside its `all` target (kg/Makefile:11). kg's own sources
happen to sit at their fixed point, so no dirty tree results today, but
the written claim is false in general. `kg/README.md` and `make
fmt-docs` likewise treat a single `fmt` run as producing canonical
form.

**The mechanism (corrected 2026-08-02).** This record originally
proposed that "the first pass measures widths against a pre-wrap layout
decision it then invalidates". `design/NUR-EFFORT-TRIAGE.0.md:139-148`
(the NUR046 bullet; the cause statement at :140-141) investigated and
found otherwise: the true cause is **re-parse
statement-segmentation drift** (root-level newlines emitted by pass 1
change how pass 2 segments statements). The width-memoisation framing
is retired.

**The standing fix, when scheduled:** a regression guard belongs with
it — format every `.boru` in the repo TWICE and require the second pass
to be a no-op, with at least one deliberately non-canonical fixture,
since the already-canonical corpus cannot detect this.

### Why allowed

Formatting does not change behaviour — all 995 cases in `utils/` pass either
way, verified — so what the non-idempotence costs is a clean tree and
readable sources, not correctness. It converges at the second pass, so a `fmt` target
that ran twice would be stable; the reason not to paper over it that way is
that the intermediate layout runs statements together on one line, which is
most of what a formatter is for.

### Evidence

- The repro above, and the **repo-wide sweep** (re-run 2026-08-02 on
  the current binary): of the 122 tracked `.boru` files, **19 are
  non-idempotent** — all 12 `utils/*.boru` programs, three SHIPPED
  library modules (`lang/go/modules/cli.boru`, `sift.boru`,
  `vault_tui.boru`), two `design/examples` programs and two
  `editors/linguist/samples`. All 11 `utils/tests/*_test.boru` suites
  ARE at their fixed point after one pass, which is the qualitative
  split that makes the divergence easy to miss.
- `utils/Makefile` keeps `fmt` OUT of its `all` target and its comment
  names this record and explains why — the same posture `kg/Makefile`
  held while its own formatter blocker was open — so the tree cannot
  silently start churning on every build.
- `kg/Makefile:11,25-28` — the idempotence claim named above, the one
  place the property is asserted rather than assumed.

**Correction (2026-08-02 review).** This record previously said the
non-idempotence hits "all six programs" in `utils/` with "the five
`tests/*.boru` suites" already at their fixed point. Those counts were
accurate on 2026-07-30 when the record was written (the tree then held
six programs and five suites) and have since drifted: it is 12 and 11,
and the blast radius reaches shipped `lang/go/modules/*.boru`, a scope
the record never mentioned. An **Allowed** record carries "the evidence
that pins it … so the acceptance cannot silently rot"; this evidence
had rotted by a factor of two.

---

## NUR072 — Three sugar kinds still canon in debug form, and their source spelling is not recoverable {#nur072}

**Status:** Pending · **Recorded:** 2026-08-15 · **Surfaced by:** NUR059's
fix — the residue its per-row fixpoint check refused

**Rule:** `CanonValue` renders canonical boru **source** — the string it
produces parses back as the same value (ADR-015).

**Divergence:** NUR059 gave the angle sugar, the paren group and the word
`/`-modifiers a source form. Three sugar kinds were attempted in the same
change and **withdrawn**, because a fixpoint check — does the new render
re-parse to itself? — proved the spellings wrong:

```
+m'src'   rendered  +m<src>    re-parses with a stray `>`
w/t       rendered  [w/q]/t    does not parse at all
```

The mini case is not a coding slip: **`SugarInfo` does not retain the
source delimiter.** `+m'src'`, `+m<src>` and `+m|src|` all lex to the same
payload, so no renderer can reproduce what the user wrote without the
parser keeping it. The type-bound case renders its `Items`, which hold the
bound as a list containing an atom rather than the bound's own text.

The lambda marker is a third shape: `=>` renders fine on its own, but every
row containing one ALSO contains a bare word, which canon spells
`word(x)` — so the row cannot reach a fixpoint whatever the lambda does.

**Verdict:** none yet. A spelling that does not round-trip is worse than
the debug form, because it looks like source; that is why these were
withdrawn rather than shipped. Fixing them needs either a parser change
(retain the mini delimiter) or the bare-word decision below.

**A THIRD kind, found by Codex review on PR #372:** the `/N` arity is an
exact int64 in Go and a JS `number` in TS, so a magnitude above 2^53 is
ROUNDED at parse — `x/9223372036854775807` canons as
`x/9223372036854776000` in TS, outside the accepted int64 range, so it
cannot be re-parsed as a modifier at all. The loss is in the TS
**payload**, not the renderer, which only made it visible; fixing it
means carrying the arity exactly in `parser/ts`. Pinned meanwhile as a
row in `parser/spec/divergent.tsv`, the shrink-toward-zero ledger.

**The bare-word question, deliberately not decided:** canon spells a Word
value `word(foo)`. Rendering it bare would change **175 of
`parser/spec/parse.tsv`'s 724 rows**, and it is a real question rather
than a formatting one — `word(foo)` denotes a word VALUE, while bare
`foo` re-parses as a word that will be DISPATCHED. NUR059 scoped its word
arm to modifier-bearing words for exactly this reason. Whoever takes this
record decides that first.

## NUR075 — `deq` is extensible per type, `eq` is not {#nur075}

**Status:** Pending · **Recorded:** 2026-08-16 · **Surfaced by:** the
retired NUR031's fix — the one part of its verdict the fix did not take

**Rule:** a per-type operation is reached through the type's capability
seam, so a type can answer for its own values. `Truther`, `Sizer`,
`Comparer`, `DeepEqualer` and `Formatter` all work this way, exposed to
boru code through `behave`.

**Divergence:** `deq` has `DeepEqualer` — a type installs one and
`DeepEqual` consults it (`core/go/deepequal_capability.go`, reached at the
bottom of the walk so it can only turn the terminal `false` into a real
answer). `eq` has no counterpart. `ExactEqual` is entirely hardcoded
arms, so a host or boru type can define what "same value" means for its
values but not what "same thing" means, and the two halves of one word
family are extensible on different terms.

**How it got here.** NUR031's verdict proposed routing `eq`/`deq` through
the type's `Behavior` for all Ideals, replacing the kernel's hardcoded
arms wholesale. Its fix did not do that: it closed every divergence the
record measured — reflexivity for every kind, identity that survives
rebinding, a name-independent function canon — by adding arms rather than
removing them, and the proposed mechanism turned out to be a refactor of
the most-exercised path in the kernel with no observable change to show
for it. What the mechanism WOULD have bought, and the arms do not, is
this one asymmetry. It is recorded here so the deviation from that
verdict is visible rather than lost with the retired record.

**Cost of the divergence:** narrow today. A type wanting reference
identity for its values gets it already — every kernel-known payload
kind has an arm, and a sealed host payload gets box identity by default
— so nothing is currently *unable* to answer `eq`. The asymmetry bites
when a type wants an `eq` that is neither box identity nor the kernel's
guess: an interned value that should be `eq` across two constructions,
or a handle whose identity lives one level below its payload box.

**Proposed verdict:** none yet. The candidates are an `ExactEqualer`
capability mirroring `DeepEqualer` (small, symmetric, and paid for only
by types that install it); the full `Behavior` routing NUR031 proposed
(uniform, but a large refactor of a hot path); or Allowed, on the
argument that reference identity is a KERNEL property — what "the same
thing" means is the runtime's answer, not a type's — in which case
`DeepEqualer` is the sound asymmetry and the register should say so.
The third reading is the strongest and is why this is not filed as a
fix-by-default.

**Evidence:** `core/go/deepequal_capability.go` (the `deq` capability and
its LCA walk); `core/go/compare.go` — `ExactEqual`'s arms, which no
capability can reach, against `DeepEqual`'s terminal
`deepEqualCapability` call; `lang/go/native/native_behave.go` (the
`behave` slots, `DeepEqualer` among them and no `eq` twin);
`core/go/capability_gaps_test.go`.

## NUR076 — A `behave`-installed capability is invisible to check mode {#nur076}

**Status:** Pending · **Recorded:** 2026-08-17 · **Surfaced by:** NUR056's
fix (the `Maker` capability); flagged for this register by the PR #379
review (Codex P1)

**Rule:** the checker and the interpreter agree. A program the interpreter
runs is a program `boru check` accepts — the same twins discipline the
compiled differential enforces for the VM.

**Divergence:** `behave` does not take effect in check mode. The type is
minted (a check pass sees `C` exist), but the capability wrapper is never
installed, so every `behave`-installed slot is absent from the checker's
view of the program.

For seven of the eight slots that is invisible: `compare`, `canon`, `nodify`,
`unify`, `truthy`, `deq` and `size` change what a program COMPUTES, and
check mode does not evaluate. `make` is different, because the checker
actively VALIDATES a construction against the target's declared schema —
so a type that supplies its own constructor is judged against rules its
constructor never runs:

```
def P class {a: Integer}
behave make/q (fn Any P [make P {a: 42}])
make P {bogus: 1}

  boru run -no-check   ->  Class/P{a:42}     the constructor ignores its source
  boru check           ->  2 errors          unknown field "bogus", missing "a"
```

The check failure gates the run, so the program does not execute at all
under the default pre-flight — a working program rejected outright.

**What the NUR056 fix could and could not do.** `makeObjReturns` now asks
`core.HasMaker` before applying `CheckMakeConstruction`, so a Maker the
checker CAN see suppresses the schema validation. That covers a Go-side
Maker, which is installed at registry construction and therefore present
during check. It cannot cover a `behave`-installed one, because the wrapper
arrives only by executing the `behave` call — which check mode does not do.
The guard is correct and the residue is exactly the ordering it cannot
close.

**Why it was not fixed here:** running `behave` in check mode means
executing arbitrary user bodies during static analysis. That is a language
decision about what `boru check` is allowed to do, several sizes larger
than the capability this record's parent added, and it would apply to all
eight slots at once rather than to `make`.

**Evidence:** `lang/go/native/native_behave.go` (the wrapper install, run
only by `behaveHandler` execution); `lang/go/native/native_make.go`
`makeObjReturns` (the `HasMaker` guard and what it cannot see);
`core/go/maker_capability.go` `HasMaker`; the transcript above, measured
2026-08-17.

**Documentation status:** `boru describe behave` states that `make` runs
ahead of every kernel construction path, which is true at RUNTIME and is
where the asymmetry shows. Nothing user-facing says a `behave` slot is
invisible to `boru check`.

**Proposed verdict:** none yet. Three candidates. (1) Run `behave` in check
mode — closes it for all eight slots, at the cost of executing user code
during analysis. (2) Have the checker recognise a `behave make` call
SYNTACTICALLY and suppress the schema validation for that target — narrow
and cheap, but a second mechanism that knows about one word. (3) Allowed,
on the argument that check mode is a static approximation and a custom
constructor is exactly the kind of dynamism it may decline to model — in
which case `make` should not be schema-validating a type it cannot prove
owns its own construction, which is closer to (2) than to accepting the
current answer.

---

## NUR060 — The parser twins disagree on open-input sources beyond the corpus {#nur060}

**Status:** Pending · **Recorded:** 2026-08-09 · **Surfaced by:** PR #337
parity-probe sweep; flagged for this register by the PR #337 review
(Codex P1)

**Rule:** one language contract, two implementations: `parser/go` and
`parser/ts` must render every source identically — the uniformity the
`parser/spec` corpus exists to enforce.
**Divergence:** a 2,587-source probe sweep measured 55 sources (~2.1%)
where the twins disagree, and follow-up probing added a class the sweep's
seed missed — nine classes so far: trailing-`=>` fold loss (TS drops the
paren group Go folds — also inside dotchains), two accept/reject splits
(trailing bare `:` — Go accepts, TS refuses; `=> ,` — Go refuses, TS
accepts and silently drops tokens), post-`]` recovery-token detail (TS
reports an empty token where Go names the offender), two error-precedence
splits (receiverless-`.` vs unmatched-`(`, and bare-`/s` vs
unmatched-`(`), an internal type-name leak in one message on both sides,
and an empty-`${}` fold split in an unterminated template (Go folds to
`interp('')`, TS keeps the hole).
**Evidence:** `parser/spec/divergent.tsv` — the live ledger, one measured
row per class; both spec runners re-render every row against their own
column on every run, so a row can neither rot nor survive its fix.
`scripts/parity-probe.sh` reproduces the sweep.
**Documentation status:** parser/spec/README.md §"The current debt" and
design/GO-TS-PARITY.0.md carry the honest scope: corpus parity exact,
open-input parity not.
**Proposed verdict:** resolve by fix, class by class — each fix moves its
ledger row to `parse.tsv` (the runners force the move: a fixed divergence
fails the ledger loudly). The behavioral classes (fold loss, the two
accept/reject splits) should go first; the diagnostic-detail classes
follow. The record discharges when the ledger is empty again.

> **Update (2026-08-10).** Two DATA-seam asymmetries this record also
> carried — the TS seam's inability to express map insertion order for
> integer-like keys, and Go alone reading `+_1` as a number — were
> dependency defects, not boru's. Both are fixed upstream in
> `tabnas/jsonic v0.6.0` / `tabnas/parser v0.8.0` (ADR-014) and are now
> pinned by ten `data.tsv` rows rather than described in prose. The nine
> GRAMMAR-level classes above are untouched by the upgrade: a fresh
> 2,587-source sweep on the new dependency measures the byte-identical 55
> divergences, which is what distinguishes the two categories. This record
> stays Pending on those nine.

---

## NUR062 — Numeric marker letters are lowercase-only while every other letter in a literal is case-flexible {#nur062}

**Status:** Allowed · **Date:** 2026-08-14 · **Recorded:** 2026-08-11 ·
**Surfaced by:** the maintainer's decision on PR #339 ("only lowercase should
be valid for numeric syntax prefixes"); flagged for this register by the
PR #339 review (Codex P1)

**Rule:** one lexical convention per kind of thing. Letters inside a numeric
literal are either case-significant or they are not.

**Divergence:** they are now both. The four MARKER letters are lowercase-only
— `0x`, `0o`, `0b` and the big-number `0d`, so `0XFF` raises
`[boru/syntax_error]: numeric prefix must be lowercase: 0XFF` — while every
other letter a numeric literal can contain stays case-flexible:

```
0xff  ==  0xFF        hex DIGITS take either case
1e3   ==  1E3         the exponent marker takes either case
0XFF  ->  syntax_error    but the base marker does not
```

So `0XFF` is refused and `0xFF` accepted, yet `1E3` and `1e3` are equally
valid, and `0xAB` and `0xab` are the same value. A reader cannot derive one
from the other; each has to be learned.

**Scope, measured.** The rule governs numeric LITERALS only. A run in a NAME
position is not a literal and behaves identically in both cases — a quoted
atom (`0XFF/q`), a `/r` word reference (`0XFF/r` -> `word(0XFF)`), a
type-bound (`0XFF/t`), and a bare map key (`{0XFF: 1}`) are all names, never
numbers, exactly as their lowercase spellings are. The `0d` family is not a
DATA numeric in either case, so `0d12` and `0D12` both decode as lenient text
through `StructUtil.parse`. Both boundaries are pinned by rows rather than
left to prose: `parser/spec/parse.tsv` §"the lowercase-only rule governs
numeric LITERALS" and `parser/spec/data.tsv` §"the 0d big-number prefix is not
a DATA numeric".

**Evidence:** `parser/spec/parse.tsv` (8 refusal rows + 5 name-position rows),
`parser/spec/data.tsv` (4 refusal rows + 3 `0d` rows), `parser/spec/lex.tsv`
(2 token rows), each re-rendered independently by both port runners.
`REFERENCE.md` §"Numeric literals" states the rule and its scope.

**Documentation status:** stated in REFERENCE.md; both editor grammars
(tree-sitter, pygments) reject uppercase markers so highlighting cannot
advertise a literal the language refuses.

**Verdict (maintainer, 2026-08-14): Allowed** — as proposed. The asymmetry
is deliberate: a marker is a *spelling of syntax* while digits and exponents
are *content*. `0XFF` is a typo for `0xFF` far more often than it is anything
a user meant, whereas `0xAB` vs `0xab` and `1E3` vs `1e3` carry no such
signal, so refusing the first while accepting the others is a diagnostic, not
an inconsistency. The rule is already stated in REFERENCE.md §"Numeric
literals" with its scope, pinned by refusal and name-position rows in
`parser/spec/` that both port runners re-render independently, and both
editor grammars reject uppercase markers so highlighting cannot advertise a
literal the language refuses. No code or documentation change follows from
this verdict — the record closes as it stands.

---

## NUR063 — Seven self-knowledge words are proposed to dispatch from two module surfaces (`boru:debug` and `boru:scry`) {#nur063}

**Status:** Pending · **Recorded:** 2026-08-12 · **Surfaced by:**
design/BORU-SCRY.0.md §6 (the boru:scry proposal); flagged for this
register by the PR #344 review (Codex P1)

**Rule:** one capability, one home: a word lives in exactly one module
surface and `boru describe` names it there — the module taxonomy
`lang/go/CLAUDE.md` documents, and the same single-source value behind
ADR-001's no-shadowing rule.

**Divergence:** `boru:debug` shipped with seven data-returning
self-knowledge words (`words`, `defs`, `modules`, `sig`, `body`,
`deps`, `shape`). The accepted-in-discussion direction (2026-08-12)
splits introspection into `boru:scry`, which adopts those seven — so if
the proposal ships as designed, the same seven capabilities dispatch
from two module surfaces backed by shared Go handlers. No code exists
yet; the divergence begins the day `BuildScryModule` registers them.

**Evidence:** design/BORU-SCRY.0.md §2 (the overlap inventory) and §6
(the containment plan: shared handlers so behaviour cannot fork, scry
canonical, the debug copies frozen at today's seven, `boru describe`
marking the debug variants' canonical home).

**Documentation status:** the plan is stated in design/BORU-SCRY.0.md
§6 and its open question §9 Q1; nothing user-facing exists to update
yet.

**Proposed verdict:** none yet — deliberately. Keeping both surfaces
indefinitely is an **Allowed** argument the maintainer may make;
deprecating and later removing the debug copies is a resolve-by-fix
path. design/BORU-SCRY.0.md §9 Q1 puts that choice to the maintainer
with a lean (keep through one release, then decide with usage
evidence). Recorded now so the dual surface cannot ship as an
unexamined default.

**Verdict (maintainer, 2026-08-15):** **`boru:scry` is canonical.** The
`boru:debug` copies stay, frozen at today's seven and backed by the
SAME handlers so behaviour cannot fork, with `boru describe` naming
scry as the canonical home — and they are **deprecated on a stated
timeline** rather than kept indefinitely.

That is the one-capability-one-home rule applied with a migration path
instead of a break: keeping both forever would need an argument for why
this capability is exempt, and none was offered beyond convenience.
Recorded before `BuildScryModule` exists, so the dual surface never
ships as an unexamined default — which is what this record was opened
to prevent. Stays **Pending** until scry ships with the deprecation
notice in place.

---

## NUR064 — Pattern clauses route-and-bind in `receive` but route-only in `add` {#nur064}

**Status:** Pending · **Recorded:** 2026-08-12 · **Surfaced by:**
`design/STATE-MACHINES.0.md` §8 (which names the asymmetry while declining to
solve it there); flagged for this register by the PR #345 review (Codex P1).
The split itself was designed deliberately in `PROCESSES.0.md` §3 and
`SERVICES.0.md` §1 but never recorded here.

**Rule:** one pattern-clause semantics per matcher. The service `add` and the
process `receive` route through the same patrun matcher, and "match a message"
is supposed to mean one thing everywhere (`SERVICES.0.md`: "the *same* matcher
`receive` uses … 'Match a message' means one thing everywhere").

**Divergence:** the two consumers give the same clause surface different
powers. A `receive` clause pattern is two layers — scalar-tag *routing* via
patrun plus `name:Type` *binding* slots destructured by the fn-param machinery
(`PROCESSES.0.md` §3 "clause matching: routing vs. binding") — while an `add`
pattern is routing only: "An `add` pattern routes; it does not bind"
(`SERVICES.0.md` §1). So `{op:"create" text:String}` binds `text` into the
body as a `receive` clause but silently binds nothing as an `add` pattern,
where the handler must destructure `req.text` by hand. A reader cannot derive
one behaviour from the other; each has to be learned.

**Evidence:** `PROCESSES.0.md` §3 (the two-layer clause spec, including the
explicit callout "The binding layer is specific to `receive`"); `SERVICES.0.md`
§1 (the route-only rule for `add`); `design/STATE-MACHINES.0.md` §8 item 5
(the asymmetry surfacing as a cost for any facility built over both).

**Documentation status:** both design docs state their own side explicitly;
no user-facing doc contrasts them.

**Proposed verdict:** none yet — genuinely open. The candidate resolutions
pull opposite ways: generalize binding slots into a facility `add` (and other
patrun consumers) can opt into, or declare the split Allowed on the argument
that `add` patterns are routing *tables* (inspectable, whole-request handlers)
while `receive` clauses are *destructuring* sites. Deciding belongs to the
processes/services design line; this record exists so the divergence is not
silently baselined meanwhile.

**Verdict (maintainer, 2026-08-15 — defer, deliberately):** decided in
the **processes/services design line**, when those modules are actually
built, not here and not now. Both candidate resolutions (generalise
binding slots into a facility `add` can opt into, or declare the split
Allowed on the routing-table-vs-destructuring-site argument) depend on
implementation experience this record does not have.

The record keeps doing its job meanwhile: the divergence is written
down, so building either module against the other's assumption is a
choice rather than an accident. Stays **Pending** by design.

## NUR065 — Two spellings of the classifier role get different static guarantees {#nur065}

**Status:** Pending · **Recorded:** 2026-08-14 · **Surfaced by:**
`design/STATE-MACHINES.0.md` §3.6 (which introduces both spellings and states
the asymmetry as a preference rather than resolving it); flagged for this
register by the PR #352 review (Codex P1).

**Rule:** one role, one set of guarantees. A spec key's static checking should
follow from *what it means*, not from which of two spellings the author
happened to pick — the same uniformity principle behind one parser, one
argument-positioning convention, one total order.

**Divergence:** `boru:state`'s classification edge has two spellings of a
single role — "turn a raw input into an alphabet member" — and they are
checked to different standards:

- **Alphabet closure.** Every `classes:` key must be a declared `events:`
  member, checked at define time (`state_unknown_name`, §6.3). A `classify:`
  fn's output domain is not statically knowable, so an event atom it invents
  that `events:` never declared is only a step-time `state_bad_event`. The
  §3.3.11 closure guarantee therefore holds up to the machine's edge for one
  spelling and through it for the other.
- **Payload shape.** `classes:` produces a frozen `{event: <class> raw: <v>}`
  (§3.6.2), so a reducer always reaches the input as `ev.raw`. A `classify:`
  fn returns the whole event map and owns its payload shape, so nothing about
  a classified event's contents is derivable from the spec.
- **Diagnostics.** `state_bad_class` (Error) and `state_class_gap` (Info)
  exist only for the table form. The fn form has no analogue of either, so a
  classifier with an unreachable branch or an uncovered input domain is
  invisible to `boru check` and to `State.lint`.

**Evidence:** `design/STATE-MACHINES.0.md` §3.6.1 (the fn form's stated cost),
§3.6.2 (the four frozen table-form properties and the payload freeze), §6.3
(the `state_*` table, where the two class diagnostics have no fn-form rows),
and open question #7 (whether the fn form should ship in v1 at all).

**Documentation status:** the design document states the asymmetry openly and
names `classes:` the preferred form; no user-facing doc exists yet, since the
module is unimplemented.

**Proposed verdict:** none yet — it is open question #7, and the candidate
resolutions are the obvious three. Drop the fn form, making the role uniform
by having one spelling (cleanest, but leaves inputs no partition describes
with no answer). Keep both and declare the split Allowed on the argument that
a *declarative partition* and an *arbitrary function* are honestly different
things whose guarantees cannot be equal — in which case `State.lint` should
report fn-form classification so the weaker guarantee is visible at the use
site, not just in this document. Or narrow the gap by requiring a fn-form
classifier to declare its output alphabet in the spec, recovering closure
while leaving the mapping opaque. This record exists so the divergence is not
silently baselined while that question is decided.

**Verdict (maintainer, 2026-08-15 — defer, deliberately):** decided in
the **state-machine design line** as its open question #7, when
`boru:state` is built. The three candidates (drop the fn form, keep
both with `State.lint` reporting the weaker guarantee, or require a
fn-form classifier to declare its output alphabet) all turn on how the
module actually gets used, and the document is still in flux.

Stays **Pending** by design, so the asymmetry cannot be silently
baselined while that question is open.

---

## NUR070 — `if` reads a List condition as CODE while every other truthiness consumer coerces it {#nur070}

**Status:** Allowed · **Date:** 2026-08-15 · **Recorded:** 2026-08-14 ·
**Surfaced by:** implementing NUR053's fix (measuring whether the three
consumers really share a domain once `convert Boolean`'s slot was
widened)

**Verdict (maintainer, 2026-08-15): Allowed — a List in a condition
position is CODE.** That is what a concatenative language should mean by
a bracketed body there, and the One Truthiness Model governs *values*,
not code positions: `if [ … ]` running its condition is the language's
way of spelling a computed condition, not an accident to be coerced
away. The two readings genuinely differ, and the code reading is the
intended one.

What the allowance costs, stated so it cannot rot: `design/TRUTHINESS.0.md`
§2 must say the model has one shape-shaped hole — a List reaching `if`
is executed, so the "every consumer agrees" claim holds for Map, None,
String and the numeric leaves and NOT for List — and `if xs` where `xs`
holds a list stays a sharp edge for anyone who expected presence
coercion. The §2 amendment landed with NUR053's fix and already states
both the domain and this split, with the measured opposite-answer table.
The spec rows in `lang/spec/edge-scalars-3.tsv` pin it in both
directions, so the accepted behaviour is executable rather than merely
described.

**Rule:** one truthiness model, applied by every construct that coerces
a value to a Boolean — `design/TRUTHINESS.0.md`, the One Truthiness
Model. NUR053's premise, and the sentence this record corrects, was that
"`if` and `make Boolean` accept any value".

**Divergence:** they do not agree on a **List**. `make Boolean` and
`convert Boolean` coerce a list by PRESENCE (non-empty → true); `if`
does not coerce it at all — it runs it as a **code body** and takes the
truthiness of what the body leaves. The two readings give opposite
answers on the same bound value, and the disagreement is not confined to
an edge case:

```
def xs [0]   if xs ['T'] ['F']        # → 'F'     the body runs, yields 0, falsy
def xs [0]   convert Boolean xs       # → true    non-empty list is present
def xs [0]   make Boolean xs          # → true

def xs []    if xs ['T'] ['F']        # → [boru/runtime_error]: if: condition
                                      #   produced no value
def xs []    convert Boolean xs       # → false
def xs []    make Boolean xs          # → false
```

Every other source shape agrees. Measured across the three consumers:
Map (`{}` → false, `{a:1}` → true), `none` → false, empty String →
false, and the numeric leaves including the Big ones (NUR055's rows)
all give the same answer through `if`, `convert Boolean` and
`make Boolean`. The List is the sole shape where the consumers split.

**Why it is not simply a defect in `if`:** the code-body condition is a
deliberate, documented form — `if [ … ] [then] [else]` runs its
condition, which is how a computed condition is spelled, and the
compiled path models it (the "if code-body condition" row in
`lang/go/context_boundary_differential_test.go`). The non-uniformity is
not that the form exists; it is that **the same value gets two different
readings depending on how it reaches `if`**, with nothing at the call
site to distinguish them: a literal `[ … ]` is unambiguously code, but a
bound name holding a List is read as code too, where the value reading
is at least as plausible.

**Evidence:** `lang/spec/edge-scalars-3.tsv` (the NUR053 domain block
now pins both sides — the Map/None agreement rows and the three List
rows showing the split); `core/go/core_helpers.go` `CoerceBoolean` (the
presence rule the two constructors share); `design/TRUTHINESS.0.md` §2
(amended by NUR053's fix to state the domain, and to name this split).

**Documentation status:** newly documented by this record and the
TRUTHINESS.0.md §2 amendment; before them, nothing stated that `if`'s
domain differs from the constructors' for one shape, and NUR053's own
text asserted the opposite.

**Proposed verdict:** argue or fix, and the options are genuinely
balanced.

- **Allowed** — a List in a condition position is code, full stop; that
  is what a concatenative language should mean by it, and the truthiness
  model governs values, not code positions. Cost: `design/TRUTHINESS.0.md`
  must say the model has one shape-shaped hole, and the `if xs` case
  stays a trap for anyone holding a list in a variable.
- **Fix by distinguishing the spellings** — a LITERAL list condition
  stays code (unchanged), while a condition that arrives as an already-
  evaluated VALUE coerces. This is the reading that makes `if xs` agree
  with both constructors, and it is what a user who wrote `def xs []`
  almost certainly meant. Cost: the two spellings stop being
  interchangeable, and the distinction has to survive the compiled path
  as well as the interpreter.
- **Fix by widening the error** — keep the code-body reading but make
  the empty case a diagnostic that names the ambiguity rather than the
  bare "condition produced no value".

This record does NOT block NUR053, which is resolved: the constructor
pair now shares a domain exactly, and this is the residue that pairing
them revealed.

---

## NUR074 — `canon` renders a function's parameter names, so alpha-equivalent functions render differently {#nur074}

**Status:** Pending · **Recorded:** 2026-08-16 · **Surfaced by:**
`design/unison-hash-identity-probe.0.md` P4, a proof-of-concept pass over the
canon contract; flagged for this register by the PR #376 review (Codex P1).

**Rule:** a value's canonical rendering should depend on the **value**, not on
names incidental to how it happened to be written. NUR031 already applies that
principle to one half of a function's incidental naming — its 2026-08-15
refinement directs that "canon renders the ANONYMOUS fn literal", removing the
*binding* name from the rendering. Parameter names are the same class of
incidental naming and are not covered by that verdict.

**Divergence:** two **unbound** anonymous functions differing only in a
parameter name render differently, so no binding name is involved and this is
independent of NUR031:

```
canon ([x:Number] => [mul x x])  ->  fn [[x:Number][Any][word(mul) word(x) word(x)]]
canon ([y:Number] => [mul y y])  ->  fn [[y:Number][Any][word(mul) word(y) word(y)]]
FAIL  alpha-equivalent fns canon identically
```

The two functions are behaviourally indistinguishable — same arity, same
parameter type, same body modulo the bound name — and there is no source
spelling either could have that the other could not. Landing NUR031 in full
leaves this exactly as it is.

**Why it matters beyond cosmetics:** any scheme deriving an *identity* from
canon (`design/CONTENT-ADDRESSING.0.md`) inherits the divergence — two
alpha-equivalent definitions get two identities, so a content-addressed cache
or dedup would treat them as different code. That document does not propose
changing `canon` to fix it; it lists alpha-normalisation as a step a hashing
layer must perform for itself. This record exists so the choice between those
two homes is made deliberately rather than by default.

**Sharpened by NUR031's fix (2026-08-16):** that record resolved by making a
function's `deq` its CONTENT compared *as canon*, so canon is no longer only a
rendering — it is the equality. Alpha-equivalent functions are therefore not
`deq`, and the bucketed collection scans inherit it: `unique` over
`([x:Number] => [mul x x])` and `([y:Number] => [mul y y])` keeps both. The
"beyond cosmetics" paragraph above is now a live language behaviour rather than
a property of a hypothetical hashing layer, which raises the stakes on
candidate 1 below. Nothing about NUR031's fix depends on the answer — it
compares whatever canon renders — so this stays independently decidable.

**Evidence:** `scripts/hash-identity-probe.boru` P4 and its wrapper
`scripts/hash-identity-probe.sh`; `design/unison-hash-identity-probe.0.md` §4;
`core/go/compare.go` `fnStructurallyEqual` (the canon-as-equality path);
`NUR.md` §NUR031 (retired 2026-08-16 — `git log -S NUR031` for the
binding-name half and its 2026-08-15 refinement).

**Documentation status:** `boru describe canon` says canon renders "canonical,
round-trippable boru source" and says nothing about naming. ADR-015 requires
only that the rendering re-parse to a `deq` value, which the current output
would satisfy for functions once NUR031 and NUR072 land — parameter names do
not break the round-trip, only the canonicity of the rendering.

**Proposed verdict:** none yet; three candidates, and they put the fix in
different places.

1. **De-name parameters in canon**, rendering them positionally the way Unison
   rewrites variables. Makes canon genuinely canonical for functions, but the
   output stops being readable source and the parser would have to accept the
   positional form — a large change reaching `parser/spec`.
2. **Allowed**, on the argument that `canon` renders *source* and source has
   parameter names, so alpha-normalisation is not its job. Then any identity
   scheme must normalise on top of canon, and that obligation should be
   written down where the scheme is (`CONTENT-ADDRESSING.0.md` §4.2 already
   lists it as step 3).
3. **Split the two jobs** — keep `canon` readable and define a separate normal
   form for identity, which is also the cleanest answer to NUR072's bare-word
   question and to the map-ordering question `CONTENT-ADDRESSING.0.md` §4.4
   Phase 0 raises. Most work, and it makes the identity story independent of
   every rendering decision.

Recorded now so the divergence is not silently baselined by NUR031 landing and
appearing to close the function-canon story.

---

## NUR077 — A StackForm can call a word but cannot apply a function value {#nur077}

**Status:** Pending · **Recorded:** 2026-08-16 · **Surfaced by:** fixing the
`OnCall` frame-skeleton over-count — the round-trip contract held for every
NAMED call once that landed, and the residue was exactly this shape

**Rule:** `stackform.Eval(Compile(reg, src))` produces the same final stack as
running `src` directly (`lang/go/stackform/eval.go`). Uniformly: the same
promise for every program, not for a favoured subset.

**Divergence:** the op vocabulary has one invocation form, `Call{Name,
Arity}`, and `Flatten` replays it as a stack-only WORD. That can express
calling a bound word. It cannot express **applying a function value**:

```
([n:Integer] => [n add 1]) 5                  # an inline lambda
def fs [ fn [[n:Integer][Integer][n add 1]] ] ((fs get 0) 5)
def m {f: (fn [[n:Integer][Integer][n add 1]])} (m.f 5)
```

Two independent obstacles, and the second is why a name would not rescue it:

1. An **anonymous** function has no name for `Call` to reference.
2. A `Call` does **not consume a receiver**, while an application consumes the
   fn value the stack already holds. So even for a value that *does* carry a
   name — `def m {f: inc/r}` then `m.f 5` — replaying it as `Call{inc}` leaves
   the function stranded on the stack and produces it twice.

**What it cost, before this record existed.** These forms replayed to the
FUNCTION instead of its result, silently. `lang/go/modules/test.go`'s
gen-program shrinker decides Fail/Pass by replaying the form, so a program in
this class could report a "minimal counterexample" its own generator cannot
produce.

**Interim:** `Eval` REFUSES rather than replays. The engine records these
applications with an EMPTY `Call` name (`execFnDefSig`'s splice, which bypasses
`execMatch` entirely and previously recorded nothing at all), and
`stackform.Replayable` rejects the form with `ErrUnnamedApply`. Loud beats
quietly wrong: the shrinker falls back to value-level shrinking, which is what
it already did for these programs, rather than trusting a bogus form. Pinned
by `TestStackFormRefusesFunctionValueApplication`
(`lang/go/test/stackform_equivalence_test.go`).

One shape is still only *incidentally* loud: a fn read from a container and
dispatched through `execMatch` (`def inc … def m {f: inc/r} (m.f 5)`) records
`Call{dot}` + `Call{inc}`, and the stranded function surfaces at replay as
`uncalled_function` rather than as this refusal. Same root cause; it needs the
same fix.

**A second face of the same gap: a function VALUE cannot be pushed inertly
either.** `Flatten` has to mark a `PushLit` of a Function `Quoted`, because a
Function reaching the engine pointer DISPATCHES and only a Quoted one stays
data. But `Quoted` is STICKY where `/r` is positional
(`design/FUNCTION-VALUE-SCOPE.0.md` §12.6), so the mark is not a faithful
stand-in for the parking the direct run does — left in place it rides out on
the result and the replayed value is permanently inert where the recorded one
was live. `Eval` therefore UNDOES the mark on the way out (`unstamp`), which
restores the recorded state exactly for every shape but one: a program that
produces both a quoted and an unquoted copy of the same function and leaves
the quoted one in the result. An apply-style Op would retire this half too,
since a function that is applied would no longer need to be pushed at all.

Raised by the PR #378 review (Codex P1/P2), which also observed that neither
`canon` nor `deq` can see the difference — `canonFnDef` deliberately omits both
`Captured` and the `Quoted` flag — so the round-trip suite compares Function
values field-wise rather than by canon (`fnValuesEqual`).

**Verdict:** resolve by fix — a **new dedicated Apply Op** (maintainer,
2026-08-17). The op carries the arity, consumes the fn value from the
stack, and is seamed at `execFnDefLiteral` (the only site that knows the
callee arrived as a value — `design/FN-VALUE-OPEN-WORK.0.md` §5.1);
`Flatten` serialises it via the existing `apply` word. `DoEval` stays
reserved — it is payloadless and cannot carry the arity. Three prerequisite
defects are fixed first, as their own changes: the argument-literal
skip-accounting over-count (§5.2 — the silently-wrong half; CLOSED
2026-08-18, pinned by `TestStackFormLiteralAccountingExact`), then the
ADR-016 0-arg anonymous-fn gate (§5 Hole 1 — **CLOSED 2026-08-21**: `apply`
applies a fn whose only signatures are 0-arg at the apply site itself, so
origin no longer decides at arity 0; the data gate itself STAYS, since
removing it defeats `/v` parking and container-held lambdas — pinned in
`lang/spec/valof.tsv` §5) and `apply`'s own double-recording (§5 Hole 2 —
**OPEN**, and the prerequisite carrying a flagged unknown: whether it can
be fixed without new engine state), so the Op does not reintroduce quiet
wrongness.

---

## NUR078 — A bare fn name before a `Function`-typed slot still resolves as a reference, against amended ADR-011 {#nur078}

**Status:** Pending · **Recorded:** 2026-08-17 · **Surfaced by:** the
2026-08-17 ADR-011 amendment (clause 2 of the 2026-08-16 `/r` ruling);
flagged for this register by the PR #381 review (Codex P1)

**Rule:** ADR-011 as amended — a bare name bound to a function CALLS,
universally; passing a function as an argument requires `/r`. The former
exception ("a bare fn name before a `Function`-typed slot resolves as a
reference") is struck.

**Divergence:** the engine still implements the struck exception. With
`def zero fn [[][Integer][7]] def h fn [[f:Function][Integer][42]]`,
`h zero` collects `zero` as a reference and answers `42` — identical to
`h zero/r` — while the same bare name before an `Any` slot is a
call/barrier error. The slot type, not `/r`, decides; that is precisely
what the amendment rejects. The mechanism is the four clause-2 sites
(`design/FN-VALUE-OPEN-WORK.0.md` §3.2): `stepWord`'s TFunction intercept
(`core/go/engine.go:2679-2702`), `hasPendingForwardExpectingFunction`,
`sigWantsFunctionAt`, and the ReachGroup-arrival `ConformsTo(TFunction)`
test.

**Evidence:** `lang/spec/path-modifier.tsv:67` pins the exception as
designed behaviour ("the planner's designed TFunction intercept"); two
more bare uses sit in the frontier divergence ledger (§3.3's count of 3).

**RE-AFFIRMED 2026-08-26 — implement as amended.** Re-litigated under O1
(`design/O1-RELITIGATION.0.md` §2) and the amendment stands: a bare name
bound to a function CALLS, universally, and passing one as an argument is
explicit. The slot type must stop deciding what a token means — that is the
same class of context-sensitivity Stage 4's descriptor would otherwise have
to carry as live state, and one rule for every slot type is worth the
ergonomic loss.

Read the record's `/r` as `/v` throughout, per this file's 2026-08-19
spelling note; `h zero/v` is the spelling that works today. An earlier draft
of the O1 brief claimed the `/r` spelling was itself a defect directing users
to a path that does not exist. It is not — the convention covers it — and
that claim is withdrawn.

**Verdict:** resolve by fix — open-work item B (ruled 2026-08-17,
re-affirmed 2026-08-26). All four sites retire together,
`sigWantsFunctionAt` included, which re-opens the NUR038 call-head question
inside the implementing PR; the `unused_def` use-recording re-homes with the
intercept, and `path-modifier.tsv:67` plus the two frontier rows are
rewritten. Every `h zero` call site becomes `h zero/v`. This record retires
when that fix lands.


## NUR079 — Gated words inside an imported file-module body escape the policy that governs the same call at top level {#nur079}

**Status:** Pending · **Recorded:** 2026-08-18 · **Surfaced by:** the
Roc comparison study (`design/roc-in-boru-report.0.md` §7.1), while
checking Roc's claim that `roc check`/`roc build` perform no dependency
I/O

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


## NUR080 — A typed def over an Integer literal loses its newtype brand under the compiler, and the bare literal gains one {#nur080}

**Status:** Pending · **Recorded:** 2026-08-18 · **Surfaced by:** the
Roc comparison study (`design/roc-in-boru-report.0.md` §7.2), while
measuring boru's nominal-newtype guarantee against Roc's opaque types

**Rule:** the two engines agree. A bare `refine` is a nominal newtype
(`EXPLANATION.md`, "Function signatures and refinement types"): a raw
`Integer` is not a `UserId`, and a value constructed as `def id:UserId 42`
is one — symmetrically, at every boundary, on either engine.

**Divergence:** with `def UserId (refine Integer)` and `def b:UserId 9`,
the compiled path answers differently from the interpreter, in **both**
directions depending on source order:

| program order | engine | `typeof 9` | `typeof b` | `b is UserId` |
|---|---|---|---|---|
| `typeof 9` first | `--no-compile` | `Integer` | `UserId` | `true` |
| `typeof 9` first | default / `--force-compile` | `Integer` | `Integer` | `false` |
| `typeof b` first | `--no-compile` | `Integer` | `UserId` | — |
| `typeof b` first | default | **`UserId`** | `UserId` | — |

So a declared binding loses the brand it was given, and a bare literal
acquires a brand it never had. `boru check` reports `0 error(s), 0
warning(s), 0 info`, and `--force-compile` does not refuse — it compiles
and answers wrongly, which puts this outside the
`MarkUncompilable`-is-always-sound architecture. Compiled mode is the
execution default since the P7 endgame, so this is the default answer.
A String newtype (`def Name (refine String)`) did not reproduce it.

**Evidence:** the shape — a literal's identity changing according to
whether a typed def elsewhere in the program mentioned it — points at the
const-pool entry for the literal being reparented rather than a fresh
value being minted, i.e. the residual of
`design/MISCOMPILE-HUNT-FINDINGS.0.md` §B, whose July-2026 update states
"The static/concrete path is untouched." No `lang/spec/*.tsv` row covers
`typeof` or `is` over a typed def of a literal in both source orders,
which is why `make verify-bytecode` is blind to it.

**Verdict:** resolve by fix — the static/concrete arm of `def name:T
<literal>` must mint a fresh value rather than reparent the shared
const-pool entry. Acceptance requires both source orderings, plus new
spec rows so the differential corpus covers the class. This record retires
when the two engines agree on the table above.

## NUR081 — `Test.skip` accepts the counts `Test.check-prop` now rejects, though it is documented as a drop-in {#nur081}

**Status:** Pending · **Recorded:** 2026-08-18 · **Surfaced by:** the
Wave 1 vacuous-property fix (`design/QUINT-FOLLOWUP-PLAN.0.md`), in review

**Rule:** one member of a word family does not get a different contract
from its siblings. `HOWTO.md` presents `Test.skip` as "a drop-in for
`Test.check-prop`" — same name, same generator, same property, same three
counts — so a call that is legal for one should be legal for the other.

**Divergence:** `Test.check-prop` now raises `range_error` on `runs < 1`
or `max-shrinks < 0`, closing a hole where a zero-run property reported
`{ok: true, runs: 0}` and printed as a PASS. `Test.skip` takes the same
six arguments and validates none of them:

```
Test.check-prop "x" [1] [true] 0 1 -7   →  error: [boru/range_error]: runs must be 1 or more (got 0)
Test.skip       "x" [1] [true] 0 1 -7   →  {name:'x' ok:true skipped:true runs:0}   skip: x
```

**Evidence:** `Test.skip` never reaches `runCheckProp` (it short-circuits
to `runSkipProp`), which is why the guard does not cover it. That is also
why the divergence is harmless today: a skipped property is *reported as
skipped*, never as a pass, so the counts it ignores cannot manufacture a
false green the way `check-prop`'s could.

**Verdict:** pending. Two defensible resolutions — validate in `skip` for
symmetry, or document that `skip` ignores the counts because it never runs
them. The second is arguably more honest, since the arguments exist only
so the call stays a textual drop-in. Not urgent: no false PASS is reachable
through `skip`.

## NUR082 — Three tree-walking subcommands, two rules for `.boru/` {#nur082}

**Status:** Pending · **Recorded:** 2026-08-18 · **Surfaced by:**
giving `boru check` directory targets (W-CLI-CHECK), which forced a
choice between the two rules already in the tree

**Rule:** one rule decides which files a subcommand's directory walk
considers. A user who learns what `boru fmt .` touches knows what `boru
check .` and `boru test .` touch.

**Divergence:** the walks disagree about `.boru/`, the package directory
holding fetched and generated artifacts:

| command | walk | `.boru/` |
|---|---|---|
| `boru fmt` (no args) | `filepath.Walk(".")`, `*.boru` | **skipped** (`cmd/go/internal/fmt/fmt.go`) |
| `boru test [dir]` | `walkDir(t)`, `*_test.boru` | **walked** (`discover()`, `cmd/go/internal/test/test.go`) |
| `boru check [dir]` | `walkDir(t)`, `*.boru` | **skipped** (`resolveTargets`, `cmd/go/internal/check/check.go`) |

`check` was written to fmt's rule rather than test's: reporting type
errors inside `.boru/` tells the user about code they did not write and
cannot fix, and a CI gate that fails on a dependency's source is a gate
people turn off. The same argument applies to `boru test` discovering and
RUNNING a vendored suite, so the divergence is `test`'s to close, but a
third rule was not invented to avoid deciding.

The rules also differ in what a walk starts from — `fmt` walks only when
given no arguments and never expands a directory operand, while `test`
and `check` expand every directory target — and `fmt` alone treats
`.md`/`.html` targets as embedded-boru carriers.

**Evidence:** `cmd/go/internal/fmt/fmt.go` (the `.boru` `SkipDir` arm),
`cmd/go/internal/test/test.go` `discover()` (no such arm),
`cmd/go/internal/check/check.go` `resolveTargets` and its
`TestRunCLIDirectoryTargetRecurses` case, which pins the skip.

**Verdict proposed:** resolve by fix — one shared walk helper carrying
the `.boru/` skip, adopted by all three, with per-command control only
over the filename predicate. This record retires when `boru test`'s
discover skips `.boru/` through that shared helper.


## NUR083 — `check` and `build` anchor relative imports to the file, `run` and `debug` to the process cwd {#nur083}

**Status:** Pending · **Recorded:** 2026-08-18 · **Surfaced by:**
multi-file `boru check` (W-CLI-CHECK), which cannot use the cwd rule

**Rule:** `import "./lib.boru"` names a file relative to the importer.
Every command that reads a boru program should resolve it the same way,
or a program's meaning depends on which command opened it and from where.

**Divergence:** two anchors are in use.

| command | anchor | site |
|---|---|---|
| `boru build` | the entry file's directory | `check.PreflightColorAt(..., cfg.EntryDir)`; `buildrt.Main` sets `NativeRegistry().BaseDir` to the same |
| `boru check <file...>` | **each target file's own directory** | `check.RunTargets` |
| `boru run` / `boru debug` | the process working directory | `check.PreflightColor` (empty baseDir), and the execution that follows |

Verified live before the multi-file change: with `sub/lib.boru` and
`sub/m.boru` importing `./lib.boru`, `boru run sub/m.boru` from the
parent directory fails with `undefined word: Lib`, while a binary built
by `boru build sub/m.boru` runs from anywhere. `boru check sub/m.boru`
followed `run` before this change and now follows `build`.

`check` could not stay on the cwd rule: `boru check a/x.boru b/y.boru`
has no single working directory that is right for both targets, so the
cwd rule cannot be applied uniformly to a multi-target invocation at all.
Anchoring per target makes `check` agree with `build` and with the file's
own text, at the cost of a NEW disagreement with `run`/`debug` — `check`
can now accept a program `run` refuses from that cwd. That is recorded
here rather than hidden: the anchor `run` uses is the one out of step
with the language's own `./` spelling, and it is the remaining half of
the fix.

**Evidence:** `cmd/go/internal/check/check.go` (`RunTargets`'s anchor and
`PreflightColorAt`'s NUR044 comment), `cmd/go/internal/build/build.go`
(`cfg.EntryDir`), `cmd/go/internal/run/run.go:192` and
`cmd/go/internal/debugcmd/debugcmd.go:132` (empty baseDir);
`TestRunCLIAnchorsImportsPerFile` and `TestRunColorKeepsCwdAnchor` pin
both sides. `CLI.md`'s `boru check` section documents the split.

**Verdict proposed:** resolve by fix — `run` and `debug` anchor a
script's relative imports to the script's directory (with the REPL and
`-e`, which have no file, keeping the cwd), so one rule covers every
command. This record retires when `boru run sub/m.boru` succeeds from the
parent directory.


## NUR084 — `-h` is not a uniform surface: some subcommands print usage, some read it as a filename, none exit 0 {#nur084}

**Status:** Pending · **Recorded:** 2026-08-18 · **Surfaced by**
`boru check -h` failing with `error: open -h: no such file or directory`
(W-CLI-CHECK), which `boru help check` tells users to run

**Rule:** `boru help <cmd>` ends with "Run 'boru <cmd> -h' for its
options", so `-h` is a documented surface of every subcommand and should
behave the same everywhere.

**Divergence:** three behaviours.

| command | `-h` |
|---|---|
| `build`, `test`, `check` (and every other `flag.FlagSet` user) | prints the flag listing to **stderr**, exits **1** |
| `fmt` | `error: open -h: no such file or directory`, exits 1 — it has no FlagSet and reads `-h` as a path |
| every command | never exits **0**, because `flag.ErrHelp` is reported through the same `return 1` as a rejected flag |

**Updated on merge with `main`:** `check` now does the RIGHT thing rather
than the majority thing. The `--pedantic` work (PR #386) gave it a
hand-written `printUsage` and made `-h` print to **stdout** and exit
**0**; the multi-target rewrite preserved that through the FlagSet
conversion instead of regressing it to `flag.ErrHelp` → exit 1. So the
table above now has a fourth row — `check`: usage on stdout, exit 0 —
and it is the row the others should converge on, not an anomaly.

The remaining non-uniformities are `fmt` (no FlagSet at all) and the
exit code everywhere else: an answered help request is a success, and a
CI script that runs `boru fmt -h` to probe for a flag cannot tell "no
such flag" from "help printed".

**Evidence:** `cmd/go/internal/fmt/fmt.go` `Run` (positional loop, no
FlagSet); `cmd/go/internal/build/build.go`, `cmd/go/internal/test/test.go`
and `cmd/go/internal/check/check.go` (`fs.Parse` error → `return 1`);
`cmd/go/internal/help/help.go` `helpCommand` (the instruction to run
`-h`); `TestRunCLIHelpFlag` pins check's side, asserting exit 0 for
`-h`/`--help`/`help` and that an unknown flag still fails.

**Verdict proposed:** resolve by fix — `fmt` gains a FlagSet, and
`flag.ErrHelp` is distinguished from a parse failure so a help request
exits 0 across the CLI, matching what `check` already does. This record
retires when `boru fmt -h` also prints its flags and exits 0; `check`'s
half is done.

---

## NUR088 — `boru fmt` collapses one of the six signature spellings, not the other five {#nur088}

**Status:** Pending · **Recorded:** 2026-08-19 · **Surfaced by:** writing
`STYLE-GUIDE.md` §S1 ("use the fewest square brackets") and measuring
what `boru fmt` actually does with each spelling

**Rule:** a formatter's job is to collapse equivalent spellings to one.
`boru fmt` is documented as producing "canonical layout"
(`describe`/`CLI.md`), and `AGENTS.md` makes forward call form canonical
for new code — one spelling per idea, mechanically enforced.

**Divergence:** a single-parameter, single-return signature has **six**
valid spellings, all building a `canon`-identical value. `fmt` rewrites
exactly one of them.

| spelling | pairs | `boru fmt` |
|---|---|---|
| `fn x:Integer Integer [mul 2 x]` | 1 | — (already the target) |
| `fn x:Integer [Integer] [mul 2 x]` | 2 | **unchanged** |
| `fn [x:Integer Integer [mul 2 x]]` | 2 | **unchanged** |
| `fn [[x:Integer] Integer [mul 2 x]]` | 3 | **unchanged** |
| `fn [x:Integer [Integer] [mul 2 x]]` | 3 | **unchanged** |
| `fn [[x:Integer] [Integer] [mul 2 x]]` | 4 | → `fn x:Integer Integer [mul 2 x]` ✅ |

The unnamed-parameter twin behaves the same way:
`fn [[Integer] [Integer] [add 1]]` → `fn Integer Integer [add 1]`, while
the four intermediates pass through.

That the six are one value, not merely one answer:

```
def s1 fn x:Integer Integer [mul 2 x]
…
def s6 fn [[x:Integer] [Integer] [mul 2 x]]
print (deq (canon s1/v) (canon s6/v))   ;# true, and likewise s2…s5
```

**Consequence:** a file can be `fmt`-clean and still carry four spellings
of one signature, so `fmt` cannot be the enforcement point for the house
rule — a reviewer has to be. The gap is not in the rule (the target form
is already chosen and already implemented) but in its reach: `fmt`
normalises from the canonical form only, rather than canonicalising the
intermediates first and then reducing.

**Evidence:** the table above, measured against this tree by running
`boru fmt` on each spelling; `design/HIGHER-ORDER-FUNCTIONS.0.md` §4.2;
`STYLE-GUIDE.md` §S1 carries the same table as the formatter-status note.
`boru describe fn` documents the underlying two-form rule (*"a list input
always selects the spec-list form"*), which is what makes
`fn [x:Integer] [Integer] [mul 2 x]` an error rather than a seventh
spelling. Not previously recorded.

**Documentation status:** the equivalence is documented, the formatter's
partial coverage is not. `boru describe fn` states the two forms and
gives the equivalence explicitly (*"fn x:Integer [Integer] [x mul 2] ≡
fn [[x:Integer] [Integer] [x mul 2]]"*) and notes that the output slot
takes both spellings. Nothing states which spelling is preferred — that
is new in `STYLE-GUIDE.md` §S1 — and nothing states that `fmt` implements
the preference for only one input form. `CLI.md`'s `fmt` section
describes canonical layout without enumerating what is and is not
normalised.

**Verdict proposed:** resolve by fix — `fmt` canonicalises a signature
before reducing it, so all five non-target spellings converge on the
one-pair form, leaving the irreducible shapes (2+ parameters, 0
parameters, multi-overload spec lists) untouched as it already does. This
record retires when each of the five rewrites to
`fn x:Integer Integer [mul 2 x]`, pinned as a `fmt` round-trip test.

---

## NUR089 — an inline lambda and a named reference to the same function are not equally checkable {#nur089}

**Status:** Pending · **Recorded:** 2026-08-19 · **Surfaced by:** the
function-type prototype (`design/FUNCTION-TYPES.0.md`), while
establishing which of the audit's combinator blocks check clean and why

**Rule:** a function value is a function value. ADR-011's "calling is an
act of the use site" makes the *call* the only thing that distinguishes
a reference from a call; nothing in the model makes an anonymous
function less analysable than a named one. `/v` and a `=>` lambda are
two spellings of "this function, as data".

**Divergence:** the same combinator, passed the same two functions,
producing the same answer, checks clean in one spelling and errors in
the other.

```
$ cat lam.boru
def bb f:Function => [g:Function => [x:Any => [f (g x)]]]
print (((bb (n:Integer => [mul n 2])) (n:Integer => [add n 3])) 4)
$ boru check lam.boru
check: 1:51: [error] no_signature: cannot call `g` — no signature matches
  the arguments; got (Integer); assuming best-fit candidate for analysis
check failed: 1 error(s)
$ boru run -no-check lam.boru
14

$ cat ref.boru
def d fn n:Integer Integer [mul n 2]
def e fn n:Integer Integer [add n 3]
def bb f:Function => [g:Function => [x:Any => [f (g x)]]]
print (((bb d/v) e/v) 4)
$ boru check ref.boru
check: 0 error(s), 0 warning(s), 0 info
$ boru run -no-check ref.boru
14
```

`got (Integer)` is the tell: with the lambda spelling the analysis binds
the **third** application's argument to `g`, the **second** parameter.
Varying only that final argument moves the reported type with it, which
pins the misbinding exactly:

```
… ) 4)     → no_signature: cannot call `g` … got (Integer)
… ) 'zz')  → no_signature: cannot call `g` … got (ProperString)
… ) 3.5)   → no_signature: cannot call `g` … got (Float)
```

The reference spelling tracks the same three applications correctly.

**Not a property of the combinator.** `ss` (the `S` of §1.1, which the
audit shows checking clean) fails identically when it is called with
inline lambdas instead of `kk/v`; `bb` passes when called with
references. Only the argument spelling moves the result. Defining either
combinator without calling it is clean in both spellings, so the defect
is in the analysis of the *call*, not the body.

**Not fixed by declaring a function type.** Replacing `f:Function` with
a declared `f:IntToInt` (`design/FUNCTION-TYPES.0.md`) leaves the lambda
call site failing — re-confirmed 2026-08-20 on the type-node-fusion
tree, where the diagnostic becomes `no_signature: cannot call bb … got
(Function); nearest [IntToInt]` (the named fn-shape param now carries
real FnUndef membership, and the inline-lambda misbinding persists
through it). Widening the declared return to `Any` — which the
lambda genuinely has, since `=>` declares no return type — makes the
program run while the check still errors. So this record is orthogonal
to the opacity of `Function` and survives that work. (Re-run
2026-08-21: under the widened `fnsig Integer Any` type the surviving
diagnostic lands on `f` and renders the got-type as the internal
placeholder `(__PE)` — the misbinding persists, and the renderer now
leaks a synthetic type name into a user-facing message.)

**Consequence:** the ergonomic spelling is the one that fails. A reader
converting an example to the shorter inline form gets a check failure on
a program that runs correctly, and the diagnostic names a type
(`Integer`) that appears nowhere in the parameter's declaration —
pointing the reader at the wrong thing.

**Verdict proposed:** resolve by fix — the analysis must bind an inline
lambda argument to the same parameter the reference spelling binds it
to. This record retires when the minimal pair above checks clean in both
spellings, pinned as a spec row.

---

## NUR091 — a rejected `fn` declaration is loud or silent depending on its output slot {#nur091}

**Status:** Pending · **Recorded:** 2026-08-19 · **Surfaced by:** probing
`fn`'s `(tnot List)` input guard while investigating NUR090 (retired
2026-08-20 — the name→node flip, `design/TYPE-REPRESENTATION.1.md`)

**Rule:** boru refuses loudly. `lang/spec/fn-triple.tsv` §4 pins the
input guard's refusal as an ERROR row — *"a bare List type literal input
is rejected by (tnot List) — a single List-typed param needs the
spec-list form"* — and §4's neighbours make a truncated triple "fail
loudly instead of absorbing a following value".

**Divergence:** the same rejected declaration is loud or silent
depending on what sits in the OUTPUT slot.

```
$ boru do 'def f fn List [Integer] [size]'
error: [boru/signature_error]: cannot call `size` …          ← loud, as pinned

$ boru do 'def f fn List Any [1]'
[1] Any List
$ echo $?
0                                                             ← silent: 3 values stranded
$ boru do 'def f fn List Any [1]  typeof f/v'
[1] Any List Atom                                             ← and `f` was never bound
```

Both declarations are rejected for the same reason: `List` at slot 0
fails the triple form's `(tnot List)` pattern, and the 1-arg spec-list
sig cannot take a bare `List` type literal either. With a bracketed
output the following word (`size`) is dispatched and raises; with a bare
`Any` output nothing raises, `fn` never dispatches, `def` binds nothing,
and the operands are left on the stack.

**Consequence:** `def f fn List Any [1]` looks like a definition, exits
0, and defines nothing — the §5.1 class of the higher-order audit (a
statement that appears to work, exit 0, wrong stack). A reader gets no
hint that the `(tnot List)` rule was the problem, or that the spec-list
form is the remedy.

**Not NUR090** (retired 2026-08-20). That record was about which type a
NAME denotes — resolved by the name→node flip; this one is about a
declaration that is correctly refused failing quietly instead of
loudly. They met only in that both were found probing slot 0.

**Evidence:** the transcripts above, reproduced against this tree. The
documented remedy works and is loud about nothing:
`boru do 'def f fn [[List] [Any] [1]]  f [1 2]'` → `1`.

**Not pinned in the corpus, deliberately.** A spec row for this was added
to `fn-triple.tsv` §4 and then withdrawn: its input displaced a seed from
the variation sample and emptied an unrelated `varyRefusalLedger` bucket,
which is NUR092. The transcripts above are the record; a documentation
row should not cost an unrelated ratchet.

**Verdict proposed:** resolve by fix — a `def` whose value expression
dispatched nothing should raise rather than strand, or `fn` should
refuse a non-matching operand shape with a diagnostic naming the
`(tnot List)` rule. This record retires when
`def f fn List Any [1]` reports an error, pinned beside
`fn-triple.tsv` §4's existing loud twin (once NUR092 makes that safe).

---

## NUR092 — the variation ledger's stale arm is corpus-sensitive {#nur092}

**Status:** Pending · **Recorded:** 2026-08-19 · **Surfaced by:** adding
two rows to `lang/spec/fn-triple.tsv` for NUR091 and watching CI fail in
an unrelated bucket

**Rule:** `vary.Sample`'s own contract, in its doc comment: *"Samples
NEST: `Sample(s, n)` is a prefix of `Sample(s, m)` for n < m, so cranking
breadth strictly ADDS variants and a bucket observed at the default
breadth stays observed at any larger one — **the property the CI ledger's
stale arm relies on**."*

**Divergence:** the nesting property holds across **n**, not across
**corpus content**. `Sample` orders seeds by `fnv64(seed.Input)` and
takes the first 32, so adding ANY spec row whose input hashes low enough
displaces a seed that was previously in the sample. If the displaced seed
was the only one exercising a refusal class, its `varyRefusalLedger`
bucket empties and the stale arm fires:

```
vary_differential_test.go:83: stale varyRefusalLedger bucket
  "other: capture ParseLang of calc unreachable at a call site"
  — no variant refuses in it any more; graduate it (delete the entry).
```

**The instruction is wrong in this case, and destructively so.** The
class is not fixed; it is merely unsampled. Re-running at greater breadth
brings it straight back:

```
$ BORU_VARY_SEEDS=64 go test ./test/go/langspec -run TestVariationDifferential
… refusal buckets: map[… other: capture ParseLang of calc unreachable at a call site:3 …]
```

Following the diagnostic would delete a ledger entry for a refusal class
that still exists — losing the record and the frontier row it points at,
for a reason (someone added an unrelated spec row) with no connection to
the class at all.

**Evidence:** the two rows were added in 6876dcd; `git checkout` of each
preceding commit on the branch (ba311b6, ebf9d58, b835aa2, 204ba21) shows
the stale arm silent, and `origin/main` is green. Removing the two rows
restores it. The rows in question were documentation of NUR091's current
behaviour and were withdrawn to the NUR record instead — a corpus row
should not cost an unrelated ratchet.

**Consequence:** a contributor adding a spec row can be told, by a
passing-looking diagnostic, to delete a live record. The failure is
remote from its cause (a `fn-triple.tsv` row emptied a `ParseLang`
bucket), which makes it hard to attribute without bisecting.

**Verdict proposed:** resolve by fix. Candidates: make the stale arm
re-check an emptied bucket at greater breadth before reporting it;
sample by a corpus-position-independent key so additions do not displace;
or pin the seed set explicitly rather than deriving it by hash prefix.
This record retires when adding an unrelated passing spec row cannot
empty a ledger bucket, pinned as a test.


## NUR096 — the checker still models a fn-shape-typed member apply as the inert fn it was before NUR095 {#nur096}

**Status:** Pending · **Recorded:** 2026-08-20 · **Surfaced by:** adding
two multi-return rows to `lang/spec/class.tsv` to pin the NUR095
retirement and watching `TestCheckTypeSoundness` fail on both

**Rule:** the interpreter, the compiled lane and the check pass model one
language. NUR095 retired the split where a fn stored through a fn-SHAPE-typed
member was applied by the interpreter but left inert by the recorder;
`core.TypeIsFnShape` / `core.IsFnTypedCarrier` closed it for *execution*.

**Divergence:** the check pass did not move with them. For a fn-shape
carrier followed by its argument at the top level, `check` still reports the
PRE-NUR095 stack — the carrier and the argument, un-applied:

```
def T fnsig [[Integer] [Integer Integer]] def C class {op:T}
((make C {op:(fn [[x:Integer] [Integer Integer] [x x]])}) dot op) 10

check   : T Integer      ← the fn is inert, 10 sits above it
runtime : 10 10          ← the fn is applied and returns two values
```

Before the NUR095 fix these agreed: the runtime really did leave the fn
inert (`fn (Integer) 10`). Fixing execution without fixing analysis turned
agreement into divergence.

**Why it went unnoticed:** a SINGLE-return shape hides it. `stackTypeCovered`
compares top-aligned and returns early when `len(actual) <= len(checked)`, so
a 1-return apply (`checked=[T, Integer]`, `actual=[6]`) only ever compares the
top slot, which matches. A 2-return apply makes the counts equal, the bottom
slot is compared, and `T` does not admit an `Integer` — a type-soundness
violation against the corpus-wide pin of 0. Every fn-members row already in
`class.tsv` is single-return, which is why the section landed green.

**Scope:** analysis only. The compiled and interpreted lanes agree for these
shapes — pinned as a differential in `lang/go/fnshape_multireturn_test.go`
across the class-member leading apply and the loop-body dynamic apply
(`setLoopBodyApply` → `OpCallDynamic`); the VM appends every result the fn
returns, so no value is lost or truncated. It is the checker's picture that
is stale.

**Resolution direction:** teach the check pass the same maybe-callable rule
execution now uses — a fn-shape-typed carrier followed by a materialisable
argument auto-applies, so the modelled stack is the applied result, not the
carrier. The gradual fallback stays available where the shape's result count
is not statically known. This record retires when the multi-return rows above
can live in `class.tsv` with the soundness pin still at 0.

---

## NUR097 — one syntax, two binding regimes: fn-locals are captured, module-scope names resolve late {#nur097}

**Status:** Pending · **Recorded:** 2026-08-21 · **Surfaced by:** the
higher-order capability audit's §5.6
(`design/HIGHER-ORDER-FUNCTIONS.0.md`), re-assessed and pinned 2026-08-21

**Rule:** one binding store, one resolution rule. A name in a fn body
should mean the same kind of thing wherever it was bound.

**Divergence:** what a closure's body name denotes depends on WHERE the
name was bound, with no marking at the use site. A parameter or body-local
`def` is *captured* — the closure keeps it after the defining scope exits,
and two closures from one factory hold distinct copies. A module-scope
name is *not* captured: it resolves at call time through the def shadow
stack, so a later `def` of the same name changes what an existing closure
computes, and `undef` changes it back.

```
$ boru do 'def n 1 end def c x:Integer => [add n x] end def n 100 end (c 5)'
105                                          ← the later shadow push wins
$ boru do 'def n 1 end def c x:Integer => [add n x] end def n 100 end undef n end (c 5)'
6                                            ← undef pops back to the original
$ boru do 'def n 1 end def mkc fn nn:Integer Function [(fn x:Integer Integer [add nn x])] end def c (mkc n) end def n 100 end (c 5)'
6                                            ← routed through a parameter, the value is frozen
```

**Consequence:** a combinator defined before a name it mentions is
re-`def`ed silently changes meaning — exit 0, no diagnostic, and the use
site gives no hint which regime the name is under. An ML/Haskell reader
expects the second answer everywhere (a new binding never reaches an
existing closure); an Emacs Lisp or Python-globals reader expects the
first. Both are internally consistent conventions; carrying BOTH behind
one syntax is the non-uniformity.

**Evidence:** the transcripts above, reproduced against this tree; pinned
as `lang/spec/frontier/frontier-hof-audit.tsv` §8 (all three rows). The
contract is documented in `REFERENCE.md` §"Definition and scoping" and
argued, with the cross-language positioning, in
`design/HIGHER-ORDER-FUNCTIONS.0.md` §5.6.

**Verdict proposed:** **Allowed, plus a diagnostic.** The late half is
load-bearing: top-level liveness — redefinition reaching existing words —
is what makes the def stack, `undef`, and REPL-driven development
coherent, and the capture half is what makes closures usable at all; the
mainstream compromise (Python globals vs locals, Racket's late REPL vs
early modules) has the same shape. The freezing idiom (route the name
through a parameter, third transcript) covers the cases that want the
other regime. What the divergence costs is a *diagnostic*: the in-file
case — a fn body reads a module-scope name that a LATER `def` in the same
file re-binds — is statically visible and is exactly the transcription
hazard; an info-severity check hint there ("`c` reads `n`, re-`def`ed at
line N; module names resolve late") would catch it without touching
semantics. This record is discharged by an Allowed verdict naming that
hint as the mitigation (or by a maintainer ruling the hint unnecessary,
with the §8 rows and the REFERENCE.md contract as the pinned acceptance).


## NUR102 — a predicate body runs a different number of times in each lane {#nur102}

**Status:** Pending · **Recorded:** 2026-08-25 · **Surfaced by:** the
Stage-2 collection-kernel feasibility probe for
`design/FULL-COMPILATION.0.md` (falsifier F1), which found overload
pruning to be an evaluation site the design had not accounted for

**Rule:** the compiled lane reproduces the interpreter's observable
behaviour exactly — values, errors, **and effects in their order**
(`design/COMPILABLE-SUBSET.md` §1: "a residual byte-identical to the
interpreter's `Run` for the same source"). A user-visible boru body must
not run a different number of times depending on which lane executes it.

**Divergence:** an effectful `fnpred` body runs **four times interpreted
and twice compiled** for the same call, with the same final value:

```
$ cat pred.boru
def Even fnpred n:Integer [ print "P" eq 0 (mod 2 n) ] end
def we fn [[a:Even][String]["even-arm"]] end
def we fn [[a:Integer][String]["int-arm"]] end
print (we 4)

$ boru run -no-compile pred.boru
P
P
P
P
even-arm

$ boru run -force-compile pred.boru
P
P
even-arm
```

**Mechanism.** Predicate types are checked by *running boru code inside
signature matching*: `SigArgMatches` reaches `PredicateUnifier.Match`
(`core/go/unify_predicate.go:51-68`) → `Registry.RunPredicate`
(`core/go/registry.go:1917`) → the predicate's body. The interpreter
reaches that path more often than the VM does, because **overload pruning
during forward collection is itself an evaluation site**: `pruneViable`
(`core/go/engine.go:1919-1934`) calls `SigArgMatches` while it is still
deciding which signatures remain viable, once per surviving candidate per
scan step. The compiled lane matches once over concrete values and
never replays the pruning. The count is therefore a function of *how the
call was written*, not of what it computes — the same probe measured the
forward form `we 4` entering the scan body while the stack form `4 we`
breaks at the boundary token and never prunes at all.

**Why it was not caught.** The differential gates compare residual values
and error taxonomy; neither compares **effect counts**, so a divergence
visible only as repeated output, repeated logging, or repeated mutation
inside a predicate passes every existing gate. The frontier ledger has no
predicate-effect row, and `design/COMPILABLE-SUBSET.md` mentions
predicates only for typed binds and tape-bound handlers.

**Why it matters beyond purity.** A predicate is ordinary boru: it may
log, may touch a store, may be expensive. "Slow, not wrong" does not
cover it — the two lanes disagree observably, and which one is *correct*
is itself unsettled, since neither count is obviously the specified one.

**Discharge.** Either (a) rule that predicate bodies must be pure, and
enforce it, at which point the count is unobservable and this becomes
Allowed; or (b) make the count part of the contract and have both lanes
produce it, which the full-compilation plan's §6.3 predicate-unit work
would need to do anyway. Whichever way it goes, the differential harness
needs an effect-count comparison, or the next divergence of this shape is
equally invisible.

## NUR103 — the checker's answer depends on who is asking {#nur103}

**Status:** Pending · **Recorded:** 2026-08-26 · **Surfaced by:** the
server-concurrency corpus (`test/go/servercorpus`), measuring why a real
protocol server does not compile

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


## NUR104 — a record type means two different things depending on how it is spelt {#nur104}

**Status:** Pending — the two field shapes found so far are fixed, the
construction asymmetry that produced them is not (see Discharge) ·
**Recorded:** 2026-08-26 · **Surfaced by:** tracing NUR103's nameless `undefined_word` —
the mechanism turned out to be general, so the neighbouring shapes were
probed and one of them diverged outright

**Rule:** one type, one meaning. A record type constrains a slot the same
way however it was written; the surface spelling is not part of the
semantics.

**Divergence.** It is. A record type reaches a parameter slot by two
routes, and only one of them ever EVALUATES its field map:

| spelling | how the field map is built | field values |
|---|---|---|
| `type R record {…}` / `refine Record [{…}]`, then `o:R` | the word DISPATCHES, so the map is evaluated | types |
| inline `o:{…}` | rides inside the fn-spec LIST, which is inert data | raw tokens |

The dispatcher papered over the easy case — `Unify` resolves a bare type
NAME at match time — so `o:{pretty:Boolean}` dispatched correctly and the
asymmetry stayed invisible. It was not invisible one level up. A field
whose type is an EXPRESSION kept an unevaluated paren the matcher could
not read, and the two spellings gave opposite answers to the same call:

```
$ boru run -e 'def R (refine Record [{a:(Integer tor String)}])
              def f fn [[o:R][Any][o.a]]  f {a:7}'
7

$ boru run -e 'def f fn [[o:{a:(Integer tor String)}][Any][o.a]]  f {a:7}'
check: [error] no_signature: cannot call `f` — no signature matches the
arguments; got (Map); nearest [Map]
```

Same type, same argument, same question; one answers 7 and the other
refuses. Nothing about `(Integer tor String)` is exotic — an optional
field (`{y?:String}`) is spelt with a disjunct too.

**Why it matters.** This is the deeper version of NUR103, whose whole
mechanism was a downstream consumer reading a field type the inline
spelling had never resolved. There, an unresolved WORD produced a
Word-typed carrier and a nameless diagnostic. Here, an unresolved
EXPRESSION produces a pattern that matches nothing. The two are the same
defect at different depths, which is the argument for fixing the cause
rather than either symptom: any consumer that reads an inline record's
field types is reading tokens where the named spelling gives it types,
and there is no reason to expect the two found so far are the last.

**Discharge.** `ResolveSigRecordFields` (`core/go/generics_unify.go`)
resolves an inline record pattern's field types at sig install, which is
where the named spelling's own dispatch had already resolved them: a bare
type word through `ResolveSigChildParam`'s cascade, a nested inline record
recursively, and a field type EXPRESSION by evaluating it, exactly as
`ResolveChildTypeExpr` evaluates a typed container's paren child.

It deliberately does NOT delegate to `ResolveFieldType`, the cascade
`record` and `class` run over their own field maps, because that resolver
also evaluates a concrete List field as code — right for a type
DECLARATION, wrong for a dispatch PATTERN, where `{a:[1 2]}` pins the
field to that list. The two share the type-name cascade, not the value
arms. That line is pinned by a spec row.

Evaluation failure at sig install is SILENT and leaves the field alone: a
paren that does not evaluate to a single type was never a type constraint,
and a signature install is not a place to raise.

One consequence is worth stating outright rather than leaving implied: a
field expression now RUNS when the signature installs, so
`fn [[o:{a:(print "hi")}] …]` prints at `def` time. That is not new
behaviour being introduced — it is the named spelling's behaviour, which
`refine Record [{a:(print "hi")}]` has always had, arriving at the
spelling that lacked it. Alignment is the point; a divergence in WHEN a
type expression runs is as real as a divergence in what it means.

**What is NOT closed by it.** The general property — that the two
spellings are built by different machinery at all — stands. This fix makes
them agree on the field types; it does not merge the two construction
paths, so a future field shape could diverge again the same way. Merging
them, or deriving one from the other, is the real discharge and is a
larger change than this one.


## NUR105 — `boru check` does not analyse a callback body passed as an argument {#nur105}

**Status:** Pending — the three defect rows are fixed, one position is
not (see Discharge) · **Recorded:** 2026-08-26 · **Surfaced by:** bounding
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

**Still open, which is why this record stays Pending.** A lambda written as
a MAP-literal value is still unanalysed, where the list-literal twin is
analysed — a different route (the map's values evaluate in a sub-engine).
It is the never-called position, so nothing raises at run time, but the
record's rule is about where the body was WRITTEN, and one position still
decides the answer.
