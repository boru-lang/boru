package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_forward_fit_test.go pins the VM halves of NUR357's forward fits,
// NUR352's seated deopt prefix and NUR351's live split beneath.

// TestDeoptIfFnFitsAndSeat: a guarded read whose value misses the forward
// fit of the dispatch that collected it takes the island as a fn does
// (DeoptSpec.Fits); a statement island's prefix is seated where the
// compiled root keeps each residual entry (DeoptSpec.Seat); a seat over a
// frame region of another depth raises.
func TestDeoptIfFnFitsAndSeat(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	vc := seam7VC(r)
	mapSig := &core.Signature{Args: []*core.Type{core.TMap}, BarrierPos: -1}
	core.NormalizeSig(mapSig)
	five := core.NewInteger(5)
	body := []core.Value{core.NewInteger(7)}
	fits := []core.ForwardFit{{Sig: mapSig, Idx: 0}}
	// Plain data that fits: no-op.
	fitting := compiler.DeoptSpec{Name: "y", Slot: 0, Depth: -1, Token: 0, RetPC: 9, Fits: fits}
	if _, fired, err := vc.deoptIfFn(r, body, true, &fitting, 0, nil, []core.Value{core.NewMap(core.NewOrderedMap())}, seam7Dbg, 0); err != nil || fired {
		t.Errorf("a Map fits keys' slot: no island, got %v %v", fired, err)
	}
	if fitting.FitsHot(core.NewMap(core.NewOrderedMap())) || !fitting.FitsHot(five) || (&compiler.DeoptSpec{}).FitsHot(five) {
		t.Error("hot on a miss only, and never with no fits")
	}
	// A miss: the island runs the statement over the seated prefix — a
	// constant, a slot, the frame region's entry and a type.
	seat := compiler.DeoptSpec{Name: "y", Slot: 0, Depth: -1, Token: 0, RetPC: 9, Fits: fits, Held: 1, Seat: []compiler.RestartSrc{
		{Kind: compiler.RestartConst, Val: core.NewInteger(1)},
		{Kind: compiler.RestartLocal, Idx: 0},
		{Kind: compiler.RestartStack, Idx: 0},
		{Kind: compiler.RestartType, Val: core.NewTypeLiteral(core.TInteger)},
	}}
	st, fired, err := vc.deoptIfFn(r, body, true, &seat, 0, []core.Value{core.NewInteger(2)}, []core.Value{five}, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 5 {
		t.Fatalf("the island's residual is the seat and the statement: got %v %v %v", st, fired, err)
	}
	if n, _ := core.AsInteger(st[1]); n != 5 {
		t.Errorf("the slot's value seated second: %v", st)
	}
	// Refusals: another depth, a slot or an entry out of range, a type no
	// registry resolves, an unknown kind.
	for _, bad := range []compiler.DeoptSpec{
		{Held: 2, Seat: []compiler.RestartSrc{}},
		{Held: 1, Seat: []compiler.RestartSrc{{Kind: compiler.RestartLocal, Idx: 4}}},
		{Held: 1, Seat: []compiler.RestartSrc{{Kind: compiler.RestartStack, Idx: 1}}},
		{Held: 1, Seat: []compiler.RestartSrc{{Kind: compiler.RestartType, Val: core.Value{ID: "no-such-type"}}}},
		{Held: 1, Seat: []compiler.RestartSrc{{Kind: compiler.RestartGuard}}},
	} {
		bad.Name, bad.Slot, bad.Depth, bad.RetPC, bad.Fits = "y", 0, -1, 9, fits
		if _, _, err := vc.deoptIfFn(r, body, true, &bad, 0, []core.Value{core.NewInteger(2)}, []core.Value{five}, seam7Dbg, 0); err == nil {
			t.Errorf("seat %+v: raises", bad.Seat)
		}
	}
}

// TestSplitBeneathReadsLiveEntries: a split's live Beneath entry is read
// where the compiled code keeps it — a local, or the stack that deep below
// its top — and a place out of range leaves the arm no layout.
func TestSplitBeneathReadsLiveEntries(t *testing.T) {
	c := core.NewInteger(1)
	sp := &compiler.PolySplit{Beneath: []core.Value{c, core.NewInteger(0), core.NewInteger(0)}, Live: []compiler.SplitLive{
		{At: 1, Local: true, Idx: 0},
		{At: 2, Idx: 1},
	}}
	stack := []core.Value{core.NewInteger(8), core.NewInteger(9)}
	out, ok := splitBeneath(sp, stack, []core.Value{core.NewInteger(7)})
	if !ok || len(out) != 3 || out[0].String() != "1" || out[1].String() != "7" || out[2].String() != "8" {
		t.Errorf("[1 local stack-1]: got %v %v", out, ok)
	}
	if sp.Beneath[1].String() != "0" {
		t.Error("the recorded Beneath is never written")
	}
	for _, l := range []compiler.SplitLive{{At: 3}, {At: -1}, {At: 0, Local: true, Idx: 1}, {At: 0, Idx: 2}, {At: 0, Idx: -1}} {
		if _, ok := splitBeneath(&compiler.PolySplit{Beneath: []core.Value{c}, Live: []compiler.SplitLive{l}}, stack, []core.Value{c}); ok {
			t.Errorf("%+v: out of range, no layout", l)
		}
	}
	if out, ok := splitBeneath(&compiler.PolySplit{Beneath: []core.Value{c}}, nil, nil); !ok || len(out) != 1 {
		t.Error("no live entry: the Beneath itself")
	}
	r, _, _ := seam7DelegReg(t)
	pr := &compiler.PolyRef{Word: "cinc", Split: &compiler.PolySplit{Beneath: []core.Value{c}, Live: []compiler.SplitLive{{At: 0, Idx: 5}}}}
	if err := polySplitRaise(r, pr, r.Lookup("cinc"), []core.Value{c}, stack, nil, seam7Dbg, 0); err != nil {
		t.Errorf("a live place out of range: no layout, no raise; got %v", err)
	}
}

// TestCallFitAt: a CALL_USER's forward-fit island is read from the running
// unit's table, the main code's for no unit; nothing for no program or a
// unit out of range. PolyFit.Missed reads each fit's operand off the stack
// top, and a position past the stack is a miss.
func TestCallFitAt(t *testing.T) {
	mapSig := &core.Signature{Args: []*core.Type{core.TMap}, BarrierPos: -1}
	core.NormalizeSig(mapSig)
	f := &compiler.PolyFit{At: []int{0}, Fits: [][]core.ForwardFit{{{Sig: mapSig, Idx: 0}}}, Restart: &compiler.StmtIsland{}}
	p := &compiler.Program{CallFits: map[int]*compiler.PolyFit{3: f}, Fns: []compiler.CompiledFn{{CallFits: map[int]*compiler.PolyFit{4: f}}}}
	if callFitAt(p, -1, 3) != f || callFitAt(p, 0, 4) != f || callFitAt(p, 0, 3) != nil || callFitAt(p, 1, 4) != nil || callFitAt(nil, -1, 3) != nil {
		t.Error("the main code's table, a unit's, and nothing else")
	}
	if f.Missed([]core.Value{core.NewMap(core.NewOrderedMap())}) || !f.Missed([]core.Value{core.NewInteger(0)}) || !f.Missed(nil) {
		t.Error("a Map fits; an Integer misses; no operand is a miss")
	}
	neg := &compiler.PolyFit{At: []int{-1}, Fits: [][]core.ForwardFit{nil}}
	if !neg.Missed([]core.Value{core.NewInteger(0)}) {
		t.Error("a position out of range is a miss")
	}
	// The island's error is stamped at the call.
	r, _, _ := seam7DelegReg(t)
	vc := seam7VC(r)
	bad := &compiler.PolyFit{Restart: &compiler.StmtIsland{}}
	if _, _, err := vc.fitRestart(r, bad, nil, nil, nil, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "statement-island") {
		t.Errorf("an empty island raises: %v", err)
	}
	if _, _, err := vc.fitRestart(r, bad, []vmFrame{{stackBase: 0}}, nil, nil, seam7Dbg, 0); err == nil {
		t.Error("a unit frame's empty island raises too")
	}
}
