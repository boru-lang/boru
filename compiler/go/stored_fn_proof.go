package compiler

import (
	core "github.com/boru-lang/boru/core/go"
)

// The strict store-fn slot's payload proof (2026-09-26, the sweep's `behave`
// × container cell).
//
// A CompileFnHandlerStrict word (behave, the service `add` family, the
// fn-util combinators) VALIDATES its fn operand as an interpreter FnDefInfo:
// behave installs the fn's raw body TOKENS on the type's behaviour wrapper
// and reads its signature's declared params and return; fn-util reshapes the
// signature. A compiled closure (the ClosurePayload an OpPushClosure mints —
// a capturing `fn` / `=>` a factory hands back) carries neither: its bridged
// shape (ClosureAsFnDef) is a Go handler over the unit's params with no body
// and no return. Such a handler therefore raises over a closure where the
// interpreter, holding the source fn, succeeds — measured before this proof:
// `def mk fn [[k:String][Function][(fn [[t:Temp][String][k]])]]  behave
// canon/q (mk 'K')` answered `behave canon: fn arg has invalid payload
// (core.ClosurePayload)` compiled and the canon string interpreted, and
// `FnUtil.compose (mk 1) (mk 2)` over a capturing `=>` factory raised
// type_error for the interpreter's 8. A CONCRETE capturing fn at the slot
// was already declined (RecordCallOperands' strict arm); a CARRIER operand —
// a factory's declared `[Function]` result, a container member read — rode
// straight into the call.
//
// So the slot admits an operand only when the recorder can PROVE it arrives
// as an FnDefInfo: the value is a baked const fn, or the single result of an
// event whose run-time value is one — a compiled user call whose unit
// returns a baked const fn (a capture-free literal the factory hands back,
// stampFnConst's bake; producedConstLambda's shape), or a member read over
// an immutable const container whose member is a concrete fn (a const Map or
// List is never mutated and never holds a closure: the check pass mints no
// ClosurePayload, so nothing interned does). Everything else — a closure, a
// fn param, a dynamic-scope read, a flex member — declines, and the program
// is a named compile failure rather than a wrong answer.

// strictFnOperandProven reports whether the operand op, resolved for the
// value v, provably holds an interpreter FnDefInfo at run time.
func (es *EmitState) strictFnOperandProven(v core.Value, op EmitOperand) bool {
	if _, ok := es.provenFnDef(v, op); ok {
		return true
	}
	switch op.kind {
	case opEvent:
		return op.resIdx == 0 && es.strictNativeFnResult(op.idx)
	case opLocal:
		pr, ok := es.producedBy[v.ID]
		return ok && pr.idx == 0 && es.strictNativeFnResult(pr.seq)
	}
	return false
}

// strictNativeFnResult reports whether event seq is a strict fn-handler
// native (CompileFnHandlerStrict) whose one declared result is a Function —
// the fn-util combinators (`FnUtil.compose`, `flip`, `partial`, …) and the
// dispatch modifiers' FnDefInfo forms (`usurp`, `force-arity`, …). Such a
// native VALIDATES every Function operand as an FnDefInfo (and each of those
// operands passed this same gate when its own call was recorded) and returns
// a Go-built FnDefInfo wrapper it mints itself, never a ClosurePayload — so
// its result arrives as an interpreter fn value on both lanes (Codex P2 on
// PR #511: `FnUtil.flip (FnUtil.compose inc/v dbl/v)`). The wrapper's body is
// not known here, so a store-fn word reading it still arms DynEnv
// (storedFnNeedsDynEnv asks provenFnDef, which declines it).
func (es *EmitState) strictNativeFnResult(seq int) bool {
	ev := es.eventInAnyFrame(seq)
	if ev == nil || ev.kind != evCall {
		return false
	}
	c := &ev.call
	if c.sig == nil || !c.sig.CompileEffect.Has(core.CompileFnHandlerStrict) || c.nout != 1 || c.dynApply > 0 || c.dynMethod != nil || c.live {
		return false
	}
	rs := c.sig.Returns
	return len(rs) == 1 && rs[0] != nil && rs[0].Equal(core.TFunction)
}

