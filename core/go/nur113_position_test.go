package core

import "testing"

// TestWithPosAt pins the SrcPos twin of WithPos: it gives a value a position
// only when the value has none and the position is real, so it can never
// overwrite a more specific location with a broader one.
func TestWithPosAt(t *testing.T) {
	at := SrcPos{Row: 1, Col: 20}

	got := WithPosAt(NewFunction(FnDefInfo{Name: "exported"}), at)
	if got.Pos() != at {
		t.Errorf("a positionless value takes the given position: got %v, want %v", got.Pos(), at)
	}

	// A value that already knows where it came from keeps it — the whole
	// reason for the guard, since the caller stamps every result of a call.
	own := SrcPos{Row: 9, Col: 3}
	v := NewFunction(FnDefInfo{Name: "own"})
	v.SetPos(own)
	if got := WithPosAt(v, at); got.Pos() != own {
		t.Errorf("an already-positioned value is left alone: got %v, want %v", got.Pos(), own)
	}

	// A zero position is not a position; stamping it would erase the
	// distinction the callers test on.
	if got := WithPosAt(NewFunction(FnDefInfo{Name: "z"}), SrcPos{}); got.Pos().Row != 0 {
		t.Errorf("a zero position must not be stamped: got %v", got.Pos())
	}
}

// TestAnalysisFunctionResultTakesCallPosition pins NUR113's core half: the
// check-mode dispatch seam stamps a positionless FUNCTION result with the
// call's own position, the equivalent of the interpreter's stampResultCells.
//
// It matters because a module export is handed back verbatim, built at
// module-construction time with no position, and it is the token a later VALUE
// dispatch reads its own position from — so without this every opcode
// downstream of a dot-access records 0:0.
func TestAnalysisFunctionResultTakesCallPosition(t *testing.T) {
	r := covRegistry(t, nil)
	r.Check.Mode = true
	t.Cleanup(func() { r.Check.Mode = false })

	prev := AnalysisImpl.CarrierResults
	AnalysisImpl.CarrierResults = func(*Registry, string, *Signature, []Value, SrcPos, *Registry, bool) []Value {
		// A module export as moduleNSGetReturns hands it back: verbatim,
		// carrying no position of its own.
		return []Value{NewFunction(FnDefInfo{Name: "assert-not-equal"})}
	}
	t.Cleanup(func() { AnalysisImpl.CarrierResults = prev })

	at := SrcPos{Row: 1, Col: 20}
	word := NewWord("covid")
	word.SetPos(at)
	e := NewTop(r)
	e.Tape = NewTape([]Value{word}, StackHeadroom)
	e.Pointer = 0

	sig := &Signature{Args: []*Type{}, Returns: []*Type{TFunction}}
	if err := e.execMatch(&MatchResult{Sig: sig, Name: "covid"}); err != nil {
		t.Fatalf("analysis dispatch must not raise: %v", err)
	}
	found := false
	for i := 0; i < e.Tape.Len(); i++ {
		v := e.Tape.At(i)
		if v.Parent() != nil && v.Parent().Equal(TFunction) {
			found = true
			if v.Pos() != at {
				t.Errorf("the Function result carries the call's position: got %v, want %v", v.Pos(), at)
			}
		}
	}
	if !found {
		t.Fatal("the analysis seam left no Function result on the tape — test premise broken")
	}
}
