package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNoteCallWindowPool pins the window pool (NUR234): an offer is keyed by
// the word token; a force-stack re-step keeps a deferred first step's offer
// and every other note replaces it; a suspended recorder pools nothing and
// drops the key's offer; the take removes it.
func TestNoteCallWindowPool(t *testing.T) {
	var nilES *EmitState
	nilES.NoteCallWindow("f", core.SrcPos{}, nil, false, false) // no-op, no panic

	at := core.SrcPos{Row: 1, Col: 5}
	first := []core.Value{core.NewInteger(7)}
	later := []core.Value{core.NewInteger(9)}

	es := NewEmitState()
	es.NoteCallWindow("f", at, first, true, false)
	es.NoteCallWindow("f", at, later, false, true) // the re-step keeps the deferred offer
	if w := es.takePendingWindow("f", at); !w.ok || !w.known || len(w.win) != 1 || intOf(w.win[0], 0) != 7 {
		t.Fatalf("a re-step keeps the first step's window: %+v", w)
	}
	if w := es.takePendingWindow("f", at); w.ok {
		t.Fatal("the take removes the offer")
	}

	es.NoteCallWindow("f", at, first, false, false)
	es.NoteCallWindow("f", at, later, false, true) // no deferred offer: replaced
	if w := es.takePendingWindow("f", at); intOf(w.win[0], 0) != 9 {
		t.Fatalf("a re-step over a settled offer replaces it: %+v", w)
	}
	es.NoteCallWindow("f", at, first, true, false)
	es.NoteCallWindow("f", at, later, true, false) // a fresh first step replaces
	if w := es.takePendingWindow("f", at); intOf(w.win[0], 0) != 9 {
		t.Fatalf("a new first step replaces a deferred offer: %+v", w)
	}

	es.NoteCallWindow("f", at, nil, true, false)
	if w := es.takePendingWindow("f", at); !w.ok || w.known {
		t.Fatalf("a speculative plan offers an unknown window: %+v", w)
	}

	es.NoteCallWindow("f", at, first, false, false)
	resume := es.Suspend()
	es.NoteCallWindow("f", at, later, false, false)
	resume()
	if w := es.takePendingWindow("f", at); w.ok {
		t.Fatalf("a suspended note pools nothing and drops the key's offer: %+v", w)
	}
}

// TestClaimHeldWindow pins the hold: HoldRegion takes the key's window with
// the region, a record under that hold claims it, and a record under
// another key claims from the pool.
func TestClaimHeldWindow(t *testing.T) {
	at := core.SrcPos{Row: 2, Col: 1}
	other := core.SrcPos{Row: 3, Col: 1}
	es := NewEmitState()
	es.NoteCallWindow("f", at, []core.Value{core.NewInteger(1)}, false, false)
	es.NoteCallWindow("g", other, []core.Value{core.NewInteger(2)}, false, false)
	release := es.HoldRegion("f", at)
	defer release()
	if w := es.claimHeldWindow("f", at); !w.ok || intOf(w.win[0], 0) != 1 {
		t.Fatalf("the held window: %+v", w)
	}
	if w := es.claimHeldWindow("g", other); !w.ok || intOf(w.win[0], 0) != 2 {
		t.Fatalf("another key claims from the pool: %+v", w)
	}
}

