package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// kept_live_deopt_test.go dials the live-read deopt's planning
// (kept_live_deopt.go) directly: the binding test, where the island starts,
// and the points it declines. The whole-program pins are lang's
// TestLiveDeopt* (nur333_334_live_deopt_test.go).

// LiveHot is the binding test: no binding, a binding a bare read dispatches,
// an active token, or a value off the compiled type is the interpreter's; a
// value of that type, or any value where the statement fixed none, is not.
func TestLiveHotArms(t *testing.T) {
	fnv := core.NewFunction(core.FnDefInfo{Name: "g"})
	splice := core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)}))
	bare := &DeoptSpec{Live: true, Model: core.TInteger}
	ref := &DeoptSpec{Live: true, Ref: true, Model: core.TInteger}
	untyped := &DeoptSpec{Live: true}
	for _, c := range []struct {
		name  string
		spec  *DeoptSpec
		v     core.Value
		bound bool
		want  bool
	}{
		{"no binding", bare, core.Value{}, false, true},
		{"a fn a bare read dispatches", bare, fnv, true, true},
		{"a fn the value spelling delivers, off the type", ref, fnv, true, true},
		{"a fn the value spelling delivers, no type fixed", &DeoptSpec{Live: true, Ref: true}, fnv, true, false},
		{"a splice", untyped, splice, true, true},
		{"a value of the compiled type", bare, core.NewInteger(5), true, false},
		{"a value off it", bare, core.NewString("s"), true, true},
		{"a type node where a value was compiled", bare, core.NewTypeLiteral(core.TInteger), true, true},
		{"any value where no type was fixed", untyped, core.NewString("s"), true, false},
	} {
		if got := c.spec.LiveHot(c.v, c.bound); got != c.want {
			t.Errorf("%s: LiveHot = %v, want %v", c.name, got, c.want)
		}
	}
}

// A read's model is its pre-body carrier's type; a type value fixes none.
func TestNoteKeptLiveReadModel(t *testing.T) {
	es := NewEmitState()
	at := deoptAt(5)
	es.noteKeptLiveRead(3, core.NewCarrier(core.TInteger), "x", at, false)
	es.noteKeptLiveRead(4, core.NewCarrier(core.TType), "y", at, true)
	if r := es.keptLiveReads[3]; r.model != core.TInteger || r.ref || r.name != "x" || r.pos != at {
		t.Errorf("a value's model is its type: %+v", r)
	}
	if r := es.keptLiveReads[4]; r.model != nil || !r.ref {
		t.Errorf("a type value fixes no model: %+v", r)
	}
}

// eventToken finds where each event's own tokens begin: a call at its word,
// a def or a twin at the binding word before the name, a branch at its
// checked condition's `if` or the nearest `if`/`case`, a loop at its word —
// the body's first token when none is found.
func TestEventTokenArms(t *testing.T) {
	body := []core.Value{
		deoptTok("def", 1), deoptTok("z", 5), deoptTok("x", 7), deoptTok("end", 9),
		deoptTok("if", 11), deoptTok("c", 14), deoptTok("for", 16), deoptTok("n", 20), deoptTok("y", 22),
		deoptTok("a", 24), deoptTok("b", 26),
		deoptLit(core.NewList([]core.Value{deoptTok("def", 30), deoptTok("q", 34)}), 28),
	}
	for _, c := range []struct {
		name string
		ev   EmitEvent
		want int
	}{
		{"a call", EmitEvent{kind: evCall, call: emitCall{pos: deoptAt(7)}}, 2},
		{"a def at its name", EmitEvent{kind: evDynBind, dyn: &emitDynBind{pos: deoptAt(5)}}, 0},
		{"a twin at its name", EmitEvent{kind: evBindTwin, twin: &emitBindTwin{pos: deoptAt(5)}}, 0},
		{"a twin at the first token", EmitEvent{kind: evBindTwin, twin: &emitBindTwin{pos: deoptAt(1)}}, 0},
		{"a checked branch", EmitEvent{kind: evBranch, br: &emitBranch{pos: deoptAt(14), condCheckPos: deoptAt(11)}}, 4},
		{"an unchecked branch", EmitEvent{kind: evBranch, br: &emitBranch{pos: deoptAt(14)}}, 4},
		{"a branch with no word near", EmitEvent{kind: evBranch, br: &emitBranch{pos: deoptAt(26)}}, 0},
		{"a loop", EmitEvent{kind: evLoop, loop: &emitLoop{pos: deoptAt(20)}}, 6},
		{"a def nested in a list", EmitEvent{kind: evDynBind, dyn: &emitDynBind{pos: deoptAt(34)}}, 11},
		{"a user call", EmitEvent{kind: evCallUser, uc: emitUserCall{pos: deoptAt(14)}}, 5},
		{"a store, known to stand nowhere in particular", EmitEvent{kind: evStore, store: &emitStore{pos: deoptAt(14)}}, 0},
	} {
		if got := eventToken(body, &c.ev); got != c.want {
			t.Errorf("%s: eventToken = %d, want %d", c.name, got, c.want)
		}
	}
}

