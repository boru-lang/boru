package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestPolyBarredNeedsARegistry pins polyBarred's guard: a recorder with no
// bound registry, or no registry to re-match over, reads no split and bars
// nothing (NUR362).
func TestPolyBarredNeedsARegistry(t *testing.T) {
	var none *EmitState
	if barred, sigs := none.polyBarred(nil, "get", nil); barred || sigs != nil {
		t.Error("a nil recorder bars nothing")
	}
	if barred, _ := NewEmitState().polyBarred(nil, "get", nil); barred {
		t.Error("an unbound recorder bars nothing")
	}
}

// TestBarredLayoutOK pins when a barred dispatch's layout is one its run
// may plan over (NUR362): a barred overload that would read a value beneath
// the operands — one that fits its slot — takes a window the call does not
// hold, and so does one whose slots stay within the call's own stack
// operands; a constant beneath that misses the slot, or a stack that runs
// out, fails the overload whatever the run holds; a live value beneath is
// never planned over.
func TestBarredLayoutOK(t *testing.T) {
	key := core.Signature{Args: []*core.Type{core.TString, core.TInteger}, BarrierPos: 1}
	short := core.Signature{Args: []*core.Type{core.TString, core.TInteger}, BarrierPos: 1}
	all := core.Signature{Args: []*core.Type{core.TString, core.TInteger}, BarrierPos: 2}
	fb := core.Signature{Args: []*core.Type{core.TString, core.TInteger}, BarrierPos: 1, Fallback: true}
	stackOnly := core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: 0}
	for _, tc := range []struct {
		name  string
		l     core.DispatchLayout
		sigs  []core.Signature
		nArgs int
		want  bool
	}{
		{"nothing beneath", core.DispatchLayout{NFwd: 2}, []core.Signature{key, all}, 2, true},
		{"a fitting constant beneath", core.DispatchLayout{NFwd: 2, Beneath: []core.Value{core.NewInteger(9)}}, []core.Signature{key}, 2, false},
		{"a missing constant beneath", core.DispatchLayout{NFwd: 2, Beneath: []core.Value{core.NewString("y")}}, []core.Signature{short, all}, 2, true},
		{"a live value beneath", core.DispatchLayout{NFwd: 2, Live: []int{0}}, []core.Signature{key}, 2, false},
		{"within the call's stack operands", core.DispatchLayout{NFwd: 1}, []core.Signature{stackOnly}, 2, false},
		{"a fallback", core.DispatchLayout{NFwd: 2, Beneath: []core.Value{core.NewInteger(9)}}, []core.Signature{fb}, 2, true},
	} {
		if got := barredLayoutOK(&tc.l, tc.sigs, tc.nArgs); got != tc.want {
			t.Errorf("%s: barredLayoutOK = %v, want %v", tc.name, got, tc.want)
		}
	}
}
