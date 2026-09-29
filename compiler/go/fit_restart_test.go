package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fit_restart_test.go dials the NUR357 forward-fit planning and seating
// helpers, the NUR352 seated deopt prefix and the NUR351 live split
// directly. The whole-program pins are lang's nur357_forward_fit_test.go.

// fitSig is keys' shape: one Map slot, forward-eligible.
func fitSig() *core.Signature {
	s := &core.Signature{Args: []*core.Type{core.TMap}, BarrierPos: -1}
	core.NormalizeSig(s)
	return s
}

// fitBody is `x end 5 k y`: the statement begins at token 2, the poly `k`
// at column 7 collects y forward.
func fitBody() []core.Value {
	return []core.Value{
		gtok(core.NewWord("x"), 1), gtok(core.NewEnd(), 3), gtok(core.NewInteger(5), 5), gtok(core.NewWord("k"), 7), gtok(core.NewWord("y"), 9),
	}
}

// fitPoly is the poly event seq 10 at column col.
func fitPoly(col int) EmitEvent {
	return EmitEvent{kind: evCall, seq: 10, call: emitCall{word: "k", poly: true, nout: 1, pos: gpos(col)}}
}

// fitState is a root over fitBody with the poly recorded and its operand's
// fits noted.
func fitState(events ...EmitEvent) (*EmitState, *lowerer) {
	es := NewEmitState()
	es.rootBody = fitBody()
	es.frames[0] = append([]EmitEvent{fitPoly(7)}, events...)
	es.fwdFitsAt = map[int]map[int][]core.ForwardFit{10: {0: {{Sig: fitSig(), Idx: 0}}}}
	return es, &lowerer{es: es, dead: map[int]bool{}}
}

func TestPlanFitRestarts(t *testing.T) {
	es, lw := fitState()
	es.planFitRestarts(lw, nil)
	if r := lw.fitRestarts[10]; r == nil || r.token != 2 || r.depth != -1 || r.held != 0 || len(r.substs) != 0 {
		t.Fatalf("the poly's statement island from token 2: %+v", r)
	}
	for _, c := range []struct {
		name string
		prep func(*EmitState) []core.Value
	}{
		{"no fits", func(es *EmitState) []core.Value { es.fwdFitsAt = nil; return nil }},
		{"no body", func(es *EmitState) []core.Value { es.rootBody = nil; return nil }},
		{"a poly past the trap", func(es *EmitState) []core.Value { es.trapAt = 5; return nil }},
		{"a stop with no position", func(es *EmitState) []core.Value { es.frames[0][0].call.pos = core.SrcPos{}; return nil }},
		{"a statement with no start", func(es *EmitState) []core.Value {
			es.rootBody[1], es.rootBody[2] = core.NewEnd(), core.NewInteger(5)
			return nil
		}},
		{"a trap program's untold stack", func(es *EmitState) []core.Value { es.trapAt = 20; return nil }},
		{"a residual literal with no position", func(*EmitState) []core.Value { return []core.Value{core.NewInteger(3)} }},
		{"an effect before the poly", func(es *EmitState) []core.Value {
			es.frames[0] = append([]EmitEvent{{kind: evCall, seq: 8, call: emitCall{word: "print", pos: gpos(5)}}}, es.frames[0]...)
			return nil
		}},
	} {
		es, lw := fitState()
		residual := c.prep(es)
		es.planFitRestarts(lw, residual)
		if lw.fitRestarts != nil {
			t.Errorf("%s: no plan, got %+v", c.name, lw.fitRestarts[10])
		}
	}
	// A trap program's poly before the trap: over the stack the pass told,
	// the values it reads and the runs it writes kept off the dead list.
	es, lw = fitState(EmitEvent{kind: evCallUser, seq: 8, uc: emitUserCall{pos: gpos(6), nout: 1}})
	es.rootBody[2] = gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 6)}), 5)
	es.frames[0][0], es.frames[0][1] = es.frames[0][1], es.frames[0][0]
	held := core.NewInteger(1)
	held.ID = "held"
	es.producedBy["held"] = producer{seq: 2}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{gpos(3): {held}}
	es.trapAt = 20
	lw.dead[2], lw.dead[8] = true, true
	es.planFitRestarts(lw, nil)
	if r := lw.fitRestarts[10]; r == nil || r.held != 1 || len(r.substs) != 1 || lw.dead[2] || lw.dead[8] {
		t.Errorf("a trapped program's island over the told stack: %+v dead=%v", r, lw.dead)
	}
}

