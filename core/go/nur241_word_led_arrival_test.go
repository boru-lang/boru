package core

import "testing"

// nur241Sigs is append's shape: a list-valued and an any-valued first slot
// over a list second slot, both all-forward, neither capturing a name.
func nur241Sigs() []Signature {
	sigs := []Signature{
		{Args: []*Type{TList, TFlexList}, BarrierPos: -1},
		{Args: []*Type{TAny, TFlexList}, BarrierPos: -1},
	}
	for i := range sigs {
		NormalizeSig(&sigs[i])
	}
	return sigs
}

// nur241SigsWithFallback is nur241Sigs plus the 0-arg Fallback signature a
// boru fn carries (the synthesized no-match), which fits any stack.
func nur241SigsWithFallback() []Signature {
	fb := Signature{BarrierPos: -1, Fallback: true}
	NormalizeSig(&fb)
	return append(nur241Sigs(), fb)
}

// nur241Arrival lays out the arrival NUR241 describes — `acc "x" ap acc
// (m.path)` at the moment the paren's value arrives: the stack beneath the
// word, the one value already collected forward, the word, its Forward
// marker and the arriving value — and runs noteWordLedArrival on a check pass
// that is (or is not) compiling. It reports whether the split was flagged.
func nur241Arrival(t *testing.T, compiling, register bool, beneath []Value, arriving Value) bool {
	t.Helper()
	return nur241ArrivalOver(t, compiling, register, nur241Sigs(), beneath, arriving)
}

// nur241ArrivalOver is nur241Arrival with the word's registered signatures
// given (the plan's own is always the first).
func nur241ArrivalOver(t *testing.T, compiling, register bool, sigs []Signature, beneath []Value, arriving Value) bool {
	t.Helper()
	r := covRegistry(t, func(r *Registry) {
		if register {
			r.RegisterNativeFunc(NativeFunc{Name: "ap", Signatures: sigs})
		}
	})
	r.Check.Mode = true
	r.Check.Compiling = compiling
	t.Cleanup(func() { r.Check.Mode, r.Check.Compiling, r.Check.AmbiguousGradualSplit = false, false, false })
	acc := NewDynamicCarrier(TFlexList)
	toks := append(append([]Value(nil), beneath...), acc, NewWord("ap"))
	funcIdx := len(toks) - 1
	fwd := ForwardInfo{FuncName: "ap", ExpectedArgs: 2, CollectedArgs: 1, FuncIndex: funcIdx, Sig: &sigs[0], WordLed: true}
	toks = append(toks, NewForward(fwd), arriving)
	e := NewTop(r)
	e.Tape = NewTape(toks, StackHeadroom)
	e.Pointer = len(toks) - 1
	e.noteWordLedArrival(&fwd, len(toks)-1, funcIdx)
	return r.Check.AmbiguousGradualSplit
}

// TestNUR241WordLedArrivalIsAmbiguous pins NUR241: a compiling pass's
// deferred word-led window takes a value its slot cannot prove (a gradual
// Any at a FlexList slot) while the all-stack window fits the values beneath
// the word — the window the interpreter's planner prunes to once the paren
// evaluates to a value the slot refuses. The split is flagged, the compile
// declines; the planned window itself is unchanged.
func TestNUR241WordLedArrivalIsAmbiguous(t *testing.T) {
	beneath := []Value{NewDynamicCarrier(TFlexList), NewString("x")}
	if !nur241Arrival(t, true, true, beneath, NewDynamicCarrier(TAny)) {
		t.Error("an unproven arrival with a narrower window fitting beneath must be flagged")
	}
	// Unregistered, the plan's own signature is the one asked: `[List
	// FlexList]` takes no String first, but its window keeping the
	// collected value does (the FlexList beneath fills the second slot).
	if !nur241Arrival(t, true, false, []Value{NewDynamicCarrier(TFlexList)}, NewDynamicCarrier(TAny)) {
		t.Error("the plan's own signature's narrower window must be asked")
	}
}

