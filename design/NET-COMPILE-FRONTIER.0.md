# NET-COMPILE-FRONTIER — what blocks the boru:net apps from compiling

> **ADDENDUM 6 (2026-07-11 — mini-s3 is FULLY STAMPED: 23/23, zero
> refusals).** The last wall ("for: body nets multiple values per
> iteration" on s3-handle-get/one) was a DISJUNCT-carrier modeling gap,
> not a loop feature: the recovered `slice` over the dynamic service
> read synthesises a Bytes|List|String disjunct (`part`), and the
> armed-compile arg generalisation's default arm bound s3-send-resp's
> `body:Bytes` param to a PAYLOAD-STRIPPED Disjunct carrier
> (`NewCarrier(a.Parent)` — alternatives and the dynamic flag lost),
> which matches NO overload; the chunk loop's `slice i hi body` then
> cascaded no_signature contagion into the multi-value refusal.
> s3-send-resp's OWN stamp never saw it (its params generalise gradual
> via fnValueInputs). Fix: a disjunct-carrier arg flowing into a
> concretely-typed param narrows to the declared type — the same
> entry-guard contract as the recovered-Any narrowing arm directly
> above it (the CALL_USER guard raises exactly where the interpreter's
> dispatch would), preserving the dynamic flag; the
> narrowArgsToParams twin gets the strict-disjunct arm (its dynamic
> case was already covered by the bound-mismatch arm). Pinned in
> lang/go/test/stamp_disjunct_narrow_test.go (fails pre-fix with
> exactly the mini-s3 refusal). `-compile-report` over the full
> PUT/HEAD/resume/range/list/delete flow: **23/23 stamped units, byte-
> correct output** — every named fn and every anonymous handler in
> mini-s3 and mini-s3-client executes on the VM. Remaining artifact
> (non-app): a top-level DRIVER program refuses whole-program compile
> with the for-multi reason — the top-level pass runs without the
> detached stamp's gradual modality, so the same chain models
> differently there; the driver runs interpreted and calls the stamped
> units, so nothing app-level is lost.
>
> **ADDENDUM 5 (2026-07-11 — the residual-timing fork is UNIFIED; the
> parse-range wall is closed).** The addendum-2 fork is resolved by an
> interpreter decision, shape-based:
>
> - **Multi-token fn body**: a trailing pending container evaluates
>   IN-frame — before the body-local defs pop — on EVERY dispatch path.
>   The CallBoru path always did this (the sub-run's end-of-run drain);
>   the same-registry SPLICED path deferred it to the consumer's scope
>   (where the body-locals are gone → `undefined_word: from`), because a
>   spliced frame has no sub-run and the eager top-engine drain does not
>   apply inside a sub-engine. The frame's DefCleanup marker now carries
>   `EvalResidual` (set when the body has more than one token) and
>   evaluates the frame's residual pending containers before the def
>   truncation — the frame is a paren group whose nested groups have
>   already collapsed, so the scan down to the frame's own OpenParen
>   touches exactly the frame's residual. All three tail-builders
>   (buildFnBodyHandler's skeleton + slow path, execFnDefSig) set the
>   flag; the TCO eager teardown runs the same eval; the break/continue
>   unwind skips it (the region's values are discarded).
> - **Single-literal container body**: UNTOUCHED — the pinned
>   no-closures transparency (def-node-binding.tsv §3): top-level
>   spliced resolves the MODULE binding; the CallBoru module path keeps
>   its in-frame answer (pre-existing split, now documented and pinned in
>   lang/go/test/stamp_residual_map_test.go). The whole-program compiler
>   keeps refusing the shape ("body result of unknown provenance").
>
> With the runtime unified, the recording admission widened soundly:
> `runFnBodyOnce` marks the body sub-engine elemEvalRecordable for
> callback bodies AND multi-token bodies, so s3-parse-range's trailing
> `{from: from upto: upto}` records its OpMakeMap assembly and the
> "fn s3-parse-range: body result of unknown provenance" refusal class
> is GONE. New spec rows pin the multi-token twins (def-node-binding.tsv
> §3); full estates green. mini-s3 stays 21 stamped with byte-correct
> output — the two `s3-handle-*` units now refuse one wall further in:
> "for: body nets multiple values per iteration" (s3-send-resp's chunked
> send loop, compiled NESTED under the handlers, models a leftover
> `[__FN, List, dynamic-Any]` in the loop body — send-resp's own stamp
> compiles the same loop clean; undiagnosed, next up).
>
> **ADDENDUM 4 (2026-07-11 — s3-serve is closed; only the fork remains).**
> `s3-serve`'s "finalize left the unit unstamped" was a **dynEnv
> plan/lowering drift**: `tryRecordDynBody` arms the program-wide
> DYNAMIC-ENVIRONMENT mode *mid-pass* (here at s3-conn's
> `do […] error […]`, reached through the serve-raw handler closure),
> but the store's append-handler unit had already FINISHED — its
> `planValueDefLocals` ran with `dynEnv=false`, so the computed
> `def old (if …)` source was never promoted to a frame local. Finalize
> then lowered the unit under the WIDENED mode (every def emits its
> OpBindDynScope twin) and refused "dynamic-scope def `old` of
> unpromoted computed value", killing the whole detached stamp. Only
> `s3-serve` hit it because only its body contains BOTH the store
> construction (the handler unit) and, later, the dyn-body dispatch.
>
> Fix: the probe-then-real sites (`tryReturnedClosure`,
> `compileStoredFnUnit`, `compileStoredBody`, `recordClosureDispatch`)
> now carry the probe's TERMINAL `dynEnv` back into the real pass before
> it starts — the probe ran the body end-to-end, so it knows the pass's
> end-state mode; pre-arming makes every unit plan under the mode it
> will lower under. The probes also inherit the caller's `dynEnv`
> (same-modality principle, as with `storedGradualDepth`). Pinned both
> ways in `lang/go/test/stamp_dynenv_drift_test.go` (stamp + runtime
> accumulation parity); validated to fail pre-fix with exactly the
> s3-serve refusal. Note: the whole-PROGRAM compile pass (no probe) can
> still drift the same way — there the top-level program is refused and
> **silently** re-run on the interpreter, which hides the failure rather
> than fixing it; it is not a lost stamp, but it IS an open defect, and
> the pre-pass that closes it is owed. mini-s3 now stamps **21 units**; the ONLY
> remaining refusals are the two `s3-handle-*` units on the addendum-2
> interpreter residual-timing fork (blocked upstream).
>
> **ADDENDUM 3 (2026-07-11 — the filter wall is closed; 20/22).** The
> bucket-list handler's "function-valued operand at filter (Stage 3)"
> was NOT a missing closure feature — it was two compile-pass defects
> stacked under the lambda-body probe:
>
> 1. **Probe/real modality drift.** Three probe constructors —
>    `forkForProbe` (recordClosureDispatch), `tryReturnedClosure`, and
>    `compileStoredBody` — built their throwaway EmitState WITHOUT
>    inheriting `storedGradualDepth`, so inside a detached stamp the
>    inner lambda's body probed STRICT while the real compile would
>    have run gradual ("probe and real must compile under the SAME
>    modality" — the rule compileStoredFnUnit's probe already followed).
>    All three now inherit it.
> 2. **Un-stepped type-name words in poly windows.** The lambda body's
>    `convert Bytes kk` over the callback entry's dynamic `.value`
>    cannot commit statically, and the recovery's claimed window still
>    held the RAW `Bytes` name token (never stepped to its literal), so
>    RecordPolyCall's operand resolution failed and the dispatch refused
>    "unmatched dispatch recovered at convert". RecordPolyCall now
>    resolves a plain type-name word via stepWord's own
>    typeNames/ResolveTypePath cascade to the canonical literal
>    (OpPushType by ID), giving callPoly's re-match exactly the operands
>    the interpreter's dispatch sees.
>
> With both landed, every anonymous service handler in mini-s3 stamps —
> the capturing-filter list handler included (pinned end-to-end in
> `lang/go/test/stamp_filterlambda_test.go`). Remaining: the two
> `s3-handle-*` units (the addendum-2 residual-timing fork, unchanged)
> and `s3-serve`'s consequence-level "finalize left the unit unstamped"
> (undiagnosed; next up). A residual imprecision worth a note: a
> synthetic sibling shape whose folded placeholder coerces the chain to
> `slice 0 N ""` (an overload-less concrete window) still refuses at the
> definite-mismatch fall-through — narrower than the mini-s3 shape and
> not exercised by it.
>
> **ADDENDUM 2 (2026-07-11 — mini-s3 walls, 2 closed / 2 mapped).** Two of
> mini-s3's walls are CLOSED as compiler extensions, taking its stamp state
> from 9/22 units to **18/22** (measured with `-compile-report` over the
> full PUT/HEAD/resume/range/list/delete flow, byte-correct output):
>
> 1. **Module-binding Bytes operand (was `dynamic input at recv-until`,
>    6 units).** Every `recv-until conn s3-crlf {…}` site refused because
>    the module-level `def s3-crlf (convert Bytes "\r\n")` read had no
>    compiled home: Bytes is ExtensionPayload-backed, and `isInertConst`
>    could not admit an opaque extension payload. Fixed with the
>    `ConstBakeable` TypeBehavior capability (typebehavior.go): an
>    extension type whose values are immutable (Bytes — BYTES.10.md §4)
>    opts its values into the const pool; every other extension type
>    (Socket, Listener, timers, flex) keeps the refusal. Freshness is
>    owned by the existing frozen-read / dep-snapshot gates. Pinned in
>    `lang/go/test/stamp_module_binding_test.go`.
> 2. **Statement-position `set … drop` over a dynamic receiver (was
>    `unmatched dispatch recovered at drop`, 4 units).** execMatch's
>    paren-tail lookahead now also accepts a PLAIN stack-shuffle consumer
>    (`dynStackShuffleWords` — drop/dup/…: stack-only, single all-Any sig)
>    as a consumed tail, so `applyGradualContagion` models the optimistic
>    single value at statement position too. Soundness owner is unchanged:
>    callPoly's result-count-claim check defers to the interpreter when
>    the runtime overload returns a different arity (vm.go). Pinned in
>    `lang/go/test/stamp_setdrop_test.go` (incl. the modified-shuffle
>    negative and the claim-mismatch fallback parity).
>
> The two REMAINING mini-s3 walls, precisely mapped:
>
> - **`s3-handle-one` / `s3-handle-get` — `fn s3-parse-range: body result
>   of unknown provenance` — BLOCKED on an interpreter semantic fork, not
>   a missing compiler feature.** s3-parse-range's trailing computed map
>   `{from: from upto: upto}` is evaluated at DIFFERENT times by the two
>   interpreter dispatch paths: a CallBoru-class call (cross-registry or
>   fork context — every serve-raw request, which is why the app works)
>   evaluates it at the callee sub-run's end, IN-frame; a same-registry
>   spliced call defers it to the CONSUMER's scope, where the body-locals
>   are gone (verified: `M.handle-get …` from the top engine raises
>   `undefined_word: from` interpreted, while the identical call inside a
>   fork context returns the values). The list twin of the deferred
>   direction is spec-pinned (`def-node-binding.tsv:54`). Recording the
>   in-frame assembly (extending the storedfn$body `elemEvalRecordable`
>   admission) would bake one side of the fork and diverge from the other
>   interpreter context, so the compiler keeps refusing. Follow-up: an
>   ARG-SEMANTICS-UNIFICATION-style decision on residual-evaluation
>   timing (+ spec re-pin), after which the recording extension is a
>   two-line gate.
> - **the bucket-list handler — `function-valued operand at filter
>   (Stage 3)`.** The genuine higher-order frontier: a CAPTURING lambda
>   (over `objects`/`prefix`/`plen`) as filter's operand inside a service
>   handler. A capture-free top-level filter lambda compiles today; the
>   capturing service-handler shape needs the closure path extended to
>   the stored-handler context. (`s3-serve`'s "finalize left the unit
>   unstamped" is consequence-level.)
>
> **ADDENDUM (2026-07-10, supersedes the per-app status below —
> see design/RUNTIME-STAMPING.0.md).** This note conflated two different
> claims: "the handler bodies pass their compile probe" and "the handlers
> execute on the VM at runtime". The ~442→~680 req/s bump cited under
> mini-redis came from the top-level DRIVER loop compiling — profiling
> showed ~97% of per-request callback CPU still in `Registry.CallBoru`
> (`-force-compile` cannot surface stored-handler refusals: the probe is
> a throwaway EmitState and declines silently). The runtime-stamping work
> closed the gap end to end: detached fn-unit compilation at the codec /
> service / module-load trigger sites, gradual-Any generalisation for
> nested user-fn compiles, filter-lambda closures with lexical captures,
> and the module-export apply reroute. mini-redis now runs **~8,100-8,400
> req/s compiled vs ~700-745 interpreted (~11x)** with ZERO CallBoru
> samples on the steady-state path. A follow-on fix closed the last
> callback — the catch-all, whose whole body is a computed map literal
> (formerly "body result of unknown provenance"): a callback body's
> trailing computed map/list now records its OpMakeMap/OpMakeList assembly
> (scoped to callback bodies, where both engines evaluate the residual
> in-frame), so **every** mini-redis callback — and `redis-serve`'s whole
> body — now executes on the VM. mini-s3's blockers (do/error trap
> lowering, its remaining higher-order shapes) stay open as described below.

Goal: get the realistic `boru:net` example handlers (`design/examples/apps/`) to
compile their bodies to bytecode units (like the echo benchmark handler does),
not just run interpreted. This note maps every remaining blocker precisely so the
work can proceed one capability at a time.

Status: **2 of 3 closed.** The echo microbenchmark handler compiles (~55k req/s
vs ~620 interpreted). **todo-api** (~12x) and **mini-redis** now compile their
handlers (see the per-app sections below); **mini-s3** remains, blocked on two
deliberate-frontier capabilities (`do`/`error` trap-region lowering + higher-order
`filter`) for an I/O-bound app. Each blocker was surfaced by
`boru run -force-compile -install network bench/networking/apps/<app>.boru`.

None of these block `boru run` (the apps run fine interpreted, default gate green,
per design/CHECK-FALSE-POSITIVES.0.md). They block only the compiled speedup.

## The walls (each app, exact blocker)

### mini-redis — TWO walls — **CLOSED**, compiles (~680 vs ~442 req/s)

1. **Unparenthesised `set … drop` residual (check-mode).** The handlers use the
   idiom `X set (k) v` newline `drop` (flex `set` returns its receiver; the `drop`
   discards it). Unparenthesised, followed by a `def`, the set's dispatch residual
   corrupts the following `def`'s binding in the compile pass (`undefined_word:
   expires`). **Fixed** by grouping all 11 sites as `(X set (k) v) drop`
   (semantics-preserving — verified by TestAppMiniRedis).

2. **Stage-3 "branch reads enclosing computation"** (was `eng/go/lower.go:431`).
   A handler `if`-arm references a value-def computed OUTSIDE the branch (LRANGE:
   `def start …; if … [ … if (start gte n) [ … ] [ slice start n cur ] ]`). The
   enclosing value-defs ARE promoted to frame locals, and their consuming
   references rewrite to local pushes — but an arm-OUT designation referencing a
   promoted producer is left for `lowerFragment` to re-resolve, and the scopeFloor
   guard treated that residual `opEvent` as a forbidden enclosing read. **Fixed**:
   the guard now skips operands whose producer is in `lw.promoted` (delivered via
   the frame, not the enclosing sim). Regression:
   `TestBranchArmReadsEnclosingValueDef`.

### mini-s3 — MULTIPLE walls, not yet closed

The top-level `operand of unknown provenance … at serve-raw` is a CONSEQUENCE:
the `serve-raw` handler `([conn] => [ s3-conn conn store ])` fails its compile
probe because `s3-conn`'s body refuses. Attributing every refusal to its fn
(instrument `MarkUncompilable` with `es.unitNames`) shows FOUR distinct blockers,
two of them deliberate frontier islands:

- **`s3-conn`: `code-body word do (Stage 2)`.** The per-connection actor is
  `def ok (do [ s3-handle-one conn store drop true ] error [ drop false ]); if ok
  [ s3-conn conn store ] [ 0 ]`. Both `do` and `error` carry
  `CompileEffect: CompileFallbackBody` (`lang/go/native/native_control.go`) — they
  DELIBERATELY island to the interpreter. Compiling this means lowering a
  `do`/`error` trap region (bytecode exception handling) — a large, deliberate
  VM feature, not a bug.
- **store `list` handler: `function-valued operand at filter (Stage 3)`.** The
  LIST handler runs `filter ([e:Any] => […]) (keys objects)` — higher-order
  compilation of a lambda operand. A separate large frontier.
- **`s3-handle-one`: `unmatched dispatch recovered at convert`** and the store
  handlers' `unmatched dispatch recovered at drop` (×2). The drop ones are the
  set/drop residual class — clearable by grouping `(objects set …) drop` (as
  mini-redis/todo-api). The `convert` one sits inside the deeply-nested method
  `if` chain and needs its own look.

So mini-s3 requires TWO deliberate-frontier capabilities (`do`/`error` body
lowering + higher-order `filter`) on top of the residual cleanups. Each is a
substantial, soundness-critical VM feature the compiler team has intentionally
deferred (the CompileFallbackBody islands). mini-s3 is also I/O-bound —
`-force-compile` vs `-no-compile` are within noise (~100 req/s both) — so the
payoff is small. Left for dedicated follow-ups; NOT closed.

### todo-api — Stage-2 "branch leaves extra values" — **CLOSED**, compiles (~12x)

`fn storedfn$body: branch leaves extra values (Stage 2 …)`. Two parts. (a) Several
handlers left their flex `set` receivers un-dropped (unlike mini-redis) — grouped
+ dropped, semantics-preserving. (b) The PUT arm `def t2 {…}; (todos set (id) t2)
drop; t2` has t2 as BOTH the `set` argument and the arm result; the intra-arm use
popped its single sim slot, leaving nothing to seat as the result. **Fixed**:
`planValueDefLocals` now promotes a fragment-internal arm-result value-def that is
also consumed as an operand within its arm (the `fragResultStaysOnSim` helper).
Regression: `TestBranchArmResultSelfConsumed`.

## Remaining — mini-s3 (`do`/`error` body + higher-order `filter`)

See the mini-s3 section above for the full attribution. Two deliberate-frontier
capabilities block it — `do`/`error` trap-region lowering (the per-connection
actor) and higher-order `filter` compilation (the LIST handler) — plus set/drop
grouping + a `convert` residual. Each capability is a substantial, soundness-
critical VM feature the compiler deliberately islands today; mini-s3 is also
I/O-bound (compiled ≈ interpreted), so the payoff is small. Not yet done — these
are dedicated follow-ups, not a quick fix.

## What landed (each a focused, fully-gated change)

- **todo-api** (Stage-2 self-consumed arm result) — `fragResultStaysOnSim`
  promotion + example set-drop cleanup. Compiles ~12x.
- **mini-redis** (Stage-3 promoted-operand scopeFloor skip) — the guard skips
  operands delivered via a frame local + example set-drop grouping. Compiles.

Each landed only after `verify-bytecode` (differential + property-fuzz + -race),
`cover-gate` 100%, `TestCheckAccuracyRatchet` unchanged, and the app's compiled
run matched its interpreted run (`-force-compile` vs `-no-compile` / the app
functional tests). A wrong lowering that MISCOMPILES is the one outcome the
codebase forbids, so none was rushed. The `bench/networking/apps/` drivers make
the before/after measurable.
