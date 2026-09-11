package compiler

import (
	"fmt"
	"strings"
	"sync"

	core "github.com/boru-lang/boru/core/go"
)

// Bytecode Program model — Stage 1 of design/boru-bytecode-plan.0.md.
//
// A Program is the flat, linear lowering of the typed region the
// carrier checker resolved: literal pushes, fixed-arity native calls
// with the checker-selected signature baked in, and (Stage 1) the
// occasional SWAP the operand layout needs. There is no VM yet — the
// Stage 1 gate is structural: a disassembler plus golden tests that
// the recording pass (emit.go) lowers each accepted source form to
// the expected instruction stream, with the three call forms
// (`add 1 2` ≡ `1 add 2` ≡ `1 2 add`) producing identical bytecode.
//
// Stack convention: matches the kernel's one argument convention —
// position 0 is the top of stack. CALL_NATIVE pops len(sig.Args)
// values, the top being sig position 0, and pushes the single
// result. Operand emission orders pushes so that layout holds.

// Opcode identifies one VM instruction.
type Opcode uint8

const (
	// OpPushConst pushes Consts[Arg].
	OpPushConst Opcode = iota + 1
	// OpSwap exchanges the top two stack values.
	OpSwap
	// OpCallNative pops Sigs[Arg].Sig's arity, calls, pushes one result.
	OpCallNative
	// OpJmp jumps to the absolute pc in Arg (forward-only until the
	// loop stage lands the step budget).
	OpJmp
	// OpJmpIfFalse pops the condition and jumps to Arg when it is
	// falsy under the engine's CoerceBoolean — the same truthiness
	// stepMove applies to a MoveIf condition.
	OpJmpIfFalse
	// OpPushLocal pushes locals[Arg] (loop iterator bindings).
	OpPushLocal
	// OpForSetup pops the iteration count and opens a counted loop
	// whose iterator writes locals[Arg].
	OpForSetup
	// OpForNext advances the innermost loop: binds the iterator and
	// falls through into the body, or closes the loop and jumps to
	// Arg when exhausted. The body's trailing JMP back to this
	// instruction is the program's only back-edge.
	OpForNext
	// OpCallUser pops Fns[Arg].NParams args (sig position 0 on top)
	// into a fresh frame's locals and enters the unit.
	OpCallUser
	// OpTailCallUser binds args like OpCallUser but REPLACES the
	// current frame — the language's tail-call guarantee: self and
	// mutual tail recursion run in O(1) frames.
	OpTailCallUser
	// OpRet pops the frame; the unit's single result stays on the
	// shared operand stack for the caller.
	OpRet
	// OpPushType pushes the type literal for Types[Arg], resolved
	// through the registry's TypeTable at RUN time — type nodes are
	// never pooled as constants because a by-value copy goes stale
	// against the canonical pointer (eng/go/CLAUDE.md, Canonical
	// *Type Pointers); the ID lookup always yields the canonical
	// node, including types the check pass minted (def Foo …).
	OpPushType
	// OpFallback runs Fallbacks[Arg] as an interpreter island: a
	// construct the checker could not type (a dynamic dispatch, a
	// code-body higher-order word) re-executes through a sub-engine
	// over its recorded tokens, threading the operand stack. The
	// compiled code on either side keeps running — this is Stage 5's
	// span-level FALLBACK_INTERP (plan §Stage 5). NIn values are
	// popped (top of stack = sig position NIn-of-the-span's first
	// threaded arg) and pushed onto the island; the island's residual
	// is pushed back.
	OpFallback
	// OpPushClosure pushes a Closure VALUE over the compiled body unit
	// Fns[Arg] (Value{Parent: TFunction, Data: ClosurePayload}). A
	// higher-order word's CODE body compiles to its own unit and rides
	// as the word's operand; at run time the word's native handler
	// invokes it through the VM's re-entrant runner via the InvokeBody
	// seam — never the interpreter (plan P2). Capture-carrying closures
	// take their captures from the operand stack below the closure
	// (CompiledFn.NCaptures trailing slots, popped into
	// ClosurePayload.Captures — the lower.go opClosure lowering pushes
	// each capture operand first).
	OpPushClosure
	// OpCallNativePoly dispatches PolyRefs[Arg].Word at RUN time: it runs
	// the kernel's own MatchSignature over the word's signatures against the
	// Arity values on top of the stack — the SAME first-match the
	// interpreter takes — then calls the matched handler. Used where the
	// checker could not commit to one overload (a dynamic operand the
	// checker widened to Any), so the VM selects faithfully at run time
	// instead of islanding through a sub-engine (plan P3). Pops Arity,
	// pushes one result; a no-match raises signature_error.
	OpCallNativePoly
	// OpCallDynamic applies a runtime FUNCTION value to Arg trailing args:
	// the value sits below the Arg args on the stack. If it is callable (a
	// compiled closure, or a Function the interpreter auto-applies — a method
	// field like `r.int`), it is invoked and the result replaces value+args;
	// if it is NOT callable, the value and args stay on the stack unchanged
	// (matching the interpreter, which leaves a non-callable residual). This
	// is the fn-value-call boundary (plan P4): a dynamic value preceding
	// residual args.
	OpCallDynamic
	// OpStoreLocal POPS the top of stack into locals[Arg]. Emitted right
	// after the event that produces a COMPUTED value referenced more than
	// once — a value-`def` bound to an event result and used several times
	// (`def a (make …) a eq a`). The single VM-stack copy would be consumed
	// by the first use; storing it once and re-pushing each reference via
	// PUSH_LOCAL gives the lowerer's single-consume stack discipline a sound
	// multiply-referenced value (the carrier-identity item's value-def
	// locals).
	OpStoreLocal
	// OpDrop pops and discards the top stack value. Emitted on the TRUE path of
	// a computed-else `if cond [then] (expr)`: the else value `(expr)` is
	// eagerly computed onto the stack before the branch, so the taken (then)
	// path drops it before running the then-body, while the false path leaves
	// it as the branch result.
	OpDrop
	// OpMakeList pops Arg values off the top of the stack and pushes a single
	// list of them, preserving order (deepest of the Arg becomes element 0). It
	// lowers a list literal whose elements are COMPUTED (`[1 add 2]` -> `[3]`,
	// `[1 (2 add 3) 4]` -> `[1 5 4]`): the elements are evaluated onto the stack,
	// then assembled. A fully-literal list (`[1 2 3]`) stays a pooled const and
	// never needs this.
	OpMakeList
	// OpMakeMap pops the values of a COMPUTED map literal off the top of the
	// stack and assembles them with the key list held in Program.MakeMaps[Arg]
	// into a single map (Keys[i] ← the i-th value, deepest of the popped run
	// becomes value 0). It lowers `make`'s construction body whose field VALUES
	// are computed and not bakeable as an inert const — `make Outer {i:(make
	// Inner …)}` — so the inner instance is freshly built each run rather than a
	// frozen, aliasable const. A fully-literal / const-foldable map stays a
	// pooled const and never needs this.
	OpMakeMap
	// OpTrap raises the boru error described by Program.Traps[Arg] and aborts the
	// run. It is the compiled form of a check-mode-suppressed runtime error: a
	// word that is deliberately lenient in check mode but raises at run time (an
	// orphan `gen [...]`, an `unpack` of a missing key). The checker is lenient,
	// so the compiled stream would otherwise silently succeed where the
	// interpreter errors; instead the trap raises the byte-identical error
	// (shared taxonomy text) at exactly the point execution reaches it. Terminal:
	// the recorder ends the program at the trap (everything after is unreachable).
	OpTrap
	// OpDispatchRematch re-runs a STATICALLY-FAILED dispatch over the live
	// runtime values — the runtime-evaluated twin of OpTrap for an unmatched
	// dispatch whose window held CARRIER operands (a concrete value at run
	// time, only a typed stand-in at check time — tryRecordUnmatchedDispatchTrap
	// declines the definite trap for those). The recorder pushed the failed
	// window's operands so stack[top-i] is sig position i (the callPoly
	// layout); the op re-matches them against the word's LIVE signatures
	// (Program.Dispatches[Arg] names the word; the live registry binding is
	// what the interpreter would consult at this point). NO MATCH — the
	// expected case, this compiled an ERROR row — raises the shared rich
	// diagnostic built over the CONCRETE values (noMatchDiag via
	// runtimeNoMatch), byte-identical to the interpreter's sigError at the
	// same point. A MATCH means the static model was wrong (a refined
	// runtime tag, a value-sensitive predicate satisfied) — the tail was
	// truncated at this terminal op, so the run defers to the interpreter
	// (vm:rematch-matched, the fenced whole-program fallback — slow, not
	// wrong). Terminal like OpTrap.
	OpDispatchRematch
	// OpReverse reverses the top Arg operand-stack values in place. It is the
	// stack-scheduling primitive for an N-operand call whose computed args sit in
	// exact REVERSE signature order — the common forward-call shape `f (a)(b)(c)`,
	// where the args evaluate left→right so sig position 0 ends up DEEPEST, but the
	// call wants it on top. SWAP handles N=2; OpReverse generalises it to N≥3
	// (the 3-deep rotate the VM previously had no opcode for, so layoutOperands
	// refused). Emitted only when layoutOperands recognises an exact reverse, so
	// it can never seat an operand wrongly.
	OpReverse
	// OpCallDynamicTrailing applies a runtime FUNCTION value to the Arg values
	// BENEATH it — the source shape where the fn TRAILS its argument (`5 m.f`,
	// where the fn `m.f` produces auto-applies to the 5 beneath it; `[..] r.one-of`).
	// The recorder lays the operands out exactly like OpCallDynamic — fn at the
	// base, the Arg args above — so the callable path is identical. The ONLY
	// difference is the NON-callable residual: where leading OpCallDynamic leaves
	// [fn, args] (fn below), the interpreter's trailing form leaves the fn ON TOP
	// of its args, so this op rotates the fn back above its args when the value
	// turns out not to be callable — keeping the residual byte-identical to the
	// interpreter. Bounded to Arg==1: with >1 arg the island's forward collection
	// would order them opposite to the interpreter's top-down stack collection.
	OpCallDynamicTrailing
	// OpFlowBreak / OpFlowContinue lower a break / continue that sits in a FN
	// BODY with no enclosing loop in its OWN unit — a flow-control signal that
	// targets the nearest loop in an ANCESTOR frame (the interpreter's
	// cross-frame FlowCtrl, compiled). The VM unwinds the frames opened since
	// that loop, discards the current iteration's partial operand pushes (down
	// to the loop's iteration base — matching the interpreter, which drops the
	// values between the loop mark and the signal), then jumps to the loop's
	// exit (break) or its FOR_NEXT (continue). A same-unit break/continue stays
	// a static JMP (lowerBreak / lowerContinue) — these ops are the cross-frame
	// case only. Reaching one with no open loop at all is a "break outside loop"
	// runtime error, surfaced as an internal_error so RunCompiled falls back and
	// the interpreter raises the canonical taxonomy.
	OpFlowBreak
	OpFlowContinue
	// OpStackMark / OpDropToMark / OpPopMark implement a VARIADIC stack region for
	// the chained variadic-statement-`if` (a 2-arg `if` whose 0-or-1 result is
	// claimed as the else of a following `if`). The stack depth of a 0-or-1 result
	// is only known at run time, so a fixed-offset DROP cannot remove it. OpStackMark
	// pushes the current stack depth onto a per-run mark stack (emitted before the
	// variadic producer's region). On the claiming if's TRUE path, OpDropToMark pops
	// the mark and truncates the stack to it (discarding the 0-or-1 eager regardless
	// of its count); on the FALSE path, OpPopMark discards the mark and keeps the
	// eager as the result. The merged result is itself a 0-or-1 the program residual
	// absorbs. Arg is unused.
	//
	// The 0-or-1 is this CLIENT's shape, not the ops' (measured 2026-09-10,
	// NUR067): DropToMark truncates to stack[:m] and PopMark keeps whatever
	// stands above the mark, so both are count-agnostic already. What is
	// 0-or-1-specific is planVariadicClaims, which only recognises the chained
	// `if`.
	OpStackMark
	OpDropToMark
	OpPopMark
	// OpSeatBelowMark seats a residual's INERT PREFIX beneath a
	// runtime-variadic REGION (NUR067's consuming half). `99 for 3 [i]` and
	// `99 await {mode:'first'} [[]]` both leave [99, <region>], and the region's
	// count is a runtime value, so the prefix cannot be pushed after it (it
	// would land on TOP of the run) and cannot be indexed past it from the top.
	// The lowering therefore opens an OpStackMark before the region's producing
	// event, pushes the prefix ABOVE the finished region in residual order, and
	// closes with this op: Arg = the prefix length n; pop the innermost mark m,
	// lift the top n values, and re-lay the stack as
	// [ … , prefix₀ … prefixₙ₋₁, region … ] — the prefix at the mark, the whole
	// run above it, order preserved on both sides. Nothing else moves, and the
	// region's count is never named.
	OpSeatBelowMark
	// OpMakeListToMark collects a runtime-variadic REGION into one List
	// (NUR067's consuming half, the collect). `[(for 3 [i])]` and
	// `size [(await {mode:'any'} [[7 8]])]` build a list of the whole run, and
	// OpMakeList cannot: its Arg is a STATIC element count. The lowering opens
	// an OpStackMark before the region's producing event and closes with this
	// op — pop the innermost mark m, replace stack[m:] with one List of those
	// values in order. Elements are ascription-stripped exactly as OpMakeList
	// strips them (list elements are stored data). Arg is unused; the count is
	// the run's, and is never named.
	OpMakeListToMark
	// OpCallDynamicMixed handles the MIXED fn-value-call boundary: a runtime
	// FUNCTION value sitting INTERIOR to the program residual, with static args
	// both BELOW it and ABOVE it (`3 m.f 2` — `m.f` is a 2-arg fn collecting the
	// forward `2` into sig[0] and the stack `3` into sig[1]). Neither the leading
	// (OpCallDynamic) nor the trailing (OpCallDynamicTrailing) layout fits, since
	// args straddle the fn. Arg is the WINDOW size (everything from the deepest
	// before-arg through the last after-arg, inclusive of the fn). The VM islands
	// that window verbatim — it is the same token sequence the interpreter ran, so
	// the fn auto-applies (or, if the value is not callable, stays put) with full
	// fidelity for any arity and any before/after split. The recorder promotes the
	// fn's producing event to a frame local so the residual re-pushes in source
	// order (the fn sits above a before-arg literal, which the in-order
	// reconciliation otherwise forbids).
	OpCallDynamicMixed
	// OpSpliceDyn spreads a runtime splice payload (REFUSAL-CLOSURE §9.2b —
	// `def d word xs d` over a computed xs): pop the payload; a plain list
	// spreads its top-level elements (spliceExpand), any other value
	// contributes itself — exactly the interpreter's marker re-step for a
	// DATA payload. A payload bearing active tokens (words, parens, reaches,
	// interpolations, nested splices — a Forth-style code macro) or a
	// function value defers to the interpreter (vmDefer: the re-step
	// dispatches against the live stack, which only the interpreter owns).
	OpSpliceDyn
	// OpInterpXml assembles an interpolated XML element from its computed
	// holes (REFUSAL-CLOSURE §9.2c — the tree twin of OpInterp).
	// Program.XmlInterps[Arg] holds the template skeleton; the VM pops one
	// operand-stack value per hole (deepest popped = hole 0 — the
	// depth-first attr-then-children traversal order BuildXmlFromTmpl
	// evaluates in) and rebuilds the element via rebuildXmlFromTmpl —
	// byte-identical to the interpreter's build over the same hole values.
	OpInterpXml
	// OpInterp assembles a template string (`` `got ${x}` ``) from its
	// computed holes. Program.Interps[Arg] holds the ordered template — literal
	// segments interleaved with HOLE markers. The VM pops one operand-stack value
	// per hole (deepest popped = hole 0, source order), then walks the template:
	// a literal segment appends verbatim, a hole appends ValToString of the next
	// popped value — byte-identical to the interpreter's evalInterpParts. Lowers
	// any interpolated string whose holes are runtime- or natively-computed
	// (`${1 add 2}`, `${x}` for a fn param, `${typeof x}`); a fully literal or
	// pure-binding-fold template stays a pooled const and never needs this.
	OpInterp
	// OpCallUserPoly dispatches a MULTI-OVERLOAD user fn at RUN time: it runs
	// the kernel's own MatchSignature over the recorded same-arity overload
	// subset of UserPolys[Arg].Word against the Arity values on top of the
	// stack — the SAME first-match the interpreter takes — then enters the
	// matched overload's compiled body unit exactly as OpCallUser would
	// (pop the args into frame locals, checkParamContract, push a frame).
	// The user-fn mirror of OpCallNativePoly: used where a gradual-Any arg
	// made the dispatch ambiguous at check time (two or more same-arity
	// overloads reachable), so the VM selects the arm faithfully at run time
	// instead of refusing the whole program. A no-match (or any drift between
	// the recorded overload set and the word's live dispatch table) defers to
	// the interpreter through the whole-program fallback, which raises the
	// canonical signature_error / runs the live definition.
	OpCallUserPoly
	// OpCallDynTrailTop applies a runtime FUNCTION value sitting ON TOP of its Arg
	// args to those args — the TRAILING fn-value-call boundary BOUNDED BY A PAREN
	// (`(prev key comp)`, a captured/param comparator applied to two computed args).
	// Unlike OpCallDynamicTrailing (recorder lays the fn at the BASE and rotates the
	// non-callable residual, bounded to 1 arg), the fn stays on TOP here, so NO
	// rotation is ever needed and it is sound for ANY arity: the args are laid out
	// in source/stack order with the fn above them; on apply the fn auto-applies to
	// the Arg args beneath it exactly as the interpreter's paren auto-dispatch (the
	// fn-on-top order matches the interpreter's top-down stack bind); if the value
	// is NOT callable, [args, fn] is ALREADY the interpreter's trailing residual, so
	// it is left untouched. Arg is the argument count (the paren's value count minus
	// the trailing fn). The recorder captures this at the paren-collapse boundary
	// (engine.go) where the arity is known, since the flattened residual cannot
	// recover the paren-group size.
	OpCallDynTrailTop
	// OpCallDynApplyTop is OpCallDynTrailTop with the `apply` WORD's semantics
	// (Stage M2a): the fn value on top came through `…args fn apply`, whose
	// interpreter handler (applyHandler) UNQUOTES the value before re-stepping
	// it — so a /v-parked (Quoted) fn value STILL applies here, where the
	// paren-bounded OpCallDynTrailTop would leave it as data. The fn is popped,
	// unquoted, and applied to the Arg args beneath it (top arg → first param,
	// the same reversed-window forward bind as OpCallDynTrailTop); a compiled
	// closure runs VM-native. A non-fn payload raises the applyHandler's own
	// byte-identical error. Emitted for a fn-body-tail `apply` over a
	// Function-typed carrier (StartFnCompile's pendingApply) and for the
	// paren-bounded RecordDynApply event when the apply word drove it.
	OpCallDynApplyTop
	// OpCallDynApplyOne is OpCallDynApplyTop for an apply over a GRADUAL lead
	// recorded as an EVENT (the twenty-seventh increment): the check matched
	// `apply`'s [Reach Any] overload over a Dynamic value and modelled ONE
	// result, so the op commits exactly one — a fn on top applies to the Arg
	// args beneath it as the interpreter's applyHandler re-step does (the
	// args as the stack, the fn stepped over them) and its result count is
	// checked; any other count, a lens on top (the [Reach Any] overload's
	// own dispatch) or a compiled closure of another arity DEFERS the run to
	// the interpreter, and a value that is no fn raises the interpreter's
	// own `apply` no-match at the apply word's position.
	OpCallDynApplyOne
	// OpCallDynTrailKeepQ is OpCallDynTrailTop for an EVENT-provenance fn (a
	// direct call result — `(1 2 (mk))`): the value arrives WITHOUT the
	// interpreter's read substitution, so its runtime quote state must
	// SURVIVE — a callee returning `quote (fn …)` stays inert in the
	// interpreter ([args, fn] is the residual) while an unquoted anonymous
	// result auto-applies. No strip: a Quoted runtime value leaves the
	// window untouched (both engines' data case); an unquoted one applies
	// exactly as OpCallDynTrailTop. This is what retires RecordDynApply's
	// event-lead hard-refusal (PR #280's probe is the quoted witness).
	OpCallDynTrailKeepQ

	// OpCallDynFrame replays a fn body's ENTIRE end-of-body residual — the
	// whole-frame dynamic-apply window. A fn body that leaves an unapplied
	// runtime FUNCTION value in its residual (`(args.0 args.1)` over an
	// unnamed Function param, or a bare `args.0` read of one) cannot lower to
	// the bounded leading/trailing apply ops: the interpreter's
	// execFnDefLiteral rule dispatches that value against the LIVE FRAME —
	// forward-collecting the residual values after it AND stack-collecting the
	// frame values below it (including the unnamed-param re-pushes at the
	// frame bottom), by whatever name/arity the value turns out to have. Arg
	// is the width of the TOKEN region: the top Arg stack entries are the
	// values the interpreter's pointer actually stepped (fn reads and their
	// following args); everything below them in the CURRENT frame is the
	// resolved prefix (the frame-bottom unnamed-param re-pushes, inert by the
	// arguments-are-inert invariant). The VM re-runs the whole frame region
	// via RunResolved(unit registry, prefix, tokens) — a nested interpreter
	// run whose stepping starts after the prefix, exactly the sub-engine twin
	// of the frame's own ArgSpan — and replaces the frame region with the
	// run's residual. Faithful by construction for ANY callee arity, type
	// mismatch (the value stays data), or below-window collection — PROVIDED
	// the window is the body's LAST statement: the replay fires at the RET,
	// so a recorded event after it would reorder observable effects ahead of
	// the apply's (the recorder's replayIsBodyTail gate refuses that shape).
	// The following RET applies the CallBoru-path trim (CompiledFn.RetReplay).
	OpCallDynFrame

	// OpPushConstFresh pushes a deep clone of Consts[Arg] with fresh container
	// identity (CloneValue) instead of the pooled instance. A compound VALUE
	// literal written in a fn body is re-EVALUATED per call by the interpreter,
	// constructing a fresh List/Map each time — so a pooled const pushed from a
	// compiled fn unit must not leak one shared identity across calls
	// (`def mk fn [[] [List] [[1]]]  (mk) eq (mk)` must stay false; miscompile
	// mechanism A, design/MISCOMPILE-HUNT-FINDINGS.0.md §A). The finalize pass
	// rewrites OpPushConst → OpPushConstFresh in place (freshenFnUnitConsts,
	// emit.go) exactly for fn-unit consts that (a) materialised from a literal
	// written in the body — an ENCLOSING binding's value read by name keeps the
	// shared push, matching the interpreter's one-instance-per-binding
	// semantics — and (b) have a single push site, so within-call reads of one
	// binding still share one instance. Main-unit pushes stay bare: top-level
	// straight-line code evaluates each literal once, and loop-spread
	// consumption refuses before lowering. Captured containers arrive through
	// capture slots (locals), never consts, so closure identity is preserved.
	OpPushConstFresh

	// OpPushConstFreshLocal is the MULTI-read twin of OpPushConstFresh: a compound
	// body literal bound once and read at SEVERAL sites (`def tree {…}` used in two
	// statements). A single OpPushConstFresh per site would mint a DISTINCT instance
	// at each read (breaking `(x) eq (x)` identity within a call); a shared pooled
	// const would leak ONE instance across ALL calls (breaking per-call freshness).
	// The interpreter's semantics are exactly one construction per call, shared by
	// every read — so every read site lowers to this op, keyed (Arg indexes
	// Program.ConstLocals) to a {ConstIdx, Slot} pair: on first execution in a frame
	// it deep-clones Consts[ConstIdx] into frame local Slot and pushes it; every
	// later read pushes the seated local. The frame's locals start zero-valued
	// (Parent==nil) and a compound clone always carries a Parent, so Parent==nil is
	// an unambiguous "not yet seated this call" sentinel. Lazy init needs no
	// dominating construction site, so the in-place rewrite (freshenFnUnitConsts)
	// leaves every jump target untouched. Only fn units emit it (main-unit literals
	// evaluate once, straight-line).
	OpPushConstFreshLocal

	// OpBindTyped is the runtime validate/reparent step of a typed value-def
	// (`def x:Pos n`) whose constraint is a REFINEMENT — a predicate type, a
	// bare-refine newtype, or an inline/named DepScalar subset — and whose body
	// is DYNAMIC (a param / computed carrier). It pops the body value, runs the
	// SAME membership check the interpreter's defTypedHandler runs (RunPredicate
	// / Unify against the builtin ancestor / the self-contained DepScalar
	// predicate — see RunTypedBind, typed_bind.go), raising the byte-identical
	// (position-less, plain — exactly what the interpreter surfaces) error on
	// failure, then pushes the value the interpreter would bind — reparented via
	// ReparentValue where the interpreter reparents, so a downstream typeof /
	// sig dispatch sees the refined tag. The STORE half of the binding stays
	// with the ordinary value-def machinery (planValueDefLocals promotes a
	// multiply-read binding to a frame local via OpStoreLocal; a dead binding
	// drops the result AFTER the validation ran, matching the interpreter, which
	// validates even a never-read typed def). Arg indexes Program.TypedBinds.
	// A STATIC (concrete) typed-def body never emits this — its reparent rides
	// the const pool, unchanged.
	OpBindTyped

	// OpCallDynMethod is the GUARDED mid-stream shaped-instance-method apply
	// (Stage M2c): a method read from a shape-instance container (a logger /
	// span / instrument / rand handle, whose check-mode ReturnsFn instance
	// resolves the member's SIGNATURES but whose runtime instance carries
	// per-call state — the freeze-gate) is applied to a statically-known
	// window of inert args at the exact point the interpreter auto-dispatches
	// it. Stack layout is the LEADING boundary's ([fn, a1..aN] with aN on
	// top, forward order); Arg indexes Program.DynMethods, whose spec pins
	// the claimed arity AND the claimed result count. Unlike OpCallDynamic
	// (residual position, count-unchecked, non-callable stays data), the
	// program CONTINUES past this op with NOut results committed downstream,
	// so every shape-claim failure — a non-callable/quoted runtime value, or
	// a result count differing from the claim — raises internal_error, which
	// RunCompiled resolves by re-running the interpreter (slow, not wrong;
	// runtimeShouldFallback) and --force-compile surfaces loudly. A genuine
	// boru error raised by the method surfaces as-is (the interpreter raises
	// the same at the same point, prior side effects included).
	OpCallDynMethod

	// OpLookupDynScope is a DYNAMIC-SCOPE name read: a fn body's word that
	// resolves through no lexical home — not a param, capture, local, const,
	// or module binding the recorder can bake — but which SOME live frame
	// binds at run time (a callee reading the caller's param, a recursive
	// base case reading the previous frame's body-local — recursion.tsv
	// 71/72). Arg indexes Program.Consts (a String holding the name); the VM
	// reads r.Defs.Top(name) exactly as the interpreter's stepWord
	// substitution does. A miss, or a binding the substitution would NOT
	// push as a simple value (a Function/class dispatch, a splice/reach
	// marker), defers to the interpreter via internal_error
	// (runtimeShouldFallback — slow, not wrong). The binder side is
	// OpBindDynScope: the whole-program lowering pass installs it in every
	// unit that binds a dynamically-read name.
	OpLookupDynScope

	// OpBindDynScope makes a frame binding REGISTRY-VISIBLE for dynamic-scope
	// readers: it pops the top value and installs it under the name in
	// Program.Consts[Arg] via InstallDef — the same installer the
	// interpreter's `def` runs — recording the name's prior depth so the
	// frame's exit (RET) truncates the binding stack back, exactly the
	// interpreter's def-cleanup discipline. Emitted only for names some
	// OpLookupDynScope reads (the DynScopeNames set), at the def site for
	// body-locals and at unit entry for params; top-level binds are never
	// popped (the interpreter leaves top-level defs installed too). A unit
	// containing binds never TAIL-calls (the interpreter keeps the frame's
	// bindings live across the call in its body tail), so bindings stack
	// per-activation and pop innermost-first.
	OpBindDynScope
	// OpCallDynMixedFromMark is the VARIADIC-REGION twin of OpCallDynamicMixed
	// (plan Phase 5, L-DO part 2b): the window is bounded by the topmost
	// OpStackMark instead of a fixed count — stack[mark:] islands verbatim
	// through the same re-step machinery, so a runtime-variable region (a
	// fallible do-catch residual: 1 Error caught vs N values) plus the fixed
	// values above it reproduce the interpreter exactly, auto-apply hazard
	// included. The mark is consumed. An empty window is a no-op (the mark
	// still pops).
	OpCallDynMixedFromMark
	// OpBindGlobal is the CROSS-REQUEST persistence twin of a top-level `def`
	// of a computed value: it pops the runtime value and writes it into the
	// KEPT check-pass binding slot (Program.GlobalBinds[Arg] names the def and
	// the depth its install recorded), so the registry binding a later request
	// (or an interpreter read) resolves is the value the program actually
	// computed — not the check pass's carrier. Without it, `def h (Model.new
	// …)` under a compiled run left `h` bound to a Model CARRIER and the next
	// request raised model_bad_handle (the 2026-07-15 flip composite's root
	// cause). SetAt replaces IN PLACE at the recorded depth — never a push —
	// so shadowing depth and undef behaviour match the interpreter exactly; a
	// slot popped by a later check-time undef skips the write (the interpreter
	// would have discarded the binding too).
	OpBindGlobal
	// OpLookupDynScopeData is the DATA-position twin of OpLookupDynScope: it
	// reads the name's live dynamic-scope binding and PUSHES it, WITHOUT the
	// FnDefInfo dispatch-defer. The emitter uses it only where a
	// Signature.FnDataArgs slot proved the fn value is consumed as DATA (the
	// parselang-fn-dispatch parser operand), so pushing the FnDefInfo binding is
	// byte-identical to the interpreter reading the /q-captured name as data. It
	// still defers on a genuine miss, an active token, and a class binding (a
	// class read as data is not a parser).
	OpLookupDynScopeData
	// OpBindTwin marks ONE bind-ledger transition's position in the
	// instruction stream (§6.5's inert-emission stage): Arg indexes
	// Program.BindTwins. INERT under the keep-the-installs regime — the VM
	// executes it as a no-op, because the check pass's install is already in
	// the registry and applying the twin too would double-install (the exact
	// hazard §6.5 names). The rollback-and-replay flip gives it semantics:
	// re-install the recorded transition at this position, against the
	// rolled-back registry. Zero stack effect either way.
	OpBindTwin
	// OpBindResident is the ARM-RESIDENT twin (§6.5's each-body recovery):
	// a bind transition whose runtime home is INSIDE a compiled
	// per-invocation unit, executing once per invocation with the RUNTIME
	// value — the semantics no existing op has (OpBindTwin replays the
	// check pass's one generalized carrier-valued capture; OpBindDynScope
	// has the value but tears down at RET via the dynBinds trail;
	// OpBindGlobal's Push mode has the lifetime but is a root-stream op).
	// Arg indexes Program.ResidentBinds. The install arm pops the runtime
	// value and installs it through core.InstallDef — the interpreter's
	// own installer, so per-element repeats stack exactly as the
	// interpreter's leak does — into the CURRENT registry, riding no
	// unwind trail (leak persistence IS the semantics: a mid-iteration
	// raise leaves earlier elements' installs, interpreter-identical).
	// The undef arm (a var param's balanced per-iteration teardown) pops
	// the name's live top entry instead, consuming no stack. The TYPE arm
	// re-installs its twin-table entry's BODY per element — a type binding
	// has no runtime value, so the expression is evaluated once at compile
	// time and each element mints its own node — and consumes no stack
	// either; the bridge proves the type expression element-independent
	// before stamping the site (typeInstallElementIndependent).
	OpBindResident
	// OpDeoptIfFn is the per-read DEOPT of a gradual word read (NUR123): a
	// bare read of a frame binding the pass types dynamic(Any) — a body-
	// local bound to a container element — that the model consumed as a
	// VALUE somewhere the whole-frame replay cannot re-step (a container
	// member, an argument, a branch or loop body, a read an event follows).
	// The interpreter dispatches the binding as a WORD when it holds a fn at
	// run time, so the op tests the binding's runtime value where the
	// statement holding the read begins — before any of its effects — and,
	// on a fn, hands the REST OF THE BODY (Fns[unit].Body from the
	// statement's token) to the interpreter with the frame's bindings
	// registry-visible, then continues at the unit's RET with the island's
	// residual as the frame region (CompiledFn.Deopts[Arg]). Plain data
	// costs the test and nothing else.
	OpDeoptIfFn
)