func TestPlanUnitFitRestarts(t *testing.T) {
	es := NewEmitState()
	es.fwdFitsAt = map[int]map[int][]core.ForwardFit{10: {0: nil}}
	rec := &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{fitPoly(7)}}, body: fitBody()}
	if n := es.planUnitFitRestarts(es.units[0], rec); n != 1 || !rec.deopts[0].fit || rec.deopts[0].token != 2 {
		t.Fatalf("the unit's poly takes a fit point: %d %+v", n, rec.deopts)
	}
	flw := &lowerer{es: es}
	seatDeoptPoint(flw, rec, rec.deopts[0])
	if r := flw.fitRestarts[10]; r == nil || r.token != 2 || r.held != -1 {
		t.Errorf("seated as the poly's fit island: %+v", r)
	}
	for _, c := range []struct {
		name string
		rec  *fnUnitRec
	}{
		{"a lambda value's body", &fnUnitRec{closure: true, lambdaUnit: true, frag: rec.frag, body: fitBody()}},
		{"no fragment", &fnUnitRec{body: fitBody()}},
		{"a stop with no position", &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{fitPoly(0)}}, body: fitBody()}},
		{"an effect before the poly", &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{{kind: evCall, seq: 8, call: emitCall{word: "print", pos: gpos(5)}}, fitPoly(7)}}, body: fitBody()}},
		{"a statement with no start", &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{fitPoly(7)}}, body: []core.Value{gtok(core.NewWord("x"), 1), core.NewEnd(), core.NewInteger(5), gtok(core.NewWord("k"), 7)}}},
	} {
		if n := es.planUnitFitRestarts(es.units[0], c.rec); n != 0 {
			t.Errorf("%s: no point, got %+v", c.name, c.rec.deopts)
		}
	}
	if n := NewEmitState().planUnitFitRestarts(es.units[0], rec); n != 0 {
		t.Error("no fits noted: no point")
	}
	kept := dropFitPoints([]deoptPoint{{seq: 1, fit: true}, {seq: 2}, {seq: 3, fit: true}})
	if len(kept) != 1 || kept[0].seq != 2 {
		t.Errorf("the fit points dropped, the rest kept: %+v", kept)
	}
}

func TestPolyFitSeating(t *testing.T) {
	es := NewEmitState()
	fits := map[int][]core.ForwardFit{1: {{Sig: fitSig(), Idx: 1, Chosen: true}}, 0: nil}
	es.fwdFitsAt = map[int]map[int][]core.ForwardFit{10: fits}
	calls := map[int]*PolyFit{}
	code := []Instr{{}, {}}
	lw := &lowerer{es: es, landingBody: fitBody(), landingRoot: true, callFits: &calls, code: &code}
	if f := lw.polyFit(10); f != nil {
		t.Error("no plan, no island")
	}
	lw.fitRestarts = map[int]*landingRestart{10: {token: 2, depth: -1, held: -1}}
	if f := lw.polyFit(10); f != nil {
		t.Error("an island the walk never seated is none")
	}
	lw.fitRestarts[10] = &landingRestart{token: 2, depth: 0, held: -1, substs: []substPlan{{path: []int{2}, span: 1, seq: 8}}}
	if f := lw.polyFit(10); f != nil {
		t.Error("a run the island writes that the code holds nowhere: none")
	}
	lw.fitRestarts[10].substs = nil
	f := lw.polyFit(10)
	if f == nil || len(f.At) != 2 || f.At[0] != 0 || f.At[1] != 1 || len(f.Fits[1]) != 1 || len(f.Restart.Island) != 3 || f.Restart.RetPC != -1 {
		t.Fatalf("the island from the statement's first token, the fits by position: %+v", f)
	}
	if len(lw.fitIslands) != 1 || lw.fitIslands[0] != f.Restart {
		t.Error("the island's RetPC is stamped at the finish")
	}
	// A user call seats its island in the target's CallFits at its pc; a
	// tail call takes none and, needing one, declines.
	if why := lw.seatCallFit(10, false); why != "" || calls[2] == nil {
		t.Errorf("seated at the call's pc: %q %v", why, calls)
	}
	if why := lw.seatCallFit(10, true); !strings.Contains(why, "NUR357") {
		t.Errorf("a tail call over a viable fit declines: %q", why)
	}
	delete(lw.fitRestarts, 10)
	if _, why := lw.fitIsland(10); !strings.Contains(why, "NUR357") {
		t.Errorf("no island over a viable fit declines: %q", why)
	}
	if why := lw.seatCallFit(10, false); why == "" {
		t.Error("a user call with no island declines too")
	}
	lw.noteFitServed(deoptPoint{fitConsumer: 10})
	if lw.fitUnserved(10) == "" {
		t.Error("a point with no fits serves nothing")
	}
	lw.noteFitServed(deoptPoint{fitConsumer: 10, fits: fits[1]})
	if lw.fitUnserved(10) != "" {
		t.Error("the read's point serves its collection")
	}
	es.fwdFitsAt[11] = map[int][]core.ForwardFit{0: nil}
	if lw.fitUnserved(11) != "" {
		t.Error("an operand with no viable candidate needs no island")
	}
	if !fitStop(&EmitEvent{kind: evCall, call: emitCall{poly: true}}) || fitStop(&EmitEvent{kind: evCall}) ||
		!fitStop(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 1}}) || fitStop(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 1, generic: true}}) ||
		fitStop(&EmitEvent{kind: evBranch}) {
		t.Error("a poly and a committed user call are stops; nothing else")
	}
}

