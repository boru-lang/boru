package core

import "testing"

// Core-side coverage for the 2026-09-28 defect pass (NUR335, NUR337): the
// bare-call context the dispatch publishes (CheckState.BareCallPos) and the
// collapse's bind-time evaluation of a trailing window's pending containers.
// Both were reached only through lang's suite; these pin them by core's own
// (make cover-gate-core).

// TestBareCallContextFrames walks every arm of Engine.bareCallContext: only
// the top engine's own stream, outside every nested body and fn body
// analysis, can be bare; beneath the first operand only an open paren or the
// tape's start; after the call only a close paren, a statement end or the
// tape's end.
func TestBareCallContextFrames(t *testing.T) {
	r := covRegistry(t, nil)
	w := NewWord("cadd")
	tape := func(vs ...Value) *Tape { return NewTape(vs, StackHeadroom) }
	for _, c := range []struct {
		name    string
		vs      []Value
		ptr     int
		ops     []int
		callEnd int
		want    bool
	}{
		// POSITIVE: nothing beneath, and each sealing follower.
		{"tape start, tape end", []Value{NewInteger(1), w}, 1, []int{0}, 1, true},
		{"open paren beneath, close paren after", []Value{NewOpenParen(), NewInteger(1), w, NewCloseParen()}, 2, []int{1}, 2, true},
		{"open paren beneath, statement end after", []Value{NewOpenParen(), NewInteger(1), w, NewEnd()}, 2, []int{1}, 2, true},
		{"open paren beneath, tape end", []Value{NewOpenParen(), NewInteger(1), w}, 2, []int{1}, 2, true},
		// A forward operand: the word itself is the first token.
		{"forward operand under an open paren", []Value{NewOpenParen(), w, NewInteger(1), NewCloseParen()}, 1, []int{2}, 2, true},
		// NEGATIVE: a value beneath the first operand, a value after the call.
		{"value beneath the first operand", []Value{NewInteger(9), NewInteger(1), w}, 2, []int{1}, 2, false},
		{"value beneath a forward call", []Value{NewInteger(9), w, NewInteger(1)}, 1, []int{2}, 2, false},
		{"value after the call", []Value{NewOpenParen(), NewInteger(1), w, NewInteger(5)}, 2, []int{1}, 2, false},
		{"word after the call", []Value{NewInteger(1), w, NewWord("cneg")}, 1, []int{0}, 1, false},
	} {
		e := NewTop(r)
		e.Tape = tape(c.vs...)
		e.Pointer = c.ptr
		if got := e.bareCallContext(c.ops, c.callEnd); got != c.want {
			t.Errorf("%s: bareCallContext = %v, want %v", c.name, got, c.want)
		}
	}

	// NEGATIVE: the same bare shape is never bare off the top engine, inside
	// a nested body or inside a fn body analysis.
	shape := []Value{NewInteger(1), w}
	for _, c := range []struct {
		name  string
		setup func(e *Engine)
	}{
		{"not the top engine", func(e *Engine) { e.IsTop = false }},
		{"nested body", func(e *Engine) { e.Registry.Check.NestedBodyDepth = 1 }},
		{"fn body analysis", func(e *Engine) { e.Registry.Check.FnBodyDepth = 1 }},
	} {
		e := NewTop(r)
		e.Tape = tape(shape...)
		e.Pointer = 1
		if !e.bareCallContext([]int{0}, 1) {
			t.Fatalf("%s: the control shape must be bare", c.name)
		}
		c.setup(e)
		if e.bareCallContext([]int{0}, 1) {
			t.Errorf("%s: must not be bare", c.name)
		}
		r.Check.NestedBodyDepth, r.Check.FnBodyDepth = 0, 0
	}

	// NEGATIVE: the top engine recording inside a unit (no longer at the top
	// event frame) is not bare either.
	e := NewTop(r)
	e.Tape = tape(shape...)
	e.Pointer = 1
	prevEmit := r.Check.Emit
	r.Check.Emit = &frameRecorder{top: false}
	if e.bareCallContext([]int{0}, 1) {
		t.Error("a top engine recording below the top event frame must not be bare")
	}

	// POSITIVE (NUR342): a fn frame's own engine (FrameRoot) is bare over
	// the same shape, off the top engine, inside the analyses, below the top
	// event frame — the run seals the frame at its bottom — and NEGATIVE
	// with a value beneath or after the call there as anywhere.
	r.Check.NestedBodyDepth, r.Check.FnBodyDepth = 1, 1
	for _, c := range []struct {
		name    string
		vs      []Value
		ptr     int
		ops     []int
		callEnd int
		want    bool
	}{
		{"frame bottom, frame end", shape, 1, []int{0}, 1, true},
		{"an unnamed param beneath", []Value{NewInteger(9), NewInteger(1), w}, 2, []int{1}, 2, false},
		{"a value after the call", []Value{NewInteger(1), w, NewInteger(5)}, 1, []int{0}, 1, false},
	} {
		fe := NewTop(r)
		fe.IsTop = false
		fe.FrameRoot = true
		fe.Tape = tape(c.vs...)
		fe.Pointer = c.ptr
		if got := fe.bareCallContext(c.ops, c.callEnd); got != c.want {
			t.Errorf("frame root, %s: bareCallContext = %v, want %v", c.name, got, c.want)
		}
	}
	r.Check.NestedBodyDepth, r.Check.FnBodyDepth = 0, 0
	r.Check.Emit = prevEmit
}