// opcodeNames is the single source of each opcode's disassembler mnemonic,
// indexed by the opcode value so the name and the iota order cannot drift. The
// disassembler renders the mnemonic via Op.String() and switches separately
// only to format each opcode's ARGUMENT (a different concern — it reads the
// program's const/sig/type/fallback pools).
var opcodeNames = [...]string{
	OpPushConst:            "PUSH_CONST",
	OpSwap:                 "SWAP",
	OpCallNative:           "CALL_NATIVE",
	OpJmp:                  "JMP",
	OpJmpIfFalse:           "JMP_IF_FALSE",
	OpPushLocal:            "PUSH_LOCAL",
	OpForSetup:             "FOR_SETUP",
	OpForNext:              "FOR_NEXT",
	OpCallUser:             "CALL_USER",
	OpTailCallUser:         "TAIL_CALL_USER",
	OpRet:                  "RET",
	OpPushType:             "PUSH_TYPE",
	OpFallback:             "FALLBACK",
	OpPushClosure:          "PUSH_CLOSURE",
	OpCallNativePoly:       "CALL_NATIVE_POLY",
	OpCallDynamic:          "CALL_DYNAMIC",
	OpStoreLocal:           "STORE_LOCAL",
	OpDrop:                 "DROP",
	OpMakeList:             "MAKE_LIST",
	OpMakeMap:              "MAKE_MAP",
	OpTrap:                 "TRAP",
	OpDispatchRematch:      "DISPATCH_REMATCH",
	OpReverse:              "REVERSE",
	OpCallDynamicTrailing:  "CALL_DYNAMIC_TRAILING",
	OpFlowBreak:            "FLOW_BREAK",
	OpFlowContinue:         "FLOW_CONTINUE",
	OpStackMark:            "STACK_MARK",
	OpDropToMark:           "DROP_TO_MARK",
	OpPopMark:              "POP_MARK",
	OpSeatBelowMark:        "SEAT_BELOW_MARK",
	OpMakeListToMark:       "MAKE_LIST_TO_MARK",
	OpCallDynamicMixed:     "CALL_DYNAMIC_MIXED",
	OpInterp:               "INTERP",
	OpInterpXml:            "INTERP_XML",
	OpSpliceDyn:            "SPLICE_DYN",
	OpCallUserPoly:         "CALL_USER_POLY",
	OpCallDynTrailTop:      "CALL_DYN_TRAIL_TOP",
	OpCallDynApplyTop:      "CALL_DYN_APPLY_TOP",
	OpCallDynApplyOne:      "CALL_DYN_APPLY_ONE",
	OpCallDynTrailKeepQ:    "CALL_DYN_TRAIL_KEEPQ",
	OpCallDynFrame:         "CALL_DYN_FRAME",
	OpPushConstFresh:       "PUSH_CONST_FRESH",
	OpPushConstFreshLocal:  "PUSH_CONST_FRESH_LOCAL",
	OpBindTyped:            "BIND_TYPED",
	OpCallDynMethod:        "CALL_DYN_METHOD",
	OpLookupDynScope:       "LOOKUP_DYN_SCOPE",
	OpBindDynScope:         "BIND_DYN_SCOPE",
	OpCallDynMixedFromMark: "CALL_DYN_MIXED_FROM_MARK",
	OpBindGlobal:           "BIND_GLOBAL",
	OpLookupDynScopeData:   "LOOKUP_DYN_SCOPE_DATA",
	OpBindTwin:             "BIND_TWIN",
	OpDeoptIfFn:            "DEOPT_IF_FN",
	OpBindResident:         "BIND_RESIDENT",
}

