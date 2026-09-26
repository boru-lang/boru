package core

import "testing"

// undecidedRecorder is an ACTIVE recorder whose dynamic-apply record answers
// as configured, counting the calls (NUR254).
type undecidedRecorder struct {
	inactiveEmit
	consumed int
	ok       bool
	calls    int
}

func (u *undecidedRecorder) Active() bool { return true }
func (u *undecidedRecorder) RecordDynApply([]Value, Value, Value, SrcPos) (int, bool) {
	u.calls++
	return u.consumed, u.ok
}

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

// TestNUR254RecordUndecidedApply pins the record: an undecided anonymous
// park on a compiling pass with an active recorder records the dynamic apply
// and collapses the window and the value to its result; a record the
// recorder declines flags the gradual split and parks as before.
func TestNUR254RecordUndecidedApply(t *testing.T) {
	run := func(ok bool) (*Engine, *undecidedRecorder, error) {
		r, err := NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		rec := &undecidedRecorder{consumed: 1, ok: ok}
		r.Check.Mode, r.Check.Compiling, r.Check.Emit = true, true, rec
		fd := nur254Lambda(false)
		e := NewTop(r)
		e.Tape = NewTape([]Value{NewInteger(9), NewCarrier(TInteger), NewFunction(fd)}, StackHeadroom)
		e.Pointer = 2
		err = e.ExecFnDefSigStackMatch(2, fd, []Value{NewInteger(9), NewCarrier(TInteger)})
		return e, rec, err
	}
	e, rec, err := run(true)
	if err != nil || rec.calls != 1 || e.Tape.Len() != 2 || e.Pointer != 2 || !e.Tape.At(1).Carrier || !ExactEqual(e.Tape.At(0), NewInteger(9)) {
		t.Fatalf("recorded: want [9 result] with the pointer past it, got %v pointer=%d calls=%d err=%v", renderAll(e.Tape.Snapshot()), e.Pointer, rec.calls, err)
	}
	if e.Registry.Check.AmbiguousGradualSplit {
		t.Error("a recorded apply is no ambiguity")
	}
	e, rec, err = run(false)
	if err != nil || rec.calls != 1 || e.Tape.Len() != 3 || e.Pointer != 3 || !e.Registry.Check.AmbiguousGradualSplit {
		t.Fatalf("declined: want the park and the flag, got %v pointer=%d split=%v err=%v", renderAll(e.Tape.Snapshot()), e.Pointer, e.Registry.Check.AmbiguousGradualSplit, err)
	}
}