// TestNUR241NoAmbiguity is the paired negative: a plain check pass, a proven
// arrival, a slot past the signature, and a stack no narrower window fits
// (`add x (m.v)` over an empty frame) flag nothing.
func TestNUR241NoAmbiguity(t *testing.T) {
	beneath := []Value{NewDynamicCarrier(TFlexList), NewString("x")}
	if nur241Arrival(t, false, true, beneath, NewDynamicCarrier(TAny)) {
		t.Error("a check pass that is not compiling never flags")
	}
	if nur241Arrival(t, true, true, beneath, NewFlexList(nil)) {
		t.Error("a concrete arrival its slot takes is proven")
	}
	if nur241Arrival(t, true, true, nil, NewDynamicCarrier(TAny)) {
		t.Error("with nothing beneath the word no narrower window fits")
	}
	// A boru fn's synthesized 0-arg Fallback fits any stack, but plans no
	// window: `g k (id 5)` over g's two 2-arg overloads and nothing beneath
	// the word is no ambiguity.
	if nur241ArrivalOver(t, true, true, nur241SigsWithFallback(), nil, NewDynamicCarrier(TAny)) {
		t.Error("a Fallback signature is no narrower window")
	}
	// The word-led token's own arrival (slot 0): the gradual first operand
	// of every `f x` — `pf d` over a gradual d — never asks, however much
	// sits beneath the word.
	sigs := nur241Sigs()
	r := covRegistry(t, nil)
	r.Check.Mode, r.Check.Compiling = true, true
	t.Cleanup(func() { r.Check.Mode, r.Check.Compiling, r.Check.AmbiguousGradualSplit = false, false, false })
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewString("x"), NewDynamicCarrier(TFlexList), NewWord("ap"), NewDynamicCarrier(TAny)}, StackHeadroom)
	e.noteWordLedArrival(&ForwardInfo{FuncName: "ap", CollectedArgs: 0, Sig: &sigs[0], WordLed: true}, 3, 2)
	if r.Check.AmbiguousGradualSplit {
		t.Error("the word-led token's own arrival is not the plan's later operand")
	}
	// A slot past the signature (a plan whose collection already filled it).
	e = NewTop(r)
	e.Tape = NewTape([]Value{NewWord("ap"), NewDynamicCarrier(TAny)}, StackHeadroom)
	e.noteWordLedArrival(&ForwardInfo{FuncName: "ap", CollectedArgs: 2, Sig: &sigs[0], WordLed: true}, 1, 0)
	if r.Check.AmbiguousGradualSplit {
		t.Error("a slot past the signature is not the plan's")
	}
}

// TestNUR241WordLedPlanIsMarked: the plan a Word leads and no name capture
// takes is marked on its Forward (insertForward), and a run through it
// collects as before — `9 cadd x 3` over x = 5 is 8 above the 9.
func TestNUR241WordLedPlanIsMarked(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("x", NewInteger(5))
	out, err := NewTop(r).Run([]Value{NewInteger(9), NewWord("cadd"), NewWord("x"), NewInteger(3)})
	if err != nil || renderAll(out) != "9 | 8" {
		t.Fatalf("got %v / %v, want 9 | 8", renderAll(out), err)
	}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("cadd"), NewWord("x"), NewInteger(3)}, StackHeadroom)
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	if err := e.insertForward(WordInfo{Name: "cadd"}, sig, 2, 0, -1); err != nil {
		t.Fatal(err)
	}
	fwd, _ := AsForward(e.Tape.At(1))
	if !fwd.WordLed {
		t.Error("a Word next and no name capture at the first slot is the deferred word-led plan")
	}
	e = NewTop(r)
	e.Tape = NewTape([]Value{NewWord("cadd"), NewInteger(1), NewInteger(3)}, StackHeadroom)
	if err := e.insertForward(WordInfo{Name: "cadd"}, sig, 2, 0, -1); err != nil {
		t.Fatal(err)
	}
	if fwd, _ = AsForward(e.Tape.At(1)); fwd.WordLed {
		t.Error("a literal next is no word-led plan")
	}
}