func (o Opcode) String() string {
	if int(o) < len(opcodeNames) && opcodeNames[o] != "" {
		return opcodeNames[o]
	}
	return fmt.Sprintf("OP(%d)", uint8(o))
}

// PolyRef names one runtime-dispatched native call: the word and the arity
// (operand count) the checker fixed at the call site. OpCallNativePoly runs
// MatchSignature over the word's signatures against that many stack values.
type PolyRef struct {
	Word  string
	Arity int
	// NOut is the result-count CLAIM the recorder's stack model committed
	// downstream ops to. The runtime re-match may land on a DIFFERENT
	// overload than the checker's model (that is poly's point), and when
	// that overload's result count differs from the claim the program's
	// stack layout no longer holds — the VM defers to the interpreter via
	// internal_error (runtimeShouldFallback: slow, not wrong) instead of
	// silently shifting every downstream operand.
	NOut int
	// Reg is the sub-registry whose signatures the VM re-matches a MODULE poly
	// word over (`StructUtil.getpath` — a sub-registry word). Nil means the main
	// registry (the common case: a core builtin like get/size/is). The pointer
	// is the same sub-registry the check pass created on the shared registry, so
	// it stays valid for the compiled run (RunProgram runs on that registry).
	Reg *core.Registry
	// NoMatch, when non-nil, is the FAITHFUL-RAISE plan for the runtime
	// no-match arm (plan 3c): the check pass proved, at the failed-dispatch
	// tape state it recovered from, that the interpreter's sigError diagnostic
	// is rebuildable from the runtime operand window alone, so callPoly raises
	// the byte-identical signature_error directly instead of deferring the
	// whole run to the interpreter. Nil (the record-time gates declined, or an
	// older/foreign record site) keeps the sound defer.
	NoMatch *core.PolyNoMatchSpec
}