func TestForwardFitNotes(t *testing.T) {
	es := NewEmitState()
	ev := &EmitEvent{seq: 4}
	es.fwdFits = map[int]map[string][]core.ForwardFit{4: {"y": {{Idx: 0}}}}
	if len(es.readFits(ev, "y", true)) != 1 || es.readFits(ev, "y", false) != nil || es.readFits(nil, "y", true) != nil {
		t.Error("the fits of a read its consumer takes as an operand, nothing else")
	}
	es.noteForwardFits(5, []core.Value{core.NewInteger(1)})
	if es.fwdFits[5] != nil {
		t.Error("no registry bound: nothing noted")
	}
}

func TestRootResidualBeforeSpelled(t *testing.T) {
	es := NewEmitState()
	es.rootBody = []core.Value{gtok(core.NewMap(core.NewOrderedMap()), 1), gtok(core.NewWord("k"), 5)}
	after := core.NewInteger(1)
	after.SetPos(gpos(9))
	if es.rootResidualBefore([]core.Value{after}, gpos(5)) {
		t.Error("a residual entry written after the start is no entry before it")
	}
	if !es.rootResidualBefore([]core.Value{core.NewMap(core.NewOrderedMap())}, gpos(5)) {
		t.Error("a positionless compound a token before the start spells is before it")
	}
	if es.rootResidualBefore([]core.Value{core.NewMap(core.NewOrderedMap())}, gpos(1)) || es.rootResidualBefore([]core.Value{core.NewCarrier(core.TAny)}, gpos(5)) ||
		es.rootResidualBefore([]core.Value{core.NewWord("k")}, gpos(9)) {
		t.Error("not spelled before the start, a carrier, a word: none")
	}
}

func TestSplitLive(t *testing.T) {
	es := NewEmitState()
	live := core.NewInteger(4)
	live.ID = "live"
	l := &core.DispatchLayout{Beneath: []core.Value{live}, Live: []int{0}}
	if _, ok := es.splitLiveProducers(l); ok {
		t.Error("a live value no event produced has no home")
	}
	es.producedBy["live"] = producer{seq: 3}
	if out, ok := es.splitLiveProducers(l); !ok || len(out) != 1 || out[0].prod.seq != 3 {
		t.Errorf("the event result: %+v %v", out, ok)
	}
	es.units = append(es.units, &emitUnit{})
	if _, ok := es.splitLiveProducers(l); ok {
		t.Error("a value produced outside the unit being recorded has no home")
	}

	sp := &PolySplit{Beneath: []core.Value{live, live}}
	lw := &lowerer{promoted: map[int]int{3: 5}, vm: []vmSlot{{seq: 7}, {seq: -1}}, variadic: map[int]bool{}}
	if lw.seatSplitLive(nil, nil) != nil || lw.seatSplitLive(sp, nil) != sp {
		t.Error("no split, or no live entry: as it was")
	}
	got := lw.seatSplitLive(sp, []splitLive{{at: 0, prod: producer{seq: 3}}, {at: 1, prod: producer{seq: 7}}})
	if got == nil || len(got.Live) != 2 || !got.Live[0].Local || got.Live[0].Idx != 5 || got.Live[1].Local || got.Live[1].Idx != 1 {
		t.Fatalf("a promoted result's slot, a stack entry's depth: %+v", got)
	}
	if lw.seatSplitLive(sp, []splitLive{{at: 0, prod: producer{seq: 9}}}) != nil {
		t.Error("a result held nowhere: no split")
	}
	lw.vm = []vmSlot{{seq: 7}, {seq: 8}}
	lw.variadic[8] = true
	if lw.seatSplitLive(sp, []splitLive{{at: 0, prod: producer{seq: 7}}}) != nil {
		t.Error("beneath a runtime-counted region: no static depth")
	}
}