// An event with no position lowered between the test and the read may be
// one the island's tokens do not hold: the point declines.
func TestUnplacedBeforeRead(t *testing.T) {
	live := EmitEvent{seq: 5, kind: evCall, call: emitCall{live: true, pos: deoptAt(9)}}
	unplaced := EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "g"}}
	placed := EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "g", pos: deoptAt(5)}}
	if !unplacedBeforeRead([]EmitEvent{unplaced, live}, 0, 5) {
		t.Error("an unplaced event before the read, after the test, declines")
	}
	if unplacedBeforeRead([]EmitEvent{placed, live}, 0, 5) || unplacedBeforeRead([]EmitEvent{unplaced, live}, 1, 5) {
		t.Error("a placed event, or one lowered before the test, is no bar")
	}
}

// A read whose position no body token holds (a token the source never
// wrote there) has no statement to start at: no point.
func TestLivePointAtNoStatement(t *testing.T) {
	es, u, rec, _ := deoptUnit(t, []core.Value{deoptTok("t", 43)}, 43)
	es.keptLiveReads = map[int]keptLiveRead{5: {id: "live-t", name: "t"}}
	if _, ok := es.livePointAt(u, rec, 5, -1, false, nil); ok {
		t.Error("a read at no position places no point")
	}
}

// Beside a gradual read's point (NUR123) whose islands a live point breaks —
// the live point's later token leaves a fn def between the two to the
// island environment, which cannot bind it — the unit keeps its own point
// and drops the live one; the fn def is one the remaining island makes.
func TestPlanDeoptsDropsUnservedLivePoints(t *testing.T) {
	w := EmitEvent{seq: 4, kind: evDynBind, dyn: &emitDynBind{name: "w", srcSeq: -1, val: core.NewCarrier(core.TFunction), pos: deoptAt(48)}}
	live := EmitEvent{seq: 5, kind: evCall, call: emitCall{word: "t", nout: 1, pos: deoptAt(55), live: true}}
	es, u, rec, _ := deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45), deoptTok("def", 47), deoptTok("w", 48), deoptTok("t", 55), deoptTok("w", 57)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}},
		w, live)
	es.keptLiveReads = map[int]keptLiveRead{5: {id: "live-t", name: "t", pos: deoptAt(55)}}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].live != nil || !rec.deoptEnv {
		t.Fatalf("the unit keeps its gradual read's point alone: %+v env=%v", rec.deopts, rec.deoptEnv)
	}
	if !rec.frag.events[3].dyn.islandMade {
		t.Error("the fn def after the kept point's token is one its island makes")
	}
	// The live point alone is planned where nothing breaks it.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("t", 55)}, 43, live)
	es.keptLiveReads = map[int]keptLiveRead{5: {id: "live-t", name: "t", pos: deoptAt(55)}}
	if n := es.planKeptLiveDeopts(u, rec); n != 1 || rec.deopts[0].live == nil || rec.deopts[0].token != 3 {
		t.Errorf("a live read placed at its own token: %d %+v", n, rec.deopts)
	}
}

// A root point's prefix is the compiled stack: every residual entry before
// its statement must be held there, not in a slot or as a constant.
func TestRootHeldBeneath(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, promoted: map[int]int{4: 7}}
	es.producedBy["held"] = producer{seq: 3}
	es.producedBy["slot"] = producer{seq: 4}
	start := core.SrcPos{Row: 2, Col: 1}
	for _, c := range []struct {
		name string
		id   string
		pos  core.SrcPos
		want bool
	}{
		{"an event's result on the stack", "held", core.SrcPos{}, true},
		{"one promoted to a slot", "slot", core.SrcPos{}, false},
		{"a literal laid out at the end", "k", core.SrcPos{Row: 1, Col: 1}, false},
		{"a literal with no position", "np", core.SrcPos{}, false},
	} {
		v := core.NewInteger(1)
		v.ID = c.id
		v.SetPos(c.pos)
		if got := es.rootHeldBeneath(lw, nil, []core.Value{v}, start, 8); got != c.want {
			t.Errorf("%s: rootHeldBeneath = %v, want %v", c.name, got, c.want)
		}
	}
}

// The disassembler names a live-read point by its binding.
func TestDisasmLivePoint(t *testing.T) {
	p := &Program{Code: []Instr{{Op: OpDeoptIfFn, Arg: 0}}, Deopts: []DeoptSpec{{Name: "x", Live: true}}}
	if got := p.Disassemble(); !strings.Contains(got, "if the live binding of x is not the compiled statement's") {
		t.Errorf("the live point's disassembly: %s", got)
	}
}
