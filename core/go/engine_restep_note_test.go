package core

import "testing"

// reStepEmit is the inactive recorder plus the RE-STEP note (NUR124): it
// records what noteFnResultReSteps hands it, by result id.
type reStepEmit struct {
	EmitRecorder
	active bool
	noted  map[string]SrcPos
}

func newReStepEmit() *reStepEmit {
	return &reStepEmit{EmitRecorder: TheInactiveEmit, active: true, noted: map[string]SrcPos{}}
}

func (h *reStepEmit) Active() bool                              { return h.active }
func (h *reStepEmit) NoteFnResultReStep(v Value, resume SrcPos) { h.noted[v.ID] = resume }

// TestNoteFnResultReSteps pins WHICH results the check pass notes as
// re-stepped into a dispatch attempt, and which it leaves alone: a fn-typed
// or fn-admitting gradual carrier with a plain body token after the call is
// noted at that token; a quoted value, a concrete fn (the pass dispatches it
// itself), a strict non-fn carrier, a user fn's (parked) result, a result a
// pending forward collects, and a result nothing plain follows — the group's
// close, a dispatch modifier, an already-placed value, the end of the tape —
// are not.
func TestNoteFnResultReSteps(t *testing.T) {
	at := func(col int) SrcPos { return SrcPos{Row: 1, Col: col} }
	tok := func(name string, col int) Value {
		w := NewWord(name)
		w.SetPos(at(col))
		return w
	}
	native := &Signature{}
	userFn := &Signature{Impl: &BoruImpl{FnFrame: &FnFrameMeta{Name: "f"}}}
	fn := NewCarrier(TFunction)
	dyn := NewDynamicCarrier(TAny)
	strict := NewCarrier(TInteger)
	quoted := NewCarrier(TFunction)
	quoted.Quoted = true
	concrete := NewFunction(FnDefInfo{Name: "g"})

	// run notes results over the tape [get drop] (the call at 0, drop after
	// it) with the given signature and returns the noted ids.
	run := func(es *reStepEmit, sig *Signature, tape []Value, pointer, callEnd int, results ...Value) map[string]SrcPos {
		e := hazardEngine(t, es)
		e.Tape = NewTape(tape, StackHeadroom)
		e.Pointer = pointer
		e.noteFnResultReSteps(sig, callEnd, results)
		return es.noted
	}
	plain := []Value{tok("get", 1), tok("drop", 9)}

	t.Run("a fn-typed or gradual result before a plain token is noted there", func(t *testing.T) {
		noted := run(newReStepEmit(), native, plain, 0, 0, fn, dyn, strict, quoted, concrete)
		if noted[fn.ID] != at(9) || noted[dyn.ID] != at(9) {
			t.Errorf("the fn-typed and the gradual result note the resume token: %v", noted)
		}
		if _, ok := noted[strict.ID]; ok {
			t.Error("a strict non-fn carrier is data on both lanes")
		}
		if _, ok := noted[quoted.ID]; ok {
			t.Error("a quoted value is data on both lanes")
		}
		if _, ok := noted[concrete.ID]; ok {
			t.Error("a concrete fn is dispatched by the pass itself")
		}
	})
	t.Run("a user fn's result is parked, never re-stepped", func(t *testing.T) {
		if noted := run(newReStepEmit(), userFn, plain, 0, 0, fn); len(noted) != 0 {
			t.Errorf("parked: %v", noted)
		}
	})
	t.Run("an inactive recorder and a nil signature note nothing", func(t *testing.T) {
		es := newReStepEmit()
		es.active = false
		if noted := run(es, native, plain, 0, 0, fn); len(noted) != 0 {
			t.Errorf("inactive: %v", noted)
		}
		if noted := run(newReStepEmit(), nil, plain, 0, 0, fn); len(noted) != 0 {
			t.Errorf("nil sig: %v", noted)
		}
	})
	t.Run("nothing after the call: the residual arms own it", func(t *testing.T) {
		if noted := run(newReStepEmit(), native, []Value{tok("get", 1)}, 0, 0, fn); len(noted) != 0 {
			t.Errorf("end of tape: %v", noted)
		}
	})
	t.Run("the group's close, a modifier, a placed value: not plain", func(t *testing.T) {
		mod := NewDispatchMod(DispatchModInfo{Val: true})
		mod.SetPos(at(9))
		placed := NewCarrier(TInteger)
		placed.SetPos(at(9))
		for name, next := range map[string]Value{"close paren": NewCloseParen(), "modifier": mod, "placed carrier": placed} {
			if noted := run(newReStepEmit(), native, []Value{tok("get", 1), next}, 0, 0, fn); len(noted) != 0 {
				t.Errorf("%s: %v", name, noted)
			}
		}
	})
	t.Run("a result a pending forward collects", func(t *testing.T) {
		fwd := NewForward(ForwardInfo{Sig: &Signature{Args: []*Type{TAny}}})
		if noted := run(newReStepEmit(), native, []Value{fwd, tok("get", 3), tok("drop", 9)}, 1, 1, fn); len(noted) != 0 {
			t.Errorf("collected by the forward: %v", noted)
		}
	})
}

// TestPlainReStepToken pins the token classes the interpreter steps after a
// call's results: words, paren groups and written literals are plain; an
// unpositioned value, a carrier, a dispatch modifier and a structural marker
// are not.
func TestPlainReStepToken(t *testing.T) {
	at := SrcPos{Row: 1, Col: 4}
	pos := func(v Value) Value {
		v.SetPos(at)
		return v
	}
	mod := pos(NewDispatchMod(DispatchModInfo{Val: true}))
	dyn := pos(NewDynamicCarrier(TAny))
	yes := []Value{pos(NewWord("drop")), pos(NewParenExpr(nil)), pos(NewInteger(7))}
	no := []Value{NewInteger(7), pos(NewCarrier(TInteger)), dyn, mod, pos(NewEnd()), pos(NewCloseParen())}
	for _, v := range yes {
		if !plainReStepToken(v) {
			t.Errorf("plain: %v", v)
		}
	}
	for _, v := range no {
		if plainReStepToken(v) {
			t.Errorf("not plain: %v", v)
		}
	}
}

// TestFnValueDispatchesAtPointerExport pins the exported predicate against
// the main loop's own: an unquoted fn value dispatches, a quoted one and a
// carrier do not; an unquoted compiled closure dispatches too (the bridge
// at execFnDefLiteral), a quoted one does not.
func TestFnValueDispatchesAtPointerExport(t *testing.T) {
	fn := NewFunction(FnDefInfo{Name: "g"})
	quoted := fn
	quoted.Quoted = true
	closure := Value{Parent: TFunction, Data: ClosurePayload{}}
	quotedClosure := closure
	quotedClosure.Quoted = true
	for _, c := range []struct {
		v    Value
		want bool
	}{{fn, true}, {quoted, false}, {NewCarrier(TFunction), false}, {NewInteger(1), false}, {closure, true}, {quotedClosure, false}} {
		if got := FnValueDispatchesAtPointer(c.v); got != c.want {
			t.Errorf("%v: %v", c.v, got)
		}
		if _, cl := c.v.Data.(ClosurePayload); !cl && FnValueDispatchesAtPointer(c.v) != fnValueDispatchesAtPointer(c.v) {
			t.Errorf("%v: the exported predicate must agree with the main loop's", c.v)
		}
	}
}
