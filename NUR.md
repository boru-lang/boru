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

> **Scope ruling (maintainer, 2026-09-17) — what this register records.**
> This register records **answer divergences only**: cases where two lanes
> disagree about a program's result. It does **not** record compile
> refusals. The two are different defects with different ledgers, and the
> distinction decides when a record is Resolved.
>
> So when a miscompile is fixed by making the shape REFUSE, the divergence
> is gone — the lanes agree — and the record is **Resolved** and deleted,
> even though the shape does not compile. That is not "resolving by
> refusing", and it does not soften the compilation contract: failure to
> compile is still a failure, and the refusal is still a defect owed a
> fix. It is tracked where compile defects belong — the refusal gates, the
> census, and the open-defect taxonomy in
> [`design/COMPILABLE-SUBSET.md`](design/COMPILABLE-SUBSET.md) §5 — not
> here. Reading a Resolved record as still-open because its shape refuses
> conflates the two ledgers and double-counts the same work.
>
> (Recorded after exactly that mistake: NUR149 was briefly reopened on the
> grounds that its shape still refuses. The reopening was withdrawn.)

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
| [NUR154](#nur154) | FIXED 2026-09-25 as a sound decline (the computed clause list — the handoff log's entry of that date): the check pass traps a non-list clause operand only when it is CONCRETE; a carrier or dynamic operand (a fn's returned list) is a list or not at run time, so the dispatch's own gate declines it and the fallback answers `'one'`; the corpus and sweep pins are retired. The original text: A `case` over a FACTORY-PRODUCED clause list miscompiles: `def mk fn [[n:Integer][List][quote [1 'one' 'many']]] end case 1 (mk 0)` is `'one'` interpreted and raises `case_error: clause list must be a concrete list of match/block pairs` compiled — silent, exit 0 until the raise, on the DEFAULT lane. The compiled `case` lowers its clause operand as a static literal; a quoted list a fn returns is a run-time value it must read, or refuse. Row `lang/spec/code-bodies.tsv:L142`; one of the five differential mismatches the expanded corpus exposed (#471) | the corpus expansion (2026-09-17); flagged by the Codex review of #471 |
| [NUR155](#nur155) | FIXED 2026-09-23 (the typed callback's contract — the handoff log's entry of that date): the VM's token seam matches a lambda-derived callback BODY unit's declared contract per element (`unmatchedLambdaBody`, eng/go/vm.go) — a list body's no-match hands back the inputs with the fn value on top, as stepping the value leaves them, a map body's raises the map arm's `no matching lambda signature`; each-variants L216 agrees and left `knownDivergences`, and the map fold `0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:"s" b:1}` (compiled `[0s1]` for the interpreter's raise) closed with it. The original text: A TYPED callback over a HETEROGENEOUS collection is applied to every element compiled where the interpreter applies it only to the matching ones: `each ([x:Integer] => [typeof x]) [1 'a' [2] {b:1} true none]` is `[Integer fn… fn… fn… fn… fn…]` interpreted (a non-matching element leaves the fn VALUE as data, no_signature swallowed) and `[Integer ProperString List Map Boolean None]` compiled. The compiled callback dispatch drops the per-element signature match the interpreter performs; it must keep it, or refuse a collection it cannot prove homogeneous. Row `lang/spec/each-variants.tsv:L215` | the corpus expansion (2026-09-17); flagged by the Codex review of #471 |
| [NUR156](#nur156) | FIXED 2026-09-22 (the quotation-body def reads — the handoff log's entry of that date): two defects, neither module-specific. The check-mode model of `apply` kept a `/v`-marked reach group's QUOTE (the runtime handler clears it), so a quoted concrete lead parked on the pass and the re-step recorded nothing — the model now delivers the value unquoted; and a bare read of a module-scope def bound to a fn-valued member is the interpreter's WORD dispatch in every unit that reads it, where the closure unit captured the VALUE — NoteWordRead names the read on the unit, the closure-body replay arms on a named def read of a tagged member, and the VM's word island dispatches through the registry's own binding. L102 and L103 agree with parity; L104 now DECLINES loudly in the dynamic-scope def family, where its local twin always did. Still open, pinned (`TestDefReadWordDispatchPending`): the same def read at the MAIN program (`5 f` is `[5 fn]`) and inside a named fn unit (a dynamic-scope-read bail). The original text: A MODULE-EXPORT fn value is not APPLIED by the compiled lane: `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end 5 M.inc/v apply` is `6` interpreted and leaves `5` and the unapplied fn compiled; `def f M.tbl.inc end each [f] [1 2 3]` returns three fn VALUES for `[2 3 4]`; and `while [i lt 3] [def i (i M.inc/v apply)]` never advances and ends in `tape_exhausted`. Rows `lang/spec/module-composition.tsv:L102–L104`. NOT closed by NUR152's home stamp (measured on ceb067c): the value carries the right home, the `apply` lowering of a module-homed fn value is what does not fire | the corpus expansion (2026-09-17); flagged by the Codex review of #471 |
| [NUR157](#nur157) | FIXED 2026-09-25 (two references to one predicate — the handoff log's entry of that date): the armed pre-pass in `unifyInner` unifies two references that resolve to the SAME predicate type as the type before either is run as a membership test, so `[:Pos] unify [:Pos]` and the fn-shape row admit under a registry as they did without one; `[:Pos]` against `[:Neg]` still fails and membership rows are unchanged. The original text: Under a registry, a signature whose parameter is a PREDICATE-TYPED container does not unify with its own text: `def Pos fnpred n:Integer [n gt 0]  def T fnsig [[xs:[:Pos]] [Boolean]]  ((fn [[xs:[:Pos]] [Boolean] [true]]) unify T)` is `~unify-fail` — the registry-armed pre-pass resolves the atom `Pos` inside one pattern and runs the predicate against the other pattern's ATOM instead of comparing two references to the same type. Unarmed (`Unify` without a registry) the pair admits | threading the unify registry through the kernel (2026-09-18, #471); found by the review's differential, kept verbatim as HEAD's verdict |
| [NUR158](#nur158) | FIXED 2026-09-25 (the closure at the wrap — the handoff log's entry of that date): the four dispatch-modifier words bridge a compiled closure to its fn definition (`ClosureAsFnDef`) before asserting the payload, so `m.a/u 10 3` over a capturing closure read out of a map is 93 on both lanes, `/s`, `/f` and `/2` alike. The original text: A capturing CLOSURE at a dispatch-modifier word's poly seat raises `illegal_ref` compiled where the interpreter wraps it: `def mk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a sub b) add k])]]  def mk2 fn [[][Map][{a:(mk 100)}]]  def m (mk2)  m.a/u 10 3` is `93` interpreted and `illegal_ref: usurp requires a function value, got Function` compiled — the VM hands the poly'd native a `ClosurePayload` its `FnDefInfo` validation rejects. The TYPED-carrier form (`def r (usurp (mk 100))`) is closed by the value-form declaration; the dynamic-Any `m.a` form stays open | the handler-migration line's fn-operand pilot (2026-09-18, design/HANDLER-MIGRATION-LINE.0.md); found by the pilot's differential probe |
| [NUR159](#nur159) | FIXED 2026-09-23 (the branch result's re-step — the handoff log's entry of that date): the recorder's branch event tells the collapse and the residual arms that a merged value MAY be a fn (`MayBeFn`, with an arg-taking flag), so the check pass notes the re-step landing on it, the lowering lands it after the merge, and the trailing / mixed arms apply an unsettled value over the values beneath it; what is left declines. Was: a NAMED fn value in a branch position is APPLIED by the interpreter and pushed as data by the compiled lane: `def one fn [[][Integer][1]] end if true one/v [2]` is `1` interpreted and `fn one` compiled; the factory (`if true (mk) [2]`) and module-export (`if true M.one [2]`) forms of the same cell agree on both lanes, the lambda and container forms fail to compile | the generated sweep (design/FULL-COMPILATION-REPLAN.0.md S0, 2026-09-18), cell `if` × named-fn |
| [NUR160](#nur160) | FIXED 2026-09-23 (the typed callback's contract): the `apply` word's pending-application arm admits a factory's baked fn CONST beside a produced closure (`producedConstLambda`), and the program's pending apply lowers over an EMPTY window too (a 0-arg closure fires, a wider one stays data) — `7 5 (mk) apply` is `[7 6]`, the named twin `[inc/v]` `[7 6]`, `(mk0) apply` 1 and `5 (mk0) apply` `[5 1]` (both silent on main); the sweep pin is retired. The original text: The apply of a FACTORY-built fn value does not fire on the compiled lane when a value sits below it on the stack: `7 def mk fn [[][Function][([n:Integer] => [n add 1])]] end 5 (mk) apply` is `[7 6]` interpreted and `[7 5 fn (Integer)]` compiled, while the clean-stack `def mk … end 5 (mk) apply` compiles with parity | the generated sweep, the prefix-stack call form of `apply` × factory |
| [NUR161](#nur161) | An `afn` whose BODY is a fn value read from a container returns the value on both lanes — `def m {f: ([n:Integer] => [n add 1])} end def f ([x:Integer] afn m.f) end f 5` is `fn (Integer)` — but with a value below on the stack the compiled lane APPLIES it to that value: `7 … f 5` is `[7 fn (Integer)]` interpreted and `[8]` compiled | the generated sweep, the prefix-stack call form of `afn` × container RESOLVED 2026-09-22 (loud compile failure) as a side effect of NUR177's fix: the residual no longer carries the container read's identity |
| [NUR162](#nur162) | FIXED 2026-09-25 (the 0-arg apply at a paren's tail — the handoff log's entry of that date): the trailing apply's window trim declines a 0-ARG callee at the recorder's one failure site (the value is data at the tail on the interpreter — `(def dbl word ([] => [1]) end 5 dbl)` is `5 fn`; the trim used to record an apply over NO argument, lowered as a native call under a signature entry carrying no signature), the lowering keeps a belt against a signature-less apply, and the disassembler names such an entry instead of crashing; the two sweep variants decline loudly now (call-form crashes 2 → 0). The original text: The compiler PANICS disassembling a program whose `word` body is a fn value once the program is wrapped in a paren group or a module body: `(def dbl word ([] => [1]) end 5 dbl)` — `disasmUnit`'s `OpCallNative` arm dereferences a nil signature entry (`compiler/go/bytecode.go:1578`); the plain form compiles with parity (`5 fn`) | the generated sweep, the paren-group and module-body call forms of `word` × lambda; `vary.Classify` now recovers a panic (`vary.Panicked`) |
| [NUR167](#nur167) | FIXED 2026-09-25 (the analysis is not a call — the handoff log's entry of that date): a fn-body analysis's type-part reservations come off with the body's bindings (`core.ForgetTypePartsSince` at `RunFnBodyOnce`'s unwind), so the run's first call is the first mint and the second conflicts as the interpreter's does; a fn unit re-installs a body-local type per call (`OpBindFnType`: the name checked against the run's reservations and reserved, the check-time node bound for the frame) instead of dropping the mint, so `f 1 f 2` raises on the second call on both lanes and the sweep's fn-wrapped type-def variants keep compiling; and a root type twin frees its reservation with the replay rollback and re-checks its name at its own position (`applyTwinPush`), so a run-time mint ahead of it conflicts where the interpreter's def does. The original text: A fn body that MINTS A TYPE, applied as a callback under compilation, conflicts one call early: `def f fn [[n:Integer][Integer][def T (class {}) n]] end each f/v [1 2]` raises `type: name part "T" conflicts with an existing type name` at element 1 interpreted (the second call re-mints) and at element 0 compiled — the check pass ran the body and its mint persists on the compiled path (RunAutoValues keeps the pass's runtime-visible installs for OpPushType), so the first real call meets its own analysis-time twin. The direct call `[(f 1) (f 2)]` refuses to compile and so hides it; the callback form compiles. Pre-existing at #474's merge base (measured on `origin/main`); S1b's lazy detached stamp declines such bodies outright (bodyHasReplayHazard) so it adds no second mint | the S1b seam pins, 2026-09-19 |
| [NUR168](#nur168) | FIXED 2026-09-25 (the def's name on the value — the handoff log's entry of that date): the recorder seats the def name on a factory's baked const lambda as it did on a produced closure, the VM's rename covers a bare fn definition, the root write-back adopts its paired dyn-scope install instead of stacking a second entry (`do [f/v]` read `fn f(String) or (String)`, the two entries unioned), and a dyn-method callee read takes the data-position lookup (`do [f 'z']` was an internal error for the interpreter's `z`); `each f/v [1 2 3]` renders `fn f(String)` three times on both lanes. The original text: A NON-CAPTURING fn value produced by a factory and def-bound loses the def's NAME on the compiled lane when it escapes as data: `def mk fn [[k:Integer][Function][([s:String] => [s])]] end def f (mk 1) end each f/v [1 2 3]` prints `[fn f(String) fn f(String) fn f(String)]` interpreted (installDef names the value it binds, and each's data fork pushes the named value) and `[fn (String) fn (String) fn (String)]` compiled — the value-def lowering (STORE_LOCAL + BIND_GLOBAL) keeps a const fn value anonymous, where a CAPTURING one is named at its PUSH_CLOSURE by the `/v` read's DefName (nameClosureValue). Render-only — the value applies the same — and the shape compiled for the first time in S1b-2 (it refused "unmatched dispatch recovered at each" before) | the S1b-2 seam rows, 2026-09-19 |
| [NUR175](#nur175) | The re-step landing applies over an EMPTY WINDOW, but `execFnDefLiteral` matches over the LIVE TAPE and the LIVE STACK, so any member with an ARG-TAKING overload can be matched differently there than at the landing. Three divergences, all present on `main` via the reach spelling and all caught by a Codex review of PR #479 before NUR174's widening could carry them to the `get` spelling: a stack operand selecting a unary overload (`5 m.f` = 6 interpreted, `5 42` landed), a `/q` slot CAPTURING the following word (`m.f z` = `z/q` interpreted, `42 z` landed — a word is not always a collection barrier), and a 0-RETURN member whose effect-only apply the landing's one-result claim cannot express (an internal_error). FIXED 2026-09-21 by screening on the RUNTIME value: only-0-arg signatures and exactly one declared return, standing aside onto the residual apply otherwise | a Codex review of PR #479, 2026-09-21 |
| [NUR176](#nur176) | FIXED 2026-09-25 (the 0-arg lead's window — the handoff log's entry of that date): a NAME-read lead whose only overloads take no argument is handed to the island in the window's WRITTEN order (a `Leading` bit on the op's head says which side of the lead the arguments were written on), the lead marked applied so an anonymous 0-arg value dispatches as the word did; `h z/v` over `[(k inc/v)]` is 8 on both lanes, `[(k 5)]` the frame's count error on both, `[(1 2 k)]` under a three-return contract `[1 2 7]`. The original text: A 0-ARG runtime lead under the one-arg leading window `(k x)` is dispatched by the interpreter with nothing and its argument is then stepped on its own — a fn VALUE applies to the lead's result (`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h z/v` is `8` interpreted), a literal survives into the frame's return check (`(k 5)` is `type_error`) — where the compiled window no-matches the lead over its one argument (`signature_error`, NUR107's raise). Loud on both lanes, never a value of its own compiled; the literal-argument rows are pre-existing (the window compiled before S1b's apply shapes; `TestLeadApplyArityMismatchParity` pins the PARAM-argument spelling `(g x)`, which the interpreter's stranded-forward barrier raises as signature_error), the fn-valued row arrived with the admission of an inert fn value at the argument position | landing S1b's apply shapes, 2026-09-22 |
| [NUR177](#nur177) | An UNDECLARED-return fn — a lambda-bodied `def w n:Integer => [...]`, whose result is its analysed body residual — called twice in one program with the same argument shape seats ONE result for both calls: `def g x:Integer => [x add 3] end def w n:Integer => [(g n)] end (w 5) (w 0)` is `[8 3]` interpreted and `[3 3]` compiled, `def w n:Integer => [if (n gt 0) [n add 3] [0]] end (w 5) (w 0)` `[8 0]` and `[0 0]` — silent, default lane, exit 0, present on `main`. AnalyseFnBody memoises the residual per arg shape and handed the SAME values to every call, and the recorder keys producers by value ID. FIXED 2026-09-22: `freshResidual` (check_fnbody.go) re-mints each call's result identities at the record; the declared-return path always minted its own, which is why the corpus's `fn [[…] [T] …]` spelling never saw it | S1b's apply shapes graduating a literal-read pin, 2026-09-22 |
| [NUR178](#nur178) | A paren over a PRODUCED closure — a compiled factory call's result re-stepped by the paren's rewind, `((mk 1) 2)` — left its window on the tape for a LATER word to collect: `((mk 1) 2) mul 10` compiled 21 (mul over 2 and 10, the closure over the product) for the interpreter's 30, `((mk 1) 2.5) mul 2` 6.0 for 7.0 — silent, default lane, exit 0, present on `main`; `add` agreed by arithmetic. The NUR121 hazard mark WAS set, and `hazardLead` exempted the lead on the INNER paren's placed mark whatever the outer paren had done. FIXED 2026-09-22 (the curried chain): the collapse records the re-stepped produced lead's apply as an event (core `parenProducedLeadApplyIdx` over the `ProducedLeadApplies` seam), and a collection made AFTER the re-stepping paren closed is a leak the hazard declines (`hazardAfterReStep`); the leading form's no-match order (`((mk 1) "s")` compiled `[s fn]` for `[fn s]`) is closed in the same landing | the curried chain, callbacks.tsv L151, 2026-09-22 |
| [NUR179](#nur179) | Every fn-value-call op that hands a compiled fn-VALUE closure its window bound a TWO-param closure's first param to the value farthest from it: `(2 3 (mk2 1))` compiled 24 for the interpreter's 33, `7 5 (mk2 1)/v apply` 76 for 58, `1 2 (kk 7) apply` 19 for 28, a Function param's `(2 3 g)` 24 for 33, a capturing closure MEMBER's `(m.g 2 3)` 33 for 24, and the residual arm's `((mk2 1) 2 3)` 33 for 24 — silent, default lane, present on `main`. The ops build their window POSITIONALLY (args[0] → the first param) and handed it to the token seam, whose fn-value arm (S1b-2, `invokeFnValueClosure`) takes STACK order and reverses it itself; every pin was a one-argument window or a commutative body. FIXED 2026-09-22: `invokeClosurePositional` (eng/go/vm.go) re-stacks a positional window for a fn-value closure | the curried chain's two-argument probes, 2026-09-22 |
| [NUR180](#nur180) | FIXED 2026-09-23 for the mechanism named here (the checker's recovery lays its window out forward-first, as the interpreter's matcher does — `checkModeFallbackPositionsFor`); the fold row belongs to NUR184. The original text: A trailing paren apply whose result the recorder can only type Any — an event lead (a factory's closure), an anonymous lambda (a count contract) — inside an UNNAMED-param frame (an each or fold body, `fn [[Integer] …]`), consumed by a typed word: `xs each [(2 (mk 1)) mul 10]` compiles `[10 10 10]` for `[30 30 30]`, `0 fold [add (2 (mk 1))] xs` 3 for 6 — silent, present on `main`. The checker's recovery re-matches the word over the frame's gradual input (`mul/2 (poly)` over the element and the result, the written 10 pushed after) instead of the written argument. MITIGATED 2026-09-22: a NAMED concrete lead's declared return types the result (`concreteFnSingleReturn`), so `(2 inc2/v) mul 10` in an each body is 40 on both lanes; the curried-chain arm stands aside in unnamed frames. The Any-result rows stay open, pinned to fail when they agree | the curried chain's each-body probes, 2026-09-22 |
| [NUR181](#nur181) | FIXED 2026-09-23 (the def-bound closure park — the handoff log's entry of that date): a call result is parked where it lands on the compiled lane too, whatever route the call took — the recorder resolves a shaped method call's callee unit from the method value's own producer and a fn-value apply's from its closure operand (`callAppliedClosureUnit`), `callResultPlaced` reads both, and the program residual's ordering treats a parked result as data; `2 r 3`, `(r 3)`, `7 (r 3) 2`, `r 3 4`, `2 ((mk3 1) 3)` and `5 (mk 3)` (which declined) all agree. The original text: A def-bound factory closure applied through the READ model nets a closure the re-step landing applies where the interpreter parks it: `def mk3 … def r ((mk3 1) 2) end r 3` — the def's pending collection takes the paren's survivors as ARGUMENTS, so r is mk3's closure and 2 stays on the stack — is `[2 fn (Integer)]` interpreted (the call result is placed) and 6 compiled (`CALL_DYN_METHOD r/1; RESTEP_LANDING` applies the closure to the 2 beneath) — silent, present on `main`, measured on a worktree at 917ecf0. Not this landing's: recorded and pinned to fail when it moves | probing the def-bound chain, 2026-09-22 |
| [NUR182](#nur182) | The whole-frame replay (OpCallDynFrame) re-stepped a paren-PLACED value: a fn unit whose body left `(ops.inc)` — a member read the paren placed — above its input compiled the APPLY, `def f fn [[Integer][Any][(ops.inc)]]  f 5` answering 6 for the interpreter's `fn (Integer)`, and `[[Integer][Integer][(ops.inc)]]`, `[5 (ops.inc)]`, `[(ops.inc) 5]`, `[x (ops.inc)]` answering 6 where the interpreter raises its return-count / return-type error — silent, default lane, present on `main`. A fn frame never re-steps a placed value (the re-step of a native's fn result is the CALLER's, NUR124), but noteDynFrameReplay counted it as the window's applicable and the replay island re-steps every token it is handed. FIXED 2026-09-22 (the quotation-body container reads): a placed value no enclosing paren re-stepped is data — skipped as an applicable, a blocker for any window beside one, re-pushed by the promotion; a `do` body's placed value with siblings stays declined, its caller re-steps it | the quotation-body container reads' probes, 2026-09-22 |
| [NUR183](#nur183) | A fn-valued container member read as the LAST token of a code body over a DUPLICATED element compiled the member as data: `def ops {inc: (fn [[n:Integer][Integer][n add 1]])}  each [dup ops.inc] [1 2 3]` is `[2 3 4]` interpreted (the reach collapse re-steps the member over the copy) and `[fn fn fn]` compiled — silent, default lane, present on `main`. The same root as each-variants L205 / fold-map-filter L215 / module-composition L98, which DECLINED ("result above a literal"): a code-body closure unit took no whole-frame replay, and with two values beneath the member the layout seated in order and the member rode as data. FIXED 2026-09-22 (noteClosureBodyReplay: a tagged fn-member read at a code body's tail arms the replay a fn unit already took) | the quotation-body container reads, 2026-09-22 |
| [NUR184](#nur184) | FIXED 2026-09-23 (the trailing value's re-step — the handoff log's entry of that date): the collapse records the trailing apply only for a lead the interpreter dispatches INSIDE the paren (a bare read of a fn-typed binding, an `apply`-owned lead) or a fn value nothing after the close can collect; a value with a collectable follower is left for the rewind (every fn-valued survivor is marked re-stepped), a paren under a pending forward marks its trailing value as that collection's LEFTOVER (never applied over later values), and the mixed island's interpreter SEALS a compiled closure at its collection's completion as it seals a FnDefInfo. `(2 (mk 1)) 10`, `(2 (mk 1)) 10 20`, `(2 inc2/v) 10 mul`, `xs each [(2 (mk 1)) 10]`, the fold row and the fn-unit `10 mul (2 (mk 1))` agree; `10 mul (2 (mk 1))` at the main program, `def r (2 (mk 1)) end r` and `[(2 (mk 1)) 10]` DECLINE; `(2 (mk 1)) 10 mul` and `(2 (mk 1)) 10 add` DECLINE too (a native poly record that collected the re-step-marked carrier the interpreter dispatches first: `polyCallDeclineReason`, the record's one compile-failure site). The original text: A paren's TRAILING fn value is dispatched like a word AFTER the paren — it forward-collects the tokens past the `)` first and takes the values inside the paren only when nothing collectable follows — and a paren closing UNDER A PENDING FORWARD hands its survivors to that forward, the fn value re-stepping over the forward's result: `def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]]  (2 (mk 1)) 10` is `[2 11]` interpreted and `[3 10]` compiled, `(2 (mk 1)) 10 mul` 22 for 30, `(2 inc2/v) 10` `[2 12]` for `[4 10]`, `10 mul (2 (mk 1))` 21 for 30, `0 fold [add (2 (mk 1))] xs` 6 for 3, `def r (2 (mk 1)) end r` `[fn 2]` for 3; `(2 (mk 1))`, `(2 (mk 1)) "s"`, `(2 (mk 1)) mul 10` and the LEADING form `((mk 1) 2) 10` agree. The compiled trailing-apply record (`recordParenTrailingFnApply`) applies the value over the values INSIDE the paren, right only for the no-follower case. Pinned by `TestParenTrailingFnForwardCollectsPending` (lang/go) | closing NUR180, 2026-09-23 |
| [NUR185](#nur185) | A `/v` READ of a def-bound closure re-stepped by the residual's mixed-window island: `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def c (mk 3) end 2 c/v 10` is `[2 fn c(Integer) 10]` interpreted (the value spelling delivers the closure inert) and `[2 30]` compiled, `c/v 10` `[fn c(Integer) 10]` for `[30]` — silent, default lane, present on `main` (a worktree at 888f234). The fn-carrier side table's `/v` read noted the def read and the local read but not the VALUE read, so `callResultPlaced` took it for the bare read's word dispatch and the mixed arm islanded the window live. FIXED 2026-09-23 (the trailing value's re-step): the carrier branch notes `NoteValRead` as the Defs path does, `callResultPlaced` treats a `/v`-only delivery as the parked result it is (`placedValRead`), and the rows decline at the render gate (the interpreter names the value after the def). Pinned in `def_bound_closure_park_test.go` | probing NUR184's neighbours, 2026-09-23 |
| [NUR186](#nur186) | FIXED 2026-09-23 (the named fn value's candidates — the handoff log's entry of that date): the landing note carries what follows the value and whether values sit beneath it, the landing op raises the interpreter's `uncalled_function` for a named fn over a candidate and an empty frame, and a concrete named fn at a fn or lambda frame's tail declines the unit (no unit-level trap yet). Was: A MODULE fn's value at a factory body's TAIL with nothing beneath it — `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end def mk fn [[][Function][M.inc]] end 5 (mk) apply` — raises `uncalled_function: call to 'inc' matched no signature` interpreted (the reach group's lone survivor is a NAMED fn at the pointer, and a name always calls: ADR-011) and answers 6 compiled (the unit returns the value, the apply then applies it); `7 5 (mk) apply` `[7 6]` for the same raise — silent, default lane, present on `main` (a worktree at 3fa1212). The local `[inc/v]` twin agrees (`/v` is inert on both lanes). Pinned pending in `TestModuleFnAtFactoryTailPending` (lang/go) | probing NUR160's neighbours, 2026-09-23 |
| [NUR187](#nur187) | FIXED 2026-09-23 (the branch result's re-step — the handoff log's entry of that date), found the same day: the residual's fn-value apply arms carried a value's collection across a STATEMENT BOUNDARY — `def m {f: inc/v} 7 m.f ; 3` islanded the window to `[7 4]` for the interpreter's `[8 3]`, `m.f ; 5` applied the member to the next statement's 5 (6 for `[fn inc 5]`) — silent, present on `main`. The pass notes every boundary's position (`NoteStatementEnd`) and no arm applies a value over an entry written past a boundary that follows it (`crossesBoundary`); an interior value past a boundary declines | probing NUR159's neighbours, 2026-09-23 |
| [NUR188](#nur188) | FIXED 2026-09-23 (the named fn value's candidates — the handoff log's entry of that date), found the same day: a native poly that collected a container MEMBER's fn value written BEFORE the word handed the word the fn where the interpreter re-steps the member first — `def m {f: M.inc} 7 m.f typeof` was `[7 Function]` for the interpreter's Integer, `m.f typeof` Function for its `uncalled_function` raise — silent, present on `main`. The poly declines a member read written before the word (`polyCallDeclineReason`); a read written after it (`typeof m.f`) stays its operand | probing NUR186's neighbours, 2026-09-23 |
| [NUR189](#nur189) | FIXED 2026-09-23 (the named fn value's candidates — the handoff log's entry of that date), found the same day: the residual's TRAILING arms applied a paren-PLACED member fn value — `def m {f: M.inc} 7 (m.f)` was 8 for the interpreter's `[7 fn inc]`, `1 7 (m.f)` `[1 8]` for `[1 7 fn]` — silent, present on `main`. The trailing, trailing-window and mixed arms ask the park (`placedNotReStepped`) as the lead arm always did | probing NUR186's neighbours, 2026-09-23 |
| [NUR190](#nur190) | FIXED 2026-09-26 (the landing's island — the handoff log's entry of that date): a `/q` claim at the landing's walk is exact on the VM — where the word is in the body at the landing's own depth (the program, a user paren, a fn or closure body) the value and the body from the word on go to the interpreter and the run continues at the unit's RET or the program's end; inside a branch arm, a loop body or a literal's member the word's call is followed at once by the paren apply over it, and the capture runs over the value and the word alone, skipping both; fn-value.tsv L317/L318 left the runtime-defers ledger. Found on the way and fixed: NUR222 (a dyn body's settled lead re-applied by the residual arm). Before: CONTAINED 2026-09-24 (NUR190's open halves deferred — the handoff log's entry of that date; the maintainer's call; the Function-typed half dissolved with NUR078, 2026-09-26 — `m.g z` is the named no-match on both lanes, `m.g z/v` is 7 on both): the `/q` capture and the Function-typed reference DEFER loudly at the landing's walk (`vm:landing-quote-claim`, `vm:landing-claim`) and the corpus keeps them on the runtime-defers ledger (runtime_defers.tsv: fn-value.tsv L317/L318). PARTLY FIXED 2026-09-23 (the landing's overload walk): a DYNAMIC fn value under a FUNCTION word is re-stepped by the interpreter over that word, and the compiled landing stood aside for any arg-taking overload (NUR175) while the residual apply took the word's RESULT — `m.f z` (h with a nullary and a unary overload, z a 0-arg fn) was 1 for `[42 0]`, `m.f typeof` Function for Integer, `m.l z` (an anonymous unary) 1 for `[fn lam(Integer) 0]`, `m.a z` (an Any-typed slot) a false `uncalled_function` for the strict barrier's stranded `signature_error`. The landing walks the run-time fn's overloads over the word with the interpreter's own plan (the word rides in the bytecode, `LandingWords`), and those four are closed. The `/q` capture (`m.q z` was `[42 0]` for `[z]`, `m.f y` `[42 42]` for `[y]`, silent; fn-value.tsv's L317/L318 passed by coincidence) and the Function-typed reference (`m.g z`, 7 interpreted) need the word's compiled call skipped, which the lowering cannot do, and no static model tells a `/q` slot from a typed slot's barrier — so a compile-time decline would be over-wide, and the deferral at the walk is precise. |
| [NUR191](#nur191) | FIXED 2026-09-25 (the module fn's return contract — the handoff log's entry of that date): a named fn call through the CallBoru seam enforces the frame's return COUNT before the types (`CallBoruStrict` / `NamedFnReturnCount`, on execFnDefLiteral's cross-registry arm and buildFnBodyHandler's foreign-registry arm; the callback seams keep the count trimmed), and a user fn's single returned closure delivered as a handler result is PARKED where it lands as the frame's return is (execMatch, after the splice) — `M.d1 10` over `[(mk x) 3]` raises the frame's count error on both lanes and `3 M.d1 10` over a factory is `[3 fn (Integer)]` everywhere. NUR210 found on the way, recorded pending: a module fn returning a NAMED fn value, read through its reach group with a value beneath (`5 M.ff`), which the interpreter re-steps. The original text: A MODULE fn's body re-steps a parked closure over the token after it where a main-registry fn body parks it, and the compiled module fn parks: `import module [def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] def d1 (fn [[x:Integer][Integer][(mk x) 3]]) export "M" {d1: d1/v}] M.d1 10` is 13 interpreted (CallBoru's deferred residual sweep re-steps the parked closure over the 3) and `type_error: d1: expected 1 return value(s), got 2 — [fn (Integer) 3]` compiled — the answer the same body gives in the main registry on BOTH lanes (`def d1 (fn […]) d1 10`). A curried chain (`TestModuleFnStampedAtLoadAndRerouted`'s decliner) the same: 16 interpreted, the count error compiled. Recorded 2026-09-23, present on `main` (c268afb); an error-versus-value divergence, not silent. |
| [NUR192](#nur192) | FIXED 2026-09-24 (the frame's closure bind — the handoff log's entry of that date), found the same day: a fn-body-LOCAL computed fn def read by a native's raw code body — `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def g fn [[xs:List][List][def a5 (mk 5)  each [a5] xs]] end g [1 2 3]` was `[[6 7 8]]` interpreted and `undefined_word: a5` compiled, `each [a5 add 1] xs` the same — because the frame's dynamic-scope bind installed nothing for a closure value (`installDef`'s carrier guard). The VM's bind pushes the closure as the top-level write-back does and the frame's trail pops it; the tail read compiles natively, the raw-body read islands to the pushed closure. Found on the way and DECLINED loudly: a fn body's computed fn def SHADOWING an enclosing frame's computed fn of the same name outlives the call in the interpreter (its install drops the overlapping outer closure at the same depth, so the def-cleanup pops nothing), which the compiled push-and-pop cannot model — `def a5 (mk 1) … g [1 2 3] each [a5] [1 2 3]` is `[[6 7 8] [6 7 8]]` interpreted, and was `[[2 3 4] [2 3 4]]` compiled on main. The `do [a5 7]` row moved to NUR193. |
| [NUR193](#nur193) | FIXED 2026-09-24 (the do body's read — the handoff log's entry of that date), found the same day: a def-bound computed fn read inside a `do` body — `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def a5 (mk 5) end 7 do [a5]` was `[12]` compiled for the interpreter's `[7 error(cannot call `a5` …)]` (the check pass modelled the do's result as the fn carrier and the program residual applied it over the 7; the island stepped the def-bound compiled closure as anonymous DATA and parked it over the empty frame where the interpreter's fn definition raises), `do [a5 7]` a `CALL_DYNAMIC underflow`. A closure a compiled program binds by `def` is now bridged into the word dispatch under its name (core `lookupUncachedBridged`, `dispatchesAsWord`; the aggregate never cached) and `do`'s result model takes the computed-body hatch when the body leaves such a carrier. The written operand the contract does not take is NUR194. |
| [NUR194](#nur194) | FIXED 2026-09-24 (the written argument's fit — the handoff log's entry of that date), found the same day: a def-bound computed fn read with a WRITTEN operand its contract does not take — `def a5 (mk 5) end each [a5 "s" add] [1 2]` is `['6s' '7s']` interpreted (the matcher falls back from the written token to the frame's element) and bailed compiled on the shaped read's claim (`result count 2 violates the host-registered shape claim 1`); `do [a5 "s"]` the caught `cannot call` interpreted, the same bail (`[fn a5(Integer) s]` on main before the do body's read, silent). The shape claim carries the wrapper's parameter types now (`FnShape.Params`, from the closure unit's declared params or a const lambda's signature) and the read window declines a written token that does not conform; the body islands and the island dispatches the word as the interpreter does, and the top-level `(a5 "s")` declines the program to the interpreter's own raise. |
| [NUR195](#nur195) | FIXED 2026-09-24 (the escaped flow after a native call — the handoff log's entry of that date), found the same day: the flag a native's body run leaves set (a break/continue with no loop of its own, handed back by the seam's sub-engine) was read by the VM only after a fallback or a fn-value apply, never after a plain native call, so `each (mk) [1 2 3]` over a computed `[break]` answered `[[1 2 3]]` silently and `for 3 [each (mk) xs i] 99` ran all three iterations; the flag is read after every native call now — the enclosing loop ends or steps on (`[99]` on both lanes), and with no loop at all the compiled lane takes the loop-less flow's designed path, the internal error RunCompiled's callers defer to the interpreter's `break outside loop` on (a loud bail, never a value). The original text: A flow sentinel inside a COMPUTED code body under `each` — `def mk fn [[][List][quote [break]]] end each (mk) [1 2 3]` — is the interpreter's `flow_error: break outside loop`, and the compiled lane answers `[[1 2 3]]`, silently; `continue` the same. The LITERAL twin `each [break] [1 2 3]` fails to compile (the Stage-2 code-body gate) and falls back with parity. Present on `main` (d75dd75) before the run-time token-body stamp, which declines a sentinel body and leaves the seam as it was. Pinned as it stands in lang `TestComputedBodyFlowSentinelPending` | found while pinning S3's first slice (2026-09-24) |
| [NUR196](#nur196) | FIXED 2026-09-24 (the escaped body ends the iteration — the handoff log's entry of that date), found the same day: an iterating native — each, fold, scan, filter, for-each, outer, inner, eachrank — read the residual of a body run that had ESCAPED (a break/continue left unresolved, the registry's flag set) as the element's result: the interpreter's sub-engine hands back the unstepped tape (a frame-cleanup marker, which `filter` reported as "must produce a Boolean, got __CP"), the VM's island nothing (which `each` reported as "body produced no result"). The natives end their iteration on an escaped body now (core.BodyEscaped) and return no result, so the run resolves the flag on both lanes — `for 3 [each [f] [1 2 3] i] 99` is `[99]` on both, and with no loop the interpreter raises where the compiled lane takes the loop-less deferral. The original text: A `break` raised by a fn CALLED from a LITERAL each body — `def f fn [[x:Integer] [Integer] [break]] end each [f] [1 2 3]` — is the interpreter's `flow_error: break outside loop`, and `for 3 [each [f] [1 2 3] i] 99` its `[99]`; the compiled lane raises each's own `each_error: body produced no result` on both — the body's unit (or the island the declined fn takes) returns nothing on the escape, and the handler complains before the flag can be read. Loud, but a different answer. Pinned as it stands in lang `TestLiteralBodyFlowThroughFnPending` | found while closing NUR195 (2026-09-24) |
| [NUR197](#nur197) | FIXED 2026-09-25 (the loop region's residual — the handoff log's entry of that date): the interpreter's loop region evaluates a pending residual container when it collects the iteration's output, with the iterator still bound (`Engine.collectLoopRegion`, for `for` and `while` alike), as a fn frame evaluates its body's residual in-frame; `for 2 [[(i add 1)] i]` is `[[1] 0 [2] 1]` on both lanes, `def i 9  for 2 [[(i add 1)] i]` reads the loop's own `i`. Two twins found and closed with it: NUR208 (the loop's iterator surviving a trapped raise) and NUR209 (the single-literal `do` body under a loop baked as a const on the compiled lane). The original text: A loop body's residual LITERAL bearing a paren group over the loop variable — `for 2 [[(i add 1)] i]`, `for 2 [{a:(flex [i])} i]` — is the interpreter's `undefined word: i` (the residual literal is evaluated at the end of the run, after the loop unbound `i`) and the compiled lane's value per iteration (`[[1] 0 [2] 1]`); inside a fn the interpreter raises the same where the compiled lane raises the fn's return-count error. Present on main at 6ccf827; found while closing the flex-member map literal. Fence: `TestLoopBodyResidualLiteralPending`. |
| [NUR198](#nur198) | FIXED 2026-09-25 (the None receiver's bare-word key — the handoff log's entry of that date): the accessor's `[Key | None]` chained-read row carried no QuoteArgs, so a BARE-WORD key over a run-time None was never collected — the dispatch parked and the word stepped on its own as `undefined word: c`, where a string or materialised-atom key always propagated None; the atom row quotes the key now on `dot` and on `dotr` (whose bare-word read over None raises the family's not_found on both lanes), and `m.b.c` over `{a:1}` is None everywhere, a static None included. The original text: A dotted read through a MISSING member — `def f fn [[m:Map][Any][m.b.c]] end f {a:1}`, `((make C {}).x get 0).y` — is the interpreter's `undefined word: c` (its `dot` over a run-time None leaves the atom unconsumed, which steps as a word) and the compiled lane's None (the chain's second hop over the first's result); a STATIC None fails to compile at the same undefined_word, consistently. Present on main at 6ccf827; found while closing the flex-member map literal. Fence: `TestDotChainOnMissingMemberPending`. |
| [NUR199](#nur199) | FIXED 2026-09-24 (the keep-defs body — the handoff log's entry of that date), found the same day: a `do` body's closure unit is a keep-defs unit — every value def installs through a kept OpBindDynScope the VM leaves standing past the unit's RET for the enclosing frame to pop (CompiledFn.KeepsDefs), the enclosing loop refreshes its carried slot from the registry after the call, a leaked name's later read seats live at its token, and the adopted twin is marked written back; `def t 0  for 3 [do [def t 5]]  t` is 5 on both lanes, `for 3 [do [def t (t add 1)]]` compiles (code-bodies.tsv 13 -> 11). The original text: A `do` body's def of a name the enclosing `for` loop CARRIES — `def t 0 end for 3 [do [def t 5]] end t`, `for 3 [do [def t (i add 1)]]` — is the interpreter's 5 / 3 (the body runs in the caller's frame and its def leaks) and the compiled lane's 0: the do$body unit kept the def frame-local and the loop's carried slot never saw it; the computed twin `for 3 [do [def t (t add 1)]]` declined instead ("dynamic-scope def `t` of unpromoted computed value"). Present on main at 3768c46. Fence: `TestDoBodyDefLeaksToTheEnclosingScope`. | found while surveying the corpus's compile failures (2026-09-24) |
| [NUR200](#nur200) | FIXED 2026-09-24 (the multi-run keep-defs body — the handoff log's entry of that date), found the same day: a multi-run body's unpaired value defs install through the kept OpBindDynScope (a loop fragment, a fn body), the enclosing loop refreshes its carried slot after the call, and every read of a leaked name seats live — the last element's install, or the interpreter's undefined_word at zero iterations; the twin regime's read fence is retired for value names. `def t 0  for 3 [[1] each [def t 5] drop]  t` is 5 on both lanes; code-bodies.tsv L183/L190 and module-composition L92 compile (the mutated flex read back seats live too). The original text: An `each` body's def of a name the enclosing `for` loop CARRIES — `def t 0 end for 3 [[1] each [def t 5] drop] end t` — is the interpreter's 5 (the body leaks its def per element) and the compiled lane's 0: no arm-resident twin is adopted inside a loop fragment (the bridge's root fence), so nothing installs the binding at run time and the carried slot keeps the pre-loop value; NUR199's multi-run twin. Present on main at 3768c46; found while closing NUR199. Fence: `TestEachBodyDefInLoopResolves`. | found while closing NUR199 (2026-09-24) |
| [NUR201](#nur201) | FIXED 2026-09-25 (the frame's error path — the handoff log's entry of that date): the interpreter's fault return tears down every fn frame the error leaves OPEN on the tape (`Engine.faultReturn` → `unwindLiveFrames`, core/go/engine.go) — the body-local defs truncated, the Args list and FnBaseline popped, the captures and params undef'd — exactly as a break/continue discarding the region unwinds them and as CallBoru's sub-run tears its frame down inline on the same error; `do [g]  t` is `[error(x) 0]` on both lanes, the callee's params and args list gone with its locals, and the frame that raised while evaluating its residual in-frame unwinds there once instead of at the marker as well. The original text: A callee's def before a raise the caller TRAPS — `def g fn [[][Integer][def t 9 raise 'x']] end do [g] end t` — is the interpreter's `[error(x) 9]` (a raise skips the frame's def-cleanup tail, so the callee's binding leaks) and the compiled lane's `[error(x) 0]` (the frame installs nothing a trap could keep; an installed one is unwound with the frame on the error path). The interpreter's is the questionable rule. Present on main at 3768c46; found while closing NUR199. Fence: `TestCalleeDefTornDownOnTrappedRaise` (lang keep_defs_leak_test.go), `TestRunErrorUnwindsLiveFrame` / `TestRunErrorUnwindsFrameOnceAfterResidualError` (core fn_frame_unwind_test.go), fn-locals-scope.tsv §12 |
| [NUR202](#nur202) | FIXED 2026-09-24 (the keep-defs token body — the handoff log's entry of that date), found the same day: the closure unit compiles (the unapplied-fn gate exempts a unit's own untouched inputs — an argument enters the frame resolved and is never stepped), and the run-time stamp of a token body is a keep-defs unit whose kept installs the host hands to the enclosing context's trail, its own defs left out of the stamp's dependency snapshot; `def f fn [[xs:List][List][def t 0 each [def t (t add 1) t] xs]]  f [1 2 3]` is [[1 2 3]] on both lanes and code-bodies.tsv L189 compiles. The original text: A keep-defs token body over a GRADUAL list inside a fn — `def f fn [[xs:List][List][def t 0 each [def t (t add 1) t] xs]] end f [1 2 3]` — is the interpreter's `[[1 2 3]]` (eachHandler drives InvokeBody per element on the shared registry, so the def leaks to the next element) and the compiled lane's `[[1 1 1]]`: the body lowers as a CONST the native runs (PUSH_CONST_FRESH, the S1a dynamic-callback path) and that run reads 0 at every element. Over a literal list the body is a closure unit and agrees; the Integer-returning twin (code-bodies.tsv L189) declines loudly. Present on main at 3768c46. Fence: `TestKeepDefsConstBodyOverGradualListPending`. |
| [NUR203](#nur203) | FIXED 2026-09-25 (the dynamic body's leak — the handoff log's entry of that date): A keep-defs word over a DYNAMIC body inside a fn — `def f fn [[b:List xs:List][Integer][def t 0 each b xs drop t]] end f (quote [def t (t add 1) t]) [1 2 3]` — is the interpreter's 3 (the body leaks its def per element into the fn's frame) and the compiled lane's 0: the run-time stamp installs the leak (NUR202's close), but the compile pass never sees the body's tokens, so the fn's later read of `t` keeps its compile-time home instead of seating live. The root twin agrees. Present on main at 3768c46. Fence: `TestDynamicKeepDefsBodyLeakInFnPending`. |
| [NUR204](#nur204) | FIXED 2026-09-25 (the lexical index scope — the handoff log's entry of that date): A body def of the for loop's OWN index — `def i 0 end for 3 [def i 9] end i` — is the interpreter's 2 (the loop leaves its index level bound past the loop: the last index; 0 inside a nested loop, whose outer cleanup pops it) and was the compiled lane's 9 on main (the loop carried the def and wrote the body's value back), for a native or a user-call value alike, inside a fn and through an arm too; neither is the pre-loop 0 a lexical loop scope would give. The compiled lane DECLINES the shape loudly now (the for's index name rides RecordLoop into the loop event). Present on main at 3768c46; found while landing the user-call write-back. Fence: `TestForIndexDefInBodyPending`. |
| [NUR205](#nur205) | FIXED 2026-09-25 (the module replay's own bind — the handoff log's entry of that date): a loop whose body binds a MODULE declines — its join's twin is noted with its placement withheld, so the twin regime's full-placement gate declines — where the compiled lane's one replay of the check pass's bind (the join's twin, placed at the loop) is not the interpreter's bind — when the loop may run zero times (the name stays unbound on the interpreter), and when the body's import mints a NEW instance per iteration (an inline `import module […]`; the two analysis rounds' namespaces share no export map) — and an `if` arm that may not run withholds a module bind's twin the same way (installArmJoins) (`if c [import …] [] M.a` raised `undefined_word` interpreted and answered compiled); a cached `boru:` import in a loop that provably runs, and the arm a decided condition takes, keep their one replay and agree. The original text: An inline `import module […]` inside a LOOP body runs its module body ONCE on the compiled lane where the interpreter re-imports it per iteration, so module STATE carries across iterations: `for 2 [import module [def acc (flex []) export "M" {acc: acc}] end M.acc push 1 end size M.acc]` is the interpreter's `[[1] 1 [1] 1]` (a fresh `acc` each time) and the compiled lane's `[[1 1] 1 [1 1] 2]` — silent, default lane, present on main at 00ec530 with no callback involved (the import is a compile-time word whose body the check pass ran once and whose bindings the loop body replays). Found by the generated sweep when the `for-each` and `walk` × module-export seeds graduated (their module carries `acc`): their for-body variants diverge the same way (`[3 6]` for `[3 3]`; `… 5 … 10` for `… 5 … 5`) and are pinned in `sweepKnownMiscompiles`. The `each` twin (`each M.stp [1 2 3]` in the same loop) diverges identically and always has; its sweep seed carries no state, so the matrix never saw it. | the generated sweep, closing the fn-value callback cells (2026-09-25) |
| [NUR206](#nur206) | FIXED 2026-09-26 (the merge of main's #508 with the reverse-order NUR run — the handoff log's entry of that date): closed by the run's NUR208, the same defect found independently — the fault path unwinds every live loop (`Engine.unwindLiveLoops`), so a caught error leaves the registry as the loop found it and all three witnesses are 99 on both lanes. The original text: A `for` loop's INDEX SURVIVES an error the enclosing `do` catches on the interpreter, for the rest of the program, where the compiled lane reads the outer binding: `def i 99 end do [for 3 [raise oops 'x']] error [drop] end i` is the interpreter's `0` (the raise unwinds the spliced body before its move cleanup, so the loop's index level stays installed over the outer `i`) and the compiled lane's `99`; the same with the handler reading `i`, and with a computed body (`for 3 (mk)` over `[raise oops 'x']`). Silent, default lane, present on main at 9e02915 — the compiled `do` body traps or islands the loop and the handler reads `i` from its compiled slot. The direction is the interpreter: an error unwind out of a spliced loop body should run the frame's cleanup as break/continue's `unwindLiveFrames` does, not leave the index bound. Pinned `TestLoopIndexSurvivesCaughtErrorPending` (lang) | the Codex review of #508 (2026-09-25), measuring the withdrawn hosted for body; the literal-body twin found on the follow-up |
| [NUR208](#nur208) | FIXED 2026-09-25 (the loop region's residual — the handoff log's entry of that date), found the same day closing NUR197: a `for` loop abandoned by a raise the caller TRAPS left its ITERATOR installed on the interpreter — `def i 9  do [for 2 [raise 'x']]  i` read 0, `for 3 [if (i eq 1) [raise 'x'] []]` under the same trap read 1 — where the compiled lane's loop keeps `i` in a frame slot and the read answers the root's 9; silent, present on main. NUR201's loop twin: the fault return unwinds every live loop's iterator (`Engine.unwindLiveLoops`) before the frames, as a break's region discard does | probing NUR197's neighbours, 2026-09-25 |
| [NUR209](#nur209) | FIXED 2026-09-25 (the loop region's residual — the handoff log's entry of that date), found the same day closing NUR197: a `do` body that is ONE container literal over the loop variable — `for 2 [do [[i]]]`, `for 2 [do [{a:i}]]`, `for 2 [do [[(i add 1)]] i]` — answered `error(undefined word: i)` per iteration on the compiled lane for the interpreter's `[0] [1]`, silent, exit 0, present on main: the token body was analysed as a DEFERRING lambda (bodyInFrame false), its residual recorded no assembly, the closure declined on the unknown provenance and the dyn-body backstop baked the literal as a const the handler re-ran through the interpreter, where the loop's `i` is a frame slot the registry never held; a multi-token body (`do [[i] 5]`) compiled. A token body compiles in-frame now (recordClosureDispatch's bodyInFrame true — the InvokeBody seam's sub-engine sweeps the residual at its end, with the bindings live), and the closure assembles the list from the captured slot | probing NUR197's neighbours, 2026-09-25 |
| [NUR210](#nur210) | FIXED 2026-09-25 (the reach group's survivor — the handoff log's entry of that date): A module fn returning a NAMED fn value, read through its reach group with a value beneath — `import module [def ff fn [[][Function][inc/v]] def inc fn [[n:Integer][Integer][n add 1]] export "M" {ff: ff/v}] end 5 M.ff` — is 6 on the interpreter (the reach group `( M dot ff )` never parks, its collapse re-steps the lone survivor, a NAMED fn at the pointer, and a name always calls: ADR-011) and `[5 fn inc(Integer)]` on the compiled lane, which seats the returned value as data; `M.ff 5` the same. The main-registry twin `5 ff` parks on both lanes, and so does `5 (M.ff)`. Present on main; found closing NUR191. Fence: `TestModuleFnNamedValueThroughReachPending` | probing NUR191's neighbours, 2026-09-25 |
| [NUR211](#nur211) | FIXED 2026-09-25 (the named value's no-match on the seam — the handoff log's entry of that date): the token seam's unmatched-lambda arm (`unmatchedLambdaBody`) raises the word's `uncalled_function` for a closure that carries a def's name (`ClosurePayload.RetName`) and keeps the anonymous value's data rule otherwise; `0 fold h/v [1 2]` raises at step 1 on both lanes. The original text: A NAMED fn value driving `fold` whose signature stops matching PAST THE FIRST STEP is parked as data on the compiled lane where the interpreter raises `uncalled_function`: `def h fn [[a:Integer b:Integer] [List] [[a b]]] end 0 fold h/v [1 2]` — step 0 answers `[0 1]`, so step 1 offers a List accumulator to `a:Integer` and no signature matches — is `fold: step 1: [boru/uncalled_function]: call to 'h' matched no signature` interpreted and `[fn (Integer, Integer)]` compiled (the value itself, as the closure-body data fork leaves an unmatched TYPED LAMBDA — NUR155's rule for an anonymous value, applied to a NAMED one). A no-match at step 0 raises on both lanes; `scan` over a no-match parks on both lanes. Pre-existing at the merge base (measured 2026-09-25 on `wt-head`); silent — a value where the interpreter raises | closing NUR166, 2026-09-25 |
| [NUR212](#nur212) | FIXED 2026-09-25 (the marker is no argument — the handoff log's entry of that date): the forward claim probe (`ForwardClaimProbeOn`) answers no claim for a dispatch-modifier marker, which fell to its literal arm where an `Any` parameter matched it; `def g M.up1/v end g 1` is `UP` on both lanes. The original text: A `/v`-marked module member read whose export takes an `Any` FIRST parameter cannot be collected as `def`'s forward argument: `import module [ def up1 fn [[value:Any] [String] ['UP']] export "M" {up1: up1/v} ] end def g M.up1/v` raises `signature_error: cannot call def — no signature matches the arguments … none were supplied` on both lanes, where `def g M.up2/v` (an `Integer` first parameter), `def g (M.up1/v)`, `def g up1/v` (no module) and the bare `M.up1/v` (data) all bind. The parser emits the reach followed by a dispatch-modifier marker (`Word/__DM`, Val); inside `def`'s forward window the reach's fn value reaches the pointer ahead of the marker and, with an `Any` parameter, the window's plan collects nothing. Interpreter-side (both lanes agree), loud | closing NUR163, 2026-09-25 |
| [NUR213](#nur213) | FIXED 2026-09-25 (the marker's intent on the value — the handoff log's entry of that date): the check pass quotes the dynamic member read a standalone marker qualifies (the drop of the marker at the pointer, where the concrete value's peek quotes it at run time), and the residual layout leaves a quoted lead or trailing value alone; `m.f/v 5` is `fn (Integer) 5` on both lanes, `5 m.f/v` declines at an existing residual limit and answers by fallback. The original text: A `/v`-marked MAP MEMBER read with arguments beside it is applied on the compiled lane and data on the interpreter: `def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f/v 5` is `fn (Integer) 5` interpreted (the marker says data; the 5 strands on top) and `6` compiled, `5 m.f/v` is `5 fn (Integer)` for `6` — the checker's shaped member apply (`RecordDynMethod` through the method-shape model) collects the window past the reach's dispatch-modifier marker as if it were not there; `m.f/v` alone agrees (`fn (Integer)`, the alone-in-a-live-reach-group rung). Silent, pre-existing at the merge base (measured on `wt-head`, 2026-09-25) | closing NUR212, 2026-09-25 |
| [NUR214](#nur214) | FIXED 2026-09-25 (the loop's fresh cell — the handoff log's entry of that date): a fresh name a loop body binds, in a loop not proven to run, is carried in the unit's cell with NO init (NoteLoopFresh) — each body def stores into it, its install a per-iteration dyn-scope bind — and the post-loop binding is a carrier aliased to the cell and read BOUND-CHECKED, so a zero-trip read raises `undefined_word` on both lanes and a loop that ran reads its last binding; a dynamic read of the name from a fn after a zero-trip loop is the interpreter's `undefined_word` too (NUR215, closed with it). The original text: A FRESH `def` inside a loop that may run ZERO times binds anyway on the compiled lane: `def n (0 add 0) end for n [def x 5] x` answers `5` compiled and raises `undefined_word` interpreted; `for n [def x (n add 5)] x` answers an empty stack compiled (the unset slot) for the same raise, and `while` and a fn body (`f 0`) do the same. NUR110's loop twin: NUR110 measured only a STATIC `for 0`, which the pass prunes, and closed the branch with a bound-checked slot the loop never got. | closing NUR205, 2026-09-25 |
| [NUR215](#nur215) | FIXED 2026-09-25 (the conditionally bound name's miss — the handoff log's entry of that date): the Program names every bound-checked cell's name (CondBoundNames: an arm's def with no pre binding, NUR110; a fresh def in a loop that may run zero times, NUR214), so a fn's dynamic read of one that misses raises the interpreter's `undefined_word` at the read, as a speculative undef's name does. The original text: a fn that reads such a name DYNAMICALLY on the path that skipped its binding — `def c (1 gt 2) end if c [def k 5] [] def g fn [[][Any][k]] end g`, `def n (0 add 0) end for n [def x 5] def g fn [[][Any][x]] end g` — bailed compiled (`internal_error`: dynamic-scope read miss) where the interpreter raises `undefined_word`; loud on both lanes, the error's code non-uniform. | closing NUR214, 2026-09-25 |
| [NUR216](#nur216) | FIXED 2026-09-25 (the quoted lead is data on every arm — the handoff log's entry of that date): no residual arm applies a value a `/v` marker quoted — the fn-typed carrier lead arm and the verbatim window islands now ask what the dynamic lead arm already asked (NUR213) — so a CLASS member's `c.op/v 5` is `fn (Integer) 5` on both lanes, and the window spellings `3 c.op/v 2` / `3 4 c.op/v` compile and agree; the map twins `3 m.f/v 2` / `3 4 m.f/v` decline at the existing residual limit ("call result above a literal") and answer by fallback. The original text: a `/v`-marked CLASS member read with arguments beside it was applied compiled and data interpreted — `def T fnsig Integer Integer def C class {op:T} def c (make C {op:(fn [[x:Integer] [Integer] [x add 1]])}) c.op/v 5` answered `6` compiled for the interpreter's `fn (Integer) 5`, `3 c.op/v 2` `[3 3]` for `[3 fn (Integer) 2]`, `3 4 c.op/v` `[3 5]` for `[3 4 fn (Integer)]`, and NUR213's map member in the two window spellings the same way. Silent, pre-existing (measured with NUR096's change stashed) | closing NUR096, 2026-09-25 |
| [NUR217](#nur217) | FIXED 2026-09-26 (the stored unit's word read — the handoff log's entry of that date): a STORED fn value's unit (storedfn$body — a container member's apply, a native seam's callback) is compiled once under the declared param types, with no per-call re-analysis, no seated deopt and no replay. It declines where its body reads a binding bare that no argument makes data — a fn-typed read no apply lowering took, a binding read both bare and `/v`, a gradual CAPTURE read in the residual — and for a GRADUAL param the body reads (`[f]`, `[[f]]`, `[x f]`) it lists the slot (CompiledFn.FnReadParams): every seam that runs the unit (the in-program frame push, the foreign host, the token seam, the callback seam) refuses an argument list with a fn in such a slot, and that one call takes the interpreter's own dispatch. `m.g ([] => [42])` is 42 on both lanes, the fn-typed param, `[[f]]` and the applied read with it; data through the same member (`m.g 5`) runs the unit, and the per-call routes (a module member, a def) are untouched. The original text: A member-read fn applied over a fn argument whose body reads a GRADUAL param bare: `def g fn [[f:Any] [Any] [f]] def m {g: g/v} m.g ([] => [42])` is 42 interpreted (the bare read of a frame binding holding a fn dispatches it — NUR123's rule) and `fn f` compiled — the unit the dynamic apply runs was compiled for an `Any` param and keeps the slot push NUR123 left a gradual read; `m.g (z/v)` the same (0 for `fn f`). Silent, pre-existing (measured on the committed head, 2026-09-26) | closing NUR078, 2026-09-26 |
| [NUR218](#nur218) | FIXED 2026-09-26 (a member reference is its word twin — the handoff log's entry of that date): the peek that consumes a group's `/v` marker DELIVERS the value — pushed and stepped past, unquoted, noted as a value read — as `stepWordVal` delivers `inc/v`; the check pass reads a quoted fn-possible binding as the word, a code body's `/v` member takes no replay, and a DYNAMIC branch arm (a flex member, bare or `/v`) is landed on the computed-arm merge as NUR159 lands a named one (`if true m.h [2]` compiled to the member for the interpreter's 1, pre-existing). Every shape answers what the word twin answers, for a map, a flex and a module member, on both lanes; a body's `[m.f/v]` declines loudly where it compiled to the applied value. The original text: A `/v`-quoted MEMBER read is not the value its word twin is: inside a paren, as a def's value, or as a code body's result the quote rides on — `each (m.f/v) [1 2 3]` is `[fn fn fn]` interpreted and `[2 3 4]` compiled (`each (inc/v) [1 2 3]` is `[2 3 4]` on both), `fold (m.f/v) …`, `if true (m.f/v) [2]` and `(m.f/v 5)` the same way; `def g (m.f/v) end g 4` is 5 interpreted and `[fn 4]` compiled, `[1 2 3] each [m.f/v]` `[fn fn fn]` interpreted and `[2 3 4]` compiled. Silent both ways, pre-existing (measured on the committed head, 2026-09-26) | closing NUR078, 2026-09-26 |
| [NUR219](#nur219) | FIXED 2026-09-26 (the callback body's word read — the handoff log's entry of that date): a callback BODY unit (each$body, fold$body, … compiled from a fn value or a lambda at a higher-order word's slot) lists the params it reads bare where it pushes the slot (CompiledFn.FnReadParams — the stored unit's list, NUR217), the pushed closure carries the callback value it was compiled from (ClosureRetSpec.Source → ClosurePayload.Source), and the VM's token seam hands an invocation with a fn in such a slot to the interpreter's own run of that value — stepped over the inputs on the TOKEN seam, matched and called on the fn-VALUE seam — with the closure's runtime captures; data elements run the unit. The original text: A fn value or lambda handed to a higher-order word reads its gradual param bare: `def g fn [[f:Any] [Any] [f]]  each g/v [([] => [1]) 7]` is [1 7] interpreted and [fn f 7] compiled; `each ([x:Any] => [x]) …`, `x typeof`, a gradual list param, a paren `(f)` and the map-iteration fold's accumulator (`fold ([a:Any kv:Any] => [a]) {x: 1} ([] => [5])`, 5 against `fn a`) the same way. Silent, pre-existing (measured on the committed head, 2026-09-26) | closing NUR217, 2026-09-26 |
| [NUR220](#nur220) | FIXED 2026-09-26 (the anonymous park at the dynamic apply — the handoff log's entry of that date): the whole-frame replay reads the interpreter's ANONYMOUS-0-ARG PARK over its lead (replayLeadParks — a lambda or macro value whose only signatures take nothing is data unless `apply` marked it), as the landing already did; a lead the body read bare BY NAME is the binding's word dispatch and still fires. An `apply` over a LONE gradual lead declines where the registered-output arm elided its identity result silently, dropping the Applied mark. The original text: A map-each lambda's member read of a 0-arg lambda: `def m {x: ([] => [5])}  each ([kv:Any] => [kv.v]) m` is `{x:fn}` interpreted and `{x:5}` compiled — the whole-frame replay entered the lambda's stamped unit over an empty window; `kv get "v"` the same. Silent, pre-existing (measured on the committed head, 2026-09-26) | closing NUR219, 2026-09-26 |
| [NUR221](#nur221) | FIXED 2026-09-26 (a landed lead is no apply-event lead — the handoff log's entry of that date): the gradual apply event (OpCallDynApplyOne) applies its lead to the one value beneath, which is the interpreter's answer only for an INERT lead; a lead whose producer carries a re-step landing (a bare member read, not `/v`, not a user paren's placed result) takes the dynamic-lead decline instead. The original text: `def m {x: inc/v}  each ([kv:Any] => [3 kv.v apply]) m` raises apply's no-match interpreted (the member claims the 3 at its own step, and apply meets the 4) and answered `{x:4}` compiled. Silent, pre-existing (measured on the committed head, 2026-09-26) | closing NUR220, 2026-09-26 |
| [NUR222](#nur222) | FIXED 2026-09-26 (the dyn body's settled lead — the handoff log's entry of that date): a dynamic lead whose argument is another result of the same DYN-BODY dispatch (`do` over a body the closure path declined) is the body's to settle — its handler runs the body with the interpreter's semantics — and the program's residual arm leaves it; a dyn-body result with nothing of its own above it stays the lead the interpreter re-steps over a later token. The original text: `def m {f: inc/v}` from a factory, `do [m.f 5]` is 6 interpreted and `CALL_DYNAMIC underflow` compiled (an internal error): the model left the member over its 5, the residual arm applied it again over a region the body had already settled; `7 do [m.f 5]` and the `/q` twin `do [m.f y]` the same. Loud, pre-existing (measured on the committed head, 2026-09-26) | closing NUR190, 2026-09-26 |
| [NUR223](#nur223) | FIXED 2026-09-26 (the seam discards the unconsumed input — the handoff log's entry of that date): the callback seam (InvokeCompiled) hands its caller exactly what the interpreter's CallBoru hands — residuals beyond the SIGNATURE's declared return count that are unconsumed unnamed params are discarded, up to the unnamed-param count; a stored fn's unit is compiled count-agnostic, so its RET used to hand back the whole residual. The original text: `def Z fnpred [[Integer] [true]]  0 is Z` is true interpreted and false compiled — the unit returned [0 true], which the predicate protocol refuses ("predicate must return exactly one value, got 2", raised outright by `def v:Z 0` compiled only); `fnpred [[Integer] [dup 0 eq]]` the same. Silent, pre-existing (measured on the committed head, 2026-09-26) | closing NUR100, 2026-09-26 |
| [NUR224](#nur224) | FIXED 2026-09-26 (the predicate's refusal is a type_error — the handoff log's entry of that date): a typed def's predicate refusal raises `type_error` on both lanes, as the typed def's other refusals do (`def q:T "x"` — does not unify with declared type T). The original text: `def Big fnpred n:Integer [n gt 10]  def f fn [[x:Any] [Any] [def q:Big x q]]  f 5` raised the refusal as a PLAIN error interpreted and as an `internal_error` annotated "this is a compiler defect" compiled — the typed-bind op raised the same plain error, and the compiled run books any non-BoruError as its own defect. Loud on both, the class split, pre-existing (measured on the committed head, 2026-09-26) | closing NUR100, 2026-09-26 |
| [NUR225](#nur225) | FIXED 2026-09-26 (templates and XML holes spell their source — the handoff log's entry of that date): canon renders a template string as backtick source (`canonTemplate`: literal text with the template lexer's escapes, each hole `${…}` over its tokens' canon) and an XML literal with holes as its XML (`canonXmlTmpl`: text and attribute text escaped as a plain element's), in both ports — TS keeps a tagged empty hole as `${}`. The original text: Template strings and XML literals with `${}` holes canon in DEBUG form — `interp('v ' ${1} ' w')`, `interp-xml(<a>${1}</a>)` — which no parser accepts (a template re-parses as a syntax error, an XML literal as the word `interp-xml` over a group): 30 of parse.tsv's rows fail the canon fixpoint on it, in both ports | the canon fixpoint gate, closing NUR072, 2026-09-26 |
| [NUR226](#nur226) | FIXED 2026-09-26 (the key canons as its key — the handoff log's entry of that date): a map key renders bare only when it lexes back as the same key (letters, digits, `_ $ - @`) and quoted otherwise (`canonKey`), in every canon map arm, both ports. The original text: A map key that needs quoting canons bare: `{'q k':2}` renders `{q k:2}`, which re-parses as two entries (`{q:q k:2}`) — ADR-015's round-trip broken on the key, in both ports | the canon fixpoint gate, closing NUR072, 2026-09-26 |
| [NUR227](#nur227) | FIXED 2026-09-26 (a comma before the angle — the handoff log's entry of that date): a sequence joins a part whose last token is a bare capitalised name to a part opening `<` with a comma (`joinCanonParts`), so `[:A, <a/>]` re-parses as the typed list it is, both ports. The original text: A typed tag before an XML literal re-lexes as the angle sugar: `<a/>:A` canons `[:A <a/>]`, which re-parses as `[:A<a/>]` — whitespace does not separate `A` from `<`, so the tag and the element fuse into `A<a/>` | the canon fixpoint gate, closing NUR072, 2026-09-26 |
| [NUR228](#nur228) | FIXED 2026-09-26 (the gradual window declines — the handoff log's entry of that date): the matcher flags a split whose window hangs on a gradual stack operand while a later overload forward-collects past the token the selected one stopped at (`laterCandidateCollectsPast`), and the compile declines with the gradual-split reason — the mirror of the existing split flag. The original text: `def v (whereis "x") v send {a: 1} "nobody"` is `[None]` interpreted (v is None, so `send (Any, String)` takes both forward tokens) and raised signature_error compiled: the check pass matched `send (Any, Pid)` over ONE forward token and the dynamic v, and compiled that window — `{a: 1}` sent to None; `whereis "x" send {a: 1} (self)` declined as a "stack discipline" compiler defect. A wrong answer (a program error the interpreter does not raise), pre-existing (measured on main at 3b5db68) | closing NUR064, 2026-09-26 |
| [NUR229](#nur229) | FIXED 2026-09-26 (one escape vocabulary, one malformed-escape report — the handoff log's entry of that date): a boru matcher refuses a malformed quoted-string escape before jsonic's lexer reads it, the escape itself named, in both ports. The original text: the two tabnas ports reported a malformed escape in a quoted string differently — `"a\x4"` an invalid ascii escape in Go and an unterminated string in TS, `"a\xZZb"` spanning `"a\xZZ` in Go and `\xZZ` in TS. Pre-existing, outside the corpus | closing NUR026, 2026-09-26 |
| [NUR230](#nur230) | FIXED 2026-09-26 (one escape vocabulary, one malformed-escape report — the handoff log's entry of that date): Go's shared escape writer pairs a UTF-16 surrogate split across two `\uXXXX` escapes into one code point, as jsonic does in a quoted string. The original text: in a template, `\ud83d\ude00` read as two U+FFFD in Go and as one code point in TS (whose UTF-16 strings pair the units); both ports read it as one in a quoted string. Pre-existing, outside the corpus | closing NUR026, 2026-09-26 |
| [NUR231](#nur231) | FIXED 2026-09-26 (value half: Bytes a refinement base, a computed bound the run's; type half: the run-time type install — the handoff log's entries of that date): a refinement constructor over a bound the check pass does not know latches a run-time construct and the dispatch records as the call it is, so the run builds the refinement; a refinement bakes only over const bounds, `between` decides no empty interval from an unknown one, and a membership check over one decides nothing in the pass (an intersection keeps the unknown bound, a complement admits). A TYPE over one compiles to the run-time install: the run installs it from the body it computed (OpBindTypeRun) and the pass's node forwards to the run's, a typed def records the run's own membership check, and an overload set over one re-matches at run time. An inline parameter or return type over one compiles too: its pattern is an anonymous node the run forwards to the refinement it computed. An inline interval over one (which the run may find empty), a typed container's child and a fn body's per-call type def decline through the existing compile-time-word site. The original text: `3 is (Integer gt (size "abc"))` was false interpreted and true compiled — the check pass built the refinement over its carrier for the computed bound and the recorder baked it; a carrier orders below every value, so a lower bound admitted everything, an upper one refused everything (a false check-time type_error too), `between` over one was Never, and a type over one bound or dispatched unchecked compiled. Pre-existing | closing NUR009, 2026-09-26 |
| [NUR232](#nur232) | FIXED 2026-09-26 (Bytes a refinement base, a computed bound the run's — the handoff log's entry of that date): the return-pattern check defers a refinement's value-level membership over an abstract residual not provably outside its base, the named return type's rule. The original text: `def g fn [[n:Integer] [(Integer gt 3)] [n]] g 5` was a check-time type_error ("expected (Integer gt 3), got Integer") that both lanes then returned 5 for; the named twin (`[Big]`) is check-clean. Pre-existing | closing NUR009, 2026-09-26 |
| [NUR233](#nur233) | FIXED 2026-09-26 (a make field's refusal is a type_error — the handoff log's entry of that date): a make field the run refuses raises a type_error on both lanes; a refusal already structured keeps its code. The original text: the refusal was a plain error, which the interpreter printed bare and a compiled run booked as a compiler defect (internal_error with its "please report it" note) — `def Big (Integer gt 100) def S class {x:Big} def n 0 for 3 [def n (n add 1)] make S {x:n}`. Pre-existing | compiling NUR231's type half, 2026-09-26 |
| [NUR234](#nur234) | FIXED 2026-09-26 (the call carries the interpreter's window — the handoff log's entry of that date): a compiled user call's param-contract no-match reports the window the interpreter's failed dispatch reports — the written run, which a bare read ends, filled from the stack beneath. The original text: a compiled direct call's param-contract no-match reported every argument, where the interpreter reports its attempted window: `def f fn [[n:String] [Integer] [0]] each ([e:Any] => [f e]) [5]` noted "the argument was 5 (an Integer)" compiled and "takes 1 argument, but none were supplied" interpreted. Pre-existing | compiling NUR231's type half, 2026-09-26 |
| [NUR235](#nur235) | FIXED 2026-09-26 (a named fn value's push carries its name — the handoff log's entry of that date): a member read of a nullary fn value from a `fn` literal fires on both lanes; the closure's unit is shared with anonymous values over the same body, so the name rides on the push. The original text: a fn-body-local fn def bound into a returned map: the member read returns the fn compiled, calls it interpreted — `def mkg fn [[c:Any][Any][def g fn [[][Any][c]] {g: g/v}]] end def m (mkg 5) end m.g` answers `[fn g]` compiled, `[5]` interpreted. A silent wrong answer | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR236](#nur236) | FIXED 2026-09-26 (a spliced consumer is ordered by the stream — the handoff log's entry of that date): a gradual def read consumed by a spliced word's expansion carries its deopt, so a read that holds a fn at run time dispatches it on both lanes. The original text: a word splice over a def-bound gradual read of a fn: `def tp word [typeof] def h fn [[m:Map][Any][def j (m get "f") j tp]] h {f: ([] => [42])}` answers `[Function]` compiled (typeof over the fn value) and `[Integer]` interpreted (`j` calls the fn). A silent wrong answer | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR237](#nur237) | FIXED 2026-09-26 (an S5 name's later root defs are registry-visible — the handoff log's entry of that date): a taken branch arm's rebind of a name an S5 loop bind bound is seen after the merge on both lanes. The original text: a def rebound in a taken branch after a loop-result def reads the pre-branch value compiled: `def x (for 2 [5]) def c true if c [def x 1] [] end x` answers `[5 5]` compiled, `[5 1]` interpreted (both arms binding x too; a read of x before the branch makes the lanes agree). A silent wrong answer | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR238](#nur238) | OPEN (recorded 2026-09-26; proposed verdict: resolve by fix): a paren-bounded trailing apply that matches nothing: the interpreter parks an anonymous value as data and raises `uncalled_function` for a named fn; the compiled apply raises `signature_error` at the top level (`(5 ([s:String] => [s]))` is `[5 fn (String)]` interpreted) and leaves a named fn's window as residue inside a fn (`(5 f/v)` over a `g/v` argument: a count error compiled) | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR239](#nur239) | OPEN (recorded 2026-09-26; proposed verdict: resolve by fix): an applied fn value's return-contract error names the fn's definition compiled and the binding it was called under interpreted: `(k 5)` over `h z/v` says `z:` compiled, `k:` interpreted; an anonymous class-field fn says `` compiled, `<fn>` interpreted (`each h.cb [1 2 3]`) | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR240](#nur240) | OPEN (recorded 2026-09-26; proposed verdict: resolve by fix): a trapped unmatched module-member call inside a branch arm raises `signature_error` compiled and `uncalled_function` interpreted — a value-level divergence where the code is caught (`do [if true [(true 5 M.dec)] [1] …] error [dot code]`) | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR241](#nur241) | OPEN (recorded 2026-09-26; proposed verdict: resolve by fix): a capturing callback run by `walk`: `acc (tag) append acc (m.path) append` in a factory's lambda raises `signature_error: cannot call append` compiled (the arguments `[]` and `''`) where the interpreter answers `['x' '' 'x' 'a' 'x' 'b']` | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR242](#nur242) | OPEN (recorded 2026-09-26; proposed verdict: resolve by fix): eight programs compile and then fail inside the compiled runtime (internal_error) where the interpreter answers: three `/q`-capturing member landings (RESTEP_LANDING), two shaped method applies, `do [args drop] 7` in a fn (STORE_LOCAL underflow), `do b 7` over a list param (CALL_DYN_FRAME underflow) and `fold` over a gradual class field (CALL_NATIVE_POLY no match) | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR243](#nur243) | FIXED 2026-09-26 (the three programs compile — the handoff log's entry of that date): a constant branch's taken arm with no value is a 0-value statement, a loop rebind of a branch-bound name is carried, and a parser dispatch declines over its parser operand only. The original text: three valid programs refused: a loop carrying a branch-bound name (`if c [def x 1] [] for 3 [def x 5] x`, "body result of unknown provenance"), a constant-true branch whose taken arm leaves no value (`def x 0 if [true] [def x 1] [2] end x`, "branch produces no value" — its guard carried a `//covergate:allow` whose proof was false, removed), and NUR109's bound-slot arm declining a parser dispatch over a branch-bound SOURCE operand | closing #505's merged-coverage gap (ADR-008), 2026-09-26 |
| [NUR244](#nur244) | OPEN (recorded 2026-09-26; proposed verdict: resolve by fix): a parser bound on a branch that does not run is used compiled when it is an inline `fn` literal: `import "boru:parselang" def c false if c [def p (fn [[source:String opts:Map] [Any] [7]])] [] end parse p 'x'` answers `[7]` compiled and raises `parse_unknown_lang` interpreted — NUR109's decline catches a promoted call-result parser, not a baked literal. A silent wrong answer | closing NUR243, 2026-09-26 |
| [NUR174](#nur174) | The re-step landing was recorded at the REACH-GROUP COLLAPSE, which made it a WHITELIST OF PRODUCERS — and `m get 'f'` is the same member read written as a word call, so no collapse ever saw it: `def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end m get 'f'` answered 42 interpreted and `fn h` compiled. FIXED 2026-09-20 by reading the fact where check's model already stands — inside `stepLiteral`, on the branch whose next act is `execFnDefLiteral` — and deleting the recording apparatus. Three rungs of `execFnDefLiteral` the landing had to mirror came with it, each caught by a probe and each a wrong answer on its own: the ANONYMOUS-0-ARG PARK, a DISPATCH MODIFIER, and a value still alone inside a LIVE reach group | measurement, 2026-09-20 |
| [NUR173](#nur173) | A REACH-lowered group (`m.f` is `( m dot f )`) never parks, so its collapse rewinds onto the one value it leaves and re-steps it — a callable one DISPATCHES. The check pass holds a carrier there and steps past it as data, and no fn-value-call arm could see the shape because every one of them needs a second residual entry. `def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end m.f` answered 42 interpreted and `fn h` compiled, silently. FIXED 2026-09-20 by recording the landing and letting the RUNTIME value decide (`OpReStepLanding`); the SEAT of that recording was then corrected by [NUR174](#nur174), which closed the `get`-WORD twin. A variadic region's top remains. This is NUR169's defect, and NUR169's "no case for `count == 1`" named its mechanism correctly | measurement, 2026-09-20 |
| [NUR169](#nur169) | SUPERSEDED BY [NUR173](#nur173), which fixed it. The mechanism recorded below — no case for `count == 1`, so a one-survivor collapse reaches no fn-value-call arm — is CORRECT; the seat is one function out. Original text: a paren that nets exactly ONE value which is a FUNCTION is AUTO-APPLIED by the interpreter and silently NOT applied on the compiled lane | a Codex review of PR #475, 2026-09-19 |
| [NUR166](#nur166) | FIXED 2026-09-25 (the value's own frame — the handoff log's entry of that date): the token seam brackets a closure unit that carries a param contract — a fn value's body, named or lambda — with `pushRootArgs` over the value's own call args, as the fn-value seam brackets the units it hosts; `each g/v [1 2]` over `[do [args]]` is `[[1] [2]]` on both lanes, a quotation body still reads the caller's. The original text: A def-bound fn VALUE handed to a higher-order word loses its own frame's `args` on the compiled lane: `def g fn [[n:Integer][Any][do [args] size]] end each g/v [1 2]` is `[[1 1]]` interpreted and `[[0 0]]` compiled (`do [args]` alone: `error(args: not inside a function)` per element). The value's body is lowered as the each site's closure unit, whose `args` is the ENCLOSING frame's list — none at top level — where the interpreter opens a frame for the value and pushes its call args. Pre-existing at #474's merge base (measured on `origin/main`); S1b's native fn-value seam pushes the value's own args for the units it hosts (pushRootArgs), but this row does not reach that seam — the closure lowering does. Same family as NUR155 (the closure unit is not the value's frame) | the S1b seam pins, 2026-09-19 |
| [NUR165](#nur165) | RESOLVED 2026-09-19 (the S1a review). A TOKEN body over a collection the check pass knows only as `Any` — a fn's declared `Any` result: `def get fn [[][Any][1]] end each [add 1] (get)` — compiled through the cross-collection shortcut (`CallableSpec.CrossCollectionTokenShape`): the recorder committed the (List, Map) arm's closure, trusting the handler to be robust to the sibling collection, and a runtime Integer raised `each_error: expected a concrete map` where the interpreter's dispatch raises `signature_error` (fold the same, `fold_error`). Pre-existing at #474's merge base (measured on `origin/main`); S1a added the Reach twin `each $.x (get)`, which baked the one reachable (Reach, List) arm and answered `[[]]` — a wrong VALUE, silent — found by a Codex review of #474. Fixed at both seats: an Any carrier operand at the dyn-body seat re-matches whatever the arm count, and the committed arm's map guard raises the dispatcher's own signature_error for a runtime value that is neither collection (routing the words to the dyn-body seat instead was measured and rejected — it arms DynEnv mode program-wide and refused fifty-three module-cli.tsv rows) | a Codex review of #474 (2026-09-19), and the sibling shapes probed from it |
| [NUR164](#nur164) | RESOLVED 2026-09-19 (S1a). A callback that matches none of a fn value's signatures raised a PLAIN Go error, not a BoruError — `each`/`fold`/`scan` over a Map (`no matching lambda signature for N argument(s)`, native_map_iter.go), `filter` and `walk` (`no matching callback signature`) — and the compiled-by-default lane reads every non-Boru error off the VM as an INTERNAL bail (`runtimeShouldFallback`): it rolled the registry back and re-ran the whole program on the interpreter, reporting a correct compiled verdict as "not compiled" — `def f fn [[c:Any][Any][each ([x:Integer] => [x add 1]) c]] end f {a:1 b:2}` raised the identical error on both lanes and answered `ran=false`, invisible to every differential. Fixed by raising `signature_error` at the three sites | the S1a pin over a gradual Map collection, 2026-09-19 |
| [NUR163](#nur163) | FIXED 2026-09-25 (the value's own signatures — the handoff log's entry of that date): the mini / emit / parse signature contracts (`MiniLangFnSigWhy`, `EmitLangFnSigWhy`, `ParseLangFnSigWhy`, the filter-shape probe and its partial) read `FnDefInfo.OwnSigs()` — a value read through `/v`, a module member in place or a paren carries the dispatch aggregate, whose synthesized 0-arg fallback signature failed the prefix check; `mini M.dbl 'ab'`, `mini (dbl/v) 'ab'`, `emit M.up {a:1}` and `parse M.p 'xy'` answer as their def-bound spellings do, on both lanes. The side note — `def g M.up/v` for an Any-first-parameter export — is recorded as NUR212. The original text: A module member read IN PLACE is not the fn its local rebind is: `mini M.dbl 'ab'` and `emit M.up {a:1}` (and their parenthesised forms) are rejected by the words' signature-prefix validation — `mini_bad_signature`, `emit_bad_signature` — while `def g M.dbl/v end mini g 'ab'` answers `abab` and `def g (M.up) end emit g {a:1}` answers `UP`; `inspect (M.up)` reports a Function literal with no signatures where `inspect up` reports the defined fn and its signatures | the generated sweep, the module-export seeds of `emit`, `mini` and `parse` |
| [NUR153](#nur153) | RESOLVED 2026-09-18 (the tape rule everywhere). One stored `=>` value, two evaluation regimes on the interpreter. A stored `=>` callback's single container residual is DEFERRED when the value is applied on the tape — `def a 99 def api (patrun Function) add {cmd:"x"} ([a:Map] => [[a]]) api def h (find {cmd:"x"} api) h {z:1}` is `[99]`, the lambda rule — and evaluated IN THE LIVE FRAME when a native seam invokes it through InvokeCallback / CallBoru — a `service` catch-all `([req:Map state:Any] => [ {message: (join "" ["unknown '" req.cmd "'"])} ])` answers `unknown 'BOGUS'` on `call`, reading its param. The compiled stamp is ONE unit and takes the CallBoru regime (the fn-body recordability gate admits a stored body by name, which is what lets mini-redis's catch-all stamp), so the tape apply of a stamped stored `=>` value diverges: `[{z:1}]` compiled for `[99]` interpreted, silent, exit 0. Pre-existing at #471's merge base (measured on `origin/main`). The compiler cannot close this alone — the interpreter needs ONE rule for the residual of a fn value, whichever seam applies it | a Codex review of #471 (2026-09-17), which attributed it to the island fix; measured pre-existing and two-sided |
| [NUR152](#nur152) | RESOLVED (2026-09-17). A fn value's HOME — the registry its free words resolve in — was stamped only at module-export resolution, so a main-program fn carried none and every seam read nil as "wherever this is running": handed INTO a module (`M.run pub/v`, `run` applying its `f:Function` param), `pub`'s `secret` resolved in the MODULE — `cannot call add` interpreted where the compiled lane answered 6, and with a same-named `def secret 100` in the module, 105 on BOTH engines for the rule's 6, invisible to any differential. The mirror image of the 2026-08-15 fix, which only covered module→main. Fixed by stamping the home at construction (`fn`, `=>`, `macro`) and comparing homes by MODULE (`Registry.Home`), which is what a concurrent fork inherits — the second face found on the way: comparing pointers sent a same-module callback back to the shared registry from its per-connection fork (`fatal error: concurrent map iteration and map write` under serve-raw) — plus compiling a stored-fn / fn-value unit at the value's home rather than the emitter's mid-foreign-compile registry (the third face: compiled 105 for 6) | investigating the main-vs-module representation split at the maintainer's request, 2026-09-17 |
| [NUR146](#nur146) | FIXED 2026-09-25 (the frame's names — the handoff log's entry of that date): The compiled lane's `undefined_word` suggests over the REGISTRY, the interpreter's over a registry that also holds the frame's bindings as defs: `def k 5  for 2 [ if (k eq 5) [undef k] [] ] 9` raises the same `undefined word: k` at `1:25` on both lanes, with ``did you mean `i`?`` interpreted (the loop iterator is a def binding there) and no suggestion compiled (the iterator is a frame slot). The first line — code, detail, position — agrees; the help line below it does not | the sixty-eighth increment's placed undef, 2026-09-16 |
| [NUR143](#nur143) | FIXED 2026-09-24 (the recorder's registry after a module body — the handoff log's entry of that date): the enclosing-binding snapshot a fn unit takes at its open read the recorder's binding — the registry of the last engine that ran, a nested import's after `import "boru:sift"` — where the module's flex is no binding, so the read fell to the check pass's const clone; it reads the unit's own registry now (compiler StartFnCompile's fnReg), both descriptors read the binding live, the oracle ledger entries are retired and the class is pinned in lang/go/module_fn_unit_registry_test.go (`R.put 'a' 1  R.count` over an inline module's flex was `[1 0]` for `[1 1]` on the unfixed tree, silent). The original text: A fn-body read of a MODULE-SCOPE flex binding is compiled as a FRESH CLONE of the check pass's snapshot (`PUSH_CONST_FRESH`), not as the binding the interpreter resolves: boru:sift's `Sift.kinds` (`keys sift-catalog`, sift.boru:1042) and `Sift.detect` (`keys sift-path-detect`, :1078) read a copy. The keys agree because the check pass PERFORMS the run's mutations (a dry-passed `set` on a concrete flex populates the snapshot before it is taken) and because a mutation in an EARLIER request makes the next compile refuse ("operand of unknown provenance or not statically materialisable at keys" — the memo's materialisation guard, whose refusal is a defect the interpreter currently absorbs); neither is the rule "a read of a binding is the binding". Two corpus descriptors, ledgered by name in `test/go/langspec/region_oracle_test.go` | the COLLECT oracle, under review of #458 (2026-09-15), the moment its agreement test became identity |
| [NUR142](#nur142) | FIXED 2026-09-25 (the family of a refinement — the handoff log's entry of that date): A REFINED container is `eq` to nothing, not even itself: `def S (refine FlexMap)  def w:S (flex {a:1})  w eq w` is false, as are `def M (refine Map)  def m:M {a:1}  m eq m` and `def L (refine FlexList)  def v:L (flex [1 2])  v eq v`, and `[w] deq [w]` with it — where the unrefined `def w (flex {a:1})  w eq w` is true. `ExactEqual` reaches its container-identity arms through `nodeFamily`, which folds only the kernel's own flex nodes, so a value whose tag is a refine of Map or List falls past every arm to the terminal `false` — the shape NUR031 closed for opaque handles ("not even eq to itself"), open again one family over. `core.SameContainer` is the identity test itself, exported for the COLLECT oracle, which needs the answer; the `eq` word does not yet read it | the COLLECT oracle, under review of #458 (2026-09-15): 22 corpus descriptors over refined flex bindings read as divergent under the `eq` rule and as the same object under the identity test |
| [NUR141](#nur141) | FIXED 2026-09-25 (the predicate runs for real — the handoff log's entry of that date): The check pass ADMITS a value to a predicate-typed parameter that the runtime scan REJECTS: `def Even fnpred n:Integer [eq 0 (mod 2 n)]  def f fn [[n:Even] [Integer] [n]]  f 5` is a `signature_error` on both lanes, but the check pass's dispatch plan claims `5` for `n:Even` (the region descriptor records a claim of one forward slot) where the runtime's candidate scan claims nothing — the predicate is run by one matcher and not the other. The answer agrees because the row errors either way; the MODEL of which signature a value matches does not, and a checker verdict built on it (a reachable arm, a narrowed result) would be wrong. One corpus row, ledgered by name in `test/go/langspec/region_oracle_test.go` (`over-claimed`) | the COLLECT oracle's first corpus walk, 2026-09-15 (the sixty-second increment) |
| [NUR139](#nur139) | RESOLVED (2026-09-11, the fifty-seventh increment). One rule — "an unreachable unit's lowering says nothing about the PROGRAM" — had TWO per-unit refusal sites in Finalize and covered only one. `lowerEvents` refusing took the stamp-only trap-stub recovery; `reconcileResults` refusing returned straight out and killed the program. Invisible while the only unreachable units were fn-value stamps, whose refusals happen to land in the first site; the moment a declined closure dispatch left one, `for 2 [def b true  do [1 2 (if b [] [9 9])]]` went from compiling to "fn do$body: body leaves extra values". Same shape as NUR136 — one invariant, two sites, one of them wrong — and the recovery is now one helper (`unreachableUnitStub`) called from both | the variation lane, on a row this increment added, 2026-09-11 |
| [NUR138](#nur138) | RESOLVED (2026-09-11, the fifty-seventh increment). Third instance of NUR133's and NUR137's shape — a screen written for the producers that happened to exist, meeting one that did not — and the FIRST in the opposite direction: it cost refusals, not a wrong answer. `regionReadsTheStack` answered "reads the stack" for any `opClosure` operand, but a closure operand is a PUSH (`OpPushClosure` puts captures then closure ABOVE the mark and the call pops what it pushed) and its captures are always promoted to frame locals. Since a body word's own body operand IS an `opClosure`, the blanket answer declined the region plan for every CLOSURE-COMPILED body word — so `7 def b true  do [1 2 (if b [] [9 9])]` seated its prefix only while the body took the dyn-body strategy and refused the moment the body compiled. The screen now walks the captures instead of assuming, so an unpromoted capture still declines | widening the whole-residual dispatch's exactness screen, 2026-09-11 |
| [NUR137](#nur137) | RESOLVED (2026-09-11). `regionReadsTheStack` read `ev.call.ops` for every event kind it did not explicitly name, and an `evBranch`'s operands live in `ev.br` — so a BRANCH region's condition was never screened. The forty-seventh increment widened `variadicRegionEvent` to admit branch regions without widening the screen, and `def zs [0] def zt (zs 0 getr)  1 (if (zt gt 0) [] [9 9])` compiled to `9 1 9` against the interpreter's `1 9 9`: silent, exit 0, on the DEFAULT lane. The fifty-fifth increment then carried the same unscreened shape into body units, where it surfaced as `bytecode: internal: SEAT_BELOW_MARK prefix reaches past the mark`. Second instance of NUR133's exact mistake — a new region producer meeting a screen written for the producers that happened to exist — so the predicate's default is now "reads the stack" rather than a silent empty-ops answer: an unnamed kind costs a refusal — a defect owed a fix — rather than a wrong answer | adversarially probing the fifty-fifth increment's own seat, 2026-09-11 |
| [NUR136](#nur136) | RESOLVED (2026-09-11, the fifty-fourth increment). One invariant — "a unit's local count must cover every local its own code stores to" — had two orderings, and the fn unit's was wrong: `cf.NLocals` was grown from `flw.numLocals` BEFORE the residual reconciliation, while the program's `NumLocals` write-back runs after it and carries a comment saying why. Invisible while nothing allocated during a fn unit's seating; the moment the body-unit residual rebuild did, the VM read past the end of a frame it had sized without the temps (`internal bytecode VM error: runtime error: index out of range [2] with length 2`, on `[10 20] each [drop (1 add 2) (3 add 4) 1 pick]`). Fence: lang/go TestBodyResidualRebuildSizesTheFrame, which walks every unit's STORE_LOCAL/PUSH_LOCAL against its own NLocals rather than pinning the one witness | writing the body-unit residual rebuild, 2026-09-11 |
| [NUR135](#nur135) | FIXED 2026-09-25 (the last pop — the handoff log's entry of that date): `TypeTable.Retire` deletes a node from `byID` with no count of how many LIVE def entries hold it, so pushing ONE minted node under a name twice makes the first `undef` unregister it out from under the second ("bytecode: internal: unresolvable type operand Big"). The interpreter never meets it — every `def Big …` mints afresh — so only something that REPLAYS one captured type entry N times does, which is what a bind twin is. Worked around in `core.ApplyResidentTypeBind`, which re-installs the captured BODY so each element mints its own node. Second face: `Retire` never unregisters the name PARTS `RegisterPart` added, so after a replay rollback `validateTypeName` rejects the re-install on the check pass's own leftovers — which is why `InstallTypeBody` exists | the fifty-third increment's cross-request parity oracle, 2026-09-11 |
| [NUR134](#nur134) | FIXED 2026-09-25 (the unit trap and the caught Error — the handoff log's entry of that date): A MODULE-exported fn's failed dispatch inside a caught `do` body is reported as an UNCAUGHT program error where the identical LOCAL fn is downgraded: `do [(true 5 zd) "x"] error [dot code]` gives `no_signature` at INFO with CaughtAtRuntime and COMPILES, while `do [(true 5 M.dec) "no-raise"] error [dot code]` gives `uncalled_function` at ERROR, uncaught, and the program refuses — both interpret to the caught code as a value. The central re-attribution in AddDiagnostic claims to cover every error family uniformly; a second analysis of the same call, with the body depths reset and outside the CaughtBodyDepth bracket, escapes it (the AnalyseCodeEffectCarrier dry pass is the suspect, and identifying it is what is owed). Fixing it does NOT graduate the two frontier-do-catch rows — the pipeline refuses on a caught model-undermining finding too, a refusal owed its own fix — so this is a check-accuracy defect, not a compile-coverage one | probing the do-catch ledger rows after the forty-ninth increment, 2026-09-11 |
| [NUR133](#nur133) | RESOLVED (2026-09-10). A region's consumers read only two of the four kinds of event that produce one: `regionReadsTheStack` walked `ev.call.ops` and a loop's operands, so a variadic USER CALL's and a FALLBACK's own operands went unexamined and the `STACK_MARK` opened above a value the region's op then popped — `def f fn [[n:Integer] [] [for n [i]]] 9 f (1 add 2)` answered `0 9 1 2` for the interpreter's `9 0 1 2`, and `def xs [1] [do [1 div (xs 0 getr)] error [drop]]` `1 []` for `[1]`. Separately `RecordFallback` marked the island a region without `regionMayBeFn`, and an island's run is arbitrary interpreted code, so a Function passed through a handler was seated as data where the interpreter re-steps it (`uncalled_function` for `[6]`). Measured on the merge base: the two `error` shapes REFUSED there, so the forty-eighth increment made those two; the user-call one diverged there too, from an older defect the review's own diagnosis missed — `lowerUserCall` force-promoted a variadic callee's result to ONE frame slot, popping one value from a runtime-variable run. All three now refuse — three wrong answers traded for three unimplemented cases, each owed a fix, absorbed meanwhile by the interpreter; the const-argument twin still compiles natively through `OpSeatBelowMark` | a Codex review of PR #448, 2026-09-10 |
| [NUR132](#nur132) | RESOLVED (2026-09-10, the fiftieth increment). A `break` / `continue` whose loop was in the SAME unit lowered to a bare `OpJmp`, which reached the right pc and did neither of the two things the interpreter does: TRIM THE ROUND (its tape splices back to the round's mark) and, for a break, CLOSE THE LOOP. `for 3 [ (7 add 2) if (i eq 2) [continue] [5] end ]` answered `9 5 9 5 9` for the interpreter's `9 5 9 5`, its `break` twin `9 5 0 9 5 1 9` for `9 5 0 9 5 1`, and `while [true] [ (7 add 2) if true [break] [5] end ]` `9` for `[]` — silent, exit 0, on the DEFAULT lane. The leak was worse than the trim: an inner loop's break landed PAST the `FOR_NEXT` that pops it, so `for 2 [ (i add 0) end for 3 [ if (i eq 1) [break] [0] end ] ]` had the OUTER loop stepping the INNER loop's stale counter and never terminated (tape_exhausted) where the interpreter answers `0 0 1 0`. Both terminators emit the FLOW signal ops now — the same pair the cross-frame case already used, whose `vmLoop` carries the very destinations the jumps named | shrinking the last `while` frontier row to its minimal shape, 2026-09-10 |
| [NUR131](#nur131) | RESOLVED (2026-09-10, the forty-fifth increment). A full-stack SHUFFLE over a produced closure compiled to the closure as DATA where the interpreter re-steps it and applies: `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]] end 5 (mk 3) 0 pick` answered `[5 fn (Integer) fn (Integer)]` compiled for the interpreter's `[45]`, its `1 roll` twin `[fn (Integer) 5]` for `[15]`, and two more witnesses (`9 (mk 3) 9 2 roll`, `7 (mk 3) 1 pick`) the same way — exit 0, silent, on the DEFAULT lane. Measured on the merge base `d65f25a`, so it PRE-DATED the residual rebuild it was found reviewing. `FoldFullStack` now declines pick/roll when a preserved entry is both event-produced and provably a Function, and the residual rebuild carries the wider possibly-callable screen | verifying a Codex P1 on PR #447, 2026-09-10 |
| [NUR130](#nur130) | FIXED 2026-09-25 (the condition's own token — the handoff log's entry of that date): A terminal trap's caret is the RECORDED site, the interpreter's is wherever its tape pointer sat: `while [] [1] end 5` raises the identical `runtime_error: while: condition produced no value` on both lanes, at `1:7` (the condition operand) compiled and `1:14` (the trailing `5`) interpreted, and the bare `while [] [1]` is `1:7` compiled against `source position unknown` interpreted. Message, code and exit agree; only the anchor differs, and the compiled one is the better anchor — the interpreter's is a tape artefact of where the loop's move token happened to sit after splicing | the forty-second increment's empty-condition trap, 2026-09-10 |
| [NUR112](#nur112) | FIXED 2026-09-25 (the plain check's stored member — the handoff log's entry of that date): on a PLAIN check a stored fn value read as a member is the value itself, so the pass applies it over what follows as the run does (`def m {a:size/v}  m.a [1 2 3]` checks [Integer]); the compile pass keeps its dynamic carrier and the shaped method model. The earlier text: NARROWED 2026-09-25 (the extension is irrelevant; the widening is the member read's designed model — the record's resolution line): The checker's residual for a parked native word applied after its name was EXTENDED does not match what runs: `def Pos (refine Integer)  def m {a:size/v}  def size fn [[n:Pos] [Integer] [200]] end  def v:Pos 3  m.a v` is checked `[dynamic(Any) Pos]` — two values, one of them the argument left behind — and actually leaves `[Integer]`. Both ENGINES agree on the answer (3); it is the static model that differs, so no differential can see it — TestCheckTypeSoundness can, and did | writing a corpus row for the parked-native apply gate, 2026-08-29 |
| [NUR009](#nur009) | FIXED 2026-09-26 (Bytes a refinement base, a computed bound the run's — the handoff log's entry of that date): the refinement bases are DECLARED — `core.DeclareRefinementBase` stamps a type's capability where its owner registers it (core its six leaves, basic Bytes), and `canonicalBaseType` reads the declaration, hand-listing nothing; the inline-signature resolver slots a refinement at its own base (a Bytes one was a wildcard), a refinement renders as one over Bytes' Formatter, and `convert Bytes <String>` folds over a const so a named Bytes refinement compiles. The original text: Bytes excluded from the DepScalar refinement bases — VERDICT 2026-08-15: WAIT for the ADR-012 `types/go` consolidation to close this through the refinement-base capability; no narrow fix meanwhile | 2026-07-22 uniformity review |
| [NUR026](#nur026) | FIXED 2026-09-26 (one escape vocabulary, one malformed-escape report — the handoff log's entry of that date): every string form reads every escape alike in both ports — the braced `\u{…}` form and split surrogate pairs are live in a template too — and a malformed `\x` / `\u` is refused alike, naming the escape: a template's literal matcher and a new quoted-string matcher answer to one definition (`escapeFault`). The original text: Escape sets diverge between quoted strings and templates — NARROWED 2026-08-15: the escape VOCABULARY is resolved by fix (templates take the quoted-string set: \b \f \v \xNN \uNNNN, and an unknown escape drops its backslash); what remains is the malformed-input REPORTING difference, which needs an error channel the template lexer seam does not have | 2026-07-22 uniformity review |
| [NUR072](#nur072) | FIXED 2026-09-26 (canon spells the sugar and the word — the handoff log's entry of that date): canon renders a plain Word bare (ADR-015 settles the bare-word question: `word(foo)` re-parses as the `word` splice over a group, bare `foo` re-parses to the Word), the lambda marker `=>` and its fold group `A => B` without parens, a mini literal `+name'src'` in one canonical delimiter with the lexer's escapes, the type bound `name/t`, and a group modifier after its group (`(1 2) /s`) — in core/go and core/ts alike; the TS `/N` arity is a bigint, so `x/9223372036854775807` round-trips in both ports and left divergent.tsv for parse.tsv; a Go disjunct canon arm (missing, it spelled its members in the debug form) matches TS. A fixpoint gate over the parser corpus runs in both runners with a shrink-only ledger (parser/spec/canon-fixpoint.tsv) — NUR072's kinds all reach their fixpoint; the 33 ledgered rows are NUR225–NUR227. The original text: Three sugar kinds (mini, type-bound, lambda) still canon in DEBUG form after NUR059 — withdrawn there because the renders do not round-trip: SugarInfo does not retain the mini delimiter, and type-bound renders its Items rather than the bound's text; also carries the undecided bare-word question (`word(foo)` vs `foo`, 175 corpus rows) | NUR059's fix, 2026-08-15 |
| [NUR075](#nur075) | FIXED 2026-09-26 (eq's capability — the handoff log's entry of that date): `eq` is extensible per type on `deq`'s terms — `core.ExactEqualer`, consulted at ExactEqual's terminal `false` exactly where DeepEqualer sits in DeepEqual (so the two reach the same values: the pairs no kernel arm names), and a `behave eq/q` slot with deq's shape (`[[T T] [Boolean]]`) and deq's seam (delegate, decline, re-entry guard). Kernel identity arms are untouched — the capability is additive, as deq's is. The original text: `deq` is extensible per type (`DeepEqualer`), `eq` is not — the one part of the retired NUR031's verdict its fix did not take: the divergences closed by adding kernel arms rather than by routing through `Behavior`, so a type can define its own deep equality but not its own identity | NUR031's fix, 2026-08-16 |
| [NUR076](#nur076) | FIXED 2026-09-26 (the check pass notes a behave make — the handoff log's entry of that date): `behave`'s check-mode half (its ReturnsFn) validates the call as the handler does and, for the `make` slot, notes the target in the pass's own state (`CheckState.BehaveMakers`), which `HasMaker` reads — so a construction after the call skips the schema validation the type's own constructor replaces, exactly as a Go-side Maker's does; one before it validates, as the run has it. Nothing is installed on the type, so no user body runs during analysis; the other seven slots change only what a program computes, which analysis does not evaluate. `def P class {a: Integer}  behave make/q (fn Any P [make P {a: 42}])  make P {bogus: 1}` checks clean and compiles (Class/P{a:42} on both lanes). The original text: A `behave`-installed capability is invisible to check mode, because `behave` does not run there — for `make` that turns a working program into a check FAILURE: a type whose Maker ignores the schema still has the schema's unknown/missing-field rules applied statically | NUR056's fix, 2026-08-17 (flagged by the PR #379 review, Codex P1) |
| [NUR060](#nur060) | FIXED 2026-09-26 (the parity ledger is empty again — the handoff log's entry of that date): all nine classes fixed in both ports and moved to parse.tsv with neighbours — a bodiless `=>` where the arrow folds is refused on the arrow, a typed list child with no value is an empty element, a `]` never closes a list no `[` opened, the first fault in source order is reported, an unclosed member group is an unmatched paren, a bare `/` modifier is a syntax_error, an empty `${}` contributes nothing. The original text: The parser twins disagree on open-input sources beyond the corpus | PR #337 parity-probe sweep (flagged for NUR by Codex P1) |
| [NUR063](#nur063) | FIXED 2026-09-26 (boru:scry ships the seven, the debug copies deprecated — the handoff log's entry of that date): `boru:scry` ships `words`, `defs`, `modules`, `sig`, `body`, `deps`, `shape` from the one constructor boru:debug's frozen copies use (`selfKnowledge`), and `describe` marks each `Debug.*` copy deprecated, naming its `Scry.*` twin and the removal release — the maintainer's verdict, implemented. The original text: Seven self-knowledge words are proposed to dispatch from two module surfaces (`boru:debug` and `boru:scry`) — VERDICT 2026-08-15: `boru:scry` canonical, the `boru:debug` copies frozen behind shared handlers and deprecated on a stated timeline | design/BORU-SCRY.0.md §6 (flagged for NUR by PR #344 Codex P1) |
| [NUR064](#nur064) | FIXED 2026-09-26 (add patterns bind as receive clauses do — the handoff log's entry of that date): a service `add` pattern is read by the one clause-pattern splitter `receive` uses (`splitClausePattern`) — scalar fields route, `name:Type` fields are binding slots that decide whether the routed handler takes the request (falling back to a slot-free catch-all, else `no_match`) and are bound by name around the handler's run; `add`'s check-mode half notes the handler body's reads of a slot so the undefined-word rescue excuses exactly those tokens. The original text: Pattern clauses route-and-bind in `receive` but route-only in `add` — VERDICT 2026-08-15: defer to the processes/services design line, to be decided when those modules are built | `design/STATE-MACHINES.0.md` §8 (flagged for NUR by the PR #345 review, Codex P1) |
| [NUR065](#nur065) | RESOLVED 2026-09-26 (one set of guarantees for both classifier spellings — the handoff log's entry of that date): open question #7 of design/STATE-MACHINES.0.md is decided in the design, `boru:state` being unbuilt — the fn form declares its output alphabet (`classify: {fn: … yields: […]}`) and returns a class ATOM the machine wraps in the table form's frozen `{event raw}` payload, so alphabet closure (define-time `state_unknown_name` on `yields:`), payload shape and the `state_bad_class` / `state_class_gap` / `state_bad_event` diagnostics are one rule for both; only the mapping inside the fn stays opaque. The original text: Two spellings of the classifier role get different static guarantees: `classes:` is alphabet-closed and diagnosed, `classify:` is neither — VERDICT 2026-08-15: defer to the state-machine design line (its open question #7) | `design/STATE-MACHINES.0.md` §3.6 (flagged for NUR by the PR #352 review, Codex P1) |
| [NUR074](#nur074) | RESOLVED 2026-09-26 (the parameter name is part of the value — the handoff log's entry of that date): not a divergence — the record's premise, that two functions differing only in a parameter name are behaviourally indistinguishable, is false in boru and was refuted by measurement: a parameter is a frame binding on the def stack, visible to every function the body reaches (FUNCTION-VALUE-SCOPE §7.4), so `def x 1  def g fn [[] [Any] [x]]` then `def f fn [[x:Any] [Any] [g]]  f 5` answers 5 and its `y`-named twin 1, on both lanes. canon rendering the name and `deq` comparing it is the uniform answer; the content-addressing note's de-naming step (§4.2 step 3) is withdrawn as unsound. The original text: `canon` renders a function's PARAMETER names, so alpha-equivalent functions render — and digest — differently; NUR031's planned fix (render the anonymous fn literal) does not reach this | `design/legacy/unison-hash-identity-probe.0.ignore` P4 (flagged for NUR by the PR #376 review, Codex P1) |
| [NUR077](#nur077) | FIXED 2026-09-25 (the Apply op — the handoff log's entry of that date): `StackForm`'s op vocabulary can CALL a word by name but cannot APPLY a function value, so an inline lambda or a fn read out of a container has no faithful representation — `Call{Name, Arity}` re-invokes by name and does not consume a receiver. `Eval` now refuses those forms (`ErrUnnamedApply`) rather than replaying them to a different answer — VERDICT 2026-08-17: resolve by fix, a NEW dedicated Apply Op (arity-carrying, consumes the value, seamed at `execFnDefLiteral`; `DoEval` stays reserved), after the three prerequisite recorder/gate defects are fixed | the `OnCall` frame-skeleton over-count fix, 2026-08-16 |
| [NUR078](#nur078) | FIXED 2026-09-26 (NUR078 closed — a bare fn name calls at every slot — the handoff log's entry of that date): A bare fn name before a `Function`-typed slot still resolves as a reference, against amended ADR-011 — the engine's TFunction intercept implements the exception the 2026-08-17 amendment struck (`h zero` ≡ `h zero/r` when the slot is `Function`-typed; a call/barrier before any other slot) — VERDICT 2026-08-17: resolve by fix, open-work item B (all four sites retire together, re-opening the NUR038 call-head question in the implementing PR) | the ADR-011 amendment, 2026-08-17 (flagged by the PR #381 review, Codex P1) |
| [NUR079](#nur079) | FIXED 2026-09-26 (NUR079 closed — one policy for a program and its modules — the handoff log's entry of that date): Gated words inside an imported file-module body escape the policy that governs the same call at top level — the module sub-registry inherited every capability seam except policy, so gates resolving `HostPolicy(r)` read nil as allow — VERDICT: resolve by fix in two halves; half (i) landed 2026-08-18 (the body now carries the parent's policy), half (ii) open | the Roc comparison study, 2026-08-18 |
| [NUR080](#nur080) | FIXED — verified 2026-09-25 (the brand in both orders — the handoff log's entry of that date): A typed def over an Integer literal loses its newtype brand under the compiler, and the bare literal gains one — VERDICT: resolve by fix | the Roc comparison study, 2026-08-18 |
| [NUR081](#nur081) | FIXED 2026-09-25 (one contract for the family — the handoff log's entry of that date): `Test.skip` is documented as a drop-in for `Test.check-prop`, but the two disagree on argument validity: `check-prop` now rejects `runs < 1` / `max-shrinks < 0` while `skip` accepts them silently | the Quint follow-up Wave 1, 2026-08-18 |
| [NUR082](#nur082) | FIXED 2026-09-25 (one walk for the tree commands — the handoff log's entry of that date): Three tree-walking subcommands, two rules for `.boru/`: `fmt` and (now) `check` skip the package directory, `boru test`'s `discover()` walks it — VERDICT 2026-08-18: resolve by fix, one shared walk helper carrying the skip | giving `boru check` directory targets, 2026-08-18 (W-CLI-CHECK) |
| [NUR083](#nur083) | FIXED 2026-09-25 (the script's own directory — the handoff log's entry of that date): `check` and `build` anchor relative imports to the FILE's directory, `run` and `debug` to the process cwd, so `boru check sub/m.boru` now accepts a program `boru run sub/m.boru` refuses from the same cwd — VERDICT 2026-08-18: resolve by fix, `run`/`debug` adopt the file anchor (the multi-target `check` cannot use cwd at all) | multi-file `boru check`, 2026-08-18 (W-CLI-CHECK) |
| [NUR084](#nur084) | FIXED 2026-09-25 (fmt answers -h — the handoff log's entry of that date): `-h` is not a uniform surface: FlagSet commands print their flags to stderr, `fmt` reads `-h` as a filename, and none exits 0 — though `boru help <cmd>` tells users to run it — VERDICT 2026-08-18: resolve by fix, `fmt` gains a FlagSet and `flag.ErrHelp` exits 0 | `boru check -h` failing as a missing file, 2026-08-18 (W-CLI-CHECK) |
| [NUR088](#nur088) | FIXED 2026-09-25 (six spellings, one form — the handoff log's entry of that date): One signature has six valid spellings; `boru fmt` collapses only ONE of them to the short form, so four survive the formatter untouched and a `fmt`-clean file still carries several spellings of one signature | writing `STYLE-GUIDE.md` §S1, 2026-08-19 (`design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` §4.2) |
| [NUR089](#nur089) | FIXED 2026-09-26 (NUR089 closed — a lambda param binds as the run binds it — the handoff log's entry of that date): An inline `=>` lambda argument and a named `/v` reference to the SAME function are not equally checkable: the reference passes the check, the lambda draws `no_signature: cannot call g … got (Integer)`, and both run to the identical answer | the function-type prototype, 2026-08-19 (`design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` §1.1) |
| [NUR091](#nur091) | FIXED 2026-09-25 (the fn that took nothing — the handoff log's entry of that date): A malformed `fn` declaration fails LOUDLY or SILENTLY depending on its output slot: `fn List [Integer] [size]` raises signature_error, `fn List Any [1]` strands its operands and binds nothing, exit 0 | the function-type prototype, 2026-08-19 |
| [NUR096](#nur096) | FIXED 2026-09-25 (NUR096 closed — the check applies a fn-shape member — the handoff log's entry of that date): The check pass did not move with NUR095: a fn stored through a fn-SHAPE-typed member is APPLIED by both engines but still modelled by the checker as the inert fn it was before that retirement, so `TestCheckTypeSoundness` fails on the two multi-return `class.tsv` rows that pin it | adding the NUR095 retirement rows to `lang/spec/class.tsv`, 2026-08-20 |
| [NUR092](#nur092) | FIXED 2026-09-25 (the stale arm consults the corpus — the handoff log's entry of that date): `varyCompileFailureLedger`'s stale arm is corpus-sensitive: adding an UNRELATED spec row can displace a seed from the hash-ordered 32-seed sample, empty a bucket, and instruct the author to delete a ledger entry whose refusal class is still live at larger breadth | adding NUR091's spec rows, 2026-08-19 |
| [NUR110](#nur110) | FIXED 2026-09-22 (the branch-carried def — the handoff log's entry of that date; the register row brought into line 2026-09-26): A FRESH `def` inside a branch that DID NOT RUN binds the name anyway in the compiled lane: `if false [def op 1] [0] end op` answers `[0 1]` compiled and `undefined_word` interpreted. The family-L CondBodyDepth gate only fires when a redefinition DROPS an existing overload, so shadowing is refused and fresh definition is not | measuring NUR109's premise, 2026-08-28 |
| [NUR122](#nur122) | FIXED 2026-09-25 (the read's own token — the handoff log's entry of that date): A bare-name dispatch of a fn-typed frame binding whose runtime value does not match is a NAMED no-match on the interpreter and something else on the compiled lane: `def f fn [[g:Function x:Integer][Integer][g x]]  f (z:String => [z]) 5` raises `signature_error: cannot call `g`` interpreted and `type_error: f: expected 1 return value(s), got 2 — [fn (String) 5]` compiled (the frame replay parks the value); the paren spelling `(g x)` raises the no-match with an EMPTY name (`cannot call `` `) at the body's position; a 0-arg `g` fires interpreted (`[42 5]`) and parks compiled (`[fn 5]`); and inside a RETURNED closure the same paren spelling over a captured NAMED fn names that fn instead of the param — `def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h (app add/v)  (h 5)` raises `cannot call `g`` at 1:64 interpreted and `cannot call `add`` at 1:80 compiled, both exit 1 (measured 2026-09-05 on main, a COMPILING row). Same shape family as NUR119: the compiled value has no binding name and no named-dispatch semantics RE-MEASURED 2026-09-06: the NUR123 deopt increments moved two of the three without this record being touched — `f (z:String => [z]) 5` now AGREES in message and position (`cannot call `g`` at 1:43), and `f ([] => [42]) 5` agrees in message (`[42 5]`, was `[fn 5]`) with only its POSITION still differing (interp 1:50, compiled 1:43); the returned-lambda `(g x)` row is unchanged. What remains is one position-only divergence plus the lambda-VALUE body's empty dispatch name. FIXED 2026-09-06 (the nineteenth increment) for the DISPATCH NAME and its position: the trailing apply's head carries the binding name and read position it was recorded under (`CompiledFn.DynApplyName`), so `(g 5)` raises `cannot call `g`` at the read on both lanes — message, code, caret, candidate notes and forward-args help alike — for a plain fn body, a lambda VALUE's body and a capture (`app add/v` named `add` at 1:80 and now names `g` at 1:64). What remains is the position-only row (`f ([] => [42]) 5`, interp 2:1 vs compiled 1:43) and the WRITTEN-TUPLE half already recorded below: a WORD argument is substituted onto the value stack before the head dispatches, so the interpreter's tuple is empty (`takes 1 argument, but none were supplied`) where the compiled lane names the applied value. FIXED 2026-09-07 (the twenty-first increment): both ops hand NoMatchDiag the leading run (fnUnitRec.localReads → DynApplyHead.NWritten and DynFrameWord.Read), and all eight witnesses reach parity. The rule the nineteenth increment recorded was wrong three ways — the tuple is a PREFIX not a filter (`(g 5 y 6)` writes `[5]`, not `[5 6]`), the gap spans OpCallDynFrame as well as the trailing apply (multi-arg applies lower there), and the discriminator is SYNTACTIC not an operand kind (a body-local bound to a literal folds to PUSH_CONST and is still not written) — so closing it needs the SYNTACTIC fact rather than the lowered operand — which `NoteLocalRead` already records ungated for every bare read (`rec.localReads`), so no kernel change and no relaxation of noteWordRead's fn gate. RENAME half FIXED 2026-09-07 (the twenty-sixth increment): the interpreter's frame binding NAMES a fn value bound for a named param (`installDef`: `fnDef.Name = name`) and the VM bound it verbatim, so a COMPILING row rendered differently — `def f fn [[g:Function][Function][g/v]]  (f (z:Integer => [z])) 3` was `fn g(Integer) 3` interpreted and `fn (Integer) 3` compiled; `nameFrameFns` now names an `FnDefInfo` of this registry at every frame entry, and the row, its returned-closure twin `[g/v]` and the nameless-builder no-match (`(g/v 5)` → ``cannot call `g` `` on both lanes) agree. STILL OPEN: a MODULE WRAPPER bound for a named param keeps its own name and signatures on the compiled lane (`(f MathUtil.sqrt/v) 16.0` renders `fn sqrt(Number) 16.0` for the interpreter's `fn g(BigDecimal) or (BigInteger) or (Float) or (Integer) 16.0` — installDef's rebinding path installs the inner native's overloads under the param's name, a registry-visible install the VM does not mirror; the wrapper still dispatches), and the position-only row `f ([] => [42]) 5` (interp 1:50, compiled 1:43). | measuring the closure-capture family's blocker (a), 2026-09-05 |
| [NUR123](#nur123) | FIXED 2026-09-25 (the guarded gradual read — the handoff log's entry of that date): A BARE READ of a frame binding holding a fn is a WORD dispatch on the interpreter (stepWord routes a bound FnDefInfo through Registry.Lookup: a 0-arg fn fires, a no-match raises `cannot call `g``) and was a slot PUSH on the compiled lane: `def f fn [[g:Function][Any][g]]  f ([] => [42])` answered `fn` for 42, `def id fn [[x:Any][Any][x]]  id ([] => [42])` the same, `f (z:Integer => [z])` answered `fn (Integer)` for the interpreter's error — default lane, exit 0. Fixed for the fn-body residual (the replay re-steps the read as the word; fn-typed reads elsewhere refuse); FIXED for a gradual body-local's read (`def h fn [[m:Map][Any][def j (m get "f")  j]]  h {f: ([] => [42])}` is 42 on both lanes: its residual read seats the replay, and any other consumption — `j typeof`, `{a: j}`, `(j typeof)`, `(j) typeof`, `if true [j] [0]`, `for 1 [j typeof]`, `j (m get "g") add`, `j  def y 1`, `def k j`, `def k (j)` — deopts to the interpreter at its statement when the binding holds a fn; a point whose statement the compiled stack cannot match declines to the slot push: `5  j typeof` raises `[5 Function]` for the interpreter's `[5 Integer]`); FIXED inside a CODE BODY a native runs in the frame (`[1] each [j]` [42], `do [j]` 42 — the closure unit deopts on its capture slot and the enclosing unit binds the name); FIXED for a lambda VALUE's own body (2026-09-06, the fourteenth increment: `def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)` is 42 on both lanes where it answered `fn`, and the `j typeof` twin `Integer` where it answered `Function` — the escaping unit seats its body tokens and plans from its OWN frame, whose captures ride in slots for the whole apply; a read inside a BRANCH ARM of such a body still keeps its slot push); OPEN for the declined points and NUR119's render (a gradual PARAM's read refuses instead: the pass re-runs it under the argument's runtime type; re-measured 2026-09-05) | measuring the closure-capture family's blocker (a), 2026-09-05 |
| [NUR126](#nur126) | RESOLVED (2026-09-05, the twelfth and thirteenth increments). A RETURNED lambda's captured COMPUTED value was baked as an unrelated CONSTANT: `def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)` answered `7` — the caller's own argument — for the interpreter's `42`, and the factory's bytecode DROPped the computed value and pushed `PUSH_CONST` over the producing event's SEQ read as a const index (the disassembler panics on it: index out of range). The promotion planner never counted a RESIDUAL closure's captures, so their producers got no frame slot. The thirteenth increment closed the nested-fragment half: a multi-value branch ARM's residual is now RECORDED whole (`captureArmResidual`), so the planner may walk into the arm and force-promote its residual events — which also fixed the arm's own count-only reconciliation (`if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]` answered `7778 [7778 7779]` for the interpreter's `99 [7779 7780]`). An arm whose residual LEADS with a parked Function still declines the capture (the auto-apply screen) — the corpus's two vault-tui sites | measuring the closure-capture family after the eleventh increment, 2026-09-05 |
| [NUR124](#nur124) | FIXED 2026-09-25 (the value-delivered window parks — the handoff log's entry of that date): TIMING axis FIXED 2026-09-07 (the twenty-fourth increment): a native word's fn-typed or fn-admitting result with a plain body token written after it is noted by the pass (`NoteFnResultReStep`), the unit plans a RE-STEP point right after the call, and the VM hands the results as TAPE TOKENS plus the rest of the body to the interpreter when one is a fn (`DEOPT_IF_FN` with `Results`, the unit's unpushed unnamed inputs seated beneath the region) — `[g/v] each [5 swap drop]`, `[5 swap drop 9 swap]`, `[5] each [m get "f" drop]`, `[5] each [{f: g/v} get "f" drop]` and the fn-body twin all agree; a fn-typed note no point serves REFUSES instead of miscompiling. PAYLOAD axis FIXED the same day (the twenty-fifth increment): the interpreter's value re-step bridges a compiled closure to a dispatchable fn (`CompiledRuntime.ClosureAsFnDef`, Anonymous as the source lambda so a 0-arg lambda VALUE still parks), the re-step deopt's test admits a closure, and — found by the bridge — the residual window islands (`OpCallDynamicMixed`) no longer re-step a PARKED user-fn result: `5 (mk 3) 7` and `1 2 (mk 3)` lay out as the parked data on both lanes, and their NAMED twins `5 (mkf 3) 7` / `1 2 (mkf 3)` (which answered `5 21` / `1 6` on main for the interpreter's parked pairs) with them. STILL OPEN: a static-index FOLD handing the fn-typed local itself to the re-step (`[5 h/v] get 1 drop 7` → 7, no event to test after), and a fn-typed value re-stepped at the MAIN program (`def m {f: g/v}  m get "f" drop 7` → 7; a gradual note with no unit body to resume into keeps the optimistic model). The earlier text of this row follows. TWO defects (re-measured 2026-09-07), not the one this row first described. TIMING, which is not about closures: the interpreter re-steps a shuffled fn AT the shuffle and the compiled body at BODY END if at all, so a plain FnDefInfo diverges as soon as anything follows the shuffle — `def g fn [[x:Integer][Integer][x mul 3]]  [g/v] each [5 swap drop]` raises each_error interpreted (g applies at the swap, leaving [15], which drop empties) and answers `[5]` compiled. PAYLOAD: a produced closure is not re-stepped even at body end. The original witness: `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  [(mk 3)] each [5 swap]` is `[15]` interpreted and `[fn (Integer)]` compiled; `[5 over]` is `[45]` / `[fn (Integer)]`; `[5 swap drop]` raises each_error interpreted and answers `[5]` compiled; and the OPPOSITE direction, measured 2026-09-06 and pre-existing on `6f583f6`, where the compiled lane APPLIES where the interpreter parks — `5 (mk 3)` is `5 fn (Integer)` interpreted and `15` compiled, with the `(mk 3) 5` twin agreeing, which places the fault on the value's ARRIVAL above an existing residual — default lane, exit 0. The top-level spellings refuse ("function value reaches swap (Stage 3)"); inside a code body over a produced closure the shuffle fold compiles. A FIFTH witness (2026-09-06, likewise pre-existing) runs the same way through the QUOTE rather than the park rule — `def appv fn [[g:Function][Integer][(g/v 5)]]  appv (z:String => [z])` parks `[fn g(String) 5]` interpreted and applies compiled, because callDynTrailTop strips the applied copy's quote to mirror a read-substituted arrival while a `/v` delivery hands the stored value over still quoted | measuring NUR123's closure bridge, 2026-09-05 |
| [NUR119](#nur119) | FIXED 2026-09-25 (the param's name on both lanes — the handoff log's entry of that date): A fn value read through a PARAM's `/v` renders under the PARAM's name on the interpreter and under its own name on the compiled lane: `def app fn [[g:Function][Function][g/v]]  (app (z:Integer => [mul 3 z]))` renders `fn g(Integer)` interpreted and `fn (Integer)` compiled, and `def sq (z:Integer => [mul z z])  … app sq/v` renders `fn g(Integer)` against `fn sq(Integer)`. Same value, one render — the interpreter's frame binding re-labels the fn under the name it is read through, and a compiled unit pushes the raw runtime value. Pre-existing for the paren-placed spelling; the bare and args-following spellings refuse (`unconsumed fn-value carrier in residual (closure render)`, callResultRenderKnown) rather than diverge | measured 2026-09-05 while fixing the returned-closure park |
| [NUR118](#nur118) | FIXED 2026-09-25 (the call's own token — the handoff log's entry of that date): A compiled fn's RETURN-CONTRACT error blames the body's first token where the interpreter blames the CALL SITE: `def f fn [[m:Map] [Integer] [m get "a"]]  f {a:"s"}` raises the same `type_error: f: return value 1: expected Integer, got ProperString` on both lanes, at `1:43` (the call) interpreted and `1:30` (the body) compiled. The compiled RET check is stamped with the unit's own position because one unit serves every call site (compiler StartFnCompile's fnPos) — a documented limitation that became reachable from a corpus-style row when the binding-sensitive unit memo let `def k 5  def f fn [[] [Integer] [k add 2]]  f  def k "x"  f` compile (Stage 4b); the row pins code and message and excludes the position | Stage 4b's cross-family rebind row, 2026-09-04 |
| [NUR114](#nur114) | FIXED — verified 2026-09-25 (the token's own text — the handoff log's entry of that date): A compiled diagnostic's caret is always ONE character wide where the interpreter underlines the whole token: the compiler's debug table is `[]core.SrcPos` carrying only row and column, so `stampAt` has no token text to set `BoruError.Src` from and the renderer's `caretCount = len(sub)` falls to its minimum of 1. Found while closing NUR108 — the positions now match exactly and the underline still does not | closing NUR108, 2026-08-30 |
| [NUR109](#nur109) | FIXED 2026-09-25 (the parser name bound on a branch — the handoff log's entry of that date): `parse` over an unbound def-scoped parser name raises `parse_error: the parser is not a usable function value` compiled and `parse_unknown_lang: no parser "op" is registered` interpreted. `TestParseFnDispatchMissParity`'s own header argues the compiled answer is the right one and asserted both lanes gave it — reading the interp side from `Run` | completing the NUR106 oracle sweep, 2026-08-27 |
| [NUR101](#nur101) | FIXED 2026-09-25 (the verdict of 2026-08-27 landed and its last shape graduated — the handoff log's entry of that date): BROAD's placement depended on ENCLOSING CONTEXT: `(mk 1) 2` places (`fn (Integer) 2`) while `((mk 1) 2)` dispatches (`3`). **RULED 2026-08-26 "place uniformly"; the ruling's PREMISE was then FALSIFIED 2026-08-27** — the survivor count IS the question, and the enclosing group is a SECOND decision, not a modifier of the first. The interpreter was right all along; the COMPILER carried five silent miscompiles in both directions, hidden by 75 parity assertions that use post-Stage-J `Run` (the compiled path) as their interpreter oracle. See [design/PAREN-RESTEP-RULE.0.md](design/PAREN-RESTEP-RULE.0.md) | re-measuring §5.4 after #402, 2026-08-25; ruled 2026-08-26; ruling's premise falsified by measurement 2026-08-27 |
| [NUR099](#nur099) | FIXED 2026-09-25 (NUR099 closed — a capitalised fn body is refused — the handoff log's entry of that date): `def <Capitalised> <fn>` is the ONLY door to an arbitrary predicate type, so it must stay ambiguous: the same fn body means a callable function under a lowercase name and a membership test under a capitalised one, and `def K fn [[a:Any b:Any][Any][a]] end K 1 2` therefore binds an uninhabitable type in silence — VERDICT 2026-08-25: resolve by fix, a `fnpred` word analogous to `fnsig` — **HALF LANDED 2026-08-25**: `fnpred` ships and the explicit route is live; what remains is migrating the 150 corpus sites off the capitalised-fn form and deleting the arity route behind it | reviewing the §5.1 diagnostic, 2026-08-25 |
| [NUR100](#nur100) | FIXED 2026-09-26 (the predicate as a one-value application — the handoff log's entry of that date): neither site counts a function's parameters any more. §1: membership is a ONE-VALUE APPLICATION — `RunPredicate` matches the candidate against the predicate's signatures with MatchFnSig, the one matcher every call takes, and runs the signature that takes it; a candidate no signature takes is not a member (so the WHOLE overload set is consulted where only the first was, a value pattern selects as for any call, and a two-parameter predicate is a type no value inhabits — `def v:K 5` answers `does not satisfy`, never `predicate must take exactly one argument`); `PredicateInputType` is the overloads' COMMON input. §2: tryRecordPoly's decline keys on a reachable overload that DECLARES CompileResteps (`restepOverloadReachable`) — what the smaller-arity count stood in for; `smallerArityOverload` is gone. The aritygate pins tightened (registry.go 3→2, compiler_dispatch_record.go 2→1). Found on the way and fixed: NUR223 (the callback seam returned an unconsumed unnamed param beneath a predicate's verdict) and NUR224 (a predicate's typed-def refusal was a plain error interpreted and a compiler-defect internal_error compiled). The original text: ADR-016 ("arity and origin never change function behaviour") is contradicted by live code: `RunPredicate` admits or refuses a function as a predicate purely on its parameter count, and `smallerArityOverload` gates a compile refusal the same way | the maintainer's ruling that the ADR-016 rule is absolute, 2026-08-25 |
| [NUR097](#nur097) | FIXED 2026-09-25 (the late-binding hint — the handoff log's entry of that date): One syntax, two binding regimes: a closure CAPTURES parameters and fn-locals but resolves module-scope names LATE through the def stack, so a later `def` silently changes an existing closure's answer — verdict proposed: Allowed (top-level liveness) plus an in-file check hint | the higher-order capability audit's §5.6, re-assessed 2026-08-21 (`design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore`) |
| [NUR102](#nur102) | FIXED 2026-09-25 (one run per dispatch — the handoff log's entry of that date): A predicate body runs a different number of times in each lane — overload pruning evaluates it 4× interpreted and 2× compiled, an effect-count divergence no differential gate can see because both lanes return the same value | the Stage-2 collection-kernel feasibility probe, 2026-08-25 |
| [NUR103](#nur103) | RESOLVED 2026-09-25 by the diagnostic-surface gate (the record's resolution line; the mini-redis instance survives only in the full module context and is ledgered): The checker's answer depends on who is asking: the same program yields a clean `boru check` and a refusing compile, so the tool a user would reach for reports the program fine — **one instance fixed 2026-08-26** (a nameless `undefined_word` from a Word-typed carrier); the mini-redis instance is diagnosed and OPEN, and its first fault is a coverage hole — `boru check` does not analyse a service-handler body at all, so a bare undefined word inside one ships clean | the server-concurrency corpus, 2026-08-26 |
| [NUR105](#nur105) | FIXED 2026-09-25 (the folded map member — the handoff log's entry of that date): `boru check` does not analyse the body of a function VALUE constructed in ARGUMENT position — all three anonymous spellings (`=>`, `fn`, `afn`) — so `each ([e:Any] => [nosuchw e]) [1 2 3]` checks clean and then raises `undefined_word` at run time, an error the checker catches without difficulty when the identical body is `def`-bound. A code BLOCK argument and a named fn REFERENCE are both analysed, so position decides it, not spelling; measured matrix in the record | bounding NUR103's mini-redis half, 2026-08-26 |
| [NUR104](#nur104) | FIXED 2026-09-25 (Allowed — the install-time resolution is the uniform rule; the handoff log's entry of that date): A record type means two different things depending on how it is spelt: the named spelling DISPATCHES and so evaluates its field map, the inline `o:{…}` rides inside the inert fn-spec list and never does — **the two field shapes found so far are fixed 2026-08-26**, the construction asymmetry is not | tracing NUR103, 2026-08-26 |
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

**Status:** FIXED 2026-09-25 (verdict: resolve by fix) · **Recorded:**
2026-08-25 · **Surfaced by:** the maintainer's review of the §5.1
`stranded_type_call` diagnostic

**The fix.** A capitalised name over an UNDECLARED fn body — a fn
definition or a compiled closure under Function — is refused at the
declaration with `def_error` (`InstallTypeBody`, via `isFnBodiedValue`),
naming both ways out: `def K fnpred …` for a predicate type, or a
lower-case name for a function. A type literal that merely sits under
Function (a module's exported member type) keeps binding as an alias. The
arity route is gone: `isPredicateFnValue` is deleted, `PredicateInputType`
answers only for a declared predicate, and unify's predicate-reference arms
key on `IsDeclaredPredicateFn`. `def` installs during the check pass, so
both lanes refuse. The corpus moved to `fnpred` first — the spec rows, the
Go test sites, core's fixtures, and the design examples' `def New fn …`
constructors (now `def new fn …`, exported under the same `New` key).
`stranded_type_call` keeps the one fn-bodied type node left: a declared
predicate written as a call. Pinned in `lang/spec/fnpred.tsv` §5 (the legacy
spelling and `def K fn [[a:Any b:Any][Any][a]]` refused, the lower-case
combinator callable), `lang/spec/edge-types-2.tsv` (the lambda spelling
refused), `core/go/nur099_fnbody_test.go`, and
`lang/go/test/stranded_type_call_test.go`. NUR100's §1 is untouched: a
DECLARED predicate of the wrong arity still reaches `RunPredicate`'s count.

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
`stranded_type_call` reports (`design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` §5.1).

---

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
operand count — removed.) Found on the way and fixed: NUR223 and NUR224.
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

## NUR146 — the compiled undefined_word's did-you-mean pool has no frame bindings {#nur146}

**Status:** FIXED 2026-09-25 (the frame's names — the handoff log's entry
of that date). Recorded 2026-09-16, the sixty-eighth increment.

**The fix.** The pool's compiled half is the running unit's slot→name
table: `localNameCandidates` (eng/go/vm.go) hands a fn unit's
`CompiledFn.LocalNames` or the main unit's new `Program.LocalNames` to
`UndefinedWordDiagWith` at every VM undefined_word site (the dyn-scope
miss, the bound-local read, the two generic-dispatch misses). The tables
name what the interpreter's registry holds as a def and a compiled frame
keeps in a slot: params and captures (already there), a loop variable
(`AnalyseLoopBody` names the local it registers through the new
`EmitRecorder.NameLocal` seam), a carried or arm-bound def
(`emitUnit.nameSlots` / `boundLocals`, merged at unit close by
`unitSlotNames`); spill temps stay anonymous. `def k 5  for 2 [ if (k eq
5) [undef k] [] ]  9` reads `did you mean \`i\`?` on both lanes. Pinned
by lang `TestCompiledUndefinedWordSuggestsFrameNames`.
**Found:** by the placed speculative undef's parity rows
(`lang/go/spec_undef_placed_test.go`): the first row raising inside a loop
agreed on its first line and disagreed on the help line.

**Rule:** one diagnostic. An error the two lanes both raise renders the
same, suggestions included — the did-you-mean is part of the message a
user reads.

**Divergence.** The VM raises the interpreter's `undefined_word` from the
seam (`core.UndefinedWordDiag`, the sixty-sixth increment's builder — the
routed dispatch's unbound slot and, since the sixty-eighth, a live read a
placed undef made miss), and the builder's near-miss pool is the
registry's `SuggestionCandidates()`. The interpreter's registry at the
raise holds every frame binding as a def — a loop iterator, a fn's
params, its body-locals — because that is how the interpreter binds them;
the compiled program holds them in FRAME SLOTS the registry never sees
(`def k 5  for 2 [ if (k eq 5) [undef k] [] ] 9`: `i` is `l0`). So the
interpreter says ``did you mean `i`?`` and the VM says nothing; with a
fn in scope, ``did you mean `f` or `i`?`` against ``did you mean `f`?``.
Code, detail and position agree.

**Fence.** The placed-undef rows compare the error's first line
(`requireParityHead`) and the position, and say so at the site.

**Verdict:** none yet. The candidates are the VM's to supply: a unit knows
its slot names (`CompiledFn`'s local table), and a raise from inside a
unit could pass them to the builder as extra candidates — an over-supply
where the interpreter's binding was already torn down, an under-supply
for a name bound only in an ENCLOSING frame. Whether the pool should be
the frame's or the message should drop the suggestion on both lanes is
the ruling to seek; either is small.

## NUR143 — a fn-body read of a module-scope flex is a snapshot clone, not the binding {#nur143}

**Status:** FIXED 2026-09-24 (the recorder's registry after a module body
— the handoff log's entry of that date). RECORDED 2026-09-15, under
review of #458.

**The fix.** The enclosing-binding snapshot a fn unit takes at its open
(compiler `snapshotAllBindingIDs` and its two siblings, at
`StartFnCompile`) read the RECORDER's binding — the registry of the last
engine that ran, which after `import "boru:sift"` is the last NESTED
import's (sift imports three) and, once `BindRegistry` restores, the
program's; in neither is the module's flex a binding, so the read fell to
the const clone. The snapshots read the unit's own registry now (`fnReg`,
which every caller passes), a module fn's body reads its module-scope
flex as the live binding wherever the unit is compiled, both descriptors
stop reproducing and their `regionOracleFindings` entries are retired.
Pinned in `lang/go/module_fn_unit_registry_test.go`, where the unfixed
tree was a SILENT miscompile: `R.put 'a' 1  R.count` over an inline
module's flex answered `[1 0]` for `[1 1]`, and `R.peek` a type error
over the clone's None. The original record follows.
**Found:** by the COLLECT oracle, once its agreement test was identity
(`oracleSameValue`): two descriptors in `module-sift.tsv` whose live word
slot resolves to a flex map that RENDERS as the pushed operand and is not
the same store.

**Rule:** one binding store. A read of a name is the binding — the same
object the interpreter's lookup hands back — in every lane; a copy is a
different value the moment anything mutates either.

**Divergence.** `boru:sift` keeps its catalogue in module-scope flex maps
(`def sift-catalog (flex {})`, `def sift-path-detect (flex {})`,
sift.boru:29–30) and reads them from exported fn bodies:

```
def sift-w-kinds fn [[] [List] [ each [convert Atom] (keys sift-catalog) ]]
```

compiles to

```
fn f0 sift-w-kinds/0 (locals=0):
0000 PUSH_CONST_FRESH
0001 CALL_NATIVE s0   ; keys (Map)
```

— the catalogue read is a const, and `OpPushConstFresh` pushes a deep clone
of it (CloneValue) on every call. The interpreter pushes the binding. The
oracle sees `sameContainer` false with equal contents.

**Why it is not (yet) a wrong answer.** Two accidents, measured:

- Within one request the check pass PERFORMS the mutations the run
  performs: `Sift.define zz …  Sift.kinds` dry-passes `sift-store-kind`'s
  `set` on the concrete flex during analysis, so the snapshot taken at
  record time already holds `zz`, and the run's `keys` over the clone
  agrees with the interpreter (probed: identical output on both lanes).
- Across requests the memo REFUSES rather than reads stale: request 1
  `import "boru:sift"  Sift.kinds`, request 2 `Sift.define zz …`, request
  3 `Sift.kinds` compiles to "operand of unknown provenance or not
  statically materialisable at keys" — a refusal, and a defect of its own,
  absorbed today by the interpreter, which answers seven kinds.

A mutation the check pass cannot perform (a `set` whose key is a value the
analysis widens) followed by a read in the same request is the shape that
would answer wrong; none is in the corpus.

**Fence (until the fix).** The two descriptors were ledgered by name in
`test/go/langspec/region_oracle_test.go` (`regionOracleFindings`), pinned
in both directions; retired with the fix.

**Verdict:** none yet. `OpPushConstFresh`'s own doc says an ENCLOSING
binding's value read by name keeps the SHARED push; a module preamble's
def is such a binding to the module's fn bodies, and the freshening did not
treat it as one. The generic lane's live read (Stage 4) replaces the
snapshot with the binding for every such site; whether the freshening rule
should be narrowed before that is the ruling to seek.

## NUR142 — a refined container is eq to nothing, not even itself {#nur142}

**Status:** FIXED 2026-09-25 (the family of a refinement — the handoff
log's entry of that date). Recorded 2026-09-15, under review of #458.

**The fix.** `containerFamily` (core/go/equal.go) walks a USER-minted tag
(`Origin == OriginUserDef`) to its nearest kernel ancestor and then
applies the exact flex fold; the six equality arms — `ExactEqual`,
`DeepEqual`, the deq-key scans — ask it, so `def S (refine FlexMap)  def
w:S (flex {a:1})  w eq w` is true, as are the refined list and map rows
and `[w] deq [w]`, on both lanes. Ordering keeps the exact `nodeFamily`:
a refinement may carry its own Comparer (`behave`), which the LCA walk
must reach before any fold (the comparison suite said so when the first
cut folded ordering too). `same_container_test.go`'s fence assertion is
retired into its positive form. Pinned by lang
`TestRefinedContainerIsEqToItself` and refine-flex.tsv §3.
**Found:** by the COLLECT oracle, the moment its agreement test became the
`eq` word's rule (`core.ExactEqual`) instead of structural equality: 22
corpus descriptors over refined flex bindings (`refine-flex.tsv`,
`as.tsv`, two `module-sift.tsv` rows) read as `diverged-value` although
the pushed operand and the bound value were ONE store under ONE tag.

**Rule:** one equality per family. `eq` is identity for the container
family — the same store, the same tag — and a value is eq to itself
whatever its tag's ancestry; `deq` is structure. A refine of a container
type is a member of the family (dispatch admits it as one, `is` says so).

**Divergence.**

```
def w (flex {a:1})  w eq w                              true
def S (refine FlexMap)  def w:S (flex {a:1})  w eq w    false
def L (refine FlexList)  def v:L (flex [1 2])  v eq v   false
def M (refine Map)  def m:M {a:1}  m eq m               false
def S (refine FlexMap)  def w:S (flex {a:1})  [w] deq [w]   false
```

`ExactEqual` (core/go/compare.go) reaches the container arms through
`nodeFamily(a.Parent).Equal(TMap)` / `.Equal(TList)`; `nodeFamily`
(core/go/equal.go) folds exactly the kernel's flex and weak-flex nodes to
their family root and returns every other type unchanged, so a refined
tag never equals `TMap` or `TList`, the arms are skipped, the flat-instance
and opaque-handle arms decline, and the function returns its terminal
`false`. `DeepEqual` fails the same way for the same reason.

**Why it stayed invisible.** No corpus row compares a refined container
with `eq`; every refined-container row asks `typeof`, `is`, `size`, a
method, or dispatch — all of which walk the parent chain. Equality is the
one family test that folds a fixed set instead of walking.

**What the oracle does meanwhile.** `core.SameContainer` is the identity
test (same tag, same store) exported from the arm ExactEqual cannot reach,
and `eng/go/region_oracle.go`'s `oracleSameValue` reads it for the
container family — so the oracle answers its own question ("is the pushed
operand the bound object") correctly over a refined binding, and the 22
descriptors reproduce. The `eq` word is unchanged: fixing it is a
language-visible change (`w eq w` becomes true) and is its own increment.

**Fence.** `core/go/same_container_test.go` asserts BOTH halves: that
`SameContainer` holds over a refined flex map, and that `ExactEqual` still
does not — the second assertion is to be retired with the fix.

**Verdict:** resolve by fix — `nodeFamily` should fold by ANCESTRY
(`ConformsTo(TMap)` / `ConformsTo(TList)`, the flex fold applied to the
nearest kernel node), or `ExactEqual`'s container arms should ask
`SameContainer` after the tag check; either way `m eq m` is true and the
refined-container `eq`/`deq` rows above become corpus rows.

## NUR141 — the check pass admits a value to a predicate-typed parameter that the runtime rejects {#nur141}

**Status:** FIXED 2026-09-25 (the predicate runs for real — the handoff
log's entry of that date; recorded 2026-09-15, the sixty-second increment).
**Found:** by the COLLECT oracle's first corpus walk (`TestRegionCollectOracle`),
as the one `over-claimed` descriptor in the corpus.

**Rule:** one matcher. Whether a value matches a signature position is
decided by `MatchSignature` and the type's own unifier, and every lane —
the check pass, the interpreter, the VM — gets the same answer for the
same value and the same declared type.

**Divergence.** `fnpred.tsv:L50`:

```
def Even fnpred n:Integer [eq 0 (mod 2 n)]
def f fn [[n:Even] [Integer] [n]]
f 5
```

is a `signature_error` on both lanes — dispatch refuses the non-member.
But the region descriptor Phase B recorded for `f` at `1:80` claims ONE
forward slot (`5` for `n:Even`): the check pass's dispatch plan matched
it. The oracle's live candidate scan, running the kernel's own routine
over the same signature at run time, claims nothing — the predicate is
run there and `5` fails it. The check pass's matcher admits what the
runtime's rejects.

**Why the answer still agrees.** The row errors either way: the check
pass's admission is discarded when the call is executed and the runtime
refuses. What differs is the MODEL — which signature a value matches — and
a checker verdict built on that model (an arm reported reachable, a
result narrowed through the admitted signature) would be wrong where no
differential can see it. This is T4 territory (FULL-COMPILATION.0.md,
checker/interpreter agreement), and it is exactly the question the
maintainer has not yet ruled on.

**Fence.** The row is ledgered by name in
`test/go/langspec/region_oracle_test.go` (`regionOracleFindings`), pinned
in both directions: a second row of this class fails the lane, and a
ledger entry that stops reproducing fails it too, so the fix that removes
the divergence must retire the entry.

**Verdict:** none yet. The candidate fix is that the check pass's
`positionalMatch` runs the predicate unifier over a CONCRETE argument
exactly as the runtime does (a carrier argument stays admitted, which is
the check pass's proper optimism); whether that is a check-side fix or a
narrower recording of the plan is the ruling to seek.

**Traced (2026-09-25).** The admission is one arm: `Registry.RunPredicate`
(core/go/registry.go) returns `candidate, true` under analysis mode —
"accept the binding without running the body" — so every predicate
unifier admits every candidate the check pass offers, concrete or not.
The fix the candidate names would run the predicate body FOR REAL during
analysis over a concrete candidate (the sandbox already brackets the
run; analysis mode would have to be suspended inside it, since the body
under analysis yields a carrier, not a verdict) — executing user code
inside the check pass, which is the T4 ruling this record waits on. The
two lanes' ANSWERS agree meanwhile: `f 5` raises the same
`signature_error` at 1:78 on both (the runtime rematch), `f 4` is 4. Left
Pending; unchanged.

**The fix (2026-09-25).** The candidate fix, on the precedent the check
pass already set for itself: the const fold runs an expression for real
under analysis (`concreteEvalOnce`, the pass's mode suspended around the
run), and a predicate over a CONCRETE candidate now runs the same way
(`RunPredicate`: analysis suspended, the def table snapshotted and
restored, an effect-free body only — `exprHasEffect` — and a run that
errors admits as before), so the pass's plan refuses `f 5` exactly as the
runtime does; a carrier candidate stays admitted, the pass's proper
optimism. `f 5` is one `signature_error` on both lanes and at check time,
`f 4` is 4; the region oracle's `over-claimed` entry is retired and
`TestPredicateAdmissionAgreesWithTheRuntime` pins it.

## NUR139 — the unreachable-unit recovery covered one of the two per-unit refusal sites {#nur139}

**Status:** Resolved (2026-09-11, the fifty-seventh increment), in the commit
that records it. Kept for the reason NUR136 is: the divergence is an
INVARIANT WITH TWO SITES, which is the shape that keeps recurring in this
lowering. **Found:** by `TestVariationDifferential`, on a `for`-body variant
of a corpus row the same increment added.

**Rule:** a unit NOTHING IN THE PROGRAM CALLS may not refuse the program. Its
lowering is an optimisation that did not pay off, not a fact about the code.

**Divergence.** Finalize's per-unit loop has two refusal sites and only the
first carried the recovery:

```go
if reason := flw.lowerEvents(…); reason != "" {
    if rec.stampOnly { …trap stub…; continue }   // recovery
    return nil, "fn " + rec.name + ": " + reason, false
}
…
if reason := flw.reconcileResults(…); reason != "" {
    return nil, reason, false                     // no recovery
}
```

The comment on the first site states the rule in full, and says what it cost
when it was missing ("two corpus rows went from compiling to 'refused: fn
storedfn$body: consumes loop results' the day stampFnConst was written
without it"). The second site was written to the same loop and did not get
it.

**Why it stayed invisible.** The only unreachable units were fn-value stamps,
and a stamp's refusals happen to land in `lowerEvents`. Nothing else produced
a compiled-then-unreferenced unit — until a whole-residual dispatch admitted
a region residual on the PROBE and declined it on the real unit, which
compiles the unit first and then records no dispatch to it.

```
for 2 [def b true  do [1 2 (if b [] [9 9])]]
  before  1 2 1 2 on both lanes
  after   refused: fn do$body: body leaves extra values (Stage 3 …)
```

**Fix.** `unreachableUnitStub` is the recovery, called from both sites. Fence:
`lang/go/closure_region_decline_test.go` pins the row that reaches the second
one; the stamp path already pinned the first.

**What actually caught it.** Not a test of the rule — the variation lane,
which takes each corpus row and transforms it. The SEED was a row this
increment added and the VARIANT was not in any corpus, which is precisely the
case the lane exists for. Worth remembering when adding corpus rows: the new
row's own gates passing says nothing about its variants.

## NUR138 — a closure operand was read as a stack read, so every closure-compiled body word lost its region plan {#nur138}

**Status:** Resolved (2026-09-11, the fifty-seventh increment), in the commit
that records it. Kept rather than deleted for the reason NUR137 is kept: what
recurred is the SHAPE of the mistake, and this is its third instance in three
days — the first one that cost REFUSALS rather than a wrong answer, which is
why nothing caught it. **Found:** widening the whole-residual dispatch's
multi-out exactness screen, when the widened screen made a pinned row start
refusing.

**Rule:** one question, one answer — an operand either takes its value from
the ENCLOSING stack (the stack a mark plan has already indexed) or it does
not.

**Divergence.** `regionReadsTheStack` ended its per-kind walk with

```go
if op.kind == opEvent || op.kind == opClosure { return true }
```

An `opEvent` operand does read the enclosing stack: its producer ran earlier,
so the value sits below the mark and the call pops it from there. That is
NUR133's two witnesses. An `opClosure` operand does not. `OpPushClosure`
pushes the closure's captures and then the closure, all ABOVE the mark, and
the call pops exactly what it pushed — net +1 above the mark, nothing read
from below it. The captures cannot be stack reads either: `planValueDefLocals`
promotes every captured producer to a frame local, and `eachClosureCap`'s own
doc states the rule ("a closure capture can only reference a frame local or an
enclosing operand at run time, never a transient simulated-stack slot").

**Why it was invisible.** A body word's OWN body operand is an `opClosure`, so
the answer was "reads the stack" for every closure-compiled body word there
is — and the effect was that such a word's region plan declined, which is a
refusal. A refusal is not silent-wrong, so no differential caught it; and the
shape only became reachable at all once body words started compiling their
bodies to units, so there was no before-and-after to notice.

The asymmetry it produced, measured on the same program:

```
7 def b true  do [1 2 (if b [] [9 9])]
  body on the dyn-body strategy   prefix seated, program compiles
  body compiled to a unit         region plan declines
```

Compiling MORE made the program compile LESS — which is the signature to look
for when a screen is inherited rather than derived.

**Fix.** `operandReadsTheStack` asks the question per operand and recurses
into a closure's captures rather than assuming the promotion: a capture that
is somehow not promoted still declines the plan instead of mis-indexing the
mark. Fence: `compiler/go/region_operand_screen_test.go`'s table gained four
rows — a closure over no captures, over an inert capture, over an EVENT
capture, and a closure capturing a closure over an event.

**The pattern this is the third instance of.** NUR133: a screen read two of
the four kinds of event that produce a region. NUR137: it read a payload an
`evBranch` does not carry. NUR138: it read a kind of OPERAND as a stack read
because every operand that was not inert happened to be one when it was
written. Each time the screen was correct for the inputs that existed when it
was written, and each time the next increment changed the inputs. The default
arm is now safe (NUR137's fix), which bounds the cost of the next instance to
a refusal — and this record is what that refusal looks like from the inside.

## NUR137 — a branch region's condition was never screened, because the screen read a payload the kind does not carry {#nur137}

**Status:** Resolved (2026-09-11), in the commit that records it. The record
is kept rather than deleted because what recurred is the SHAPE of the
mistake, not the line: NUR133 fixed the same predicate ONE DAY earlier and
left a comment predicting exactly this, and the register is where that
pattern is visible. **Found:** 2026-09-11, adversarially
probing the fifty-fifth increment's own new seat.

**Rule:** one region-prefix plan, one screen. A mark may not open above a
value the region's own event then pops.

**Divergence.** `regionReadsTheStack` switched on the event kind and ended
with `default: ops = ev.call.ops`. An `evBranch`'s operands are in `ev.br`,
so for a branch the default read the ZERO-VALUE `emitCall` — nil ops — and
answered "does not read the stack" for every branch region there is.

```
def zs [0] def zt (zs 0 getr)  1 (if (zt gt 0) [] [9 9])
  interpreted  1 9 9
  compiled     9 1 9        silent, exit 0, the DEFAULT lane
```

The condition `zt` is event-produced, so it is on the stack when the mark
opens; the branch then pops it from beneath its own mark, and the prefix and
the run interleave.

**Dated by bisect.** The merge base `6bc55db` REFUSED this ("residual shape
beyond Stage 1"). `25b1af3` (increment 46) still refused. `d663fe1`
(increment 47) answers `9 1 9`. That increment's own doc says it widened the
producer gate — *"the plan's single-slot gate was inherited from the two
producers that happened to exist, not from anything the mechanism needs"* —
which was true, and the screen was inherited exactly the same way.

**The second face.** The fifty-fifth increment gave a body unit the same
plan, so the same unscreened shape became reachable inside a fn unit, where
it is not a wrong answer but an INTERNAL error:
`def zs [1] def zt (zs 0 getr)  do [1 (if (zt gt 0) [] [9 9])]` raised
`bytecode: internal: SEAT_BELOW_MARK prefix reaches past the mark` on the
default lane, where increment 54 had refused cleanly.

**Why this is a register entry and not just a bug.** It is NUR133's mistake
a second time, and NUR133's own fix left a comment predicting it: *"A kind
whose operands are not listed here does not 'have none': it is
unscreened."* The comment was right and did not prevent the recurrence,
because the default still silently produced an answer. So the fix changes
the DEFAULT rather than adding a fifth case: an event kind the screen does
not name is now assumed to read the stack. Adding a region producer costs a
refusal until someone lists its operands — a defect that announces itself,
rather than a silent wrong answer.

Pinned in `lang/go/region_stack_read_test.go` (both arms of the branch, the
body-unit face, and the inert-condition twins that must keep compiling) and
`compiler/go/region_operand_screen_test.go` (the per-kind table and the
unnamed-kind default).

---

## NUR136 — the frame was sized before the seating that allocates into it {#nur136}

**Status:** Resolved (2026-09-11, the fifty-fourth increment). **Found:** the
first time a body unit's residual rebuild fired.

**Rule:** one invariant, one place. A unit's local count must cover every
local its own code stores to, and the two units — the program and a fn body —
should establish that the same way.

**Divergence.** They did not. The program's write-back sits after the residual
reconciliation and says so in a comment that names the bug it was moved for:

```go
// AFTER the residual reconciliation, not before it: the residual's own
// seating allocates spill temps too (seatResidualRebuild), and a count
// written back before it left those locals outside the frame …
es.units[0].numLocals = lw.numLocals
```

The fn unit's grew `cf.NLocals` from `flw.numLocals` **before**
`reconcileResults`. Nothing allocated during a fn unit's seating, so the
ordering was inert — until the body-unit rebuild landed, and then:

```
[10 20] each [drop (1 add 2) (3 add 4) 1 pick]
  internal bytecode VM error: runtime error: index out of range [2] with length 2
```

**Why it is a non-uniformity and not just a bug.** The correct ordering had
already been derived once, for the other unit, and written down beside the
code. The fn unit's copy of the same step did not get it, and nothing tied
the two together — so the second unit was free to be wrong for as long as no
caller exercised it. That is the shape this register exists to make visible.

**Fixed** by moving the fn unit's write-back to sit immediately before its
RET, with the program's comment mirrored. The fence is deliberately not a
witness pin: `TestBodyResidualRebuildSizesTheFrame` walks every unit's
`STORE_LOCAL`/`PUSH_LOCAL` argument against that unit's own `NLocals`, so a
third seating that allocates late fails on its own program rather than on
this one.

---

## NUR135 — a minted type node is retired by the FIRST pop, however many live bindings hold it {#nur135}

**Status:** FIXED 2026-09-25 (the last pop — the handoff log's entry of
that date). Measured 2026-09-11, worked around in the arm-resident type
twin.

**The fix.** The first face: `DefTable.HoldsType` (core/go/deftable.go)
reports whether any live entry, under any name, still binds a node, and
both retiring pops — `UninstallType` and `PopLiveBinding` — retire a
minted node only when none does, so a node pushed twice under one name
survives its first pop and `LookupByID` keeps resolving it; the last pop
retires it. Pinned by core `TestMintedNodeRetiredByTheLastPop`. The second
face — a popped name's PARTS stay reserved — is the reservation rule
NUR167's close documented as the language's on both lanes (a body's `def
T` reserves the part for the registry's lifetime, so a second call
conflicts on it; `def Big (refine Integer)  undef Big  def Big (refine
Integer)` conflicts identically interpreted and compiled), and the
check-pass leftovers a rollback left behind are forgotten by
`ForgetTypePartsSince` (NUR167); it is not changed here.
**Found:** the fifty-third increment, by the cross-request
parity oracle — no same-request lane can see it.

**Rule:** one binding store, one retirement rule. Popping a def entry should
affect that entry and nothing else.

**Divergence.** `TypeTable.Retire` is `delete(tt.byID, def.ID)`, and the
`undef` path calls it whenever the popped entry is `Minted`. Nothing counts
how many LIVE def entries hold that `*Type`. Push the same minted node twice
under one name and the first pop unregisters it out from under the second:

```
r.Defs.PushType("Big", node, body)   // twice, same node
undef Big                            // pops one level, retires the node
Big                                  // the surviving level's node is GONE
                                     //   bytecode: internal: unresolvable type operand Big
```

The interpreter never reaches this because every `def Big …` MINTS: N
executions leave N distinct nodes, and each pop retires its own. So the
divergence is invisible until something replays ONE captured node N times —
which is exactly what a bind twin does, and what the arm-resident type twin
was first written to do.

**Where it bites, and the workaround.** `core.ApplyResidentTypeBind` now
re-installs the captured BODY through `InstallTypeBody`, so each element
mints its own node and the retirement rule is never asked the question. That
is the right shape for the twin on its own merits (it is what the
interpreter does), so the register records the underlying asymmetry rather
than claiming the twin is still broken. Any future op that replays one
captured type entry more than once will meet it again.

**Second, smaller face of the same thing.** `Retire` removes the node from
`byID` and never unregisters the name PARTS `RegisterPart` added, while
`validateTypeName` skips the part check only when the name is currently a
type binding. So after a rollback that restores bindings but not parts, a
replay through the installer's front door is rejected on the check pass's own
leftovers ("name part \"Big\" in \"Big\" conflicts with an existing type
name" — measured on element 0). `InstallTypeBody` exists to enter past that
check for exactly this replay.

**What the fix would be.** Either reference-count a minted node across live
def entries (retire on the last pop), or make retirement and part
registration one reversible operation so the two halves cannot drift. Both
are registry-model changes with their own parity rows, which is why this is
recorded rather than folded into a compile increment.

---

## NUR134 — a MODULE-exported fn's failed dispatch escapes the caught-body bracket {#nur134}

**Status:** FIXED 2026-09-25 (the unit trap and the caught Error — the
handoff log's entry of that date). **Found:** probing the
two `frontier-do-catch.tsv` ledger rows after the forty-ninth increment.

**The fix.** Traced once more on 2026-09-25, with call stacks: the check
pass analyses the do body three times — twice under `DoListReturnsFn`'s
caught bracket (no finding: below the uncaught top level), once as the
do-body closure UNIT (no finding either) — and the finding came from a
FOURTH dispatch: the body's residual carried the failed call's wreckage
(the value marked `FailedDispatch`, with its window) out of the bracket as
the do's results, and the enclosing tape re-stepped it at the uncaught top
level, where it failed again over `'no-raise' error [dot code]` and was
reported as a program error. Two pieces close it. The do body's model is
the caught Error when the body raises UNCONDITIONALLY at its own level
(`CheckState.RaiseWatches`: `DoListReturnsFn` opens a watch, a definite
no-match or a `raise` at exactly that nesting marks it, and the result is
one Error carrier — the runtime's); and the compiled do-body unit raises the
no-match IN PLACE — a unit-scoped trap (`RecordUnitTrapErr` /
`recordUnitTrap`: the unit's root frame only, first trap wins, the dead
tail dropped at finish, the unit diverging with OpTrap last and no RET), the
top-level terminal trap's twin. Both spellings now compile and answer the
caught code: `do [(true 5 M.dec) "no-raise"] error [dot code]` is
`uncalled_function` on both lanes, the local `zd` twin `signature_error`
(the interpreter's two codes for one failure remain its own
non-uniformity, recorded above), and the frontier rows' `def msg (do …)
msg` spelling compiles too. Found with it and fixed the same way, a
PRE-EXISTING silent miscompile: a def AFTER an unconditional raise in a do
body leaked into the keep-defs model — `def x 0 do [raise bad_input "boom"
def x 1] error [dot code] x` compiled to `[bad_input 1]` for the
interpreter's `[bad_input 0]`, and without the earlier def to 1 where the
interpreter raises undefined_word. The watch keeps the def-stack depths at
the raise and the model rolls the later defs back (the undefined read now
declines loudly at the check pass). Pinned: `TestModuleNoMatchInDoBodyIsCaught`
(`lang/go/register_tail_test.go`).

**Rule:** inside an error-TRAPPING region (`do [...]`, `CaughtBodyDepth`) the
runtime catches every body error, so an error-severity finding there is not a
program error. `CheckState.AddDiagnostic` re-attributes one centrally —
downgrade to info, stamp `CaughtAtRuntime` — and the comment on that site says
why it is central: "This covers EVERY error family uniformly … instead of each
emitter special-casing the region."

**Divergence.** It does not cover a MODULE-exported fn value. The same
program, one word different:

```
def zd fn [[bad:Boolean x:Any][Any][…]] end do [(true 5 zd) "x"] error [dot code]
    no_signature    severity INFO   CaughtAtRuntime true    COMPILES
import module [ … export "M" {dec: dec/v} ] end
  do [(true 5 M.dec) "no-raise"] error [dot code]
    uncalled_function  severity ERROR  CaughtAtRuntime false   refuses
```

Both interpret to the caught code as a value. The local spelling is
downgraded and compiles; the module spelling is reported as an uncaught
program error and the whole program refuses with "check diagnostics".

`execFnDefLiteral`'s `uncalled_function` arm is guarded by
`analysisAtUncaughtTopLevel()` (FnBodyDepth and NestedBodyDepth both 0), so
INSIDE the `do` body it emits nothing at all — which means the finding we see
is attached by a SECOND analysis of the same call, running with those depths
reset and outside the `CaughtBodyDepth` bracket. The dry pass in
`AnalyseCodeEffectCarrier` is the suspect (the same pass task #27 records a
memo-isolation defect in), and that identification is the part still owed.

**Fixing it does NOT graduate the two ledger rows, and that is worth knowing
before someone tries.** The compile pipeline refuses on a CAUGHT
model-undermining finding too — the guard is deliberate (`lang/go/boru.go`:
the `do` body's contents were recorded from the same guess, so the runtime
catching the error does not make the compiled region's value right), the
refusal it produces is not. A correct caught attribution changes the rows'
reason, not their verdict. What they need is the dispatch to resolve, which
is family A's problem — the fix those refusals are owed.

**Traced (2026-09-25).** The second analysis is not the suspect after
all: execFnDefLiteral's `uncalled_function` arm (core/go/engine.go)
records a TERMINAL trap for a definite no-match only at the program's
top level (`RecordTrap`'s frames-and-units-at-depth-1 guard — a `do` body
is a nested unit), and otherwise notes the diagnostic, which
`AddDiagnostic` downgrades and stamps `CaughtAtRuntime`; the compile gate
refuses on that stamp by design (lang/go/boru.go). The local spelling
compiles because a WORD's no-match takes the dispatch recovery's poly
seat, which has no top-level guard. Closing this needs a unit-scoped
trap — a raise the `do` body's unit carries and its own catch handles —
and the interpreter's two codes for one failure (`signature_error` at the
word, `uncalled_function` at the value) are their own non-uniformity.
Left Pending; unchanged.

---

## NUR133 — a region's consumer read only two of the four kinds of event that produce one {#nur133}

**Status:** Resolved (2026-09-10). **Found:** 2026-09-10, by a Codex review
of PR #448 — three P1 findings, all three real, all three reproduced before
being fixed.

**Rule:** a runtime-variadic REGION is consumable only when the mark that
bounds it opens BELOW everything its own event will pop, and only when its
run cannot carry a CALLABLE (the two lanes disagree about one — the
interpreter re-steps a Function that arrives on its stack, the VM appends it
as data; NUR129).

**Divergence.** `regionReadsTheStack` — the predicate that answers the first
half — walked `ev.call.ops` and a loop's operands and nothing else. That was
COMPLETE for the two producers that existed when it was written (a
value-producing loop and await's winner, both `evCall`) and silently
incomplete the moment two more were admitted:

```
def f fn [[n:Integer] [] [for n [i]]] 9 f (1 add 2)
                     compiled [0 9 1 2]  interpreted [9 0 1 2]   (evCallUser)
def xs [1] [do [1 div (xs 0 getr)] error [drop]]
                     compiled [1 []]     interpreted [[1]]       (evFallback)
```

In both the `STACK_MARK` was emitted AFTER the value the region's own op then
popped, so the op consumed from beneath its own mark.

Separately, `RecordFallback` marked the island a region without
`eventFlags.regionMayBeFn`. An island's run is the INTERPRETER executing
arbitrary code, so what it appends is not bounded by the modelled out at all:

```
def xs [1] def g fn x:Integer Integer [x add 1] 5
  do [if ((xs 0 getr) eq 1) [g/v] [1 div 0]] error [drop]
                     compiled uncalled_function  interpreted [6]
```

**One of the three is OLDER than the PR, and that matters for the record.**
Measured on the merge base `6bc55db`: the two `error`-region shapes REFUSED
there ("the single-output island model would leave the stack one short"), so
the forty-eighth increment turned two refusals into wrong answers. The
`evCallUser` witness diverged there identically (`0 1 9 2` — a different
wrong answer from the same program's mark-plan variant), which makes the
review's diagnosis of it wrong about the cause and right about the
divergence: underneath the mark plan, `lowerUserCall` force-promoted a
VARIADIC-returning callee's result to ONE frame slot. One `STORE_LOCAL` pops
one value; the run had three, and the other two were stranded beneath the
prefix. `lowerCall` has carried the equivalent guard since PR #280
("variadic result promoted to frame slots") and needs `nout >= 2` there; the
user-call twin is needed at `nout` 1, because a variadic slot IS one slot.

**Fix.** `regionReadsTheStack` switches on the event kind and names every
producer's operands (a kind that is not listed is UNSCREENED, not
operand-less — the comment says so, because that is the mistake to prevent);
`RecordFallback` marks its region possibly-callable unconditionally; and
`lowerUserCall` refuses to promote a variadic callee's result. All three
shapes now refuse instead of answering wrongly; the interpreter absorbs each
refusal, so the answers agree — three wrong answers traded for three
unimplemented cases, each owed a fix. The const-argument twin `9 f 3`
still compiles natively and is still seated by `OpSeatBelowMark`, which is
the pin that keeps the fix from being a blanket retreat.

Pinned by `lang/go/region_stack_read_test.go`.

---

## NUR132 — a same-unit break/continue jumped, where the interpreter splices {#nur132}

**Status:** Resolved (2026-09-10, the fiftieth increment). **Found:**
2026-09-10, shrinking the last `while` frontier row (the diverging arm of a
computed-condition no-else `if`) to its minimal shape.

**Rule:** `break` and `continue` end the CURRENT ROUND. The interpreter
splices the round's tape back to the mark it opened at the round's start, so
whatever that round already produced goes with it; a `break` additionally
closes the loop. The VM has one mechanism that does both —
`flowSignal`, reached by `OpFlowBreak` / `OpFlowContinue` — and it was used
only for the CROSS-FRAME case (a break raised in a callee).

**Divergence.** A break/continue whose loop lived in the SAME unit lowered to
a bare `OpJmp`: to the loop's end for a break (a hole `lowerLoop` patched),
back to `FOR_NEXT` for a continue. Both destinations were right. Neither
jump trimmed the round, and the break's landed PAST the `FOR_NEXT` that is
the only op popping the loop — so:

```
for 3 [ (7 add 2) if (i eq 2) [continue] [5] end ]
                        compiled [9 5 9 5 9]   interpreted [9 5 9 5]
for 3 [ (7 add 2) if (i eq 2) [break] [5] end i ]
                        compiled [9 5 0 9 5 1 9]  interpreted [9 5 0 9 5 1]
while [true] [ (7 add 2) if true [break] [5] end ]
                        compiled [9]           interpreted []
for 2 [ (i add 0) end for 3 [ if (i eq 1) [break] [0] end ] ]
                        compiled tape_exhausted   interpreted [0 0 1 0]
```

The first three are silent wrong answers on the DEFAULT lane with exit 0.
The fourth is worse: the inner loop's leaked counter is what the OUTER
loop's `FOR_NEXT` then stepped, so the program never terminated and died at
the evaluation stack's growth ceiling.

**Why the corpus never caught it.** The round's value has to be COMPUTED.
`lang/spec/control.tsv` §7 already pins the rule over a CONST round value
(`while [true] [1 break] end 'x'`), and those rows are right for a reason
that hides this one: a const the round never seats is dropped at lowering,
so there is nothing left to trim. The computed twins mostly refuse before
reaching the terminator ("branch leaves extra values"), and the narrow gap
between those two facts — a computed prefix, a branch arm with an explicit
else — is where every witness lives.

**Fix.** `lowerBreak` / `lowerContinue` emit the FLOW signal ops
unconditionally. The signal resolves the nearest OPEN loop at run time,
which for a same-unit terminator is this loop, and `vmLoop` already carries
the destinations the jumps used to name: `exitPC` is the `FOR_NEXT`'s own
exit target and `nextPC` is the `FOR_NEXT`. So the destinations did not
move; only the discipline the jump skipped was added, and `loopCtx`'s
`endHoles` — a second source of truth for a pc the FOR_NEXT already holds —
is deleted. The `while` lowering's condition-false exit has emitted
`OpFlowBreak` for exactly this reason since the thirty-seventh increment;
the body's own terminators now agree with it.

Pinned by `lang/go/loop_flow_trim_test.go` (all four witnesses on both
lanes, plus the rows that always agreed), `compiler/go/loop_flow_signal_test.go`
(the emitted op, arm by arm, and that a refused break emits nothing) and
four `lang/spec/control.tsv` §8 rows, which put the shape under the corpus
differential and the variation sweep for good.

---

## NUR131 — a full-stack shuffle over a produced closure compiled it as data {#nur131}

**Status:** Resolved (2026-09-10, the forty-fifth increment). **Found:**
2026-09-10, verifying a Codex P1 review finding against the forty-third
increment's residual rebuild.

**Rule:** a value that ARRIVES on the stack is re-stepped — a Function
dispatches over what is beneath it. NUR124's rule, and the one NUR129 leaves
open for a loop region.

**Divergence.** Four witnesses, all silent wrong answers on the DEFAULT lane
with exit 0:

```
def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]] end
  5 (mk 3) 0 pick      compiled [5 fn (Integer) fn (Integer)]   interpreted [45]
  5 (mk 3) 1 roll      compiled [fn (Integer) 5]                interpreted [15]
  9 (mk 3) 9 2 roll    compiled [fn (Integer) 9 9]              interpreted [27 9]
  7 (mk 3) 1 pick      compiled [7 fn (Integer) 7]              interpreted [7 21]
```

The interpreter's `pick`/`roll` splices its permutation back onto the tape,
where the pointer steps each value and any fn that matches what is beneath it
FIRES. `FoldFullStack` models the same permutation statically and its output
is data, so the closure just sits there.

**It pre-dated the increment it was found beside.** The review read these as
shapes the residual rebuild had newly admitted ("these shapes previously
refused at residual seating"). They did not: measured on the merge base
`d65f25a`, all four compile to the same wrong answers, through the fold's own
promotion path (`MarkValueDef` + `STORE_LOCAL`/`PUSH_LOCAL`) rather than
through any residual seating. The rebuild's disassembly is absent from all
four. What the review got right is the CLASS, and that is what made the
finding worth acting on.

**The fix, and why the two screens differ.** `FoldFullStack` declines
`pick`/`roll` when a preserved entry is BOTH event-produced AND provably a
Function; the residual rebuild uses the wider possibly-callable screen
(`regionValsMayBeCallable`, which counts a Dynamic entry too). The asymmetry
is deliberate and measured: the rebuild is new machinery, so a wide screen
costs only graduations that were never realised, while the fold has live
correct compiles a wide screen would take with it — `def g … (1 add 2) g/v
0 pick` is 5 on both lanes today, because a def-bound `/v` read is not
event-produced and the deopt machinery already covers it. Both spellings of
that shape, and a non-callable event pair, are pinned as still compiling.

**What stays open.** A DYNAMIC event result that turns out to be callable at
run time is still not screened by the fold — the same edge NUR129 names, and
for the same reason: the predicate that would catch it refuses a large class
of programs that compile correctly today. The rebuild does screen it, at no
cost, which is the only reason the two differ.

---

## NUR130 — a terminal trap's caret is the recorded site, the interpreter's is its tape pointer {#nur130}

**Status:** FIXED 2026-09-25 (the condition's own token — the handoff
log's entry of that date). Found 2026-09-10, the forty-second increment
(the statically-empty `while` condition compiled to a terminal `OpTrap`).

**The fix.** The first of the two directions the record named — the
interpreter moves to the better anchor. The `while` continuation carries
the condition operand's position (`ForCont.CondPos`, seated by the `while`
handler from its operand), and `stepMoveWhile` raises "condition produced
no value" there instead of at the spliced pointer: `while [] [1] end 5`
reads 1:7 on both lanes, and `while [] [1]` — positionless interpreted
before — reads 1:7 too. Pinned by lang
`TestWhileEmptyConditionAnchorsAtTheOperand`. (`if [] [1] [2]` declines
the compile — "if: condition body produces no value" — so its anchor is
the interpreter's alone.)

**Rule:** a position is part of the error a user meets (NUR108), so two lanes
raising the same error should anchor it at the same place.

**Divergence.** Message, code and exit agree; the caret does not.

```
while [] [1] end 5
  compiled     runtime_error: while: condition produced no value  --> 1:7   (the `[]`)
  interpreted  runtime_error: while: condition produced no value  --> 1:14  (the `5`)

while [] [1]
  compiled     --> 1:7
  interpreted  --> source position unknown
```

**Why, and which one is right.** A trap carries the position the RECORDER
saw — here the condition operand, which is the thing that is wrong. The
interpreter raises from `stepMoveWhile`, whose error takes the engine's
current pointer, and after the loop's mark/move triple has been spliced that
pointer is wherever the tape happens to sit: the token after the loop, or
nothing at all. The compiled anchor is the useful one; the interpreter's is
an artefact.

**Not narrow to `while`.** Every `RecordTrap` client has this shape — the
trap's `pos` is chosen at the record site and the interpreter's comes from
its own raise. `RecordTrapErr` does not: it serialises the interpreter's
whole diagnostic, spans included, so those traps agree by construction.
Nothing was measured across the other plain-`RecordTrap` clients here, so
this record claims only the `while` witness and the mechanism.

**What the fix would be.** Either give `stepMoveWhile` (and its siblings) the
operand position their errors are about, which moves the INTERPRETER to the
better anchor and closes the gap from the correct side, or teach the trap to
carry the interpreter's own position. The first is the one worth doing, and
it is an interpreter change with its own parity rows, which is why it is not
folded into a compile increment.

---

## NUR129 — a value-producing loop's region can carry a CALLABLE, and only the interpreter re-steps it {#nur129}

**Status:** FIXED 2026-09-25 (the reach survivor's iteration — the handoff
log's entry of that date; the bare witness narrowed the same day).
**Found:** 2026-09-10, verifying a review finding against
the region increments (NUR067). The finding was about `await`'s winner
residual; measuring it turned up the same divergence, older and unguarded, in
the loop the region representation was borrowed from.

**Rule:** a Function value that ARRIVES on the stack is re-stepped — it
dispatches over what is beneath it — and both lanes must agree on that. It is
the rule NUR124 and `OpDeoptIfFn` exist for ("a native's fn result dispatches
where it lands").

**Divergence.** A value-producing loop's region has no per-value seat, so
nothing in the compiled lane re-steps a Function the body leaves. Measured on
`0e0ad83`, before the region increments, so this is not theirs:

```
def g fn [[x:Integer] [Integer] [x add 1]]  for 2 [g/v]
  compiled     [fn g(Integer) fn g(Integer)]
  interpreted  ERROR uncalled_function: call to 'g' matched no signature
```

The compiled lane answers where the interpreter raises. `lowerLoop` marks the
region `lw.variadic` and the program residual absorbs it; the values are
appended as data and nothing asks whether one of them is callable.

**Not fixed here, and the reason is measurement, not appetite.** The
predicate that would refuse it — `regionValsMayBeCallable`, added for the
region CONSUMERS in the same change — reads a DYNAMIC residual as
possibly-callable, because the model does not bound it. Applied at
`RecordLoop` it would refuse every loop whose body residual is dynamic
(`for 3 [(f i)]` and its whole family), which is a large class of programs
that compile correctly today. Refusing them to close this would trade a rare
wrong answer for a common lost compile, and the register should not make that
call quietly.

**What the fix needs.** Either a PRECISE callable test the model can support
(a dynamic residual that provably excludes Function — the `sigTypeMatches`
not-disjoint rule the residual lowering already uses for the same question),
or the re-step itself: `OpDeoptIfFn` over a region, which is the general
answer and the one `design/FULL-COMPILATION.0.md` §6.6 points at.

**Guarded where it was reachable.** The two region CONSUMERS added alongside
this record (`OpSeatBelowMark`, `OpMakeListToMark`) and `await`'s region
recording all decline a region that may carry a callable, so
`5 for 1 [g/v]`, `[(for 2 [g/v])]`, `await {mode:'first'} [[5 g/v]]` and
`9 await {mode:'first'} [[g/v]]` refuse rather than diverge. Only the bare
loop residual above is open.

**Narrowed (2026-09-25).** The bare witness no longer diverges: the check
pass's own loop-round analysis re-steps the NAMED value the first round
left and notes `uncalled_function` at `g/v` (execFnDefLiteral's analysis
arm), so `for 2 [g/v]` declines the compile and answers by fallback with
the interpreter's raise — pinned by lang
`TestLoopNamedFnValueResidualDeclinesSoundly`. A factory's ANONYMOUS
closure per iteration (`for 2 [(mk 1)]`) is parked data on both lanes
(`[fn (Integer) fn (Integer)]`, NUR155's rule) and keeps its compile — a
static "provably callable" refusal at RecordLoop was tried and reverted
the same day for refusing exactly that shape. What stays open is the
DYNAMIC residual that turns out callable at run time, on the measured
trade-off above; the region re-step (§6.6) remains the general answer.

**The fix (2026-09-25).** Narrower than the refused trade-off: the dynamic
residual that turns out callable is, in the measured shapes, a REACH
group's survivor — a member fn read the pass cannot type (`m.f` over `{f:
g/v}`), which the interpreter re-steps at every iteration. The collapse
records a reach group's dynamic survivor (`CheckState.ReachSurvivorFnIDs`,
NUR210's mark widened), and `RecordLoop` asks the body residual about it
beside the named-fn hazard (`loopHazards`, through `residualStands`), so
`for 2 [m.f]` declines with the interpreter's `uncalled_function` where it
answered `[fn g fn g]`; a typed member read (`for 2 [m.count]`) and a
user-paren residual (`for 3 [(f i)]`) keep their compiles, an anonymous
member fn's loop loses one (it declines, both lanes' `[fn fn]` reached by
fallback). Pinned by `TestLoopReachSurvivorDeclinesSoundly`.

---

## NUR128 — a module fn's body gets no declaration-shaped analysis, so its dead branches are reported only when someone calls it {#nur128}

**Status:** FIXED 2026-09-25 (the export-time analysis — the handoff log's
entry of that date). **Found:** 2026-09-09, designing the `unreachable_branch`
attribution fix — every candidate that could distinguish "constant for this
call" from "constant for this code" lost this same class, from opposite
directions, which is what identified it as the shared cause rather than a
quirk of one design.

**Rule:** a fn body is analysed twice — once DECLARATION-shaped, with every
param bound to a carrier (`checkFnBodyAtConstruction` →
`ParamBodyCarrier`), and once per concrete CALL SHAPE. The generalized run is
the one whose verdict holds for every caller, so it is the one entitled to
report a property of the code.

**Divergence.** A module fn never gets the generalized run.
`checkFnBodyAtConstruction` bails on `!r.Check.IsActive()`, and a module
sub-registry's check is INACTIVE by design: the body must execute for real so
its exports get concrete names ("check mode is not propagated into a module
body — carrier-stripping would destroy the concrete export names",
`native_module_module.go`; `design/legacy/module-fn-checkstate-ownership.1.ignore` §3.2).
So a module fn's body is only ever analysed under whatever call shapes a
program happens to use. Measured on HEAD, the same fn either way:

```
def f fn [[n:Integer] [Integer] [if true [n] [0]]]              -> warns
import module [def mf fn [[…][…][if true [n] [0]]] export …]   -> SILENT
import module [ … same … ]  M.mf 4                             -> warns
```

The dead branch is identical in all three. Whether it is reported depends on
where the fn is defined and, for a module fn, on whether anyone calls it.

**What the attribution fix changed, and what it did not.** Suppressing
`unreachable_branch` inside a call-specialised analysis (`CallShapeDepth`,
2026-09-09) makes the module class consistently SILENT instead of
call-dependent. That is a true positive lost — a genuinely dead branch in a
CALLED module fn no longer warns — and it is recorded as the accepted cost of
that fix rather than hidden by it. It does not create this non-uniformity; the
generalized run was already missing, which is why the fix has nothing to fall
back on there.

**The recovery, and the trap in it.** Do NOT make the module sub-registry's
check active at construction: that is the reason the export names are
concrete, and it is a documented decision, not an oversight. The narrower
shape is to run the construction-shaped analysis for a module's exported fns
at EXPORT time, on the PARENT's CheckState — which also restores the warning
for the defined-but-never-called module fn that HEAD already misses. That is
its own increment with its own design question; it moves the parity ledger on
its own and has nothing to do with attribution.

**The fix (2026-09-25).** The narrower shape, as named: `export` queues each
exported fn value for the end-of-pass body check on the IMPORTING pass
(`NoteFnBodyPendingIn(parent, modReg, fd)`), and the drain shares that
pass's check state into the module registry for the analysis
(`CheckBraid.ShareCheckStateFrom`, the transient sharing a dispatch
already uses), so the declaration-shaped run happens in the registry the
body was written in with its diagnostics on the importer's pass. The
module's own check stays inactive — the export names stay concrete. Two
guards the first cut needed: the run is speculative where the module's
scope and the importer's state disagree (a real app's `set` and a
body-local read came back as false positives), so only its STRUCTURAL
findings are kept — `unreachable_branch`, the class this record is about
(NUR105's third discovery: a speculative analysis that cannot run reports
nothing) — and it runs past the pass's call-shape summaries of the same fn
(`CheckState.ForceFnReanalysis`), since it comes after them and is the
one entitled to speak. The never-called exported fn's dead branch warns at
its own position, and the called one's too
(`TestModuleExportedFnBodyAnalysedAtExport`: the uncalled, the called and
the top-level twin all report `unreachable_branch`).

## NUR127 — five enumerated string options had no declared domain, so a typo picked an arm silently {#nur127}

**Status:** RESOLVED 2026-09-09. **Found:** 2026-09-09, in review of the
coverage repair that followed the lazy-help change — the completeness loop
of the new `TestStringOptionValueOutsideDomainIsRefused` derives its rows
from `strOptEnums`, and a reviewer asked why four of the option keys it
skipped were absent from that table.

**Rule:** one option-validation mechanism. `strOptEnums`
(`lang/go/native/native_string_helpers.go`) exists precisely so that an
out-of-domain option VALUE fails as loudly as an unknown option KEY; its own
comment states the bug it closed — "`scope:"bogus"` simply was not `all`, so
it behaved as `first`".

**Divergence.** The table declared six keys and omitted five that are
consumed by exactly the same shape — a switch with a quiet default. Measured
on the default lane, exit 0, every one of these silently picking an arm the
caller did not ask for:

```
StringUtil.changecase 'hello' {style:'typo'}  ->  'hello'   (default: "lower")
StringUtil.escape     'a b'   {tgt:'typo'}    ->  'a\ b'    (default: "sh")
StringUtil.escape     'a b'   {quote:'typo'}  ->  'a\ b'    (switch has no default)
StringUtil.normalize  'abc'   {form:'typo'}   ->  'abc'     (applyNorm's default)
StringUtil.split ',' 'a,b'    {norm:'typo'}   ->  ['a' 'b'] (same applyNorm default)
```

while the SAME shape on a declared key refused correctly, which is what made
the omission a non-uniformity rather than a missing feature:

```
StringUtil.trim '  x  ' {side:'bogus'}
    error: [boru/string_option_error]: trim: option "side" got "bogus"
```

**Two details the fix had to carry, or it would have broken working
programs.** `form` and `norm` are upper-cased before use
(`strings.ToUpper`), so `form:'nfc'` has always worked and their domains are
checked case-INSENSITIVELY (`strOptEnumsFold`). And `norm` doubles as a
BOOLEAN switch — `norm:true` means NFC — so a boolean value is the other
spelling of the option, not a member of the string domain, and is skipped.
`quote` carries `"none"` because the corpus spells the no-quoting request
that way (`corpus-modules.tsv:121`), so it is a member of the domain rather
than the absence of one.

**Why it stayed invisible.** No test asserted the refusal for these five,
and the only thing exercising the validator's error arms at all was a side
effect: the eager dynamic-help hook passed a sample map `{a:1,b:2}` to every
string word at registration, which took the unknown-KEY arm and never the
out-of-domain-VALUE arm. Making help generation lazy removed that side
effect, the coverage gate noticed, and writing the real test is what
surfaced the gap.

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
and diverges, and the quoted verdict is wrong in its second word too: a
refusal there would be a defect owed a fix, not a sound outcome. Its fix
is the deopt inside a lambda unit (the handoff's twelfth-increment note).

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

## NUR119 — a fn value read through a param's `/v` renders under the param's name on one lane only {#nur119}

**Status:** FIXED 2026-09-25 (the param's name on both lanes — the handoff
log's entry of that date). **Found:** 2026-09-05, measuring the
returned-closure park (`design/PAREN-RESTEP-RULE.0.md` §2.1).

**The fix.** Verified closed on the record's own witnesses, by the fn-value
seam work landed earlier in this run (NUR118, NUR122 and the render gate):
`(app (z:Integer => [mul 3 z]))` and the `Any`-typed twin `app (…)` render
`fn g(Integer)` on BOTH lanes, and `app sq/v` DECLINES on the compiled lane
(the render gate's closure-render refusal) rather than answering under a
different name — no live divergence remains. Pinned by
`TestFnValueReadThroughParamRendersAlike` (lang/go).

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

**Status:** FIXED 2026-09-25 (the call's own token — the handoff log's
entry of that date) · **Recorded:** 2026-09-04 · **Surfaced by:** the
binding-sensitive unit memo (Stage 4b) letting a cross-family rebind row
compile

**The fix.** The proposed verdict, in two halves. The check pass recorded a
user call with the FIRST ARGUMENT's position, so the CALL_USER
instruction's debug entry was the argument's — and empty for a 0-argument
call; `callAnchor` (check/go/check_fnbody.go) seats the call WORD's
position, the argument's only where the word carries none. And the VM's
RET stamped a nested frame's contract error at the callee's own last
instruction; it now stamps at the CALL — the frame's return address, in
the caller's debug table — where the interpreter's ReturnCheck marker
anchors. `def f fn [[m:Map] [Integer] [m get "a"]]  f {a:"s"}` reads 1:43
on both lanes, `def h fn [[][Integer][1 2]] end h` 1:33, a return-type
failure over an argument 1:45 (the word, not the argument), and a runtime
no-match over a gradual argument stays at the word. Found again on
NUR147's close (the `do`-wrapped service shape it made compile). One
consequence had to be caught: a 0-argument call's event position used to
be empty, and a no-return lambda's leaked body residual (`def zzvlam ([]
=> [… def f m.f end f 5]) zzvlam` hands the caller its body's `f` and
`5`, attributed to the call event) answered the residual layout's
statement-boundary test with its own token positions, so the `end`
between them settled the lead as data (NUR187's rule); with the call word
on the event, both entries read as the call and the lead arm applied one
over the other at the caller (`CALL_DYNAMIC underflow`, the sweep's `def`
× container body variants). A call delivers its results resolved and the
interpreter never re-steps a returned fn over the values returned beside
it, so the lead arm now stands aside for a lead whose entries above it
are outputs of the same call (`siblingCallOutputs`); a caller-written
entry above a returned closure (`((w 5) 4)`, the paren rewind) still
applies. Pinned by lang `TestReturnContractErrorAnchorsAtTheCall`. The
second flavour (the
fn-VALUE seams) closed the same day: a lambda read from a map member
(`m.f 5`) anchors at the call through the closure's `RetPos`, and a
Function param applied inside a user fn (`(g 5)`) anchors at the READ —
the dyn-apply event and the trailing apply take the read's recorded
position (`wordReadPos`) when the lead carries none — 1:36 and 1:31 on
both lanes; pinned by lang `TestFnValueSeamAnchorsAtTheRead` (NUR122's
witnesses).

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

**Status:** FIXED — verified 2026-09-25 (the token's own text — the handoff log's entry of that date) · **Recorded:** 2026-08-30 · **Surfaced by:** closing
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

**Retired (2026-09-25).** Measured on the current tree, the witness
renders identically on both lanes — `def x:(Integer gt 10) 5 x`
underlines `def` (`^^^`) at 1:1 compiled and interpreted: the recorder's
event positions carry the token text the parser stamps on them
(`SrcPos.Src`), the debug table keeps it, and `stampAt` copies it onto
the error as the interpreter's `stampErrPos` does — the upstream carry
the record costed. Pinned by lang `TestTypedDefUnifyErrorCaretIsTheToken`
(the rendered diagnostics compared whole).

---

## NUR112 — a parked native applied after its name was extended is checked as two values {#nur112}

**Status:** FIXED 2026-09-25 (the plain check's stored member — the handoff
log's entry of that date) · **Recorded:** 2026-08-29 ·
**Surfaced by:** writing a corpus row to pin the VM's parked-native apply
gate; the row never landed, the observation did.

**The fix.** The retirement condition below, met the direct way: a
check-mode dispatch of a concrete member value. `getNodeReturns`
(`lang/go/native/native_storage.go`) reads a stored fn value (an FnDefInfo)
as ITSELF on a plain check — the recorder not armed — so the pass re-steps
it and dispatches it over what follows, exactly as the run does; the
compile pass keeps the dynamic carrier and its shaped method model, so
nothing it lowers moved. `def m {a:size/v}  m.a [1 2 3]` and the record's
own program check `[Integer]`; the accuracy ratchet, the type-soundness
gate and the diagnostic-surface parity measured unchanged over the corpus.
Pinned by `lang`'s `TestPlainCheckAppliesStoredFnMember`.

**Resolution 2026-09-25 (the reverse-order NUR run).** Measured: the
EXTENSION is irrelevant. `def m {a:size/v}  m.a [1 2 3]` alone checks
`[dynamic(Any) List]` for the runtime's `[3]`, and `def sz size/v  sz [1 2
3]` checks `[Integer]` — the parked native dispatches when it is a def-bound
value and is widened when it is a CONTAINER MEMBER. That widening is the
member read's DESIGNED model, stated at its site (`getNodeReturns`,
`lang/go/native/native_storage.go`): a field bearing dispatch keeps the
dynamic Any so the compile pass claims the arrival through the shaped
method model (`NoteMethodShape`, M2c) rather than folding a fn-value call
it cannot lower — which is exactly how the record's program COMPILES and
answers 3 on both lanes today. So the two engines agree, the compiled lane
is sound, and what remains is the plain check surface's residual claiming
two values where one runs: Stage 8 checker-totality debt, not a lane
divergence. Retirement condition: the plain pass modelling a member-fn
arrival (the M2c arrival model lifted from the compile pass to the plain
one, or a check-mode dispatch of a concrete member value), at which point
`TestCheckTypeSoundness` can carry the row. Until then this record is the
pin; no engine work is owed here.

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

## NUR109 — an unbound parser name is two different errors {#nur109}

**Status:** FIXED 2026-09-25 (the parser name bound on a branch — the
handoff log's entry of that date). **Recorded:** 2026-08-27 · **Surfaced
by:** completing the NUR106 oracle sweep

**The verdict, re-derived with NUR110 closed.** The interpreter's
`parse_unknown_lang` IS the consistent answer: with `op` bound only in the
branch that does not run, the name is unbound at the dispatch and the atom
resolves as a registered kind. The compiled lane still raised `parse_error`
because the check pass resolved the QUOTED atom through the registry — the
arm's def is installed during the arm's analysis and a quoted atom does not
read the join's guarded carrier the way a bare read does — to the arm's own
`Parse.parser` event, which the lowering promoted to a frame slot and pushed
verbatim: the zero slot, when the arm did not run. A fn-valued arm def is
not carried by the branch join at all (`carryBranchJoin` leaves fn values
to their own machinery), so no bound-check guarded it.

**The fix.** No op re-resolves a name at run time, so the unit DECLINES:
`lowerCall` refuses a `parselang-fn-dispatch` whose parser operand is
pushed from a slot a branch arm's promoted event fills (or a bound-checked
carried slot — `lowerer.boundSlots`), "parser name is bound only on a
branch — an unbound name resolves as a kind at run time", and the
interpreter answers: `parse_unknown_lang` when the branch did not run, the
parse result when it did — one verdict per program under Run. The condition
is a run-time value, so both twins decline. Pinned:
`TestParseFnDispatchMissParity` (`lang/go/bytecode_m3m4_test.go`), now the
decline and the interpreter's verdict on both twins.

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
[design/legacy/O1-RELITIGATION.0.ignore](design/legacy/O1-RELITIGATION.0.ignore).

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
(`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`). The single coercion rule this
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
investigate (`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):

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
(`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`). The operand-return semantics —
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
(the domain is infinite). DepScalar refinement construction does —
Bytes declares itself a refinement base since NUR009 closed
(`def Hi (Bytes gte (convert Bytes "m"))`) — and so does the
nominal-split route, `refine Bytes`.

### Why allowed

Vacuous rather than divergent: no kernel mechanism requires a scalar
family to have leaves, and nothing dispatches on their presence. There
is no useful structural split of Boolean — `True`/`False` subtypes would
duplicate what value-level machinery already provides uniformly (`case`
literal coverage per NUR002, and DepScalar refinements: `(Boolean gte
true)` *is* the true-only subset, since Boolean is one of the
refinement bases — Integer, Float, Number, String, Boolean, Atom and
Bytes each declare themselves one, `DeclareRefinementBase`). Users who want a
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
- `core/go/depscalar.go` — Boolean declared a refinement base
  (`DeclareRefinementBase`, read by `canonicalBaseType`; `Boolean gte
  true` constructs).
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
`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

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

**Status:** FIXED 2026-09-26 (Bytes a refinement base, a computed bound the run's — the handoff log's entry of that date) · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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
remediation, `design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):** the Bytes omission
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

**The fix** (2026-09-26 — the capability, not the one-line patch the
verdict refused). The refinement bases are DECLARED, not listed.
`core.DeclareRefinementBase(t)` stamps the type's `RefinementBase`
capability (its typeMeta), and `canonicalBaseType` walks a type's ancestry
to the first node that declared itself — a subtype refines as its
declaring ancestor, an undeclared type is no base. Each owner declares
where it registers the type: core its Integer, Float, Number, String,
Boolean and Atom leaves (`depscalar.go`'s init), basic its Bytes leaf
(`registerBytesType`). No resolver hand-lists a base — the mechanism the
verdict said the consolidation replaces — and when ADR-012's `types/go`
move lands, the declaration moves with the type. What the capability
needed to be whole:

- the inline-signature resolver (`ResolveSigType`) hand-listed five bases
  for an inline refinement, and a Bytes one fell to its TAny tail — a
  wildcard parameter (`[b:(Bytes gt …)]` took any value); a refinement now
  slots at its own base;
- Bytes' value Formatter rendered every Bytes refinement `Bytes<?>`, and
  the compile pass's const pool, keyed on that rendering, merged two of
  them — `c is (Bytes lt b) c is (Bytes gt a)` answered `[false false]`
  compiled. `Value.String` renders a refinement before any base Formatter,
  and a refinement over an identity-payload bound pools by identity;
- there is no Bytes literal, so every Bytes bound is computed: the compile
  pass now folds `convert Bytes <String>` over a const string (the overload
  declares CompileScalarFold, and the fold admits a type literal at a
  declared type slot), so a named Bytes refinement compiles.

Found on the way: NUR231 (a refinement over a computed bound, in the
compiled lane) and NUR232 (an inline refinement return, at check time).
Pinned by core's `nur009_refinement_base_test.go` and lang's
`nur009_bytes_refinement_test.go` — value use, named type, typed def,
parameter and return, positive and negative; the rendering; the pool
collision; and the bounds each base refuses.

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
  lead-in (added with this verdict); `design/legacy/LISP-ANALYSIS.5.ignore` (the original
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
`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):** the two-level model is to grow
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
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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
2026-07-22; verdict: maintainer, via `design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

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
`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):** bring `del` into symmetry with
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
2026-07-22; verdict: maintainer, via `design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

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
decision it then invalidates". `design/legacy/NUR-EFFORT-TRIAGE.0.ignore:139-148`
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

**Status:** FIXED 2026-09-26 (canon spells the sugar and the word — the
handoff log's entry of that date) · **Recorded:** 2026-08-15 · **Surfaced
by:** NUR059's fix — the residue its per-row fixpoint check refused

**The fix.** Every strand, in both ports (core/go canon.go, core/ts
canon.ts), and a gate so it stays fixed.

- **The bare-word question, decided by ADR-015 itself.** `word(foo)` is not
  source: it re-parses as the `word` splice applied to the group `(foo)`. The
  record's worry — that bare `foo` "re-parses as a word that will be
  DISPATCHED" — is evaluation, which canon does not model: bare `foo` parses
  back to exactly this Word (a paren body and a type tag already spelled
  their words bare). So a plain Word renders as its name plus any modifier
  suffix. The corpus moved with it: parse.tsv, shape.tsv, divergent.tsv,
  core/spec, eng/spec and the lang spec rows whose expected canon held
  `word(…)` — each row rewritten only where the new render differs from the
  old by exactly that spelling (verified per row), the rest untouched.
- **The lambda.** The marker renders `=>`; its FOLD group — the paren the
  parser wraps around every `A => B` so the arrow binds tightest — renders
  without parens, because `A => B` re-folds into the same group and `(A =>
  B)` re-reads as an explicit paren AROUND a fold. An explicit paren the user
  wrote holds the fold as its one token, so `(x => [1])` keeps its parens and
  round-trips (`canonParen`).
- **The mini literal.** The lexer takes any non-space delimiter, closed by
  the same character, so the delimiter is not part of the value and no parser
  change is needed: canon renders `+name'src'` in one canonical delimiter
  with the lexer's own escapes (`canonMiniSrc`). The withdrawn `+m<src>`
  failed because `<` is closed by `<`, not `>` — the pairing, not the
  delimiter.
- **The type bound** renders `name/t`, reading the name out of the `[name/q]`
  list its Items hold.
- **The group modifiers**, a fourth kind the record did not list (it believed
  them unwritable): `(1 2)/s`, `a.b /2` put the marker BEFORE the group in the
  stream, so a sequence rule (`canonSeqParts`, exported as `CanonValues` in Go
  and applied by TS's `canon`) spells it after the group — `(1 2) /s`.
- **`/N` precision.** TS carries the arity as a `bigint`, as exact as Go's
  int64, so `x/9223372036854775807` canons identically in both ports and left
  divergent.tsv for parse.tsv (divergent 10 → 9, parse 724 → 725).
- **A disjunct arm in Go canon.** It had none and spelled its members through
  the debug `String()`; the TS canon renders them through canon. The two
  ports split on `{a?:Integer}` the moment a plain word stopped being
  `word(…)`, and agree again.

**The gate.** ADR-015 §4 asked for a property gate in both ports; the
fixpoint diagnostic now runs over every parse.tsv row that parses, in the Go
runner (`TestParserCanonFixpoint`) and the TS runner, against one
shrink-only ledger (parser/spec/canon-fixpoint.tsv, pinned at 33 rows in
both). Every row of NUR072's kinds reaches its fixpoint; the 33 ledgered rows
were other kinds the gate found — template strings and XML `${}` holes
(NUR225), a map key that needs quoting (NUR226), a typed tag before an XML
literal (NUR227) — all fixed the same day, and the ledger is empty. Pinned also: core `TestNUR072SugarKindsSpellTheirSource`
and `TestNUR072UnspellableSugarKeepsTheFallback`.

Two results changed along the way and are recorded where they belong: the
lang bail ceiling fell 37 → 36 (NUR224's refusal, no longer booked as a
defect), and the FnModel golden took `behave`'s new ReturnsFn (NUR076).

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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

**Status:** FIXED 2026-09-26 (eq's capability — the handoff log's entry of
that date) · **Recorded:** 2026-08-16 · **Surfaced by:** the retired
NUR031's fix — the one part of its verdict the fix did not take

**The fix.** The first candidate: an `ExactEqualer` capability mirroring
`DeepEqualer` in every respect, so the two halves of the word family are
extensible on the same terms.

- `core.ExactEqualer` (`ExactEqualValues(a, b) (bool, error)`,
  `ErrNoExactEqualer`) is consulted by `exactEqualCapability` — the LCA
  walk `deepEqualCapability` takes — at ExactEqual's terminal `false`,
  after every kernel arm. It is additive exactly as deq's is: it can only
  turn that `false` into an answer, never override a scalar leaf, a
  container's, a function's or a handle's identity. Measured, the two
  terminals are reached by the same pairs — values of a type whose payload
  no kernel arm names (a host payload with no pointer to carry an identity)
  — so the capabilities have the same reach.
- `behave eq/q (fn [[T T] [Boolean] [body]])` installs it: deq's validator
  (now `validateEqualitySig`, shared), deq's seam on the wrapper
  (`ExactEqualValues`: delegate to the previous Behavior, decline, guard
  re-entry, read the Boolean verdict through the shared
  `runEqualityBody`). `describe behave` lists the slot.

The third reading the record weighed — reference identity as a kernel
property no type may answer for — is what the ADDITIVE placement keeps: no
arm the kernel has an identity rule for can be reached. What changed is
that the pairs the kernel has no rule for are the type's to answer for on
both halves, not on one. Pinned: core `TestExactEqualCapability*` (answers
at the terminal, declines and failures fall through, cannot reach past the
scalar or container arms), lang/native `TestBehaveEqSlotInstalls`,
`TestBehaveEqSeam`, and the eq rows of the wrong-shape and untyped-param
negatives.

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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

**Status:** FIXED 2026-09-26 (the check pass notes a behave make — the
handoff log's entry of that date) · **Recorded:** 2026-08-17 · **Surfaced
by:** NUR056's fix (the `Maker` capability); flagged for this register by
the PR #379 review (Codex P1)

**The fix.** Candidate (1) in its narrowest sound form, which is neither
running user code in analysis nor a second mechanism beside the word:
`behave` gained a CHECK-MODE HALF (`behaveReturns`, its ReturnsFn, the
seam every word uses to model its effect on the analysis). It validates the
call exactly as the handler does (`behaveTarget`, shared) and, for the
`make` slot, notes the target in the pass's own state
(`CheckState.NoteBehaveMaker`, reset per pass); `core.HasMaker` reads the
note beside the installed Behaviors. So `make` skips the schema validation
of a type whose own constructor builds it from the `behave make` call on —
as it already did for a Go-side Maker — and a construction BEFORE the call
still validates, which is the run's order too (it raises there).

It installs NOTHING on the type. Installing the real wrapper during
analysis would put user bodies within reach of analysis-time rendering,
comparison and construction — the objection the record raised against
candidate (1) — and the other seven slots need no model: they change what a
program computes, which analysis does not evaluate. The note is the one
fact analysis consults. A call the pass cannot see through (a fn carrier, a
computed name) or that the handler would refuse notes nothing, and the run
raises the refusal where it happens.

`boru describe behave` now says the slot is visible to check from the call
on. Pinned: lang `TestNUR076BehaveMakeIsVisibleToCheck` (the transcript's
program checks clean and answers Class/P{a:42} compiled; five negatives —
construction before the call, a compare-only behave, no behave, a carrier
fn, a refused target — still validate), core
`TestNUR076BehaveMakerIsVisibleToCheck`.

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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

**Status:** FIXED 2026-09-26 (the parity ledger is empty again — the
handoff log's entry of that date) · **Recorded:** 2026-08-09 · **Surfaced
by:** PR #337 parity-probe sweep; flagged for this register by the PR #337
review (Codex P1)

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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

**The fix (2026-09-26).** Every class is fixed in BOTH ports, each ledger
row moved to `parse.tsv` (§"NUR060: the parity-debt ledger, resolved")
with one or two neighbours, and `divergent.tsv` is empty (pinned at 0 in
both runners). Where the two renders disagreed, neither was taken as the
contract; each class got the rule its agreed neighbours imply:

- **Bodiless `=>`** (fold loss): an `arrowfold`/`arrowfoldelem` Open
  alternate matches the arrow followed by the end, a closer or a
  separator and raises `arrow_no_body` on the arrow
  (`` `=>` has no body ``). Before, the half-built fold fell to the val
  coalescer, which dropped the arrow (`[1 =>]` read `[1]`, in both ports)
  or doubled the input as the body (`def x 2 =>`, Go). A bare `x =>` that
  does not fold is unchanged.
- **Trailing bare `:`** (accept/reject): an element that opens with `:`
  and has no value is refused as an empty element (`empty_child`) in both
  grammars — Go's tabnas port records an empty list child as a nil
  `Child`, indistinguishable from none, so only its grammar can see it.
  The same `? :` shape was `[:]` and `[1, :]` too, both newly pinned.
- **`=> ,`** (accept/reject): the bodiless-arrow rule.
- **Post-`]` recovery detail**: a list whose open token is not `[` is
  implicit, and its `]` is refused where it stands — `1 2 ]` used to parse
  as `1 2` while `0 ]` was refused, and `1 2 ] 3` failed only at the
  end-of-parse check the two tabnas ports report on different tokens.
- **Error precedence** (`. (`, `/s (`): TS's paren BC threw for a trailing
  hole — including the empty element TS leaves where the source ends inside
  a group — before its converter ran; it now marks the group unclosed, as
  Go's derailed grammar does, and the converter reports the first fault in
  source order.
- **Type-name leak** (`quote . ( =`, `a.(`): the value converters gained
  the unclosed-paren arm the item loop had.
- **Bare `/s`**: `` `/s` modifies nothing `` is a syntax_error in both,
  no longer a plain `empty word`.
- **Empty `${}`**: the empty-expression alternate consumed the `}`, so the
  Close wanted another and the template broke after an empty hole
  (`` `x${}y` `` read `y` as unexpected); it now backtracks, and both
  converters skip an empty hole — `` `x${}y` `` is `'xy'`, as an XML
  attribute's empty hole already folded.

Pinned: the 24 new `parse.tsv` rows and the 12 re-rendered ones, in both
runners; `TestArrowWave3Degenerate`, `TestTemplateWave3InterpErrors`, the
seam tests for the now-direct-only fold guards; the TS guard test for the
bare modifier. Go and TS parser coverage stay at 100%, and the crossdiff
over eng/spec is identical. REFERENCE.md states the three user-visible
rules (bodiless arrow, empty hole, bare modifier).

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

**Status:** FIXED 2026-09-26 (boru:scry ships the seven, the debug copies
deprecated — the handoff log's entry of that date) · **Recorded:**
2026-08-12 · **Surfaced by:** design/BORU-SCRY.0.md §6 (the boru:scry
proposal); flagged for this register by the PR #344 review (Codex P1)

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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

**The fix (2026-09-26).** Scry ships, with the notice in place:

- `lang/go/modules/scry.go` builds `boru:scry` (namespace `Scry`) from
  `selfKnowledge`, one constructor of the seven natives; `debug.go` builds
  its seven from the same constructor, in their old export positions. The
  handlers are one body of Go: a surface supplies only the name an error
  gives the word (`Scry.sig` / `Debug.sig`) and the code an unknown word
  raises (`scry_unknown_word` per BORU-SCRY §4 / the historical
  `debug_error`). The copies stay exported — a timeline, not a break.
- `describe` names the canonical home: each `Debug.*` doc reads
  "Deprecated (NUR063): canonical home boru:scry (`Scry.<word>`); this
  frozen copy is removed in the first minor release after the one that
  ships boru:scry" — the stated timeline (BORU-SCRY §9 Q1's lean, keep
  through one release). The module catalog says the same.
- Profiles: every shipped profile admits modules by allowlist and none
  lists `boru:debug`, so `boru:scry` is denied identically (BORU-SCRY §8)
  with no profile change.

Pinned: `lang/spec/module-scry.tsv` (every export, positive and negative
rows, §4 the two surfaces answering alike under `deq`); modules
`TestScryExportsTheSelfKnowledgeWords`,
`TestDebugSelfKnowledgeIsDeprecatedToScry` (the seven marked, naming their
twin and the timeline; no other Debug word marked).

---

## NUR064 — Pattern clauses route-and-bind in `receive` but route-only in `add` {#nur064}

**Status:** FIXED 2026-09-26 (add patterns bind as receive clauses do —
the handoff log's entry of that date) · **Recorded:** 2026-08-12 ·
**Surfaced by:** `design/STATE-MACHINES.0.md` §8 (which names the asymmetry
while declining to solve it there); flagged for this register by the PR #345
review (Codex P1). The split itself was designed deliberately in
`PROCESSES.0.md` §3 and `SERVICES.0.md` §1 but never recorded here.

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

**The fix (2026-09-26).** The verdict's condition holds: both modules are
built (`lang/go/native/native_process.go`, `native_service.go`), so the
design line decides on implementation experience — and it generalises the
binding slots, the resolution that keeps "match a message" one thing. The
implementation had drifted from both design texts: `add` did not silently
ignore a `name:Type` field, it REFUSED it ("pattern value for "text" must be
a Scalar, got Scalar").

- One reading: `splitClausePattern(r, pat, op, code)` splits a pattern into
  routing tags and binding slots for `receive` (raw, a slot's type spelled
  as a word) and `add` (evaluated, a type literal) alike, with one error
  message shape.
- One guard: routing picks the handler by its tags; its slots then decide
  whether it takes the request (`bindSlots`). A declining one falls back to
  a slot-free catch-all (`add {} …`, a `{}` clause), else the call raises
  `no_match` "routed to a handler but failed its typed binding slots" — the
  `receive` text.
- One binding: the slots' fields are frame bindings around the code the
  clause runs (`withSlotBindings`) — a `receive` body, an `add` handler (and
  the fns it calls or builds, by dynamic scope); `req` still carries the
  whole request.
- Layering keeps its identity by routing tags: each stacked handler carries
  its own slots, bound from the request it receives, so a request `prior`
  passes on without a lower handler's field reaches no handler (`no_match`).
- The check: the handler's body is analysed where its fn literal is built,
  before `add` names the slots, so `add`'s check-mode half
  (`serviceAddCheck`) notes the body's word tokens that read a slot
  (`CheckState.SlotBoundReads`, by name AND position), and
  `RescueForwardRefDiagnostics` excuses exactly those — a misspelt read, or
  the name read in code no slot binds, stays `undefined_word`.

Raw patrun (`add {pattern} value patrun`, `find`) stays scalar-only: it
stores values, not code, so there is nothing to bind into. Pinned: lang
`TestNUR064AddPatternBindsItsSlots`, `TestNUR064DecliningSlotsRaiseNoMatch`
(both lanes, with the `receive` twins), `TestNUR064SlotReadsPassTheCheck`;
native `TestServiceCoverTopSlotsOfAnEmptyStack`. `PROCESSES.0.md` §3,
`SERVICES.0.md` §1 and `STATE-MACHINES.0.md` §8 state the one semantics.

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

**Status:** RESOLVED 2026-09-26 (one set of guarantees for both classifier
spellings — the handoff log's entry of that date) · **Recorded:**
2026-08-14 · **Surfaced by:**
`design/STATE-MACHINES.0.md` §3.6 (which introduces both spellings and states
the asymmetry as a preference rather than resolving it); flagged for this
register by the PR #352 review (Codex P1).

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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

**The resolution (2026-09-26, the reverse-order run's goal to resolve every
record, which supersedes the deferral above).** Open question #7 is decided
in the design, on the register's own rule — one role, one set of
guarantees — by the third candidate, strengthened until every divergence the
record lists is gone:

- **Alphabet closure.** A `classify:` declaration names its output alphabet,
  `classify: {fn: by-fields  yields: [header row trailer any/q]}`, and every
  `yields:` atom must be a declared `events:` member — `state_unknown_name`
  at define time, exactly as a `classes:` key. A fn returning an atom outside
  its `yields:` has classified the input to no declared class: the table
  form's unmatched input, `state_bad_event`, the same code.
- **Payload shape.** The fn returns the class ATOM, not an event map, and the
  machine builds the frozen `{event: <class>  raw: v}` of §3.6.2 — a reducer
  reads `ev.raw` whichever form classified the input.
- **Diagnostics.** `state_bad_class` covers a malformed `classify:` (no
  `yields:`, an empty or duplicated one) as it covers a malformed table, and
  `state_class_gap` (Info) flags either form without an `any/q` class.
  Disjointness needs no check for a function.

What stays different is only what must: the mapping inside the fn is opaque,
so `classes:` remains the form `State.graph` can draw per input, and the
preferred one where a partition describes the inputs. The design's §3.6.1,
§3.6.2's payload paragraph, §6.3's `state_unknown_name` / `state_bad_class` /
`state_class_gap` rows and open question #7 carry the decision; there is no
code to change, the module being unbuilt, and whoever builds it builds these
guarantees.

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

**Status:** RESOLVED 2026-09-26 (the parameter name is part of the value —
the handoff log's entry of that date) · **Recorded:** 2026-08-16 ·
**Surfaced by:** `design/legacy/unison-hash-identity-probe.0.ignore` P4, a
proof-of-concept pass over the canon contract; flagged for this register by
the PR #376 review (Codex P1).

**The resolution — the name is not incidental.** The record's rule holds
and is kept: a canonical rendering depends on the value, not on names
incidental to how it was written. What fails is its premise, "the two
functions are behaviourally indistinguishable". In boru a parameter is a
FRAME BINDING on the def stack, and a free name resolves at call time
through that stack — the documented scoping model (a name bound inside fn
F "is visible — via boru's dynamic scoping — to any fn F reaches on the
call stack", design/FUNCTION-VALUE-SCOPE.0.md §7.4). So a parameter's name
is visible to every function the body calls, and renaming it changes the
function:

```
def x 1  def g fn [[] [Any] [x]]
def f fn [[x:Any] [Any] [g]]  f 5     ->  5    (both lanes)
def f fn [[y:Any] [Any] [g]]  f 5     ->  1    (both lanes)
```

Unison can de-name because its variables are lexical; boru's parameters
are not. A canon that erased the name (candidate 1) or a `deq` that
compared an alpha-normal form (candidate 3's identity half) would equate
functions that behave differently — the NUR031 failure (`unique`
discarding a function it should keep) in the other direction. That the
record's own example body, `mul x x`, reaches no callee that reads `x`
does not change the rule: whether a body's callees observe a name is not
something equality can decide per pair without making `deq` depend on
what the rest of the program binds. So canon renders parameter names and
`deq` compares them, uniformly, and nothing in the code changes.

The one document that prescribed the opposite is corrected:
`design/CONTENT-ADDRESSING.0.md` §4.2's step 3 ("De-name parameters, to
positional references") is withdrawn as unsound, with the measurement, and
its summary table no longer lists alpha-normalisation. Pinned: lang
`TestNUR074ParamNameIsPartOfTheValue` (the behaviour on both lanes,
renamed parameters not `deq`, the same spelling and a rebinding `deq`).

**Reviewed 2026-09-25 (the reverse-order NUR run).** The recorded verdict stands and nothing in this run moved it; left pending on its design line.

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
`scripts/hash-identity-probe.sh`; `design/legacy/unison-hash-identity-probe.0.ignore` §4;
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

**Status:** FIXED 2026-09-25 (the Apply op — the handoff log's entry of that
date). **Recorded:** 2026-08-16 · **Surfaced by:** fixing the
`OnCall` frame-skeleton over-count — the round-trip contract held for every
NAMED call once that landed, and the residue was exactly this shape

**The fix.** The op exists: `Apply{Arity}` (`lang/go/stackform`), recorded
where the engine reports a fn VALUE's application with no name
(`execFnDefSig`'s splice, `OnCall("")`) and replayed by `Flatten` as the
window's rotation — `swap` for one argument, `rot` for two — followed by a
statement end, which resolves the re-stepped value from the stack by the
interpreter's own binding rule (top of stack first); a 0-argument value is
applied where it stands by the `apply` word. Two accounting defects fell
first. The recorder credited a native's Function-valued result a skip it
could never spend (a dispatching value never fires `OnPushLit`), and the
credit swallowed the application's first argument: `(m.f 7 2)` recorded `7
apply/2` — `recordDispatch` counts only the results that push. And at
replay an unquoted fn result dispatched at the pointer before the Apply
could rotate it, collecting the wrong window (`a=2, b=7`): every `Call` now
flattens with a statement end behind it, which parks a Function result until
the Apply re-steps it and is inert after any other. Wider windows still
decline (`ErrUnnamedApply`). Hole 2 — `apply`'s own double recording — is
closed by a DECLINE, not a replay: the word's Function overload hands its
value back for the engine to re-step, so the fn's dispatch records right
after the word's; replayed as written, `5 f/v apply` ran f twice (7 for 6,
measured). The recorder marks that Call (`Call.ReStep`, the word with a
dispatching result) and `Replayable` refuses the form (`ErrApplyReStep`); the
lens overload, which returns a value, replays. Pinned:
`TestStackFormReplaysFunctionValueApplication` (the record's three witnesses
and a two-argument map member), `TestStackFormDeclinesApplyWordReStep`
(`lang/go/test/stackform_equivalence_test.go`); `TestStackFormLiteralAccountingExact`
keeps its literals. The second face (the sticky Quoted mark on a pushed
Function, undone by `Eval`) stands as recorded.

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

**Status:** FIXED 2026-09-26 · **Recorded:** 2026-08-17 · **Surfaced by:** the
2026-08-17 ADR-011 amendment (clause 2 of the 2026-08-16 `/r` ruling);
flagged for this register by the PR #381 review (Codex P1)

**The fix (open-work item B, 2026-09-26).** All four sites retired
together. `stepWord`'s TFunction intercept (A) and
`hasPendingForwardExpectingFunction` (B) are deleted, so a bare name bound to
a fn goes through the word dispatch at every slot; the plan's word arm claims
a fn binding BY VALUE only when the name is written `/v` — a `Function`-typed
slot included — and treats the check pass's stand-ins the same way (a
fn-typed carrier, and a dynamic binding at a Function-typed slot, which fills
it only by holding a fn), so the compiled lane cannot collect by value what
the interpreter calls. `sigWantsFunctionAt` (C) and the ReachGroup arrival's
`ConformsTo(TFunction)` exemption (D) are deleted, which re-opened the NUR038
call-head question as the verdict said: it is answered by the one rule that
was already there — a reach-read fn that WOULD CLAIM the next token is a call
head, a claim-less one an operand, whatever the collecting slot's type — so
`each M.inc [1 2 3]` still passes the reference (`inc` cannot claim a list)
while `filter M.big [1 2 3]` calls. The `/v` spelling had to work everywhere
the bare one used to: a reach's `/v` marker is consumed by the forward scan
(it is never an argument — `mini M.dbl/v 'ab'` counted it as mini's String),
the reference is DELIVERED unquoted as `inc/v` is (a quoted delivery was a
value the interpreter's token seam stepped as data while the compiled lane
applied it: `[1 2 3] each M.inc/v`, `if true m.f/v [2]`), the modifier sugar
wraps a reach in a paren so `m.a/u` modifies the value, `mini` / `emit` /
`parse` apply a transducer however it was quoted, and a `/v` word after a
landed value is a collected value, not a function word (`m.g z/v` is 7 on
both lanes; it raised a false `uncalled_function` compiled). The
`unused_def` use-recording lives on the `/v` read (ResolveRef), where the
explicit spelling already recorded it. Every `h zero` call site in the tree
became `h zero/v`: `path-modifier.tsv:67` is rewritten to the call it now is
(`ERROR:cannot call `wa``), the sweep's four module-export seeds, and the spec
and unit rows that passed a callback bare. NUR190's Function-typed half
dissolves with it (`m.g z` is the named no-match on both lanes; the VM
landing's `vm:landing-claim` arm is gone). Pinned: lang
`TestNUR078BareFnNameCalls`, `TestFunctionSlotArgIsNotUnused`,
`TestNamedFnCandidatesOpenShapes`, `TestWordReadDispatchParity` /
`…FailsToCompile`; eng `TestReStepLandingWalk`; core
`TestS7PendingForward*` retired with site B.

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
(`design/legacy/O1-RELITIGATION.0.ignore` §2) and the amendment stands: a bare name
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


## NUR080 — A typed def over an Integer literal loses its newtype brand under the compiler, and the bare literal gains one {#nur080}

**Status:** FIXED — verified 2026-09-25 (the brand in both orders — the
handoff log's entry of that date) · **Recorded:** 2026-08-18 · **Surfaced by:** the
Roc comparison study (`design/legacy/roc-in-boru-report.0.ignore` §7.2), while
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
and answers wrongly, which puts this beyond the reach of `MarkUncompilable`
altogether: that machinery can contain a refusal (itself a defect owed a
fix); it cannot contain a wrong answer. Compiled mode is the
execution default since the P7 endgame, so this is the default answer.
A String newtype (`def Name (refine String)`) did not reproduce it.

**Evidence:** the shape — a literal's identity changing according to
whether a typed def elsewhere in the program mentioned it — points at the
const-pool entry for the literal being reparented rather than a fresh
value being minted, i.e. the residual of
`design/legacy/MISCOMPILE-HUNT-FINDINGS.0.ignore` §B, whose July-2026 update states
"The static/concrete path is untouched." No `lang/spec/*.tsv` row covers
`typeof` or `is` over a typed def of a literal in both source orders,
which is why `make verify-bytecode` is blind to it.

**Verdict:** resolve by fix — the static/concrete arm of `def name:T
<literal>` must mint a fresh value rather than reparent the shared
const-pool entry. Acceptance requires both source orderings, plus new
spec rows so the differential corpus covers the class. This record retires
when the two engines agree on the table above.

**Retired (2026-09-25).** Measured on the current tree, both orderings
agree on both lanes: `typeof 9 typeof b b is UserId` is `Integer UserId
true`, and `typeof b typeof 9` is `UserId Integer` — the typed def mints
its own branded value and the literal keeps its own (the const-pool
reparenting the record traced no longer happens; the fresh-value arm the
verdict asked for is the typed-bind path's, `ReparentValue` over a copy).
The acceptance rows are user-types.tsv's closing section (both orders)
and lang `TestTypedDefLiteralKeepsItsBrandInBothOrders`.

## NUR081 — `Test.skip` accepts the counts `Test.check-prop` now rejects, though it is documented as a drop-in {#nur081}

**Status:** FIXED 2026-09-25 (one contract for the family — the handoff
log's entry of that date) · **Recorded:** 2026-08-18 · **Surfaced by:** the
Wave 1 vacuous-property fix (`design/QUINT-FOLLOWUP-PLAN.0.md`), in review

**The fix.** `Test.skip`'s property form applies `Test.check-prop`'s
argument contract before it parks the property: `runs` below 1 and
`max-shrinks` below 0 raise `range_error`, blamed on `Test.skip`
(`requirePropCountFor`, lang/go/modules/test.go). Pinned by two
`module-test.tsv` rows.

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

**Status:** FIXED 2026-09-25 (one walk for the tree commands — the handoff
log's entry of that date) · **Recorded:** 2026-08-18 · **Surfaced by:**
giving `boru check` directory targets (W-CLI-CHECK), which forced a
choice between the two rules already in the tree

**The fix.** The proposed verdict: `pathutil.WalkSources` is the ONE tree
walk `fmt`, `check` and `test` share, carrying the `.boru/` skip and the
filename suffix; `boru test`'s discovery no longer descends into the
package directory (pin: `TestDiscoverSkipsPackageDir`).

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

**Status:** FIXED 2026-09-25 (the script's own directory — the handoff
log's entry of that date) · **Recorded:** 2026-08-18 · **Surfaced by:**
multi-file `boru check` (W-CLI-CHECK), which cannot use the cwd rule

**The fix.** The remaining half: `run` and `debug` anchor a script's
relative imports at the script's own directory, as `check` and `build` do —
`lang.Options.BaseDir` (applied to the registry by `lang.New`) is set from
the script path in run.go and debugcmd.go, and both pre-flights go through
`PreflightColorAt` with the same anchor; `-e` and the REPL, which have no
file, keep the process cwd. `boru run sub/m.boru` now succeeds from the
parent directory (pin: `TestRunAnchorsRelativeImportsAtTheScript`).

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

**Status:** FIXED 2026-09-25 (fmt answers -h — the handoff log's entry of
that date) · **Recorded:** 2026-08-18 · **Surfaced by**
`boru check -h` failing with `error: open -h: no such file or directory`
(W-CLI-CHECK), which `boru help check` tells users to run

**The fix.** The last non-uniform surface: `boru fmt -h` (and `--help`,
`help`) prints its usage to stdout and exits 0, as `check` does, instead of
reading `-h` as a file to format. Pinned by `TestFmtHelpExitsZero`.

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

**Status:** FIXED 2026-09-25 (six spellings, one form — the handoff log's
entry of that date) · **Recorded:** 2026-08-19 · **Surfaced by:** writing
`STYLE-GUIDE.md` §S1 ("use the fewest square brackets") and measuring
what `boru fmt` actually does with each spelling

**The fix.** The proposed verdict: `elideFnTriple` reads a wrapper as
`params… ret body` — the body is the last list, the return the token before
it (bracketed or bare), the params everything else (one bracketed list or a
bare run) — and `elideBareTripleRet` handles the wrapperless spelling, so
each of the five non-target spellings rewrites to `fn x:Integer Integer [mul
2 x]` (the unnamed twin to `fn Integer Integer [add 1]`); the irreducible
shapes (two or more params, a bare `List` param, a multi-overload spec list)
pass through as before. Pinned by
`TestFormatCollapsesEverySingleParamFnSpelling` (lang/go/formatter).

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
`boru fmt` on each spelling; `design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` §4.2;
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

**Status:** FIXED 2026-09-26 · **Recorded:** 2026-08-19 · **Surfaced by:**
the function-type prototype (`design/legacy/FUNCTION-TYPES.0.ignore`), while
establishing which of the audit's combinator blocks check clean and why

**The fix.** Not a misbinding after all: `g` WAS bound to the second
lambda. The check pass bound an analysed body's params and captures with a
raw `Defs.Push`, where the run's frame binds them through
`core.InstallFrameBinding`, which compiles a fn value's authored signatures
into dispatch-ready ones. A named fn's signatures were compiled at its
`def`, so `e/v` dispatched; an inline lambda's authored signature (afn
builds it to dispatch as a VALUE, straight from the authored form) carries
no argument types, so `g x` inside the body matched nothing whatever `x`
held — which is why the reported type followed the final argument.
`RunFnBodyOnce` now binds a concrete fn value the run's way
(`bindFrameValue`), and every other value is the plain push it was. The
minimal pair and the S-combinator pair check clean in both spellings and
run to the same answer; a String the run rejects is still reported in both
spellings, naming its real type. Pinned: lang
`TestInlineLambdaChecksLikeReference`, check
`TestRunFnBodyOnceCallsFnValueParam` / `…Capture` (both fail without the
fix). The combinators themselves still decline to compile ("fn bb: body
result of unknown provenance") on both spellings — a compile defect of its
own, not this record's.

**Reviewed 2026-09-25 (the reverse-order NUR run).** Re-measured on the
record's witness: `boru check lam.boru` still reports `no_signature: cannot
call `g`` at 1:51 for the inline lambda while the `/v` spelling passes, and
both run to the same answer. The inline lambda's Function-typed param
carrier reaches the inner body's analysis without the value's signatures,
where the named reference's carries them. Not closed in this run.

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
a declared `f:IntToInt` (`design/legacy/FUNCTION-TYPES.0.ignore`) leaves the lambda
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

**Status:** FIXED 2026-09-25 (the fn that took nothing — the handoff log's
entry of that date) · **Recorded:** 2026-08-19 · **Surfaced by:** probing
`fn`'s `(tnot List)` input guard while investigating NUR090 (retired
2026-08-20 — the name→node flip, `design/legacy/TYPE-REPRESENTATION.1.ignore`)

**The fix.** The silent path was the synthesized 0-argument fallback: when
the triple's `(tnot List)` rejected the input and the spec-list form found
no list, `fn` matched the fallback, constructed nothing, and left its
operands to the tape — which is also why `fn List [Integer] [size]` only
failed LOUDLY by accident (the body list ran as code and `size` raised).
`fn` now carries an explicit 0-argument signature whose handler raises
`signature_error` naming the rule ("expected a spec list or an
input/output/body triple after it — a bare List input is rejected by (tnot
List) …"), so `def f fn List Any [1]`, `fn List Any [1]` and a bare `fn`
all raise at the declaration on both lanes (the check pass reports it as
the compile's reason). The signature is declared `CompileDiverges` (the
handler always raises) and `def`'s synthesized keyword forms inherit that
one bit, so the declaration census stays at its ceiling. Pinned by two
`fn-triple.tsv` rows, back in the corpus now that NUR092's stale arm
consults the full breadth.

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
the variation sample and emptied an unrelated `varyCompileFailureLedger` bucket,
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

**Status:** FIXED 2026-09-25 (the stale arm consults the corpus — the
handoff log's entry of that date) · **Recorded:** 2026-08-19 · **Surfaced
by:** adding two rows to `lang/spec/fn-triple.tsv` for NUR091 and watching
CI fail in an unrelated bucket

**The fix.** The first candidate: `TestVariationDifferential`'s stale arms
re-check the UNSAMPLED rest of the corpus (lazily, once — the full sweep is
slow and an emptied bucket rare) before calling a ledger bucket or a pinned
variant stale (`ledgerStale`), so a corpus row unrelated to a refusal class
cannot instruct the author to delete the class's entry; a bucket the
default breadth merely failed to sample logs as unsampled-but-live. Pinned
by `TestLedgerStaleConsultsFullBreadth`. The NUR091 rows the record was
recorded over are back in `fn-triple.tsv`.

**Rule:** `vary.Sample`'s own contract, in its doc comment: *"Samples
NEST: `Sample(s, n)` is a prefix of `Sample(s, m)` for n < m, so cranking
breadth strictly ADDS variants and a bucket observed at the default
breadth stays observed at any larger one — **the property the CI ledger's
stale arm relies on**."*

**Divergence:** the nesting property holds across **n**, not across
**corpus content**. `Sample` orders seeds by `fnv64(seed.Input)` and
takes the first 32, so adding ANY spec row whose input hashes low enough
displaces a seed that was previously in the sample. If the displaced seed
was the only one exercising a refusal class, its `varyCompileFailureLedger`
bucket empties and the stale arm fires:

```
vary_differential_test.go:83: stale varyCompileFailureLedger bucket
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

**Status:** FIXED 2026-09-25 · **Recorded:** 2026-08-20 · **Surfaced by:**
adding two multi-return rows to `lang/spec/class.tsv` to pin the NUR095
retirement and watching `TestCheckTypeSoundness` fail on both

**The fix.** The plain check applies a fn-SHAPE-typed carrier the way both
lanes apply the fn it stands for (`tryFnShapeTypedWindow`, first in the
plain-check fn-value models): the shape's one declared signature is the
claim — a named shape's own content, or, for an anonymous `{op:(fnsig …)}`
field whose carrier is typed by the bare FunctionSignature node, the
signature `getObjectReturns` notes at the read (`FnShape.Returns`,
`ReturnsKnown`) — and its parameter count of evaluation-fixed tokens that
fit its parameter types is replaced by one carrier per declared return.
That is sound for any stored fn, whose returns the shape covers. It stands
aside where the run keeps the fn a value (a paren that places it, `/v`, an
argument that does not fit, no argument) and for a shape the types alone do
not decide (arity 0, several signatures, an optional, patterned or quoted
parameter). `c.op 10` over a two-result shape checks `[Integer Integer]`
where it checked `[T Integer]`. The two multi-return rows live in
`class.tsv` again (named and anonymous shape), and the production-registry
runs agree with them — the earlier review's `fn … 10` was the paren
spelling, which places the fn on every lane. Pinned: lang
`TestPlainCheckModelsFnShapeMemberApply`, check
`TestFnShapeTypedWindow*` / `TestFnShapeOfSpecDeclines`.

**Reviewed 2026-09-25 (the reverse-order NUR run).** The retirement test
was tried: with the two multi-return rows in `class.tsv` (`10 10`), the
default runner and `TestCheckTypeSoundness` (0 violations over the file)
now AGREE — the check pass models the apply — but `TestSpecProd` and its
TCO-disabled twin, which run the row on the PRODUCTION registry
(`native.DefaultRegistry()`, no check pass), answer `fn [[x:Integer]
[Integer Integer] [x x]] 10`: the interpreter alone leaves the
multi-return fn-shape member INERT where the single-return rows (L125,
L126) are applied. The rows were withdrawn again; the record stays open
on that new measurement, one lane deeper than it was recorded.

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

**Status:** FIXED 2026-09-25 (the late-binding hint — the handoff log's entry
of that date): the Allowed verdict below, with its hint as the mitigation.
**Recorded:** 2026-08-21 · **Surfaced by:** the
higher-order capability audit's §5.6
(`design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore`), re-assessed and pinned 2026-08-21

**The fix.** The verdict's diagnostic exists: `late_binding` (info). The
check pass records, for every named fn, the names its body reads
(`CheckState.FnReads`, from `recordUse` under the fn-name stack) and every
root-level def site by name (`RootDefSites` — the def-NAME token
`InstallAndRecordDef` stages, so a fn value, which carries no position of its
own, is sited too), and `EmitLateBindingHints`, run with the unused-def pass
at the end of `Check`, reports a name a fn reads that was bound by a root def
BEFORE the fn and RE-bound by one after it: ``c` reads `n`, re-def'ed at line
N; module names resolve late — the fn computes with the later binding (route
the name through a parameter to freeze it)`. A name first defined AFTER the
fn is an ordinary forward reference and hints nothing (the diagnostic-surface
sweep's `def f fn [[] [Integer] [g]] def g …` rows); a parameter, a
body-local and a def before the fn hint nothing; the semantics are untouched
(the §8 rows keep their answers). Pinned: `TestLateBindingHint`
(`lang/go/register_tail_test.go`).

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
`design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` §5.6.

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

**Status:** FIXED 2026-09-25 (one run per dispatch — the handoff log's
entry of that date; Discharge (b), the count is the contract). **Recorded:**
2026-08-25 · **Surfaced by:** the
Stage-2 collection-kernel feasibility probe for
`design/FULL-COMPILATION.0.md` (falsifier F1), which found overload
pruning to be an evaluation site the design had not accounted for

**The fix.** Traced, the four interpreter runs were the four planning
phases of ONE pending word — forward collection, the candidate scan, the
arrival, the final match — each asking the predicate type the same
question; the two compiled runs were the poly re-match and the entry
guard behind it. Both lanes now run the body ONCE per dispatch:
`RunPredicate` memoises its run-time verdict per (predicate, candidate)
until an effect may have moved the predicate's basis (`ClearPredMemo` at
every dispatch commit and at a statement end; the interpreter's predicate
fn carries no value ID, so its boru body is the identity), and the VM
skips `checkParamContract` behind a runtime poly re-match, which has just
matched every param with the interpreter's own rule. Found on the way and
fixed with it: a CARRIER candidate against a predicate-typed arm — `we (f
2)` over `[a:Even]` / `[a:Integer]` — was rejected by the lattice walk at
analysis, so the Integer arm committed statically and the compiled lane
answered int-arm for the interpreter's even-arm, silently. The predicate
unifier now ADMITS a check-mode carrier of the predicate's input type
(only the run can tell), so the arm stays reachable and the call goes
poly. Pinned: `TestPredicateRunsOncePerDispatch`
(`lang/go/register_tail_test.go`) — the "P" count on both lanes for `we
4`, `we 3` and `we (f 2)`. The differential harness still compares values
and errors, not effect counts; this pin is the effect-count comparison
for the predicate shape.

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
log, may touch a store, may be expensive. "Slow, not wrong" defends
nothing anywhere, and least of all here — the two lanes disagree
observably, and which one is *correct* is itself unsettled, since neither
count is obviously the specified one.

**Discharge.** Either (a) rule that predicate bodies must be pure, and
enforce it, at which point the count is unobservable and this becomes
Allowed; or (b) make the count part of the contract and have both lanes
produce it, which the full-compilation plan's §6.3 predicate-unit work
would need to do anyway. Whichever way it goes, the differential harness
needs an effect-count comparison, or the next divergence of this shape is
equally invisible.

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


## NUR104 — a record type means two different things depending on how it is spelt {#nur104}

**Status:** FIXED 2026-09-25 — Allowed: the install-time resolution IS the
uniform rule (the handoff log's entry of that date). **Recorded:**
2026-08-26 · **Surfaced by:** tracing NUR103's nameless `undefined_word` —
the mechanism turned out to be general, so the neighbouring shapes were
probed and one of them diverged outright

**The fix.** The two spellings differ in WHEN the field map is evaluated
(the named type's dispatch, the inline pattern's sig install), not in what
the type means: `ResolveSigRecordFields` resolves every field shape the
grammar admits at install — a bare type word, a nested record, a type
EXPRESSION, an optional field — so a consumer reading an inline record's
field types reads the same types the named spelling gives it. Measured
today on both lanes: `def R (refine Record [{a:(Integer tor String)}]) def
f fn [[o:R][Any][o.a]]  f {a:7}` and its inline twin both answer 7,
`o:{pretty:Boolean}` true, `o:{y?:String}` over `{}` None. The
"construction asymmetry" the Discharge kept open is the grammar's shape
(an inline pattern rides an inert fn-spec list) and has no observable
consequence left; it is Allowed. Pinned: `TestRecordTypeSpellingsAgree`
(`lang/go/register_tail_test.go`).

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

## NUR147 — the poly native seat commits an arity over a gradual residual, and the run refutes it {#nur147}

**Status:** FIXED 2026-09-25 (the seat's other arities — the handoff
log's entry of that date). Recorded 2026-09-16.

**The fix.** `callPoly` re-matches only at the recorded count; when that
fails it now tries every other arity some non-fallback overload takes,
from the recorded count down, over the same stack top (`polyHasArity`,
`MatchSignature` at k), and dispatches the first that matches — the
overload the interpreter's matcher takes over the live values. `def svc
(service {})  add {} ([r:Map state:Any] => [1]) svc  call {} svc  call {}
svc` is `1 1` compiled, no defer — and so are the `do`-wrapped shapes
whose bail NUR149's propagation rule used to carry out (`do [call {} svc
call {} svc]`, the same inside a fn body and a nested `do`), which now
compile and answer as the interpreter does (`TestDoDeferFallsBackNotTrapped`;
the fn-body one exposed NUR118). Pinned by lang
`TestPolySeatRetriesOtherArities`.
**Found:** by the stored-handler latch's F1 measurement
(`lang/go/bytecode_stored_handler_freeze_test.go`,
`TestStoredHandlerMidProgramRebindCompilesAndMatches`): the program
compiles once the latch lifts, and its run falls back to the interpreter
at the second `call`. Reproduced on `main` with no dependency at all:
`def svc (service {})  add {} ([r:Map state:Any] => [1]) svc  call {} svc
call {} svc` answers `1 1` on the default lane by whole-program fallback
and raises an internal error under `-force-compile`.

**Rule:** all valid code compiles, and a program that compiles runs
compiled; a runtime bail is a defer the census counts
(`vm:poly-no-match`, one of its two) — a defect the runtime contains, not
an outcome the design allows, and every one is owed a retirement.

**Divergence.** `call` has two overloads, `[Map Service]` and `[Map
Service Map]`. The first `call` leaves its handler's residual on the
stack as a GRADUAL carrier (the handler's return is not declared). The
check pass matches the second `call {} svc` with that carrier standing
in for the third Map: `MatchSignature` takes the three-operand overload,
and the poly record commits to three operands (`CALL_NATIVE_POLY call/2`
in the listing names the word; the record's `NArgs` is 3). At run time
the third value is the Integer the first call produced; the
three-operand overload does not match, and the seat re-matches only at
the recorded count (`window` is built from `n` stack values with
`WordInfo{ArgCount: n}`), so the two-operand overload the interpreter
takes is never tried: `vmDeferAlt(... "vm:poly-no-match")`.

**Fence.** The F1 pin asserts the program compiles and the run matches
the interpreter by fallback, and names this entry at the site.

**Verdict:** none yet. The seat's own: a poly record whose matched
overload consumed a GRADUAL operand at a position a shorter overload
would not consume is a speculative count, and the op must be able to
re-match at the shorter count — the value-stack claim is the run's, not
the pass's — with the stack sim's consumption reconciled as the generic
op's claim-drift check reconciles it, or the record must carry the
overloads' counts and let the run choose. Until then the second call of
any service is the interpreter's.

## NUR148 — the stored-handler lookup half's first cut: four divergences found in review {#nur148}

**Status:** Fixed (recorded and closed 2026-09-16, the seventy-first
increment, #467 — the first cut's review by Codex; each fixed in the same
PR).
**Found:** review of #467's first cut (4f5fee1), three reproduced on that
tree, the fourth on the restore predicate.

**Rule:** a compiled program answers as the interpreter does, and all
valid code compiles — a refusal is a defect owed a fix, never a
sanctioned outcome.

**Divergences and fixes.**

1. *A live READ rebound to a dispatching value.* `def k 6  def svc
   (service {})  add {} ([r:Map state:Any] => [print "x" k]) svc  undef k
   def k fn [[][Integer][11]] end  call {} svc`: the interpreter prints
   `x` and returns 11 (a bare word bound to a fn dispatches it); the
   compiled handler's live lookup (`OpLookupDynScope`) met a fn, deferred
   past the print, and raised an internal error. Fix: a module-scope
   transition of a name a stored unit reads live to a binding the lookup
   op would DISPATCH — a fn, a class, an active token — refuses through
   the undef site (`liveReadDispatching`, at `RecordBindTwin`).
2. *A name read both ways.* `add {} (… => [helper 5 drop def h helper/v 5
   h/v apply]) svc  def helper fn [[x:Integer][Integer][x add 2]] end
   call {} svc`: the routed lead was live, the `/v` read a bake, and
   marking the name live skipped the latch — 6 for the interpreter's 7.
   Fix: a stored unit counts its frozen notes (`NoteFrozenRead`'s stored
   arm: a value const, a type identity, a committed call target) against
   its live seats; a name whose bakes outnumber its seats is not live for
   the ref, and the latch keeps it.
3. *The admission by word.* After a stored handler dispatched `helper`,
   `def f fn [[x:Integer][Integer][def helper fn [[y:Integer][Integer][y add
   10]] end  helper x]] end  f 5` routed the body-local helper's call as a
   live module lead — 6 for the interpreter's 15. Fix: the admission rides
   on the dispatch's descriptor (`RegionDesc.LiveLead`, set by the stored
   unit's own record), never on the word.
4. *The restore predicate.* `Program.LiveLeadNames`/`LiveReadNames` had
   joined the condition that rolls the registry back to `ReplayBase`
   before a run; a program with a live lead and no transition, re-run
   after a later request rebound the name, undid that rebind (7 back to
   6) and left the registry rolled back. Fix: the sets leave the
   predicate — a live read implies no transition to replay.

**Fence.** `lang/go/stored_handler_live_test.go` (the refused rows, the
body-local row, `TestStoredHandlerLiveNamesDoNotRestoreBase`),
`compiler/go/stored_live_test.go`, `eng/go/vm_replay_base_test.go`.

**Verdict:** closed with the fixes; recorded because the rule says every
divergence surfaced in review is recorded, fixed or not.

## NUR150 — the fn-local placement's first cut: three divergences found in review {#nur150}

**Status:** FIXED 2026-09-16 (closed with the fixes in #468, the
seventy-second increment; the register marked it Pending until 2026-09-25
by oversight — its verdict below says closed).
**Found:** review of #468's first cut (286a6c9), all three reproduced on
that tree.

**Rule:** a compiled program answers as the interpreter does, and all
valid code compiles — a refusal is a defect owed a fix, never a
sanctioned outcome.

**Divergences and fixes.**

1. *Only the first local placed.* `BodyRefsFnLocalFn` reported the FIRST
   fn-local fn a body named, so a body naming two — `def f …  def h …
   do [raise x "e"] error [[f 5 h "b"]]` — placed `f` alone; the islanded
   handler resolved `h` in the registry and raised `undefined word: h`
   for the interpreter's `[6 'b']`. Fix: `BodyRefsFnLocalFns` reports
   every local the bodies name, each once, and the site places each or
   refuses.
2. *A stale def event stamped.* The placement matched the unit's latest
   def event by NAME alone; a closed body's redefinition — `do [def f fn
   [… x add 10 …] end]` before an islanded `error [f 5]` — leaves the
   current binding with no event of the unit's, and stamping the
   original's installed the original: 6 for the interpreter's 15. Fix:
   the event must be the current binding's own declaration
   (`sameFnDecls`: the same signature count and declaration sites), or
   the site refuses. Two first-cut rows that answered by index — the
   redefining body reading its own redefinition (`55`, `111`) — refuse
   under the rule; the interpreter absorbs them, so the answers match, and
   two refusals are now owed a fix.
3. *A value read admitted.* `do [f/v]` returned the fn as data from the
   compiled closure where the interpreter dispatches the returned value
   and raises `uncalled_function`; the guard had refused it. Fix: a
   `/v` or `/u` reference to a local keeps the refusal
   (`BodyRefsFnLocalFns`'s value-read report).

**Fence.** `lang/go/fn_local_placed_test.go` (the islanded two-name row,
the unit-level redefinition row, the four refused rows),
`compiler/go/fn_local_test.go`, `compiler/go/carrier_nur037_test.go`
(`TestBodyRefsFnLocalFnsAllNamesAndValueReads`).

**Verdict:** closed with the fixes; recorded because the rule says every
divergence surfaced in review is recorded, fixed or not.

## NUR151 — the seventy-third increment's first cut: two divergences found in review {#nur151}

**Status:** FIXED 2026-09-16 (closed with the fixes in #469, the
seventy-third increment; the register marked it Pending until 2026-09-25
by oversight — its verdict below says closed).
**Found:** review of #469's first cut (ea3ac11), both reproduced on that
tree.

**Rule:** a compiled program answers as the interpreter does, and all
valid code compiles — a refusal is a defect owed a fix, never a
sanctioned outcome.

**Divergences and fixes.**

1. *A user `raise internal_error` no longer caught.* `bodyErrorPropagates`
   keyed the re-raise on the public `internal_error` code (`IsInternalError`),
   but a user `raise internal_error "boom"` carries that same code — so
   `do [raise internal_error "boom"] error [dot code]` re-raised instead of
   catching and returning the code, in BOTH lanes (the shared `do` handler),
   and the comment's claim "the interpreter never raises internal_error" was
   false. Fix: a designed VM defer now carries a distinct marker
   (`BoruError.VMDefer`, set by `vmErrAt` and the panic guards); `IsVMDefer`
   keys the escape hatch on the marker, not the code, so a user error stays
   trapped and a real defer propagates.
2. *An in-function speculative family over-refused.* The family-L-in-fn-body
   refusal keyed on `specFnJoin(name)` alone, so a fn that creates `f` in an
   undecidable in-fn branch and then unconditionally redefines it LATER in
   the same fn was refused too — but that family is absent at the fn's
   baseline, its install and replacement stay above it, and RET pops them
   (no leak past the call). Refusing it rejected a shape that is not the
   NUR149 leak. Fix: `specFamilyAtFnBaseline` gates the refusal on the
   dropped binding existing at the enclosing fn's baseline (a MODULE-scope
   family), so an in-function family is not refused (it compiles; its routed
   dispatch may still defer at run time and fall back — a deferred defect the
   runtime contains, not an outcome the design allows).

**Fence.** `lang/go/do_defer_fallback_test.go` (the user-internal-error
trapped rows, the in-function-family not-refused row),
`core/go/check_fncarrier_test.go`
(`TestInstallDefDoesNotLowerSpecFamilyRedefinitionInFnBody` — the baseline gate
and `specFamilyAtFnBaseline`'s arms), `core/go/rununit_test.go`
(`TestIsVMDefer`).

**Verdict:** closed with the fixes; recorded because the rule says every
divergence surfaced in review is recorded, fixed or not.

## NUR152 — a fn value had a home only if a module exported it {#nur152}

**Status:** Resolved (recorded 2026-09-17; fixed in the same PR).
**Found:** investigating the main-vs-module representation split, at the
maintainer's request — "code loaded from the main file is in a module as
well; there should be no difference in code based on module" — reproduced
on `82816e9`.

**Rule:** a function value carries the scope it was written in; its free
names are looked up in its DEFINING module, at call time
(design/FUNCTION-VALUE-SCOPE.0.md §11, rule 1) — in both directions.

**The split.** `FnDefInfo.Registry` was set in exactly one place,
`resolveModuleExport`, so a fn defined in the main program carried nil,
and `FnHome`'s nil arm — documented as "a fn defined in the running
scope, whose defining registry IS r" — handed it whatever registry
happened to be running. That is true only while a main-program fn is
never applied anywhere but main.

**Divergences, interpreter first.**

1. *Main fn into a module, module has no `secret`.*
   `import module [def run fn [[f:Function][Integer][(f 5)]] export "M"
   {run: run/v}] end def secret 1 end def pub fn [[x:Integer][Integer][x
   add secret]] end M.run pub/v` — interpreted `cannot call add` (`secret`
   looked up in M, unbound), compiled `6`. A compile ≠ interpret
   divergence in which the interpreter is the wrong lane.
2. *Same, module has `def secret 100`.* `105` on BOTH engines — main's
   `pub` silently reading the module's `secret` — for the rule's `6`. No
   differential can see it; only the rule can. The lambda form
   (`M.run ([x:Integer] => [x add secret])`) is the same.
3. *After stamping the home at construction* (the first cut of the fix):
   interpreted `6`, compiled `105`. `compileStoredFnUnit` and the fn-value
   unit compile took `r := es.reg` — the emitter's CURRENT registry, the
   module's while `run`'s body compiles foreign — so `pub`'s body became a
   `storedfn$body` unit stamped with the module as its owner, and the VM
   read `secret` against the module at run time.
4. *Forks.* Comparing home to caller by POINTER sent a same-module callback
   back to the shared original from its per-connection `ForkConcurrent`
   clone: `boru:repl`'s service handler lost the fork's live state
   (`repl-eval-line: expected 1 return value(s), got 2`), `serve-raw`'s
   handler raced the acceptor — `fatal error: concurrent map iteration and
   map write` took the corpus differential down.

**Fix.** Every boru-bodied fn is stamped with the registry that minted it
(`FnConstruct`, `=>`, `macro`); a nil home is left to Go-built values,
which have no free words. A registry has a `Home()` — itself, or for a
concurrent fork the registry it was forked from — and every "is this fn
foreign?" test (`FnHome`, `FnHomeForeign`, the engine's dispatch arms, the
compiler's `foreignFnHome`, the VM's `tryNativeFnApply` and frame naming,
`deq`'s fn arm) compares homes, so a fn runs where it is invoked whenever
that is an instance of its own module. Export resolution mints a fresh
value whether or not the fn already carries its home (a region claim was
otherwise keyed by the `name/v` token's position inside the module body).
Stored-fn and fn-value units compile at the value's home, the caller's
check state shared onto a foreign one exactly as `tryRecordLambdaClosure`
already did. `IsInertConstMember` no longer keys on a nil home: a fn value
is immutable code whichever module it belongs to; only a capture is live
state.

**Fence.** `lang/go/fn_home_main_module_test.go` (rows 1–2, the lambda,
a main factory's closure returned through the module, and the control),
`core/go/fn_home_module_test.go` (`Home`, `SameHome`, `FnHomeForeign`
over main / module / fork), `eng/go/frame_name_test.go` (at-home vs
foreign naming), `lang/spec/module-composition.tsv` (the reverse-direction
rows), and the module suites (`boru:repl`, `serve-raw`) that the fork
rule keeps green.

**Not in this record.** Applying a returned closure INSIDE the foreign
body (`1 (f 5) apply` in `run2`) is a compile error today ("unmatched
dispatch recovered at apply") — a refusal, not a divergence, and a defect
owed its own fix. `Registry.ModuleScope` (the open-words rule) and
`ModuleRef` (the per-export policy identity) are deliberate module-only
designs and stay as they are; the main program still has neither.

## NUR153 — one stored `=>` value, two evaluation regimes {#nur153}

**Status:** RESOLVED 2026-09-18 — the tape rule implemented everywhere;
the divergence is closed and the fence pins the rule.

**Ruling (maintainer, 2026-09-18): follow the recommendation.** Regime 1
is the specification: a lambda's single container residual DEFERS,
whichever seam applies the value — the `CallBoru` / `InvokeCallback`
seam evaluates through the same rule the tape does. The by-name
admission (`storedfn$body` / `spawnbody$body` in `check/go/carrier.go`)
goes and the stamp follows the one rule; a handler that reads its
parameter inside a returned container (mini-redis's catch-all) is
written as a computing body; `TestServiceAddStampsComputedMapHandler`'s
pinned answer changes; the fence's three pins move to one rule and the
compiled tape-apply divergence closes with it.
**Found:** a Codex review of #471, which attributed the divergence to that
PR's island fix; measured on `origin/main` (the merge base) and present
there, and measured to be two-sided on the interpreter alone.

**Rule:** a function value means the same thing wherever it goes
(design/FUNCTION-VALUE-SCOPE.0.md §11). Its residual — here the single
container literal an anonymous fn returns — must evaluate under ONE rule
whichever seam applies the value.

**The two regimes, interpreter only.**

1. *Applied on the tape.* `def a 99 def api (patrun Function) add
   {cmd:"x"} ([a:Map] => [[a]]) api def h (find {cmd:"x"} api) h {z:1}`
   answers `[[99]]`: the lambda rule — the container is deferred past the
   frame and its bare `a` resolves in module scope
   (`EvalResidual: !anonymous || BodyEvalsResidual(body)`).
2. *Invoked through a native seam* (`InvokeCallback` / `CallBoru`). A
   `service` catch-all `add {} ([req:Map state:Any] => [ {message: (join
   "" ["unknown '" req.cmd "'"])} ]) svc` answers `unknown 'BOGUS'` to
   `call {cmd:"BOGUS"} svc` — the computed map read its param, in the
   live frame. (`lang/go/native/native_service_stamp_test.go`,
   `TestServiceAddStampsComputedMapHandler`, pins the plain-interpreter
   answer.)

Same value, same body, two answers to the same question.

**The compiled divergence.** The stamp is one unit. The fn-body
recordability gate (`check/go/carrier.go`, `RunFnBodyOnce`) admits a
`storedfn$body` / `spawnbody$body` BY NAME — regime 2 — which is what lets
mini-redis's catch-all handler stamp at all. A stamped stored `=>` value
then applied on the tape takes regime 2 where the interpreter takes
regime 1: row 1 compiles to `[[{z:1}]]`, silent, exit 0. The `fn`-word
twin (`(fn [[a:Map][List][[a]]])`) is `[[{z:1}]]` on both engines and
under both regimes, since a named fn evaluates in-frame everywhere.

**Why the compiler cannot close it alone.** Dropping the by-name
admission makes the stamp follow regime 1 — and then the service
catch-all stops stamping (measured: `TestServiceAddStampsComputedMapHandler`
fails "must stamp") while the interpreter still answers regime 2 on
`call`; the divergence moves, it does not close. The unit would have to
know how it is being invoked, which is the non-uniformity restated.

**What closes it.** The interpreter takes one rule. Either the CallBoru
seam defers an anonymous fn's single container residual exactly as the
tape does (regime 1 everywhere — mini-redis's catch-all then reads `req`
as unbound, and must be written as a computing body), or the tape
evaluates it in-frame (regime 2 everywhere — which is the no-closures
transparency of def-node-binding.tsv §3 removed for anonymous fns, a
language decision). Either way the by-name admission goes and the stamp
follows the one rule. A maintainer decision.

**Fence.** `lang/go/stored_callback_residual_test.go` pins all three
answers as measured — regime 1 on the tape, regime 2 through `call`, and
the compiled tape-apply divergence — so the day any of them moves, this
record is the first thing to update. `lang/spec/callbacks.tsv` carries the
`fn`-word twin (parity); the diverging `=>` row stays out of the main
corpus, as a divergence must.

**How it closed (2026-09-18).** The rule is ONE predicate,
`core.ResidualEvalsInFrame(anonymous, body)` (`core/go/fn_frame.go`): a
residual container evaluates in the live frame unless the fn is an anonymous
`=>` whose body is a single bare container, which defers. Every seam asks it
and none spells it for itself:

- the CallBoru sub-run a native seam invokes (`Registry.CallBoruNamed`) holds
  its end-of-run sweep for a deferring body (`Engine.DeferResidual`) and
  sweeps the pending container AFTER the frame teardown, in the scope the
  consumer would have evaluated it in — so a bare param name is exactly as
  unbound as the tape leaves it;
- the compiler's residual-recording admission (`check/go/carrier.go`) loses
  its by-NAME arm (`isCallbackBodyName`, `storedfn$body` / `spawnbody$body`,
  deleted): the condition was
  `isCallbackBodyName(name) || !anonymous || BodyEvalsResidual(body)`, which
  collapses exactly into the shared predicate.

**Measured.** The diverging row `def a 99 … ([a:Map] => [[a]]) … h {z:1}`
answers `[[99]]` on all three lanes (interpreted, compiled, compiled-strict);
it answered `[[{z:1}]]` compiled before. The seam and its tape twin now answer
identically, value AND error taxonomy: both `[]` with
`[boru/undefined_word]: undefined word: req` for a deferred container that
reads its param.

**The cost, and the migration.** Every `=>` callback whose body is a SINGLE
CONTAINER LITERAL READING ITS PARAMS now defers and raises — loudly, never a
wrong answer. The migration is one character pair: wrap the container so the
body COMPUTES (`=> [ ({message: req.cmd}) ]`), which evaluates in-frame as
before. Rewritten here: mini-redis's catch-all and todo-api's hits handler
(`design/examples/apps/`), the net codec handlers and the model action
(`lang/go/modules/net_test.go`, `net_stamp_test.go`,
`lang/go/test/model_test.go`), and the service stamp pin
(`native_service_stamp_test.go`). Eight tests moved in all — the ruling's
record named only mini-redis's catch-all, so the true scale is recorded here.

**What the gates measured.** The interp-entry census falls 54 -> 52: a
deferring anonymous callback no longer enters the interpreter through the
CallBoru seam to evaluate its residual in the live frame, so two
`each-variants` rows stop entering (`interpEntryRowCeiling`, lowered in the
same change). One corpus row is added pinning that the bare spelling now
raises, with a `unflaggedPins` entry — the checker cannot flag it statically,
since the name is a bound param where the checker reads it and only the
residual rule, applied as the container leaves the frame, makes it unbound.
Inserting that row shifted `each-variants.tsv` below it by one line, so
NUR155's ledger key moved `L215` -> `L216`; NUR155 itself is UNCHANGED and
still open — the key moved, not the defect.

**Fence.** `lang/go/stored_callback_residual_test.go` pins THE RULE in four
parts: the named-fn twin (in-frame everywhere), the tape rule, the seam
answering exactly what the tape answers (value and taxonomy), and the
computing-body migration working on both engines.

## NUR154 — a `case` over a factory-produced clause list miscompiles {#nur154}

**Status:** FIXED 2026-09-25 as a sound decline (the computed clause list —
the handoff log's entry of that date). Recorded 2026-09-17.

**The fix.** The wrong error was the check pass's: `CaseReturnsFn` recorded
a TERMINAL trap for any clause operand that is not a code body, and a
fn's returned list is a List CARRIER under analysis — a list or not only
at run time, where the interpreter reads the concrete value and answers
`'one'`. The trap is recorded only for a KNOWN non-list now — a scalar
or a bare type node (`case 1 5` and `case 1 Integer` still raise on both
lanes, natively); a carrier or dynamic operand returns the
dynamic result and the dispatch's own gate declines the computed clause
list ("code-body word case (Stage 2)"), so the program falls back and
answers as the interpreter does. The corpus ledger's `knownDivergences`
pin for code-bodies.tsv:L142 and the sweep's NUR154 pin are retired
(`sweepFailureCeiling` 26 → 27). Pinned by lang
`TestCaseComputedClauseListDeclines`.
**Found:** the corpus expansion in #471 (`lang/spec/code-bodies.tsv:L142`);
flagged by the Codex review of that PR as a row that must not join the
main corpus while it diverges.

**Rule:** a compiled program answers as the interpreter does; a compile
that cannot honour a shape refuses it rather than answering differently.

**Divergence, interpreter first.** `def mk fn [[n:Integer][List][quote [1
'one' 'many']]] end case 1 (mk 0)` — interpreted `'one'`; compiled
`[boru/case_error]: case: clause list must be a concrete list of
match/block pairs (optional trailing default)`, raised at run time on the
DEFAULT lane. `TestRegionCollectOracle` sees the same row error under the
oracle where the interpreter does not.

**Mechanism (located, not fixed).** `case` lowers its clause list as a
static literal operand; a quoted list a fn RETURNS is a run-time value
the lowering never reads. Either the clause operand is read live at the
`case` dispatch, or a non-literal clause operand refuses — and a refusal
is itself a defect owed a fix.

**Fence.** The differential (`TestSpecCompiledDifferential`) and the
region oracle both name the row; it stays in the main corpus so neither
can go quiet on it.

## NUR155 — a typed callback over a heterogeneous collection is applied to every element {#nur155}

**Status:** FIXED 2026-09-23 (the typed callback's contract — the handoff
log's entry of that date). Recorded 2026-09-17.

**The fix, in one paragraph.** The divergence lived in ONE route: a typed
lambda the compiler lowers as the higher-order word's own BODY unit
(`each$body`, `fold$body` — the lambda's param contract recorded on the
unit, `Lambda` false because it is not a fn VALUE unit), which the VM's
token seam (`invokeClosureOn`) entered blind. The same lambda over `[1 'a']`
took the stored-fn const route and matched per element already; over
`[1 none]` or `[[1] {b:1}]` the check pass chose the body unit. The seam
now asks the unit's own contract per element (`unmatchedLambdaBody`,
eng/go/vm.go, over the positional args the bind would seat): a list
body's no-match hands back the inputs with the closure on top — what
stepping the fn value leaves, the element's result the fn itself,
rendered as the interpreter renders the lambda — and a map body's
(ClosureInKeyVal) raises the map arm's `no matching lambda signature for
N argument(s)`. each-variants L216 agrees and left `knownDivergences`;
`0 fold ([a:Integer x:Integer] => [a add x]) [1 none]` is `fn (Integer,
Integer)` on both lanes (compiled 1 before), and the map fold `0 fold
([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:"s" b:1}` raises at key b
on both (compiled `[0s1]` before). Pinned in
`typed_callback_hetero_test.go` (lang/go). The original record follows.
**Found:** the corpus expansion in #471 (`lang/spec/each-variants.tsv:L215`);
flagged by the Codex review of that PR.

**Rule:** a compiled program answers as the interpreter does.

**Divergence, interpreter first.** `each ([x:Integer] => [typeof x]) [1
'a' [2] {b:1} true none]` — interpreted `[Integer fn [[x:Integer][Any]
[word(typeof) word(x)]] fn … fn … fn … fn …]`: the lambda is applied to
the one Integer, and for every element its signature rejects, the
dispatch falls through and the fn VALUE itself is the element's result.
Compiled `[Integer ProperString List Map Boolean None]`: the closure unit
is entered for every element with no signature check.

**Mechanism (located, not fixed).** The compiled callback dispatch binds
the element straight into the unit's param slot; the interpreter's
per-element `MatchFnSig` is not mirrored. Either the unit's entry keeps
the match (and falls through to the value on a miss, as the interpreter
does), or the lowering refuses a collection it cannot prove homogeneous
in the param's type.

**Fence.** `TestSpecCompiledDifferential` names the row; it stays in the
main corpus.

## NUR156 — a module-export fn value is not applied by the compiled lane {#nur156}

**Status:** FIXED 2026-09-22 (the quotation-body def reads — the handoff
log's entry of that date) for rows 1 and 2, with their local twins; row 3
now DECLINES loudly (see below). Recorded 2026-09-17.
**Found:** the corpus expansion in #471 (`lang/spec/module-composition.tsv:L102–L104`);
flagged by the Codex review of that PR.

**What it was — two defects, neither of them module-specific.** The
neighbours said so before any code was read: `5 (M.inc) apply` (the
unmarked group) compiled to `CALL_USER inc/1` and agreed, and the LOCAL
twin of row 2, `def h1 … def tbl {inc: h1/v} end def f tbl.inc end each
[f] [1 2 3]`, returned three fn values exactly as the module row did.

1. *The `/v`-marked reach group under `apply`.* The parser emits a
   `/v` on a paren or dotted-path result as a Word/__DM marker after the
   group; `execFnDefLiteral` consumes it at the collapse and marks the
   value QUOTED (core/go/engine.go, the marker peek). The runtime
   `applyHandler` clears the quote and hands the value back, and the
   re-step dispatches it. The check-mode model, `applyReturns`, was
   `ReturnsIdentity(0)` plus the 0-arg mark — the quote stayed — so on the
   pass the concrete lead PARKED as data, the re-step dispatched nothing,
   nothing was recorded, and `recordCallElided`'s "apply of a fn VALUE:
   the re-step records it" arm elided the apply itself: the program was
   `PUSH_CONST 5; PUSH_CONST fn`. A word's own `/v` read (`inc/v`) is
   delivered unquoted (`deliverValRead`) and never met this; a gradual
   member (`m.f/v`) takes the pending-apply event instead. The model now
   clears the quote as the handler does. `TestCheckTypeSoundness` read
   row 1 as type-unsound for the same reason, from the checker's side.
2. *A module-scope def bound to a fn-valued member, read bare in a code
   body.* `stepWord` never substitutes a binding whose value is a fn
   ("goes through normal Lookup"): the read is a WORD dispatch under the
   binding name — a 1-arg fn takes the element beneath it, a no-match
   raises `cannot call `f``, a named-param lambda with nothing beneath
   raises the same. The check pass binds the def to the CARRIER the
   member read produced (tagged NoteMemberFnRead), the closure unit
   captured that value into a slot, and its body was `PUSH_LOCAL ×2;
   RET`. `NoteWordRead` had deliberately not counted a read of an
   enclosing-scope binding ("not this unit's to seat"); it now records
   such a read's NAME when it is a def-table read (never the strict
   count — an unreached consumption keeps the slot push it has today
   rather than declining), `noteClosureBodyReplay` arms on a named def
   read of a tagged member with the word table, and the VM's word
   island (`callDynFrameWords`) dispatches a name the registry already
   binds to a fn THROUGH that binding rather than installing the
   captured value on top of it — installing it stacked a second,
   identical overload under the name and the no-match listed every
   candidate twice. A match runs native through the lone-token path
   (`invokeLoneToken`); the island takes the no-match and the
   nothing-beneath shapes and raises the interpreter's own error.

**Row 3.** `while [i lt 3] [def i (i M.inc/v apply)]` compiled to a
runaway loop ONLY because the apply inside its body never fired; with the
apply recorded it declines "dynamic-scope def `i` of unpromoted computed
value", where its local twin `def i (i inc/v apply)` always did — the
dynamic-scope def family's own gate, three corpus rows before this one.
It moved from the known-divergence ledger to the compile-failure ledger
(module-composition 2 → 3, corpus 20 → 21): the S1a trade, a loud failure
for a silent miscompile.

**What remains, pinned to fail when it moves**
(`TestDefReadWordDispatchPending`, lang/go): the same def read where no
replay window exists yet — at the MAIN program (`def f tbl.inc end 5 f`
is `[5 fn]` for the interpreter's 6) and inside a NAMED fn unit (`def g
fn [[Integer][Any][f]] end g 5` bails as a dynamic-scope read of a
dispatching binding). Both are NUR123's open points, the read model's.

**Pins.** `quotation_body_def_read_test.go` (lang/go):
`TestQuotationBodyDefReadParity` (fourteen rows),
`TestQuotationBodyDefReadNoMatchParity`,
`TestQuotationBodyDefReadSoundCompileFailures`; compiler
`TestNoteWordReadDefReadName`, `TestNoteClosureBodyReplayDefRead`; eng
`TestCallDynFrameWordsArms`' bound-name arm. The sweep's `apply` ×
module-export seed graduated from DIVERGED to passing (13 of 14 call
forms; the each-body variant declines in the twin-regime family).

**The original record follows.**

**Rule:** a compiled program answers as the interpreter does.

**Divergences, interpreter first.**

1. `import module [def inc fn n:Integer Integer [n add 1] export "M"
   {inc: inc/v}] end 5 M.inc/v apply` — `6`; compiled leaves `5` and the
   unapplied fn on the stack.
2. `… def tbl {inc: h1/v} export "M" {tbl: tbl}] end def f M.tbl.inc end
   each [f] [1 2 3]` — `[2 3 4]`; compiled returns three fn VALUES.
3. `… def i 0 end while [i lt 3] [def i (i M.inc/v apply)] end i` — `3`;
   compiled never advances `i` and ends in `tape_exhausted`.

All three are one defect: the `apply` (and the each-body apply) of a
MODULE-HOMED fn value does not fire on the compiled lane. NUR152's home
stamp does not close it (measured on `ceb067c`, where every fn carries
its home): the value is right, the lowering of its application is what
does not happen. `TestCheckTypeSoundness` also reads row 1 as TYPE
UNSOUND (`checked` and `actual` stacks differ), the same fact seen from
the checker.

**Fence.** `TestSpecCompiledDifferential` and `TestRegionCollectOracle`
name the rows; they stay in the main corpus. The reverse-direction rows
NUR152 added to the same file are the control: applying the fn INSIDE
the module compiles with parity; applying the exported value FROM main
does not.

## NUR157 — a predicate-typed container child refuses to unify with its own text under a registry {#nur157}

**Status:** FIXED 2026-09-25 (two references to one predicate — the
handoff log's entry of that date). Recorded 2026-09-18.

**The fix.** The one arm the record named: before the armed pre-pass
runs a predicate against the other side, `unifyInner` resolves BOTH
sides through `resolvePredicateRef`, and two references to the same
predicate type — atoms, bare nodes or the predicate's own fn value, as a
`[:Pos]` pattern's child is — unify as the type. `[:Pos] unify [:Pos]`
admits, the fn-shape row (`((fn [[xs:[:Pos]] [Boolean] [true]]) unify T`)
admits, `[:Pos]` against `[:Neg]` fails, and the membership rows (`[1 2]
unify [:Pos]` true, `[1 -2]` false, `5 unify Pos` true) are unchanged.
The `unify` word itself is still uncompilable ("unannotated or opaque
word"), so the compiled lane answers by fallback. Pinned by lang
`TestSamePredicateReferencesUnify` and fnpred.tsv's closing section.
**Found:** threading the registry through the kernel's unify calls in
#471 (`core/go/unify.go` — the registry a `UnifyExplainR` is armed with
once sat on a package-global stack that every goroutine's unify read;
the parallel corpus walks were the first to race on it); the review's
differential between the old and the new binary.

**Rule:** a value unifies with its own denotation; two references to
one type are the same type.

**Divergence.** Armed with a registry, as the `unify` word and a typed
def are:

```
def Pos fnpred n:Integer [n gt 0]
def T fnsig [[xs:[:Pos]] [Boolean]]
((fn [[xs:[:Pos]] [Boolean] [true]]) unify T)     ~unify-fail
```

Unarmed (`core.Unify` with no registry, the exported
`FnSigSatisfiesSpec`) the same pair admits: two `[:Pos]` patterns settle
structurally. The armed pre-pass (`unifyInner`'s predicate-constraint
arm, `resolvePredicateRef`'s Atom case) resolves the atom `Pos` inside
one pattern to its predicate and runs that predicate against the OTHER
pattern's atom `Pos`, which is not a positive integer — a membership
test where a type comparison was meant. HEAD's verdict, kept verbatim by
the threading (the review's lens was "identical semantics"), pinned as
such by `TestUnifyThreadedFnShapePatternKeepsRegistry`; the fix is one
arm in the pre-pass (an atom against an atom naming the same predicate
is the type, not a candidate) and belongs with the checker-precision
programme.

**What the same change closed, deliberately.** Before it, a predicate
BODY that dispatched a fn whose parameter is a predicate-typed container
(`def chk fn [[xs:[:Pos]] [Boolean] [true]]  def Wrap fnpred xs:List
[(chk xs)]  [1 2] is Wrap`) matched — but only because the dispatcher's
registry-less `Unify` read whatever registry-armed unify happened to be
in flight, on ANY goroutine: the same call at top level, `(chk [1 2])`,
raised `signature_error` on both engines, and under concurrency the
in-body answer depended on another goroutine's timing. Engine re-entry
now starts unarmed, exactly like top level (`[1 2] is Wrap` is `false`,
`([1 2] unify Wrap)` is `~unify-fail false`, `{a:5} is Wrap` `false`),
on both engines with parity, pinned by
`TestPredicateBodyDispatchIsUnarmedLikeTopLevel` (lang/go). One rule
where there were two.

## NUR158 — a capturing closure at a dispatch-modifier word's poly seat raises `illegal_ref` compiled where the interpreter wraps it {#nur158}

**Status:** FIXED 2026-09-25 (the closure at the wrap — the handoff log's
entry of that date). Recorded 2026-09-18.

**The fix.** The natives' payload assertions (`rebarrierResult` for
`stack-args` / `forward-args`, `forceArityHandler`, `usurpHandler`) bridge a
`ClosurePayload` to the fn definition whose dispatch runs the closure
(`ClosureAsFnDef`, the value-path bridge the seams already use) before
asserting, so the wrapper stores that definition as it stores any fn and
the later re-dispatch runs the closure. `m.a/u 10 3` is 93, `10 3 m.a/s`
93, `m.a/f 10 3` and `m.a/2 10 3` 107, on both lanes; the word forms the
same. Pinned by lang `TestWrapWordsBridgeACompiledClosure` and fn-value.tsv
§16.
**Found:** the handler-migration line's fn-operand pilot
([design/HANDLER-MIGRATION-LINE.0.md](design/HANDLER-MIGRATION-LINE.0.md)
§Progress), probing the four dispatch-modifier words' VALUE forms with
`RunInterp` against `RunCompiled` before declaring them.

**Rule:** one fn value, one dispatch: a `Function` the interpreter wraps
(`usurp` / `stack-args` / `forward-args` / `force-arity`) the compiled
lane wraps too. The value-form sigs declare
`CompileStoresFn | CompileFnHandlerStrict`: the native validates its
operand as an `FnDefInfo` (`core.UsurpFunction` / `rebarrierFunction` /
`ForceArityFunction`'s payload assertion) and stores the original in the
wrapper for the later re-dispatch — the shape `CompileFnHandlerStrict`
exists for, and which the recorder's `RecordCallOperands` refuses ("a
CAPTURING handler at a STRICT store slot … the native rejects").

**Divergence.** These words run in check mode, so their only recorder
seat is the GRADUAL poly record (`recordGradualWrap` →
`RecordPolyCall`), and that seat reads no declaration: the wrap lowers to
`OpCallNativePoly`, the VM's `callPoly` hands the native the live value,
and a compiled closure is a `ClosurePayload`, not an `FnDefInfo`:

```
def mk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a sub b) add k])]]
def mk2 fn [[][Map][{a:(mk 100)}]]
def m (mk2)
m.a/u 10 3            interpreted 93; compiled illegal_ref: usurp requires a function value, got Function
usurp (m.a) 10 3      the word form, the same
```

A capture-free returned lambda and a returned named fn are delivered as
`FnDefInfo` values and wrap correctly on both lanes; only the CAPTURING
closure diverges.

**What the pilot closed.** The TYPED-carrier form — a user fn's declared
`Function` result wrapped directly (`def r (usurp (mk 100))  r 10 3`,
`usurp (mk 100) 10 3`, and the `stack-args` / `forward-args` /
`force-arity` twins) — answered `illegal_ref` against the interpreter's
`93` / `107` before 2026-09-18. `recordGradualWrap` now honours the
declaration: at a `CompileFnHandlerStrict` word a typed (non-dynamic)
`Function` carrier declines the poly record, so the residual REFUSES
("… of unknown provenance") and the fallback agrees with the
interpreter, value and taxonomy (`TestModifierOverReturnedClosureDoesNotCompileWithParity`,
lang/go). The decline is deliberately narrow: the DYNAMIC `Function`
carrier a sibling modifier's gradual wrap produced (a composed chain,
path-modifier.tsv:52-55 — the inner poly's runtime result is always the
constructor's wrapper `FnDefInfo`) and the dynamic-Any `m.a` read
(path-modifier.tsv:17-18, 93-94) keep their poly record, because
declining them regresses eight corpus rows that are correct today.

**What stays open — the `m.a` form above.** A dynamic-Any carrier is a
closure at run time whenever the map was built from a computed closure,
and the check pass cannot see that through the read. The fix belongs at
the poly seat, not in the word: either the recorder's `RecordPolyCall`
reads `CompileFnHandlerStrict` off the word's sigs and refuses a gradual
Function-bearing operand there (the recorder honouring the declaration
it already honours on the mono path), or the VM's `callPoly` bridges /
defers a `ClosurePayload` arriving at a strict native (the runtime
honouring it, counted by the defer census). Both are compiler/VM work —
the S1 fn-value line's "one convention" (FULL-COMPILATION.0.md §10.1),
not the handler line's.

## NUR159 — a named fn value in a branch position is applied by the interpreter and pushed by the compiled lane {#nur159}

**Status:** FIXED 2026-09-23 (the branch result's re-step — the handoff
log's entry of that date). Recorded 2026-09-18.

**The fix, in one paragraph.** The merge widens a fn arm's type to its
lattice parent (Word), so no static fn test saw the branch's value; the
recorder's branch event already knew (`eventFlags.mayBeFn`, set when an
arm is a fn value), and the fact is now a seam every collapse-side and
residual-side gate asks — `EmitRecorder.MayBeFn`, with a second flag,
`mayBeFnArgs`, for a fn arm that may TAKE ARGUMENTS (a named fn with a
parameterised overload, a carrier). The check pass's re-step landing note
admits a may-be-fn value (`noteReStepLanding`), the lowering lands the
merged value right after the merge (`emitBranchLanding`: a named 0-arg fn
fires, a 0-arg lambda parks, an arg-taking fn stands aside — so `if true
one/v [2]` is 1 on both lanes, in a paren, a def, a fn body and a code
body alike), and the residual's trailing, trailing-window and mixed arms
read an UNSETTLED value (arg-taking, neither paren-placed nor `/v`-read)
as fn-like, so `7 if true inc/v [2]` is 8 and `1 7 if true inc/v [2]` is
`[1 8]`. The collapse's park and re-step marks ask the same seam, so `7
(if true inc/v [2])` is the placed pair on both lanes and `(7 (if true
inc/v [2]))` is 8. What no arm seats declines through existing sites: a
later dispatch that collected the branch's value (the poly record), an
arg-taking value interior to the residual past a boundary, a list
literal's element with siblings, a fn body's residual over a param, and a
def bound to an arg-taking result (the interpreter installs the fn under
the name and dispatches the name as a word — the read poisons the
placement gate). A factory's closure in the arm is classified by its
recovered shape (`branchArmMayTakeArgs`: a 0-arg lambda parks, so `7 if
true (mk) [2]` is the pair on both lanes), and the VM's trailing apply
islands its window as written so an unrecoverable shape is decided at run
time (`7 if true (mk true) [2]` is `[7 fn]`, its 1-arg twin 8). Pinned in
`branch_fn_value_test.go` (lang/go: fifty-one parity rows, nineteen sound
declines); the sweep pin is retired and the `if` × named-fn cell PASSES.
The original record follows.
**Found:** the generated sweep of
[design/FULL-COMPILATION-REPLAN.0.md](design/FULL-COMPILATION-REPLAN.0.md)
S0 (`test/go/sweep`; the matrix is `test/go/langspec/SWEEP_STATUS.md`),
cell `if` × named-fn, on the sweep's first run.

**Rule:** one value, one meaning — what a fn value in a branch position
means is decided by the interpreter, and the compiled lane answers the
same.

**Divergence.**

```
def one fn [[][Integer][1]] end if true one/v [2]     interpreted 1; compiled fn one
```

The interpreter lands the 0-arg fn value and fires it (the arity-0
landing NUR123 and NUR124 describe for bare reads); the compiled lane
pushes the value as data. The other kinds of the same cell: the factory
form `def mk fn [[][Function][([] => [1])]] end if true (mk) [2]` and the
module-export form `if true M.one [2]` agree on both lanes; the lambda
form `if true ([] => [1]) ([] => [2])` fails to compile ("unconsumed
fn-value carrier in residual"), the container form `if true m.f [2]`
fails to compile ("0-arg landing not modelable at fn value"). A compile
failure is honest; the named form is the one that answers wrong.

**Where it belongs:** S1's fn-value convention (FULL-COMPILATION.0.md
§10.1) — the branch-position landing is one more seat where a fn value
must obtain a unit or fail to compile, never pass through as data.
Pinned in `sweepKnownMiscompiles` until then (retired with the fix).

## NUR160 — the apply of a factory-built fn value does not fire compiled under a dirty stack {#nur160}

**Status:** FIXED 2026-09-23 (the typed callback's contract — the handoff
log's entry of that date). Recorded 2026-09-18.

**The fix, in one paragraph.** A factory whose body is a capture-free
lambda — or a named fn's value — returns a baked fn CONST, not a closure,
and the `apply` word's pending-application arm (`recordCallElided`)
admitted only a produced closure (`producedFnValue`) or, inside a fn unit,
a produced carrier; at the program the factory's declared `[Function]`
return typed the result as a carrier with no pending entry, and the
paren-shaped residual arms PARKED it under a value beneath (the clean-stack
form went through the 2-entry trailing arm, which applies). The arm admits
a produced fn const now (`producedConstLambda`), so the program's pending
apply lowers as `OpCallDynApplyTop` — applyHandler's own semantics, a
named fn re-stepping through its word — and `programPendingApplyTop`
takes the fn ALONE as well (an empty window: a 0-arg closure fires, a
wider one finds no match and stays data), which closed two silent
neighbours on main: `def mk fn [[][Function][([] => [1])]] end (mk) apply`
compiled `fn` for 1, and `5 (mk) apply` `[fn 5]` for `[5 1]`. Three
sound-compile-failure rows of the empty-window shape (`(kk 7) apply`,
`(mk 1)/v apply`, `def p (kk 7) end p/v apply`) compile with parity and
moved to `dirty_stack_apply_test.go` (lang/go), which pins the family; the
sweep pin is retired. The module-fn factory twin raises on the interpreter
inside the factory itself — NUR186. The original record follows.
**Found:** the generated sweep, the `prefix-stack` call form of the
`apply` × factory cell.

**Rule:** a value below the operands on the stack changes nothing about
an apply — the interpreter's answer is the same with or without it.

**Divergence.**

```
def mk fn [[][Function][([n:Integer] => [n add 1])]] end 5 (mk) apply      both lanes 6
7 def mk fn [[][Function][([n:Integer] => [n add 1])]] end 5 (mk) apply    interpreted [7 6]; compiled [7 5 fn (Integer)]
```

With `7` beneath, the compiled program leaves `5` and the unapplied fn
where the interpreter applies it. The same seed under every other call
form of the sweep (a fn body, a lambda body, `do`, the branch arms, a
loop body, a module body, a def before or after, a splice) compiles with
parity, so the dirty stack is the trigger, not the factory: the lowering
that seats the apply's operands reads a stack position the extra value
shifts. NUR156 was the module-export sibling (the apply of `M.inc/v` never
fired, clean stack or not — a quoted lead the check model kept quoted;
FIXED 2026-09-22).

**Where it belongs:** S1 (the Apply kernel's first branch); pinned in
`sweepKnownMiscompiles` until then.

## NUR161 — an afn over a container-held fn value applies it compiled, under a dirty stack, where the interpreter returns it {#nur161}

**Status:** RESOLVED 2026-09-22 — the divergence is gone; the row is a
LOUD compile failure ("residual shape beyond Stage 1 (call result above a
literal)"), the residual layout's standing decline for a call result over
a value beneath it. It closed as a side effect of NUR177's fix: the
compiled lane applied the container fn because the afn's memoised body
residual carried the CONTAINER READ's own value identity out to the
program residual, and the residual lowering re-stepped that identity as
the read it had been inside the body; each call's result now has its own
identity, and the residual lowering sees a call result it cannot lay out
under a literal, and says so. The `sweepKnownMiscompiles` pin is retired.
**Found:** the generated sweep, the `prefix-stack` call form of the
`afn` × container cell (recorded 2026-09-18).

**Rule:** as NUR160 — a value below the operands changes nothing.

**Divergence.**

```
def m {f: ([n:Integer] => [n add 1])} end def f ([x:Integer] afn m.f) end f 5      both lanes fn (Integer)
7 def m {f: ([n:Integer] => [n add 1])} end def f ([x:Integer] afn m.f) end f 5    interpreted [7 fn (Integer)]; compiled [8]
```

The afn's body is the fn value `m.f`, so `f 5` returns it unapplied on
both lanes. With `7` beneath, the compiled lane APPLIES the container fn
to the `7` — 8 — and the returned value is gone. The opposite direction
from NUR160: there the compiled lane under-applies, here it over-applies,
and both only under a dirty stack; the two read as one seating defect
seen from both sides.

**Where it belongs:** S1; pinned in `sweepKnownMiscompiles` until then.

## NUR162 — the compiler panics disassembling a `word` whose body is a fn value, inside a paren group or a module body {#nur162}

**Status:** FIXED 2026-09-25 (the 0-arg apply at a paren's tail — the
handoff log's entry of that date). Recorded 2026-09-18.

**The fix.** The entry without a signature came from the trailing
fn-value apply (`RecordDynApply`): its window trim keeps the callee's own
arity of arguments, and a 0-ARG callee — the lambda `word` splices at the
paren's tail — trimmed the window to NOTHING, so the record carried
`dynApply: 0` and the lowering, which routes an apply by `dynApply > 0`,
emitted it as a plain native call under a `SigRef` with no signature. The
disassembler dereferenced it, and so did the VM (an internal_error at run
time). The interpreter's own answer is that the value is DATA at the tail
(`5 fn`, ADR-016's gate parks a 0-arg lambda value), and the trim now
declines a 0-arg callee at the recorder's ONE failure site (the census is a
downward ratchet) — the program falls back and answers `5 fn`; the lowering
keeps a belt ("fn-value apply over no argument"), `RecordDynApplyName`
declines the same window up front, and the disassembler names a
signature-less entry instead of crashing. The sweep's two crashing
variants (`word` × lambda under paren-group and module-body) decline
loudly now: its crash ceiling 2 → 0, its decline ceiling 171 → 173.
Pinned by lang `TestZeroArgTailApplyNeverCrashes`.
**Found:** the generated sweep, the `paren-group` and `module-body` call
forms of the `word` × lambda cell. The sweep's first run went down with
it — a panic in one classification took the whole test binary — so
`vary.Classify` now recovers a panic into `vary.Panicked`, named by the
phase it fired in, and the sweep's crash ceiling holds seeds at 0.

**Rule:** a program the interpreter answers is a program the compiler
answers, compiles, or fails to compile with a reason — never one it
crashes on.

**Divergence.**

```
def dbl word ([] => [1]) end 5 dbl          interpreted 5 fn; compiled 5 fn (parity)
(def dbl word ([] => [1]) end 5 dbl)        interpreted 5 fn; the compiler PANICS in Disassemble
import module [ def zzvmod fn [[] [] [def dbl word ([] => [1]) end 5 dbl]] export "ZZV" {run: zzvmod/v} ] end ZZV.run
                                            the same
```

`Program.disasmUnit`'s `OpCallNative` arm reads
`p.Sigs[in.Arg].Sig.TotalArgs()` and the entry's `Sig` is nil
(`compiler/go/bytecode.go:1578`): the recorder emitted a native call
whose signature entry carries no signature. Whether the VM survives
running the program is not measured — the classifier disassembles
before it runs, and the panic stops it there. The interpreter's own
answer — a `word` bound to a fn VALUE splices the value unapplied,
where a `word` bound to a named fn REFERENCE (`def dbl word inc/v end 5
dbl` → 6) splices a call — is a non-uniformity worth its own look; the
crash is what this record is for.

**Where it belongs:** the recorder (which entry is emitted without a
signature, and why) and, defensively, the disassembler; until then the
sweep's call-form ceiling names the two variants.

## NUR186 — a module fn's value at a factory body's tail raises interpreted and returns compiled {#nur186}

**Status:** FIXED 2026-09-23 (the named fn value's candidates — the handoff
log's entry of that date). Recorded the same day, probing NUR160's
neighbours; present on `main` (a worktree at 3fa1212), silent, default
lane, exit 0.

**The fix, in one paragraph.** The interpreter's re-step of a NAMED fn
value (execFnDefLiteral) raises when its candidates — the values beneath
it in the frame and the tokens after it, a fn frame's tail markers among
them — are non-empty and none matches; with no candidate the value is
data. The compiled landing (OpReStepLanding, NUR173) stood aside as data
wherever no zero-argument overload existed. Two halves. The DYNAMIC half
(a member read, `def m {f: M.inc} def g fn [[][Any][m.f]] end (g)`): the
check pass's landing note now carries what follows the value
(`NoteLandingNext`: a FUNCTION word the re-step's forward phase stops at,
a word bound to a VALUE the phase collects — `m.f k` with `def k 2` is 3
— a boundary, the tape's end) and whether values sit beneath it in its
frame (the interpreter's own `EffectiveResolved`), the lowering hands the
landing op a CANDIDATE flag — a function word after the value, or the
tape's end inside a unit whose frame carries the tail markers (a user fn
or a lambda value's body, `frameTail`; a code-body closure has none, so
`do [m.f]` stays data) — and the VM's landing raises
the interpreter's `uncalled_function` for a named fn with no zero-argument
overload over a candidate and an empty frame (`reStepLanding`,
`uncalledFunctionError`); with values beneath it stands aside and the
residual arms apply or raise as before. The CONCRETE half (`def mk fn
[[][Function][M.inc]] end (mk)`, the original witness): the check pass's
body tape carries no tail markers, so it saw the module export as data and
the unit baked it; a concrete named fn with no zero-argument overload left
in a fn or lambda frame's residual with no replay to re-step it declines
the unit (`namedFnUnmatchedAtTail` — a `/v` read, a quoted value and a
paren-placed one are data on both lanes), and dispatch WRECKAGE the pass
did mark (`FailedDispatch`: `[M.inc typeof]`, a word after the value
inside the body) declines at the word's operand record
(`RecordCallOperands`, `polyCallDeclineReason`). A unit-level trap that
raises the identical error where the unit now declines is the follow-on.
Pinned in `named_fn_candidates_test.go` (lang/go: forty-eight parity rows,
twelve rows raising alike, sixteen sound declines); the pending pin
`TestModuleFnAtFactoryTailPending` is retired. Found on the way and closed
in the same entry: NUR188 (a poly collecting a member read the interpreter
re-steps first), NUR189 (the trailing arms applying a paren-placed member),
and NUR187's two other halves (the whole-frame replay re-stepping `[M.inc
; 5]` across the `;`; a WORD after a value ending its collection — `7 m.f
three` is `[8 3]`, islanded to `[7 4]`). The original record follows.

**Rule:** a compiled program answers as the interpreter does.

| witness (`M.inc` adds 1) | interpreted | compiled |
|---|---|---|
| `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end def mk fn [[][Function][M.inc]] end 5 (mk) apply` | `uncalled_function: call to 'inc' matched no signature` | `[6]` |
| the same, `7 5 (mk) apply` | the same raise | `[7 6]` |
| `def inc fn [[n:Integer][Integer][n add 1]] end def mk fn [[][Function][inc/v]] end 5 (mk) apply` | `[6]` | `[6]` |

**The defect, in one sentence.** The reach group `M.inc` at the factory
body's tail collapses to a lone NAMED fn value, and the interpreter's
landing dispatches it — a name always calls (ADR-011) — over nothing,
raising the no-match, where the compiled unit returns the member's value
and the program's `apply` then applies it. The `/v` twin is inert on both
lanes, so the divergence is the landing's, not the apply's: NUR173/NUR175's
family (a lone survivor's re-step at a body tail), for a NAMED member with
no 0-arg overload, where the landing's screen leaves the value as data.

**Where it belongs.** The re-step landing (OpReStepLanding, NUR173) and its
screens (NUR175). Pinned pending in `TestModuleFnAtFactoryTailPending`
(lang/go, dirty_stack_apply_test.go) until the fix retired it.

## NUR187 — the residual's fn-value apply arms cross a statement boundary {#nur187}

**Status:** FIXED 2026-09-23 (the branch result's re-step — the handoff
log's entry of that date). Recorded the same day, probing NUR159's
neighbours. Present on `main`, silent, default lane, exit 0.

**Rule:** a statement boundary ends a value's collection on both lanes.

| witness (`inc` adds 1, `m` is `{f: inc/v}`) | interpreted | compiled |
|---|---|---|
| `7 m.f ; 3` | `[8 3]` | `[7 4]` |
| `1 7 m.f ; 3` | `[1 8 3]` | `[1 7 4]` |
| `m.f ; 5` | `[fn inc(Integer) 5]` | `[6]` |
| `def one fn [[][Integer][1]] end if true one/v [2] ; 3` | `[1 3]` | `[1 3]` |

**The defect, in one sentence.** The program residual is a flat list of
values, and the apply arms that classify it — the leading dynamic and
carrier arms (`OpCallDynamic` over the entries above the lead) and the
mixed island (`OpCallDynamicMixed`, the whole window re-stepped verbatim)
— had no notion of the `;` the interpreter stopped at: the member read's
re-step took the 7 beneath it and left the 3 to the next statement, where
the island laid `[7 fn 3]` out as one window and the fn collected the 3
forward; with nothing beneath, the lead arm applied the member to the
next statement's 5.

**The fix.** The pass notes every statement boundary's position as it
steps it (`stepEnd` → `EmitRecorder.NoteStatementEnd`, the compiler's
`stmtEnds`), and `crossesBoundary` orders a value's producing event (or
its token) against the entries above it: an entry written past a boundary
that follows the value is a later statement's, and no arm applies the
value over it — the lead arms stand aside (the value is data beneath the
later entries, as the interpreter left it: `m.f ; 5` is the pair on both
lanes), the mixed island declines (an interior value past a boundary has
no layout here yet, so `7 m.f ; 3` is a loud compile failure — "dynamic
value precedes residual args"). Only a PROVEN crossing counts: both
positions known and a recorded boundary strictly between them; a
def-bound read's position is the name's, which is not recorded, so it
proves nothing (`def fs (FnUtil.flip sub/v) end (fs 3 10)` keeps its
arm). A branch record whose condition token carries no position (`if
true …`) takes the dispatching word's (`branchRecordPos`), or the branch
family would have crossed unseen. A newline is not a boundary (`7 m.f`
then `3` on the next line is `[7 4]` on both lanes). Pinned in
`branch_fn_value_test.go` (lang/go), the member-read rows.

**Two more halves, the same day** (the named fn value's candidates — the
handoff log's entry of that date). Inside a fn body the whole-frame
replay (`noteDynFrameReplay`, NUR123's machinery) re-stepped the frame's
residual as a flat run of values, so `def mk fn [[][Any][M.inc ; 5]] end
(mk)` re-stepped `fn 5` to 6 where the interpreter's re-step stopped at
the `;` and raised the frame's count error over `[fn 5]`; the replay
declines a proven crossing. And a FUNCTION WORD right after a value ends
its collection as a boundary does — the re-step's forward phase stops at
a word that dispatches (a registered word, a fn binding), so every
residual entry above the value was pushed after it, by the word or later:
`7 m.f three` is `[8 3]` interpreted (the fn over the 7 beneath, three's
result above) and the mixed island answered `[7 4]`. A word bound to a
VALUE is no boundary: the phase collects it exactly as the island's flat
re-step does (`7 m.f k` with `def k 2` is `[7 3]` on both lanes), and a
draft that read every word as one raised `m.f k` where the interpreter
answers 3 and declined a capturing lambda's `[kv.v k]` — caught by the
unit-suite ledger before it landed. The landing note's what-follows fact
(`NoteLandingNext`, NUR186) is the signal: with a function word next,
`crossesBoundary` holds for every later entry, positions or no
positions. The note has the landing's own gates: a
VARIADIC result is not noted (its outputs are a region, and a catch
region's paths deliver different tails — `def x (do [(1 add 2) "a" "b"]
error [dot code]) x` re-steps the do's first result under its sibling
results on the success path and under `x` on the raise path, and the
raise path's note rerouted the S9 promotion's own diagnosis), a def-bound
read is excluded (written where the name is), and a value re-stepped
more than once (a paren's collapse re-steps what its group parked)
merges its notes with a word first. Pinned in
`named_fn_candidates_test.go`.

## NUR188 — a native poly collected a container member's fn value the interpreter re-steps first {#nur188}

**Status:** FIXED 2026-09-23 (the named fn value's candidates — the handoff
log's entry of that date). Recorded the same day, probing NUR186's
neighbours. Present on `main`, silent, default lane, exit 0.

**Rule:** a member read dispatches at its own token (ADR-011).

| witness (`inc` adds 1, `m` is `{f: M.inc}`) | interpreted | compiled |
|---|---|---|
| `7 m.f typeof` | `Integer` | `[7 Function]` |
| `m.f typeof` | `uncalled_function: call to 'inc' matched no signature` | `Function` |
| `"s" m.f typeof` | the same raise | `[s Function]` |
| `typeof m.f` | `Function` | `Function` |

**The defect, in one sentence.** The check pass holds a DYNAMIC carrier for
`m.f`, so `typeof` records as a poly over it, and the compiled window hands
`typeof` the member's fn value — where the interpreter re-steps the member
at its own token first: over the 7 beneath (8, then Integer), or raising
with the word as its only candidate.

**The fix.** `polyCallDeclineReason` declines a member-read operand
(`MemberFnReadValue`) written BEFORE the word — its producing event's
position against the word's — unless it is quoted, `/v`-read, owned by a
pending `apply`, or PLACED by a user paren no enclosing paren re-stepped
(`placedNotReStepped`: `(m dot a) eq (m dot a)` in compare-restrict.tsv
is data the word collects, and a draft without that exclusion put the row
back on the corpus ledger); a read written after the word (`typeof m.f`)
is the word's collected operand and stays. The dynamic landing's candidate raise
(NUR186) answers the same rows the interpreter raises on when nothing
collects the value. A faithful model — the member applied over the values
beneath, then the word — is the follow-on.

## NUR210 — a module fn's named fn value, read through its reach group, re-steps on the interpreter only {#nur210}

**Status:** FIXED 2026-09-25 (the reach group's survivor — the handoff log's entry of that date; recorded 2026-09-25, found probing NUR191's
neighbours — the handoff log's "the module fn's return contract" entry).
Present on main before that change (a worktree at ae92338), silent,
default lane, exit 0.

**Rule:** a compiled program answers as the interpreter does — a fn value
a call leaves is applied, or seated as data, on both lanes alike.

**Divergence.**

```
import module [def ff fn [[][Function][inc/v]] def inc fn [[n:Integer][Integer][n add 1]] export "M" {ff: ff/v}] end 5 M.ff
  interpreted   [6]
  compiled      [5 fn inc(Integer)]
… end M.ff 5
  interpreted   [6]
  compiled      [fn inc(Integer) 5]
… end 5 (M.ff)
  interpreted   [5 fn inc(Integer)]
  compiled      the same
def inc … def ff fn [[][Function][inc/v]] end 5 ff
  interpreted   [5 fn inc(Integer)]
  compiled      the same
```

`M.ff` is the reach group `( M dot ff )`: the 0-arg wrapper fires inside
it and leaves the named fn value `fn inc` as the group's lone survivor,
and a reach-lowered group never parks — its collapse re-steps the
survivor (fnReturnPark's one exclusion; a name always calls, ADR-011),
which dispatches over the 5 beneath. A user paren around the same read
(`5 (M.ff)`) parks, and so does the main registry's frame return (`5
ff`); the compiled lane seats the returned value as data in every
spelling. NUR173's guarded re-step landing covers a reach group's member
fn value; the fn value a module fn RETURNS through the group is not
landed.

**Traced (2026-09-25, closing NUR212).** Under the check pass the 0-arg
wrapper fires inside the reach group and the group collapses to a
Function CARRIER (`5 Function` at the enclosing position — the module
fn's declared `[Function]` return, not the concrete `inc` the interpreter
holds), and no re-step landing is noted for it: the compiled stream is
`CALL_USER ff/0; STORE_LOCAL; PUSH_CONST 5; PUSH_LOCAL` with no
`OpReStepLanding`, so the value seats as data. The landing rung
(method_shape.go) admits a fn-typed carrier and the tape's end, so the
note is lost between `NoteReStepLanding` and the lowering — the
carrier's identity across the module boundary (the event's out vs the
re-minted return) is the likely seam. Still pending.

**Fence.** `TestModuleFnNamedValueThroughReachPending` (lang
module_fn_return_contract_test.go) pins both lanes as they stand, so
closing it is loud.

**The fix (2026-09-25).** The reach group's survivor rule, on the compiled
lane: the collapse records a fn-typed CARRIER it leaves as the group's
lone survivor (`CheckState.ReachSurvivorFnIDs`, beside the concrete named
value's `ReachGroup` tag), and the residual lowering's placed-call gate
(`callResultPlaced`) exempts it — a reach group never parks, so the call's
result is not placed data, and the trailing and leading apply arms lay it
out as the interpreter re-steps it: `5 M.ff` and `M.ff 5` are 6 on both
lanes. An ENCLOSING user paren's placement wins (`5 (M.ff)` stays `[5 fn
inc]` everywhere). The fence became the pin
(`TestModuleFnNamedValueThroughReachApplies`).

## NUR209 — a single-literal `do` body under a loop baked as a const on the compiled lane {#nur209}

**Status:** FIXED 2026-09-25 (the loop region's residual — the handoff
log's entry of that date). RECORDED the same day, found probing NUR197's
neighbours; present on main before that change (a worktree at ae92338),
silent, default lane, exit 0.

**Rule:** a compiled program answers as the interpreter does — a `do`
body's residual literal reads the loop's binding on both lanes.

**Divergence (as it stood).**

```
for 2 [do [[i]]]
  interpreted   [[0] [1]]
  compiled      [error(undefined word: i) error(undefined word: i)]
for 2 [do [{a:i}]]
  interpreted   [{a:0} {a:1}]
  compiled      [error(undefined word: i) error(undefined word: i)]
for 2 [do [[(i add 1)]] i]
  interpreted   [[1] 0 [2] 1]
  compiled      [error(undefined word: i) 0 error(undefined word: i) 1]
for 2 [do [[i] 5]]
  interpreted   [[0] 5 [1] 5]
  compiled      the same
```

The token body's closure compile passed `bodyInFrame` false, so
`AnalyseFnBody` analysed it as an ANONYMOUS lambda — whose single bare
container literal DEFERS past the frame (ResidualEvalsInFrame's one
exception) — and recorded no assembly for the residual; the closure
declined on the unknown provenance, and the dyn-body backstop
(`tryRecordDynBody`) baked the body `[[i]]` as a const (a word inside a
nested compound is an inert const MEMBER) and lowered a plain
`CALL_NATIVE do`, whose handler re-ran the literal through the
interpreter's sub-engine — where the loop's `i` is a frame slot the
registry never held. A multi-token body evaluates in-frame under either
flag (BodyEvalsResidual), which is why `[[i] 5]` compiled to a closure
assembling the list from the slot.

**The fix.** A token body compiles in-frame (`recordClosureDispatch`'s
`bodyInFrame` true at the token-body site): the InvokeBody seam's
sub-engine sweeps a pending residual at its end with the body's inputs
and bindings live — never deferred — so the interpreter's own predicate
for a body that is not an anonymous lambda applies, the residual records
its assembly, and the closure unit assembles the list from the captured
slot (`PUSH_CLOSURE do$body … PUSH_LOCAL; MAKE_LIST`). The lambda-value
sites keep `!fd.Anonymous`.

**Fence.** `TestLoopBodyResidualLiteralResolves` (lang
map_literal_flex_member_test.go): the shapes as parity rows and the
lowering (a closure assembling the list, no `word(i)` const); control.tsv
§3's `for 2 [do [[i]]]` row.

## NUR208 — a loop's iterator survives a trapped raise on the interpreter, not on the VM {#nur208}

**Status:** FIXED 2026-09-25 (the loop region's residual — the handoff
log's entry of that date). RECORDED the same day, found probing NUR197's
neighbours; present on main before that change (a worktree at ae92338),
silent, default lane, exit 0. NUR201's loop twin.

**Rule:** a compiled program answers as the interpreter does — the
bindings a raise leaves behind are the same on both lanes.

**Divergence (as it stood).**

```
def i 9 do [for 2 [raise 'x']] i
  interpreted   [error(x) 0]
  compiled      [error(x) 9]
def i 9 do [for 3 [if (i eq 1) [raise 'x'] []]] i
  interpreted   [error(x) 1]
  compiled      [error(x) 9]
do [for 2 [raise 'x']] i
  interpreted   [error(x) 0]
  compiled      does not compile (the check pass's undefined_word: i)
```

The interpreter's `for` installs its iterator as a registry def
(`InstallDef`) and uninstalls it when the loop finishes or a break
discards the region (handleLoopBreak); a raise abandoned the tape with
the loop region still open and the iterator installed, shadowing the
enclosing binding after the trap. The compiled loop keeps `i` in a frame
slot (`FOR_SETUP`), so nothing outlives the abandoned run.

**The fix.** `Engine.faultReturn` unwinds every live loop's iterator
(`unwindLiveLoops`: a `for` continuation whose mark has stepped and whose
move the pointer has not reached) before it unwinds the frames — a loop
inside a live frame installed its iterator after the frame's entry
snapshot, so the frame's truncation has nothing left to pop for the name,
and a loop enclosing a live frame keeps its iterator beneath that
snapshot, where only the loop walk reaches it. Both lanes read 9; with no
outer binding both raise undefined_word (the compiled lane at the check
pass); a while loop installs no iterator of its own, and its body's def
leaks by design on both lanes.

**Fence.** `TestLoopIteratorTornDownOnTrappedRaise` (lang
map_literal_flex_member_test.go): the shapes above, nested loops, the
loop inside and around a fn frame, a callback raising inside the loop, a
range loop's own iterator name, the `error` handler, the while twin; and
control.tsv §3's row.

## NUR229 — the two parser ports report a malformed quoted-string escape differently {#nur229}

**Status:** FIXED 2026-09-26 (one escape vocabulary, one malformed-escape
report — the handoff log's entry of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** closing NUR026, probing the malformed-escape matrix.

**Rule:** NUR060's — one language contract, two parsers, every source
rendered identically.

**Divergence** (pre-existing, outside the corpus):

```
"a\x4"      go: invalid ascii escape: "a\x4"    ts: this string is never closed: "a\x4"
"a\u12"     go: invalid unicode escape: "a\u    ts: this string is never closed: "a\u12"
"a\xZZb"    go: invalid ascii escape: "a\xZZ   ts: invalid ascii escape: \xZZ
```

Both come from the two tabnas ports' string lexers (the Go one spans from
the opening quote; the TS one spans the escape and bounds-checks a short
escape differently).

**The fix.** boru's `string_escape` matcher (both ports, NUR026) refuses a
malformed escape before either library lexer reads the string, with the
escape itself as the reported source: all three rows now read
`invalid … escape: \x4` / `\u12` / `\xZZ` in both ports. Pinned by
NUR026's `parse.tsv` rows.

## NUR230 — a surrogate pair in a template: two replacement characters in Go, one code point in TS {#nur230}

**Status:** FIXED 2026-09-26 (one escape vocabulary, one malformed-escape
report — the handoff log's entry of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** closing NUR026, probing the escape matrix.

**Rule:** one escape vocabulary, two parsers.

**Divergence:** `` `\ud83d\ude00` `` read as `'��'` in Go and `'😀'` in TS:
Go's `writeStringEscape` decoded each `\uXXXX` as a rune, and a lone
surrogate becomes U+FFFD; TS's UTF-16 strings join the two code units. In a
quoted string both ports already read one code point (jsonic's Go lexer
pairs explicitly).

**The fix.** `writeStringEscape` pairs a high surrogate followed by a
`\u` low surrogate into one code point, as jsonic does; a lone surrogate
is still U+FFFD. Pinned by NUR026's `parse.tsv` rows and the direct
`processTemplateEscapes` cases.

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

## NUR232 — an inline refinement return refused an abstract residual at check time; the named twin defers it {#nur232}

**Status:** FIXED 2026-09-26 (Bytes a refinement base, a computed bound the run's — the handoff log's entry of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** closing NUR009, pinning NUR231's check-clean half.

**Rule:** one return contract, one check, whatever the type's spelling.

**Divergence** (pre-existing):

```
def g fn [[n:Integer] [(Integer gt 3)] [n]] g 5          boru check: type_error "return value 1: expected (Integer gt 3), got Integer"
                                                         both lanes: [5]
def Big (Integer gt 3) def g fn [[n:Integer] [Big] [n]] g 5   boru check: clean
```

The inline spelling rides the fn's return PATTERN, whose check Unify'd the
refinement against the abstract residual — which never unifies. The named
spelling rides the return TYPE, whose check defers an abstract residual to
the RET and decides only a compile-time-known scalar.

**The fix.** The pattern check defers a failed Unify when the pattern is
(or a union carries) a refinement and the residual is abstract and not
provably outside its base (`refinementUndecided`, check_fnbody.go) — the
named path's rule. A residual provably outside (`[n:String]`) and a failing
constant (`[2]`) are still flagged. Pinned by lang's
`TestNUR232InlineRefinementReturnDefers`.

## NUR233 — a make field's refusal: a bare error interpreted, a compiler defect compiled {#nur233}

**Status:** FIXED 2026-09-26 (a make field's refusal is a type_error — the
handoff log's entry of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** compiling NUR231's type half — a class field typed by a
refinement over a computed bound is the run's to check.

**Rule:** one refusal, one error, on both lanes.

**Divergence** (pre-existing; measured at b3bcd9a):

```
def Big (Integer gt 100) def S class {x:Big} def n 0 for 3 [def n (n add 1)] make S {x:n}
  interp:   make: field "x": expected Big, got Integer (3): value does not satisfy DepScalar bounds
  compiled: [boru/internal_error]: make: field "x": … — "this is a compiler defect: the program
            compiled and then failed inside the compiled runtime; please report it"
```

`make`'s field checks wrapped the refusal in a plain Go error. The
interpreter printed it bare; a compiled run books a plain error as a
compiler defect (`compiledRunError`). The check pass already calls the same
refusal a type_error when it knows the value, so the compile only reached
the run-time raise for a value only the run knows: a loop-carried one, or,
since NUR231's type half, a field typed by a refinement over a computed
bound.

**The fix.** The refusal is a type_error on both lanes (`makeFieldError`,
core_make.go, at all five field-check sites). A refusal that is already
structured keeps its own code beneath the prefix. Five corpus rows the
runtime-defer ledger carried as compiled bails now answer the
interpreter's error (edge-dispatch-3.tsv L59, generics.tsv L56,
module-struct.tsv L100, record.tsv L97/L98; `bailDefectCeiling` 44 → 39).
`make`'s OTHER refusals — an unknown or missing field, a source of the
wrong shape — are the same class and stay plain errors, still ledgered as
bails there. Pinned by core's `TestMakeFieldErrorIsStructured` and lang's
`TestNUR233MakeFieldRefusalIsATypeError`.

## NUR234 — a compiled direct call's contract no-match reports every argument; the interpreter reports its attempted window {#nur234}

**Status:** FIXED 2026-09-26 (the call carries the interpreter's window —
the handoff log's entry of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** compiling NUR231's type half — a call over a parameter
typed by a computed-bound name is admitted by the pass and checked by the
run.

**The fix.** The check pass derives the window at the user fn's dispatch
itself — sigError's own derivation over its tape (`rematchWritten`, a
gradual operand standing where the run's value will), taken at the
dispatch's FIRST step, since the run fails there: its plan sees every
operand but a speculative slot's, and a speculative plan offers no window.
The offer (`NoteCallWindow`) is keyed and held like the region offer, so a
forward collection's force-stack re-step keeps it and the callee's body
analysis cannot overwrite it. The call's record maps each window value to
where the run holds it — an argument by identity, a definite scalar by
value, an event result by its seat (a promoted slot, else its depth
beneath the call's operands on the stack the lowering simulates), a local
read whose binding had not moved when the window was offered — and the
lowering writes `CallWindows[pc]`, which the VM reads when the contract
fails. A value with no such home leaves the call reporting its arguments,
as before: a local read rebound between the read and the call (`a def a 9
f b` reports b's 6 compiled, a's 5 interpreted) is the residue. Pinned:
lang `TestNUR234ContractNoMatchReportsTheAttemptedWindow` (fifteen rows
byte-identical, notes included); compiler `TestNoteCallWindowPool`,
`TestClaimHeldWindow`, `TestCallWindowOps`, `TestSeatCallWindow`; eng
`TestCallWindowAt`.

**Rule:** one failed dispatch, one diagnostic, on both lanes.

**Divergence** (pre-existing; measured at b3bcd9a with a gradual argument):

```
def f fn [[n:String] [Integer] [0]] each ([e:Any] => [f e]) [5]
  interp:   signature_error … note: candidate `f (String)` takes 1 argument, but none were supplied
  compiled: signature_error … note: the argument was 5 (an Integer)
                              note: candidate `f (String)` — argument 1: expected String, got 5 (an Integer)
```

The code, message head and caret agree; the notes differ. The interpreter
reports its ATTEMPTED WINDOW (`attemptedWindowOver`): the forward
candidates written after the word, which stop at the first bare read, and,
when those are fewer than the smallest overload's arity, the stack prefix
beneath. `f e` inside a body, and `def v 2 f v` at the top level, attempt
an empty window. The compiled direct call's contract check
(`checkParamContract` at `OpCallUser` / `OpTailCallUser`) reports every
argument (`RuntimeNoMatch` over `guardArgs`). The spellings whose window
holds the value agree: `e f` (a stack argument), `f (e)` (a paren group)
and `f (g e 1)` (a nested call).

NUR122 closed the same gap for a trailing fn-value apply, by carrying the
written run (`DynApplyHead.NWritten`, from `writtenRun`). **Proposed fix:**
carry each user call's written run and forward/stack split on its record,
and build the contract's no-match over the same window: the written
prefix, filled from the unit's stack beneath the call's operands to the
smallest arity.

The window, measured on the interpreter inside a fn body (`e` a param
holding 5, `f` one String parameter, `g` two):

```
7 f e      the argument was 7              (a bare read ends the run; the prefix fills)
f e 7      takes 1 argument, none supplied (the run is empty and so is the prefix)
9 e f      the arguments were 5 and 9      (no forward run: the prefix, top first)
g "x" e    the argument was 'x'            (the run is ["x"]; the prefix is empty)
g e "x"    none supplied                   (the first forward token is a read)
e g "x"    'x' and 5                       (the run ["x"], filled from the prefix)
"x" e g    5 and 'x'                       (the prefix, top first)
f (e)      the argument was 5              (a paren group is evaluated first: written)
```

A bare word read is resolved by lookup at the dispatch and never lands
in the forward window. A paren group is evaluated before the dispatch and
does. So the written count is the check pass's own
`ReorderForwardCandidates` at the dispatch, bounded by the forward count,
published beside `CurCallWord` for the user-fn record. The VM then needs
it per call site, next to the call's forward count, to rebuild the window
over its own stack prefix.

## NUR235 — a fn-body-local fn def bound into a returned map: the member read returns the fn {#nur235}

**Status:** FIXED 2026-09-26 (a named fn value's push carries its name — the handoff log's entry of that
date) · **Recorded:** 2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the emit.go coverage agent's probes.

**Rule:** one read, one dispatch — a member read of a function calls it
(ADR-011, NUR078).

**Divergence** (measured at 5c0d6b1). A silent wrong answer:

```
def mkg fn [[c:Any][Any][def g fn [[][Any][c]] {g: g/v}]] end def m (mkg 5) end m.g
  interp:   [5]
  compiled: [fn g]
```

The body-local `g` captures the param `c`; the returned map carries it as
a member, and `m.g` reads it. The interpreter calls the 0-arg fn (5); the
compiled read leaves the fn value.

**The fix.** The closure unit a fn value compiles to is shared by every fn
value over the same body and inputs (its memo key), so it cannot carry the
value's anonymity; `lambdaUnit` read every fn value as the anonymous `=>`
flavour, and the landing parked a named one. The push now carries the name
(`namedFnValueSpec` marks `ClosureRetSpec.Named` for a value whose FnDefInfo
is not Anonymous, making a contract-free spec when the value declares none;
the VM copies it to `ClosurePayload.Named`). The landing fires a named
closure over a nullary unit (`ClosureCallsAtLanding`) where an anonymous
one parks, and the bridge and the renamed render read anonymity as the
unit's lambda flavour without a name (`ClosureIsAnonymous`). Pinned: lang
`TestNUR235NamedFnValueMemberCalls` (six rows); compiler
`TestNamedFnValueSpec`, `TestClosureCallsAtLanding`.

## NUR236 — a word splice over a def-bound gradual read of a fn {#nur236}

**Status:** FIXED 2026-09-26 (a spliced consumer is ordered by the stream — the handoff log's entry of that
date) · **Recorded:** 2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the emit.go coverage agent (block 16050, `deoptStatementStart`).

**Rule:** one read, one dispatch — a bare name bound to a function calls
wherever it is written (NUR078).

**Divergence** (measured at 5c0d6b1). A silent wrong answer:

```
def tp word [typeof] def h fn [[m:Map][Any][def j (m get "f") j tp]] h {f: ([] => [42])}
  interp:   [Integer]    — `j` calls the fn (42), and typeof names its type
  compiled: [Function]   — typeof runs over the fn value
```

The same program over a data member (`h {f: 5}`) agrees; the agent's test
(`TestGradualReadConsumedBySplicedWord`) pins those.

**The fix.** The per-read deopt (planDeopts) places its test at the read's
push when the consumer follows the read — decided by comparing source
positions. A spliced word's expansion carries the positions of the word's
DEFINITION, earlier than the read, so the point was declined and the read
kept its slot push. A consumer from outside the body is now ordered by the
event stream (`deoptStatementStart`): tested at the read's push when no
body event after the read runs before the consumer, else before that
event, as the in-body case. Pinned: lang `TestNUR236SplicedConsumerDeopts`
(six rows, an intervening event among them). The general "best effort"
remains where a point cannot be placed at all.

## NUR237 — a def rebound in a taken branch after a loop-result def reads the pre-branch value {#nur237}

**Status:** FIXED 2026-09-26 (an S5 name's later root defs are registry-visible — the handoff log's entry of that
date) · **Recorded:** 2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the main-side coverage agent (branch_carried.go).

**Rule:** one binding, one value — a read sees the last def that ran.

**Divergence** (measured at 5c0d6b1). A silent wrong answer:

```
def x (for 2 [5]) def c true if c [def x 1] [] end x
  interp:   [5 1]
  compiled: [5 5]
def x (for 2 [5]) def c true if c [def x 1] [def x 2] end x
  interp:   [5 1]
  compiled: [5 5]
```

`def x (for 2 [5])` binds x to the loop's last value and leaves the other
on the stack. A read of `x` before the branch makes the lanes agree.

**The fix.** A top-level read of a name an S5 first-value loop bind bound
has no event or local home and reads the live registry binding
(`dynScopeRescue`'s top-level arm). The program's residual is resolved
only after the events lower, so its rescue marked the name dynamic-scope
too late for the arm's `def x 1` — lowered to nothing, its twin a carrier
the replay skips. Every later ROOT def of such a name is now
registry-visible (`loopSplitRebind`: not the split bind itself, whose
splice installs it). Pinned: lang `TestNUR237LoopSplitRebindInABranch`
(eight rows, `undef` and a fn read among them).

## NUR238 — a paren-bounded trailing apply that matches nothing {#nur238}

**Status:** OPEN (proposed verdict: resolve by fix) · **Recorded:**
2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the eng coverage agent (vm.go's nameless trail-top raise).

**Rule:** one failed dispatch, one outcome, on both lanes.

**Divergence** (measured at 5c0d6b1):

```
(5 ([s:String] => [s]))
  interp:   [5 fn (String)]           — an anonymous value that matches nothing parks as data
  compiled: signature_error: cannot call ``
def lam ([s:String] => [s]) end (5 lam/v)
  interp:   [5 fn lam(String)]
  compiled: signature_error: cannot call `lam`
def g fn [[s:String] [String] [s]] end (5 g/v)
  interp:   uncalled_function: call to 'g' matched no signature
  compiled: signature_error: cannot call `g` — the argument was 5
def g fn [[s:String] [String] [s]] end def h fn [[f:Function] [Any] [(5 f/v)]] end h g/v
  interp:   uncalled_function: call to 'f' matched no signature
  compiled: type_error: h: expected 1 return value(s), got 2 — [5 fn f(String)]
```

At the top level no delivery head is recorded (`seatDynApplyName` records
nothing for the main code), so the op takes its nameless raise; inside a
fn the named window parks.

## NUR239 — an applied fn value's return-contract error names the fn, not the binding {#nur239}

**Status:** OPEN (proposed verdict: resolve by fix) · **Recorded:**
2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the eng and main-side coverage agents.

**Rule:** one failure, one diagnostic, on both lanes.

**Divergence** (measured at 5c0d6b1):

```
def z fn [[] [Integer] ['s']] end def h fn [[k:Function] [Integer Integer] [(k 5)]] end h z/v
  interp:   type_error: k: return value 1: expected Integer, got ProperString
  compiled: type_error: z: return value 1: …
def Handler class {cb: Function} def h (make Handler {cb: (fn [[n:Integer][Integer][n 1]])}) each h.cb [1 2 3]
  interp:   type_error: <fn>: expected 1 return value(s), got 2 — [1 1]
  compiled: type_error: : expected 1 return value(s), got 2 — [1 1]
```

The interpreter names the binding the fn was called under (`k`) and an
anonymous fn `<fn>`; the compiled checks read the definition's own name
(`checkFnValueReturn` reads `fd.Name`). `(1 k)` and a module fn
(`h M.z/v`) split the same way.


**Progress (2026-09-26).** The anonymous half is fixed: a nameless fn value's
return check names its frame `<fn>`, as the interpreter's fn-value frame
does (`core.FnValueFrameName`; lang `TestNUR239NamelessFnValueIsFn`). The
binding half — `k:` for the interpreter, `z:` compiled — stays open.

## NUR240 — a trapped unmatched member call inside a branch arm: a different code {#nur240}

**Status:** OPEN (proposed verdict: resolve by fix) · **Recorded:**
2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the emit.go coverage agent (the unit trap).

**Rule:** one failed dispatch, one error code, on both lanes.

**Divergence** (measured at 5c0d6b1). The code is caught as data, so the
answers differ:

```
import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] export "M" {dec: dec/v} ] end def msg (do [if true [(true 5 M.dec)] [1] "no-raise"] error [dot code]) msg
  interp:   [uncalled_function]
  compiled: [signature_error]
```

The unit trap declines when the call sits inside an arm rather than at the
unit's root frame, and the call's compiled no-match raises its own code.

## NUR241 — a capturing callback run by walk: a captured read and a flex append {#nur241}

**Status:** OPEN (proposed verdict: resolve by fix) · **Recorded:**
2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the main-side coverage agent (walk_core.go's closure hook).

**Rule:** one callback, one result, on both lanes.

**Divergence** (measured at 5c0d6b1):

```
def acc (flex []) end def mk fn [[tag:String][Function][([m:Any] => [acc (tag) append acc (m.path) append])]] end def h (mk "x") end walk {mode: "depth"} {a:1 b:2} h/v ; acc
  interp:   [{a:1 b:2} ['x' '' 'x' 'a' 'x' 'b']]
  compiled: signature_error: cannot call `append` — the arguments were [] (a FlexList) and '' (an EmptyString)
```

## NUR242 — programs that compile and then fail inside the compiled runtime {#nur242}

**Status:** OPEN (proposed verdict: resolve by fix) · **Recorded:**
2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the coverage agents' probes.

**Rule:** a program the compiler admits, the compiled runtime runs — the
bail ledger (`bailDefectCeiling`) counts the corpus's; these are outside
it.

**Divergence** (measured at 5c0d6b1; each an internal_error compiled with
the "please report it" note):

```
Q = def z fn [[] [Atom] [(quote z)]] end def y fn [[] [Integer] [42]] end def h fn [[] [Integer] [42]] end
Q def h fn [[x:Atom/q] [Atom Atom] [x x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end [(m.f y)]
  interp [[y y]]            compiled RESTEP_LANDING: the /q capture of `y` left 2 value(s) where the apply after it claims 1
Q def h fn [[x:Atom/q] [Atom] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end if true [(m.f add 1 2)] [0]
  interp [add 1 2]          compiled RESTEP_LANDING at h: the re-step CAPTURES the word `add`
Q def h fn [[x:Atom/q] [Atom] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end def dbl fn [[n:Integer][Integer][n mul 2]] end if true [(m.f y dbl)] [0]
  interp signature_error: cannot call `dbl`     compiled RESTEP_LANDING at h: the re-step CAPTURES the word `y`
def h fn [[] [Integer] [42]] end def h fn [[n:Integer] [Integer] [n add 1]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end def z fn [[] [Integer] [0]] end def y fn [[] [Integer] [42]] end if true [(m.f y)] [0]
  interp [42 42]            compiled shaped method apply (paren apply): value is not an appliable function
import module [def inc fn [[n:Integer] [Integer] [n 1]] export "M" {inc: inc/v}] end def mk fn [[] [Map] [{f: M.inc/v}]] end def m (mk) end do [m.f 5]
  interp [error(inc: expected 1 return value(s), got 2 — [5 1])]    compiled shaped method apply inc: result count 2 violates the host-registered shape claim 1
def f fn [[Integer] [Any] [do [args drop] 7]] end f 5
  interp [7]                compiled STORE_LOCAL stack underflow
def g fn [[b:List] [Any] [do b 7]] end g [5 drop]            (and `b:Any`)
  interp [7]                compiled CALL_DYN_FRAME underflow
def Box class {data: Any} end def b (make Box {data: "s"}) end 0 fold [add] b.data
  interp signature_error: cannot call `fold`    compiled CALL_NATIVE_POLY no match for fold
```

## NUR243 — three valid programs refused {#nur243}

**Status:** FIXED 2026-09-26 (the three programs compile — the handoff
log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced by:** closing #505's merged-coverage gap (ADR-008)
— the coverage agents.

**Rule:** valid code compiles (a compile failure is a compiler defect).

**Divergence** (measured at 5c0d6b1):

```
def f fn [[c:Boolean][Any][if c [def x 1] [] for 3 [def x 5] x]] f true
  interp [5]      compiled: fn f: body result of unknown provenance
def x 0 if [true] [def x 1] [2] end x
  interp [1]      compiled: if: branch produces no value (Stage 2 lowers single-result branches)
```

The first is emit.go's `NoteLoopCarried` guard (the "NUR204" arm): its
comment says it keeps the loop's own index out of the carry, but
`AnalyseLoopBody` already skips bind names and `boundLocals` never holds
an index — what it catches is a branch-join binding that may be unbound.
The second is the constant-condition arm of `if` (basic/go
native_control.go): its guard carried a `//covergate:allow` whose proof
(a defensive guard) was false. The pragma is removed and
`TestNUR243ConstTakenArmWithNoValueDeclines` covers the guard, pinning
today's decline. The third: NUR109's bound-slot arm (compiler lower.go)
checks every operand of a parser dispatch, so a SOURCE operand bound on
one branch (`parse p s`) declines with a reason naming the parser; checked
over the parser operand alone both lanes agree
(`TestFnDispatchBranchBoundOperandDeclines` pins today's decline).


**The fix.** The constant branch records a taken arm that leaves no value
as a 0-value statement: a phantom None result, the event marked zeroOut,
as the both-arms-zero branch does. The loop carry's first guard, which
returned on a branch-join pre binding, is gone. The loop reuses the
branch's cell, so no init reads the pre early, and the joined binding stays
bound-checked after a loop that may not run. The parser dispatch's arm
checks the parser operand alone. Its bound-slot test went with the others:
a parser is a fn value the join never carries. Pinned: lang
`TestNUR243ValidProgramsCompile` (nine rows, a zero-iteration loop's
undefined_word among them), `TestFnDispatchBranchBoundSourceAgrees`. The constant
branch's decline site retired: both compile-failure censuses 91 → 90.

## NUR244 — a branch-bound inline fn parser is used where the interpreter finds no binding {#nur244}

**Status:** OPEN (proposed verdict: resolve by fix) · **Recorded:**
2026-09-26 · **Surfaced by:** closing NUR243 — probing NUR109's arm.

**Rule:** one binding, one lookup — a name a skipped arm would have bound
is unbound.

**Divergence** (pre-existing; measured at 8516705). A silent wrong answer:

```
import "boru:parselang" def c false if c [def p (fn [[source:String opts:Map] [Any] [7]])] [] end parse p 'x'
  interp:   parse_unknown_lang: parse: no parser "p" is registered
  compiled: [7]
```

With `[0]` as the else it is `[0 7]` compiled. NUR109's decline catches a
parser operand that is the arm's own promoted event (`(Parse.parser g)`);
an inline `fn` literal bakes as a value, and the dispatch uses it.

## NUR228 — a native's forward window binds a gradual stack operand the runtime value may not fit {#nur228}

**Status:** FIXED 2026-09-26 (the gradual window declines — the handoff
log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced by:**
closing NUR064, probing `send` beside a `receive`.

**Rule:** one dispatch, one operand window, on both lanes — a window the
runtime value decides is not compiled as one of its outcomes.

**Divergence** (measured on main at 3b5db68 and on the branch head):

```
def v (whereis "x") v send {a: 1} "nobody"
  interp:   [None]            — v is None; send (Any, String) takes {a: 1} and "nobody"
  compiled: signature_error   — send (Any, Pid) took {a: 1} and v: "argument 2: expected Pid, got None"
whereis "x" send {a: 1} (self)
  interp:   [None]
  compiled: compile_failed "stack discipline: result operand of send is not on top"
```

`send`'s overloads split their operands by type: a Pid beneath is the
destination after one forward token; anything else stays, and both forward
tokens go to the String (or Pid-paren) overload. The matcher's scan for
`send (Any, Pid)` stops at `"nobody"` (not a Pid) and fills the slot from the
stack — at run time only when the value there IS a Pid, but in the check
pass whenever it is a dynamic carrier, which matches every slot
optimistically. The pass compiled that window, one of two the runtime value
chooses between.

**The fix.** `PlanMatch` notes a stack operand taken on an UNPROVEN match (a
carrier whose static type does not conform to the slot); when the selected
candidate took some forward tokens and such an operand, and a LATER
candidate's own scan collects past the token this one stopped at
(`laterCandidateCollectsPast`) — as a value: a function word the plan
admits only speculatively at an Any slot (its dispatch's result to complete
the slot) is no wider window, which is what kg/queries.boru's `and` before a
`var` body's `__varundef` cleanup meets — a compiling pass flags
`AmbiguousGradualSplit` — the latch the reverse case (the static choice
forward-collects, the runtime grabs the carrier) already sets — and the
compile declines with "forward/stack split depends on a gradual operand".
All-stack matches stay the forward-drift guard's. A window no later
candidate would widen (`v send {a: 1} 5`), a concrete value beneath
(`"q" send …`) and a paren that seals the call off still compile and agree.
Pinned: core `TestNUR228GradualStackWindowIsAmbiguous`,
`TestNUR228NoLaterClaimIsNoAmbiguity`,
`TestNUR228SpeculativeClaimIsNoWiderWindow`; lang
`TestNUR228GradualStackWindowDeclines`,
`TestNUR228ProvenWindowsStillCompile`.

## NUR225 — template strings and XML `${}` holes canon in debug form {#nur225}

**Status:** FIXED 2026-09-26 (templates and XML holes spell their source —
the handoff log's entry of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** the canon fixpoint gate that landed with NUR072.

**The fix.** Canon arms for both kinds, in core/go and core/ts. A template
string renders as backtick source (`canonTemplate`): literal text takes the
template lexer's own escapes — `\\`, `` \` ``, `\$` before `{`, and the control
escapes canonString uses — and each hole renders `${…}` over its tokens'
canon (`CanonValues`, so a hole's words are bare and its lambda folds). An
XML literal with holes renders as its XML (`canonXmlTmpl`): literal text and
attribute text escaped as a plain element's are, holes as `${…}`, nested
templates recursing. The TS twin keeps a tagged EMPTY hole as `${}` (Go's
untagged parts cannot hold one in a template, which divergent.tsv's `\`${}`
row records). All 30 ledgered rows reach their fixpoint. Pinned: core
`TestNUR225TemplateCanonIsSource`, `TestNUR225XmlTmplCanonIsSource`;
parse.tsv's rows.

**Rule:** ADR-015 — canon renders source that re-parses to the same value;
a debug spelling is a defect.

**Divergence:** a template string canons as `interp('v ' ${1} ' w')` and an
XML literal with a `${}` hole as `interp-xml(<a>${1}</a>)`. Neither parses
back: the template re-parses as a syntax error (`unexpected ${`), the XML
form as the word `interp-xml` over a paren group. 30 of the fixpoint
ledger's 33 rows. Both ports render the same debug form, so this is a
contract defect, not a parity one.

## NUR226 — a map key that needs quoting canons bare {#nur226}

**Status:** FIXED 2026-09-26 (the key canons as its key — the handoff log's
entry of that date) · **Recorded:** 2026-09-26 · **Surfaced by:** the canon
fixpoint gate.

**The fix.** `canonKey`: a key renders bare only when it lexes back as the
same single key — letters, digits, `_`, `$`, `-`, `@` — and as a quoted
string otherwise (an empty key, whitespace, a structural character such as
`?`, `.`, `/`), in every canon map arm (map, flex, weak flex, typed map,
options and inspect maps in TS), both ports. `{'q k':2}` and `{'':1}` keep
their one key; `{1.5:2}` renders `{'1.5':2}`, the same map. Pinned: core
`TestNUR226MapKeysCanonAsTheirKey`; parse.tsv's rows.

**Rule:** ADR-015.

**Divergence:** `{'q k':2}` canons `{q k:2}`, which re-parses as two
entries, `{q:q k:2}`; `{'a b': 1}` the same. Two ledger rows, both ports.

## NUR227 — a typed tag before an XML literal re-lexes as an angle sugar {#nur227}

**Status:** FIXED 2026-09-26 (a comma before the angle — the handoff log's
entry of that date) · **Recorded:** 2026-09-26 · **Surfaced by:** the canon
fixpoint gate.

**The fix.** The angle gate opens on a `<` after ANY bare capitalised token,
whitespace or not, so a sequence joins such a part to a part opening `<`
with a comma — the list/paren separator every sequence accepts, which parses
to nothing (`joinCanonParts`, both ports). `<a/>:A` canons `[:A, <a/>]`, and a
word `A` before an XML literal `A, <a/>`. Pinned: core
`TestNUR227CommaSeparatesAnAngleReceiver`; parse.tsv's row.

**Rule:** ADR-015.

**Divergence:** `<a/>:A` canons `[:A <a/>]`, and the parser discards the
space, so `A <a/>` lexes as the angle sugar `A<a/>`: the re-parse is `[:A<a/>]`.
One ledger row, both ports.

## NUR224 — a predicate's typed-def refusal is a plain error on one lane and a compiler defect on the other {#nur224}

**Status:** FIXED 2026-09-26 (the predicate's refusal is a type_error — the
handoff log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced
by:** closing NUR100, probing a type literal bound through a predicate type.

**Rule:** one refusal, one error, on both lanes — and a program's own
refusal is never booked as a compiler defect.

**Divergence** (measured on the committed head):

```
def Big fnpred n:Integer [n gt 10] end def f fn [[x:Any] [Any] [def q:Big x q]] end f 5
  interp:   def q: value 5 does not satisfy predicate type Big                  (a plain error)
  compiled: [boru/internal_error]: def q: value 5 does not satisfy predicate type Big
            + "this is a compiler defect: the program compiled and then failed inside the compiled runtime"
```

The typed def's other refusals are `type_error`s built with `r.BoruError`
(`def q:T "x"` — does not unify with declared type T — on both lanes). The
predicate branch alone raised `fmt.Errorf`, in the interpreter's
`defFnPredicateBind` and in the compiled typed-bind op's `RunTypedBind`
alike, and the compiled run's error boundary (`compiledRunError`) wraps any
error that is not a BoruError as an `internal_error` with the compiler-defect
note. So the same refusal was a bare error on one lane and a reported
compiler defect on the other. Loud on both; pre-existing.

**The fix.** Both sites raise `type_error` with the same detail, so the
refusal is one error everywhere and the compiled boundary passes it through
as the program's own. The boundary's comment, which cited this refusal as an
example of a handler's internal_error, is corrected. Pinned: lang
`TestNUR224PredicateRefusalIsATypeError` (a runtime candidate and a type
literal, both lanes, with a member's bind as the negative).

## NUR223 — the callback seam returns an unconsumed unnamed param beneath the answer {#nur223}

**Status:** FIXED 2026-09-26 (the seam discards the unconsumed input — the
handoff log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced
by:** closing NUR100, probing a value-pattern predicate.

**Rule:** a call's unnamed arguments are call-scoped data: what the body
leaves of them beneath its declared returns is discarded at the call's end,
on every path that runs the body (the frame collapse's `__RC`, CallBoru's
trim, the compiled RET's `NUnnamed` allowance).

**Divergence** (measured on the committed head):

```
def Z fnpred [[Integer] [true]] end 0 is Z           interp: true   compiled: false
def Z fnpred [[Integer] [dup 0 eq]] end 0 is Z       interp: true   compiled: false
def Z fnpred [[0] [true]] end def v:Z 0 v            interp: 0      compiled: RunPredicate: predicate must return exactly one value, got 2
```

A predicate's body runs through the callback seam (InvokeCallbackFn). On the
interpreter, CallBoru trims the residual to the signature's declared return
count (fnpred's implicit `[Any]`), discarding the unconsumed unnamed input;
the compiled seam ran the fn's STORED unit, which is compiled
count-agnostic (it declares no returns of its own, so its RET hands back the
whole residual), and returned `[0 true]` — which the predicate protocol
refuses, and `is` turns a refusal into `false`. Silent on `is`, loud on a
typed def; pre-existing.

**The fix.** `InvokeCompiled`, which holds the signature, applies CallBoru's
own discard to a callback's result (`trimUnconsumedUnnamed`): residuals
beyond the signature's declared return count are dropped from the bottom,
up to its unnamed-param count. A named call's root RET already discards
through the frame's contract. Pinned: lang
`TestNUR223CallbackSeamDiscardsUnconsumedUnnamed` (five rows on both lanes,
with the consuming and named-param negatives, and a short residual's parity),
`lang/spec/fnpred.tsv` §8's last two rows.

## NUR222 — a dyn body's settled lead is applied again by the program's residual arm {#nur222}

**Status:** FIXED 2026-09-26 (the dyn body's settled lead — the handoff
log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced by:**
closing NUR190, probing the `/q` capture inside a `do` body.

**Rule:** a code body's residual is the body's own: the interpreter re-steps
a value inside the body where it lands, and what the body leaves is data to
the program around it until a later token re-steps it.

**Divergence** (measured on the committed head):

```
def inc fn [[n:Integer] [Integer] [n add 1]] end def mk fn [[] [Map] [{f: inc/v}]] end def m (mk) end
do [m.f 5]       interp: 6        compiled: CALL_DYNAMIC underflow (internal error)
7 do [m.f 5]     interp: 7 6      compiled: the same
```

A `do` whose body the closure path declined runs as a DYN BODY
(tryRecordDynBody): the handler runs the body with the interpreter's
semantics, and the result is marked variadic — its count is the body's own.
The check pass modelled the member read inside as a lead left over its
argument ([fn, 5]), and the program's residual arm (resolveDynamicApply)
planned the fn-value apply over that window — a second application of a
lead the body had already applied, over a region holding one value where
the op expected two. Loud, pre-existing.

**The fix.** A residual lead produced by a dyn-body dispatch with another
of that dispatch's own results above it (`dynBodySettledLead`) is the
body's window, settled at run time: the residual arms stand aside and the
region seats as the body's residual. A dyn-body result with nothing of its
own above it is still the lead the interpreter re-steps over a LATER token
— `do [m.f/v] 5` and `5 do [m.f/v]` are 6 on both lanes, and keep the
apply. Pinned: lang `TestNUR222DynBodySettlesItsOwnLead` (eight rows, the
settled and the re-stepped).

## NUR221 — a gradual apply event's lead the interpreter re-steps where it stands {#nur221}

**Status:** FIXED 2026-09-26 (a landed lead is no apply-event lead — the
handoff log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced
by:** closing NUR220, probing `apply` over a map-each member read with a
value beneath it.

**Rule:** the interpreter steps a member read where it is written (NUR038):
a fn that claims the value beneath applies there, before any later word
sees it.

**Divergence** (measured on the committed head):

```
def inc fn [[n:Integer][Integer][n add 1]] end def m {x: inc/v} end
each ([kv:Any] => [3 kv.v apply]) m        interp: apply's signature_error   compiled: {x:4}
```

The gradual apply event (recordGradualApplyEvent → OpCallDynApplyOne, the
twenty-seventh increment) models `apply` over [lead, receiver] as the lead
applied to the receiver — right for an INERT lead (`3 kv.v/v apply`, `3
(kv get "v") apply`: 4 on both lanes). A bare member read is not inert: its
re-step claims the 3 at its own step (the check pass notes the landing),
so apply meets the 4 and raises. The landing applies over an empty window
only, so nothing compiled the claim; the event applied the member once and
answered 4. Silent, pre-existing.

**The fix.** The event admits no lead whose producing event carries a
re-step landing (`landingAfter`), unless a `/v` read delivered it
(`valReadNoted`) or a user paren placed it (`ParenPlacedFnIDs` — `nd (m
get "inc") apply`, whose re-step ran inside the sealed paren over
nothing); such a lead takes the existing dynamic-lead decline, and the
callback's other strategies answer it. Pinned: lang
`TestNUR221LandedLeadIsNoApplyEventLead` (the inert spellings compiled at
4, the landed one's no-match on both lanes).

## NUR220 — a dynamic apply enters an anonymous 0-arg lambda the interpreter parks {#nur220}

**Status:** FIXED 2026-09-26 (the anonymous park at the dynamic apply —
the handoff log's entry of that date) · **Recorded:** 2026-09-26 ·
**Surfaced by:** closing NUR219, probing the map-iteration seam.

**Rule:** ADR-016 as the interpreter reads it (execFnDefLiteral): a
lambda VALUE with an empty window — no forward args, no stack args — is
data, unless `apply` asked for the application (FnDefInfo.Applied).

**Divergence** (measured on the committed head):

```
def m {x: ([] => [5])} end each ([kv:Any] => [kv.v]) m          interp: {x:fn}   compiled: {x:5}
def m {x: ([] => [5])} end each ([kv:Any] => [kv get "v"]) m    interp: {x:fn}   compiled: {x:5}
```

The body's member read arms the whole-frame replay (noteClosureBodyReplay:
a member fn read at the residual's top), and the replay's lone token went
to `dynApplyEnter`, which matched the 0-arg signature and entered the
lambda's stamped unit. The landing reads the park (landingWalk); the apply
entries did not. Silent, pre-existing.

**The fix.** The replay reads the park over its lead
(`replayLeadParks`) before the Apply kernel, and its island parks the value
as the interpreter does. The park is the VALUE re-step's, so a lead the
body read bare BY NAME (the replay's word table) is exempt: that is the
binding's word dispatch, which fires a 0-arg fn whatever its origin — `def
r (mk)  r` is 42 on both lanes (`TestBodyLocalWordReadParity`, which a
first draft reading the park inside `dynApplyEnter`, a seam the shaped
method's name reads share, turned red). That exposed the other half: `[kv.v apply]` compiled to the SAME
code as `[kv.v]` — apply's identity result carries the lead's own id, and
the registered-output arm elided it as the lead's producer's, dropping the
Applied mark (so the replay answered 5 for both, right only by accident
for the apply). An `apply` over a LONE gradual lead is now exempt from
that arm (`loneGradualApplyLead`), so it reaches the dynamic-lead decline
the apply block already had — no new decline site — and the callback's
other strategies answer `{x:5}`. A named 0-arg member still fires (`{x:1}`). Pinned: lang
`TestNUR220DynamicApplyParksAnonymousZeroArg`.

## NUR219 — a callback body reads its gradual param as a slot {#nur219}

**Status:** FIXED 2026-09-26 (the callback body's word read — the handoff
log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced by:**
closing NUR217, probing the same read through `each`.

**Rule:** NUR123 — a bare read of a frame binding that holds a fn is a WORD
dispatch on the interpreter, whatever the param's declared type.

**Divergence** (measured on the committed head):

```
def g fn [[f:Any] [Any] [f]] end each g/v [([] => [1]) 7]             interp: [1 7]              compiled: [fn f 7]
each ([x:Any] => [x]) [([] => [1]) 7]                                  interp: [1 7]              compiled: [fn x 7]
def g fn [[x:Any] [Any] [x typeof]] end each g/v [([] => [1]) 7]      interp: [Integer Integer]  compiled: [Function Integer]
fold ([a:Any kv:Any] => [a]) {x: 1} ([] => [5])                        interp: 5                  compiled: fn a
```

A fn value or lambda handed to a higher-order word compiles to a callback
BODY unit with the value's own named params (tryRecordLambdaClosure), run
by the handler per element through the token seam (InvokeBody) or the
fn-VALUE seam (InvokeCallbackBody). The unit is analysed under the
element's carrier — gradual for a mixed or unknown collection, fn-typed
for a list of fns — and a callback body's reads are not accounted
(NUR123's accounting is a plain-unit concern), so the read pushed the
slot. Silent, pre-existing; the named call `g ([] => [1])` already
declines loudly.

**The fix.** A callback body unit lists the params it reads bare where it
pushes the slot — a gradual read, or a fn-typed read no apply lowering
credited (`storedUnitFnReadParams`, NUR217's list, now for every closure
unit) — and the pushed closure carries the callback VALUE it was compiled
from (`callbackSourceSpec` → `ClosureRetSpec.Source` →
`ClosurePayload.Source`). The VM's token seam (`closureSourceStep`, before
`applyClosure`) hands an invocation with a fn in such a slot to the
interpreter's run of that value, as the handler's own interpreter lane runs
it: stepped over the inputs (InvokeBody's no-Invoker branch) on the TOKEN
seam, matched and called by `InvokeCallbackFn` on the fn-VALUE seam — with
the closure's runtime captures in place of the check pass's carriers
(`run 5 [([] => [1]) 7]` over `[x:Any] => [(x add k)]` is `[6 12]`). Data
elements run the unit. Pinned: lang `TestNUR219CallbackParamReadIsTheWord`
(twelve rows compiled, the 1-arg no-match on both lanes); compiler
`TestFnReadRefused`, `TestCallbackSourceSpec`.

## NUR218 — a `/v`-quoted member read is not the value its word twin is {#nur218}

**Status:** FIXED 2026-09-26 (a member reference is its word twin — the
handoff log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced
by:** closing NUR078, probing the `/v` spelling a member read now needs at
every slot.

**The fix.** The member value is DELIVERED, the way the word twin is:
execFnDefLiteral's peek consumes the marker and steps past the value
without quoting it (position keeps it inert, as it keeps `inc/v`; a rewind
over it is the language's apply), and under the check pass notes the value
read (`NoteValRead`, as stepWordVal does) so the residual lowering knows the
delivery is inert — a named fn at a frame's tail is returned, not NUR186's
zero-argument call. The check pass's quoted stand-ins stop leaking: a bare
read of a binding that may hold a fn is modelled as the word dispatch it is
whatever the bound value's quote (`def g (m.f/v) end g 4` compiled to `fn
4`), the marker drop notes the carrier's delivery too, and a code body
whose top is a `/v`-delivered member takes no replay (`[1 2 3] each
[m.f/v]` applied it compiled; it declines loudly now, the one shape here the
word twin compiles and the member twin does not). The forward scan tags the
value it quoted for its marker so the arrival delivers it unquoted, a
carrier included — the flex twin of `if true m.h/v [2]` was data compiled.
Found alongside and fixed with it (pre-existing for the BARE spelling): a
DYNAMIC member at a branch arm (`if true m.h [2]` over a flex) is a value
the interpreter re-steps after `if` returns, firing a 0-arg fn; the branch
event marks such an arm `MayBeFn` and the computed-arm lowering lands the
merge (`OpReStepLanding`) as the general merge does — it compiled to the
member itself. Pinned: lang `TestNUR218MemberRefIsItsWordTwin` (ten shapes
× word / map / flex / module, both lanes, compiled; the body negative),
`TestNUR218DynamicBranchArmLands`; core `TestFnValueDispatchModLeavesInert`
(the delivery is unquoted).

**Rule:** `/v` yields the binding's VALUE, whatever reads it (ADR-011 as
renamed 2026-08-19): a member read `m.f/v` is the same value as `inc/v`
when `m.f` holds `inc`, and ends up wherever the word twin's value ends up.

**Divergence** (measured on the committed head, `def inc fn [[n:Integer]
[Integer] [n add 1]] def m {f: inc/v}`, `s2` a two-param adder, `one` a
0-arg fn under the same member):

```
each (m.f/v) [1 2 3]      interp: [fn fn fn]   compiled: [2 3 4]   word twin: [2 3 4] both
fold (m.f/v) [1 2 3] 0    interp: fn s2        compiled: 6         word twin: 6 both
if true (m.f/v) [2]       interp: fn one       compiled: 1         word twin: 1 both
(m.f/v 5)                 interp: fn 5         compiled: 6         word twin: 6 both
def g (m.f/v) end g 4     interp: 5            compiled: fn 4      word twin: 5 both
[1 2 3] each [m.f/v]      interp: [fn fn fn]   compiled: [2 3 4]   word twin: [fn fn fn] both
```

The interpreter's value is QUOTED where the word twin's is not:
execFnDefLiteral's peek consumes the marker and quotes the member value at
the pointer, where `stepWordVal` delivers `inc/v` unquoted and steps past
it, so the quote rides into a paren's survivor, a callback slot (the token
seam steps a quoted fn as data) and a code body's result. The compiled lane
models neither spelling's value consistently — it applies the member value
where the interpreter's quote keeps it data, and reads a def of the quoted
carrier as data where the run binds (and a bare read of the binding calls).
Silent both ways. NUR078 fixed the forward-collected spelling (`each
M.inc/v [1 2 3]`, `[1 2 3] each m.f/v`, `if true m.f/v [2]` — CollectArrival
delivers the reference unquoted); the paren, def and body spellings are this
record.

## NUR217 — a member-read fn applied over a fn argument reads its gradual param as a slot {#nur217}

**Status:** FIXED 2026-09-26 (the stored unit's word read — the handoff
log's entry of that date) · **Recorded:** 2026-09-26 · **Surfaced by:**
closing NUR078 (`m.g z/v` stopped raising and reached the apply).

**The fix.** The unit a stored fn value runs (compileStoredFnUnit's
`storedfn$body`) is compiled ONCE, under the declared param types: nothing
re-runs it under an argument's runtime type the way a named call's unit is
(the per-call-shape analysis that makes `g ([] => [42])` compile a frame
replay), no body tokens are seated for a deopt, and the code-body replay
is for member reads only. So a bare read of one of its own frame bindings
that may hold a fn had no faithful lowering, and the slot push answered
the value where the interpreter dispatches the word. Two halves close it.
The unit DECLINES where no argument can make the read data
(`storedUnitFnRead`): a fn-typed read no apply lowering credited, a
binding read both bare and `/v`, and a gradual CAPTURE read left in the
residual (a capture is no argument, so no seam can refuse it) — the value
keeps its plain const and the apply falls to the interpreter's own
dispatch, per value, not per program. A GRADUAL param the body reads
(`[f]`, `[[f]]`, `[x f]`, `f typeof`) is data for every argument but a fn,
so declining it would decline every `m k get` over an `Any` param — and
declining the residual read sent `m.p 5` over `[x:Any] [x]` to the
interpreter (nine corpus rows, the engine-entry census) — so the unit
instead LISTS the slot (`storedUnitFnReadParams` →
`CompiledFn.FnReadParams`) and each seam that runs a stored unit — the
in-program frame push (`dynApplyEnter`), the foreign host
(`dynApplyForeign`), the token seam (`invokeFnValue`) and the callback
seam (`invokeCompiled`, `CompiledFnRef.RefusesArgs`) — refuses an argument
list with a fn in such a slot, so that one call takes the interpreter's
dispatch and data keeps the unit. Measured: `m.g ([] => [42])` 42, `m.g
(z/v)` 0, a `f:Function` param 42, `[x f]` 6, `[[f]]` `[[42]]`, the 1-arg
no-match the word's `signature_error`, all on both lanes; `m.g 5` is 5 and
runs the unit; the module member and a def-bound value compile per call
and are untouched. The same read in a CALLBACK body is NUR219.
Pinned: lang `TestNUR217StoredFnParamReadIsTheWord`.

**Rule:** NUR123 — a bare read of a frame binding that holds a fn is a WORD
dispatch on the interpreter, whatever the param's declared type.

**Divergence** (measured on the committed head):

```
def g fn [[f:Any] [Any] [f]] end def m {g: g/v} end m.g ([] => [42])            interp: 42   compiled: fn f
def g fn [[f:Any] [Any] [f]] end def z fn [[] [Integer] [0]] end
def m {g: g/v} end m.g (z/v)                                                   interp: 0    compiled: fn f
```

`g z/v` (the direct call) is 0 on both lanes: the pass re-runs the body
under the argument's RUNTIME type (a Function carrier), so NUR123's replay
seats the read. The member read's apply is DYNAMIC — the unit runs over
whatever the value receives, compiled for the declared `Any` param, whose
bare read keeps the slot push NUR123 left a gradual read ("a gradual one
keeps the slot push it always had"). Silent, pre-existing.

## NUR216 — a `/v`-marked class member read with arguments beside it is applied compiled, data interpreted {#nur216}

**Status:** FIXED 2026-09-25 (the quoted lead is data on every arm — the
handoff log's entry of that date) · **Recorded:** 2026-09-25 · **Surfaced
by:** closing NUR096, probing the `/v` spelling of the fn-shape member.

**Rule:** `/v` marks a value as data wherever it is written, on both lanes
(NUR213's rule).

**Divergence.**

```
def T fnsig Integer Integer def C class {op:T}
def c (make C {op:(fn [[x:Integer] [Integer] [x add 1]])})
c.op/v 5      interp: fn (Integer) 5      compiled: 6
3 c.op/v 2    interp: 3 fn (Integer) 2    compiled: 3 3
3 4 c.op/v    interp: 3 4 fn (Integer)    compiled: 3 5
def m {f: (fn [[x:Integer] [Integer] [x add 1]])}
3 m.f/v 2     interp: 3 fn (Integer) 2    compiled: 3 3
3 4 m.f/v     interp: 3 4 fn (Integer)    compiled: 3 5
```

NUR213 taught the pass to QUOTE the value a standalone `/v` marker follows
(the run-time peek quotes the concrete fn) and the DYNAMIC lead arm of the
residual layout to honour the quote. A class member read is not dynamic —
it is a carrier typed by the member's fn shape — so it reached the
Function-carrier lead arm, which never asked; and the verbatim window
islands (`OpCallDynamicMixed`, mixed and trailing) classify through
`fnLikeResidual`, which did not ask either, so they re-stepped the fn live.
Silent; pre-existing.

**The fix.** A quoted value is data on every arm: the Function-carrier lead
arm skips a quoted lead, and `fnLikeResidual` answers false for a quoted
value, so neither window island claims it. The class spellings compile and
agree; the map window twins decline at the existing residual limit ("call
result above a literal") — NUR213's `5 m.f/v` does the same — and the
fallback answers as the interpreter. Pinned by lang
`TestClassMemberValueMarkerIsData`.

## NUR214 — a fresh def in a loop that may not run binds anyway {#nur214}

**Status:** FIXED 2026-09-25 (the loop's fresh cell — the handoff log's
entry of that date) · **Recorded:** 2026-09-25 · **Surfaced by:** closing
NUR205 (the zero-trip module bind), whose value-def twin this is.

**The fix.** The loop-carried mechanism now carries a FRESH name too. The
loop join (`AnalyseLoopBody`) gives a fresh name bound in a loop that is not
proven to run a post-loop binding of its own — a carrier (`JoinCarriers(v,
v)`), so no read folds the body's value into it — and `NoteLoopFresh`
seats it in the unit's cell for the name with NO init: the zero slot is
"unbound", every body def stores into the cell (the carried rebind store)
and installs the name per iteration (the carried def's dyn-scope bind),
and the joined carrier's reads load the cell BOUND-CHECKED, raising the
interpreter's `undefined_word` when the body never ran. The carrier's
twin replays nothing ahead of the loop (ApplyBindTwin's carrier skip), so
the registry holds the name exactly when a body def installed it. NUR204's
index guard learnt the difference between a loop index and a fresh cell
(`loopFresh`), so a second loop re-carries the name as any pre-loop
binding. Every shape in the table below agrees on both lanes; `lang`'s
`TestLoopFreshDefZeroTrips` holds thirteen of them. A fn that reads the name
DYNAMICALLY after a zero-trip loop (`g` below) reached the VM's
dynamic-scope read miss, which bailed with `internal_error` where the
interpreter raises `undefined_word` — NUR215, closed the same day.

**Rule:** a `def` runs when its loop body runs. A name first bound inside a
loop that runs zero times is not bound afterwards — the interpreter raises
`undefined_word` on the read.

**Divergence:** the compiled lane binds it anyway, silently.

```
def n (0 add 0) end for n [def x 5] x              compiled [5]      interp undefined_word
def n (0 add 0) end for n [def x (n add 5)] x      compiled []       interp undefined_word
def n (0 add 0) end while [n gt 0] [def x 5 def n 0] x    compiled [5]    interp undefined_word
def f fn [[n:Integer][Any][for n [def x 5] x]] end f 0    compiled [5]    interp undefined_word
def n (0 add 0) end for n [def x 5] def g fn [[][Any][x]] end g    compiled [5]    interp undefined_word
```

Measured beside it, and agreeing: a loop that runs (`def n (2 add 0) end for
n [def x i] x` is `1` on both lanes), and a static `for 0` (the pass prunes
the body, NUR110's table).

**Why.** The loop join (check/go/carrier.go `AnalyseLoopBody`) installs a
FRESH name's body value as the post-loop binding — the body's own value,
not a join with "unbound" — so a read after the loop const-folds the
body's constant or reads the body's promoted value slot, which a zero-trip
run never stored (the slot pushes nothing, and the residual drops it). The
branch twin of this (NUR110) was closed by the branch-carried def: a slot
per name, a store at the arm's def, the post-merge read BOUND-CHECKED
(`OpPushLocalBound` raises the interpreter's `undefined_word`). The
loop-carried def (`NoteLoopCarried`) is that mechanism's source, but it
carries only a name with a PRE-loop binding (its init); a fresh name gets
no slot and no check. The join's twin, placed before the loop, also binds
the name in the registry whether or not the body runs, which a dynamic
read (`g` above) sees.

**Verdict:** fix, compiler-side, on the loop-carried mechanism: a fresh
name bound in a loop that may not run is carried in a slot with no init,
its post-loop read bound-checked, its registry twin not replayed ahead of
the body.

## NUR213 — a `/v`-marked map member read with arguments beside it is applied compiled, data interpreted {#nur213}

**Status:** FIXED 2026-09-25 (the marker's intent on the value — the
handoff log's entry of that date). Recorded the same day, closing NUR212.

**The fix.** Not the shaped member apply after all: under the check pass
the member read is a DYNAMIC Any carrier, `fnDefAtPointer` fails on it,
so execFnDefLiteral's marker peek — which quotes a concrete value at run
time — never ran, the marker reached the pointer standalone and was
dropped, and the pass's residual held an unquoted dynamic lead beside the
5, which Finalize's residual layout applied (`resolveDynamicApply`,
`CALL_DYNAMIC /1`). The standalone-marker drop now quotes the dynamic or
carrier value the marker follows (the peek's own rule, for the value the
pass holds), and the residual layout honours the quote: a quoted lead is
not applied by the dynamic-lead arms nor declined at the "dynamic value
precedes residual args" boundary (it is data beside its neighbours), and
a quoted trailing value is not a trailing apply. `m.f/v 5` is `fn
(Integer) 5` on both lanes; `5 m.f/v` declines at an existing residual
limit ("call result above a literal") and answers by fallback. The drop
quotes a DYNAMIC or CARRIER value only — a concrete Function value is the
peek's own at run time on both lanes — and a quoted fn-typed carrier
passes the residual render gate as data (a fn that returns the value it
was handed returns it quoted: `(f MathUtil.sqrt/v) 16.0`). Pinned by
lang `TestReachValueMarkerIsNoArgument` (its NUR213 rows) and fn-value.tsv
§15.

**Rule:** `/v` marks a value as data wherever it is written, on both lanes.

**Divergence.**

```
def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f/v 5
  interp:    fn (Integer) 5
  compiled:  6
def m {f: (fn [[a:Integer] [Integer] [a add 1]])} 5 m.f/v
  interp:    5 fn (Integer)
  compiled:  6
def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f/v
  interp:    fn (Integer)
  compiled:  fn (Integer)
def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f 5
  interp:    6
  compiled:  6
```

The interpreter's re-step of the reach-collapsed member peeks the marker
right after the group and leaves the value inert (execFnDefLiteral's
dispatch-modifier peek). The checker's shaped member apply
(method_shape.go → `RecordDynMethod`, the guarded shaped-instance-method
model) declines only the ALONE case (`aloneInLiveReachGroup`: "`m.f/v`
landed a call one token before the `/v` that says DATA") and otherwise
collects the forward window — the marker between the group and the `5`
is not read — and records the apply the interpreter never makes. Silent;
pre-existing at the merge base.

**Where it belongs:** the method-shape model's window over a reach group
followed by a dispatch-modifier marker — the marker says data, so no
shaped apply may record past it (the same rung the alone case has,
widened to a marked value with a window).

## NUR212 — a `/v`-marked module member with an Any first parameter is not collectable as a def's forward argument {#nur212}

**Status:** FIXED 2026-09-25 (the marker is no argument — the handoff
log's entry of that date). Recorded the same day, closing NUR163, whose
record carried the shape as a side note.

**The fix.** The plan's reach-as-call-head test (`ReachCallHeadBarrierOn`
→ `ReachFnWouldClaimOn`) probes the token after the reach through
`ForwardClaimProbeOn`, which had no arm for a dispatch-modifier marker:
the marker fell to the literal arm ("a concrete literal") and was offered
to the fn's first parameter — an `Any` matched it, an `Integer` or
`String` did not — so the reach read as a call head about to claim its own
`/v`, a barrier, and `def`'s window collected nothing. The probe answers
`probeNone` for a marker now (it qualifies the value before it and is never
an argument), so the reach is one datum and the value binds. Pinned by
lang `TestReachValueMarkerIsNoArgument` and module-fnvalue-boundary.tsv §5.

**Rule:** a fn value is the same fn however it is reached, and `/v` marks
it as data wherever it is written.

**Divergence.**

```
import module [ def up1 fn [[value:Any] [String] ['UP']] export "M" {up1: up1/v} ] end
  def g M.up1/v          signature_error: cannot call `def` — no signature matches the arguments (none were supplied)
  def g (M.up1/v)        binds; g 1 → 'UP'
  def g M.up2/v          binds (up2's first parameter is Integer)
  M.up1/v                fn up1(Any)   — data, on its own
  def g up1/v            binds (no module: the word carries ForceVal itself)
```

Both lanes agree — the interpreter's verdict, which the check pass
reproduces. The parser emits a dotted path's `/v` as the reach followed by a
dispatch-modifier marker (`Word/__DM`, `groupModifier`); at the pointer
`execFnDefLiteral` peeks the marker and leaves the value inert. Inside
`def`'s FORWARD window the trace shows the plan collecting nothing at all
(`candidate … takes 7 arguments, but none were supplied`): the reach's
value is a Function whose `Any` parameter admits what follows, and the
window's plan does not read the marker the way the pointer does.

**Where it belongs:** the forward window's plan over a reach followed by a
dispatch modifier (`ForwardClaimProbeOn` treats a reach as optimistic and a
`/v` WORD as one datum; the reach-plus-marker pair has no such arm).

## NUR211 — a named fn value driving `fold` that stops matching past step 0 parks compiled, raises interpreted {#nur211}

**Status:** FIXED 2026-09-25 (the named value's no-match on the seam — the
handoff log's entry of that date). Recorded the same day, closing NUR166.

**The fix.** `unmatchedLambdaBody` (the token seam's arm for a callback
body unit with the value's own param contract that no step matches) applied
NUR155's rule — an unmatched anonymous lambda is data — to every closure.
A closure compiled from a NAMED value carries the def's name
(`ClosurePayload.RetName`, seated by the callback compile and the
def-time rename), and a named value's no-match is the word's raise on the
interpreter (execFnDefLiteral's `uncalled_function`, with its hint). The
arm raises that error for a named closure now and keeps the data rule for
an anonymous one; step 0 already raised through the fn-value arm, and
`scan`'s park stands on both lanes. The raise anchors where the
interpreter's does — at the reference's own token (`h/v`, 1:60), which
the closure carries as `RetPos`. Pinned by lang
`TestNamedValueNoMatchOnTheSeamRaises` and callbacks.tsv §11.

**Rule:** a compiled program answers as the interpreter does.

**Divergence.**

```
def h fn [[a:Integer b:Integer] [List] [[a b]]] end 0 fold h/v [1 2]
  interp:    fold: step 1: [boru/uncalled_function]: call to 'h' matched no signature
  compiled:  [fn (Integer, Integer)]
def h fn [[a:Integer b:Integer] [Integer] [a add b]] end 0 fold h/v ['x']
  interp:    fold: step 0: [boru/uncalled_function]: call to 'h' matched no signature
  compiled:  the same raise
```

Step 0 matches (`0`, `1`) and answers `[0 1]`; step 1 offers that List as
the accumulator to `a:Integer`, and no signature matches. The interpreter's
seam steps the NAMED value and raises `uncalled_function` (the word's
no-match under its name, NUR107's rule); the compiled fold's seam reaches
`unmatchedLambdaBody`, whose rule is NUR155's — an unmatched TYPED LAMBDA
at its call site leaves the value as DATA, the interpreter's own answer for
an ANONYMOUS value — and applies it to a named value, whose no-match is a
raise. A no-match at step 0 raises on both lanes (the first step takes a
different arm), and `scan` parks on both lanes (its own handler's rule).
Pre-existing at the merge base (measured on `wt-head`, 2026-09-25).

**Where it belongs:** the token seam's unmatched-lambda arm — a value with
a NAME (a def-bound fn, `h/v`) raises the word's no-match as the interpreter
does; only an anonymous value parks. The same seam's fn-VALUE arm
(`invokeFnValueClosure`) already distinguishes the two through
`noMatchIfSigged`; the closure-body arm does not.

## NUR204 — a body def of the for loop's own index: the interpreter's index level, the compiled lane's write-back {#nur204}

**Status:** FIXED 2026-09-25 (the lexical index scope — the handoff log's entry of that date; recorded 2026-09-24, found while landing the user-call
write-back promotion — the handoff log's "the user-call write-back" entry).
Present on main before that change (measured on a clean worktree at
3768c46: the native shapes below compiled to the same wrong values; the
user-call twin declined "unpromoted computed value" until the promotion
reached it). The compiled lane DECLINES the shape now, loudly, on both
paths.

**Rule:** a compiled program answers as the interpreter does — what a
counted loop leaves bound under its own index name after the loop is the
same on both lanes.

**Divergence.**

```
def i 0 end for 3 [def i 9] end i
  interpreted   [2]
  compiled      [9]     (before the decline)
def i 0 end for 3 [for 2 [def i 9]] end i
  interpreted   [0]
  compiled      [9]     (before the decline)
def i 0 end for 3 [def i (i add 1) i] end i
  interpreted   [1 2 3 2]
  compiled      [1 2 3 3]
```

The interpreter's `for` binds its index as a def-stack level it rebinds
per iteration; a body `def i` pushes a level above it, and the loop's
cleanup pops one level — so the INDEX level survives the loop, bound to
the last index (2), where the plain `def i 7  for 3 [i]  i` restores 7,
and an inner loop's leftover is popped by the outer loop's own cleanup
(0). The compiled loop carried the body's def as the loop-carried root def
it is by name and wrote the body's value back (9), inside a fn frame the
same (a frame slot), through an `if` arm the same. Neither lane answers
what a lexical loop scope would (the pre-loop 0).

**Fence.** `TestForIndexDefInBodyPending` (lang
loop_carried_user_call_test.go) pins the interpreter's values and the
compiled lane's decline ("def of the enclosing for loop's own index `i`
inside its body", lowerDynBind over loopCtx.iterName — the for's index
name rides RecordLoop into emitLoop.iterName now), so closing it is loud.

**Verdict (2026-09-25): the LEXICAL index scope, resolved by fix on both
lanes.** A body def of the loop's own index rebinds the ITERATION's
binding and nothing else. The interpreter records the index level's depth
at loop entry (`ForCont.IterDepth`) and pops the body's levels with the
iteration and the index level with the loop (`popIterLevels`, on the
done, break and fault-unwind paths alike), so the pre-loop binding shows
after the loop as it does when the body never rebinds; the check pass's
loop analysis pops its bind names to the same depths and never joins or
carries them. The compiled loop stores the def into its index slot
(`lowerDynBind` → `storeBindInto`; the twin marked written back), where
FOR_NEXT overwrites it with the next index and no write-back reaches the
root. `def i 0 end for 3 [def i 9] end i` is 0 on both lanes, `for 3 [def
i (i add 1) i]` leaves `1 2 3 0`, a nested loop and an `if` arm the same;
the fence became the pin (`TestForIndexDefInBodyIsTheIterations`).


## NUR203 — a dynamic keep-defs body's leak is invisible to the fn's later reads {#nur203}

**Status:** FIXED 2026-09-25 (the dynamic body's leak — the handoff log's entry of that date; recorded 2026-09-24, found while closing NUR202 — the
handoff log's "the keep-defs token body" entry). Present on main before
that change (measured on a clean worktree at 3768c46: the same wrong
value, for the opposite reason — the run-time body's def never reached the
registry at all).

**Rule:** a compiled program answers as the interpreter does — a body a
keep-defs word runs in the caller's frame leaks its defs to that frame,
whether the body is a literal or a value.

**Divergence.**

```
def f fn [[b:List xs:List][Integer][def t 0 each b xs drop t]] end f (quote [def t (t add 1) t]) [1 2 3]
  interpreted   [3]
  compiled      [0]
```

The body is a VALUE (a List param; a def-bound quoted list is the same),
so the compile pass never sees its tokens: the dispatch records as a
dynamic body (tryRecordDynBody), the run-time stamp of the body keeps its
defs in the registry per element now (NUR202's close — the each's result
list is right), but the fn's read of `t` after the call keeps its
compile-time home: NoteKeepDefsLeak seats a later read live only for the
names a COMPILED keep-defs unit defs, and a body the pass cannot see names
nothing, so the read answers the pre-call 0. The same shape at the ROOT
agrees (`def t 0  def b (quote [def t (t add 1) t])  each b [1 2 3] drop
t` is 3 on both lanes: a root read is live). `fold` over a dynamic body
diverges the same way.

**Fence.** `TestDynamicKeepDefsBodyLeakInFnPending` (lang
keep_defs_leak_test.go) pins both lanes as they stand, so closing it is
loud.

**The fix (2026-09-25).** The directed rule: after a keep-defs dispatch
over a DYNAMIC body inside a unit, every name the unit value-defined
before the dispatch is taken as leaked (`noteDynKeepDefsLeak`, from
`tryRecordDynBody`): it joins `keepLeakNames` and `dynLeakNames`, so the
unit's later reads of it seat LIVE whatever their compiled home
(`NoteLiveRead`), and a carried slot is refreshed from the registry as
`NoteKeepDefsLeak` does. The registry — where the body's per-element
install put the value under the dynamic-environment mode the dispatch
arms — is the one answer: both witnesses are 3 on both lanes, and the
root shape is unchanged. The fence became the pin
(`TestDynamicKeepDefsBodyLeaksToTheFn`).


## NUR202 — a keep-defs body over a gradual list runs as a const the native drives, and its def does not leak {#nur202}

**Status:** FIXED 2026-09-24 (the keep-defs token body — the handoff log's
entry of that date). Recorded 2026-09-24, found while measuring the
code-bodies compile failures after the keep-defs bodies landed.

**The fix.** Two halves. The closure unit for `[def t (t add 1) t]` over
a gradual list declined in its probe — "unapplied fn-value in body
residual" — because the untouched Any ELEMENT beneath `t` read as a
dynamic value that might auto-apply; an input enters the frame resolved
and is never stepped on either lane, so the gate
(closureResidualHasUnappliedFn) exempts a unit's own untouched inputs,
and the body compiles to its each$body closure (the kept install of
NUR200), the Integer-returning twin (code-bodies.tsv L189) with it. And
the path the decline took — the body as a const the native runs, stamped
at run time as a detached unit (StampTokenBody) — is a KEEP-DEFS unit now
(stampDetachedSig arms the unit flag for a token body's stamp; the
re-stamp box carries it), the host hands the unit's kept installs to the
enclosing context's trail (hostForeign, so the enclosing frame's exit
pops them as its own), and the body's own defs are left out of the
stamp's dependency snapshot (bodyDefNames — its rebinding of `t`
re-stamped the body at every element and, past the budget, ran the rest
on the interpreter: three entries over seven elements, measured). The
run-time bodies — `each b xs` with `b` a List param bound to `[def t (t
add 1) t]` — stamp once and run the whole list on the VM, and the each's
result agrees; the fn's later READ of `t` after such a dynamic body is
NUR203.

**Rule:** a compiled program answers as the interpreter does — a
multi-run body's def leaks one install per element on both lanes.

**Divergence.**

```
def f fn [[xs:List][List][def t 0 each [def t (t add 1) t] xs]] end f [1 2 3]
  interpreted   [[1 2 3]]
  compiled      [[1 1 1]]
```

Over a GRADUAL list (a fn's `xs:List` param, a declared-List factory's
result) the each body is not a closure unit: it lowers as a CONST the
native runs per element (`PUSH_CONST_FRESH` then `CALL_NATIVE each`, the
S1a dynamic-callback path). The interpreter's eachHandler drives
InvokeBody once per element on the shared registry with no def cleanup,
so `def t (t add 1)` leaks and the next element reads it (1, 2, 3); the
compiled lane's run of the same const body reads 0 at every element —
the body's def does not reach the read the next element makes. The same
body over a LITERAL list compiles to an each$body closure unit (the kept
OpBindDynScope of NUR200) and agrees. The Integer-returning twin —
`… each [def t (t add 1) t] xs drop t` (code-bodies.tsv L189) — declines
"fn f: body result of unknown provenance" on the same path, loudly.

**Fence.** `TestKeepDefsConstBodyOverGradualListPending` (lang
keep_defs_leak_test.go) pins both lanes as they stand, so closing it is
loud.

**Verdict:** the compiled lane's, as the recording directed: a keep-defs
token body the lowering hands the native as a const leaks its defs as the
interpreter's InvokeBody does (the per-element install with the runtime
value), and the closure the const stood in for compiles.


## NUR201 — a callee's def survives a trapped raise on the interpreter, not on the VM {#nur201}

**Status:** FIXED 2026-09-25 (the frame's error path — the handoff log's
entry of that date). RECORDED 2026-09-24, found while closing NUR199 —
the handoff log's "NUR199 closed — the keep-defs body" entry. Present on
main before that change (measured on a clean worktree at 3768c46).

**The fix.** The interpreter took the verdict below: a raise runs the
frame's cleanup. `Engine.faultReturn` — the one exit every run-loop error
takes — tears down every fn frame the error leaves OPEN on the tape
(`unwindLiveFrames(0, len)`: a frame whose open paren has stepped and
whose `__DC __pa undef…` tail has not), innermost first, replaying each
tail exactly as a break/continue discarding the region replays it and as
CallBoru's sub-run tears its own frame down inline on the same error: the
body-local defs truncated to the entry snapshot, the per-call Args list
and FnBaseline popped, the captures and params undef'd. The residual-eval
error at the DefCleanup marker (a computing body's pending container
raising) no longer replays its tail at the marker — that frame is one of
the live frames the fault return unwinds, and a second replay would pop
the CALLER's args entry and its same-named binding (the retired
`unwindFrameTailOnError`). `do [g]  t` is `[error(x) 0]` on both lanes,
so is the param twin `def g fn [[t:Integer][Integer][raise 'x']]  do [g
9]  t`, the nested frames (`do [h]` over `h` calling `g`), the raise
inside a paren group, a callback body, a residual container and a native
error, a lambda value and an applied fn value, the trap inside a fn frame
and a loop; a `do` body's OWN def under the same raise still leaks on both
lanes (NUR199's rule — a `do` body is not a frame).

**Rule:** a compiled program answers as the interpreter does — the
bindings a raise leaves behind are the same on both lanes.

**Divergence (as it stood).**

```
def t 0 end def g fn [[][Integer][def t 9 raise 'x']] end do [g] end t
  interpreted   [error(x) 9]
  compiled      [error(x) 0]
```

The interpreter's raise skipped the fn frame's def-cleanup tail (the
`__dc` markers never step), so a def the callee made before raising
LEAKS into the caller's scope when the error is trapped — `do [g]`
answers the Error value and `t` reads 9. The VM's frame installs nothing
a trap could keep (a fn body's def of a name nobody dyn-reads lowers to
no registry install at all; one that does is unwound with the frame on
the error path, `vmContext.run`'s deferred unwind), so the read answers
the root binding. The same shape with the def in the `do` body ITSELF
agrees since NUR199: a keep-defs body's own installs survive the raise
(`def t 0  do [def t 5 raise 'x']  t` is 5 on both lanes).

**Fence.** `TestCalleeDefTornDownOnTrappedRaise` (lang
keep_defs_leak_test.go): the shape and its neighbours as parity rows, and
the interpreter's own answers — the callee's local, its param and its
args list gone when the trap resumes, the frame beneath the raise still
reading its own. `TestRunErrorUnwindsLiveFrame` and
`TestRunErrorUnwindsFrameOnceAfterResidualError` (core
fn_frame_unwind_test.go) pin the kernel mechanics over a hand-built
frame: the tail replayed, and replayed ONCE. fn-locals-scope.tsv §12
carries the rows.

**Verdict:** resolve by fix, taken. The interpreter's behaviour was the
questionable one — a frame's bindings outliving the frame because it
raised — and "a raise runs the frame's cleanup" is the rule a language
would choose; the VM already kept that rule, and the interpreter's error
unwinding now keeps it too.

## NUR200 — an each body's def of a loop-carried name: the compiled loop's slot never sees it {#nur200}

**Status:** FIXED 2026-09-24 (the multi-run keep-defs body — the handoff
log's entry of that date). RECORDED the same day, found while closing
NUR199; present on main before that change (measured on a clean worktree
at 3768c46).

**The fix.** A multi-run defs-keeping body (each, fold, scan, for-each,
outer, walk — `CallableSpec.BodyMultiRunKeepsDefs`) is a KEEP-DEFS unit
exactly as `do`'s body is (NUR199): a value def the root's arm-resident
bridge did not pair with a twin — inside a loop fragment, a fn body — lowers
to the kept OpBindDynScope, installed once per element and left standing
past the unit's RET for the enclosing frame to pop; the enclosing loop
refreshes its carried slot from the registry after the call
(NoteKeepDefsLeak); and every read of a leaked name seats LIVE at its token,
a CONCRETE read included — the multi-run leak is per element and absent at
zero iterations, so the registry at the read is the one answer, and a miss
raises the interpreter's undefined_word (Program.LiveReadNames). The twin
regime's read fence for such names (`armBoundNames`, "read of `x` after a
multi-run body binds it") is retired for VALUE names — their reads are live
now — and kept for TYPE names, whose per-element node has no live read. A
flex a multi-run body MUTATES and reads back after it (`def acc (flex [])
for-each [acc swap append drop] xs  acc`) seats live too: the pass holds a
re-modelled carrier with no compiled home, and the registry's cell is the
very object the body's own live lookups mutated (NoteLiveRead's
mutable-ref arm). Only a TOKEN body is keep-defs: a lambda callback runs in
a frame of its own on both lanes.

**Rule:** a compiled program answers as the interpreter does — a def an
`each` body makes leaks into the enclosing scope on both lanes, once per
element, and a read after the loop sees the last one.

**Divergence.**

```
def t 0 end for 3 [[1] each [def t 5] drop] end t
  interpreted   5
  compiled      0
```

NUR199's multi-run twin. The `for` loop carries `t` in a frame slot (the
check pass's loop join sees the body rebind it), but the each$body unit
keeps its def frame-local: no arm-resident twin is adopted inside a loop
fragment (AdoptResidentTwins' root fence), so nothing installs the
binding at run time and nothing refreshes the slot — the post-loop read
answers the pre-loop value. NUR199's keep-defs mechanism is the once-run
`do` body's (one install, the closure's own RET keeping it); a
per-element install rides the resident-twin bridge, which is fenced to
the root stream today.

**Fence.** `TestEachBodyDefInLoopResolves` and the multi-run rows of
`TestDoBodyDefLeaksToTheEnclosingScope` (lang keep_defs_leak_test.go):
the loop shape above, its computed twin, the read after each / fold / scan
/ var bodies, the zero-iteration miss, the fn frame's teardown, the
frozen-read rows `7 [9] 11`, and the mutated flex read back; the langspec
multi-run parity oracle's read-after rows are parity rows now.

## NUR199 — a do body's def of a loop-carried name: the compiled loop's slot never sees it {#nur199}

**Status:** FIXED 2026-09-24 (the keep-defs body — the handoff log's entry
of that date). RECORDED the same day, found while surveying the corpus's
compile failures (code-bodies.tsv rows 180 and 186 declined "dynamic-scope
def `t` of unpromoted computed value" inside a `do` body, and the probe
beside them answered wrong).

**The fix.** A `do` body's closure unit is a KEEP-DEFS unit
(`fnUnitRec.keepsDefs`, armed by the dispatch through
`EmitState.keepDefsUnitDepth` and stamped by StartFnCompile on the unit
the `do` opens): every value def it makes lowers to a kept
OpBindDynScope — the install through the interpreter's own installer —
whose trail entry the VM leaves standing past the unit's own RET
(`CompiledFn.KeepsDefs`), so the ENCLOSING frame's exit pops it (a fn
frame, as the interpreter's def-cleanup tears the leak down) or the run
keeps it (root, as a root def). The enclosing side
(`EmitState.NoteKeepDefsLeak`): a name an armed loop carries in a frame
slot gets a store right after the call — the registry value into the
slot — and every leaked name's later read seats live at its token
(NoteLiveRead's keep-leak arm), where the interpreter reads it. The twin
the root adopts for such a def (AdoptBodyTwins) is marked written back
by name (`markKeepDefsTwins`), so its replay installs nothing and the
runtime install is the one level `undef` pops. A def whose value the
install cannot re-push — a fn value, a `word` splice marker, a macro
(`emitDynBind.keepSkip`) — keeps the lowering it had, nothing, with the
adopted twin's replay standing; the sweep's twenty-two do-body variants
of the `afn`, `word`, `macro`, `walk` and `for-each` seeds measured that
fence into existence.

**Rule:** a compiled program answers as the interpreter does — a def a
`do` body makes leaks into the enclosing scope on both lanes, and a read
after the loop sees it.

**Divergence (as it stood).**

```
def t 0 end for 3 [do [def t 5]] end t
  interpreted   5
  compiled      0
def t 0 end for 3 [do [def t (i add 1)]] end t
  interpreted   3
  compiled      0
def f fn [[][Integer][def t 0 for 3 [do [def t 5]] t]] end f
  interpreted   5
  compiled      0
```

The `for` loop carried `t` in a frame slot (the check pass's loop join
saw the body rebind it), but the do$body unit kept its def frame-local —
lowered to nothing, the twin the loop analysis noted for the post-loop
join replaying a carrier — so nothing installed the binding at run time
and nothing stored the slot: the post-loop read answered the pre-loop
value. The computed twin (`def t (t add 1)`) declined instead, the
corpus rows.

**Fence.** `TestDoBodyDefLeaksToTheEnclosingScope` (lang
keep_defs_leak_test.go): the three shapes above answer as the
interpreter does (the fn-frame one declined "result is a variadic loop
value" at first — a counted defect where it answered 0 — and compiles
with parity since the multi-run keep-defs body of the same day: its
post-loop read has a compiled home, the refreshed carried slot), with the
computed def, the root leak read back as a residual and as an operand,
the undef interplay, a def inside a branch arm of the body, a fn def, a
loop inside the body, and the fn frame's teardown.

## NUR198 — a dotted read through a MISSING member: the interpreter's undefined_word, the compiled lane's None {#nur198}

**Status:** FIXED 2026-09-25 (the None receiver's bare-word key — the
handoff log's entry of that date). RECORDED 2026-09-24, found while
closing the flex-member map literal — the handoff log's "the flex-member
map literal" entry. Present on main before that change (measured on a
clean worktree at 6ccf827).

**The fix.** The accessor's `[Key | None]` row — "chained-read
propagation", the row that makes `none get 'c'` and `None dot (quote x)`
None — carried no QuoteArgs, so it never collected a BARE-WORD key: the
matcher parked `dot` on a pending forward and the word `c` stepped on its
own, `undefined word: c` (a no-match over any other receiver raises `cannot
call dot`; only None parked). The check pass mirrored the park for a
static None and stopped at the same undefined_word; a run-time None behind
a typed receiver it modelled as a member read, and the compiled chain
answered None — the compiled lane held the rule. The atom row quotes the
key now, on `dot` (`{TAtom, TNone}` with QuoteArgs, beside the `{TAny,
TNone}` row for an evaluated key; `get` strips the quote as it strips
every other row's) and on its strict twin `dotr`, whose bare-word read
over None raises the family's not_found (`getr: parent is None`) on both
lanes instead of undefined_word. `m.b.c` over `{a:1}` is None on both
lanes, so are `none.c`, a None behind an `Any` param, a list element and
an Error value's missing field, and `5 m.b.c` keeps the 5 beneath.

**Rule:** a compiled program answers as the interpreter does — the same
raise, or the same value, for a dotted read whose receiver is None at run
time.

**Divergence (as it stood).**

```
def f fn [[m:Map][Any][m.b.c]] end f {a:1}
  interpreted   undefined_word: undefined word: c
  compiled      None
def f fn [[m:Map][Any][(m.b).c]] end f {a:1}
  interpreted   undefined_word: undefined word: c
  compiled      None
def C class {x:[{y:1}]} end ((make C {}).x get 0).y
  interpreted   undefined_word: undefined word: y
  compiled      None
```

`m.b` over `{a:1}` is None on both lanes (`def m {a:1} m.b`). The
interpreter's `dot` over a None receiver does not consume the atom that
follows: `c` steps as a WORD and raises undefined_word. The compiled
chain reads the second hop as a `dot` over the first's result and answers
None. Where the None is STATIC (`none dot y`, `(none).y`, a fn over an
`Any` parameter called with none) the check pass stops at the same
undefined_word and the compiled lane fails to compile — consistent; the
divergence is the run-time None behind a typed receiver, which the check
pass models as a member read.

**Fence.** `TestDotChainOnMissingMemberResolves` (lang
map_literal_flex_member_test.go): the three shapes and their neighbours —
a static None, a chain of misses, a value beneath the read, the strict
twin's not_found, `get`'s evaluated key — as parity rows, with the
interpreter's own answers; accessor.tsv's None section carries the rows,
and REFERENCE.md names the chained read.

**Verdict:** resolve by fix, taken on the interpreter's accessor table:
one answer, None, the row's own promise.

## NUR197 — a loop body's residual literal over the loop variable: the interpreter's undefined_word, the compiled lane's value {#nur197}

**Status:** FIXED 2026-09-25 (the loop region's residual — the handoff log's
entry of that date). RECORDED 2026-09-24, found while closing the
flex-member map literal — the handoff log's "the flex-member map literal"
entry. Present on main before that change (measured on a clean worktree at
6ccf827), for list and map literals alike.

**The fix.** The compiled lane held the rule: a literal a loop body leaves
is the ITERATION's, evaluated with the loop's binding, as a fn body's
residual is evaluated in its frame (ResidualEvalsInFrame) — the
interpreter's deferral to the end of the run was the accident of a loop
region living on the top tape, and it read an OUTER `i` where one was
bound (`def i 9  for 2 [[(i add 1)] i]` was `[[10] 0 [10] 1]`). The
interpreter's loop region now evaluates a pending residual container when
it collects the iteration's output, with the iterator still bound
(`Engine.collectLoopRegion`, shared by the `for` continuation and the
`while` body region; every other value — a typed container's inert shape
included — is collected as it stood and resolved by the end-of-run sweep
as before). `for 2 [[(i add 1)] i]` is `[[1] 0 [2] 1]` on both lanes, the
map twins likewise, the fn shape raises the fn's own count error on both,
and the while body's literal reads its round's binding (`[[2] [3]]`).
Two twins found probing the neighbours, both closed the same day: NUR208
(the loop's iterator surviving a raise the caller traps, NUR201's loop
twin) and NUR209 (the single-literal `do` body under a loop baked as a
const on the compiled lane).

**Rule:** a compiled program answers as the interpreter does — the same
raise, or the same value, for a literal a loop body leaves as its residual.

**Divergence (as it stood).**

```
for 2 [[(i add 1)] i]
  interpreted   undefined_word: undefined word: i
  compiled      [[1] 0 [2] 1]
for 2 [{a:(i add 1)} i]
  interpreted   undefined_word: undefined word: i
  compiled      [{a:1} 0 {a:2} 1]
for 2 [{a:(flex [i])} i]
  interpreted   undefined_word: undefined word: i
  compiled      [{a:[0]} 0 {a:[1]} 1]
def f fn [[][List][for 2 [[(i add 1)] i]]] end f
  interpreted   undefined_word: undefined word: i
  compiled      type_error: f: expected 1 return value(s), got 4 — [[1] 0 [2] 1]
```

The interpreter leaves a body's residual literal UNEVALUATED on the stack
and evaluates it at the end of the run (autoEvalStack, the deferred
residual), by which time the loop has unbound `i`: the raise. The recorder
evaluates the same literal where it stands in the body — the top engine's
`case e.IsTop` arm of autoEvalList / the map gate's top arm — with `i`
bound, so the compiled lane assembles a value per iteration. Which lane
holds the rule is not decided here: the interpreter's deferral is the
documented behaviour the list gate's comment names ("a fn body returning a
bare-word list raises undefined_word at run time"), and the compiled
answer is the one a reader expects.

**Fence.** `TestLoopBodyResidualLiteralResolves` (lang
map_literal_flex_member_test.go): the shapes above and their neighbours —
the bare literal, the nested literal, the outer binding, the while body, a
`do` and a fn around the loop, the do-body twin's shapes and its lowering
— as parity rows, with the interpreter's own answers; control.tsv §3 and
§7 carry the rows.

**Verdict:** resolve by fix, taken on the interpreter: the compiled
answer was the one a reader expects, and the loop region is the
literal's frame.

## NUR196 — a fn called from a literal each body breaks: each's no-result error for the interpreter's flow_error {#nur196}

**Status:** FIXED 2026-09-24 (the escaped body ends the iteration — the
handoff log's entry of that date). RECORDED the same day, found while
closing NUR195.

**The fix.** Both lanes read the residual of a body run that had escaped
as the element's result, and each read something different: the
interpreter's sub-engine hands back the unstepped tape — a frame-cleanup
marker, which `filter` reported as "must produce a Boolean, got __CP" and
`each` collected as junk the flow error then discarded — and the VM's
island hands back nothing, which `each` reported as "body produced no
result" before the flag could be read. Whatever an iterating native
returns on an escape is discarded by the flow's resolution (the loop's
iteration is abandoned, or the run raises `outside loop`), so the one
answer both lanes share is NO result: each, fold, scan, filter (list and
map), for-each, outer, inner (1D and 2D, both ops) and eachrank end their
iteration on `core.BodyEscaped` and return nothing, and the run resolves
the flag — `for 3 [each [f] [1 2 3] i] 99` is `[99]` on both lanes, and
with no loop the interpreter raises `flow_error` where the compiled lane
takes the loop-less deferral (a counted bail, as NUR195's witnesses). The
interpreter's own `filter` error over the marker is gone with it. The
original record follows.

**Rule:** a compiled program answers as the interpreter does — the same
raise, or the same value, for a break that escapes a fn called from a
code body.

**Divergence.**

```
def f fn [[x:Integer][Integer][break]] end each [f] [1 2 3]
  interpreted   flow_error: break outside loop
  compiled      each_error: each: element 0: body produced no result
def f fn [[x:Integer][Integer][break]] end for 3 [each [f] [1 2 3] i] 99
  interpreted   [99]                (the break ends the for)
  compiled      each_error: each: element 0: body produced no result
```

The COMPUTED twin (`each (mk) [1 2 3]` over a returned `[break]`, NUR195)
agrees since the escaped flow is read after every native call. Here the
body is a LITERAL, lowered as the each body's unit: the fn `f` — its body
a bare sentinel, which the compiler declines — runs through the island,
whose contract on an unresolved flow is to tear down and return NO values
(runIslandResolved's FlowUnwind); the each handler then raises its own
no-result error for the element before the VM can read the flag the
island left set. The interpreter's sub-engine, splicing `f`'s body onto
the each body's tape, hands back a residual and the top run raises the
flow error after `each` returns. Not yet root-caused past that: whether
the island's "no values" or the handler's eagerness is the wrong half.

**Fence.** `TestLiteralBodyFlowThroughFnPending` (lang
runtime_token_body_test.go) pins the divergence as it stands, so closing
it is loud.

**Verdict:** none yet. Directed at the each handler's contract with an
escaped flow — a body that escaped should end the iteration and let the
run resolve the flag, on both lanes.

## NUR195 — a flow sentinel in a computed code body is swallowed by the compiled lane {#nur195}

**Status:** FIXED 2026-09-24 (the escaped flow after a native call — the
handoff log's entry of that date). RECORDED the same day, found while
pinning S3's first slice (the run-time token-body stamp); present on
`main` (d75dd75) before that change.

**The fix.** The seam's sub-engine hands an unresolved break/continue back
with the registry's FlowCtrl set (Engine.exitWithFlowCtrl's sub-engine
contract) for the outer run to resolve; the interpreter's run loop reads
the flag after every step, and the VM read it only after a fallback and
after a fn-value apply (`resolveEscapedFlow`), never after a plain native
call — so the flag `each`'s body run left set was dropped where the
interpreter's top run raised. The VM reads it after every native call now
(CALL_NATIVE and its poly twin): with an enclosing loop the flow unwinds
to it (`for 3 [each (mk) [1 2 3] i] 99` is `[99]` on both lanes, for the
compiled lane's `[[1 2 3] 0 [1 2 3] 1 [1 2 3] 2 99]`), and with none the
compiled lane takes the loop-less flow's designed path — the internal
error RunCompiled's callers defer to the interpreter on, which raises the
canonical `break outside loop`. The silent value is gone; what remains
is a bail the lang ledger counts (`TestComputedBodyFlowSentinelDefers`),
never an answer. The literal-body twin — a fn CALLED from a literal each
body breaking — diverges by another mechanism and is NUR196. The original
record follows.

**Rule:** a compiled program answers as the interpreter does. A `break` or
`continue` stepped outside a loop is the interpreter's `flow_error`; a
program that raises on one lane raises on the other.

**Divergence.**

```
def mk fn [[][List][quote [break]]] end each (mk) [1 2 3]
  interpreted   flow_error: break outside loop
  compiled      [[1 2 3]]                    (silent, default lane, exit 0)
def mk fn [[][List][quote [continue]]] end each (mk) [1 2 3]
  interpreted   flow_error: continue outside loop
  compiled      [[1 2 3]]
```

The literal twin `each [break] [1 2 3]` FAILS TO COMPILE (the Stage-2
code-body gate declines `each` over a literal body carrying a sentinel)
and the fallback answers with parity. The computed body compiles: the
program lowers `each` over a dynamic body, and the body reaches the VM's
InvokeBody seam at run time as a raw token list, where the interpreter's
sub-run raises and the compiled lane's does not — the seam's run treats
the escape as a loop's (the island contract's FlowUnwind, or the each
handler's per-element result) where the interpreter's plain sub-engine
raises `outside loop`. Not yet root-caused past that.

**Fence.** `TestComputedBodyFlowSentinelPending` (lang
runtime_token_body_test.go) pins the divergence as it stands, so closing it
is loud; the run-time token-body stamp declines a sentinel body (the same
rule as the lazy fn-value stamp, storedSigEligible), so the seam's own
change neither caused nor masks it.

**Verdict:** none yet. Directed at the seam's flow-escape contract: the
InvokeBody seam's sub-run should raise `outside loop` where no loop of the
program's encloses it, as the interpreter's does.

## NUR194 — a def-bound computed fn read with a written operand its contract does not take {#nur194}

**Status:** FIXED 2026-09-24 (the written argument's fit — the handoff
log's entry of that date). RECORDED earlier the same day (the do body's
read), found while pinning NUR193. Present on `main` (db7cffb); a loud
bail there since the do body's read, silent for the `do` row before it.

**The fix.** The shape claim carries the wrapper's PARAMETER TYPES
(`core.FnShape.Params`: a compiled closure unit's declared params, a const
lambda's signature; nil where only the arity is known, as for the fn-util
wrappers), and the read model's window (`shapedFnReadWindow`) asks
`SigTypeMatches` whether each written token fits before claiming it —
declining with `a written argument does not fit the wrapper's parameter`
when it does not. Inside a code body the decline is the closure probe's,
so the body islands and the island dispatches the word exactly as the
interpreter does (NUR193's bridge): `each [a5 "s" add] [1 2]` is `['6s'
'7s']` on both lanes, `do [a5 "s"]` the caught raise on both. At the top
level the decline is the program's, and the interpreter raises its own
no-match (`(a5 "s")`). Pinned in `TestDoBodyReadWrittenNoMatchParity`
(lang; the unit ledger's bail line 39 -> 37, the compile line 282 ->
283), the check arrival tests and the compiler's shape-claim test.

**Rule:** the interpreter's word dispatch matches its signatures over the
written tokens AND the frame: a written token no signature takes is not
an argument, and the frame's values are tried.

| witness (`def a5 (mk 5)`, `mk` returning `([n:Integer] => [n add k])`) | interpreted | compiled |
|---|---|---|
| `each [a5 "s" add] [1 2]` | `[['6s' '7s']]` (a5 over the element, then `"s" add`) | `shaped method apply a5: result count 2 violates the host-registered shape claim 1` on `main`; the same as interpreted since the fix |
| `do [a5 "s"]` | `[error(cannot call `a5` — …)]` | the same bail (`[fn a5(Integer) s]` before the do body's read); the same as interpreted since the fix |
| `(a5 "s")` | `signature_error: cannot call `a5`` | a compile failure since the fix, the interpreter's raise |

**The defect, in one sentence.** The shaped fn read's window model
(`tryShapedFnReadArrival`) claims the wrapper's arity of written tokens
as the arguments and records one result, where the interpreter's matcher
tries the written token against the signature and falls back to the
frame when it does not fit; the compiled apply then leaves the unmatched
[fn, token] pair and the claim's count check bails. **The fix** belongs
to the read model asking the signature whether the written token fits
before claiming it — declining, or recording the frame apply, when it
does not.

## NUR193 — a def-bound computed fn read inside a `do` body {#nur193}

**Status:** FIXED 2026-09-24 (the do body's read — the handoff log's
entry of that date). RECORDED earlier the same day (the frame's closure
bind), found while classifying NUR192's `do` row. Present on `main`
before and after the tail-read increment (worktrees at c268afb and
db7cffb); the first row was SILENT, default lane, exit 0.

**The fix, in two pieces.** (1) A compiled closure a program binds by
`def` (bindGlobal's write-back, bindDynScope's push) is the interpreter's
NAMED fn definition: the registry's `Lookup` bridges the def-stack
payload under the binding's name through the compiled runtime's own view
(`lookupUncachedBridged`, the aggregate never cached since the bridge
captures the run's invoker) and the word step routes the name through
that dispatch (`dispatchesAsWord`), so an island's `a5` matches over the
frame or the written tokens and raises the interpreter's `cannot call`
over nothing — where the simple-value substitution pushed the payload as
data and the re-step parked it. Outside a run the payload stays data. (2)
`DoListReturnsFn` takes the computed-body hatch (one dynamic(Any)) when
the body's residual carries a def-bound computed fn CARRIER — the
body's check-time run notes no read, so the carrier stood as the do's
Function result and the program residual applied it over the 7. Pinned
in `TestDoBodyReadParity` (lang: the five do shapes) and core
`TestDefBoundClosureDispatchesAsWord`.

**Rule:** a compiled program answers as the interpreter does; a `do`
body's frame holds no operand of the caller's, so a word dispatched
inside it over nothing raises.

| witness (`def a5 (mk 5)`, `mk` returning `([n:Integer] => [n add k])`) | interpreted | compiled |
|---|---|---|
| `7 do [a5]` | `[7 error(cannot call `a5` — no signature matches the arguments)]` | `[12]` on `main`; the same as interpreted since the fix |
| `do [a5]` | `[error(cannot call `a5` — …)]` | `[fn]` on `main`; the same as interpreted since the fix |
| `do [a5 7]` | `[12]` | `CALL_DYNAMIC underflow` on `main`; `[12]` since the fix |
| `do [7 a5]` | `[12]` | a compile failure (the closure render), unchanged |
| `do [(a5 7)]` | `[12]` | `[12]` |
| `do [a5 "s"]` | the caught `cannot call` | `[fn a5(Integer) s]` on `main`, silent; a bail on the shape claim since the fix — NUR194 |

**The defect, in one sentence.** The check pass's own run of the `do`
body (the plain analysis, not the closure-body compile) does not model
the def-bound computed fn's read as the dispatch it is — the read's
statement window is short and the read stands as a VALUE — so the body's
result is the carrier, and the program residual's trailing fn-value
apply takes it over the 7 beneath (`CALL_DYNAMIC_TRAILING`), where the
interpreter's dispatch inside the body's frame finds no operand and
raises; `do [a5 7]` records the apply over the written 7 and the lowering
looks for its operand on the stack. **The fix** belongs to the do body's
analysis modelling the read at its frame: a short window with an empty
frame is the interpreter's raise, a written operand is the apply's, and
the closure-body probe's stand-aside (the tail-read increment) must not
be the plain run's.

## NUR192 — a fn-body-local computed fn def under a native's raw code body {#nur192}

**Status:** FIXED 2026-09-24 (the frame's closure bind — the handoff log's
entry of that date). RECORDED earlier the same day (the def-bound computed
fn read at a closure body's tail), found when that increment's tail rule
was probed against a nested closure. Present on `main` (a worktree at
c268afb, and db7cffb before the fix); an error-versus-value divergence,
not silent.

**The fix.** The VM's `bindDynScope` pushes a compiled closure value onto
the def stack as the top-level write-back (`bindGlobal`) does — the
installer's carrier guard installs nothing for a Function-family value
without an FnDefInfo payload — and the frame's trail truncation pops it
with the frame; the closure body's real compile routes a def-read fn
carrier whose producer is an enclosing unit's event to the live lookup
(`resolveOperand`'s enclosing-event arm), so the tail read lowers as
`OpLookupDynScopeData` + `OpCallDynTrailTop` where it seated the parent's
unreachable event, and the raw-body read's island dispatches the pushed
closure. Pinned in `def_fn_body_tail_test.go` (lang: the nested read
native, the same-frame redefinition, the raw-body read open with parity,
the second call binding afresh), eng `TestBindDynScopeClosureValue`.

**Found on the way, declined loudly.** A fn body's computed fn def that
SHADOWS an enclosing frame's computed fn of the same name outlives the
call in the interpreter: its install drops the overlapping outer closure
and pushes the new one at the SAME depth, so the frame's def-cleanup pops
nothing (`def a5 (mk 1)  def g fn [[xs:List][List][def a5 (mk 5)  each
[a5] xs]]  g [1 2 3]  each [a5] [1 2 3]` is `[[6 7 8] [6 7 8]]`
interpreted; the compiled push-and-pop answered `[[6 7 8] [2 3 4]]`, and
`main` `[[2 3 4] [2 3 4]]`). The existing "computed fn shadows a live
binding" decline covered only a Defs-held outer binding; it now covers a
side-table-held computed fn bound at a shallower fn-body depth
(`CheckFnCarrierBindDepth`, the table keeping the outermost depth since
a fn body is analysed more than once), pinned in
`TestDefFnBodyTailShadowSoundCompileFailures` (the lang ledger 281 ->
282). The `do [a5 7]` row of the original table is NUR193.

**Rule:** a compiled program answers as the interpreter does; a name
`def` binds inside a fn body is in dynamic scope for every body the fn
reaches (the interpreter's def stack).

| witness (`mk` returns `([n:Integer] => [n add k])`; inside `def g fn [[xs:List][List][def a5 (mk 5)  …]] end g [1 2 3]`) | interpreted | compiled on `main` | compiled since the fix |
|---|---|---|---|
| `each [a5] xs` | `[[6 7 8]]` | `undefined_word: a5` | `[[6 7 8]]`, the body native (`LOOKUP_DYN_SCOPE_DATA` + `CALL_DYN_TRAIL_TOP`) |
| `each [a5 add 1] xs` | `[[7 8 9]]` | `undefined_word: a5` | `[[7 8 9]]`, the raw body islanded to the pushed closure |
| `do [a5 7]` | `type_error: g: return value 1: expected List, got Integer` | `CALL_DYN_FRAME underflow` (an internal error) | the same — NUR193, the `do` body's read |

**The defect, in one sentence.** A computed fn value `def` binds inside
a fn body is a binding the compiled-closure machinery owns — the runtime
installer's carrier guard installs NOTHING for a closure value
(`installDef`: a Function-family body with no FnDefInfo payload), so the
frame's `BIND_DYN_SCOPE` leaves no Defs entry — and a code body the
closure probe declines is handed to the native raw and stepped on the
interpreter against a registry that never saw `a5`. At the top level the
same def is a root computed bind (`RecordDynBind`'s root arm,
`BIND_GLOBAL`'s write-back), which the live lookup finds; inside a frame
nothing writes it, and the closure body's real compile resolves the read
to the parent's event, unreachable from its frame, where the probe (no
producedBy) rescued it live — the two verdicts disagree, so the tail
rule fires only over the live lookup (`opDynScope`) and the unapplied-fn
guard declines the body. **The fix** is the frame-scoped computed fn
def's runtime binding — the dynamic-scope bind installing a closure
value (rendered, or as the payload the lookup can call) and tearing it
down with the frame — after which the read's live route closes the first
row natively and the raw-body rows read the name on the island.

## NUR191 — a module fn's body re-steps a parked closure the main registry parks {#nur191}

**Status:** FIXED 2026-09-25 (the module fn's return contract — the
handoff log's entry of that date). RECORDED 2026-09-23 (the placed value
inside a code body — the handoff log's entry of that date), found when
that increment's first draft let `TestModuleFnStampedAtLoadAndRerouted`'s
decliner compile. Present on `main` (a worktree at c268afb) under the
whole-program compile; an error-versus-value divergence, not silent.

**The fix.** The two interpreter paths disagreed twice over, and the
frame path held the rule both times. (1) The COUNT: the spliced frame's
ReturnCheck raises "expected N return value(s), got M" (stepCloseParen);
the CallBoru seam only type-checked the aligned tail
(enforceCallBoruReturns, NUR069) and handed the whole residual back, so
`def d1 fn [[x:Integer][Integer][x 3]]` answered `[10 3]` as a module fn
for the count error the same fn raises on the main registry and as a
compiled module fn. A NAMED call through the seam — execFnDefLiteral's
cross-registry arm (check mode through `CallBoruStrict`, run time through
`InvokeCallbackStrict`, whose fallback is the strict seam) and
buildFnBodyHandler's foreign-registry arm — enforces the frame's count
now, BEFORE the types (`NamedFnReturnCount`: a residual both the wrong
count and the wrong type raises the count error, as the frame does); the
callback seams (InvokeCallbackFn, InvokeCallback) keep the CallBoru
discipline, the count trimmed. (2) The PARK: the handler arm returns the
body's residual as a handler result, which execMatch re-presents to the
main loop from the first argument's index — a returned closure stepped
there dispatched over the values beneath the call (`3 M.d1 10` was 13 for
the main registry's `[3 fn (Integer)]`). A user fn's single returned
closure delivered as a handler result (`sig.FnFrame() != nil`, one value
that would dispatch at the pointer) is stepped past after the splice, as
fnReturnPark steps past a frame's Function return. With both, `M.d1 10`
over `[(mk x) 3]` raises the frame's count error on both lanes, the
decliner's chain with it, and `3 M.d1 10` parks everywhere; the
undeclared-return twin `[[x:Integer][][(mk x) 3]]` is 13 on both, as it
always was. The one preamble that leaned on the leniency — boru:repl's
`repl-eval-line`, whose `st set history …` left the store beneath its
one declared return — drops it now.

**Found on the way, recorded:** NUR210, a module fn returning a NAMED fn
value read through its reach group with a value beneath (`5 M.ff` is 6
interpreted, `[5 fn inc]` compiled): the reach group's collapse re-steps
its named survivor (a name always calls), which the compiled lane seats
as data.

**Rule:** a compiled program answers as the interpreter does; a user fn's
single returned closure is PARKED where it lands (NUR101, NUR124).

| witness (`mk` returns `([n:Integer] => [n add k])`) | interpreted | compiled |
|---|---|---|
| `def d1 (fn [[x:Integer][Integer][(mk x) 3]]) d1 10` | `type_error … got 2 — [fn 3]` | the same |
| `import module [… def d1 (fn [[x:Integer][Integer][(mk x) 3]]) export "M" {d1: d1/v}] M.d1 10` | `[13]` | `type_error … got 2 — [fn 3]` |
| the module decliner's chain `(((A) 1) 2) 3` | `[16]` | the count error |

**The defect, in one sentence (as recorded).** A boru fn defined inside a
module runs its body through `CallBoru` in the captured sub-registry,
whose seam let the residual's COUNT through and re-presented a returned
closure to the caller's tape, where it re-stepped over the token after it
— where the main registry's inline splice parks it and both lanes raise
the count error; the compiled module fn (the whole-program compile stamps
it) parks as the main registry does. The two interpreter paths disagreed
with each other before the compiler was involved;
`TestModuleFnStampedAtLoadAndRerouted` keeps the decliner unstamped at
load (its probe now expects the count error on both interpreters), and
the closure body's park rule (`parkedInBody`) admits only a user fn's
returned closure and a placed fn literal, never a paren-apply's result.
**Fence.** `TestModuleFnReturnContractIsTheFrames` (lang
module_fn_return_contract_test.go): the witnesses and their neighbours
with the same verdict on both lanes (an error by code and detail — the
positions differ by lane, NUR118), the interpreter's own answers, and the
main-registry twins; edge-modules-2.tsv §12 carries the rows.

## NUR190 — a dynamic fn value under a function word its `/q` or Any-typed overload claims {#nur190}

**Status:** FIXED 2026-09-26 (the landing's island — the handoff log's
entry of that date). Before: CONTAINED 2026-09-24 (NUR190's open halves
deferred — the handoff log's entry of that date), the maintainer's call: the `/q`
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
claimed (`landingSkip`; a different count defers). Every placement probed
agrees and compiles — `m.f y` `[y]`, `(m.f y)`, `((m.f y) 3)`, `if true
[(m.f y)] [0]`, `for 1 [(m.f y) drop]`, `[(m.f y)]`, `{a: (m.f y)}`, a fn
body, a callback body — and fn-value.tsv L317/L318 left the runtime-defers
ledger (the defer census 7 -> 5; the two rows run one landing island each,
engine entries 183 -> 185, interp-entry rows 36 -> 38). Probing the class
found NUR222 (a dyn body's settled lead re-applied at the program's
residual), fixed with it. Pinned: lang `TestNamedFnCandidatesOpenShapes`
(fifteen rows, every placement), the lang ledger's bail line 40 -> 37.

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

## NUR189 — the residual's trailing arms applied a paren-placed member fn value {#nur189}

**Status:** FIXED 2026-09-23 (the named fn value's candidates — the handoff
log's entry of that date). Recorded the same day, probing NUR186's
neighbours. Present on `main`, silent, default lane, exit 0.

**Rule:** a paren places its lone survivor (the park rule,
design/PAREN-RESTEP-RULE.0.md).

| witness (`inc` adds 1, `m` is `{f: M.inc}`) | interpreted | compiled |
|---|---|---|
| `7 (m.f)` | `[7 fn inc(Integer)]` | `[8]` |
| `1 7 (m.f)` | `[1 7 fn inc(Integer)]` | `[1 8]` |
| `(7 (m.f))` | `[8]` | `[8]` |

**The defect, in one sentence.** The lead arm asks `leadPlacedNotRead`
before applying a lead, but the trailing arm (`trailingApply`) asked only
the call-result park (`callResultPlaced`), and the two window arms the
same — so a member fn value a user paren had placed applied over the value
beneath it. **The fix:** all three ask `placedNotReStepped`; an enclosing
paren's re-step still undoes the placement (`(7 (m.f))` is 8). The rows
decline at the residual's layout (data above a literal) rather than
compile, which is sound; laying the placed pair out is the follow-on.

## NUR185 — a `/v` read of a def-bound closure re-stepped by the mixed-window island {#nur185}

**Status:** FIXED 2026-09-23 (the trailing value's re-step — the handoff
log's entry of that date), found probing NUR184's neighbours. Present on
`main` (a worktree at 888f234), silent, default lane, exit 0.

**Rule:** a compiled program answers as the interpreter does.

| witness (`mk` returns `mul k z`) | interpreted | compiled (before) |
|---|---|---|
| `def c (mk 3) end 2 c/v 10` | `[2 fn c(Integer) 10]` | `[2 30]` |
| `def c (mk 3) end 2 c/v 10 20` | `[2 fn c(Integer) 10 20]` | `[2 30 20]` |
| `def c (mk 3) end c/v 10` | `[fn c(Integer) 10]` | `[30]` |
| `def c (mk 3) end c 10` (the bare read) | `[30]` | `[30]` |

**The defect, in one sentence.** The `/v` read of a name def-bound to a
computed fn CARRIER (stepWordVal's fn-carrier side-table branch) noted the
def read and the local read but not the VALUE read, so the residual's
`callResultPlaced` took the delivery for the bare read's word dispatch
(its defReads exemption) and the mixed-window arm islanded `[2 fn 10]`
live, where the island's interpreter re-steps the closure the `/v`
spelling had delivered inert.

**The fix.** The carrier branch notes `NoteValRead` as the Defs path does
(core/go/engine.go); the recorder keeps every noted `/v` read program-wide
(`valReadNoted`), and `callResultPlaced` treats a `/v`-only delivery — not
also read bare (core's `WordReadFnIDs`), not under a pending `apply` — as
the parked result it is (`placedValRead`, compiler/go/emit.go). The rows
DECLINE at the residual's render gate: the interpreter renders the value
under the def's name (`fn c(Integer)`), which the raw closure cannot
reproduce (NUR119's rule). Pinned in `def_bound_closure_park_test.go`
(lang/go).

## NUR184 — a paren's trailing fn value forward-collects past the paren's close {#nur184}

**Status:** FIXED 2026-09-23 (the trailing value's re-step — the handoff
log's entry of that date); what does not compile declines. Recorded
2026-09-23 while closing NUR180 (its fold row is this record's). Present
on `main` (`4ee542d`), silent, default lane, exit 0.

**The fix, in one paragraph.** The collapse (stepCloseParen's trailing
arm) records the trailing apply only for a lead the interpreter dispatches
INSIDE the paren — a bare word read of a fn-typed binding (`(5 3 comp)`,
CheckState.WordReadFnIDs, noted by noteWordRead) or a lead the `apply`
word owns — or a fn VALUE nothing after the close can collect
(trailingFnCollectsPastClose: a word, a close paren, an `end`, a modifier,
a literal none of a concrete fn's forward positions takes). A value with a
collectable follower is left for the rewind, and recordParenReStep marks
EVERY fn-valued survivor re-stepped, not the lead alone. A paren under a
pending forward — the eager group evaluator's own close (stepCloseParen's
feedsForward) or a parked Forward still collecting (parenFeedsPendingForward)
— marks its trailing value re-stepped AND as that collection's LEFTOVER
(CheckState.ForwardLeftoverFnIDs): the residual's trailing arm applies it
over what lay beneath, the lead arm never over the values above it, which
a later statement produced (`def r (2 (mk 1)) end r` compiled 3 for `[fn
2]` under the mark alone). And the mixed-window island's interpreter now
SEALS a compiled closure at its collection's completion as it seals a
FnDefInfo (the completion site bridges through fnDefAtPointer): unsealed,
the re-step re-planned forward-first over the NEXT literal and walked the
closure past every follower — `(2 (mk 1)) 10 20` islanded to `[2 10 21]`
(NUR124's payload axis, in the island). After: `(2 (mk 1)) 10` `[2 11]`,
`(2 (mk 1)) 10 20` `[2 11 20]`, `(2 inc2/v) 10 mul` 24, `xs each [(2 (mk
1)) 10]`, the fold row 6, the fn-unit `10 mul (2 (mk 1))` 21 and `10 mul
(2 (mk 1)) 5` `[20 6]` agree; `10 mul (2 (mk 1))` at the main program,
`def r (2 (mk 1)) end r` and `[(2 (mk 1)) 10]` (RecordMakeListInner
declines any re-step-marked element) DECLINE, and so do `(2 (mk 1)) 10
mul` and `(2 (mk 1)) 10 add` (22 and 13 interpreted: the closure takes the
10 before the word runs) — the check pass stepped the marked carrier as
data and the word's poly record collected it, so the compiled program
raised the no-match the interpreter never does; a native poly record that
collects a re-step-marked carrier declines instead (`polyCallDeclineReason`,
compiler/go/emit.go — the record's one compile-failure site, the same
reason string discipline as the binder's). The stack-word twins (`drop`,
`dup`, `swap`, `size`, `eq`) already declined through the collection
hazard (NUR121). The original record follows.

**Rule:** a compiled program answers as the interpreter does.

**The oracle, measured.** A paren whose LAST survivor is a fn value does
not apply it at the close: the value is sealed and dispatched like a word
right after the paren — the interpreter's forward phase takes the tokens
past the `)` while they match the value's parameters, and only with nothing
collectable there does the value fall to the stack, the values inside the
paren. A paren that closes UNDER A PENDING FORWARD (a word before it still
collecting) hands its survivors to that forward in order, and the fn value
left over re-steps over the forward's result.

| witness (`mk` returns `x add n`; `inc2` adds 2) | interpreted | compiled |
|---|---|---|
| `(2 (mk 1)) 10` | `[2 11]` | `[3 10]` |
| `(2 (mk 1)) 10 20`, `5 (2 (mk 1)) 10`, `((2 (mk 1)) 10)`, `[(2 (mk 1)) 10]` | the same shape | the same shape |
| `(2 (mk 1)) 10 mul` | `[22]` | `[30]` |
| `(2 inc2/v) 10`, `(2 inc2/v) 10 mul` | `[2 12]`, `[24]` | `[4 10]`, `[40]` |
| `10 mul (2 (mk 1))`; the same in a fn unit | `[21]` | `[30]`; `[3]` |
| `def xs [1 2 3]  xs each [(2 (mk 1)) 10]` | `[[11 11 11]]` | `[[10 10 10]]` |
| `0 fold [add (2 (mk 1))] xs` (NUR180's fold row) | `[6]` | `[3]` |
| `def r (2 (mk 1)) end r` | `[fn (Integer) 2]` | `[3]` |

Agreeing: `(2 (mk 1))` (3), `(2 (mk 1)) "s"` (`[3 s]` — the String is not
collectable), `(2 (mk 1)) mul 10` (30 — a word follows), `(2 (mk 1)) add
10`, `((mk 1) 2) 10` (`[3 10]` — a LEADING fn collects inside the paren, the
rewind's re-step), a body whose tail is the paren.

**The mechanism.** `recordParenTrailingFnApply` (core/go/engine.go) records
the trailing window `(args… fn)` as one apply EVENT over the values inside
the paren and collapses the tape to its result — the model of the
no-follower case. The interpreter's sealed value looks past the close
first. The record must either look past the close as the interpreter does
(the follower literal is the argument, the inside values stay as residual
— `[2 11]`) or decline when a collectable token follows; and a paren under
a pending forward must hand its survivors to the forward's collection
rather than record its own apply.

**Where it belongs.** The paren re-step rule (design/PAREN-RESTEP-RULE.0.md)
and NUR124's payload axis; the trailing-apply record's first cut. Pinned
by `TestParenTrailingFnAgrees` and `TestParenTrailingFnSoundCompileFailures`
(lang/go, paren_trailing_forward_test.go), and the fold row by
`TestUnnamedFrameApplyResultTyped`.

## NUR183 — a member read at a code body's tail over a duplicated element compiled as data {#nur183}

**Status:** FIXED 2026-09-22 (the quotation-body container reads — the
handoff log's entry of that date). Present on `main` (a worktree at
1ae0a21), silent, default lane, exit 0.

**Rule:** a compiled program answers as the interpreter does.

| witness | interpreted | compiled (before) |
|---|---|---|
| `def ops {inc: (fn [[n:Integer][Integer][n add 1]])} end each [dup ops.inc] [1 2 3]` | `[[2 3 4]]` | `[[fn (Integer) fn (Integer) fn (Integer)]]` |

**The defect, in one sentence.** The reach group `ops.inc` never parks
(NUR173): its collapse re-steps the member over the element beneath, and
the body nets `inc(e)` — where the compiled each body, a CODE-BODY closure
unit that took no whole-frame replay, seated the residual `[e, e, member]`
in order and returned the member as data; `closureResidualHasUnappliedFn`
does not count a gradual top as unapplied, and the layout had nothing to
decline. The corpus rows of the same shape with ONE value beneath the
member (each-variants L205, fold-map-filter L215, module-composition L98)
declined "result above a literal" instead — loud, and the same root.

**The fix.** `noteClosureBodyReplay` (compiler/go/emit.go): a code-body
closure unit whose residual's top is a carrier the check pass tagged as a
fn-valued MEMBER read arms the whole-frame replay (OpCallDynFrame) a named
fn unit already takes for the identical residual — the unnamed inputs are
the resolved prefix, the token region re-steps under execFnDefLiteral's
own rule. Pinned in `TestQuotationBodyMemberReadParity` (lang/go).

## NUR182 — the whole-frame replay re-stepped a paren-placed value {#nur182}

**Status:** FIXED 2026-09-22 (the quotation-body container reads). Present
on `main` (a worktree at 1ae0a21), silent, default lane, exit 0.

**Rule:** a compiled program answers as the interpreter does.

| witness (`ops` as in NUR183) | interpreted | compiled (before) |
|---|---|---|
| `def f fn [[Integer][Any][(ops.inc)]] end f 5` | `[fn (Integer)]` | `[6]` |
| `def f fn [[Integer][Integer][(ops.inc)]] end f 5` | `type_error: return value 1: expected Integer, got Function` | `[6]` |
| `def f fn [[Integer][Any][5 (ops.inc)]] end f 1` | `type_error: expected 1 return value(s), got 2 — [5 fn (Integer)]` | `[6]` |
| `def f fn [[Integer][Any][(ops.inc) 5]] end f 1` | `type_error … [fn (Integer) 5]` | `[6]` |
| `def f fn [[x:Integer][Any][x (ops.inc)]] end f 5` | `type_error … [5 fn (Integer)]` | `[6]` |

**The defect, in one sentence.** `noteDynFrameReplay` counted a paren-PLACED
member read as the window's one applicable and armed OpCallDynFrame, whose
island re-steps every token it is handed — where a fn frame never re-steps
a placed value at all (measured: `[5 (ops.inc)]` and `[(ops.inc) 5]` are the
count error over `[5 fn]` / `[fn 5]`; the only re-step of such a value is
the CALLER's, of a NATIVE's returned fn over the stack beneath the call —
NUR124's machinery, `7 do [(ops.inc)]` is 8 — and a user call's result is
placed there too, `7 f 5` is `[7 fn]`).

**The fix, in three parts** (compiler/go/emit.go).

- *The replay.* A value `placedNotReStepped` marks (paren-placed, no
  enclosing paren's re-step recorded) is data: `noteDynFrameReplay` skips
  it as an applicable — a residual whose only maybe-callable is placed
  compiles with its count mismatch and the RET raises the interpreter's
  own error — and a window that would carry one beside an applicable
  declines (`windowHasPlaced`), since the island cannot honour placement.
- *The layout.* `residualForceOrder` takes a data predicate: a placed
  value no longer bails the out-of-order promotion, so `[l0, fn]` re-pushes
  in order (STORE / PUSH / PUSH) as the fn-value read always did.
- *The `do` exception.* A code body whose driver returns the WHOLE residual
  to the caller's tape (`CallableSpec.BodyOut == BodyOutResidual`,
  `fnUnitRec.residualToCaller`) hands a placed fn with siblings to the
  caller's re-step — `do [5 (ops.inc)]` is 6, and no unit-side layout
  models the re-step over a SIBLING result — so there the bail stands and
  the shape keeps today's decline. Alone (`do [(ops.inc)]`) it parks, and
  the caller's re-step over the outer stack is NUR124's.

**What the fix uncovered.** With the layout no longer declining a placed
carrier, the generated sweep's `apply` factory · lambda-body variant
(`def zzvlam ([] => [5 (mk) apply]) zzvlam`) DIVERGED — the `apply` word's
dispatch over a produced fn-typed carrier inside a unit had been elided
with its application seated nowhere (`recordCallElided`'s pending arm
admitted only produced closures), hidden by the decline. The pending
registration now covers any produced fn-typed carrier inside a fn unit
(`producedFnCarrierInFnUnit`), and the shape declines loudly as before.

**Pins.** `TestPlacedMemberInFnUnitErrorsWithParity`,
`TestQuotationBodyMemberReadParity` (lang/go); `TestNoteDynFrameReplayPlaced`,
`TestResidualForceOrderPlaced` (compiler). One pin graduated:
`TestEdgeFindingDynamicFnValueApplyBodyTail`'s mid-body row, which expected
a decline ("unapplied fn-value in body residual") and now compiles to the
interpreter's own count error with the print in its place.

## NUR181 — a def-bound factory closure applied through the read model: the landing re-steps a placed result {#nur181}

**Status:** FIXED 2026-09-23 (the def-bound closure park — the handoff
log's entry of that date). Recorded 2026-09-22 while probing the curried
chain; measured present on `main` (a worktree at 917ecf0).

**What it was.** Not the landing's: RESTEP_LANDING fires only for a
0-arg-only member, and the disassembly showed the apply as the program
residual's TRAILING arm (`CALL_DYNAMIC_TRAILING /1`) over `[2, result]`,
with the mixed windows (`CALL_DYNAMIC_MIXED`) islanding the wider shapes
live — `7 (r 3) 2` was `[7 6]`, `r 3 4` was `[2 8]`. Every apply arm
already consults the park rule (`callResultPlaced`: a user call's single
returned closure is parked where it lands, NUR101), but the classifier
knew two producers — a named user call (evCallUser) and a user MEMBER's
dyn-method call (`userMemberFn`, read off the def table) — and a shaped
method call over a DEF-BOUND closure resolves to neither: at check time
`r` holds the factory's carrier, not a FnDefInfo, and the method value's
operand is a promoted slot that names no unit. A compiled fn-value
apply's result (`2 ((mk3 1) 3)`, the paren's KEEPQ apply of `(mk3 1)`
over 3) was the same defect one route over: a `wordDynApply` event no
arm read as placed.

**The fix** (compiler/go/emit.go). `RecordDynMethod` resolves the
callee's closure UNIT from the method value's own producer
(`eventProducedFnOp` — a factory call's returned closure, an earlier
apply's) and records it on the event (`emitCall.calleeUnit`); a fn-value
apply names it through its closure operand. `callAppliedClosureUnit`
reads both, `callResultPlaced` admits an evCall whose callee is a
compiled closure (a boru fn: fnReturnPark parks what it returns), and
`callResultRenderKnown` renders the parked value as that unit's single
result does. Finalize's program-residual ordering then treats a parked
result as the data it is — its gradual out carrier (a closure unit's
count-contract Any) had read as an apply lead and suppressed the
reorder, which is why `5 (mk 3)` had declined "call result above a
literal" all along; a result the `apply` WORD claims (appliedByWord)
keeps the boundary. The shuffles the interpreter re-steps at the shuffle
(`5 (mk 3) 1 roll`, NUR124's timing axis at the main program) keep their
own declines. Pinned in `def_bound_closure_park_test.go` (lang/go);
compiler `TestCallAppliedClosureUnit`.

**The original record follows.**

**Rule:** a compiled program answers as the interpreter does.

| witness | interpreted | compiled |
|---|---|---|
| `def mk3 fn [[a:Integer][Function][( fn [[b:Integer][Function][( fn [[c:Integer][Integer][a add b add c]] )]] )]]  def r ((mk3 1) 2) end r 3` | `[2 fn (Integer)]` | `[6]` |
| the same with `(r 3)` | `[2 fn (Integer)]` | `[6]` |

**The mechanism.** `def r ((mk3 1) 2)` does NOT apply the closure: the
paren closes on behalf of the def's pending forward collection, which takes
the survivors `[closure, 2]` as its ARGUMENTS — r binds mk3's closure and 2
stays on the stack (core `stepCloseParen(false)`; the same rule makes
`def r ((mk 1) 2) end r` answer 3 by applying r to the leftover). `r 3` is
then a shaped method call (the FnShapes claim, `CALL_DYN_METHOD r/1`) whose
result is a closure. The interpreter PARKS a call result where it lands
(`fnReturnPark`, NUR101's "place uniformly"); the compiled lane's
`RESTEP_LANDING` applies it to the 2 beneath (`CALL_DYNAMIC_TRAILING /1`).

**Where it belongs.** The re-step landing's claim (NUR173–NUR175): a shaped
method call's closure RESULT is placed, not re-stepped, and the landing
must not seat over it. Pinned by `TestDefBoundFactoryClosureLandingPending`
(lang/go), which fails the day the rows agree.

## NUR180 — a trailing paren apply's Any result inside an unnamed-param frame is re-matched over the frame's input {#nur180}

**Status:** FIXED 2026-09-23 (the recovery's window — the handoff log's
entry of that date) for the mechanism this record names; MITIGATED
2026-09-22 (the curried chain) for a named concrete lead. Present on
`main` (a worktree at 917ecf0), silent, default lane. The fold row of the
table below (`0 fold [add (2 (mk 1))] xs`) is NOT this mechanism's: its
paren closes UNDER `add`'s pending forward, which is NUR184's record.

**The fix.** The checker's unmatched-dispatch recovery
(`checkModeAssumeSig`, check/go/check_recovery.go) gathered its assumed
window from the STACK first and filled any shortfall from the tokens after
the word — the opposite of the interpreter's matcher, whose forward phase
takes the written tokens for the forward-eligible leading positions FIRST
and the stack for the rest. Inside an unnamed-param frame the frame's input
sits on the stack beneath the result, so a two-arg word found two stack
values and never looked at the written `10`; in a named frame it found one
and filled the shortfall forward, which is why only unnamed frames
diverged. `checkModeFallbackPositionsFor` now lays the window out
forward-first per candidate signature under the word's modifiers
(`core.EffectiveForwardLimit`: the barrier, `/s`, `/f`), a written token
filling its position while it is compatible (a type match, an Any
carrier, a raw word), and hands the stack run's length to `SigOrderArgs`.
`xs each [(2 (mk 1)) mul 10]` is 30 and the lambda row 40 on both lanes;
`TestUnnamedFrameApplyResultTyped` holds them. One pin moved with it:
`g2 k v` over a speculative undef (spec_undef_placed_test.go) now
recovers over the interpreter's window `[k, v]` and declines at the arm
plan's operand instead of at the generic seat — still declined, still the
same hatch.

**The original record follows.**

**Rule:** a compiled program answers as the interpreter does.

| witness | interpreted | compiled | after the mitigation |
|---|---|---|---|
| `def inc2 fn [[x:Integer][Integer][x add 2]]  def xs [1 2 3]  xs each [(2 inc2/v) mul 10]` | `[[40 40 40]]` | `[[10 10 10]]` | agrees |
| `def inc2 … def f fn [[Integer][Integer][(2 inc2/v) mul 10]]  f 1` | `[40]` | `[10]` | agrees |
| `def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]]  def xs [1 2 3]  xs each [(2 (mk 1)) mul 10]` | `[[30 30 30]]` | `[[10 10 10]]` | open |
| `def xs [1 2 3]  xs each [(2 ([x:Integer] => [x add 2])) mul 10]` | `[[40 40 40]]` | `[[10 10 10]]` | open |
| `def mk … def xs [1 2 3]  0 fold [add (2 (mk 1))] xs` | `[6]` | `[3]` | open |

The same rows inside a NAMED frame (`def f fn [[a:Integer]…]`, a
`([e:Integer] => …)` callback) and at the top level agree; only an
UNNAMED-param frame — an each or fold body, `fn [[Integer] …]` — diverges.

**The mechanism.** The trailing arm's result carrier is a strict Any
(`recordParenTrailingFnApply`), and a typed word cannot match it
statically. In a named frame the checker's recovery still lands on the
written argument. In an unnamed frame the frame's gradual INPUT sits on the
stack beneath the result, and the recovery re-matches the word over that:
the disassembly shows `mul/2 (poly)` over `[l0, out]` with the written `10`
pushed AFTER the call, so each body nets `10`.

**The mitigation.** A NAMED concrete lead's one declared return types the
result (`concreteFnSingleReturn`, core/go/engine.go): `inc2/v` declares
Integer, the interpreter enforces it, so `mul` matches statically. An
ANONYMOUS fn value's declared return is a count contract only
(`LambdaCountContract`, NUR120) and an event lead's closure unit records
Any for the same reason, so those results stay Any and the rows stay open;
the curried-chain arm (NUR178) stands aside inside an unnamed frame for the
same reason (`ProducedLeadApplies`), which keeps `xs each [((mk 1) 2) mul
10]` on the whole-frame replay that answers it correctly.

**Where it belongs.** The checker's recovery for a strict Any result under
an unnamed frame — the arm that admits the frame's input into a window the
written argument should fill. Pinned by `TestUnnamedFrameApplyResultTyped`
(the mitigated rows) and `TestUnnamedFrameApplyResultAnyPending` (the open
rows, which fails the day they agree).

## NUR179 — the fn-value-call ops bound a two-param closure backwards {#nur179}

**Status:** FIXED 2026-09-22 (the curried chain). Present on `main` (a
worktree at 917ecf0), silent, default lane, exit 0.

**Rule:** a call binds its arguments in signature order — the value stack
in reverse, top first (CLAUDE.md's one rule).

| witness | interpreted | compiled (before) |
|---|---|---|
| `def mk2 fn [[n:Integer][Function][( fn [[x:Integer y:Integer][Integer][(x mul 10) add y add n]] )]]  (2 3 (mk2 1))` | `[33]` | `[24]` |
| `… 7 5 (mk2 1)/v apply` | `[58]` | `[76]` |
| `def kk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a mul 10) add b add k])]]  1 2 (kk 7) apply` | `[28]` | `[19]` |
| `… def f fn [[g:Function][Integer][(2 3 g)]]  f (mk2 1)` | `[33]` | `[24]` |
| `… ((mk2 1) 2 3)` (the residual arm) | `[24]` | `[33]` |
| `… def m {g: (mk2 1)}  (m.g 2 3)` (the method op; `m.g 2 3`, `((m 'g' get) 2 3)` and a list element's `(fs.0 2 3)` alike) | `[24]` | `[33]` |

**The defect, in one sentence.** `callDynTrailTop`, `callDynApply`,
`callDynMethod` and `callDynamic` build their window in POSITIONAL order (args[0] → the first
param — the trailing ops read the stack top-down, the leading op reads up
from the fn) and hand it to `invokeClosure`, the TOKEN seam, whose fn-value
arm (S1b-2, `invokeFnValueClosure`) takes the STACK order a native hands
it and reverses it itself — so a positional window was reversed twice.

**Why nothing saw it.** Every fn-value-call pin was a one-argument window,
where the two orders coincide, or a commutative body
(`TestTopLevelApplyParity`'s `(a add b) add k`).

**The fix.** `invokeClosurePositional` (eng/go/vm.go): for a fn-VALUE
closure the positional window is re-stacked before the seam reverses it; a
callback body unit's closure keeps the seam's positional binding as it is.
The four call sites take it. Pinned by `TestFnValueApplyBindingOrderParity`
(lang/go, thirteen spellings with an order-sensitive body) and
`TestInvokeClosurePositional` (eng/go). A capture-FREE lambda member
(`{g: ([x:Integer y:Integer] => …)}`) never showed it: a const, not a
closure payload, it never reached the seam.

## NUR178 — a re-stepped produced lead's window leaked to a later dispatch {#nur178}

**Status:** FIXED 2026-09-22 (the curried chain — the handoff log's entry of
that date). Present on `main` (a worktree at 917ecf0), silent, default lane,
exit 0, `boru check` clean.

**Rule:** a compiled program answers as the interpreter does.

| witness | interpreted | compiled (before) |
|---|---|---|
| `def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]]  ((mk 1) 2) mul 10` | `[30]` | `[21]` |
| `def mk fn [[n:Integer][Function][( fn [[x:Number][Number][x add n]] )]]  ((mk 1) 2.5) mul 2` | `[7.0]` | `[6.0]` |
| `… ((mk 1) 2) add 10` | `[13]` | `[13]` — by arithmetic |
| `… ((mk 1) "s")` | `[fn (Integer) s]` | `[s fn (Integer)]` |

**The defect, in one sentence.** The paren re-step rule
(design/PAREN-RESTEP-RULE.0.md) applies `(mk 1)`'s closure to 2 at the outer
paren's close, but the check pass stepped the carrier past as data, `mul`
then collected the 2, and the residual classifier's leading-carrier arm
applied the closure over mul's PRODUCT (its arity, one, matched the one
value after it):

```
0001 PUSH_CONST  k1   ; 1
0002 CALL_USER   f0   ; mk/1
0003 PUSH_CONST  k2   ; 2
0004 PUSH_CONST  k3   ; 10
0005 CALL_NATIVE s0   ; mul
0006 CALL_DYNAMIC /1
```

NUR121's collection-hazard mark WAS set when `mul` took the 2 — and
`hazardLead` exempted the lead on the INNER paren's placed mark
(`parenPlacedMemberFn`), whatever the outer paren had done to it.

**The fix, in two halves.**

- *The record.* The collapse records the re-stepped produced lead's apply
  as the SAME event the trailing spelling seats (`RecordDynApplyLead`),
  substituting the event's out for the lead — core's
  `parenProducedLeadApplyIdx` / `recordParenProducedLeadApply`, over the
  new `ProducedLeadApplies` seam: the compiler vouches that the closure a
  produced value holds (a compiled factory's returned closure, or a
  recorded apply's result — the chain's next level, `eventProducedFnOp`)
  takes EXACTLY the window's arguments (declared arity, no patterns, each
  static type conforming), and types the result. The chain `(((mk3 1) 2)
  3)` (callbacks.tsv L151) is three events. Off the main loop — a collapse
  on behalf of a pending forward collection, `a add ((mk 1) e)` — nothing
  re-steps and the arm stands aside (`reStepped`).
- *The hazard's order.* `NoteCollectionHazard` records whether the lead
  was ALREADY re-stepped when the collection marked it
  (`hazardAfterReStep`): a lazy lead's own collection inside the paren
  (`((mk) 7 add 1)` is 9 on both lanes) stays admitted, a collection past
  the apply the interpreter already performed declines (NUR121's text).

**The sibling.** The leading form's NO-MATCH: `callDynamic` handed the
closure to the token seam, whose no-match fallback re-steps the value
TRAILING and answered `[s fn]` for the interpreter's `[fn s]`; a fn-value
closure the window does not match is now left as written.

**Pins.** `TestCurriedChainParity` (twenty-seven rows), `TestCurriedChain
SoundCompileFailures` (the NUR121 decline for a gradual argument, the
arity mismatch, the def-bound chain's read-model decline),
`TestCurriedChainPendingCollection`; core `TestS5BCloseParenProducedLead
Apply*`; compiler `TestProducedLeadApplies`, `TestEventProducedFnOpArms`,
`TestFnOpContract`, `TestArgConformsStatically`. Three pins graduated:
`TestCurriedFactoryCompiles`'s three-level fence,
`TestFnValueAutoApplyCompileFailures`'s nested-factory row and NUR101's
`[((mk 1) 2)]` list-element fence.

**Measured.** See the handoff log's curried-chain entry (2026-09-22).

## NUR177 — an undeclared-return fn called twice seats one result for both calls {#nur177}

**Status:** FIXED 2026-09-22. **Found by S1b's apply shapes** (the handoff log's
entry of that date): the literal-read pin `def g f:Function => [(f 3)] end def
w n:Integer => [def kk (fn r:Integer Any [mul n r]) if (n gt 0) [(g kk/v)]
[0]] end (w 5) (w 0)` graduated from "then-branch result of unknown
provenance" and answered `[0 0]` for the interpreter's `[15 0]`. Reduced and
measured on a worktree at the previous commit: present on `main`, on the
default lane, exit 0, with no fn value anywhere in it.

**Rule:** a compiled program answers as the interpreter does.

**The defect, in one sentence.** The undeclared-return branch of
`BuildFnBodyReturnsFn` hands `AnalyseFnBody`'s MEMOISED residual — the same
`[]Value` for every call of one argument shape — to `RecordUserCall` as the
call's results, and the recorder keys a value's producer by its ID.

| witness | interpreted | compiled (before the fix) |
|---|---|---|
| `def g x:Integer => [x add 3] end def w n:Integer => [(g n)] end (w 5) (w 0)` | `[8 3]` | `[3 3]` |
| `def w n:Integer => [if (n gt 0) [n add 3] [0]] end (w 5) (w 0)` | `[8 0]` | `[0 0]` |
| `def w n:Integer => [n] end (w 5) (w 0)` | `[5 0]` | `[0 0]` |

The second call's event became the producer of BOTH results, both resolved to
one frame local, and the program's residual pushed it twice:

```
0006 STORE_LOCAL l0
0007 STORE_LOCAL l1
0008 PUSH_LOCAL  l0
0009 PUSH_LOCAL  l0
```

**Why the corpus never saw it.** The declared-return branch above it mints a
fresh carrier per declared return on every call (`core.NewCarrier(t)`), so
the verbose `fn [[…] [T] …]` spelling — nearly every corpus fn — was never
exposed. Only a fn whose result IS its residual (a `=>` lambda def-bound, a
0-return `fn`) takes the memoised values as its outs, and the corpus's
lambda-def rows call each such fn once.

**The fix.** `freshResidual` (check/go/check_fnbody.go): a shallow copy of
each residual value under a fresh ID, per call, at the record — the payload is
the same value on every call, only the identity is the call's own. It is the
undeclared path's equivalent of what the declared path always did.
`TestUndeclaredFnRepeatedCallsParity` (lang/go) pins six shapes on both lanes,
including a pass-through body and two closures of one factory.

**What it does not touch.** `spliceAnonCheckResult` — the DIRECT dispatch of
a lambda value in check mode — splices the same memoised residual and records
no user call; nothing keys on those identities today, and it is left as it is.

**Measured.** See the handoff log's S1b apply-shapes entry (2026-09-22): the
ledgers that moved are that increment's, and this fix moved none of its own.


## NUR176 — a 0-arg runtime lead under the one-arg window fires interpreted and no-matches compiled {#nur176}

**Status:** FIXED 2026-09-25 (the 0-arg lead's window — the handoff log's
entry of that date). Recorded 2026-09-22 while landing S1b's apply shapes.

**The fix.** The window op (`callDynTrailTop`) asks the screen the re-step
landing already asks (`FnValueOnlyZeroArgSigs`, NUR175) of a NAME-read
lead: one whose only overloads take no argument is not a no-match — the
interpreter dispatches the bare read as a WORD where it stands, the fn
fires over nothing, and the window's other tokens are then stepped on
their own. The op hands such a window to the island in its WRITTEN order
— the op's head carries a `Leading` bit now (`DynApplyHead.Leading`, set
by `RecordDynApplyLead`), so a leading window's arguments follow the lead
(a fn value dispatching over the result, a literal landing beside it) and
a trailing window's precede it (they stay beneath the result) — with the
lead marked applied (`FnDefInfo.Applied`) so the island's re-step
dispatches an anonymous 0-arg value exactly as the word dispatch did. The
island IS the interpreter's residual semantics over that window, which is
why the fix is one arm in the op and not a second dispatch path; an
event-produced or `/v`-delivered lead has no name to dispatch under and
keeps the no-match. Every row below agrees now: `(k inc/v)` is 8, `(k 5)`
the frame's count error on both lanes, `(g 3)` over a 0-arg lambda the
same, and `(1 2 k)` under a three-return contract `[1 2 7]`.

**Rule:** a compiled program answers as the interpreter does.

**The non-uniformity (as it stood).** The leading one-arg window `(k x)` over a fn-typed
carrier `k` lowers to `OpCallDynTrailTop` over `[x, k]`: the runtime lead is
matched over its one argument, and a lead none of whose overloads takes an
argument raises `signature_error` (NUR107's rule). The interpreter steps the
lead FIRST: a 0-arg fn dispatches with nothing, its result lands on the
stack, and the argument token is then stepped on its own.

| witness (`h` applies its `k:Function` param to one argument) | interpreted | compiled |
|---|---|---|
| `def z fn [[] [Integer] [7]] end def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h z/v` | `8` — z fires, inc applies to the 7 beneath it | `signature_error` |
| `def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer] [(k 5)]] end h z/v` | `type_error` — `[7 5]` fails h's one-return contract | `signature_error` |
| `def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k ([] => [9]))]] end h app/v` | `type_error` — the 0-arg lambda fires inside app's `(g 3)` | `signature_error` |

The second and third rows are PRE-EXISTING: a literal argument under an
eligible lead compiled before this increment, and
`TestLeadApplyArityMismatchParity` pins the PARAM-argument spelling `(g x)`
at `signature_error` on both lanes — there the interpreter's stranded-forward
barrier raises for the word token where a literal is simply left as residual.
The first row arrived with the admission of an inert fn VALUE at the argument
position (`RecordDynApplyLead`): under every 1-arg and 2-arg runtime lead the
two lanes agree (a Function or Any param binds the value, any other param
no-matches on both), and the 0-arg lead is the one arity where the
interpreter's answer is a VALUE. Compiled it is loud, never a value of its
own — `TestApplyShapesZeroArgLeadIsLoud` pins all three rows on both lanes
and re-opens this record if the oracle moves.

**What a fix looked like (as recorded), and what it was.** The window op
would have to know, at run time, that the lead's only overloads are 0-arg
— the screen `FnValueOnlyZeroArgSigs` already asks at the re-step landing
(NUR175) — and then run the interpreter's residual semantics: fire the
lead, re-step the argument over the result. That is what landed, as one
change to the window op covering the trailing window `(1 2 c)` too, and
not as an arity exception at the record (ADR-016): the island already runs
those semantics over a token window, so the op orders the window as it was
written and hands it over.

**Fence.** `TestApplyShapesZeroArgLeadResolves` (lang apply_shapes_test.go):
the three rows and their neighbours — a two-return contract, the trailing
window, a fn value that takes the fired result — with the same verdict on
both lanes, and the interpreter's own answers.


## NUR175 — the landing matches an empty window where the interpreter matches the tape and the stack {#nur175}

**Status:** FIXED 2026-09-21. **Found by a Codex review of PR #479**, on the
commit that widened NUR174's gate — before the widening could reach `main`.

**Rule:** a compiled program answers as the interpreter does.

**The defect, in one sentence.** `OpReStepLanding` applies the runtime value
over an EMPTY argument window, but the step it models — `execFnDefLiteral` —
matches over the LIVE TAPE and the LIVE STACK, so a member with any arg-taking
overload can be matched differently in the two places.

It has neither, and cannot be given them: the tokens written after the read
compiled into LATER OPS, and consuming a stack operand would leave the stack
shallower than the lowering predicted. So the landing is faithful only where
the interpreter's own match at that point would ALSO be empty.

**Three divergences, each measured on both spellings:**

| witness | interpreted | landed |
|---|---|---|
| `h` = `[] -> 42` and `[n:Integer] -> n add 1`; `5 m.f` | `6` — the unary takes 5 off the stack | `5 42` — the nullary fired over nothing |
| `h` = `[] -> 42` and `[x:Atom/q] -> x`; `m.f z` | `z/q` — the `/q` slot CAPTURES the word | `42 z` — the nullary fired one token early |
| `h` = `def h fn [[] [] []] end`; `5 m.f` | `5` — applied for its EFFECT | `internal_error`: "returned 0 values where the read's recorded landing claims one" |

**All three are on `main` today**, through the reach spelling, and were
introduced with NUR173's landing. NUR174's widening would have carried each to
the `get` spelling as well — where the residual apply had been answering them
CORRECTLY all along. That is the part worth keeping:

> A fix that widens a model widens its holes with it. The measurement that
> shows the fix works does not show what the model was previously standing
> aside from.

The corpus did not catch them: every fn-value witness it carried had a member
with exactly one 0-arg signature and one return, so the three holes had no row
that could tell the difference. `m.f/v` was the same shape of miss in NUR174.

**The fix, and why it belongs in the VM.** Both screens are properties of the
RUNTIME VALUE, which is the only thing that knows its own overload set:

1. `FnValueOnlyZeroArgSigs` — no overload can take the stack operand, and none
   can quote-capture the following word. This settles the first two at once.
2. the matched signature declares exactly ONE return — a 0-return member is
   applied for its effect and leaves the stack as it found it, which the
   landing's one-result claim cannot express.

Standing aside costs nothing: the read keeps today's RESIDUAL APPLY, which is
what answered all three correctly before the landing existed. No compile
failure is added and no ledger moves.

**What it does NOT reach**, and this is a stand-aside rather than a fix: a
member with mixed overloads read with nothing after it and nothing on the
stack (`m get 'f'` where `h` is `[] -> 42` and `[n:Integer] -> n add 1`) is
`42` interpreted and `fn h(Integer)` compiled. Identical on `main`; the
landing declines it because it cannot rule the unary out, and the residual
apply leaves it as data.

**Measured.** All four corpus ledgers, diagnostic parity, runtime defers and
the generated sweep unchanged. Six new rows in `lang/spec/fn-value.tsv` §10 —
one per witness on each spelling — and two of them cost an interpreter entry
each, because the 0-return stand-aside islands: the ratchets rise 2 with the
rows that moved them, both named, on a seam the census already carries.


## NUR174 — the re-step landing was recorded per PRODUCER, so the same read written as a word call was never seen {#nur174}

**Status:** FIXED 2026-09-20. **Corrects the SEAT of** [NUR173](#nur173)'s fix,
whose mechanism was right and whose recording site was a whitelist.

**Rule:** a compiled program answers as the interpreter does.

**Divergence** (reachable on `main` at `b39be40` by plain `boru run`, with
NUR173 already merged):

```
def h  fn [[] [Integer] [42]] end
def mk fn [[] [Map]     [{f: h/v}]] end
def m (mk) end
m get 'f'
  interp:    42
  compiled:  fn h        <- silent
```

`m.f` was fixed the day before. `m get 'f'` is the SAME READ written as a word
call, and it answered wrongly, because NUR173 recorded the landing at the
REACH-GROUP COLLAPSE and a `get` call has no collapse. A dispatch result is
spliced at the pointer and `stepLiteral` re-steps it there, with nothing to see
it.

**The seat, and why a producer list is the wrong shape.** NUR173 put the fact in
`CheckState.ReachReSteppedFnIDs`, written by `recordReachGroupReStep` at the
collapse and read by `noteReStepLanding` in `stepLiteral`'s model chain. But
`noteReStepLanding` is CALLED FROM `stepLiteral`, in the branch whose very next
act is `execFnDefLiteral` on a Function value — and a value the loop PARKED
never reaches it, because parking moves the pointer past. The model was already
standing where the interpreter stands. Asking a side table who had put the value
there added nothing and could only ever be as complete as the shapes measured so
far.

> A model that stands where the decision is made does not need to be told who
> brought the value. A whitelist of producers is a list of the cases someone
> thought of.

So the gate is now the value's own callability, the apparatus is deleted —
`ReachReSteppedFnIDs`, `recordReachGroupReStep` and the core-side plumbing — and
`m.f` and `m get 'f'` are one case rather than two.

**What it cost, measured before it was believed.** Widening emitted 20,710
landing ops over one compile of every spec row, against 4,363 for the
reach-only gate. A narrower alternative was then BUILT — recording at
`spliceMatchResults`, the site NUR173 itself predicted — and measured at
20,495: a **1%** saving, because nearly every fn-typed carrier the pass steps
arrived from a dispatch splice. The economy argument evaporated on measurement,
and with it the only reason to keep a producer list.

**Three rungs of `execFnDefLiteral` the landing had to mirror.** Each was found
by a probe written against the interpreter's own source rather than by running
the corpus, and each was a wrong answer on its own. None is a consequence of
widening: the first was reproduced under the NARROW gate too, which is what
settled the design.

| rung | the interpreter's rule | what the landing did without it |
|---|---|---|
| **anonymous-0-arg park** | a lambda / macro VALUE that matched nothing is DATA, so `def f ([] => [body])` binds the function; a NAMED 0-arg fn dispatches | applied the wrapper, so `def p (FnUtil.partial f/v 10) end (p)` met an Integer where it expected a function (`module-fn.tsv:L47`) |
| **dispatch modifier** | a `Word/__DM` marker after the value states DATA intent and is honoured by QUOTING | `m.f/v` answered 42 against the interpreter's `fn h` — a call landed on the one read written specifically not to be one |
| **alone in a LIVE reach group** | the group's job is to produce the value; the call belongs to whatever encloses it (NUR035) | landed one token before the `/v`, where the close paren reads as a boundary |

The park is mirrored in BOTH representations a lambda arrives in: `FnDefInfo.
Anonymous` for the interpreter's own value, and `CompiledFn.Lambda` — which is
where `closureFnDef` reads that flag from — for a compiled closure. A first
draft screened the closure arm for zero-arg matchability instead; that was
SUBSUMED, because `ClosureIsFnValue` already implies `Lambda`, and keeping both
would have left dead code behind a correct answer.

**Measured cost.** Every gate at or below its ceiling and **zero regressions**:
the four corpus ledgers (53 / 284 / 32 / 111 / 52), diagnostic parity at 351,
runtime defers at 8, the interp-entry census at 78, and the generated sweep
diffed against a pre-change baseline with no cell moved.

**Engine entries: the park removes two, and NUR175's witnesses add two back.**
The park takes `bytecode-migrated.tsv:L285` and `callbacks.tsv:L150` off the
interpreter — curried chains whose 1-param wrapper the landing invoked, had
`invokeFnValueClosure` decline, and paid a `RunResolved` entry to step the body
to the same "stays data". They already compiled correctly: `module-rand.tsv:
L16`'s trade one increment on. The ceiling touched 420 inside the branch and
came back to 422 when NUR175's two 0-RETURN witnesses were added, because the
landing stands aside from those and their residual apply islands. Net against
`main`: unchanged at 422, with two rows of interpretation swapped for two rows
of proof.

What proves the fix is **twelve new rows in `lang/spec/fn-value.tsv` §9** —
nine covering the `get`-word family at every arity, and one per rung above.

**What remains.** Of NUR173's list, the `get`-word twin is closed. Still open:

- **A collectable token written after the survivor.** The landing stands aside
  rather than islanding the window the interpreter's forward collection would
  take (`OpCallDynamicMixed`'s shape).
- **A VARIADIC producer's region top**, whose re-step belongs to the
  mark-window machinery (`OpCallDynMixedFromMark`).
- **The sweep's two `def container` CRASH cells** (`CALL_DYNAMIC underflow`
  under `fn-body`, `SWAP underflow` under `module-body`), unmoved by this
  increment.
- A **pre-existing compile failure** this increment measured but did not widen
  into: a container member that is an ANONYMOUS lambda declines with "0-arg
  landing not modelable at fn value" on both the `.` and the `get` spelling
  (`def ml {f: ([] => [9])} end ml.f`). Identical at `b39be40`; it is the
  shaped-method guard's decline, not the landing's.


## NUR173 — a reach-lowered group's lone survivor is re-stepped interpreted and pushed as data compiled {#nur173}

**Status:** FIXED 2026-09-20 for the reach-group family (`OpReStepLanding`).
What it does not yet reach is named at the end.
**Supersedes the diagnosis of** [NUR169](#nur169), whose MECHANISM was right
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
   **SUPERSEDED the next day by [NUR174](#nur174)**, which found the fact is a
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

- ~~**The `get`-WORD twin.**~~ CLOSED 2026-09-20 by [NUR174](#nur174) — and
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


## NUR169 — a one-value paren applies a function interpreted and skips it compiled {#nur169}

**Status:** SUPERSEDED 2026-09-20 by [NUR173](#nur173), which FIXED it. Its
MECHANISM below is right — every fn-value-call arm needs a second residual
entry, so a one-survivor collapse reached none of them. Its SEAT is one
function out: the live half is `recordParenReStep`'s reach-group exclusion
plus `resolveDynamicApply`'s `len(residual) >= 2`, not the switch named here.
Read NUR173.

**Original status:** Pending (recorded 2026-09-19).
**Found:** a Codex review of PR #475, whose `CompileStoresFn` declaration
retired the gate that had been masking this — the programs that reach it
did not compile at all before, so the defect had nowhere to show. The
declaration is reverted; this is the defect that blocks it.

**Rule:** a compiled program answers as the interpreter does.

**Divergence.** Two witnesses, both silent — the program compiles and
answers wrongly:

```
def h fn [[] [Integer] [42]] end def m ({} set 'f' h/v) end (m.f)
  interp:    42
  compiled:  fn h

def f fn [[] [List] [def fns [] end for [0 2] [def fns (push ([] => [i]) fns)] end
                     each ([g:Function] => [(g)]) fns]]  f
  interp:    [0 1]
  compiled:  [fn g fn g]
```

The second is NOT a lost capture, which is how it first reads: the
non-capturing twin `([] => [7])` diverges identically, `[7 7]` against
`[fn g fn g]`. Both are the same defect.

**Where it is.** `stepCloseParen`'s recorder switch (core/go/engine.go)
classifies a paren window three ways: a TRAILING concrete fn applied to
the values before it, the Stage-G leading one-arg fn-carrier, and a
LEADING dynamic with `count >= 2`. There is no `count == 1` case. The
interpreter auto-applies a lone appliable value at the paren; the recorder
records the read that produced it and nothing else, so the lowering emits
the member read and no call. The disassembly is the proof — the working
control `def m {f: h/v} end (m.f)` emits `CALL_DYN_METHOD h/0 -> 1`, and
the diverging twin emits the `dot` and stops.

**Why the obvious fix is wrong.** Refusing whenever a one-value paren
holds something that could be a function at run time was built and
measured: it fixes both witnesses and then stops far more from compiling,
because a dot access is ITSELF expanded to a paren group whose single net
value is a dynamic `Any` carrier (`expandReach`). `0 fold reg.f [1 2 3]`
stopped compiling. Trading wrong answers for programs that do not compile
is a lesser defect, not a fix — the bar is compiling, not merely being
right. So the real fix has to tell a user-written apply paren from an
internal expansion and LOWER the apply; narrowing the gate instead is at
best scaffolding to stop the bleeding while that is built.

**Where it belongs:** the fn-value lowering half S1b owes — the same
family as NUR156 (an apply that did not fire; fixed 2026-09-22). It is what must land before
`set` / `push` / `unshift` / `append` can declare CompileStoresFn, which
is worth six corpus rows (compile failures 53 → 47, compute gaps 49 → 42)
and is otherwise sound: the declaration itself is right, a word that
STORES what it is handed never puts it back on the tape. Until both land,
those seven programs do not compile, which is seven open defects and not a
position the compiler has chosen.

## NUR168 — a non-capturing factory-built fn value, def-bound, escapes anonymous on the compiled lane {#nur168}

**Status:** FIXED 2026-09-25 (the def's name on the value — the handoff
log's entry of that date). Recorded 2026-09-19 by the S1b-2 seam rows,
measuring the top-level computed fn values that compile for the first
time.

**The fix.** One rule at the bind, as the record asked: RecordDynBind
seats the def name on a factory's baked const lambda
(`producedConstLambda`) exactly as it seated it on a produced closure,
and the VM's store-time rename (`nameClosureValue`) renames a bare
`FnDefInfo` as well as a `ClosurePayload`; the root write-back names the
value it writes too. Two neighbours on the same value, found while
closing it and closed with it: the root's dyn-scope bind and global
write-back each installed the value — two entries under `f`, which
`Registry.Lookup` unions, so `do [f/v]` read `fn f(String) or (String)`
— and the write-back now ADOPTS its paired install
(`GlobalBindSpec.AfterDynScope`: the entry comes off the dyn-bind trail
and nothing is pushed); and a bare-word call of the bound value inside a
code body (`do [f 'z']`) lowered its callee read to the dispatch lookup,
which defers on a fn definition ("vm:dyn-scope-dispatching", an internal
error for the interpreter's `z`) — `RecordDynMethod` takes the
data-position lookup for it, as the closure-tail apply already did.
Pinned by lang `TestDefBoundFnValueIsNamed` and fn-value.tsv §14.

**Rule:** a compiled program answers as the interpreter does.

**Divergence.**

```
def mk fn [[k:Integer][Function][([s:String] => [s])]] end def f (mk 1) end each f/v [1 2 3]
  interp:    [fn f(String) fn f(String) fn f(String)]
  compiled:  [fn (String) fn (String) fn (String)]
```

`f`'s value matches no element, so each's data fork pushes the value
itself per element (NUR155's rule, both lanes). The interpreter's `def`
names the fn value it binds — installDef sets the FnDefInfo's Name and
registers its signatures under `f` — so the escaped value renders
`fn f(String)`. The compiled lane's value-def lowering (STORE_LOCAL +
BIND_GLOBAL) binds the value as it came out of `mk`: a NON-capturing
lambda is a const fn value with no name of its own. A CAPTURING twin
(`[n:Integer] => [n add k]`) renders `fn f(Integer)` on both lanes: its
PUSH_CLOSURE carries the `/v` read's DefName (nameClosureValue, the
thirty-third increment). Render-only: the value applies identically,
its errors already name it (checkFnValueReturn reads the sig's own
name), and the shape failed to compile before S1b-2 (unmatched
dispatch at each), so the divergence is newly reachable, not new.

**Where it belongs:** the value-def lowering's fn arm — a def of a fn
value names the value as installDef does, whatever its representation
(const, closure, bridged) — one rule at the bind rather than one per
representation, which is S1b's "one convention" applied to the name.

## NUR167 — a type-minting fn body, applied as a callback, conflicts one call early under compilation {#nur167}

**Status:** FIXED 2026-09-25 (the analysis is not a call — the handoff
log's entry of that date). Recorded 2026-09-19 by the S1b seam pins,
probing which callback bodies the lazy detached stamp must never compile.

**The fix.** The conflict is a NAME-PART reservation (`IsKnownPart`), not
the minted node: a fn-body `def T` reserves `T` for the registry's
lifetime — the frame's teardown pops the binding and keeps the part, which
is why the interpreter's second call conflicts and why `undef T` inside the
body does not help. The check pass's analysis of the body reserved the
part too, and the run's first call — the interpreter through the callback
seam — collided with it; a single call (`each f/v [1]`) raised where the
interpreter answered. Now `RunFnBodyOnce` snapshots the reservations and
forgets, at its unwind, every part the body reserved for a binding it
popped (`core.ForgetTypePartsSince`; the node stays in the ID index, the
sandbox's partition). Two consequences closed with it: the pass no longer
conflicts with its own second analysis, so a fn UNIT with such a body
compiles — and it DROPPED the mint (`f 1 f 2` answered `[1 2]`), since
`RecordTypeInstall` recorded nothing outside an arm-resident bracket and
the ledger keeps fn-body transitions out (no twin). The recorder's
fn-unit arm now records the install with the entry the check pass pushed,
and the lowering emits `OpBindFnType`, a generic per-call lowering rather
than a decline (the compile-failure census is a downward ratchet, and the
generated sweep's fn-wrapped type-def variants must keep compiling): the
name is checked against the run's reservations exactly as the front door
checks it (`core.TypeNameFree`) and reserved (`core.ReserveTypeParts`),
the check-time node is bound as an ADOPTED entry (the node the unit's
`OpPushType` reads bake; an `undef` in the body must not retire it) on the
dyn-bind trail, so the frame's RET pops it — the interpreter's observable
exactly: the first call answers, the second conflicts, a stored body's
CallBoru frame (`Test.check-prop`'s gen body) conflicts on its second
trial, a `do` body nested in a fn leaks to the fn's frame and pops with
it. A ROOT `do` body still records nothing (its install is a root
transition the adopted twin replays). And a ROOT `def T` after the call kept its reservation
from check time (the twin regime retains mints), so the callback's mint
conflicted at element 0 where the interpreter's root def conflicts — the
replay rollback frees the parts of the bindings it rolls back
(`BindingSandbox.typeParts`) and the type twin re-checks and re-reserves
its name at its own position (`applyTwinPush`, `ApplyBindTwin` returns the
raise). Pinned by lang `TestFnBodyTypeMintIsTheCalls` and user-types.tsv's
closing section.

**Rule:** a compiled program answers as the interpreter does.

**Divergence.**

```
def f fn [[n:Integer][Integer][def T (class {}) n]] end each f/v [1 2]
  interp:    each: element 1: type: name part "T" in "T" conflicts with an existing type name
  compiled:  each: element 0: … the same detail, one call early
```

The check pass runs `f`'s body once for the analysis and mints `T`; on
the compiled path the pass's runtime-visible installs persist
(RunAutoValues keeps them so OpPushType can resolve minted IDs), so the
first real call re-mints against its own analysis-time twin. The direct
call `[(f 1) (f 2)]` refuses to compile and falls back — the interpreter
answers — which is why the shape hid until a callback form compiled.
Pre-existing at #474's merge base (measured on `origin/main`).

**Where it belongs:** the replay of a fn body's registry mutations
across the check pass and the run — the same rollback-and-replay the
bind twins give a top-level `def T` (§6.5), owed to a mint inside a fn
body. S1b's lazy stamp declines such bodies (bodyHasReplayHazard) so
the detached compile pass adds no third mint.

## NUR166 — a def-bound fn value passed to a higher-order word loses its own frame's `args` {#nur166}

**Status:** FIXED 2026-09-25 (the value's own frame — the handoff log's
entry of that date). Recorded 2026-09-19 by the S1b seam pins.

**The fix.** The value's body IS the `each` site's closure unit, entered
through the token seam (`applyClosure`), and that entry pushed no args
list — `pushRootArgs` bracketed only the units the fn-value seam hosts.
A closure unit that carries a param contract (`CompiledFn.Params`: a
named fn's body or a lambda's, compiled at the callback slot with the
value's own params — a quotation body carries none and runs in the
caller's frame, reading the caller's `args`) is the frame the
interpreter's dispatch would have opened for the value, so `applyClosure`
now brackets it with the value's own call args (`locals[0:NArgs]`, the
DynEnv bracket's list; a no-op outside a DynEnv program, where nothing
reads it). `args` read directly in the body already folded to the
params' locals; the `do` body inside it reads the list at run time, which
is where the two lanes parted. Pinned by lang
`TestFnValueCallbackHasOwnArgs` and callbacks.tsv §10.

**Rule:** a fn value applied anywhere opens the frame its definition
declares — with the value's own call args as `args`.

**Divergence.**

```
def g fn [[n:Integer][Any][do [args] size]] end each g/v [1 2]
  interp:    [[1 1]]
  compiled:  [[0 0]]
def g fn [[n:Integer][Any][do [args]]] end each g/v [1 2]
  interp:    [[[1] [2]]]
  compiled:  [[error(args: not inside a function) error(args: not inside a function)]]
```

The recorder lowers the def-bound value's body as the `each` site's
closure unit (the callback path), and a closure's `args` is the
ENCLOSING frame's list — none at top level. The interpreter opens a
frame for the value and pushes its call args. Pre-existing at #474's
merge base (measured on `origin/main`). S1b's native fn-value seam
pushes the value's own args for the units it hosts (`pushRootArgs`, at
every fn-value entry: the token seam, RunUnit, a foreign ref), but this
row never reaches the seam — the closure lowering takes it first.

**Where it belongs:** the closure lowering of a NAMED fn value at a
callback slot — either it opens the value's frame (an args push per
element, the value's contract) or it declines to the fn-value seam,
which does. Same family as NUR155 (the closure unit is not the value's
frame).

## NUR165 — a token body over a declared-Any collection committed one arm and met an Integer {#nur165}

**Status:** RESOLVED 2026-09-19 (the S1a review, PR #474).
**Found:** a Codex review of #474 — the Reach twin `def get fn [[][Any][1]]
end each $.x (get)` answered `[[]]` compiled where the interpreter raises
`signature_error` — and the sibling shapes probed from it.

**Rule:** a dispatch the check pass cannot prove is a runtime re-match.
An operand the pass knows only as `Any` (a fn's declared `Any` result)
may be anything at run time; no arm the checker picks from it is a
proof, whichever arm count the other operands leave.

**Divergence.** Two recorder seats trusted such a pick:

```
def get fn [[][Any][1]] end each [add 1] (get)
  interp:    signature_error — cannot call `each`: no signature matches
  compiled:  each_error — each: expected a concrete map     (main, pre-existing)
def get fn [[][Any][1]] end fold [add] (get) 0
  compiled:  fold_error — fold: expected a concrete map     (main, pre-existing)
def get fn [[][Any][1]] end each $.x (get)
  compiled:  [[]]                                            (S1a, silent)
```

The token-body shapes took the cross-collection shortcut
(`CallableSpec.CrossCollectionTokenShape`, `tryRecordClosure`): the
committed (List, Map) closure relies on the handler delegating to list
or map iteration by the runtime value's type — robust to the SIBLING
collection, not to an Integer. The Reach shape reached S1a's dyn-body
seat, whose re-match fired only when two arms were reachable; with a
Reach body only (Reach, List) is, so the pick was baked and the handler
iterated an Integer as an empty list.

**Fix.** `tryRecordDynBody` re-matches whenever an Any carrier — strict
or gradual — is among the operands, whatever the count (the VM's poly
re-match raises, or defers to, the interpreter's own no-signature
verdict). The token-body shapes keep the shortcut — routing a
CompileDynBody word to the dyn-body seat instead was measured and
rejected: the seat arms DynEnv mode program-wide, and fifty-three
module-cli.tsv rows importing a module whose fn defs a computed value
refused with it — and the committed arm's guard (`requireConcreteMap`)
raises the dispatcher's own `signature_error` (`core.NoMatchDetail`) for
a concrete runtime value that is neither collection: the handler's
robustness contract, extended from the sibling collection to no
collection at all. Pinned in `lang/go/bytecode_dynbody_callback_test.go`
(`TestStrictAnyOperandReMatches`): thirteen rows, every one compiling,
the two lanes agreeing on value and error code.

## NUR164 — a callback mismatch inside a native handler was a plain Go error, which the compiled lane read as its own bug {#nur164}

**Status:** RESOLVED 2026-09-19 (S1a of design/FULL-COMPILATION-REPLAN.0.md).
**Found:** the S1a pin `def f fn [[c:Any][Any][each ([x:Integer] => [x add 1]) c]] end f {a:1 b:2}`, written to prove the poly re-match picks the Map overload the interpreter picks.

**Rule:** a handler's verdict on the program is a BoruError. The
compiled-by-default lane (`RunAutoValues` → `runtimeShouldFallback`)
treats a non-Boru error off the VM as an INTERNAL error — a lowering or
VM soundness bug — and resolves it by rolling the registry back and
re-running the whole program on the interpreter.

**Divergence.** Three sites raised a bare `fmt.Errorf` when a callback
matched none of a fn value's signatures: the Map iteration of
`each`/`fold`/`scan` (`no matching lambda signature for N argument(s)`,
`lang/go/native/native_map_iter.go`), `filter` and `walk` (`no matching
callback signature`). Both lanes raised the identical message, so every
differential passed — and the auto lane answered `ran=false` with an
interpreter re-run behind it: a correct compiled verdict reported as a
bail, counted under the runtime-bail attribution, and blocked by the
effect fence had the body printed first.

```
def f fn [[c:Any][Any][each ([x:Integer] => [x add 1]) c]] end f {a:1 b:2}
  strict:  each: key "a": no matching lambda signature for 1 argument(s)
  interp:  the same
  auto:    the same — ran=false (re-run on the interpreter)
```

**Fix.** The three sites raise `signature_error` through `reg.BoruError`;
the wrapping `each: key %q: %w` stays, and `errors.As` finds the code
beneath it. The pin (`lang/go/bytecode_dynbody_callback_test.go`) asserts
the compiled lane for the Map case.

## NUR163 — a module member read in place is not the fn its local rebind is {#nur163}

**Status:** FIXED 2026-09-25 (the value's own signatures — the handoff
log's entry of that date). Recorded 2026-09-18 by the generated sweep,
writing the module-export seeds for `emit`, `mini` and `parse`.

**The fix.** The member read hands the words the fn they wanted; what they
could not see past was the dispatch AGGREGATE the value carries: a `/v`
read, a module member in place and a paren all deliver `Registry.Lookup`'s
aggregate, whose synthesized 0-arg fallback signature (`Signature.Fallback`,
an empty parameter list) failed "every signature must start with the
standard prefix" — while `def g M.dbl/v` stores the own signatures
(installDef's rebind) and passed. The record's `mini (dbl/v) 'ab'` failed
the same way with no module at all. The three contracts
(`MiniLangFnSigWhy`, `EmitLangFnSigWhy`, `ParseLangFnSigWhy`), the
filter-shape probe (`MiniLangFnFilterShaped`) and the partial's builder
(`miniPartialFromSigs`'s callers) read `FnDefInfo.OwnSigs()` now — the
fallback was never a declaration. The filter-shaped `/v` value used to
crash the partial's build once past the probe (`index out of range [2]`
over the fallback's parameters); it builds over the own signatures. The
side note — `def g M.up/v end` raising `cannot call def` for an export
whose first parameter is `Any` — is a forward-window quirk of its own,
recorded as NUR212. Pinned by lang `TestMiniLanguageValueIsTheFn` and the
`§fnv` / `§10` closing sections of module-minilang.tsv,
module-emitlang.tsv and module-parselang.tsv.

**Rule:** a fn value is the same fn however it is reached — through the
module member in place, through a local `def` of that member, or through
`/v`.

**Divergence.** With `import "boru:minilang"` and a module `M` exporting
`dbl` (`fn [[src:String opts:Map] [String] [src add src]]`):

```
mini M.dbl 'ab'                        mini_bad_signature: every signature must start with the standard prefix [src:String opts:Map …]
mini (M.dbl) 'ab'                      the same
def g M.dbl/v end mini g 'ab'          abab
```

and with `import "boru:emitlang"` and `M` exporting `up`
(`fn [[value:Any opts:Map] [String] ['UP']]`):

```
emit M.up {a:1}                        emit_bad_signature: every signature must start [value:Any opts:Map …] and return a value
emit (M.up) {a:1}                      the same
def g (M.up) end emit g {a:1}          UP
def g M.up/v end emit g {a:1}          signature_error: cannot call `def` (the spelling works for a single-signature fn: `def g M.inc/v`)
inspect (M.up)                         {type:'Type' struct:'Function' kind:literal}
inspect up                             {name:'up' type:'Function' kind:defined signatures:[…]}
```

`parse` behaves as `mini`. The words' validators read the signatures of
the value they are handed; the in-place member read hands them a
Function whose signatures they cannot see, and `inspect` shows the same
difference. This is the interpreter's verdict (the compiled lane never
gets past it in the sweep), so it is an interpreter-side non-uniformity;
the sweep records the module-export cells of these words through the
local rebind, which is the spelling the language accepts.

**Where it belongs:** the module member read (`dot` over a Module) and
what it hands back for a fn export — the `FnDefInfo` the local rebind
recovers, or the wrapper that hides it.

## NUR170 — a container-read fn value at a word's operand slot arrives in the wrong position {#nur170}

**Status:** FIXED 2026-09-25 as a sound decline (the dynamic emit lead —
the handoff log's entry of that date). Recorded 2026-09-19.

**The fix.** The transposition was the `emit` macro's ROUTE, not a slot:
under analysis `m.up` is a dynamic Any carrier, which is not a Function to
the macro's fn-operand test, so the macro committed the lead to the DATA
route (`emitlang-auto <lead> <opts>`) and recorded that call, where the
interpreter's expansion reads the concrete fn value at run time and takes
the fn route. A dynamic lead is an emitter or data only at run time; the
macro degrades it now exactly as it degrades a not-concrete Function lead
(an advisory and a dynamic String carrier), so the program declines
loudly ("residual value of unknown provenance") and the fallback runs the
emitter. A bound data lead (`def d {a:1} emit d`) and a kind lead compile
as before. The sweep's `emit` × container pin is retired and its seed
counts under `sweepFailureCeiling` (25 → 26). Pinned by lang
`TestEmitDynamicLeadIsNotRoutedAsData`.
**Found:** the generated sweep's `emit/container` seed, on the change that
removed the interpreter fallbacks. It is not a new defect — it was there
before, and the whole-program fallback answered the program correctly so the
sweep classifier never saw a divergence. Removing the fallback is what made it
visible, which is the point of removing it.

**The witness.** A fn value read out of a MAP and handed to a word that takes
`(Any, Map)`:

```
import "boru:emitlang" end
def m {up: (fn [[value:Any opts:Map] [String] ['UP']])} end
emit m.up {a:1}

  interpreted   UP
  compiled      [boru/signature_error]: cannot call `emitlang-auto` —
                no signature matches the arguments
                  = note: the arguments were {a:1} (a Map) and fn (Any, Map) (a Function)
```

The note is the diagnosis: the compiled lane dispatches `emitlang-auto` with
`{a:1}` where the FN belongs and the fn where the Map belongs. The two
operands arrive transposed, so no signature matches and the dispatch raises
where the interpreter runs the emitter.

**Where it sits.** `m.up` is a dot access, which `expandReach` expands to a
paren group whose single net value is a dynamic `Any` carrier — the same
machinery NUR169 turns on. A carrier at an operand slot has no declared
position of its own, and this row is the case where the slot it lands in is
not the one the interpreter gives it.

**Its family.** NUR154, NUR156, NUR159, NUR160, NUR161 and NUR169 are all the
compiled lane treating a computed fn VALUE differently from the interpreter —
applying one it should pass, passing one it should apply, or (here) seating
one in the wrong slot. S1b is the increment that owes them a single answer at
every seam, not six.

**Pinned:** `sweepKnownMiscompiles`, keyed to this entry. A pinned divergence
is debt on the record, never a decision: the pin exists so the sweep fails the
day the WRONG ANSWER changes shape, not so the row can stay wrong.

## NUR171 — a compiled no-match diagnostic loses its source position {#nur171}

**Status:** FIXED 2026-09-25 (the lens's own token — the handoff log's entry
of that date; recorded 2026-09-19).
**Found:** `reach.tsv:L52` on the change that removed the interpreter
fallbacks. Not a new defect: the position was always missing, and the
whole-program re-run supplied one before anything could notice.

**The witness.**

```
5 $.name apply

  interpreted   [boru/signature_error]: cannot call `dot` — no signature
                matches the arguments
                  --> 1:1
  compiled      the same code and the same detail,
                  --> source position unknown
```

Code and detail match. What is missing is `Row`/`Col`, so the renderer prints
"source position unknown" instead of underlining the token. (The two lanes'
NOTES also differ at this site, for an unrelated reason — that is NUR172, and
it is pinned separately.)

**Where it sits.** The VM raises the interpreter's own error here rather than
bailing, which is the right disposition — a trap that raises the interpreter's
error at the same moment. It builds it through `polyNoMatchRaise` →
`NoMatchDiag(…, spec.Pos, …)` and then `stampAt(ae, curDebug, pc, r)`, and
BOTH position sources are empty for this site: the recorded
`PolyNoMatchSpec.Pos` carries none, and the debug table has none at that `pc`.
So the fix is in the RECORDER — give the no-match spec its dispatch position —
and not in the raise, which already stamps whatever it is given.

**What was tried and rejected.** Walking the debug table BACK from `pc` to the
nearest earlier instruction that does carry a position. It is a reasonable
degradation in general and it fixes nothing here, because the whole region is
unpositioned; a change that broad with no measured benefit is not worth the
risk to every other VM error's position.

**Tried (2026-09-25, closing NUR172 beside it).** Two anchors for the
raise, neither reaching this site: the recorded spec now takes the first
WRITTEN operand's position at record time when the word has none
(`polyNoMatchProbe.Spec`) and the VM raise takes the first written
value's position at run time — the lens's `dot` is applied inside the
`apply` native's own run, where the window's values are pooled constants
and the synthesized tokens carry none — and a walk back through the
debug table (the rejected degradation, tried scoped to this one raise)
found nothing either, as the record predicted. The interpreter's 1:1 is
the receiver's position the lens expansion stamps on its `dot`; the check
pass's expansion stamps none. Still pending.

**The fix (2026-09-25).** In the recorder after all, one level down: the
lens unit's body is `lowerReach` over a synthesized receiver
(`__reach_recv`, compiledLensSig) that carries no position, so the `dot`
it stamped from the receiver carried none either. `lowerReach` now takes
the segment's own KEY token as the anchor when the receiver has none, so
the recorded no-match spec and the raise carry `1:5` (the key) where the
interpreter underlines the receiver at `1:1` — a column apart, both
present, which is what the gate asserts. The ledger entry below is
retired; pinned by `TestLensNoMatchKeepsAPositionCompiled`.

**Pinned (until the fix):** `knownPositionLoss["reach.tsv:L52"]`, its own ledger rather than
`knownDivergences`: position presence is asserted only by the
compile-or-fallback gate, and a pin in `knownDivergences` must drift on every
gate that walks the corpus. The gate compares error PRESENCE and position
presence, not exact Row/Col — two lanes legitimately differ by a column — so
this pin is specifically about a position that is absent, not one that is
different.

## NUR172 — the two lanes describe different argument windows at a poly no-match {#nur172}

**Status:** FIXED 2026-09-25 (the attempted window — the handoff log's
entry of that date). Recorded 2026-09-19.

**The fix.** In the direction the record named — the interpreter. Its
no-match report (`sigError`) took the forward candidates the collection
had FILLED, else the stack prefix; `ReorderForwardCandidates` stops at a
bare word, so the `name` a `/q` slot would capture was never a candidate
and the report fell back to the prefix (`the argument was 5`) and an
arity failure the source does not have. `core.attemptedWindow` builds the
window the dispatch attempted: the forward candidates, a bare word right
after the pointer counted as the Atom when some overload's first slot is
`/q`, and — when those are fewer than the smallest overload's arity — the
stack prefix beneath, in signature order. `5 $.name apply` and `5 dot
name` read `the arguments were name (an Atom) and 5 (an Integer)` with the
type-failure candidate notes on both lanes; a pure stack no-match (`'x'
add`) reports as before. The compiled lane derives the same tuple: the
check pass's record-time twin (`rematchWritten`, the carrier-aware forward
walk, then `attemptedWindowOver`'s rule) feeds both the poly no-match
spec and the runtime-rematch trap, whose render bound became an INDEX
tuple (`DispatchSpec.Written`) because a mixed tuple — the forward operand
and the stack value beneath it, `[1 2] each [dup mul]` under a variadic
lead, `(x add 1)` over a gradual x — is not a contiguous slice of a window
that lists the stack run first. The corpus ledger's `knownDiagDrift` pin
for reach.tsv:L52 is retired. Pinned by lang
`TestNoMatchWindowIsTheAttemptedOne`, the poly no-match and rematch
byte-identity tests, and `TestPolyNoMatchDeeperStackShapeRaisesCanonical`
(the former bounded edge: `9 1 x add` raises the canonical error on both
lanes).
**Found:** `reach.tsv:L52`, on the change that removed the interpreter
fallbacks. Not a new defect: both diagnostics were always built this way, and
the whole-program re-run replaced the compiled one before anything could
compare them.

**The witness.**

```
5 $.name apply        # a field lens applied to an Integer

  both lanes    [boru/signature_error]: cannot call `dot` — no signature
                matches the arguments

  interpreted   the argument was 5 (an Integer)
                candidate `dot (Integer, Node)` takes 2 arguments, but 1 was supplied
                candidate `dot (Atom, Module)` takes 2 arguments, but 1 was supplied
                …

  compiled      the arguments were name (an Atom) and 5 (an Integer)
                candidate `dot (Atom, Module)` — argument 2: expected Module, got 5 (an Integer)
                candidate `dot (Atom, Class)` — argument 2: expected Class, got 5 (an Integer)
                …
```

Code and detail are identical. The NOTES describe two different failures: an
ARITY failure over one argument, and a TYPE failure on the second of two.

**Where it sits.** Not in the lens, and not in the compiler. `5 dot name`
typed straight at the interpreter reports the same one-argument window, so
this is how the interpreter's matcher reports a word whose forward slot it
never filled: no candidate's stack half matched, forward collection stopped,
and the report is written over what was collected. The VM's
`CALL_NATIVE_POLY` has both operands on the stack by construction, so
`NoMatchDiag` writes the fuller — and more accurate — report.

**The direction of the fix is the interpreter, not the VM.** The compiled
report names the two values the user actually wrote; the interpreted one
describes an arity the source does not have. Closing this means the
interpreter's no-match diagnostic reporting the window it ATTEMPTED rather
than the window it managed to fill. Weakening the compiled note to match is
not a fix — it would trade an accurate diagnostic for a uniform one.

**Not sized here.** It is an interpreter-diagnostic change that touches every
`signature_error` the interpreter raises, so it is scheduled on its own, not
carried by the fallback removal that found it.

**Pinned:** `knownDiagDrift["reach.tsv:L52"]`, its own ledger for the same
reason NUR171 has one: the notes are compared only by the compile-or-fallback
gate, so a `knownDivergences` pin would fail the differential gate for not
seeing a divergence it does not look for.

## NUR206 — the loop index survives an error the enclosing `do` catches {#nur206}

**Status:** FIXED 2026-09-26 (the merge of main's #508 with the
reverse-order NUR run — the handoff log's entry of that date). Recorded
2026-09-25.

**The fix.** None of its own: the reverse-order NUR run had closed the same
defect as NUR208 (found closing NUR197) — the interpreter's fault return
unwinds every `for` loop the error leaves live on the tape
(`Engine.unwindLiveLoops`, the escape paths' twin), so the raise that `do`
catches uninstalls the loop's index level. Measured on the merged tree: all
three witnesses answer 99 on both lanes, compiled, and with no outer
binding the name is `undefined_word` after the caught error. The pin is
flipped to the closed state: `TestLoopIndexUnwoundByCaughtError`.

**Found:** the Codex review of #508, measuring the hosted computed `for`
body that push withdrew; the literal-body twin measured on the follow-up,
on `main` at 9e02915.

**The witness.**

```
def i 99 end do [for 3 [raise oops 'x']] error [drop] end i

  interpreted   0      the loop's index level stays installed after the raise
  compiled      99     the handler and the read see the outer binding
```

The same with the handler reading the name (`error [i]`), and with a
computed body (`def mk fn [[][List][quote [raise oops 'x']]] end do [for 3
(mk)] error [drop] end i`). An `each` body raising inside the same `do`
agrees on both lanes (99): the callback runs in its own body run, whose
cleanup the error path takes.

**Where it sits.** The interpreter splices the `for` body onto its tape
with a move cleanup after it; the raise unwinds to `do`'s trap without
reaching that cleanup, and nothing else pops the index level, so `i` stays
bound to the iteration's value for the rest of the program. Break and
continue take `unwindLiveFrames` for exactly this reason; the error path
does not. The compiled lane traps or islands the loop inside `do`'s body
and the later read of `i` is its compiled slot, the outer binding.

**The direction of the fix is the interpreter, not the VM.** A caught error
should leave the registry as the loop found it — the frame cleanup the
escape paths already run. Making the compiled lane leak the index to match
would be a uniform wrong answer.

**Pinned:** `TestLoopIndexSurvivesCaughtErrorPending` (lang/go), asserting
the divergence as it stands so the close is noticed.
