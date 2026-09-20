package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The Apply kernel's VM half: a dynamic apply whose callee turns out to carry a
// compiled unit becomes a runtime-resolved CALL_USER, rather than an island.
//
// WHY A FRAME PUSH AND NOT A NESTED RUN. enterBodyUnit documents four ways the
// VM reaches a body, and brackets three of them — RunUnit, runUnitNested,
// invokeClosureOn — with a per-body context frame. The fourth, OpCallUser and
// friends, is explicitly NOT there: a call is a frame push inside the run loop,
// and the interpreter's own fn call (execFnDefLiteral / CallBoru) brackets no
// body frame either.
//
// A fn APPLICATION is the fourth kind. An earlier version of this work routed
// it through the callback seam and therefore through enterBodyUnit, and the
// extra context frame showed up exactly where that funnel's comment warns it
// would:
//
//	TestContextBoundaryDifferential/paren-grouped_map-slot_lambda_method
//	  compiled [false] != interpreted [true]
//
// A `context set` inside the applied body escaped on the interpreter and not on
// the VM — a silent wrong answer, not an error. So this half models
// OpCallUserPoly instead, which is the existing precedent for entering a unit
// the run loop only learns at RUN time: match, pop the args into frame locals,
// re-check the param contract, push a frame. No re-entry, no context bracket.

// dynEnter is a dynamic apply's "the callee compiled — enter its unit" outcome.
// The helpers cannot push the frame themselves (frames, locals, curUnit and pc
// are the run loop's), so they hand this back and the loop does it.
type dynEnter struct {
	unit   int
	locals []core.Value
	// retFn is the RETURN CONTRACT the entered frame's RET must apply, in
	// place of the entered unit's own (which for a stamped fn value has none).
	// Built by applyRetContract; never nil for an entry this file produces.
	retFn *compiler.CompiledFn
	// allForward reports that the matched signature collects every argument
	// FORWARD — read by the replay window, which may only enter over a
	// non-empty resolved prefix when the callee cannot reach into it.
	allForward bool
}

// allForwardSig reports whether every parameter of a matched signature is
// forward-eligible: the barrier sits at or past the last param, so the call
// collects all of its arguments from the tokens written after it and none from
// the stack below.
//
// -1 is the UNSET sentinel and means all-forward (upsertFnDef resolves it to
// len(Params)); an explicit 0 is all-stack, a different call, and must not be
// admitted. The documented Go-zero ambiguity on BarrierPos does not reach here
// — every sig this file sees carries a compiled boru unit, and the boru source
// path emits the sentinel.
func allForwardSig(sig *core.Signature) bool {
	return sig.BarrierPos == -1 || sig.BarrierPos >= len(sig.Params)
}

// applyRetContract is the return contract an APPLICATION of a fn value has to
// carry into the frame it enters.
//
// A stamped stored-fn unit compiles COUNT-AGNOSTIC — compileStoredFnUnit goes
// through compileClosureBody with bodyOut 0, whose `declared = nil` leaves
// CompiledFn.Returns empty — so its RET enforces nothing. That is right for the
// unit, whose other consumer (the callback seam) wants the raw residual, and
// WRONG for an application: the fn VALUE carries a declared return contract,
// and the island path applies it, because the interpreter's __RC runs inside
// the CallBoru the island's nested Run reaches. Measured before this existed:
//
//	def bad fn [[n:Any][Integer][n]] end
//	def mk  fn [[][Function][bad/v]] end
//	((mk) 'str')
//	  interpreted  [boru/type_error] bad: return value 1: expected Integer, got ProperString
//	  compiled     'str'
//
// A silent wrong answer rather than an error — the class this project ranks
// strictly above a compile failure. The declared return is only checkable at RUN time
// here (the body returns its `n:Any` param, so the check pass cannot rule the
// String out statically), which is exactly why the runtime contract has to
// ride along.
//
// Only the DECLARED contract comes from the sig. The frame-layout fields
// (NUnnamed's trim allowance, RetReplay, Reg) come from the entered unit,
// because they describe how that unit's own residual sits at RET.
func applyRetContract(unit *compiler.CompiledFn, name string, sig *core.Signature) *compiler.CompiledFn {
	ov := *unit
	ov.Name = name
	ov.Returns = sig.Returns
	ov.ReturnPatterns = sig.ReturnPatterns
	ov.Decl = sig.Decl
	return &ov
}

// dynApplyEnter reports how to enter a dynamically-applied fn VALUE on the VM,
// or nil when the VM cannot take it and the caller's island path stands.
//
// The match must be the one the ISLAND would have made, or this changes answers
// rather than engines: core.MatchFnSig is the interpreter's own selection rule,
// and the island's nested Run reaches the same overload through it. A nil match
// declines here and the island decides, unchanged.
//
// Only an IN-PROGRAM ref is taken. A detached ref (its own standalone Program)
// cannot be entered as a frame of this one — its unit index means nothing
// against p.Fns — and hosting it nested would reintroduce the body bracket this
// file exists to avoid. Those keep the island.
func (vc *vmContext) dynApplyEnter(fnVal core.Value, args []core.Value) *dynEnter {
	fd, isFn := fnVal.Data.(core.FnDefInfo)
	if !isFn {
		return nil
	}
	// A QUOTED fn is DATA, not a callee. The island got this for free — the
	// interpreter's tape leaves a quoted value inert — and a direct frame push
	// does not, so it has to be said here. Measured as a silent wrong answer:
	//
	//	def mk fn [[] [Function] [quote (fn [[b:Integer] [Integer] [b add 1]])]]
	//	((mk) 2)
	//	  interpreted [fn (Integer) 2]     the quoted fn and its neighbour, as data
	//	  applied     [3]                  the frame push called it anyway
	//
	// IsAppliableFn alone does not exclude a quoted value (callDynApplyTop
	// tests `|| fnVal.Quoted` separately for the same reason).
	if fnVal.Quoted {
		return nil
	}
	sig := core.MatchFnSig(fnVal, args)
	if sig == nil {
		return nil
	}
	ref := compiler.CompiledRef(sig)
	if ref == nil || ref.Prog != vc.p || ref.Unit < 0 || ref.Unit >= len(vc.p.Fns) {
		return nil
	}
	fn := &vc.p.Fns[ref.Unit]
	// The unit's own params must be exactly what the match produced. A shape
	// mismatch is a compile/run drift, and entering on one would bind the
	// wrong locals silently — decline and let the island answer.
	if fn.NParams != len(args) || fn.NCaptures != 0 {
		return nil
	}
	locals := make([]core.Value, fn.NLocals)
	copy(locals, args)
	// Delivery discipline, identical to OpCallUserPoly's: strip the ascribed
	// view (the match above already consumed it) and quote list params so body
	// references are data — the compiled mirror of the interpreter's binding
	// rule (core_helpers.go).
	for i := 0; i < fn.NParams && i < len(locals); i++ {
		locals[i] = core.StripAscribed(locals[i])
		if locals[i].Parent.Equal(core.TList) && !locals[i].Quoted {
			locals[i].Quoted = true
		}
	}
	return &dynEnter{unit: ref.Unit, locals: locals, retFn: applyRetContract(fn, fd.Name, sig), allForward: allForwardSig(sig)}
}