// TestCallWindowOps pins the record's resolution of a window over a call's
// arguments: an argument by identity, a definite scalar by value, an event
// result of the unit, a stable local read; any other value — or no offer,
// or an unknown window — leaves the call with no window. An empty window is
// a window.
func TestCallWindowOps(t *testing.T) {
	at := core.SrcPos{Row: 1, Col: 1}
	arg := core.NewCarrier(core.TAny)
	arg.ID = "arg"
	ev := core.NewCarrier(core.TAny)
	ev.ID = "ev"
	loc := core.NewCarrier(core.TAny)
	loc.ID = "loc"
	lost := core.NewCarrier(core.TAny)
	lost.ID = "lost"

	es := NewEmitState()
	if es.callWindowOps("f", at, nil) != nil {
		t.Fatal("no offer: no window")
	}
	es.NoteCallWindow("f", at, nil, false, false)
	if es.callWindowOps("f", at, nil) != nil {
		t.Fatal("an unknown window: no window")
	}
	es.NoteCallWindow("f", at, []core.Value{}, false, false)
	if w := es.callWindowOps("f", at, nil); w == nil || len(w) != 0 {
		t.Fatalf("an empty window is a window: %#v", w)
	}

	es.producedBy["ev"] = producer{seq: 4, idx: 1}
	es.units[0].localByID["loc"] = 3
	es.NoteCallWindow("f", at, []core.Value{arg, core.NewInteger(7), ev, core.NewAtom("q")}, false, false)
	w := es.callWindowOps("f", at, []core.Value{arg})
	if len(w) != 4 || w[0].kind != WinArg || w[0].idx != 0 || w[1].kind != WinValue ||
		intOf(w[1].value, 0) != 7 || w[2].kind != WinStack || w[2].op.idx != 4 || w[2].op.resIdx != 1 ||
		w[3].kind != WinValue {
		t.Fatalf("arg, scalar, event, atom: %#v", w)
	}

	// A local read resolves only while its binding has not moved.
	es.NoteCallWindow("f", at, []core.Value{loc}, false, false)
	if es.callWindowOps("f", at, nil) != nil {
		t.Fatal("a local read with no stable binding: no window")
	}
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.Defs.Push("a", loc)
	es.reg = reg
	es.NoteDefRead("loc", "a")
	es.NoteCallWindow("f", at, []core.Value{loc}, false, false)
	if w := es.callWindowOps("f", at, nil); len(w) != 1 || w[0].kind != WinLocal || w[0].idx != 3 {
		t.Fatalf("a stable local read: %#v", w)
	}

	es.NoteCallWindow("f", at, []core.Value{lost}, false, false)
	if es.callWindowOps("f", at, nil) != nil {
		t.Fatal("a value with no home: no window")
	}
	es.NoteCallWindow("f", at, []core.Value{core.NewCarrier(core.TAny)}, false, false)
	if es.callWindowOps("f", at, nil) != nil {
		t.Fatal("a carrier with no identity: no window")
	}

	// Inside a unit, an event the unit did not produce has no home here.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	es.NoteCallWindow("f", at, []core.Value{ev}, false, false)
	if es.callWindowOps("f", at, nil) != nil {
		t.Fatal("an enclosing unit's event: no window")
	}
}

// intOf is v's integer, or -1.
func intOf(v core.Value, _ int) int64 {
	n, err := core.AsInteger(v)
	if err != nil {
		return -1
	}
	return n
}

// TestWindowScalar pins the values a window carries as themselves.
func TestWindowScalar(t *testing.T) {
	dyn := core.NewInteger(1)
	dyn.Dynamic = true
	for _, c := range []struct {
		v    core.Value
		want bool
	}{
		{core.NewInteger(1), true},
		{core.NewString("s"), true},
		{core.NewAtom("a"), true},
		{core.NewCarrier(core.TInteger), false},
		{dyn, false},
		{core.NewTypeLiteral(core.TInteger), false},
		{core.NewList(nil), false},
	} {
		if got := windowScalar(c.v); got != c.want {
			t.Errorf("windowScalar(%v) = %v, want %v", c.v, got, c.want)
		}
	}
	untyped := core.NewInteger(1)
	untyped.Parent = nil
	if windowScalar(untyped) {
		t.Error("a value with no type is no scalar")
	}
}