// UserPolyRef names one runtime-dispatched multi-overload USER-FN call: the
// word, the arity the checker fixed at the call site, and the recorded
// same-arity overload arms — SigIdx[i] indexes the word's aggregated dispatch
// table (Registry.Lookup(Word).Signatures), Units[i] is the arm's compiled
// body unit in Program.Fns, and Impls[i] is the arm's run-implementation
// identity (Signature.Impl is a stable pointer per installed overload). The VM
// re-derives the arm subset from the LIVE table at run time and verifies each
// entry's Impl against Impls — any drift (a re-def between the compile and the
// run) defers to the interpreter rather than running a stale body unit.
// The user-fn mirror of PolyRef (OpCallNativePoly).
type UserPolyRef struct {
	Word  string
	Arity int
	// Reg is the registry the check pass dispatched the word against (a module
	// fn re-matches over its own sub-registry). Nil means the VM's registry.
	// Like PolyRef.Reg, the pointer is the same registry the check pass ran on,
	// so it stays valid for the compiled run.
	Reg    *core.Registry
	SigIdx []int
	Units  []int
	Impls  []core.SigImpl
	// Sigs, when non-empty, is the STORED dispatch table (REFUSAL-CLOSURE.0
	// §6b): the arm signatures frozen at record time, for a BODY-LOCAL
	// multi-overload fn whose binding is popped before the VM runs — a live
	// name Lookup could never resolve it, so the runtime re-match runs over
	// this frozen subset instead (matchUserPoly's stored mode). Freezing is
	// faithful because a body-local fn's construction is source-determined
	// and per-call identical (captures and conditional redefinitions refuse
	// upstream, and a same-named local in ANY other fn refuses the freeze —
	// the dynamic-scope mutation gate in tryCompileUserPolyArms), so the
	// frozen table IS the table the interpreter's dispatch sees at the same
	// program point. Empty = the live-Lookup mode with its index/Impl drift
	// guard (module-scope words, where a later rebind must defer).
	Sigs []core.Signature
}

const (
	// ClosureInValue passes the per-invocation inputs through unchanged — the
	// token-quotation form (list element / map value, plus fold's accumulator).
	ClosureInValue core.ClosureInShape = iota
	// ClosureInKeyVal wraps a map entry as a KeyVal {k v i n} before the (last)
	// input — the map-iteration LAMBDA convention (`each (kv => …) {m}`).
	ClosureInKeyVal
	// ClosureInStackPair says the handler hands its inputs in STACK order
	// (deeper first) while the body's params were declared in the order the
	// interpreter's top-down assignment produces, so the bind REVERSES them.
	//
	// This is the list fold/scan lambda, and it is the one place the two
	// engines' argument rules genuinely disagree rather than merely look like
	// they might. Both handlers hand `(accumulator, element)`. The MAP path
	// calls the lambda directly — CallBoruFn binds args positionally, so
	// sig[0] is the ACCUMULATOR. The LIST path goes through InvokeBody, whose
	// interpreter arm runs the inputs as a STACK (RunResolved), and
	// MatchSignature fills from the top down — so sig[0] is the ELEMENT. Same
	// handler order, opposite assignment, because one path is positional and
	// the other is a stack.
	//
	// A compiled closure is positional like the map path, so without this the
	// list pair arrives swapped:
	//
	//	fold ([e:Integer a:Integer] => [a mul 10 add e]) [1 2 3] 0
	//	  interpreted 123    compiled (unpermuted) 60
	//
	// Carriers cannot fix it — they TYPE the body, they do not choose which
	// local receives what — which is why this rides as a per-word shape at the
	// bind rather than as a carrier order.
	ClosureInStackPair
)

// NewClosure builds a closure Value over prog's body unit (default value input
// shape). The VM stamps the unit's real InShape at OpPushClosure.
//
// prog is required rather than optional: a closure's Unit is an index into ITS
// program's Fns table, and the VM routes an invoke by that identity
// (ClosurePayload.Prog). A nil prog is legal and means "no identity recorded" —
// it reads as whichever program is running, which is only ever right for a
// value that is inspected rather than invoked.
func NewClosure(prog *Program, unit int, captures []core.Value) core.Value {
	return core.Value{Parent: core.TFunction, Data: core.ClosurePayload{Prog: prog, Unit: unit, Captures: captures, Ident: core.NewFnIdentity()}}
}

