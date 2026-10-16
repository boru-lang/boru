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
	SetID(&v, "fn-"+IDOf(&v))
	return v
}

func TestTrailingFnCollectsPastClose(t *testing.T) {
	carrier := NewCarrier(TFunction)
	SetID(&carrier, "fnc-1")
	intFn := ptfFn(t, TInteger)
	strFn := ptfFn(t, TString)
	nullary := ptfFn(t)
	noSigs := NewFunction(FnDefInfo{Name: "h"})
	quotedWord := NewValueRaw(TWord, WordInfo{Name: "cmul", ArgCount: -1, ForceVal: true})
	cases := []struct {
		name string
		last Value
		tail []Value // the tokens after the close paren
		want bool
		why  string
	}{
		{"nothing after the close", carrier, nil, false, "the value falls to the stack — the values inside the paren"},
		{"a function word after the close", carrier, []Value{NewWord("cmul")}, false, "`(2 (mk 1)) mul 10` is 30: a function word is the strict barrier"},
		{"a value word after the close", intFn, []Value{NewWord("v")}, true, "`(\"s\" lam/v) v` over `def v 5` is `[s 105]`: the binding's value arrives (NUR344)"},
		{"a /v word after the close", intFn, []Value{quotedWord}, true, "a reference is no barrier: its value arrives and the re-step decides"},
		{"a paren expression after the close", intFn, []Value{NewParenExpr([]Value{NewInteger(2)})}, true, "`(\"s\" lam/v) (2 add 3)` is `[s 105]`: the group is evaluated for the slot (NUR344)"},
		{"a reach after the close", intFn, []Value{NewReach(ReachInfo{})}, true, "`(\"s\" lam/v) m.a` reads the member for the slot (NUR344)"},
		{"an interpolated string after the close", strFn, []Value{NewInterpString(nil)}, true, "the template is evaluated for the slot (NUR344)"},
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
			e := &Engine{Registry: covRegistry(t, nil), Tape: NewTape(tape, 8)}
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
