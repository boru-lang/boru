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
	mapCarrier.SetCarrier(true)
	mapSig := FnSig{Params: []FnParam{{Name: "m", Type: TMap, Pattern: &mapPat}}, Returns: []*Type{TAny}}
	if MatchFnSig(fnOf(mapSig), []Value{mapCarrier}) == nil {
		t.Fatal("the map pattern must admit the shaped carrier by open unification")
	}
	if ProvenWindowMatch(fnOf(mapSig), []Value{mapCarrier}) {
		t.Error("a map pattern over a map carrier is a run-time question")
	}
}