// ClosureWantsKeyVal reports whether v is a compiled closure whose body expects
// a map entry presented as a KeyVal (the map-iteration lambda convention), so a
// map-iteration handler wraps the entry rather than passing the bare value.
func ClosureWantsKeyVal(v core.Value) bool {
	cl, ok := v.Data.(core.ClosurePayload)
	return ok && cl.InShape == ClosureInKeyVal
}

// IsCompiledClosure reports whether v is a compiled-closure VALUE (a body unit
// the VM runs via InvokeBody), as opposed to an interpreter FnDefInfo lambda.
// Both are Parent=TFunction, so a higher-order handler that treats a lambda
// differently from a plain code body (e.g. map iteration hands a lambda a
// KeyVal but a body the value) must discriminate on this.
func IsCompiledClosure(v core.Value) bool {
	_, ok := v.Data.(core.ClosurePayload)
	return ok
}

// CompiledFnRef is a DURABLE reference from a runtime fn VALUE to its AOT-
// compiled body unit. Unlike a ClosurePayload — which OpPushClosure feeds
// straight to a higher-order handler INSIDE the live VM run — a CompiledFnRef
// rides on an FnDefInfo-shaped value that a native callback word (a serve-raw
// connection handler, a spawned process) invokes AFTER the enclosing RunProgram
// has returned, from its own forked registry. RunUnit uses it to start a fresh
// VM run entered at the unit. Prog is the parent program whose Consts / Fns /
// Sigs pools the unit's instructions index; Unit indexes Prog.Fns; Captures are
// the construction-time lexical captures bound into the body's trailing local
// slots (nil for a capture-free body).
//
// Prog is stamped at Finalize — the point the *Program first exists — over the
// Program.storedFnRefs side-list, so a ref recorded before Finalize gets its
// program back-filled once. A nil Prog (a ref that never reached Finalize)
// means "no runnable unit": the invoke seam treats it as absent and falls back
// to the interpreter, so a missed stamp is slow, never wrong.
type CompiledFnRef struct {
	Prog     *Program
	Unit     int
	Captures []core.Value
	// depNames are the MODULE-LEVEL names the stored handler / spawn body reads
	// (every body word bound as a user `def` when the ref was created at its
	// store site). A stored unit is FROZEN at the definitions live when it was
	// compiled; the interpreter resolves the same names at CALL time. So if any
	// dep is undef'd or redefined LATER in the program — after this ref was
	// created — NotifyNameRebound sets poisoned, Finalize leaves Prog nil, and
	// InvokeCallback falls back to CallBoru, which resolves the live definition
	// exactly as the interpreter does. Compile-time only; unused at run time (a
	// stamped ref always has poisoned=false).
	depNames map[string]bool
	poisoned bool
	// optional marks a ref that exists ONLY as an optimisation — stampFnConst's
	// fn-value consts, whose fallback is the island the program used before
	// anything stamped them. It changes what a dep REBIND costs: poisoning
	// drops the ref (Finalize leaves Prog nil, the apply islands), and for an
	// optional ref that IS the whole remedy, because islanding restores the
	// exact pre-stamp behaviour the differential already validates.
	//
	// A store-site ref is not optional and keeps the program-level refusal:
	// its handler is invoked at RUNTIME, after module-scope def sites have all
	// executed in the compile pass, so the CallBoru fallback reads the
	// PASS-FINAL binding where the interpreter read the point-in-program one
	// (design/RELOAD-INVALIDATION.0.md §3 F1). Nothing the compile pass can do
	// makes that right, which is why the whole program falls back there.
	optional bool
	// DepSnap is the RUNTIME-stamped twin of the compile-time poisoning above
	// (StampDetachedFn): a ref created OUTSIDE a whole-program pass has no
	// recording EmitState alive to observe later rebinds, so freshness moves
	// to invoke time. Each dep name maps to the binding state captured when
	// the ref was stamped — the DefTable shadow depth plus the name's
	// mutation generation (DefTable.Gen, bumped by every push / pop /
	// replace / truncate / delete / set of that name, and carried across
	// ForkConcurrent clones). A generation mismatch catches every rebind
	// path — including an undef+redef that lands back at the same depth
	// with an ID-less runtime value, which a depth+ID probe would miss —
	// PLUS live shadowing (a body-local def of the same name active at
	// invoke time) that compile-time poisoning structurally cannot see.
	// InvokeCallback checks depsFresh before the VM path; any mismatch
	// falls to CallBoru, which resolves the live binding exactly as the
	// interpreter. nil = compile-time ref, no validation (nil is the
	// unambiguous unset for a map).
	DepSnap map[string]DepSnapEntry
	// Restamp is the JIT re-stamp box (REFUSAL-CLOSURE.0 §7c): a DETACHED
	// ref whose DepSnap went stale re-compiles against the LIVE bindings at
	// invoke time (CompiledFnRef.jitRestamp) instead of degrading
	// permanently to CallBoru. Allocated by StampDetachedFn only — a
	// compile-time ref re-poisons via NotifyNameRebound and never re-stamps
	// (nil box → the interpreter, as before). A POINTER field keeps the
	// struct copyable while the box's mutex serialises concurrent invokers
	// of the same shared sig.
	Restamp *RestampBox
}

// RestampBox carries a detached ref's stamp inputs and its current
// re-stamped twin (see CompiledFnRef.Restamp). Tries caps the TOTAL
// re-compiles per ref so a hot rebinding loop cannot pay a compile per
// invoke — once exhausted, the seam stays on CallBoru (slow, not wrong).
type RestampBox struct {
	mu     sync.Mutex
	fd     core.FnDefInfo
	sigIdx int // which own sig this ref compiled (REFUSAL-CLOSURE §7b: per-sig refs)
	pos    core.SrcPos
	Tries  int
	Cur    *CompiledFnRef
}

// DepSnapEntry is one dep's binding state at stamp time (see DepSnap).
type DepSnapEntry struct {
	Depth int
	Gen   int64
}

// depsFresh reports whether every module-level dep a runtime-stamped unit's
// body reads still resolves to the binding captured at stamp time on r's def
// table. A compile-time ref (nil DepSnap) is vacuously fresh — its staleness
// is handled by NotifyNameRebound poisoning before Finalize. Any mismatch —
// a changed generation (rebind, undef, in-place replace) or a changed depth
// (live shadow) — reports false and the caller falls back to the
// interpreter, so validation only ever degrades toward CallBoru, never away
// from it.
func (ref *CompiledFnRef) DepsFresh(r *core.Registry) bool {
	if ref == nil || ref.DepSnap == nil {
		return true
	}
	if r == nil {
		return false
	}
	for name, snap := range ref.DepSnap {
		if r.Defs.Gen(name) != snap.Gen || r.Defs.Depth(name) != snap.Depth {
			return false
		}
	}
	return true
}

// Instr is one fixed-width instruction.
type Instr struct {
	Op  Opcode
	Arg int32
}

// SigRef names one interned signature: the word plus the exact
// *Signature the checker selected at the call sites that reference it.
type SigRef struct {
	Word string
	Sig  *core.Signature
	// Guard marks a CALL_NATIVE recorded for a dispatch the checker could NOT
	// statically commit (a concrete-mismatch / Any-carrier recovery over a
	// SINGLE-overload native) — the compiled mirror of the interpreter's runtime
	// matchSignature. The VM re-checks the concrete args against Sig.Args before
	// the handler (checkNativeParamContract): on match it dispatches Sig.Handler
	// (== the interpreter's sole-overload dispatch), on mismatch it raises the
	// byte-identical signature_error (== the interpreter, which finds no overload).
	// Sound ONLY for a single-overload word: with a sibling overload a runtime arg
	// that misses Sig could match the sibling, where the interpreter dispatches but
	// the guard raises. Unlike OpCallNativePoly (which re-matches ALL overloads and
	// can OPTIMISTICALLY dispatch a sibling the interpreter rejects — proven unsound
	// for the concrete-mismatch case), the guard never re-matches: it commits to the
	// one sig and raises otherwise, so it diverges from the interpreter only if a
	// sibling exists — which the single-overload gate forbids.
	Guard bool
}

// TypeRef names one type operand: the canonical type ID (resolved
// through the registry at run time) plus the display name for the
// disassembler.
type TypeRef struct {
	Name string
	ID   string
}

// MakeMapSpec names the key list (and the source map's Implicit flag) of one
// OpMakeMap assembly: the VM pops len(Keys) values and pairs Keys[i] with the
// i-th value (deepest popped = value 0). The keys ride here rather than as
// stack operands so OpMakeMap only handles the VALUE operands (which may be
// computed event results), reusing the same operand-layout engine as a call.
type MakeMapSpec struct {
	Keys     []string
	Implicit bool
}

// InterpSeg is one segment of an OpInterp template: either a literal run of
// text (Hole false, Lit set) or a hole that consumes the next popped value
// (Hole true). Segments are in source order.
type InterpSeg struct {
	Lit  string
	Hole bool
}

// InterpSpec is the template of one OpInterp assembly: the ordered segments
// and the hole count (= how many operand-stack values the op pops). The
// literal text rides here rather than as stack operands, so OpInterp only
// pops the computed hole VALUES — reusing the same operand-layout engine as a
// call (mirrors MakeMapSpec).
type InterpSpec struct {
	Segs   []InterpSeg
	NHoles int
}

// XmlInterpSpec is OpInterpXml's template skeleton: the parsed XmlTmpl
// (tags, attribute names, literal segments — the ${...} holes consume the
// popped operands in traversal order) and the hole count to pop.
type XmlInterpSpec struct {
	Tmpl   core.XmlTmpl
	NHoles int
}

