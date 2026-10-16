package core

import "testing"

// nur254Lambda is `[0] => [1]`'s shape: one unnamed Integer param under the
// value pattern 0.
func nur254Lambda(named bool) FnDefInfo {
	zero := NewInteger(0)
	p := FnParam{Type: TInteger, Pattern: &zero}
	if named {
		p.Name = "x"
	}
	sig := FnSig{Params: []FnParam{p}, Returns: []*Type{TAny}}
	NormalizeSig(&sig)
	return FnDefInfo{Anonymous: true, Signatures: []Signature{sig}}
}

// TestNUR254UndecidedPatternWindow pins the detection: a pattern over a
// carrier is undecided (named and unnamed param alike), a concrete value
// decides it either way, a slot the value's type refuses and a window too
// short are no candidates.
func TestNUR254UndecidedPatternWindow(t *testing.T) {
	carrier := NewCarrier(TInteger)
	for _, named := range []bool{false, true} {
		fd := nur254Lambda(named)
		sigs := fd.OwnSigs()
		if n := undecidedPatternWindow(sigs, []Value{carrier}); n != 1 {
			t.Errorf("named=%v: a carrier under the pattern is undecided, got %d", named, n)
		}
		if n := undecidedPatternWindow(sigs, []Value{NewInteger(5)}); n != 0 {
			t.Errorf("named=%v: a concrete miss is definite, got %d", named, n)
		}
		if n := undecidedPatternWindow(sigs, []Value{NewString("x")}); n != 0 {
			t.Errorf("named=%v: a refused type is no candidate, got %d", named, n)
		}
		if n := undecidedPatternWindow(sigs, nil); n != 0 {
			t.Errorf("named=%v: a short window is no candidate, got %d", named, n)
		}
	}
	// A pattern-free slot over a carrier beside a concrete pattern hit:
	// nothing undecided. A 0-arg signature is skipped.
	zero := NewInteger(0)
	two := FnSig{Params: []FnParam{{Name: "a", Type: TInteger, Pattern: &zero}, {Name: "b", Type: TInteger}}}
	NormalizeSig(&two)
	if n := undecidedPatternWindow([]Signature{{}, two}, []Value{carrier, NewInteger(0)}); n != 0 {
		t.Errorf("a concrete pattern hit decides the window, got %d", n)
	}
}
