package core

import "testing"

// TestForwardFnCallIsTheBoundary pins the recovery's reading of a forward
// word (the NUR078 × #509 merge): a bare name bound to a function — a
// FnDefInfo, a function carrier, a dynamic binding at a function-typed slot
// — calls where it is written, so it is never an operand; a `/v` read, a
// value binding, an unbound name and a literal are not calls.
func TestForwardFnCallIsTheBoundary(t *testing.T) {
	fnv := NewFunction(FnDefInfo{Name: "g", Signatures: []Signature{{Impl: Boru([]Value{NewInteger(1)})}}})
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	r := covRegistry(t, func(r *Registry) {
		r.Defs.Push("g", fnv)
		r.Defs.Push("fc", NewCarrier(TFunction))
		r.Defs.Push("x", NewInteger(1))
		r.Defs.Push("d", dyn)
	})
	e := NewTop(r)
	e.Tape = NewTape([]Value{
		NewWord("g"), NewWordRef("g"), NewWord("fc"), NewWord("x"),
		NewWord("d"), NewWord("nope"), NewInteger(5),
	}, StackHeadroom)
	fnSig := &Signature{Params: []FnParam{{Name: "k", Type: TFunction}}}
	intSig := &Signature{Params: []FnParam{{Name: "k", Type: TInteger}}}
	for _, c := range []struct {
		at   int
		sig  *Signature
		want bool
	}{
		{0, intSig, true},  // a fn binding calls at any slot
		{1, fnSig, false},  // `g/v` arrives as the value
		{2, intSig, true},  // a function carrier stands for one
		{3, intSig, false}, // a value binding is an operand
		{4, fnSig, true},   // a dynamic binding fills a Function slot only by calling
		{4, intSig, false}, // …and at any other slot is an operand
		{5, fnSig, false},  // unbound: not this rule's
		{6, fnSig, false},  // a literal
	} {
		if got := e.forwardFnCall(c.at, 0, c.sig); got != c.want {
			t.Errorf("forwardFnCall(%d) = %v, want %v", c.at, got, c.want)
		}
	}
}

// TestRecoveryRefusesACallingForwardWord pins the guard itself: a recovered
// user call whose forward window holds a bare fn-bound name declines (the
// interpreter's dispatch stops at the name and raises), and the same window
// over a value binding still records.
func TestRecoveryRefusesACallingForwardWord(t *testing.T) {
	sole := Signature{
		Params:     []FnParam{{Name: "k", Type: TAny}},
		Impl:       Boru([]Value{NewInteger(1)}),
		ReturnsFn:  func(args []Value, r *Registry) []Value { return []Value{NewInteger(9)} },
		BarrierPos: BarrierAllForward,
	}
	fn := &FnDefInfo{Name: "h", Signatures: []Signature{{Fallback: true}, sole}}
	fnv := NewFunction(FnDefInfo{Name: "g", Signatures: []Signature{{Impl: Boru([]Value{NewInteger(1)})}}})
	r := covRegistry(t, func(r *Registry) {
		r.Defs.Push("g", fnv)
		r.Defs.Push("x", NewInteger(1))
	})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("h"), NewWord("g"), NewWord("x")}, StackHeadroom)
	e.Pointer = 0
	spliced := 0
	prev := CheckBraid.SpliceCheckResults
	CheckBraid.SpliceCheckResults = func(e *Engine, positions []int, results []Value) { spliced++ }
	t.Cleanup(func() { CheckBraid.SpliceCheckResults = prev })
	if e.TryRecordRecoveredUserFn(&fn.Signatures[1], fn, []Value{fnv}, 0, []int{1}) {
		t.Error("a forward fn-bound name calls: the recovery declines")
	}
	if !e.TryRecordRecoveredUserFn(&fn.Signatures[1], fn, []Value{NewInteger(1)}, 0, []int{2}) || spliced != 1 {
		t.Errorf("a forward value binding recovers (spliced %d)", spliced)
	}
}

// TestFnHasBoruSig pins the user-fn test the dispatch's window offer keys
// on: a boru body anywhere among the signatures.
func TestFnHasBoruSig(t *testing.T) {
	native := &FnDefInfo{Signatures: []Signature{{Impl: Go(nil)}}}
	user := &FnDefInfo{Signatures: []Signature{{Fallback: true}, {Impl: Boru([]Value{NewInteger(1)})}}}
	if fnHasBoruSig(native) || !fnHasBoruSig(user) {
		t.Fatalf("native %v, user %v", fnHasBoruSig(native), fnHasBoruSig(user))
	}
}

// callWindowEmit is the inactive recorder plus the call-window offer: it
// keeps what noteCallWindow hands it.
type callWindowEmit struct {
	EmitRecorder
	word     string
	window   []Value
	deferred bool
	restep   bool
	calls    int
}

func (c *callWindowEmit) Active() bool { return true }
func (c *callWindowEmit) NoteCallWindow(word string, _ SrcPos, window []Value, deferred, restep bool) {
	c.word, c.window, c.deferred, c.restep = word, window, deferred, restep
	c.calls++
}

// TestNoteCallWindowOffer pins what a user fn's dispatch offers the
// recorder (NUR234): the attempted window over the tape — present but EMPTY
// when nothing is written and nothing sits beneath (a nil window means "no
// offer", so the empty one must stay non-nil) — no window for a speculative
// plan, and the deferred flag when a matched position lies past the word (a
// forward collection re-steps the call).
func TestNoteCallWindowOffer(t *testing.T) {
	fn := &FnDefInfo{Name: "h", Signatures: []Signature{{Fallback: true}, {
		Params: []FnParam{{Name: "k", Type: TInteger}},
		Impl:   Boru([]Value{NewInteger(1)}),
	}}}
	sig := &fn.Signatures[1]
	w := WordInfo{Name: "h", ForceStack: true}
	run := func(tape []Value, pointer int, sig *Signature, positions []int, specAt int) *callWindowEmit {
		es := &callWindowEmit{EmitRecorder: TheInactiveEmit}
		e := hazardEngine(t, es)
		e.Tape = NewTape(tape, StackHeadroom)
		e.Pointer = pointer
		e.noteCallWindow(w, fn, sig, positions, specAt, SrcPos{})
		return es
	}

	es := run([]Value{NewWord("h")}, 0, sig, nil, -1)
	if es.calls != 1 || es.word != "h" || es.window == nil || len(es.window) != 0 || es.deferred || !es.restep {
		t.Errorf("an empty attempted window is offered non-nil: %+v", es)
	}
	es = run([]Value{NewWord("h"), NewInteger(3)}, 0, sig, []int{1}, -1)
	if !es.deferred || len(es.window) != 1 || !ValuesEqual(es.window[0], NewInteger(3)) {
		t.Errorf("a position past the word defers the offer over the written run: %+v", es)
	}
	es = run([]Value{NewInteger(2), NewWord("h")}, 1, sig, []int{0}, 0)
	if es.deferred || es.window != nil {
		t.Errorf("a speculative plan offers no window, a stack position does not defer: %+v", es)
	}
	es = run([]Value{NewWord("h"), NewInteger(3)}, 0, nil, []int{1}, -1)
	if es.deferred {
		t.Errorf("no matched signature never defers: %+v", es)
	}
}
