package core

import "testing"

// nur228Match runs MatchSignature for `dyn w {a:1} "s"` — a dynamic Any
// carrier on the stack, the word, then a Map and a String forward — over
// sigs, on a check pass that is (or is not) compiling, and reports the
// selected signature's forward count and whether the split was flagged.
func nur228Match(t *testing.T, compiling bool, sigs []Signature) (int, bool) {
	t.Helper()
	r := covRegistry(t, nil)
	r.Check.Mode = true
	r.Check.Compiling = compiling
	t.Cleanup(func() { r.Check.Mode, r.Check.Compiling = false, false })
	dyn := NewDynamicCarrier(TAny)
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	e := NewTop(r)
	e.Tape = NewTape([]Value{dyn, NewWord("w"), NewMap(om), NewString("s")}, StackHeadroom)
	e.Pointer = 1
	fn := &FnDefInfo{Name: "w", Signatures: sigs}
	sig, positions, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: 2}, []Value{dyn})
	if sig == nil {
		t.Fatalf("a signature must be selected")
	}
	fwd := 0
	for _, p := range positions {
		if p > 1 {
			fwd++
		}
	}
	return fwd, r.Check.AmbiguousGradualSplit
}

// TestNUR228GradualStackWindowIsAmbiguous pins NUR228: a candidate that takes
// one forward token and fills its next slot from the stack with a DYNAMIC
// carrier — an unproven match — while a later candidate forward-collects the
// token its scan stopped at, is a split the runtime value decides. A compiling
// pass flags it; the selected window is unchanged.
func TestNUR228GradualStackWindowIsAmbiguous(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny, TBoolean}, BarrierPos: 2},
		{Fallback: true},
		{Args: []*Type{TAny, TAny, TAny}, BarrierPos: 3}, // another arity: skipped
		{Args: []*Type{TAny, TInteger}, BarrierPos: 1},   // forwards no further: skipped
		{Args: []*Type{TAny, TAtom}, BarrierPos: 2},      // stops at the same token
		{Args: []*Type{TAny, TString}, BarrierPos: 2},    // collects past it
	}
	for i := range sigs {
		NormalizeSig(&sigs[i])
	}
	fwd, flagged := nur228Match(t, true, sigs)
	if fwd != 1 {
		t.Errorf("the selected candidate takes one forward token, took %d", fwd)
	}
	if !flagged {
		t.Error("a gradual stack window a later candidate forwards past must be flagged")
	}
}

// TestNUR228NoLaterClaimIsNoAmbiguity is the paired negative: with no later
// candidate claiming the stop token, the runtime takes the same window
// whatever the value (and the poly re-match picks among the sigs it fits), so
// nothing is flagged — and a plain check pass never flags at all.
func TestNUR228NoLaterClaimIsNoAmbiguity(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny, TBoolean}, BarrierPos: 2},
		{Args: []*Type{TAny, TAtom}, BarrierPos: 2},
	}
	for i := range sigs {
		NormalizeSig(&sigs[i])
	}
	if _, flagged := nur228Match(t, true, sigs); flagged {
		t.Error("no later candidate forwards past the stop token: no ambiguity")
	}
	withClaim := []Signature{
		{Args: []*Type{TAny, TBoolean}, BarrierPos: 2},
		{Args: []*Type{TAny, TString}, BarrierPos: 2},
	}
	for i := range withClaim {
		NormalizeSig(&withClaim[i])
	}
	if _, flagged := nur228Match(t, false, withClaim); flagged {
		t.Error("a plain check pass must not flag the split")
	}
}
