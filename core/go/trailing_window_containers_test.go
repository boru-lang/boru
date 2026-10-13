package core

import "testing"

// TestProvenWindowMatch walks the proof both halves of a paren-bounded
// fn-value apply share (the compiler's applyWindowFits, the collapse's
// bind-time evaluation of a pending container — NUR337): a match over
// concrete arguments proves the apply; a gradual argument, no match, and a
// value pattern over a carrier prove nothing.
func TestProvenWindowMatch(t *testing.T) {
	fnOf := func(sigs ...FnSig) Value {
		return NewFunction(FnDefInfo{Anonymous: true, Signatures: sigs})
	}
	one := FnSig{Params: []FnParam{{Name: "n", Type: TInteger}}, Returns: []*Type{TAny}}
	if !ProvenWindowMatch(fnOf(one), []Value{NewInteger(1)}) {
		t.Error("a concrete conforming argument proves the match")
	}
	if !ProvenWindowMatch(fnOf(one), []Value{NewCarrier(TInteger)}) {
		t.Error("a typed carrier proves a match with no value pattern")
	}
	if ProvenWindowMatch(fnOf(one), []Value{NewString("s")}) {
		t.Error("a non-conforming argument proves nothing")
	}
	dyn := NewCarrier(TInteger)
	SetDynamic(&dyn, true)
	if ProvenWindowMatch(fnOf(one), []Value{dyn}) {
		t.Error("a gradual argument proves nothing")
	}
	pat := NewInteger(7)
	valPat := FnSig{Params: []FnParam{{Name: "n", Type: TInteger, Pattern: &pat}}, Returns: []*Type{TAny}}
	if !ProvenWindowMatch(fnOf(valPat), []Value{NewInteger(7)}) {
		t.Error("a value pattern over its own concrete value proves the match")
	}
	if ProvenWindowMatch(fnOf(valPat), []Value{NewCarrier(TInteger)}) {
		t.Error("a value pattern over a carrier is a run-time question")
	}
	// A map pattern admits a shaped map CARRIER by open unification (the
	// carrier's own keys), so the match succeeds — and still proves nothing
	// about the run-time value.
	mapPat := NewMap(NewOrderedMap())
	shape := NewOrderedMap()
	shape.Set("a", NewCarrier(TInteger))
	mapCarrier := NewMap(shape)
	mapCarrier.Carrier = true
	mapSig := FnSig{Params: []FnParam{{Name: "m", Type: TMap, Pattern: &mapPat}}, Returns: []*Type{TAny}}
	if MatchFnSig(fnOf(mapSig), []Value{mapCarrier}) == nil {
		t.Fatal("the map pattern must admit the shaped carrier by open unification")
	}
	if ProvenWindowMatch(fnOf(mapSig), []Value{mapCarrier}) {
		t.Error("a map pattern over a map carrier is a run-time question")
	}
}

// TestEvalTrailingContainerRawSlot: a pending container at a signature
// position that takes its argument RAW (NoEvalArgs / NoEvalMapArgs) is never
// evaluated by the collapse — under a fitting window it is the callee's code
// body, bound as written and marked consumed in its own cell; under a window
// that may park it stays pending, owed the residual sweep's evaluation
// (NUR337).
func TestEvalTrailingContainerRawSlot(t *testing.T) {
	r := covRegistry(t, nil)
	body := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	SetEval(&body, true)
	m := NewOrderedMap()
	m.Set("a", NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}))
	mv := NewMap(m)
	SetEval(&mv, true)
	if !IsPendingActiveContainer(body) || !IsPendingActiveContainer(mv) {
		t.Fatal("both literals are pending active containers")
	}
	// lead is a one-param fn taking its argument raw.
	lead := func(ty *Type, rawMap bool) Value {
		sig := FnSig{Params: []FnParam{{Type: ty}}, Returns: []*Type{TAny}, BarrierPos: 1}
		if rawMap {
			sig.NoEvalMapArgs = map[int]bool{0: true}
		} else {
			sig.NoEvalArgs = map[int]bool{0: true}
		}
		return NewFunction(FnDefInfo{Anonymous: true, Signatures: []FnSig{sig}})
	}
	for _, c := range []struct {
		name      string
		lead, arg Value
		fits      bool
	}{
		{"list, fitting", lead(TList, false), body, true},
		{"list, may park", lead(TInteger, false), body, false},
		{"map, fitting", lead(TMap, true), mv, true},
		{"map, may park", lead(TInteger, true), mv, false},
	} {
		if trailingWindowFits(c.lead, []Value{c.arg}) != c.fits {
			t.Fatalf("%s: the window's fit is not the case's", c.name)
		}
		e := NewTop(r)
		e.Tape = NewTape([]Value{NewOpenParen(), c.arg, c.lead, NewCloseParen()}, StackHeadroom)
		e.evalTrailingWindowContainers(c.lead, 0, 3, 2)
		got := e.Tape.At(1)
		if IsEval(&got) == c.fits || !BearsActiveTokens(got) {
			t.Errorf("%s: got %v eval=%v", c.name, got, IsEval(&got))
		}
	}
	// The literal the window held is the caller's: a consumed mark went to
	// the cell alone.
	if !IsEval(&body) || !IsEval(&mv) {
		t.Error("a raw slot's consumed mark reached the literal the cell was copied from")
	}
	// NEGATIVE: a container with nothing to evaluate, a quoted one, and a
	// scalar are not pending active containers.
	inert := NewList([]Value{NewInteger(1)})
	SetEval(&inert, true)
	quoted := body
	SetQuoted(&quoted, true)
	for _, v := range []Value{inert, quoted, NewInteger(3)} {
		if IsPendingActiveContainer(v) {
			t.Errorf("%v must not be a pending active container", v)
		}
	}
}
