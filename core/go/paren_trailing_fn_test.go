package core

import "testing"

// paren_trailing_fn_test.go pins the collapse-side helpers of NUR184 (the
// paren's trailing fn value forward-collects past the close):
// trailingFnCollectsPastClose, parenFeedsPendingForward, markForwardLeftover
// and the bare-read mark noteWordRead leaves for the collapse
// (CheckState.WordReadFnIDs).

func ptfFn(t *testing.T, params ...*Type) Value {
	t.Helper()
	fp := make([]FnParam, len(params))
	for i, p := range params {
		fp[i] = FnParam{Type: p}
	}
	sig := Signature{Params: fp, BarrierPos: BarrierAllForward}
	NormalizeSig(&sig)
	v := NewFunction(FnDefInfo{Name: "g", Signatures: []Signature{sig}})
	v.ID = "fn-" + v.ID
	return v
}

func TestTrailingFnCollectsPastClose(t *testing.T) {
	carrier := NewCarrier(TFunction)
	carrier.ID = "fnc-1"
	intFn := ptfFn(t, TInteger)
	strFn := ptfFn(t, TString)
	nullary := ptfFn(t)
	noSigs := NewFunction(FnDefInfo{Name: "h"})
	cases := []struct {
		name string
		last Value
		tail []Value // the tokens after the close paren
		want bool
		why  string
	}{
		{"nothing after the close", carrier, nil, false, "the value falls to the stack — the values inside the paren"},
		{"a word after the close", carrier, []Value{NewWord("mul")}, false, "`(2 (mk 1)) mul 10` is 30: a word is not collectable"},
		{"a close paren after the close", carrier, []Value{NewCloseParen()}, false, "an enclosing group's boundary"},
		{"an end after the close", carrier, []Value{NewEnd()}, false, "a statement boundary"},
		{"a modifier after the close", carrier, []Value{NewDispatchMod(DispatchModInfo{})}, false, "`/v` leaves the value inert"},
		{"an open paren after the close", carrier, []Value{NewOpenParen()}, true, "the group's result arrives at the value's pending forward"},
		{"a carrier over a literal", carrier, []Value{NewInteger(10)}, true, "the runtime parameters are unknown: the literal may be collected"},
		{"a concrete fn over a literal its param takes", intFn, []Value{NewInteger(10)}, true, "`(2 inc2/v) 10` is `[2 12]`"},
		{"a concrete fn over a literal its param rejects", intFn, []Value{NewString("s")}, false, "`(2 (mk 1)) \"s\"` is `[3 s]`"},
		{"a String param over a String", strFn, []Value{NewString("s")}, true, "the type test is the signature's own"},
		{"a nullary fn over a literal", nullary, []Value{NewInteger(10)}, false, "no position to collect into"},
		{"a fn with no signatures", noSigs, []Value{NewInteger(10)}, true, "nothing rules the collection out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tape := []Value{NewOpenParen(), NewInteger(2), tc.last, NewCloseParen()}
			tape = append(tape, tc.tail...)
			e := &Engine{Tape: NewTape(tape, 8)}
			if got := e.trailingFnCollectsPastClose(tc.last, 3); got != tc.want {
				t.Errorf("collects = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}
}

func TestParenFeedsPendingForward(t *testing.T) {
	sig := &Signature{Args: []*Type{TAny, TAny}, BarrierPos: 2}
	collecting := NewForward(ForwardInfo{FuncName: "mul", ExpectedArgs: 2, CollectedArgs: 1, Sig: sig})
	complete := NewForward(ForwardInfo{FuncName: "mul", ExpectedArgs: 2, CollectedArgs: 2, Sig: sig})
	group := []Value{NewOpenParen(), NewInteger(2), NewCloseParen()}
	cases := []struct {
		name   string
		before []Value
		want   bool
		why    string
	}{
		{"no forward on the tape", []Value{NewInteger(10)}, false, "the fast exit: nothing to feed"},
		{"a collecting forward directly before", []Value{NewInteger(10), collecting}, true, "the group is that word's argument (the arrival path)"},
		{"a completed forward before", []Value{NewInteger(10), complete}, false, "nothing left to collect: the survivors are the main loop's"},
		{"an open paren between", []Value{collecting, NewOpenParen(), NewInteger(1)}, false, "a nested group is the enclosing group's, re-stepped by its own loop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tape := append(append([]Value(nil), tc.before...), group...)
			e := &Engine{Tape: NewTape(tape, 8)}
			if got := e.parenFeedsPendingForward(len(tc.before)); got != tc.want {
				t.Errorf("feeds = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}
}

// TestMarkForwardLeftover mirrors TestMarkReStepped: the same "might be
// callable" gate, keyed by value ID, quoted and ID-less values and a run
// with no analysis state recording nothing, the map reused.
func TestMarkForwardLeftover(t *testing.T) {
	carrier := func(id string) Value {
		v := NewCarrier(TFunction)
		v.ID = id
		return v
	}
	dynAny := NewCarrier(TAny)
	dynAny.Dynamic = true
	dynAny.ID = "dyn-2"
	quoted := carrier("fnc-q")
	quoted.Quoted = true
	e := &Engine{Registry: &Registry{Check: &CheckState{}}}
	for _, v := range []Value{quoted, carrier(""), NewInteger(1), NewCarrier(TInteger)} {
		e.markForwardLeftover(v)
	}
	if len(e.Registry.Check.ForwardLeftoverFnIDs) != 0 {
		t.Errorf("a quoted, ID-less, concrete or non-callable value records nothing: %v", e.Registry.Check.ForwardLeftoverFnIDs)
	}
	e.markForwardLeftover(carrier("fnc-2"))
	e.markForwardLeftover(dynAny)
	if !e.Registry.Check.ForwardLeftoverFnIDs["fnc-2"] || !e.Registry.Check.ForwardLeftoverFnIDs["dyn-2"] {
		t.Errorf("a fn-typed carrier and a fn-admitting dynamic value are marked, the map reused: %v", e.Registry.Check.ForwardLeftoverFnIDs)
	}
	for _, e := range []*Engine{{}, {Registry: &Registry{}}} {
		e.markForwardLeftover(carrier("fnc-3"))
		if e.Registry != nil && e.Registry.Check != nil {
			t.Error("a run with no check state records nothing")
		}
	}
}

// TestNoteWordReadMarksFnCarrier: the bare read of a fn-typed binding
// leaves its carrier's ID for the collapse (WordReadFnIDs) — under the same
// gate as the recorder note — while a quoted value, a concrete value and a
// read a Function-expecting forward takes record nothing.
func TestNoteWordReadMarksFnCarrier(t *testing.T) {
	carrier := NewCarrier(TFunction)
	carrier.ID = "fnc-r"
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	dyn.ID = "dyn-r"
	newEngine := func(tape ...Value) *Engine {
		r := &Registry{Check: &CheckState{}}
		return &Engine{Registry: r, Tape: NewTape(tape, 8), Pointer: len(tape)}
	}
	e := newEngine(NewInteger(1))
	e.noteWordRead(carrier, "g", SrcPos{})
	e.noteWordRead(dyn, "x", SrcPos{})
	if !e.Registry.Check.WordReadFnIDs["fnc-r"] || !e.Registry.Check.WordReadFnIDs["dyn-r"] {
		t.Errorf("a fn-typed carrier and a fn-admitting dynamic read are marked: %v", e.Registry.Check.WordReadFnIDs)
	}
	e = newEngine(NewInteger(1))
	quoted := carrier
	quoted.Quoted = true
	idless := carrier
	idless.ID = ""
	e.noteWordRead(quoted, "g", SrcPos{})
	e.noteWordRead(idless, "g", SrcPos{})
	e.noteWordRead(NewInteger(5), "n", SrcPos{})
	e.noteWordRead(NewCarrier(TInteger), "n", SrcPos{})
	if len(e.Registry.Check.WordReadFnIDs) != 0 {
		t.Errorf("a quoted, ID-less, concrete or non-fn read records nothing: %v", e.Registry.Check.WordReadFnIDs)
	}
	// A pending forward expecting a Function is no exception (NUR078): the
	// bare read dispatches on both engines, so it is marked like any other.
	sig := &Signature{Args: []*Type{TFunction}, BarrierPos: 1}
	fwd := NewForward(ForwardInfo{FuncName: "each", ExpectedArgs: 1, Sig: sig})
	e = newEngine(fwd)
	e.noteWordRead(carrier, "g", SrcPos{})
	if !e.Registry.Check.WordReadFnIDs["fnc-r"] {
		t.Errorf("a bare read before a Function slot is marked: %v", e.Registry.Check.WordReadFnIDs)
	}
}

// TestParenFeedsPendingForwardAfterGroup pins the scan's exhausted exit: a
// forward that lives AFTER the group (the tape holds one, so the fast exit
// does not fire) is nobody the group feeds — the backward scan runs out of
// tape without meeting a forward or an open paren.
func TestParenFeedsPendingForwardAfterGroup(t *testing.T) {
	sig := &Signature{Args: []*Type{TAny, TAny}, BarrierPos: 2}
	collecting := NewForward(ForwardInfo{FuncName: "cadd", ExpectedArgs: 2, CollectedArgs: 1, Sig: sig})
	tape := []Value{NewOpenParen(), NewInteger(2), NewCloseParen(), collecting}
	e := &Engine{Tape: NewTape(tape, 8)}
	if e.parenFeedsPendingForward(0) {
		t.Error("a forward after the group is not one the group feeds: the scan exhausts the tape")
	}
}

// trailingFnCloseEngine is the collapse harness for NUR184's trailing-fn
// arms: a check pass with an ACTIVE recorder, the paren `(3 <fn carrier>)`
// laid out after any prefix tokens, the pointer on the close paren.
func trailingFnCloseEngine(t *testing.T, prefix []Value, suffix ...Value) (*Engine, *s5bEmit, Value) {
	t.Helper()
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.dynApplyOK = true
	installS5BEmit(t, r, es)
	e := NewTop(r)
	last := NewCarrier(TFunction)
	last.ID = "fnc-trailing"
	tape := append([]Value(nil), prefix...)
	tape = append(tape, NewOpenParen(), NewInteger(3), last, NewCloseParen())
	closeIdx := len(tape) - 1
	tape = append(tape, suffix...)
	e.Tape = NewTape(tape, StackHeadroom)
	e.Pointer = closeIdx
	return e, es, last
}

// TestCloseParenTrailingFnFeedsForward pins stepCloseParen's forward-fed
// arm of the trailing-fn switch (NUR184): a collapse under the eager
// forward-argument evaluator (feedsForward) or directly under a PARKED
// forward still collecting (parenFeedsPendingForward) records NO apply —
// the survivors become that collection's candidates, and the fn value is
// marked re-stepped and a possible forward leftover. `10 mul (2 (mk 1))`
// is 21: mul takes the 2, the closure re-steps later over the 20.
func TestCloseParenTrailingFnFeedsForward(t *testing.T) {
	sig := &Signature{Args: []*Type{TAny, TAny}, BarrierPos: 2}
	collecting := NewForward(ForwardInfo{FuncName: "cadd", ExpectedArgs: 2, CollectedArgs: 1, Sig: sig})
	cases := []struct {
		name         string
		prefix       []Value
		reStepped    bool
		feedsForward bool
	}{
		{"the group's own close under the eager evaluator", nil, false, true},
		{"a main-loop collapse directly under a collecting forward", []Value{NewInteger(10), collecting}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, es, last := trailingFnCloseEngine(t, tc.prefix)
			if err := e.stepCloseParen(tc.reStepped, tc.feedsForward); err != nil {
				t.Fatalf("stepCloseParen: %v", err)
			}
			if es.dynApplies != 0 || len(es.trailing) != 0 {
				t.Errorf("a forward-fed collapse records no apply and registers no residual: applies=%d trailing=%v", es.dynApplies, es.trailing)
			}
			if e.Tape.Len() != len(tc.prefix)+2 {
				t.Errorf("both survivors stay for the collection, tape len %d", e.Tape.Len())
			}
			ck := e.Registry.Check
			if !ck.ParenReSteppedFnIDs[last.ID] || !ck.ForwardLeftoverFnIDs[last.ID] {
				t.Errorf("the fn value is marked re-stepped and a forward leftover: restepped=%v leftover=%v", ck.ParenReSteppedFnIDs, ck.ForwardLeftoverFnIDs)
			}
		})
	}
}

// TestCloseParenTrailingFnCollectsPastClose pins the switch's third arm: a
// trailing fn value that would forward-collect the token after the close
// paren once re-stepped (`(2 (mk 1)) 10` is `[2 11]`) records no apply over
// the values INSIDE the paren and is marked re-stepped only — not a
// forward leftover, since no collection is pending.
func TestCloseParenTrailingFnCollectsPastClose(t *testing.T) {
	e, es, last := trailingFnCloseEngine(t, nil, NewOpenParen(), NewInteger(10), NewCloseParen())
	if err := e.stepCloseParen(true, false); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if es.dynApplies != 0 || len(es.trailing) != 0 {
		t.Errorf("a fn collecting past the close records no in-paren apply: applies=%d trailing=%v", es.dynApplies, es.trailing)
	}
	if e.Tape.Len() != 5 {
		t.Errorf("both survivors stay ahead of the following group, tape len %d", e.Tape.Len())
	}
	ck := e.Registry.Check
	if !ck.ParenReSteppedFnIDs[last.ID] {
		t.Errorf("the fn value is marked re-stepped: %v", ck.ParenReSteppedFnIDs)
	}
	if ck.ForwardLeftoverFnIDs[last.ID] {
		t.Errorf("no collection is pending, so no forward-leftover mark: %v", ck.ForwardLeftoverFnIDs)
	}
}