// GlobalBindSpec describes one OpBindGlobal: the top-level def's name and the
// binding DEPTH its check-pass install recorded (RecordDynBind stamps
// Defs.Depth(name) right after InstallDef), so the runtime value lands in the
// exact kept slot even under same-name shadowing (`def x (f) def x (g)`
// records depths 1 and 2; each write-back hits its own level). A slot popped
// by a later check-time undef makes the write a no-op — the interpreter would
// have discarded the binding the same way.
// ResidentBindSpec describes one OpBindResident — see the opcode's doc.
// Name is the binding; Twin indexes Program.BindTwins (the satisfied
// ledger entry — provenance and placement accounting, never a value
// source); Undef selects the teardown arm of a balanced per-iteration
// pair (a var param's def/undef, both executing per element so the pair
// nets zero per completed iteration and a mid-body raise leaves the def
// half installed, interpreter-identical). Pop mirrors GlobalBindSpec's
// mode split: the install arm PEEKS by default — a computed def's value
// sits live on the unit sim for its downstream readers (the def consumes
// nothing the compiled model still needs) — and POPS when the lowering
// pushed a copy for the install (a baked literal, a promoted local).
type ResidentBindSpec struct {
	Name  string
	Twin  int
	Undef bool
	Pop   bool
	// TypeInstall selects the TYPE arm: the binding has no runtime value,
	// so the op re-installs BindTwinEntries[Twin].Body per element, minting
	// that element's own node (a top-level OpBindTwin replays the captured
	// node instead, which is right for ONE execution and wrong for N —
	// NUR135). Mutually exclusive with Undef and Pop, and never emitted
	// until AdoptResidentTwins has proved the type expression
	// element-independent.
	TypeInstall bool
}

type GlobalBindSpec struct {
	Name  string
	Depth int
	// Pop selects the copy-path mode: the lowering re-pushed the value from
	// its promoted frame local, so the bind consumes the copy (one op, no
	// separate DROP). The fast path peeks the live value in place (Pop=false)
	// and leaves it for its downstream consumers.
	Pop bool
	// Splice selects the S5 first-value loop-bind mode: bind the value
	// SpliceFromTop entries below the stack top and remove it — the
	// interpreter's pending-forward collection of a loop region's first
	// value, at the region's statically-known depth (REFUSAL-CLOSURE S5).
	Splice        bool
	SpliceFromTop int
}

// ConstLocalRef backs OpPushConstFreshLocal (see the opcode doc): ConstIdx names
// the pooled Program.Consts entry to deep-clone once per call, Slot the fn-unit
// frame-local it is seated in and re-read from.
type ConstLocalRef struct {
	ConstIdx int
	Slot     int
}

// TrapSpec describes the boru error one OpTrap raises: the taxonomy code, the
// detail message, the word it is attributed to, an optional hint, and the full
// structured diagnostic payload (secondary spans, notes, suggestions) — built
// from the SAME BoruError the interpreter raises for the matching runtime error,
// so the compiled and interpreted diagnostics are byte-identical and
// error-scraping tooling can never tell which engine ran. It lowers a
// check-mode-suppressed runtime error (an orphan gen, an unpack of a missing
// key, a statically-definite unmatched dispatch) into the compiled stream
// rather than refusing the whole program. The rich fields are populated by
// RecordTrapErr (serialising a built BoruError); the plain string RecordTrap
// leaves them nil for the simpler callers.
type TrapSpec struct {
	Code        string
	Detail      string
	Word        string
	Hint        string
	Spans       []core.DiagSpan
	Notes       []string
	Suggestions []core.DiagSuggestion
}

// DispatchSpec describes one OpDispatchRematch (see the opcode doc): the
// word whose dispatch statically failed, the failed window's operand count
// (the values the recorder pushed, stack[top-i] = sig position i), and the
// dispatch site's source position, stamped onto the runtime-built
// diagnostic so it labels the same site the interpreter's would.
//
// NWritten is the RENDER BOUND: how many LEADING window operands form the
// WRITTEN tuple the interpreter's sigError renders (its forward-else-stack
// derivation). The match view can be wider than the raise view — the
// local-add shape's match probed 3 positions where its error renders the
// single stack value — so the rematch re-runs the match over the FULL
// window but builds the diagnostic over window[:NWritten]. Always explicit,
// 1..NArgs (never the Go zero): the record gate proves the bound by ID
// identity (the written tuple IS the window's leading slots) before
// recording, and the VM rejects a spec outside the range.
type DispatchSpec struct {
	Word     string
	NArgs    int
	NWritten int
	// WrittenOff is the 0-based window index where the written slice starts
	// (the each shape's written tuple is the body operand at offset 1, after
	// the region carrier). Valid domain 0..NArgs-NWritten; NWritten >= 1 is
	// what makes the pair explicit — a spec with NWritten 0 is malformed.
	WrittenOff int
	Pos        core.SrcPos
}

// DynMethodSpec is one OpCallDynMethod's shape claim (Stage M2c): the member
// word name (diagnostics only — dispatch is over the runtime VALUE, never the
// name), the arity the check-mode match consumed, and the result count the
// matched member signature declares. The VM enforces both halves of the claim
// and defers to the interpreter via internal_error when either fails.
type DynMethodSpec struct {
	Word  string
	NArgs int
	NOut  int
}

// Program is a compiled unit: code, interned constants, the signature
// table, a pc → source-position map, and the precomputed stack bound.
type Program struct {
	Code       []Instr
	Consts     []core.Value
	Types      []TypeRef
	Sigs       []SigRef
	PolyRefs   []PolyRef
	UserPolys  []UserPolyRef
	Fallbacks  []core.FallbackSpan
	MakeMaps   []MakeMapSpec
	Interps    []InterpSpec
	XmlInterps []XmlInterpSpec
	Traps      []TrapSpec
	Dispatches []DispatchSpec
	// Regions backs OpCollect / OpDispatchGeneric: one entry per G-lane
	// region, recording what the interpreter would have read off the tape
	// (region_desc.go). Peer to Dispatches. Nil for a program with no
	// generically-lowered dispatch.
	Regions []RegionDesc
	// ClosureRet carries a pushed closure's CALLBACK return contract, keyed by
	// the pc of its OpPushClosure. Keyed by pc rather than by unit because the
	// unit is SHARED across fn values with identical bodies and inputs — the
	// contract is the value's, not the body's.
	ClosureRet map[int]ClosureRetSpec
	// StoreNames is the main code's twin of CompiledFn.StoreNames (see
	// there), keyed by the main code's own pc.
	StoreNames map[int]string
	TypedBinds []core.TypedBindSpec
	// GlobalBinds backs OpBindGlobal: one entry per top-level computed `def`,
	// naming the binding and the DEPTH its check-pass install recorded. The
	// runtime value is PUSHED: the pass's install was rolled back to
	// ReplayBase before the run, so there is no kept carrier to replace.
	GlobalBinds []GlobalBindSpec
	// BindTwins is the program's bind-twin table (design/FULL-COMPILATION.0.md
	// §6.5): the check pass's bind ledger, mirrored entry for entry through
	// NoteBindTransition's own funnel and finalized with the Program. Every
	// entry has an op at its source position (Finalize's full-placement gate
	// refuses otherwise) — OpBindTwin replays it, OpBindResident installs it
	// per invocation — and the langspec gate asserts table == ledger for
	// every compiled corpus program.
	BindTwins []core.BindTransition
	// BindTwinEntries is 1:1 with BindTwins: the DefEntry each push
	// transition installed, captured at the note — the identical binding
	// object the twin re-installs under rollback-and-replay (§6.5 "replay,
	// never re-execution"). Zero for a BindUndef (its twin pops the
	// then-live entry). Proven against the pass-left registry, corpus-wide,
	// by the sandbox harness (test/go/langspec/bind_replay_sandbox_test.go).
	BindTwinEntries []core.DefEntry
	// ResidentBinds backs OpBindResident (the arm-resident twins —
	// §6.5's each-body recovery): one entry per resident install/teardown
	// site inside a compiled per-invocation unit. Twin indexes BindTwins
	// — the ledger entry the resident op satisfies for the full-placement
	// scan; the entry's captured generalized value is deliberately NOT
	// replayed (the op carries the runtime value instead, which is the
	// whole point). Only a twin-regime program carries entries.
	ResidentBinds []ResidentBindSpec
	// ReplayBase is the twin regime's ROLLBACK BASE (§6.5): the program
	// registry's runtime-visible bindings as they stood when the recorder
	// first bound it (EmitState.BindRegistry — before the check pass
	// performed a single transition). RunProgram restores it before
	// executing, so the placed twins replay onto the pre-pass state instead
	// of stacking a second install on the pass's kept one. Carried BY THE
	// PROGRAM so every compile-then-run caller inherits the rollback —
	// lang's entry points and the low-level CompileCheck-then-RunProgram
	// flow alike (Codex P1 on #426: the direct flow left `def X (refine
	// Integer)` at depth 2, and a later `undef X` a binding and its type
	// live). Zero (invalid) for a hand-built Program: the restore is a no-op
	// and the caller owns its registry. Restored only when BindTwins is
	// non-empty — an empty table means the pass moved no binding (table ==
	// ledger), so there is nothing to roll back and no def-table clone to pay.
	ReplayBase core.BindingSandbox
	// ReplayReg is the registry ReplayBase was captured from — the program
	// registry (EmitState.progReg). RunProgram restores only when it runs
	// ON that registry: a program run elsewhere has nothing of its own to
	// roll back there, and a foreign DefTable must never be installed.
	ReplayReg  *core.Registry
	DynMethods []DynMethodSpec
	// ConstLocals backs OpPushConstFreshLocal: a {ConstIdx, Slot} pair naming the
	// pooled const to deep-clone and the frame-local slot to seat it in, for a
	// multi-read compound body literal that needs one per-call construction shared
	// across its read sites. Slot is frame-relative to the fn unit whose Code holds
	// the op (each op appears in exactly one unit). Nil for programs with none.
	ConstLocals []ConstLocalRef
	Fns         []CompiledFn
	Debug       []core.SrcPos // 1:1 with Code
	MaxStack    int           // a floor when the program loops (results accumulate)
	NumLocals   int
	// DynEnv marks a program containing a dynamic code-body dispatch
	// (CompileDynBody — tryRecordDynBody): the VM brackets every CALL_USER
	// frame with an args-stack push so a body's runtime sub-run reads `args`
	// exactly as under the interpreter (name visibility rides the widened
	// OpBindDynScope emission). Ordinary programs pay nothing.
	DynEnv bool
	// storedFnRefs is the set of CompiledFnRefs recorded during compilation
	// (store-fn handler bakes) whose Prog pointer Finalize back-fills once the
	// *Program exists. Not part of the executable program; a build-time
	// side-list so Finalize needn't re-scan Consts structurally. Nil for a
	// program with no stored-fn callbacks.
	storedFnRefs []*CompiledFnRef
}

