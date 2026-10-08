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
	dyn.Dynamic = true
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
// body, bound as written and marked consumed; under a window that may park
// it stays pending, owed the residual sweep's evaluation (NUR337).
func TestEvalTrailingContainerRawSlot(t *testing.T) {
	body := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	body.Eval = true
	m := NewOrderedMap()
	m.Set("a", NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}))
	mv := NewMap(m)
	mv.Eval = true
	if !IsPendingActiveContainer(body) || !IsPendingActiveContainer(mv) {
		t.Fatal("both literals are pending active containers")
	}
	sig := &Signature{NoEvalArgs: map[int]bool{0: true}, NoEvalMapArgs: map[int]bool{1: true}}
	e := &Engine{}
	for _, c := range []struct {
		v    Value
		p    int
		fits bool
	}{{body, 0, true}, {body, 0, false}, {mv, 1, true}, {mv, 1, false}} {
		got, ok := e.evalTrailingContainer(c.v, sig, c.p, c.fits)
		if ok != c.fits || got.Eval == c.fits || !BearsActiveTokens(got) {
			t.Errorf("raw slot %d (fits=%v): got %v ok=%v eval=%v", c.p, c.fits, got, ok, got.Eval)
		}
	}
	// NEGATIVE: a container with nothing to evaluate, a quoted one, and a
	// scalar are not pending active containers.
	inert := NewList([]Value{NewInteger(1)})
	inert.Eval = true
	quoted := body
	quoted.Quoted = true
	for _, v := range []Value{inert, quoted, NewInteger(3)} {
		if IsPendingActiveContainer(v) {
			t.Errorf("%v must not be a pending active container", v)
		}
	}
}