// frameRecorder answers TopFrameOnly as told (a recorder inside a closure
// unit answers false).
type frameRecorder struct {
	inactiveEmit
	top bool
}

func (f *frameRecorder) TopFrameOnly() bool { return f.top }

// TestEvalTrailingWindowContainersArms drives the collapse's bind-time
// evaluation of a paren window's pending containers (NUR337) through every
// arm: a window that fits the lead's one signature evaluates a list or a map
// the bind's way (and a raising evaluation replaces nothing); a window that
// may park folds only a pure top-frame constant, never one that reads a
// carrier, runs in a closure unit, fails to fold, or folds to a shared
// mutable or a capturing fn.
func TestEvalTrailingWindowContainersArms(t *testing.T) {
	r := covRegistry(t, nil)
	at := &SrcPos{Row: 4, Col: 2, Src: "["}
	pendingList := func(items ...Value) Value {
		v := NewList(items)
		v.Eval = true
		v.pos = at
		return v
	}
	sum := func() Value { return pendingList(NewInteger(1), NewWord("cadd"), NewInteger(2)) }
	pendingMap := func() Value {
		m := NewOrderedMap()
		m.Set("a", NewParenExpr([]Value{NewInteger(1), NewWord("cadd"), NewInteger(2)}))
		v := NewMap(m)
		v.Eval = true
		v.pos = at
		return v
	}
	lead := func(params ...*Type) Value {
		ps := make([]FnParam, len(params))
		for i, p := range params {
			ps[i] = FnParam{Type: p}
		}
		return NewFunction(FnDefInfo{Anonymous: true, Signatures: []FnSig{{Params: ps, Returns: []*Type{TAny}, BarrierPos: len(ps)}}})
	}
	// run collapses the window `( arg lead )` and hands back the argument
	// slot as the collapse left it.
	run := func(e *Engine, lead, arg Value) Value {
		e.Tape = NewTape([]Value{NewOpenParen(), arg, lead, NewCloseParen()}, StackHeadroom)
		e.evalTrailingWindowContainers(lead, 0, 3, 2)
		return e.Tape.At(1)
	}
	untouched := func(name string, got Value) {
		t.Helper()
		if !got.Eval || !IsPendingActiveContainer(got) {
			t.Errorf("%s: the literal must stay pending, got %v (eval=%v)", name, got, got.Eval)
		}
	}
	evaluated := func(name string, got Value, want string) {
		t.Helper()
		if got.Eval || got.String() != want || got.pos != at {
			t.Errorf("%s: got %v (eval=%v pos=%v), want %s evaluated at the literal's position", name, got, got.Eval, got.pos, want)
		}
	}

	// A window with no argument for the signature, or fewer literals than
	// its params, is left alone.
	untouched("zero params", run(NewTop(r), lead(), sum()))
	untouched("more params than literals", run(NewTop(r), lead(TList, TList), sum()))

	// FITS: a list and a map evaluate as the bind would.
	if !trailingWindowFits(lead(TList), []Value{sum()}) || !trailingWindowFits(lead(TMap), []Value{pendingMap()}) {
		t.Fatal("a pending list/map provably fits a List/Map param")
	}
	evaluated("fitting list", run(NewTop(r), lead(TList), sum()), NewList([]Value{NewInteger(3)}).String())
	m3 := NewOrderedMap()
	m3.Set("a", NewInteger(3))
	evaluated("fitting map", run(NewTop(r), lead(TMap), pendingMap()), NewMap(m3).String())
	// A bind-time evaluation that raises replaces nothing.
	untouched("raising list", run(NewTop(r), lead(TList), pendingList(NewInteger(1), NewWord("cadd"), NewString("s"))))

	// MAY PARK (the lead is quoted, so the collapse does not apply it).
	parks := func() Value { v := lead(TList); v.Quoted = true; return v }
	if trailingWindowFits(parks(), []Value{sum()}) {
		t.Fatal("a quoted lead proves nothing")
	}
	// Inside a closure unit nothing folds.
	inner := NewTop(r)
	prevEmit := r.Check.Emit
	r.Check.Emit = &frameRecorder{top: false}
	untouched("closure unit", run(inner, parks(), sum()))
	r.Check.Emit = prevEmit

	// A literal that reads a carrier never folds.
	prevRefs := CheckBraid.ExprRefsCarrier
	CheckBraid.ExprRefsCarrier = func(*Engine, []Value) bool { return true }
	untouched("carrier read", run(NewTop(r), parks(), sum()))
	CheckBraid.ExprRefsCarrier = prevRefs

	// The pure constant fold: taken only when it folds to a plain value.
	prevOnce := CheckBraid.ConcreteEvalOnce
	defer func() { CheckBraid.ConcreteEvalOnce = prevOnce }()
	fold := func(v Value, ok bool) {
		CheckBraid.ConcreteEvalOnce = func(*Engine, []Value) (Value, bool) { return v, ok }
	}
	three := NewList([]Value{NewInteger(3)})
	fold(three, true)
	evaluated("constant fold", run(NewTop(r), parks(), sum()), three.String())
	fold(Value{}, false)
	untouched("no fold", run(NewTop(r), parks(), sum()))
	fold(NewFlexList([]Value{NewInteger(3)}), true)
	untouched("shared mutable fold", run(NewTop(r), parks(), sum()))
	fold(NewFunction(FnDefInfo{Anonymous: true, Captured: []CapturedBinding{{}}}), true)
	untouched("capturing fn fold", run(NewTop(r), parks(), sum()))
}