// CompiledFn is one compiled boru fn overload at one arg shape: its
// own code unit with frame-relative locals (params in slots 0..N-1,
// sig order).
// DynFrameWord is one entry of CompiledFn.DynFrameWords: the binding NAME a
// replayed token-region entry was read bare under, and the READ's source
// position — where the interpreter anchors the dispatch's own errors.
type DynFrameWord struct {
	Name string
	Pos  core.SrcPos
	// Read marks a region entry that arrived by a BARE READ of a binding,
	// whatever its type — the syntactic fact Name does not carry, since Name
	// is set only for the fn-admitting reads noteWordRead gates on.
	//
	// The no-match diagnostic needs it: the interpreter prints the tokens its
	// forward window CONSUMED, and the window stops at the first word read
	// (the pointer had already substituted it onto the value stack), so the
	// tuple is the longest LEADING RUN of non-read arguments — `(g 5 y 6)`
	// reports `[5]`, not `[5 6]`. Measured 2026-09-07; NUR122.
	Read bool
}

// DynApplyHead is one entry of CompiledFn.DynApplyName: the binding NAME the
// head of a paren-bounded trailing fn-value apply was read under, that READ's
// position, and how many of the apply's arguments the interpreter's forward
// window would have consumed (see DynFrameWord.Read for the rule).
type DynApplyHead struct {
	Name     string
	Pos      core.SrcPos
	NWritten int
}

type CompiledFn struct {
	Name    string
	NParams int
	// NCaptures is how many of the NParams leading slots are CAPTURES (for a
	// closure body unit): the per-invocation inputs fill slots
	// 0..NParams-NCaptures-1, the captures fill the trailing NCaptures slots.
	// OpPushClosure pops NCaptures values off the stack into the closure's
	// captures; 0 for an ordinary user fn or a capture-free body. (For a user
	// fn the captures still ride as trailing CALL_USER args, so NCaptures
	// stays 0 there — it is closure-specific.)
	NCaptures int
	// Render is the interpreter's formatFnDef string for a returned-closure
	// unit (empty otherwise) — see ClosurePayload.Render.
	Render string
	// Lambda marks the fn-VALUE flavour of a closure unit (an anonymous
	// `=>` / `fn` literal's body): the interpreter's FnDefInfo for it is
	// Anonymous, which parks a 0-arg lambda VALUE nothing calls (ADR-016's
	// gate), and the value-path bridge (CompiledRuntime.ClosureAsFnDef)
	// carries the flag so a compiled closure parks in the same places.
	Lambda bool
	// NArgs is the fn's REAL argument count — the sig-matched args, excluding
	// the trailing capture slots a user fn's call site pushes (NParams
	// includes them; NCaptures stays 0 for user fns). The DynEnv args bracket
	// pushes exactly locals[0:NArgs] as the frame's args list — the same list
	// the interpreter's per-call args push holds.
	NArgs int
	// NUnnamed is how many of the params are UNNAMED (stack-flowing): the
	// lowering re-pushes each unnamed param onto the operand stack at unit
	// entry (mirroring the interpreter's frame, where unnamed args sit
	// resolved at the body's bottom), so an unnamed arg the body never
	// consumes remains in the frame region at RET. The RET contract
	// (checkReturnContract) DISCARDS up to NUnnamed extra bottom values —
	// the exact __RC discipline (engine.go: extras beyond the declared
	// returns are tolerated up to the unnamed-arg allowance, then trimmed).
	// 0 for all-named fns and closures.
	NUnnamed int
	NLocals  int
	// Reg is the fn's OWNING registry when it differs from the program's —
	// a module-preamble fn compiled through a foreign dispatch. The VM runs
	// the unit's native dispatches against it (vmContext curReg), exactly
	// as the interpreter's CallBoru runs the body in the fn's own registry:
	// module-private names resolve there, and registry-visible handler
	// effects (Net.listen forking per-connection registries, dynamic-scope
	// binds) land in module scope on both engines. Nil for ordinary fns —
	// the unit runs on the program's registry. The pointer is the same
	// sub-registry object the check pass created on the shared registry
	// (like PolyRef.Reg), so it stays valid for the compiled run.
	Reg *core.Registry
	// InShape is the input convention a closure over this unit presents to its
	// driving handler (ClosureInValue for an ordinary fn / token body, or
	// ClosureInKeyVal for a map-iteration lambda body). Copied onto the
	// ClosurePayload at OpPushClosure. The VM never branches on it; the native
	// map-iteration handler reads it via ClosureWantsKeyVal.
	InShape core.ClosureInShape
	Code    []Instr
	Debug   []core.SrcPos
	// Returns are the declared return types, enforced at RET against the
	// body's result the same way the interpreter's ReturnCheck (__RC)
	// does — via v.Is(exp), so a predicate refine runs its predicate, a
	// bare refine stays nominal, and builtins are unchanged. Empty for a
	// fn with no declared return (no check runs).
	Returns []*core.Type
	// ReturnPatterns are the per-return structural/value patterns
	// (FnSig.ReturnPatterns), positional against Returns — the RET-side twin
	// of ParamPatterns. A declared UNION return (`def IS (Integer tor
	// String)` used as an output sig) has no lattice node to name, so its
	// Returns entry degrades to Any and admits everything; the pattern is
	// then the whole contract. Enforced at RET via the same Unify the
	// interpreter's ReturnCheck runs, so the compiled and interpreted
	// engines agree. Nil where the return has no pattern.
	ReturnPatterns []*core.Value
	// Params are the declared PARAM types (param slots 0..len(Params)-1, which
	// align with the leading param locals; captures, if any, follow and are NOT
	// listed). Enforced at CALL_USER entry against the incoming arg the same way
	// Returns is enforced at RET — via v.Is(exp). This guards the gradual-Any
	// boundary: a value of static type exactly Any optimistically matches a
	// concrete param at check time, but the runtime value may not match; the
	// interpreter runtime-matches it, so the compiled OpCallUser must too, else a
	// laundered List bound to an `m:Map` param silently runs the body. A nil
	// entry (a closure's [Any] input) is a guaranteed-pass, like Returns=[Any].
	Params []*core.Type
	// ParamPatterns are the per-param structural/value patterns (FnParam.Pattern):
	// an inline disjunct (`x:(Integer tor String)`), inline predicate
	// (`b:(Integer gt 10)`), bounded (`x:Map/t`), or map/list shape — the
	// constraint that rides in Pattern, NOT Type (Type is left a loose root for
	// these). Checked at CALL_USER alongside Params via the SAME OpenUnifyMap /
	// Unify the interpreter's dispatch runs, so a value laundered past such a
	// param raises the same signature_error rather than running the body. Nil
	// where the param has no pattern.
	ParamPatterns []*core.Value
	// Decl is the return-contract declaration site (FnSig.Decl): the
	// output-signature token's position plus the declaring program's
	// source/file. A compiled RET return error labels it as a secondary
	// span exactly as the interpreter's ReturnCheck does — so the two
	// engines' return diagnostics carry the same declaration span. Zero
	// for anonymous closures (no meaningful declaration site).
	Decl core.DeclSite
	// LocalNames maps a frame local slot to its source name (params in
	// slots 0..NParams-1, then captures), for a debugger / disassembler.
	// Body-local iterator slots have no name (empty string). Purely
	// metadata — the VM never reads it.
	LocalNames []string
	// ClosureRet is the unit-code twin of Program.ClosureRet: a closure
	// PUSHED INSIDE THIS UNIT carries its callback return contract here,
	// keyed by the push's unit-local pc. It has to be per unit: the pc
	// spaces of the main code and of every unit overlap, so one program-wide
	// map cannot address them — and until it was split, the unit lowerer
	// keyed every in-unit push at the MAIN code's length, a pc the VM never
	// reaches inside a unit, so `def f fn [[] [Any] [each (x:Integer => [x
	// 1]) [1 2]]]  f` answered `[1 1]` compiled where the interpreter raises
	// the count error (measured 2026-09-05, the in-unit face of NUR120).
	ClosureRet map[int]ClosureRetSpec
	// DynFrameWords names, per OpCallDynFrame (keyed by its unit-local pc),
	// which entries of the replayed token region were BARE READS of a frame
	// binding — index i of the slice is the i-th region entry (deepest
	// first), "" for an entry that was not such a read. A bare read of a
	// binding holding a fn is a WORD dispatch on the interpreter (stepWord
	// routes a bound FnDefInfo through Registry.Lookup under the binding
	// name: a 0-arg fn fires, a no-match raises `cannot call `g``), so the
	// VM installs each such fn-valued entry as a frame binding and re-steps
	// the region with the WORD in its place (vm.go callDynFrame, NUR123).
	// Nil for a replay whose region holds no bare read.
	DynFrameWords map[int][]DynFrameWord
	// DynApplyName names the BINDING the head of a paren-bounded trailing
	// fn-value apply was read under, per OpCallDynTrailTop /
	// OpCallDynTrailKeepQ (keyed by its unit-local pc), together with the
	// READ's source position.
	//
	// The op applies a VALUE, so its own no-match diagnostic had no name and
	// no position to give: the interpreter reads `g` as a WORD (stepWord
	// routes a bound FnDefInfo through Registry.Lookup) and raises `cannot
	// call `g`` at the read, while the compiled lane raised `cannot call
	// ``. That is the same divergence DynFrameWords closes for the replay,
	// at the one dispatch shape the replay never reaches — a paren-bounded
	// apply whose window the recorder resolved statically. The applied fn
	// is a frame SLOT, never a registry binding, so the VM cannot recover
	// the name at run time; it has to ride in the bytecode.
	//
	// Nil for an apply whose head was not a bare read (an event-produced
	// fn, a `/v` delivery) — the nameless diagnostic stays exactly as it was.
	DynApplyName map[int]DynApplyHead
	// StoreNames names the DEF a promoted STORE_LOCAL binds a produced fn
	// value under, keyed by the store's pc: the interpreter's installDef
	// renames a fn value bound by `def` (`fnDef.Name = name`), so `def h
	// (mk 1)` renders `fn h(Integer)` and names h in the value's own
	// diagnostics (`h: expected 1 return value(s), got 2`), where a
	// compiled closure stored to its local kept the payload nameless. The
	// VM's store op renames the ClosurePayload it stores (RetName, Render)
	// exactly there. Nil when the unit binds no produced fn value.
	StoreNames map[int]string
	// RetReplay marks a body that ends in a whole-frame dynamic-apply replay
	// (OpCallDynFrame): its residual count is RUNTIME-variable, so the RET
	// contract switches discipline. A FOREIGN-registry fn (Reg set, a module-
	// preamble fn) is dispatched via CallBoru in the interpreter, whose return
	// path is TRIM-ONLY — it discards up to NUnnamed extra bottom values and
	// has never enforced return count or type (registry.go::CallBoru, the
	// documented asymmetry) — so the compiled RET mirrors that trim, then
	// DEFERS to the interpreter (internal_error → sound whole-program
	// fallback) if the count still differs from the callers' static model. A
	// same-registry fn keeps the frame-path contract (count error + type
	// validation), which checkReturnContract already mirrors byte-identically.
	RetReplay bool
	// Body is the fn's source body tokens (the sig's Boru body, seated by
	// SetUnitBody), the token stream a DEOPT hands to the interpreter from
	// the deopting statement on. Nil for a unit that seated none (a
	// closure body, a stored fn), which then records no deopt point.
	Body []core.Value
	// Deopts is the unit's per-read deopt table (OpDeoptIfFn's Arg).
	Deopts []DeoptSpec
}