// provenFnDef is strictFnOperandProven with the proven fn value itself: the
// FnDefInfo the operand holds at run time, known here because every proof
// ends at a const.
func (es *EmitState) provenFnDef(v core.Value, op EmitOperand) (core.FnDefInfo, bool) {
	switch op.kind {
	case opConst:
		return es.constFnDef(op.idx)
	case opEvent:
		if op.resIdx != 0 {
			return core.FnDefInfo{}, false
		}
		return es.seqFnDef(op.idx)
	case opLocal:
		// A promoted value-def (`def c (mk)` then `c/v`) reads its frame
		// slot, which holds exactly the producing event's result; a param or
		// a loop iterator has no producer and stays unproven.
		if pr, ok := es.producedBy[v.ID]; ok && pr.idx == 0 {
			return es.seqFnDef(pr.seq)
		}
	}
	return core.FnDefInfo{}, false
}

// constFnDef is const idx's interpreter fn value, when it is one.
func (es *EmitState) constFnDef(idx int) (core.FnDefInfo, bool) {
	if idx < 0 || idx >= len(es.consts) {
		return core.FnDefInfo{}, false
	}
	fd, isFn := es.consts[idx].Data.(core.FnDefInfo)
	return fd, isFn
}

// seqFnDef is event seq's single result when it is an interpreter fn value
// at run time: a monomorphic user call whose unit returns a baked const fn,
// or a member read over all-const operands whose member is a concrete fn.
func (es *EmitState) seqFnDef(seq int) (core.FnDefInfo, bool) {
	ev := es.eventInAnyFrame(seq)
	if ev == nil {
		return core.FnDefInfo{}, false
	}
	switch ev.kind {
	case evCallUser:
		if ev.uc.poly != nil || ev.uc.nout != 1 {
			return core.FnDefInfo{}, false
		}
		if out, ok := es.producerReturnedOutOpSeq(seq); ok && out.kind == opConst {
			return es.constFnDef(out.idx)
		}
	case evCall:
		return es.constMemberFn(&ev.call)
	}
	return core.FnDefInfo{}, false
}

// constMemberFn is the member a plain get-family read (no apply, method
// landing or live lookup riding the event) over all-const operands reads,
// when that member is a concrete fn value.
func (es *EmitState) constMemberFn(c *emitCall) (core.FnDefInfo, bool) {
	if !isGetFamilyWord(c.word) || c.nout != 1 || c.dynApply > 0 || c.dynMethod != nil || c.live {
		return core.FnDefInfo{}, false
	}
	vals := make([]core.Value, len(c.ops))
	for i, op := range c.ops {
		if op.kind != opConst || op.idx < 0 || op.idx >= len(es.consts) {
			return core.FnDefInfo{}, false
		}
		vals[i] = es.consts[op.idx]
	}
	member, ok := readFnMemberValue(vals)
	if !ok || !core.IsConcrete(member) {
		return core.FnDefInfo{}, false
	}
	fd, isFn := member.Data.(core.FnDefInfo)
	return fd, isFn
}

// storedFnNeedsDynEnv reports whether recording a store-fn dispatch whose
// word runs the stored body LATER against the registry (a CompileDynBody
// store-fn word — behave) must arm the DynEnv mirror: true unless every
// Function operand is a proven fn value (provenFnDef) whose bodies name
// nothing (valueRefsName — a name the deferred run would resolve in the
// interpreter's dynamic scope, which only DynEnv reproduces). A body of pure
// data (`['T']`) resolves nothing, so its program keeps its ordinary
// lowering — arming the mirror there only declines unrelated defs of
// unknown provenance (the `behave` × factory seed's do-body variants).
func (es *EmitState) storedFnNeedsDynEnv(sig *core.Signature, args []core.Value, ops []EmitOperand) bool {
	for i := range args {
		if !strictFnSlot(sig, i) {
			continue
		}
		fd, ok := es.provenFnDef(args[i], ops[i])
		if !ok {
			return true
		}
		for si := range fd.Signatures {
			for _, tok := range fd.Signatures[si].Body() {
				if valueRefsName(tok) {
					return true
				}
			}
		}
	}
	return false
}
