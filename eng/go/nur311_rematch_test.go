package eng

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestRematchStoppedTuple pins the rematch's rebuild of the interpreter's
// no-match tuple (NUR311): a written operand that is no concrete value at
// run time stops the tuple there, and the recorded stack prefix fills it; a
// concrete written run, a type literal on the stack, or no recorded prefix
// leaves the render tuple as recorded.
func TestRematchStoppedTuple(t *testing.T) {
	r := pnmRegistry(t, []core.Signature{pnmSig(-1, core.TInteger, core.TString)})
	fn := r.Lookup("pnmw")
	nine, three := core.NewInteger(9), core.NewInteger(3)
	typ := core.NewTypeLiteral(core.TInteger)
	for _, c := range []struct {
		why    string
		ds     compiler.DispatchSpec
		window []core.Value
		want   string
	}{
		{"a type written first: the prefix alone",
			compiler.DispatchSpec{NArgs: 3, NFwd: 2, Written: []int{1, 2}, Prefix: []int{0}, PrefixKnown: true},
			[]core.Value{nine, typ, three}, "[9]"},
		{"a type written second: the first, filled from the prefix",
			compiler.DispatchSpec{NArgs: 3, NFwd: 2, Written: []int{1, 2}, Prefix: []int{0}, PrefixKnown: true},
			[]core.Value{nine, three, typ}, "[3 9]"},
		{"nothing beneath and a type first: none supplied",
			compiler.DispatchSpec{NArgs: 2, NFwd: 2, Written: []int{0, 1}, PrefixKnown: true},
			[]core.Value{typ, three}, "[]"},
		{"no recorded prefix: the recorded tuple",
			compiler.DispatchSpec{NArgs: 3, NFwd: 2, Written: []int{1, 2}},
			[]core.Value{nine, typ, three}, "[Integer 3]"},
		{"a prefix index out of the window: the recorded tuple",
			compiler.DispatchSpec{NArgs: 3, NFwd: 2, Written: []int{1, 2}, Prefix: []int{7}, PrefixKnown: true},
			[]core.Value{nine, typ, three}, "[Integer 3]"},
		{"concrete written operands: the recorded tuple",
			compiler.DispatchSpec{NArgs: 3, NFwd: 2, Written: []int{1, 2}, Prefix: []int{0}, PrefixKnown: true},
			[]core.Value{nine, three, three}, "[3 3]"},
		{"a type on the stack renders",
			compiler.DispatchSpec{NArgs: 2, NFwd: 1, Written: []int{1, 0}, Prefix: []int{0}, PrefixKnown: true},
			[]core.Value{typ, three}, "[3 Integer]"},
	} {
		written, ok := tupleAt(c.window, c.ds.Written)
		if !ok {
			t.Fatalf("%s: bad render tuple", c.why)
		}
		if got := fmt.Sprint(rematchStoppedTuple(&c.ds, fn, c.window, written)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.why, got, c.want)
		}
	}
}

// TestPolyNoMatchRaiseStopsAtATypeOperand: the poly's faithful raise
// rebuilds the same stopped tuple from its StackTuple, so a written type
// literal leaves only the stack values in the report (NUR311).
func TestPolyNoMatchRaiseStopsAtATypeOperand(t *testing.T) {
	r := pnmRegistry(t, []core.Signature{pnmSig(-1, core.TInteger, core.TString)})
	vc := seam7VC(r)
	fn := r.Lookup("pnmw")
	window := []core.Value{core.NewTypeLiteral(core.TBoolean), core.NewBoolean(true)}
	spec := &core.PolyNoMatchSpec{Written: []int{0, 1}, NFwd: 1, StackTuple: []int{1}, NSigs: 1, Pos: core.SrcPos{Row: 1, Col: 1}}
	err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, fn, window, seam7Dbg, 0)
	var be *core.BoruError
	if !errors.As(err, &be) || be.Code != "signature_error" {
		t.Fatalf("got %v, want the no-match signature_error", err)
	}
	notes := strings.Join(be.Notes, "\n")
	if !strings.Contains(notes, "the argument was true") || strings.Contains(notes, "Boolean (") {
		t.Errorf("the report renders the stack value alone, got:\n%s", notes)
	}
}