// DeoptSpec is one OpDeoptIfFn: the gradual word read it guards (Name, its
// read position Pos), where the read's value lives at the op — a frame
// local (Slot >= 0) or the operand stack Depth entries below the top (Slot
// < 0; the island's prefix drops that entry, the interpreter's frame never
// held it) — the Body token index the interpreter resumes from (Token) and
// the unit's RET (RetPC) the VM continues at with the island's residual.
// A RE-STEP point (Results > 0, NUR124) tests the Results values on top
// instead — a native call's results — and hands them to the island as its
// first tokens, followed by the Body from Token: what the interpreter
// splices back onto the tape and steps after that call. Prefix lists the
// unit's unnamed param slots the interpreter's frame still holds on its
// stack bottom at the point (a named param is a binding the island reads
// through the dyn-scope env; an unnamed one is stack data this unit has
// not pushed yet), seated beneath the frame region as the island's prefix.
type DeoptSpec struct {
	Name    string
	Pos     core.SrcPos
	Slot    int
	Depth   int
	Prefix  []int
	Results int
	Token   int
	RetPC   int
}

// slotNames renders a CompiledFn's slot→name table for the
// disassembler: " [n acc]" with empty (anonymous body-local) slots shown
// as "_". Empty string when no names are known.
func slotNames(names []string) string {
	if len(names) == 0 {
		return ""
	}
	parts := make([]string, len(names))
	for i, n := range names {
		if n == "" {
			parts[i] = "_"
		} else {
			parts[i] = n
		}
	}
	return " [" + strings.Join(parts, " ") + "]"
}

// StoredRefCount reports how many stored-fn callback refs (service/spawn/codec
// handler bakes) were recorded during compilation, and StoredRefStampedCount how
// many were back-stamped with the program (Prog != nil, so InvokeCallback runs
// the compiled unit) rather than left unstamped for interpreter fallback — the
// decline a later def/undef of a dependency name triggers to keep compile ==
// interpret. Test-support introspection for the callback-freeze correctness gate;
// not consulted by execution.
func (p *Program) StoredRefCount() int { return len(p.storedFnRefs) }

func (p *Program) StoredRefStampedCount() int {
	n := 0
	for _, ref := range p.storedFnRefs {
		if ref.Prog != nil {
			n++
		}
	}
	return n
}

// Disassemble renders the program for golden tests and debugging.
func (p *Program) Disassemble() string {
	var sb strings.Builder
	p.disasmUnit(&sb, p.Code, nil)
	for fi := range p.Fns {
		fmt.Fprintf(&sb, "fn f%d %s/%d (locals=%d)%s:\n", fi, p.Fns[fi].Name, p.Fns[fi].NParams, p.Fns[fi].NLocals, slotNames(p.Fns[fi].LocalNames))
		p.disasmUnit(&sb, p.Fns[fi].Code, p.Fns[fi].Deopts)
	}
	fmt.Fprintf(&sb, "; consts=%d types=%d sigs=%d fallbacks=%d fns=%d max-stack=%d locals=%d",
		len(p.Consts), len(p.Types), len(p.Sigs), len(p.Fallbacks), len(p.Fns), p.MaxStack, p.NumLocals)
	// polyrefs is appended only when present, so the common (no-poly) summary —
	// and every golden asserting it — stays byte-identical while a poly program
	// still surfaces its CALL_NATIVE_POLY table count.
	if len(p.PolyRefs) > 0 {
		fmt.Fprintf(&sb, " polyrefs=%d", len(p.PolyRefs))
	}
	// userpolys likewise appears only when present, keeping the common summary
	// (and every golden asserting it) byte-identical.
	if len(p.UserPolys) > 0 {
		fmt.Fprintf(&sb, " userpolys=%d", len(p.UserPolys))
	}
	sb.WriteByte('\n')
	return sb.String()
}

func (p *Program) disasmUnit(sb *strings.Builder, code []Instr, deopts []DeoptSpec) {
	for i, in := range code {
		fmt.Fprintf(sb, "%04d %-11s", i, in.Op.String())
		switch in.Op {
		case OpPushConst, OpLookupDynScope, OpLookupDynScopeData, OpBindDynScope:
			c := p.Consts[in.Arg]
			fmt.Fprintf(sb, " k%-3d ; %s (%s)", in.Arg, core.CanonValue(c), c.Parent.Leaf())
		case OpCallNative:
			s := p.Sigs[in.Arg]
			names := make([]string, s.Sig.TotalArgs())
			for j, t := range s.Sig.ArgTypes() {
				names[j] = t.Leaf()
			}
			guard := ""
			if s.Guard {
				guard = " [guarded]"
			}
			fmt.Fprintf(sb, " s%-3d ; %s (%s)%s", in.Arg, s.Word, strings.Join(names, ", "), guard)
		case OpJmp, OpJmpIfFalse, OpForNext:
			fmt.Fprintf(sb, " -> %04d", in.Arg)
		case OpPushLocal, OpForSetup, OpStoreLocal:
			fmt.Fprintf(sb, " l%d", in.Arg)
		case OpPushType:
			fmt.Fprintf(sb, " t%-3d ; %s", in.Arg, p.Types[in.Arg].Name)
		case OpFallback:
			fb := p.Fallbacks[in.Arg]
			fmt.Fprintf(sb, " b%-3d ; %s (nin=%d)", in.Arg, fb.Desc, fb.NIn)
		case OpCallUser, OpTailCallUser:
			fmt.Fprintf(sb, " f%-3d ; %s/%d", in.Arg, p.Fns[in.Arg].Name, p.Fns[in.Arg].NParams)
		case OpPushClosure:
			fmt.Fprintf(sb, " f%-3d ; closure %s/%d", in.Arg, p.Fns[in.Arg].Name, p.Fns[in.Arg].NParams)
		case OpCallNativePoly:
			pr := p.PolyRefs[in.Arg]
			fmt.Fprintf(sb, " p%-3d ; %s/%d (poly)", in.Arg, pr.Word, pr.Arity)
		case OpCallUserPoly:
			up := p.UserPolys[in.Arg]
			fmt.Fprintf(sb, " u%-3d ; %s/%d (user poly, %d arms)", in.Arg, up.Word, up.Arity, len(up.Units))
		case OpCallDynamic:
			fmt.Fprintf(sb, " /%d ; apply fn-value", in.Arg)
		case OpCallDynamicTrailing:
			fmt.Fprintf(sb, " /%d ; apply trailing fn-value", in.Arg)
		case OpCallDynFrame:
			fmt.Fprintf(sb, " /%d ; replay frame residual (dynamic apply)", in.Arg)
		case OpMakeList:
			fmt.Fprintf(sb, " n%-3d ; assemble %d into a list", in.Arg, in.Arg)
		case OpMakeMap:
			mm := p.MakeMaps[in.Arg]
			fmt.Fprintf(sb, " m%-3d ; assemble {%s}", in.Arg, strings.Join(mm.Keys, " "))
		case OpTrap:
			fmt.Fprintf(sb, " x%-3d ; trap %s", in.Arg, p.Traps[in.Arg].Code)
		case OpReverse:
			fmt.Fprintf(sb, " n%-3d ; reverse top %d", in.Arg, in.Arg)
		case OpInterp:
			fmt.Fprintf(sb, " i%-3d ; interpolate %d hole(s)", in.Arg, p.Interps[in.Arg].NHoles)
		case OpBindTyped:
			tb := p.TypedBinds[in.Arg]
			fmt.Fprintf(sb, " y%-3d ; typed bind %s:%s", in.Arg, tb.Name, tb.Describe)
		case OpBindGlobal:
			gb := p.GlobalBinds[in.Arg]
			fmt.Fprintf(sb, " g%-3d ; global bind %s @depth %d", in.Arg, gb.Name, gb.Depth)
		case OpBindTwin:
			tw := p.BindTwins[in.Arg]
			fmt.Fprintf(sb, " w%-3d ; bind twin %s %s @depth %d (replay)", in.Arg, tw.Kind, tw.Name, tw.Depth)
		case OpDeoptIfFn:
			if int(in.Arg) < len(deopts) && deopts[in.Arg].Results > 0 {
				fmt.Fprintf(sb, " d%-3d ; re-step %d result(s) on the interpreter if one is a fn", in.Arg, deopts[in.Arg].Results)
			} else {
				fmt.Fprintf(sb, " d%-3d ; deopt to the interpreter if the read holds a fn", in.Arg)
			}
		case OpBindResident:
			rb := p.ResidentBinds[in.Arg]
			arm := "install"
			switch {
			case rb.Undef:
				arm = "undef"
			case rb.TypeInstall:
				arm = "type"
			}
			fmt.Fprintf(sb, " a%-3d ; resident bind %s (%s, twin %d)", in.Arg, rb.Name, arm, rb.Twin)
		case OpCallDynMethod:
			dm := p.DynMethods[in.Arg]
			fmt.Fprintf(sb, " d%-3d ; %s/%d -> %d (shaped method)", in.Arg, dm.Word, dm.NArgs, dm.NOut)
		}
		sb.WriteByte('\n')
	}
}

// ReturnPattern returns the declared pattern for return position k, or nil
// when that position has none (or the unit carries no patterns at all).
// Mirrors ReturnCheckInfo.ReturnPattern so the compiled and interpreted RET
// contracts read the same.
func (f *CompiledFn) ReturnPattern(k int) *core.Value {
	if k < 0 || k >= len(f.ReturnPatterns) {
		return nil
	}
	return f.ReturnPatterns[k]
}

// CompiledRef returns the sig's durable compiled-unit reference, or nil
// when the body was never compiled (a Go sig, an un-armed boru body, a
// refused body). It is the read surface the callback-invocation seam and
// the lang layer consult to choose the VM path over CallBoru. A free
// function in the compiler piece (not a Signature method): the core
// Signature holds the ref as an OPAQUE handle it cannot name.
func CompiledRef(s *core.Signature) *CompiledFnRef {
	if a, ok := s.Impl.(*core.BoruImpl); ok {
		ref, _ := a.Compiled.(*CompiledFnRef)
		return ref
	}
	return nil
}
