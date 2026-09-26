# Session handover — the full-compilation project

**Purpose.** The one page a fresh session reads to know where the project
stands *right now*. It is deliberately short and deliberately
CURRENT-STATE-ONLY: the per-increment narrative, the measurements and the
lessons live in [FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md),
which is an append-only log and the wrong place to look for "what is true
today". Update this file at the end of every increment.

Last updated: **2026-09-25**.

**Read in this order:** the definition of done below; then
[FULL-COMPILATION-REVIEW.0.md](FULL-COMPILATION-REVIEW.0.md) (2026-09-17,
the plan re-examined and re-staged — its §5 is the work order) and
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md) §10.1 (the same staging as
the design's own amendment); then `make gate-status` for the live numbers.
The per-increment narrative, every measurement and every lesson live in
[FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md), the
append-only log, which is the wrong place to look for what is true today.
This page is kept under 200 lines on purpose.

> **This page is about 800 lines** (1150 on 2026-09-21; the arm-binding
> narrative and the live-miscompile record moved to the handoff log on
> 2026-09-22 when both closed). The merged-and-done accounts still here —
> the fallback removal, the 2026-09-20 sweep, NUR173/174/175 — are history
> and should move there too. The sections worth keeping are the definition
> of done, the gate table, what is next, the process rules and the
> instruments. Two ceiling comments in `test/go/langspec/` cite the heading
> "Where the join seats" by name, so that heading survives every move.

## OPEN RIGHT NOW — read this before picking up work

The two items this section led with on 2026-09-21 — the live miscompile
and the arm-binding join — were ONE defect at the branch join
(`InstallJoinedDefs`), the miscompile was NUR110 (recorded 2026-08-28), and
both are CLOSED on 2026-09-22 by the **branch-carried def**
(`compiler/go/branch_carried.go`): the loop-carried def's own frame-slot
mechanism, seated at `RecordBranch`. The `fn-locals-scope` §6 cluster
compiles with parity on both paths (19 → 2 in the per-file ledger; the two
left are fn-VALUE branch results, Stage 3), and `if false [def op 1] [0] end
op` raises `undefined_word` compiled, at the read, as the interpreter does.
The account, the four regressions the corpus gates caught on the way and
the rules they became are in the handoff log under **"S5 — the
branch-carried def"**; the re-estimate is
[FULL-COMPILATION-REPLAN.0.md](FULL-COMPILATION-REPLAN.0.md) §11.

**Open now, in the order the re-estimate ranks them:**

1. **S1b's apply shapes, the remainder** — the first slice landed on
   2026-09-22 (the handoff log's "the apply shapes" entry: the leading
   one-arg window `(f x)` records wherever the lead is a slot of the
   recording unit — a branch arm, a loop body, a callback lambda — over a
   typed gradual argument or an inert fn value; callbacks 17 → 10), and
   S1b-3 re-landed the same day (a fn value stored in a container; 45 →
   39 → 32 corpus compile failures in the two steps), and the DYNAMIC-lead
   group landed the same day (the handoff log's "the dynamic-lead group"
   entry: the `apply` word at the main program over a gradual lead or a
   produced fn-typed carrier — callbacks L40/L41/L50/L51,
   module-composition L100; 32 → 27), and the container-member calls
   after it (the handoff log's "the container-member calls" entry:
   `(fs.b 10)` over a produced closure — callbacks L60/L73,
   module-composition L96; 27 → 24), and the curried chain after that
   (the handoff log's "the curried chain" entry: a paren over a produced
   closure records its re-stepped lead's apply at the collapse — L151;
   24 → 23 — and closed two silent miscompiles on `main` on the way,
   NUR178 (`((mk 1) 2) mul 10` leaked its window to `mul`: 21 for 30) and
   NUR179 (every fn-value-call op bound a two-param closure backwards)).
   The container reads inside quotation bodies landed after that (the
   handoff log's "the container reads inside quotation bodies" entry: a
   code-body closure unit takes the whole-frame replay for a fn-valued
   member read at its tail — each-variants L205, fold-map-filter L215,
   module-composition L98; 23 → 20 — and closed NUR182, the fn-unit replay
   re-stepping a paren-placed value, and NUR183, `each [dup ops.inc]`
   compiling the member as data, both silent on `main`). NUR156 closed
   after that (the handoff log's "the quotation-body def reads" entry:
   the check model of `apply` kept a `/v`-marked reach group's quote, so
   `5 M.inc/v apply` parked on the pass; and a module-scope def bound to
   a fn-valued member read bare in a code body — `def f tbl.inc end each
   [f] xs` — is the interpreter's WORD dispatch, now armed under its name
   through the closure-body replay and the VM's word island;
   module-composition L102 and L103 agree, L104 declines loudly in the
   dynamic-scope def family where its local twin always did — 20 → 21,
   the S1a trade; the sweep's `apply` × module-export seed graduated).
   What is left of the family, each declining where it declined before:
   the same def read at the MAIN program (`5 f` is `[5 fn]`) and inside
   a named fn unit (a dynamic-scope-read bail) — NUR123's open points,
   pinned by `TestDefReadWordDispatchPending`; a member read in
   a WORD's forward slot inside a fold body (`0 fold [add ops.inc] xs`),
   `do [5 (ops.inc)]` (the caller's re-step over a sibling result),
   `((reg.cb) 5)` (L61 —
   the flex shape is not threaded on the compile pass, the flex family's
   own item), apply-twice's two pending applies (L125), a def-bound
   factory result read back and applied (`def p (mk 1) end 5 p/v apply`,
   the read's statement window); NUR176's 0-arg runtime lead is CLOSED
   2026-09-25 (the handoff log's "the 0-arg lead's window" entry: the
   window op hands a name-read lead whose only overloads take no
   argument to the island in written order, `h z/v` over `[(k inc/v)]`
   is 8 on both lanes), and so is NUR168 the same day (the handoff log's
   "the def's name on the value" entry: a def names the fn value it
   binds whatever its representation — a factory's const lambda renders
   `fn f(String)` on both lanes — the root write-back adopts its paired
   dyn-scope install instead of stacking a second entry, and a
   code-body call of the bound value by its bare word dispatches, where
   it was an internal error), and NUR167 (the handoff log's "the analysis
   is not a call" entry: a fn-body analysis's type-part reservations come
   off with the body's bindings, so a type-minting callback body conflicts
   on its second call as the interpreter's does, a fn unit re-installs the
   type per call — `OpBindFnType`, a generic lowering, not a decline —
   and a root type twin re-checks its name at its own position), and NUR166 (the handoff
   log's "the value's own frame" entry: a fn value applied by a
   higher-order word reads its own call args as `args`, through a `do`
   body inside it too — the token seam brackets a unit that carries a
   param contract with the value's args), NUR163 (the handoff log's "the
   value's own signatures" entry: the mini / emit / parse contracts read
   a fn value's own signatures, so a module member in place, a paren or a
   `/v` read — all carrying the dispatch aggregate's fallback — pass as
   the def-bound spelling does) and NUR162 (the handoff log's "the 0-arg
   apply at a paren's tail" entry: the trailing apply's window trim
   declines a 0-arg callee instead of recording a signature-less native
   call the disassembler crashed on; the sweep's crash ceiling is 0),
   NUR212 (the handoff log's "the marker is no argument" entry: the
   forward claim probe answers no claim for a reach's `/v` marker, so a
   module member with an Any first parameter binds through `def`) and
   NUR211 (the handoff log's "the named value's no-match on the seam"
   entry: a named fn value's no-match on the callback seam raises
   uncalled_function as the word does, past step 0 too), and NUR213 (the
   handoff log's "the marker's intent on the value" entry: the check pass
   quotes the dynamic member read a `/v` marker qualifies, and the
   residual layout leaves a quoted lead alone — `m.f/v 5` is data on
   both lanes), NUR172 (the handoff log's "the attempted window" entry:
   the interpreter's no-match report describes the window the dispatch
   attempted, the poly window's own layout) and NUR170 (the handoff
   log's "the dynamic emit lead" entry: the emit macro degrades a dynamic
   lead under analysis, a wrong answer turned into a loud decline),
   NUR158 (the wrap words bridge a compiled closure to its fn
   definition), NUR157 (two references to one predicate unify as the
   type), NUR154 (the check pass no longer traps a computed `case` clause
   list — a sound decline, the sweep and corpus pins retired) and NUR147
   (the poly seat retries the word's other arities before it defers),
   NUR142 (the handoff log's "the family of a refinement" entry: a
   refined container is eq to itself — equality folds a user refinement
   to its kernel ancestor), NUR146 ("the frame's names": the compiled
   undefined_word's did-you-mean pool reads the unit's frame-local names
   through the new `NameLocal` seam and `Program.LocalNames`), NUR135
   ("the last pop": a minted node is retired when the last live binding
   holding it pops), NUR118 ("the call's own token": a fn's
   return-contract error anchors at the call word on both lanes — the
   recorded call carries the word's position and the RET stamps at the
   caller's return address; the `(g 5)` fn-value seam anchors at its
   lead's read), NUR122 ("the read's own token": every fn-value apply
   witness agrees on both lanes, message and position), NUR130 ("the condition's own token": `while`'s empty-condition
   raise anchors at the operand interpreted too), NUR080 and NUR114
   (verified retired on the current tree — the typed def keeps its brand
   in both orders, the compiled caret is the token's width — with the
   acceptance pins the records asked for); NUR129 is narrowed (the named
   witness declines soundly through the check pass's own diagnostic),
   and NUR141 and NUR134 carry traced notes (the predicate admission is
   `RunPredicate`'s analysis-mode arm; the module fn value's caught
   no-match needs a unit-scoped trap) with their rulings still owed. The
   milestone batch of 2026-09-25 also re-derived the compiled twin of
   NUR172's attempted window (`rematchWritten` feeds the poly no-match
   spec and the runtime rematch, whose render bound is an index tuple
   now) and scoped NUR213's quoting to dynamic and carrier values;
   NUR150 and NUR151, closed in their own PRs, are marked FIXED. Two
   silent miscompiles the chain's probes found, both CLOSED now: NUR180
   (a trailing paren apply's Any result inside an UNNAMED-param frame,
   consumed by a typed word — `xs each [(2 (mk 1)) mul 10]` was 10 for 30;
   closed 2026-09-23, the handoff log's "the recovery's window" entry: the
   checker's unmatched-dispatch recovery lays its window out forward-first,
   as the interpreter's matcher does) and NUR181 — CLOSED
   2026-09-23 (the handoff log's "the def-bound closure park" entry: a
   shaped method call's or a fn-value apply's returned closure is parked
   on the compiled lane too — `def r ((mk3 1) 2) end r 3` is `[2 fn]` on
   both lanes, and `5 (mk 3)` seats as the parked pair where it declined).
   The silent family found on the way, NUR184 — a paren's TRAILING fn
   value forward-collects the tokens past the `)` (`(2 (mk 1)) 10` is
   `[2 11]`; `(2 inc2/v) 10` the same for a named fn value) and a paren
   closing under a pending forward hands its survivors to it (`10 mul (2
   (mk 1))` is 21; NUR180's fold row is this record's) — is CLOSED
   2026-09-23 (the handoff log's "the trailing value's re-step" entry):
   the collapse records the trailing apply only for a lead the
   interpreter dispatches inside the paren, a collectable follower leaves
   the value to the rewind, a forward's leftover is marked as such, and
   the mixed island's interpreter seals a compiled closure at its
   collection's completion. What is left declines
   (`TestParenTrailingFnSoundCompileFailures`): the leftover at the main
   program, a leftover with nothing beneath it, a list literal's
   re-stepped element, and a numeric word whose poly record collected
   the marked carrier (`(2 (mk 1)) 10 mul`). NUR185, found probing its
   neighbours and present on main — the mixed island re-stepped a `/v`
   read of a def-bound closure (`def c (mk 3) end 2 c/v 10` was `[2 30]`
   for `[2 fn c(Integer) 10]`) — is closed in the same entry; its rows
   decline at the render gate. The typed callback's contract (2026-09-23,
   the handoff log's entry of that name) closed two more silent families:
   NUR155 (a typed lambda lowered as the word's own body unit ran on
   every element of a heterogeneous list; each-variants L216 left
   `knownDivergences`) and NUR160 (the `apply` word over a factory's
   baked fn const under a dirty stack, and over an empty window; the
   sweep pin retired), and found NUR186 (a module fn's value at a
   factory body's tail raises interpreted, returns compiled — pinned
   pending). The branch result's re-step (2026-09-23, the handoff log's
   entry of that name) closed NUR159 — the sweep's `if` × named-fn cell:
   the recorder's branch event tells every collapse-side and
   residual-side gate that a merged value MAY be a fn (`MayBeFn`, with
   an arg-taking flag), so `if true one/v [2]` is 1 everywhere and `7 if
   true inc/v [2]` is 8 — and found and closed NUR187 on the way: the
   residual's apply arms carried a fn value's collection across a
   statement boundary (`7 m.f ; 3` islanded to `[7 4]` for `[8 3]`, `m.f
   ; 5` applied for `[fn 5]`); the pass notes every boundary's position
   now and no arm applies a value over a later statement's entry. The
   named fn value's candidates (2026-09-23, the handoff log's entry of
   that name) closed NUR186 — the landing note carries what follows the
   value (a function word, a value-bound word the forward phase collects,
   a boundary, the tape's end) and whether values sit beneath it, the VM's
   landing raises the
   interpreter's `uncalled_function` for a named fn over a candidate and
   an empty frame, and a concrete named fn at a fn frame's tail declines
   the unit (the unit-level trap is the follow-on) — and found and
   closed NUR188 (a poly collecting a member read the interpreter
   re-steps first) and NUR189 (the trailing arms applying a paren-placed
   member), plus NUR187's fn-unit and word-after halves — and recorded
   NUR190: a DYNAMIC fn value under a function word. The landing's
   overload walk (the same day, the handoff log's entry of that name)
   closed its typed-slot, Any-typed and anonymous halves — the landing
   carries the word and runs the interpreter's own plan over it — and
   left the `/q` capture and the Function-typed reference open: both
   need the word's compiled call skipped. The maintainer ruled
   (2026-09-24): defer at the walk, and keep such deferrals on a ledger
   — `runtime_defers.tsv`, the per-file twin of `compile_failures.tsv`,
   names every corpus row that compiles and then bails, fn-value.tsv's
   L317/L318 the first booked by choice (runtime defers 8 -> 10). The
   placed value
   inside a code body (the same day) compiled the `[(lambda)]` and
   `[(mk 2)]` bodies into their units (seven census rows left), and the
   def-bound computed fn read at a closure body's tail (2026-09-24)
   lowered `each [a5] xs` as the body-tail trailing apply over the
   read's live lookup, the fn-util wrappers with it (six census rows
   left); its fn-body-LOCAL twin (NUR192) closed the same day with the
   frame's closure bind — the VM pushes a closure value the installer's
   carrier guard ignored, and the nested read compiles natively — with
   a fn body's computed fn def shadowing an enclosing one declined
   loudly (the interpreter's install outlives the call). NUR193 (a
   def-bound computed fn read inside a `do` body, `7 do [a5]` 12 compiled
   for the interpreter's caught raise, silent on `main`) closed the same
   day: a closure a compiled program binds by `def` dispatches as the
   named fn it is inside every island (core's bridged Lookup), and `do`'s
   result model no longer claims the carrier; NUR194 (a written operand
   the contract does not take) closed the same day — the shape claim
   carries the wrapper's parameter types and the read window declines a
   token that does not fit, so the body islands to the bridged dispatch.
   The fn-util wrapper at the token seam (the same day) applies a
   self-contained Go fn value natively under each/fold/scan (callbacks.tsv
   L154 left the census) and the recorder's registry follows the running
   engine again after an inline module's body (module-composition.tsv L94
   and three `$module` rows left), and the third piece of the same change
   closed NUR143 — a fn unit's enclosing-binding snapshot reads the unit's
   OWN registry, so a module fn's read of its module-scope flex is the
   live binding (boru:sift's two descriptors left the region oracle's
   ledger) — and, seeing the right registry, DECLINES kg/main.boru
   (gomod.boru's `[[repo-entity] …]`, a module fn's body literal over a
   shared module-level map: the per-call spine the const rule could neither
   freshen nor share) — graduated again the same day by the selective
   freshen (the fresh push clones the literal's spine and KEEPS the
   embedded members, Program.ConstKeep; PR #225 P1's open item closed,
   the lang ledger 283 -> 282). The foreign-home fn value at the apply
   seam (the same day): a module fn value's own detached stamp is hosted
   nested at every dynamic-apply site where the frame entry declined it
   (callbacks L146, module-composition L100, module-fnvalue-boundary L51
   left the census). S3's first slice (the same day): a TOKEN body reaching
   the InvokeBody seam at run time — read from a flex, returned by a fn,
   passed as a List param — is stamped at run time (a synthetic fn over the
   tokens, typed by the seam's inputs, memoised by text or ID and input
   shape, the detached ref's freshness) and hosted; twenty of
   code-bodies.tsv's twenty-one rows and four elsewhere left the census.
   Open there: `args` inside a token body, a map literal bearing paren
   groups (the dyn-scope rescue's family), an identity-less body carrying a
   reference value. NUR195 (a flow sentinel in a computed body, swallowed
   by the compiled lane) closed the same day: the VM reads the flag a
   native's body run leaves set after every native call, as after a
   fallback — the enclosing loop ends, or the loop-less flow defers loud;
   NUR196 (a fn called from a LITERAL each body breaking: each's
   no-result error for the interpreter's flow_error) closed the same day —
   every iterating native ends its iteration on an escaped body
   (core.BodyEscaped) and returns no result, so the run resolves the flag
   on both lanes; the boru:test bodies reached the VM the same day (the
   handoff log's "the boru:test bodies reach the VM" entry: `Test.invoke`
   through the InvokeBody seam, a handler's throwaway signature over a raw
   body stamped at run time with params typed by the inputs —
   native.StampBodySig, the carrier's twin — the value-level shrinker's
   literal forms answered without an engine run, the recorder run
   attributed "stackform-record"; what stays is the generator over the
   opaque `r`, the dynamic-landing frontier); the flex-member map literal
   compiles the same day (the handoff log's "the flex-member map literal"
   entry: a folded flex or store keeps the recorded path, the member runs
   as a list element's inline region, canon.tsv's Vm.run round trips
   leave the census; NUR197 and NUR198 recorded on the way — a loop
   body's residual literal over the loop variable, and a dotted read
   through a missing member, each the interpreter's undefined_word for
   the compiled lane's value — both CLOSED 2026-09-25, the handoff log's
   entries of that date: the loop region evaluates its residual with the
   iterator bound, and the None receiver's atom row quotes a bare-word
   key; NUR208 and NUR209 found and closed with the first, NUR191 closed
   the same day — a named module fn call enforces the frame's return
   count on every path and parks its single returned closure — with
   NUR210 recorded pending); the keep-defs body lands the same day
   (the handoff log's "NUR199 closed — the keep-defs body" entry: a `do`
   body's defs install as kept dynamic-scope bindings the enclosing frame
   pops, the enclosing loop refreshes its carried slot after the call and
   a leaked name's read seats live; `for 3 [do [def t 5]]  t` was 0 for
   the interpreter's 5 on main — NUR199, closed — and code-bodies L180 /
   L186 compile, 13 -> 11; NUR201, a callee's def outliving a trapped
   raise on the interpreter only, is recorded against the interpreter —
   and CLOSED 2026-09-25 by the frame's error path, the handoff log's
   entry of that name: the interpreter's fault return tears down every
   fn frame the error leaves open on the tape, so `do [g]  t` is
   `[error(x) 0]` on both lanes);
   the multi-run keep-defs body closes NUR200 the same day (the handoff
   log's "NUR200 closed" entry: each / fold / scan bodies are keep-defs
   units too, every read of a leaked name seats live — the twin regime's
   value-name read fence is retired — and a mutated flex read back after
   a body seats live; code-bodies L183 / L190 and module-composition L92
   compile, the corpus's compile failures 19 -> 16); the variadic loop
   body lands the same day (the handoff log's "the variadic loop body"
   entry: a loop body whose one result is a 0-or-1 branch is admitted,
   the S5 first-value bind over such a loop declining instead;
   code-bodies L181 / L182 compile, 16 -> 14); the var splice's
   token sites land the same day (the handoff log's "the var splice's
   token sites" entry: `var`'s synthesized def and __varundef tokens carry
   the declaration name's position, so the root `do [var …]` twins adopt
   and place — code-bodies L197 compiles, 14 -> 13; NUR202 recorded on the
   way: a keep-defs body over a GRADUAL list runs as a const the native
   drives and its def does not leak, `[[1 1 1]]` for `[[1 2 3]]`, fenced); the
   keep-defs token body lands the same day (the handoff log's "the keep-defs
   token body" entry: NUR202 closed — the unapplied-fn gate exempts a unit's
   own untouched inputs so the each body over a gradual list compiles to
   its closure, and a run-time token body's stamp is a keep-defs unit whose
   kept installs the host hands to the enclosing trail; code-bodies L189
   compiles, 13 -> 12; NUR203 recorded on the way: the fn's read after a
   DYNAMIC keep-defs body keeps its compile-time home, 0 for 3, fenced); the
   user-call write-back lands the same day (the handoff log's "the user-call
   write-back" entry: a user call whose result a loop-carried root def
   writes back is promoted to a frame slot like a native producer, so
   `while [i lt 3] [def i (inc i)]` and its `apply` spellings compile —
   callbacks L89 and module-composition L104, 12 -> 10; NUR204 recorded on
   the way: a body def of the for loop's OWN index is the interpreter's
   leftover index level (2) and was the compiled lane's write-back (9),
   silent on main — the shape declines loudly now, fenced); filter's predicate
   seam for a Go-implemented value and a `flip`ped wrapper are the
   family's open rows. The run-time list bodies (code-bodies.tsv's twenty) are S3's runtime
   compilation. Most of the interp-entry census rows are still this
   family.
2. **S4's evaluating host** — the knowledge-graph generator dies at
   `DISPATCH_GENERIC at ev` and the kg gate is off until it lands.
3. **S5's remainder** — 8 provenance rows on the same seat as the join.
4. **What the branch-carried def does not reach**, each declining where it
   declined before: a `_`-prefixed arm def (`RecordDynBind` never records
   it), a pre binding from ANOTHER frame (a recursive callee's arm-local
   meeting the caller's — never seeded, or it commits the name to dynamic
   scope program-wide), a `word`/fn/type/module value bound in an arm, an
   arm def inside a computed `do` body.
   The reverse-order run's last batch (2026-09-25) closed NUR119,
   NUR105, NUR092, NUR091, NUR088, NUR084, NUR083, NUR082 and NUR081 (the
   handoff log's entries of that date, "the param's name on both lanes"
   down to "one contract for the family"), measured NUR109 and NUR089
   (still open, dated notes on the records), found NUR096's multi-return
   rows inert on the PRODUCTION registry (recorded on NUR096, still
   pending), and left the design-level records with dated review lines
   (NUR124, 123, 112, 104, 103, 102, 100, 097, 079, 078, 077, 076, 075,
   074, 072, 065, 064, 063, 060, 026, 009). After the merge with main the
   run went on down the register: NUR210 (the reach group's survivor),
   NUR204 (the lexical index scope, both lanes) and NUR203 (the dynamic
   body's leak) are CLOSED (the handoff log's entries of 2026-09-25), and
   below them NUR171 (the lens's own token), NUR141 (the predicate runs
   for real), NUR129 (the reach survivor's iteration) and NUR128 (the
   export-time analysis); then NUR124 (the value-delivered window parks),
   NUR123 (the guarded gradual read: the last shape is a loud designed
   defer), NUR097 (the late-binding hint), NUR077 (the StackForm Apply
   op, the apply word's double recording declined) and NUR109 (the parser
   name bound on a branch declines to the interpreter's kind lookup),
   NUR104 (Allowed: the install-time resolution of inline record fields)
   and NUR102 (one predicate run per dispatch on both lanes, and the
   carrier candidate's arm kept reachable), NUR134 (the unit-scoped trap
   and the caught Error; a def after a raise in a do body no longer leaks),
   and NUR101 closed on the
   register (its verdict landed on 2026-08-27, the last shape graduated on
   2026-09-22); NUR112 is narrowed to Stage 8
   (the member read's designed widening) and NUR103 resolved by the
   diagnostic-surface gate (its mini-redis instance ledgered). Main's #507
   is merged in (the handoff log's "The merge with main's #507" entry: this
   run's NUR205, NUR206 and NUR207 are NUR211, NUR212 and NUR213 now, main
   keeping NUR205), and main's NUR205 is CLOSED (the module replay's own
   bind: a module a loop or an arm may not bind as the replay does keeps
   no twin placement and declines); NUR214, found beside it (a fresh def
   in a loop that may run zero times bound anyway), is CLOSED (the loop's
   fresh cell), and NUR215 with it (a fn's dynamic read of a
   conditionally bound name raises the interpreter's undefined_word);
   NUR112 is CLOSED too (the plain check applies a stored fn member). NUR099 is CLOSED (a capitalised name over an undeclared fn body is refused with def_error; predicates are declared with `fnpred`, and the arity route is gone). NUR096 is CLOSED (the plain check applies a fn-shape-typed member over its argument window, so `c.op 10` checks as the shape's declared returns). NUR216 was found closing it and is CLOSED too (no residual arm applies a `/v`-quoted value: the class member's `c.op/v 5` is data on both lanes). NUR089 is CLOSED (an analysed body binds a fn-valued param or capture through the run's frame install, so an inline lambda checks exactly like its `/v` twin). NUR079 is CLOSED (a file-module import passes the native path's policy checks with `kind: "file"`, the restrictive profiles admit file modules, refusals are coded, and check / the pre-flight / describe / the LSP analyse under the run's profile). NUR078 is CLOSED (a bare fn name calls at every slot — the four clause-2 sites are gone, a reference is spelled `/v` everywhere, and a reach-read fn is a call head exactly when it would claim the next token; NUR190's Function-typed half dissolved with it). NUR218 is CLOSED (a member `/v` read is delivered as its word twin is — unquoted, stepped past — on both lanes, and a dynamic member at a branch arm is landed on the computed-arm merge). NUR217 is CLOSED (a stored fn value's unit declines a bare read no argument makes data, and lists the gradual params its body consumes, so every seam that runs it hands a call with a fn there to the interpreter's dispatch). NUR219 was found closing it and is CLOSED (a callback body unit lists the same slots, the pushed closure carries its source value, and a fn element in such a slot runs that value on the interpreter). NUR220 is CLOSED (the whole-frame replay parks an anonymous 0-arg lambda it re-steps as a value — a bare name read still fires — and an `apply` over a lone gradual lead declines instead of losing its Applied mark). NUR221 is CLOSED (the gradual apply event admits no lead the interpreter re-steps where it stands). NUR190 is CLOSED (a `/q` claim at the landing takes the landing's island — the value and the body from the word on, on the interpreter — or, inside an arm, a loop or a literal, a capture over the value and the word that skips the word's call and the apply after it). NUR222 was found closing it and is CLOSED (a dyn body's own lead is settled by the body, and the residual arm leaves it). NUR100 is CLOSED (a predicate is a one-value application through the matcher — every overload consulted, no parameter count — and the poly decline keys on a reachable overload that declares a re-step, not on a smaller arity). NUR223 and NUR224 were found closing it and are CLOSED (the callback seam discards an unconsumed unnamed input beneath the answer, as CallBoru does; a predicate's typed-def refusal is a type_error on both lanes, never a compiler-defect internal_error).

## Definition of done (ruled by the maintainer, 2026-09-14)

> done is a language that compiles, as a developer expects. all valid
> code compiles, no exceptions

Read as a checkable contract, this is T1 exactly as
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md) §1 states it, with **no
carve-out list**:

- Every program the interpreter accepts produces a `Program`. That is
  the whole contract, not one branch of two: `compile_failed` is not a
  result, it is a DEFECT against the contract — an unimplemented or
  unproven case, owed a fix and tracked to closure. **The
  `BORU_COMPILE_FALLBACK` hatch retired on 2026-09-19, with every other
  fallback:** the two `RunCompiled` carve-outs, the runtime-bail re-run,
  `Run`'s own fallback, the CLI's warn-and-re-run and its three compile
  modes, the fn-value seam's degrade, the detached-callback retry, and
  the `await` branch re-run. There is one outcome now, and the debt that
  was hiding behind them is counted in three ledgers
  (`test/go/langspec/compile_failures.tsv`,
  `lang/go/compile_defect_test.go`,
  `lang/go/test/compile_defect_test.go`).
- A refusal site is such a defect, and its disposition is the fix it is
  owed: exactly three are legal — a generic lowering, a trap that raises
  the interpreter's own error at the same moment, or deletion.
  "Carve-out" is not one of them, and neither is "leave it to the
  interpreter".

**Vocabulary — say what it is.** A program that does not compile has hit a
BUG. Do not call it a "refusal", in a gate name, a message, a commit, a
report or a conversation: "refuse" reads as a decision the compiler made
and was entitled to make, and it is not one. Say **compile failure**, or
bug, or defect. The counters are RATCHETS ON A BUG COUNT, never budgets
— a ceiling exists so the number cannot grow while it is being driven to
zero, and lowering one by deleting a corpus row rather than by compiling
it is the one move that is never allowed. The maintainer has had to
correct this twice; the 2026-09-18 sweep (PR #471 §11) fixed the gate
names, the user-facing message and the `compile_failed` error code, and the
2026-09-20 sweep finished the tree: **260 distinct "refus" identifier forms
→ 65, 4380 occurrences → 665**, every survivor an ENTITLED refusal (policy
denials, vault and proxy security, weak containers, option validation, exit
ranges, capability gates, `await` isolation, signature matching), where the
word is the right one. Do not add more in the compilation sense.
- "Valid" is decided by the interpreter. The checker may never be the
  reason a program fails to compile, so the whole-program "check
  diagnostics" sentinel goes (Stage 8's T1 half is inside done).
- Computed code is code: runtime-supplied bodies compile too (Stage 7 is
  inside done, and "runtime compilation must itself never refuse" is part
  of the contract).

**Framing correction (maintainer, 2026-09-16).** The interpreter is NOT a
fallback for the compiler, and it is not allowed to be one. Failure to
compile is a FAILURE — never "slow, not wrong", never an acceptable worst
case, never a co-equal branch of a two-outcome contract. The run-time path
that **silently** re-runs a refused program on the interpreter is
SCAFFOLDING that absorbs a known defect so the user still gets an answer —
and nothing in the run says the compile was refused, which makes that
failure worse, not milder: it hides itself. Where the sections
below describe that path, they describe machinery, not a sanctioned
outcome, and it never makes a refusal acceptable: every refusal recorded on
this page is a defect owed a fix and tracked to closure.

In [FULL-COMPILATION-ASSESSMENT.0.md](FULL-COMPILATION-ASSESSMENT.0.md)
§5 this settles the **T1 half** of outcome B: no compilation carve-outs,
so O4 is answered NO for compilation and "carve-out" is no longer a
disposition a refusal site can take. It does NOT choose between B and
B′, because that choice turns on T2's attributed set (question 1 below),
which the ruling does not address. Plan against B's figures as the upper
bound: Tier A plus the whole of Tier B (§6), with the generic lane (B1)
as the critical path, because it is the only mechanism that can give a
refusal site a "generic lowering" disposition in bulk.

Two questions the ruling does not settle, put to the maintainer the same
day and open until answered:

1. Whether a word whose ANSWER is the engine's own behaviour
   (`boru:debug`'s stepper and profiler, `RunTrace`) may still interpret
   at run time inside a compiled program. This is T2's attributed set,
   a runtime question, not a compilation one; the census currently treats
   those entries as specified and not debt.
2. Whether `boru check` agreeing with `boru run` (T4, the 279 rows where
   the checker reports an error the compiler compiles past) is inside
   "as a developer expects".

## The interpreter fallbacks are gone (2026-09-19, PR #476 — MERGED)

Read this first: it changes what every other number on this page means.

The maintainer ruled: *"remove all interpreter fallbacks, flags, modes, etc.
Failure to compile is a plain bug only."* That is the scaffolding
[COMPILABLE-SUBSET.md](COMPILABLE-SUBSET.md) §1 has always called scaffolding,
brought forward from its planned Stage-9 retirement. **PR #476 MERGED
2026-09-19 (`ba64e11` on `main`).** The vocabulary and dead-machinery sweep it
left owing landed on 2026-09-20 — see "The sweep that finished it" below.

**What went.** Ten mechanisms. The count is the finding — nobody had them in
one list:

1. the `"check diagnostics"` carve-out in `RunAutoValues`;
2. the fn-carrier-read carve-out beside it;
3. `BORU_COMPILE_FALLBACK=1`, the one-release hatch;
4. the runtime-bail arm (roll back, re-run the whole source);
5. `Run`'s own explicit fallback;
6. the CLI's try mode and warning, with `CompileOff`/`CompileTry`/
   `CompileForce`, `--compile`, `--no-compile`, `--force-compile` and their
   three environment variables;
7. the fn-value token seam's degrade;
8. the detached-callback retry (`InvokeCompiled` → `CallBoru`);
9. an `await` BRANCH's re-run of its raw tokens;
10. the REPL's per-line re-run and the HTTP exec handler's per-request one.

The C1 effect fence went with them: it counted output escaping the check pass
so a re-run could be blocked before duplicating it, and there is no re-run to
block.

**What stayed, and why it is not an exception.** `Vm.run`'s sub-engine
(`modules.CompiledSubRun`). It executes source constructed at RUNTIME, so
there is no ahead-of-time program to compile — "compiling" it is
parse+compile+run at run time, which IS the interpreter. That is tier 1 of
`compiled_metafallback_test.go`'s partition, the one category the project has
reasoned is genuinely irreducible. Removing it made a LANGUAGE WORD refuse
programs the interpreter runs (two `canon` rows, caught by CI on the
INTERPRETED spec gate). The compile there is an opportunity, not an
obligation.

Interpreter ISLANDS also stay, and the number is on the record because the
removal was built and measured before being reverted: unit-test compile
defects **284 → 292**, five pinned tests regressed on `error [...]` handlers
and quoted `do` bodies. An island is a compiled program with a COUNTED,
ratcheted interpreted span (`islandCeiling` 0 on the corpus), so removing it
buys no honesty and costs a core control-flow family. Stage 9 deletes it,
after those shapes lower natively — the order is the point.

**The contract now.** One outcome. A program compiles and its bytecode runs,
or it does not compile and that is an error naming the construct. Three
refinements the corpus forced, each of which had been getting it wrong in the
same direction — claiming the compiler's failure as the program's verdict:

- a failed CHECK PASS is a compile failure, not the program's error (the pass
  crashes on `each [if [gt 1] ['big'] ['small']] [1 2 3]`, which runs clean
  interpreted). A PARSE error is still the program's own;
- a blocking check DIAGNOSTIC names the compile failure rather than standing
  in as the verdict — the checker flags code the program never reaches;
- a handler's `internal_error` is the PROGRAM's. Only `core.IsVMDefer` marks
  the compiler's.

**Four ledgers carry the debt.** All ratchets on a bug count, never budgets:

| ledger | counts | at |
|---|---|---:|
| `test/go/langspec/compile_failures.tsv` | corpus rows that do not compile, per spec file | 21 |
| `lang/go/compile_defect_test.go` | unit-test programs: do not compile / compile then bail | 279 / 34 |
| `lang/go/test/compile_defect_test.go` | language tests answered on the reference engine | 111 |
| `test/go/langspec/compiled_defect_test.go` | corpus rows that compile and then bail | 52 |

The last is new and was needed: a program that COMPILED and then abandoned the
run used to be re-run, so the lanes agreed and five differential gates saw
nothing. `compiledDefect` classifies; `bookCompiledDefect` counts, and only
the corpus walk calls it, so the ceiling means one thing.

**Two numbers that look like improvements and are not.** Sweep compile
failures 36 → 31 and call-form failures 206 → 200: five seeds were classified
as failures because the classifier read the try-mode fallback's error as a
compile failure. They always compiled.

**What it exposed, which is the point.** Four genuine miscompiles the fallback
had been absorbing: `unresolvable type operand` after an `undef` (twice), a
`STORE_LOCAL` stack underflow in a net-zero `do` body, and **NUR170** (a fn
value read out of a Map and handed to a word taking `(Any, Map)` arrives
TRANSPOSED). **NUR171** and **NUR172** are the fifth and sixth, both at
the same site (`5 $.name apply`) and neither a miscompile. NUR171 is
position-only: the compiled no-match diagnostic carries no source position,
because neither the recorded `PolyNoMatchSpec` nor the debug table has one to
stamp. NUR172 is the two lanes' NOTES describing different argument windows —
`CALL_NATIVE_POLY` holds both operands and reports a type mismatch on the
second, while the interpreter never filled the forward slot and reports an
arity failure over one. There the compiled text is the accurate one, so the
fix points at the interpreter's matcher, not the VM; weakening the compiled
note to match would buy uniformity with a worse diagnostic.

Each is pinned in its own `pinLedger` (`knownPositionLoss`, `knownDiagDrift`),
not in `knownDivergences`: that map is checked by every corpus gate and its
entries must diverge on all of them, while a PRESENTATION drift is visible
only where presentation is asserted — which is the compile-or-fallback gate
alone. Both ledgers carry the same retirement half: a pin that stops drifting
on a full walk fails the gate.

## The sweep that finished it (2026-09-20)

What PR #476 left owing was TEXT and DEAD MACHINERY, not behaviour, and both
are now gone. Nothing in this section changes a gate value.

- **The `vmDefer` messages.** All 28 occurrences across `vm.go`,
  `vm_generic.go` and `vm_rematch.go` said "deferring to the interpreter"
  (or "to interpreter", or "deferred to"), false in every one. They say what the site actually does now — *the compiled
  runtime cannot execute it* — and the one gate pin that asserted the old text
  (`eng/go/compile_pipeline_cov_test.go`) was RE-READ rather than rewritten:
  its stated reason ("the interpreter re-runs and owns the canonical error")
  was false, exactly one test reaches it (`TestFnReturnCountDivergenceParity`,
  a `vm:poly-nout-drift` on `cvar2`), and it is now a ceiling of ONE pinned to
  that site's own text, with the real fix named — a `DeferAlt` at the site.
- **The C1 effect fence was DEAD MACHINERY, not merely a stale comment.**
  `ArmEffectFence` had zero non-test callers and nothing read the count: the
  arms went in #476 and the writer-wrapping half stayed, wrapping writers for
  nobody. The writer half and its three tests are deleted. `EffectLedger` /
  `NoteEffect` survive, re-documented as the observability seam they became —
  security-relevant tests read the count to prove a denied request never
  reached the network. `TestCheckPassIsEffectFree` was re-instrumented on the
  OUTPUT BUFFER, which is strictly stronger than the counter it replaced.
- **~200 stale comments.** Every "whole-program fallback", "sound interpreter
  fallback" and "falls back to the interpreter" that named removed machinery.
  The per-callback path (`InvokeCallback` with no stamped unit → `CallBoru`)
  and interpreter ISLANDS are still live, so those comments were kept — an
  explicit exclusion list, not a blanket sweep.
- **The `refus*` vocabulary: 269 distinct identifier forms → 77, 5447
  occurrences → 740** (measured 2026-09-20 over `[A-Za-z_][A-Za-z0-9_]*`
  tokens; see the correction below — the figures first published for this
  were wrong). Every survivor is an ENTITLED refusal, where the word is
  correct: policy denials, vault and proxy security, weak containers, option
  validation, exit ranges, capability gates, help renders, `await` isolation,
  `del`/process/debugger rejections. **`compiler`, `check`, `eng` and `basic`
  contain the string `refus` exactly zero times.** 131 test names, 50
  identifiers and 6 files renamed; `refusalCeiling` became `failureCeiling`,
  `refusalSiteCeiling` → `compileFailureSiteCeiling`,
  `refusalDispositionCeiling` → `compileFailureDispositionCeiling`; four
  EXPORTED symbols moved (`RefuseCarriedUndef`, `RefuseSpeculativeUndef`,
  `RefuseForwardStackDrift`, `RefuseStrandedMemberFn` → `Decline*`); the
  corpus TSVs' 133 `REFUSES:` descriptions became `DOES NOT COMPILE:`.
  Verb forms became *declines* (a code path declining to lower is a fact, not
  a claim of entitlement); nouns became *compile failure*.

  **THE MECHANISM IS UNTOUCHED, and this was only ever vocabulary:** 92
  `MarkUncompilable` sites and 34 `vmDefer` sites stand, and the ledgers do
  not move. Not one additional program compiles because of this work.
- **The dead `BORU_COMPILE_FALLBACK` references**, including two live
  `t.Setenv` calls on a variable nothing reads.

**A CORRECTION, and the method lesson under it (2026-09-20).** The first
published figures for this sweep — "260 → 65, 4380 → 665, every survivor
entitled" — were WRONG, and the count and the claim failed together for one
reason. The counting regex was
`[A-Za-z_][A-Za-z0-9_]*[Rr]efus[A-Za-z0-9_]*`, which requires at least one
character BEFORE `refus`: every identifier that STARTS with `refus`/`Refus`
was invisible to it, at both ends of the measurement. Hidden that way were
`refuseUndef`, `refuseStrandedMemberFn`, `refuseArrival`,
`refuseForwardStackDrift`, four exported `Refuse*` symbols, and — worst —
two GATE NAMES, which is the first thing the doctrine says to fix. So the
sweep reported itself finished while roughly 185 compilation-sense
occurrences stood, and the "every survivor is entitled" claim was false.
**A measurement that cannot see a class of its subject will report that
class as absent.** Sanity-check the instrument against a case you KNOW is
there before trusting a count — the bug was one anchor character, and it
survived a full review round because every number it produced looked
plausible.

**The method lesson this sweep paid for.** A blanket regex over a vocabulary
is a *refactor of claims*, and it breaks them two ways. It rewrote history
(past-tense narrative describing the OLD behaviour became false), and it
mislabelled: "the `group` word refuses non-string keys" is an entitled
decision, and renaming it a compile failure asserts something untrue — worse
than the word it replaced. Both needed a hand-built exclusion list. It also
hit 40 CODE identifiers (a local named `refusal` became `compile failure`,
with a space), which the compiler caught at once — the cheap half.

**Still owed on this line.** The island machinery, at Stage 9, after the
shapes it covers lower natively — the order is the point.

**A blocker this sweep uncovered: the knowledge graph cannot be rebuilt.**
`make -C kg graph` dies at `pc=8` with `DISPATCH_GENERIC at ev: the walk needs
an evaluation this host cannot perform` — the generic lane's EVALUATING HOST,
which [FULL-COMPILATION-REVIEW.0.md](FULL-COMPILATION-REVIEW.0.md) §2 names as
one of the three unbuilt cores. Verified A/B against a clean worktree at
`ba64e11`: **identical failure on unmodified `main`**, so it is pre-existing,
not this sweep's. `make -C kg verify` still PASSES on an untouched tree, which
is why nobody has hit it: it only bites when a cited document changes, and
then there is no way to make it green again.

This is the fallback removal's first real cost, and it is the intended one
made visible: a boru program that hits a compiler defect can no longer be run
at all, and the KG generator is such a program. The CLI has no interpreter
flag by design.

**The workaround, until the evaluating host lands.** Run the generator on the
REFERENCE ENGINE from a throwaway Go test — `lang.New()` then
`a.RunInterp(string(src))` with the working directory set to `kg/` — and
delete the test afterwards. Two traps, both paid for here: put the harness in
an EXISTING package (a new package changes the go-tree digest the graph
hashes, so the graph you just built is stale the moment you delete it), and
regenerate AFTER every document edit is final.

**The gate is OFF as of 2026-09-20 (maintainer's call).** Gating documentation
edits on a generator that cannot run blocks every doc change in the repo, so
`kg-verify` is deactivated in `scripts/ci-steps.sh` and the matching
commit-gate lane in `scripts/commit-gate.sh`. Both carry the reason and the
one-line re-activation; `make -C kg verify` and `graph` are untouched and
still run by hand. RE-ACTIVATE THEM WITH THE EVALUATING HOST — the gate is
worth having back, and the graph it guards is the fastest orientation in the
repo. CLAUDE.md and AGENTS.md say the same so a fresh session is not misled
into thinking a doc change owes a rebuild it cannot perform.

**Three method lessons this increment paid for.** They are the reusable part:

- **A fence is only as good as the arm it guards.** Removing the effect fence
  before the seams it protected left three fences reading clear and re-running
  unconditionally; the `await` test caught it printing `once` twice. An
  unguarded arm looks exactly like a passing one.
- **A booking that skips work is a fallback wearing a different hat.**
  Teaching a parity helper to tolerate a compile failure stops it reporting a
  compile REGRESSION, which is why every tolerant branch books against a
  ceiling; and injecting that booking as an early `return` in front of a
  helper's interpreter oracle swallows the oracle — six helpers reported that
  the ORACLE had moved when what had moved was the test.
- **A flag's meaning can expire under a census.** The bail census sorted on
  `wasCompiled`, which was equivalent to "did the program survive" only while
  a bail re-ran the whole program. CI called the result a regression; sorting
  on the defect class restored both ceilings EXACTLY (8 and 1), which is the
  proof that only the bucketing had moved.

## The arm-binding join — LANDED 2026-09-22 (the branch-carried def)

The 2026-09-21 read of this cluster — the census re-derived and then READ,
the bind side measured twice, the dyn-scope route built and ruled out by
its program-wide cost, the per-name frame slot specified with its three
obligations and the case it must decline — is history now and lives in the
handoff log (its dated entries of 2026-09-21, and "S5 — the branch-carried
def"). What survives here is the seat, because two ceiling comments in
`test/go/langspec/` cite it by name.

#### Where the join seats

At `RecordBranch`, on the LOOP-CARRIED def's own mechanism
(`NoteLoopCarried` — which the 2026-09-21 design had re-specified from
scratch without noticing it existed): one frame slot per NAME per unit
(`emitUnit.nameSlots`, shared by every loop and branch carrying the name);
every arm def a STORE into it at its own site (`emitDynBind.armCarried`,
first in `lowerDynBind`); the pre-branch binding seeded before the branch
(`emitBranch.carried`, top of `lowerBranch`) unless it already lives in the
slot; the joined carrier's identity aliased to the slot (`localByID`); and
a name with no pre binding bound in one arm only read through
`OpPushLocalBound`, which raises the interpreter's `undefined_word` on the
zero slot. The three obligations hold by construction, and the case the
design said must DECLINE compiles instead and raises where the interpreter
raises. The one measurement that decided it: a slot means "bound since this
frame started" — `for 2 [if (i eq 0) [def z 9] [] end z]` is `9 9`
interpreted — so nothing is ever re-seeded per branch execution.

## NUR175: the landing's WINDOW — read this before touching OpReStepLanding (2026-09-21)

A Codex review of PR #479 posted three P1 findings against the widened gate.
All three were REAL, and all three were ALREADY on `main` through the reach
spelling — the widening would have carried them to `get` as well, where the
residual apply had been answering them correctly.

`OpReStepLanding` applies over an EMPTY window; `execFnDefLiteral`, the step it
models, matches over the LIVE TAPE and the LIVE STACK. The op has neither and
cannot be given them: the tokens after the read compiled into LATER ops, and
consuming a stack operand would leave the stack shallower than the lowering
predicted.

| witness | interpreted | landed (before the fix) |
|---|---|---|
| `h` = `[] -> 42` and `[n:Integer] -> n add 1`; `5 m.f` | `6` | `5 42` |
| `h` = `[] -> 42` and `[x:Atom/q] -> x`; `m.f z` | `z/q` | `42 z` |
| `h` = `def h fn [[] [] []] end`; `5 m.f` | `5` | internal_error |

The corpus could not catch them: every fn-value witness it carried had a member
with exactly one 0-arg signature and one return.

> A fix that widens a model widens its holes with it. The measurement that
> shows the fix works does not show what the model was previously standing
> aside from.

**Fixed by two screens on the RUNTIME VALUE** in `reStepLanding` —
`FnValueOnlyZeroArgSigs` (settles the stack operand and the `/q` capture at
once) and exactly one declared return. Standing aside costs nothing: the read
keeps today's residual apply. Six new rows in `lang/spec/fn-value.tsv` §10.

Still a stand-aside, unchanged from `main`: a mixed-overload member read with
nothing after it and nothing on the stack (`m get 'f'`) is 42 interpreted and
`fn h(Integer)` compiled.

## NUR174: the landing's SEAT, corrected — read this first (2026-09-20)

The section below describes NUR173's fix as merged in #478. Its mechanism
stands; **its recording site does not**, and [NUR174](../NUR.md#nur174)
replaced it the same day.

NUR173 recorded the landing at the REACH-GROUP COLLAPSE. That made the model a
**whitelist of producers**, and `m get 'f'` — the same member read written as a
word call — had no collapse to see it, so it answered 42 interpreted and `fn h`
compiled exactly as `m.f` had.

The fix was not a second recording site. `noteReStepLanding` is called FROM
`stepLiteral`, in the branch whose next act is `execFnDefLiteral`, and a value
the loop PARKED never reaches it — so the model was already standing where the
interpreter decides. The gate is now the value's own callability;
`CheckState.ReachReSteppedFnIDs`, `recordReachGroupReStep` and the core-side
plumbing are DELETED.

> A model that stands where the decision is made does not need to be told who
> brought the value.

A narrower alternative WAS built before this was believed — recording at
`spliceMatchResults`, the site NUR173 itself predicted — and measured at 20,495
landing ops against the broad gate's 20,710, over one compile of every spec
row (the reach-only gate emitted 4,363). A 1% saving, because nearly every fn-typed
carrier the pass steps arrived from a dispatch splice. The economy argument
evaporated on measurement.

**Three rungs of `execFnDefLiteral` came with it**, each found by a probe
written against the interpreter's source rather than by running the corpus, and
each a wrong answer on its own: the ANONYMOUS-0-ARG PARK (a lambda value that
matched nothing is data — `module-fn.tsv:L47`), a DISPATCH MODIFIER (`m.f/v`
answered 42 against `fn h`), and a value still ALONE INSIDE A LIVE REACH GROUP
(where the group's job is to produce the value, not call it — NUR035). The
first was reproduced under the NARROW gate too, which is what settled the
design: a producer list protected against none of them.

**Engine entries end where they started, 422** — the park takes two
curried-chain rows off the interpreter (`bytecode-migrated.tsv:L285`,
`callbacks.tsv:L150`, which were paying a `RunResolved` entry to reach the same
"stays data"), and NUR175's two 0-RETURN witnesses add two back, because the
landing stands aside from those and their residual apply islands. Every other
gate is at its ceiling and the sweep did not move. Eighteen new rows in
`lang/spec/fn-value.tsv` §9 and §10 prove both increments.

Still open from NUR173's list: a collectable token after the survivor, a
variadic producer's region top, the sweep's two `def container` CRASH cells.

## NUR173 is FIXED, and this page's own first account of it was wrong (2026-09-20)

Read this before picking up the fn-value line. Its RECORDING SITE is superseded
by NUR174 above; everything else below stands.

**The defect.** `m.f` is not a dot operator at the tape level: it lowers to the
REACH GROUP `( m dot f )`. That collapse never parks — an unmarked dot-read of
a function is a CALL (NUR038) — so the rewind lands ON the one value it leaves
and `stepLiteral` RE-STEPS it, dispatching a callable one. The check pass holds
a CARRIER there and steps past it as data. `recordParenReStep` excludes reach
groups (its contract is the more-than-one-survivor case) and every
fn-value-call arm of `resolveDynamicApply` needs a second residual entry, so a
lone survivor reached no arm at all and the program pushed the runtime fn as
DATA. `def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end m.f` was 42
interpreted and `fn h` compiled — silently, on `main`, with no gate lifted.

**Two corrections this page owes.**

1. NUR169 said "no case for `count == 1`". That MECHANISM is right — the seat
   is one function out, at `recordParenReStep`'s exclusion and
   `resolveDynamicApply`'s `len(residual) >= 2`.
2. This page's earlier entry said *"not the paren — bare `m.f` and `(m.f)`
   diverge identically"*. **Both spellings lower to the same paren**, so that
   control varied nothing. A control that cannot vary its variable proves
   nothing, and this is the week's second measurement bug of that shape (the
   first was a counting regex blind to its own subject, PR #478). **Vary the
   axis at the level the MACHINE works at, not the level the source is written
   at.**

**The fix, and why it is a runtime one.** Declining instead — a carrier
receiver whose member type still admits a Function — was built and measured at
**53 -> 282** compile failures: nearly every computed container is `Map`-of-
`Any` to the check pass, so there is no static answer. The decision belongs to
the value, so the collapse records what it alone knows
(`CheckState.ReachReSteppedFnIDs` — the part NUR174 replaced), `check`'s
`noteReStepLanding` NOTES the
producing event as owing a landing, and `OpReStepLanding` — emitted right after
that event's own op — islands an unquoted appliable value ALONE. `Run` over one
token IS `stepLiteral`'s re-step, so a matching signature runs and a
non-matching fn stays data with no no-match raised. The note consumes nothing,
splices nothing and DECLINES nothing, which is what lets it sit last without
disturbing the three models above it.

**It stands aside four ways, and none of them declines:** a collectable token
written after the survivor (the alone-island cannot take it — `Cli.parse
{name:"x"} ["x"]` ran an export's body with its parameters unbound), a
MULTI-result event (which of several the rewind lands on is not this model's
to guess), a variadic producer's runtime-variable region, and a value with no
producing event to hang the op on. Declining those was measured at **53 ->
181** corpus compile failures, because `def x m.y` is the commonest shape in
the language — which is also why the op is emitted at the top of
`seatCallResults`, the one moment the result is on the stack on every path,
promoted to a frame slot or not.

**Cost, measured.** Every gate unchanged — the four ledgers, the 92
`MarkUncompilable` sites, and the generated sweep diffed cell by cell against a
clean-worktree baseline (zero regressions, zero movement). The sweep does not
move because it has no statement-tail member read off an EVENT-RESULT
container. What proves the fix is **twelve new rows in `lang/spec/fn-value.tsv`
§8**, every one of which answered wrongly before it.

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

**Three holes remain, all named in [NUR173](../NUR.md#nur173):** the `get`-WORD
twin (`m get 'f'` is not a reach group, so nothing records its landing), a
collectable token written after the survivor, and a variadic producer's region
top. The sweep's two `def container` CRASH cells (`CALL_DYNAMIC underflow`,
`SWAP underflow`) are still open; the FIRST draft of this fix closed both, so
the shape is reachable from here, but which stand-aside holds them is
unmeasured — and this page has already been wrong once today about a cause it
had not measured.

## Where the project is

The gates carry two numbers each (`test/go/langspec/lanes_test.go`): an
END STATE — the design's number, asserted by the direction lane
(`BORU_DIRECTION_GATES=1`, `make test-direction`, red by design until the
work is done; CI renders the lane's table from the regression shards into
every run's summary) — and a REGRESSION ceiling, the last merged value,
which only falls and which the default lane asserts.
`make gate-status` prints both for every gate, refreshes
[../test/go/langspec/GATE_STATUS.md](../test/go/langspec/GATE_STATUS.md)
and appends the instant censuses. **That generated file is the authority; this
table is a copy and goes stale** — it sat at 2026-09-19's values for two days
while four ceilings moved. The values below are this branch at the head that
lands the branch-carried def, 2026-09-22 (`main` at 49354a6 plus S5's first
slice), and the whole table reads **0 regressions, 11 open, 2 at end
state**:

| gate | live | end state | what moved it |
|---|---:|---:|---|
| compile failures | 21 | 0 | **20 → 21 on 2026-09-22** (NUR156: module-composition L104 GRADUATED from the known-divergence ledger — a runaway loop because its apply never fired — to a loud decline in the dynamic-scope def family, the S1a trade). Before: **23 → 20 on 2026-09-22** (the container reads inside quotation bodies: a code-body closure unit takes the whole-frame replay for a fn-valued member read at its tail — each-variants L205, fold-map-filter L215, module-composition L98; NUR182 and NUR183 closed on the way). Before: **24 → 23 on 2026-09-22** (the curried chain: a paren over a produced closure records its re-stepped lead's apply at the collapse — callbacks L151; NUR178 and NUR179 closed on the way). Before: **27 → 24 on 2026-09-22** (the container-member calls: `(fs.b 10)` over a produced closure — callbacks L60/L73, module-composition L96). Before: **32 → 27 on 2026-09-22** (the dynamic-lead group: the `apply` word at the main program over a gradual lead or a produced fn-typed carrier — callbacks L40/L41/L50/L51, module-composition L100). Before: **45 → 39 → 32 on 2026-09-22** (S1b-3 re-landed: a fn value stored in a container — set/push/unshift/append declare CompileStoresFn; then S1b's apply shapes: the leading one-arg fn-carrier window records inside a branch arm, a loop body and a callback lambda, over a typed gradual or an inert fn-valued argument — callbacks 17 → 10). Before: 113 at the 2026-09-17 corpus expansion (+710 rows of ordinary idioms); every one a BUG in COMPILABLE-SUBSET.md §5, not a policy. Since P0 the ceiling is the sum of `test/go/langspec/compile_failures.tsv`, one line per spec file, asserted per file under `BORU_SPEC_FILES` too. 113 → 60 (S1a) → 53 (S1b-2) on 2026-09-19; 53 → 62 on 2026-09-21 (PR #482) — nine paired FALSE-PATH witnesses for the `fn-locals-scope` §6 arm-binding cluster, debt written down; **62 → 45 on 2026-09-22 (S5's first slice, the branch-carried def)** — the seventeen §6 rows compile with parity on BOTH paths, and twelve new witness rows (§6b, §6c) compile too |
| compute gaps | 16 | 0 | **15 → 16 on 2026-09-22** (NUR156: L104, the same graduated row). Before: **18 → 15 on 2026-09-22** (the container reads inside quotation bodies: each-variants L205, fold-map-filter L215, module-composition L98). Before: **19 → 18 on 2026-09-22** (the curried chain: callbacks L151). Before: **22 → 19 on 2026-09-22** (the same three rows). Before: **27 → 22 on 2026-09-22** (the same five rows). Before: **41 → 27 on 2026-09-22** (the same two landings: seven stored-fn rows, six apply-shape rows, and callbacks L61 moved to the reducible bucket — 3 → 4 there, a partition move). Before: 107 at the expansion; three fell when NUR153 closed; 104 → 56 (S1a); 56 → 49 (S1b-2); 49 → 58 on 2026-09-21, the same nine witnesses; **58 → 41 on 2026-09-22**, the seventeen leaving together |
| diagnostic parity / armed-only | 349 / 9 | 0 / 0 | unchanged on 2026-09-22, row for row — callbacks L139 diverged for one measurement (a plain-check false positive on a fn-carrier window over a lambda literal) and the plain surface was fixed the same day. Before: checker debt the expansion exposed; both fell at S1b-2; 351 → 353 and 11 → 13 on 2026-09-21 — exactly TWO of the nine witnesses diverge, measured not assumed: L179 and L181, the two OPERAND-spelling rows, each mirroring a pre-existing armed-only twin (L178, L180) diagnostic for diagnostic; **353 → 349 and 13 → 9 on 2026-09-22** — those four rows compile, and leave both ledgers together as predicted |
| interpreter islands | 0 | 0 | 12 at the expansion, all fn-VALUE callbacks; two fell when NUR153 closed; the last ten at S1a. **At end state** |
| interpreter-only rows | 0 | 0 (ceiling 3) | **at end state**; the ceiling keeps headroom for a genuine irreducibility claim |
| interp-entry census rows | 75 | 0 | **74 → 75 on 2026-09-23** (the recovery's window, NUR180: generics-fn L55 GRADUATES in — it did not compile at all before, the check pass stopping at a false `undefined word: value`; its fold body over a generic element runs as a raw token body at the callback seam). Before: **77 → 74 on 2026-09-22** (the container reads inside quotation bodies: none of the three compiled rows enters, and callbacks L103, callbacks L55 and module-composition L76 LEAVE — a closure body's probe compile no longer leaves an orphaned stamp on a capture-free fn value, so the fn-value seams run its unit VM-native). Before: **78 → 77 on 2026-09-22** (the curried chain: callbacks L85, `each ([k:Integer] => [((mk k) 100)]) [1 2 3]`, leaves — the chain inside the callback records natively and the callback runs as a closure unit; Engine.Run 418 → 415 with it). Before: **77 → 78 on 2026-09-22** (the dynamic-lead group: module-composition L100 compiles and its apply of a fetched MODULE export runs through the re-step island — the module-fn seam; the S1a trade in miniature). Before: **80 → 77 on 2026-09-22** (S1b's apply shapes: three `each ([f:Function] => [(f n)]) fs` rows compile their callback as a closure unit instead of running it through RunResolved — callbacks L54/L105, fold-map-filter L200). Before: 54 at the expansion; 52 → 102 (S1a, the G-lane-first landing); 102 → 77 (S1b-1); 77 → 78 (S1b-2); 78 → 80 on 2026-09-21 (PR #481, the two 0-RETURN `fn-value.tsv` witnesses standing aside onto the residual apply) |
| engine entries / runtime defers | 408 / 8 | 0 / 0 | **406 → 408 on 2026-09-23** (the same graduated row, one Engine.Run and one RunResolved). Before: **415 → 406 on 2026-09-22** (the container reads inside quotation bodies: the three rows that leave the census ran their fn value through the stepping island per element). Before: **418 → 415 on 2026-09-22** (the curried chain: callbacks L85's three elements, one Engine.Run + one RunResolved each, leave with the row). Before: **416 → 418 on 2026-09-22** (the same row). Before: **422 → 416 on 2026-09-22** (the same three callback rows, six entries). Before: 379 at the expansion; thirteen fell when NUR153 closed; 366 → 489 (S1a); 489 → 419 (S1b-1); 419 → 422 (S1b-2). Unchanged since |
| locally-resolved defers | 1 | 0 | `vm:poly-no-match×1` — a caller's own fallback absorbed, the program staying compiled |
| reducible (tier-2) rows | 3 | 0 | word-class gaps the compiler does not model |
| correct-error compile failures | 1 | 0 | a known-to-error row must compile an OpTrap / RET error path |
| type-soundness violations | 4 | 0 | checker debt the expansion exposed; 5 → 4 on 2026-09-22 (NUR156: L102 is sound once the `apply` model unquotes its lead) |
| known divergences (`knownDivergences`) | 1 | 0 | NUR154 — the ledger is pinned both ways; NUR155's row left it on 2026-09-23 (the typed callback's contract: it agrees), NUR156's three rows on 2026-09-22 (two agree, one declines loudly) |
| `MarkUncompilable` sites / undeclared handlers | 92 / 94 | 0 / 0 | sites unchanged since 2026-08-25 and the count ONLY FALLS — a new compile-failure site is new debt, which is why a screen must decline through an existing site rather than latch its own; handlers 114 → 94 on the migration line |
| the generated sweep (S0): seeds failing / islanded / diverged | 29 / 2 / 6 | 0 / 0 / 0 | `test/go/langspec/SWEEP_STATUS.md` is the list; the divergences are NUR154, NUR161 and the rest of `sweepKnownMiscompiles`, pinned; NUR156's seed graduated 2026-09-22, the `word` × factory seed 2026-09-23 (NUR181), the `if` × named-fn seed 2026-09-23 (NUR159, the branch result's re-step) |
| routed dispatches / oracle reproduced | 676 / 446,999 of 473,151 | — | increments 62–65 |

**Two ledgers the gate table does not show, and a third that is easy to miss:**
`lang/go/compile_defect_test.go` (`compileDefectCeiling` 284, `bailDefectCeiling`
32) and `lang/go/test/compile_defect_test.go` count programs in the UNIT suites
that do not compile, or compile and then bail. Their report prints only on
failure or under `-v`, so a passing run shows nothing. A change can be green
across every `langspec` gate and still move these.

The forecast and the probabilities are the review's §2.3; refresh them at
the end of each step of §5, not each increment.

## What is next

**The immediate items are listed at the top of this page** (the OPEN RIGHT
NOW section): the remainder of S1b's apply shapes (the first slice, the
S1b-3 re-land, the dynamic-lead group and the container-member calls
landed on 2026-09-22), S4's evaluating host, S5's eight remaining
provenance rows. The two items that led here on
2026-09-21 closed on 2026-09-22 (the branch-carried def); the re-estimate
is [FULL-COMPILATION-REPLAN.0.md](FULL-COMPILATION-REPLAN.0.md) §11 —
**60–108 session-days remaining**, T1 + T2 by year end about 30%.
Everything below is the standing programme they sit inside.

**One lesson from 2026-09-21 that applies to every increment on this line**,
because it cost a full revert: `module-sift.tsv` (65 rows) and
`TestRealProgramsCompile` (62 programs) are the load-bearing regression signals
for anything touching operand resolution, and NEITHER is in the commit gate's
fast lane. A screen can pass `compiler/go`, `lang/go`, `core`, `check`, `eng`
and every hand-written witness and still destroy both. **Run the full
unfiltered corpus (`-timeout 40m`; the default 10m kills it mid-run and the
goroutine dump looks like a failure it is not) before believing a change of
that kind, and before pushing it.**

> **Read [FULL-COMPILATION-REPLAN.0.md](FULL-COMPILATION-REPLAN.0.md)
> first (2026-09-18).** It re-estimates the remainder at **75–130
> session-days** (the review said 105–175, measured two hours before the
> velocity work landed), and changes the order in four ways: **P0** per-file
> compile-failure ratchets go FIRST — `BORU_SPEC_FILES` reported counts
> instead of asserting them, which is how two regressions reached a working
> tree on 2026-09-18 while six-second filtered runs stayed green — **P0
> LANDED the same day**: `test/go/langspec/compile_failures.tsv`, one
> compile-failure count per spec file, asserted for every file a run walks,
> filtered or not (`compile_failure_ledger_test.go`). **S0 STARTED the same
> day**: the generated sweep — `test/go/sweep` (the seed table
> `seeds.tsv`, one hand-written program per declaration-relevant word ×
> operand kind), `TestGeneratedSweep` (its gates, asserted under a corpus
> filter too) and `make sweep-status` (the matrix,
> `test/go/langspec/SWEEP_STATUS.md`) — whose first run found NUR159–163:
> three miscompiles, a compiler panic, an interpreter non-uniformity. S0
> still owes the module exports as rows, signature-level cells, and the
> corpus ratchets re-based on the sweep; **S1a**,
> the gradual-Any collection overload commitment, is carved out ahead of S1
> as 19 rows on one mechanism — **S1a LANDED 2026-09-19**, in one
> session-day: each/fold/scan/filter declare `CompileDynBody`, the nineteen
> rows and thirty-one more compile (113 → 60), islands 10 → 0, at the cost
> of the interp-entry census (52 → 102) and engine entries (366 → 489),
> which is the G-lane-first landing S1b retires — **S1b STARTED the same
> day**: its first increment makes the fn-value seam native (a callback
> value runs its unit on the VM, stamped at first application and memoised
> on the value — the review's "unit half"), census 102 → 77, engine entries
> 489 → 419, no compile-failure change; its SECOND increment the same day
> resolves a computed fn value at a forward slot (the collection seat and
> the `/v` read consult the fn-carrier side table, so `each f/v xs` over a
> factory's result dispatches instead of refusing) and makes a fn-VALUE
> CLOSURE a fn value at every callback seam — matched against its own
> signature before its unit runs, which is what S1a's release had left
> unsound — compile failures 60 → 53, census 77 → 78 and engine entries
> 419 → 422 on the single wrapper row that newly compiles; what S1b still
> owes is in the handoff log's S1b entries; a THIRD increment was built,
> measured and REVERTED the same day — `set`, `push`, `unshift` and
> `append` declaring CompileStoresFn is worth six rows (53 → 47) and is
> sound in itself, but it retires a gate that was MASKING NUR169, a paren
> netting one fn value that the interpreter applies and the compiled lane
> silently does not; re-land it once that apply lowers; **S2 splits**, because only 35 of its 94
> signatures are a sweep and the other 59 need a mechanism that depends on
> S1b; and S2 is judged by `undeclaredHandlerCeiling`, never by the compile-
> failure count. The binding gate is the full unfiltered corpus at about
> twelve minutes, and it only halved — the project got about 1.4× faster,
> not fifty.

The review's §5, in order: **S0** the generated word-inventory sweep
(the corpus is a sample and under-measures by construction); **S1** fn
values as one convention (the 12 islands, 59 refusals and 23 census rows
are one family; NUR153 was ruled on 2026-09-18 — the tape rule
everywhere — and its implementation opens S1); **S2** in parallel, the
handler-migration line —
[HANDLER-MIGRATION-LINE.0.md](HANDLER-MIGRATION-LINE.0.md) is a second
session's brief and `make handler-worklist` its list; then S3–S7. The
rulings each step waits on (O2, O4, O5, the attributed set, NUR110,
NUR078) carry a recommendation each in the review's §6; NUR153 is ruled
(2026-09-18, the recommendation). What was
in flight before the review (increments 69–73's detail, the parked
increment 58, the earlier candidate list) is in the log under "Moved from
SESSION-HANDOVER.0.md (2026-09-17)".

## Process rules this line has paid for

The first is the one that cost the most, four times in one session:

1. **Re-run the gate that owns the edit — including when the edit looks
   cosmetic.** `make commit-gate` (three minutes, on what the change
   touched) before every commit. A refactor made *while* fixing something
   else is itself the trigger to re-run everything. Four separate red CIs
   traced to skipping this: the variation lane, `gocyclo`, the ADR-008
   statement floor, and a stale knowledge graph on a commit that contained
   nothing but a design note.
2. **"CI green" ≠ "gated".** `ci.yml` runs `make test` + `cover-gate-core`.
   The repo-wide merged ADR-008 gate is a SEPARATE workflow
   (`cover-gate.yml`, nightly + `workflow_dispatch`). Dispatch it on the
   branch and wait for it before merging. It has two stages — stage 1 is
   `make test` under a coverage profile (where `TestVariationDifferential`
   lives), stage 2 is the 100% statement floor — and the stage tells you
   which kind of failure you have.
3. **A `what remains is…` narrowing inherits the last reader's vantage
   point.** Two frontier notes in a row pointed one layer off the real gap.
   Re-derive from the refusal, not from the sentence.
4. **A count test may be asked of the probe; a SHAPE test may not.** The
   probe carries no `producedBy`, so a const there can be an event in the
   real compile.
5. **Never run two jobs that write `kg/out` concurrently**, and never
   `git add -A` while one is running — that commits a half-written graph.
6. **A pin that stops failing has not necessarily graduated.** Check which
   claim it was making.
7. **A refusal message is a claim about the code, not a measurement of it.**
   Two different shapes shared one sentence for twelve increments, and a test
   pinned the generalisation as a contract with its reasoning written out —
   which is the form in which a wrong claim is hardest to see. Increment 59
   was four lines once the print was read.
8. Recording a non-uniformity in `NUR.md` is mandatory and not subject to
   maintainer instruction; the **Allowed** verdict is what needs the
   maintainer. Never add to `ADR.md` unless explicitly instructed.
9. **Work by mechanism, not by row.** A family-level increment retires ten
   to twenty rows; a row-level one retires one to three and one in seven
   is reverted. A new shape lands on the generic lane FIRST and takes a
   typed lowering later, by proof — every miscompile of 2026-09-17 was a
   bespoke typed lowering, none in shared-kernel code — and the count of
   `vm:generic-*` defer arms may only fall (FULL-COMPILATION.0.md §10.1).
10. **`make commit-gate` before a commit, `make ci-local` before a push.**
    Both are three-minute contracts: the commit gate on what the change
    touched, CI as parallel jobs each under the ceiling (`scripts/ci-steps.sh`
    is the one definition `ci.yml` and `ci-local` share). The regression
    lane is what blocks; the direction lane is the gate table CI renders
    from the shards. A regression ceiling is raised only with the row that
    moved it named in the constant's comment; an end state is never raised.
11. **Iterate on one family with `BORU_SPEC_FILES`**, and run the whole
    package (`make test-langspec SHARD=n` for every shard) before the push:
    the corpus-wide ceilings, floors and both-ways ledgers are reported,
    not asserted, under a filter. The per-file compile-failure ledger
    (`compile_failures.tsv`) is the exception: it asserts on every file the
    filtered run walks, both ways, so a change that compiles a file's rows
    lowers its line in the same change, and one that stops a row compiling
    fails the six-second run with the rows named.

## Instruments

- `TestInterpEntryCensus` — the only thing that sees a compiled program whose
  BODY is interpreted. `-force-compile` reports SUCCESS for those.
- `TestVariationDifferential` — transforms each corpus row; catches what a
  new corpus row's own gates cannot. Runs in `make test` and the merged
  coverage gate, NOT in the fast per-PR checks.
- `TestMultiRunBindParityOracle`, `TestFrontierSpec*`, `TestCorpusThreeModes`.
- A temporary `println` at the decision site beats reading the code. Several
  increments' real causes were found that way and only that way.
- `BORU_SPEC_FILES=<names or globs>` — every corpus walk in langspec and
  the interpreter oracle (`TestSpecProd`) over the named files only; the
  per-file compile-failure ledger still asserts for those files. Every
  walk runs on all cores through `specWalk` (`walk_test.go`);
  `BORU_SPEC_WORKERS=1` is the sequential, directory-ordered form for a
  temporary println.
- `make gate-status` / `BORU_DIRECTION_GATES=1` — every gate's live value
  against its end state and its regression ceiling; the ledgers
  (`knownDivergences`, `regionOracleFindings`, `diagSurfaceLedger`) are
  pinned both ways whichever lane runs.
- `make handler-worklist` — the Stage-6 list, one undeclared signature per
  line; `make cover-gate` is cached per module (`COVER_FRESH=1` to redo)
  and profiles every module before failing.
- `TestGeneratedSweep` / `make sweep-status` — the generated sweep (S0):
  every declaration-relevant word × operand kind (`test/go/sweep/seeds.tsv`)
  through both engines and every call form of `vary`'s transform table,
  in about 15 s; its word × kind matrix is `SWEEP_STATUS.md`, its counts
  are ratchets that assert under `BORU_SPEC_FILES` too, and a divergence
  is a miscompile pinned to its NUR (`sweepKnownMiscompiles`). A program
  that panics an engine or blocks past `vary.Deadline` names itself
  instead of taking the run down.
- The spawn seams (`timeout`, `interval`, the model watcher, the net
  acceptor and each connection) all run their bodies on a fork; a
  parent-minted callback stays on the fork (NUR152's `FnHome`), pinned
  under the race detector by `TestTimeoutBodyAppliesParentFnOnItsFork` and
  `TestModelWatchForkNoRace` (40/40 under load on 2026-09-17). The
  registry a unify is armed with is threaded through the kernel, not
  kept on a package-global stack (2026-09-18; the parallel corpus walks
  found the race); `TestUnifyRegistryArmedConcurrentNoRace` pins it and
  CI's race gates run it. NUR157 is the one oddity that threading kept.

## The live miscompile of 2026-09-21 — it was NUR110, and it is CLOSED (2026-09-22)

The six shapes measured on 2026-09-21 (`if b [def z 9] [] end z` with
`b=false` answering 9; the top-level `[0 1]`, `[1]`, `[0 Integer]`
residuals; the leaked nil) were [NUR110](../NUR.md#nur110), recorded
2026-08-28 with a pinned unit test inviting its closure, and the two fixes
that record had already built and rejected (a refusal at the join: 131
rows; a refusal at the read: the `t2` false positive) are the reasons the
fix has the shape it has. The withdrawn 2026-09-21 screen's five failure
modes are answered the same way: the fix keys on nothing the screen keyed
on — not a name, not a fragment id — but on the joined carrier's own
identity, which only a read past the merge ever carries. All six shapes
now raise `undefined_word` compiled, at the read's position, with the
interpreter's did-you-mean; `TestCondBodyFreshDefRaisesLikeInterpreter`
and `TestBranchCarriedDefParity` (twenty-four shapes) pin it, and
`fn-locals-scope.tsv` §6b/§6c carry the witnesses. The record's closing
note in NUR.md has the mechanism; the handoff log has the measurements.