// TestSeatCallWindow pins the lowering of a window: arguments, scalars and
// locals copy through; an event seats at its promoted slot or at its depth
// beneath the call's operands; an event the layout does not hold seats
// nothing, and nor does a call with no window or no table.
func TestSeatCallWindow(t *testing.T) {
	var table map[int][]CallWindowOperand
	code := []Instr{{Op: OpPushConst}, {Op: OpPushConst}}
	lw := &lowerer{callWindows: &table, code: &code,
		vm:       []vmSlot{{seq: 5, idx: 0}, nonEventSlot, {seq: 9, idx: 0}},
		promoted: map[int]int{7: 10}}
	lw.seatCallWindow(nil, 1)
	if table != nil {
		t.Fatal("no window: no entry")
	}
	lw.seatCallWindow([]callWinOp{
		{kind: WinArg, idx: 1},
		{kind: WinValue, value: core.NewInteger(3)},
		{kind: WinLocal, idx: 2},
		{kind: WinStack, op: EventOperand(7, 1)},
		{kind: WinStack, op: EventOperand(5, 0)},
	}, 1)
	got := table[2]
	want := []CallWindowOperand{{Kind: WinArg, Idx: 1}, {Kind: WinValue, Value: core.NewInteger(3)},
		{Kind: WinLocal, Idx: 2}, {Kind: WinLocal, Idx: 11}, {Kind: WinStack, Idx: 1}}
	if len(got) != len(want) {
		t.Fatalf("entry: %#v", got)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].Idx != want[i].Idx {
			t.Errorf("entry %d: %#v, want %#v", i, got[i], want[i])
		}
	}
	code = append(code, Instr{Op: OpPushConst})
	lw.seatCallWindow([]callWinOp{{kind: WinStack, op: EventOperand(9, 0)}}, 1)
	if _, ok := table[3]; ok {
		t.Fatal("an event among the call's own operands is not beneath them: no entry")
	}
	(&lowerer{}).seatCallWindow([]callWinOp{}, 0) // no table: no-op
	if simDepthBeneath(nil, 0, EventOperand(1, 0)) != -1 {
		t.Fatal("an empty stack holds nothing")
	}
}

// TestNamedFnValueSpec pins NUR235's push flag: a named fn value marks its
// push, making a spec with no contract when it declares none; an anonymous
// one's spec is untouched.
func TestNamedFnValueSpec(t *testing.T) {
	anon := &core.FnDefInfo{Anonymous: true}
	named := &core.FnDefInfo{}
	if namedFnValueSpec(nil, anon) != nil {
		t.Error("an anonymous value with no contract has no spec")
	}
	if s := namedFnValueSpec(nil, named); s == nil || !s.Named || s.Types != nil {
		t.Errorf("a named value with no contract: a Named spec, no types: %+v", s)
	}
	spec := &ClosureRetSpec{Name: "g"}
	if s := namedFnValueSpec(spec, named); s != spec || !s.Named {
		t.Errorf("a named value marks its own spec: %+v", s)
	}
}

// TestClosureCallsAtLanding pins the landing's reading of NUR235's flag: a
// named closure over a nullary unit calls; an anonymous one, one over a
// unit that takes arguments, one from a program not its own, and a value
// that is no closure park; the bridge's anonymity is the unit's lambda
// flavour without a name.
func TestClosureCallsAtLanding(t *testing.T) {
	p := &Program{Fns: []CompiledFn{{Lambda: true}, {Lambda: true, NArgs: 1}}}
	mk := func(unit int, named bool, prog any) core.Value {
		return core.Value{Parent: core.TFunction, Data: core.ClosurePayload{Prog: prog, Unit: unit, Named: named}}
	}
	for name, c := range map[string]struct {
		v    core.Value
		want bool
	}{
		"named nullary":     {mk(0, true, p), true},
		"anonymous":         {mk(0, false, p), false},
		"named with args":   {mk(1, true, p), false},
		"foreign program":   {mk(0, true, "other"), false},
		"unit out of range": {mk(5, true, p), false},
		"not a closure":     {core.NewInteger(1), false},
	} {
		if got := ClosureCallsAtLanding(c.v); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	if !ClosureIsAnonymous(&p.Fns[0], core.ClosurePayload{}) || ClosureIsAnonymous(&p.Fns[0], core.ClosurePayload{Named: true}) || ClosureIsAnonymous(nil, core.ClosurePayload{}) {
		t.Error("anonymous: a lambda unit's closure without a name")
	}
}

// TestCallWindowOpEmptyIdentity pins the record's refusal of a window value
// with no identity that is neither an argument nor a scalar: nothing names
// where the run holds it.
func TestCallWindowOpEmptyIdentity(t *testing.T) {
	es := NewEmitState()
	c := core.NewCarrier(core.TAny)
	c.ID = ""
	if _, ok := es.callWindowOp(c, false, nil); ok {
		t.Fatal("a carrier with no identity has no home")
	}
}