// TestRootReadStatementPointSeats: the statement island of a root read a
// later word consumes seats the residual's entries before the statement
// (rootPreStart, NUR352); an entry it cannot place leaves no island.
func TestRootReadStatementPointSeats(t *testing.T) {
	es := NewEmitState()
	es.rootBody = []core.Value{gtok(core.NewWord("x"), 1), gtok(core.NewEnd(), 3), gtok(core.NewInteger(1), 5), gtok(core.NewWord("y"), 7), gtok(core.NewInteger(7), 9), gtok(core.NewWord("add"), 11)}
	es.frames[0] = []EmitEvent{{kind: evCall, seq: 10, call: emitCall{word: "add", nout: 1, pos: gpos(11), ops: []EmitOperand{localOperand(0)}}}}
	lw := &lowerer{es: es, promoted: map[int]int{4: 0}}
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}, body: es.rootBody}
	r := rootWordRead{name: "y", reads: []core.SrcPos{gpos(7)}}
	d, ok := es.rootReadStatementPoint(lw, rec, r, 4, 0, false, nil)
	if !ok || !d.seated || d.token != 2 || d.seatHeld != 0 || len(d.seat) != 0 {
		t.Fatalf("the statement's island over nothing beneath: %+v %v", d, ok)
	}
	if _, ok := es.rootReadStatementPoint(lw, rec, r, 4, 0, false, []core.Value{core.NewInteger(3)}); ok {
		t.Error("a residual literal with no position is no placeable prefix")
	}
}

// TestSeatedDeoptPoint: a root read's statement island seats its prefix
// where the compiled stack holds exactly the entries it counts; another
// depth leaves the point a guard.
func TestSeatedDeoptPoint(t *testing.T) {
	var table []DeoptSpec
	code := []Instr{}
	debug := []core.SrcPos{}
	lw := &lowerer{deoptTable: &table, code: &code, debug: &debug, promoted: map[int]int{4: 0}}
	d := deoptPoint{seq: 4, slot: -1, name: "x", start: gpos(1), token: 0, seated: true, seatHeld: 0,
		seat: []RestartSrc{{Kind: RestartConst, Val: core.NewInteger(5)}}, fits: []core.ForwardFit{{Idx: 0}}, fitConsumer: 9}
	lw.deopts = []deoptPoint{d}
	lw.emitDeoptsBefore(core.SrcPos{})
	if len(table) != 1 || len(table[0].Seat) != 1 || table[0].Bail || len(table[0].Fits) != 1 || !lw.fitServed[9] {
		t.Fatalf("the seat and the fits ride the spec: %+v", table)
	}
	lw.vm = []vmSlot{{seq: 2}}
	lw.deopts = []deoptPoint{d}
	lw.emitDeoptsBefore(core.SrcPos{})
	if len(table) != 2 || !table[1].Bail || table[1].Seat != nil || table[1].Slot != 0 {
		t.Errorf("another depth: a guard over the read's slot, got %+v", table[1])
	}
}

// The disassembler names a poly's fit island and a read point's fits.
func TestDisasmFitIslands(t *testing.T) {
	p := &Program{
		Code:     []Instr{{Op: OpCallNativePoly, Arg: 0}, {Op: OpDeoptIfFn, Arg: 0}},
		PolyRefs: []PolyRef{{Word: "keys", Arity: 1, Fit: &PolyFit{}}},
		Deopts:   []DeoptSpec{{Name: "y", Fits: []core.ForwardFit{{Idx: 0}}}},
	}
	out := p.Disassemble()
	if !strings.Contains(out, "keys/1 (poly) [fit island]") || !strings.Contains(out, "misses its collection's fit") {
		t.Errorf("both islands named: %s", out)
	}
}
