package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// deopt_test.go pins planDeopts / deoptPointFor (NUR123's per-read deopt):
// where each statement shape begins, which reads qualify, and which points
// decline as best effort.

func deoptAt(col int) core.SrcPos { return core.SrcPos{Row: 1, Col: col} }

func deoptTok(name string, col int) core.Value {
	w := core.NewWord(name)
	w.SetPos(deoptAt(col))
	return w
}

func deoptLit(v core.Value, col int) core.Value {
	v.SetPos(deoptAt(col))
	return v
}

// deoptUnit opens a user-fn unit over a Map param `m` whose body begins
// `def j (m get "f")` — the get event (seq 1) producing j's value, the def
// binding it (seq 2) — followed by rest, with j read bare at readCol.
func deoptUnit(t *testing.T, tail []core.Value, readCol int, rest ...EmitEvent) (*EmitState, *emitUnit, *fnUnitRec, core.Value) {
	t.Helper()
	es := NewEmitState()
	m := core.NewCarrier(core.TMap)
	unit, _, ok := es.StartFnCompile("k", "h", nil, []core.Value{m}, []*core.Type{core.TAny}, []string{"m"}, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	u := es.units[len(es.units)-1]
	rec := es.fnRecs[unit]
	j := core.NewDynamicCarrier(core.TAny)
	es.producedBy[j.ID] = producer{seq: 1}
	events := []EmitEvent{
		{seq: 1, kind: evCall, call: emitCall{word: "get", nout: 1, pos: deoptAt(33), ops: []EmitOperand{localOperand(0), ConstOperand(es.internUnpooled(core.NewString("f")))}}},
		{seq: 2, kind: evDynBind, dyn: &emitDynBind{name: "j", srcSeq: 1, pos: deoptAt(28)}},
	}
	rec.frag = &EmitFragment{events: append(events, rest...)}
	body := []core.Value{deoptTok("def", 24), deoptTok("j", 28), deoptLit(core.NewList([]core.Value{deoptTok("m", 31), deoptTok("get", 33)}), 30)}
	es.SetUnitBody(unit, append(body, tail...))
	es.NoteWordRead(j, "j", deoptAt(readCol))
	es.NoteLocalRead(j.ID, deoptAt(readCol))
	return es, u, rec, j
}

func TestPlanDeoptsShapes(t *testing.T) {
	// `j typeof`: the consumer follows the read on the stack — tested at
	// the read's push (atPush), the statement starting at the read.
	es, u, rec, _ := deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 || !rec.deoptEnv || !u.deoptEnv {
		t.Errorf("a stack consumer tests at the read's push: %+v env=%v", rec.deopts, rec.deoptEnv)
	}
	// `def k j`: a def sourcing the value THROUGH the read consumes it, and
	// its statement begins at the `def` word before the name.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("def", 43), deoptTok("k", 47), deoptTok("j", 49)}, 49,
		EmitEvent{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "k", srcSeq: 1, pos: deoptAt(47)}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a def consumer starts at its word: %+v", rec.deopts)
	}
	// `if true [j] [0]`: a read inside an arm — the statement begins at the
	// `if` word before the arm.
	arm := deoptLit(core.NewList([]core.Value{deoptTok("j", 52)}), 51)
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("if", 43), deoptTok("true", 46), arm, deoptLit(core.NewList([]core.Value{core.NewInteger(0)}), 55)}, 52,
		EmitEvent{seq: 3, kind: evBranch, br: &emitBranch{cond: localOperand(0), then: &EmitFragment{}, thenOut: EventOperand(1, 0), hasThenOut: true, els: &EmitFragment{}}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a branch consumer starts at its if: %+v", rec.deopts)
	}
	// `{a: j}`: a read inside a literal — the literal's own token.
	lit := deoptLit(core.NewMap(core.NewOrderedMap()), 45)
	es, u, rec, _ = deoptUnit(t, []core.Value{lit}, 49,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "{}", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}, makeMap: true}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 45 || rec.deopts[0].token != 3 {
		t.Errorf("a literal consumer starts at the literal: %+v", rec.deopts)
	}
	// A residual read an event follows (`j  def y 1`): the statement begins
	// at the read, tested before that event.
	one := deoptLit(core.NewInteger(1), 51)
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("def", 45), deoptTok("y", 49), one}, 43,
		EmitEvent{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1, val: one, pos: deoptAt(49)}})
	rec.outOps = []EmitOperand{EventOperand(1, 0)}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a residual read an event follows starts at the read: %+v", rec.deopts)
	}
}

func TestPlanDeoptsDeclines(t *testing.T) {
	// A def the island reads whose source is no single-output call (a
	// variadic producer) cannot bind registry-visibly: the points decline.
	es, u, rec, _ := deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}})
	es.eventInfo[1] = eventFlags{variadicResult: true}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 || rec.deoptEnv {
		t.Errorf("a def with no re-pushable source declines the unit's points: %+v", rec.deopts)
	}
	// A def the island reads of a FN value (`def w fn […]  … w`): no
	// const or local re-pushes it, so the unit would decline at its bind —
	// the points decline; a def of an inert literal binds.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45), deoptTok("w", 52)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}},
		EmitEvent{seq: 4, kind: evDynBind, dyn: &emitDynBind{name: "w", srcSeq: -1, val: core.NewCarrier(core.TFunction), pos: deoptAt(48)}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 || rec.deoptEnv {
		t.Errorf("a def of a fn value the island spells declines: %+v", rec.deopts)
	}
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45), deoptTok("w", 52)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}},
		EmitEvent{seq: 4, kind: evDynBind, dyn: &emitDynBind{name: "w", srcSeq: -1, val: core.NewInteger(7), pos: deoptAt(48)}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !rec.deoptEnv {
		t.Errorf("a def of an inert literal binds: %+v", rec.deopts)
	}
	// A def the island reads made inside a branch arm declines too.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45), deoptTok("y", 52)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}})
	rec.frag.events = append([]EmitEvent{{seq: 0, kind: evBranch, br: &emitBranch{cond: localOperand(0), then: &EmitFragment{events: []EmitEvent{{seq: 9, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1, pos: deoptAt(20)}}}}, els: &EmitFragment{}}}}, rec.frag.events...)
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a def bound inside an arm declines: %+v", rec.deopts)
	}

	// A residual read the replay's word island seats needs no deopt.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43)}, 43)
	rec.dynFrameWords = []DynFrameWord{{Name: "j"}}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 || rec.deoptEnv {
		t.Errorf("a seated residual read declines: %+v", rec.deopts)
	}
	// A lone residual read no island seats tests at its push.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43)}, 43)
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !rec.deopts[0].atPush {
		t.Errorf("an unseated residual read tests at its push: %+v", rec.deopts)
	}
	// A literal of a LATER event written before the start (`5 j typeof
	// add`): the compiled stack lacks it at the test — decline.
	five := deoptLit(core.NewInteger(5), 43)
	es, u, rec, _ = deoptUnit(t, []core.Value{five, deoptTok("j", 45), deoptTok("typeof", 47), deoptTok("add", 54)}, 45,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(47), ops: []EmitOperand{EventOperand(1, 0)}}})
	rec.frag.events = append(rec.frag.events, EmitEvent{seq: 4, kind: evCall, call: emitCall{word: "add", nout: 1, pos: deoptAt(54), ops: []EmitOperand{ConstOperand(es.internUnpooled(five)), EventOperand(3, 0)}}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a deferred literal declines the point: %+v", rec.deopts)
	}
	// A param read before the start and consumed after it (`x {a: j} add`
	// with x read once, consumed once, after): declines; consumed BEFORE
	// the start as often as read, it does not (`x drop {a: j}`).
	lit := deoptLit(core.NewMap(core.NewOrderedMap()), 47)
	es, u, rec, j := deoptUnit(t, []core.Value{deoptTok("x", 45), lit, deoptTok("add", 54)}, 51,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "{}", nout: 1, pos: deoptAt(47), ops: []EmitOperand{EventOperand(1, 0)}, makeMap: true}},
		EmitEvent{seq: 4, kind: evCall, call: emitCall{word: "add", nout: 1, pos: deoptAt(54), ops: []EmitOperand{localOperand(0), EventOperand(3, 0)}}})
	_ = j
	var mID string
	for id, slot := range u.localByID {
		if slot == 0 {
			mID = id
		}
	}
	es.NoteLocalRead(mID, deoptAt(31))
	es.NoteLocalRead(mID, deoptAt(45))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a deferred param read declines the point: %+v", rec.deopts)
	}
	// A closure unit and a unit without a body plan nothing.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}})
	rec.body = nil
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Error("no body: nothing to hand to the interpreter")
	}
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45)}, 43,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}})
	// A lambda value's unit plans from its OWN frame (the fourteenth
	// increment): the factory's frame is gone by apply time, but this unit's
	// params and captures ride in slots for the whole call, so the point
	// stands and lambdaDeopt marks emitDynParamBinds to bind the captures
	// too. Here every name the island spells is the unit's own — the param
	// `m` and the def `j` the fragment itself makes. A code body's unit
	// (each, do) plans like a fn unit, through seedParentDeopt instead.
	rec.closure, rec.lambdaUnit = true, true
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !rec.lambdaDeopt {
		t.Errorf("a lambda unit plans from its own frame: %+v lambdaDeopt=%v", rec.deopts, rec.lambdaDeopt)
	}
	rec.deopts, rec.lambdaDeopt = nil, false
	rec.lambdaUnit, rec.storedRefUnit = false, true
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !rec.lambdaDeopt {
		t.Errorf("a stored fn's unit takes the same own-frame route: %+v", rec.deopts)
	}
	rec.deopts, rec.lambdaDeopt = nil, false
	rec.closure, rec.storedRefUnit = false, false
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.lambdaDeopt {
		t.Errorf("a code body's unit plans like a fn unit: %+v", rec.deopts)
	}
}

// planDeoptsEnv's LAMBDA arm: a lambda seeded by a closure child (the child's
// island reads a name from this lambda's frame) binds from its own slots and
// is marked lambdaDeopt, so emitDynParamBinds covers its captures too. When a
// seeded name is one only an ENCLOSING unit supplies — gone with the factory's
// frame by apply time — the seeding is withdrawn instead: the child's points
// drop and both units keep their slot pushes.
func TestPlanDeoptsEnvLambdaArm(t *testing.T) {
	mk := func(seeded string) (*EmitState, *emitUnit, *fnUnitRec, *fnUnitRec) {
		es := NewEmitState()
		outer := &fnUnitRec{name: "h", nParams: 1, locals: []string{"m"}, rootFrame: 0, frag: &EmitFragment{}}
		lam := &fnUnitRec{name: "fnval$body", nParams: 1, caps: []core.CapturedBinding{{Name: "j"}},
			locals: []string{"x", "j"}, closure: true, lambdaUnit: true, frag: &EmitFragment{},
			deoptNames: map[string]bool{seeded: true}}
		kid := &fnUnitRec{name: "do$body", deopts: []deoptPoint{{name: seeded}}, deoptEnv: true,
			deoptNames: map[string]bool{seeded: true}, frag: &EmitFragment{}}
		es.fnRecs = append(es.fnRecs, outer, lam, kid)
		lam.deoptChildren = []int{2}
		es.frames = [][]EmitEvent{{}}
		es.openUnitRecs = []int{0, 1}
		return es, &emitUnit{}, lam, kid
	}

	// `j` is the lambda's own capture: the seeding stands.
	es, u, lam, kid := mk("j")
	es.planDeoptsEnv(u, lam)
	if !lam.deoptEnv || !lam.lambdaDeopt || len(kid.deopts) != 1 {
		t.Errorf("a servable seeded name keeps the child's points: env=%v lambdaDeopt=%v kid=%d",
			lam.deoptEnv, lam.lambdaDeopt, len(kid.deopts))
	}
	if !u.deoptEnv || len(u.deoptNames) != 1 {
		t.Errorf("the unit carries the island's names: %v %v", u.deoptEnv, u.deoptNames)
	}

	// `m` is the ENCLOSING unit's param, which the lambda did not capture:
	// the factory frame that holds it is gone by apply time.
	es, u, lam, kid = mk("m")
	es.planDeoptsEnv(u, lam)
	if lam.deoptEnv || lam.lambdaDeopt || len(kid.deopts) != 0 || kid.deoptEnv {
		t.Errorf("an unservable seeded name withdraws the seeding: env=%v lambdaDeopt=%v kid=%d/%v",
			lam.deoptEnv, lam.lambdaDeopt, len(kid.deopts), kid.deoptEnv)
	}
}

// lambdaNamesSelfBound: a lambda serves its islands from its own frame, so
// the ONE name it cannot serve is one an ENCLOSING unit supplies and it did
// not capture — that binding dies with the factory's frame. A bare word the
// registry does not resolve (`true`) needs no binding and must not decline
// the point; that over-strict registry test was the `if true [j] [0]`
// witness.
func TestLambdaNamesSelfBound(t *testing.T) {
	es := NewEmitState()
	parent := &fnUnitRec{name: "h", nParams: 1, locals: []string{"m"}, rootFrame: 0,
		frag: &EmitFragment{}}
	child := &fnUnitRec{name: "fnval$body", nParams: 1, caps: []core.CapturedBinding{{Name: "j"}},
		locals: []string{"x", "j"},
		frag: &EmitFragment{events: []EmitEvent{
			{seq: 9, kind: evDynBind, dyn: &emitDynBind{name: "k"}},
		}}}
	es.fnRecs = append(es.fnRecs, parent, child)
	es.frames = [][]EmitEvent{{
		{seq: 1, kind: evDynBind, dyn: &emitDynBind{name: "outer"}},
	}}
	es.openUnitRecs = []int{0, 1}

	cases := []struct {
		name  string
		names map[string]bool
		want  bool
	}{
		{"its own param and capture", map[string]bool{"x": true, "j": true}, true},
		{"a def the island itself makes", map[string]bool{"k": true}, true},
		{"a native the registry resolves", map[string]bool{"typeof": true}, true},
		{"a bare literal-word the registry does not", map[string]bool{"true": true}, true},
		{"an ENCLOSING unit's param", map[string]bool{"m": true}, false},
		{"an ENCLOSING unit's frame def", map[string]bool{"outer": true}, false},
	}
	for _, c := range cases {
		if got := es.lambdaNamesSelfBound(child, c.names); got != c.want {
			t.Errorf("%s: lambdaNamesSelfBound = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBodyTokenHelpers(t *testing.T) {
	body := []core.Value{deoptTok("a", 1), deoptTok("b", 5), core.NewInteger(3)}
	if bodyTokenAt(body, deoptAt(5)) != 1 || bodyTokenAt(body, deoptAt(6)) != -1 || bodyTokenAt(body, core.SrcPos{}) != -1 {
		t.Error("bodyTokenAt matches an exact position only")
	}
	if bodyTokenContaining(body, deoptAt(7)) != 1 || bodyTokenContaining(body, deoptAt(0)) != -1 {
		t.Error("bodyTokenContaining takes the last token at or before the position")
	}
	if p := eventPos(EmitEvent{kind: evBreak}); p.Row != 0 || p.Col != 0 {
		t.Error("a break event stands nowhere")
	}
	es := NewEmitState()
	es.SetUnitBody(-1, body)
	es.SetUnitBody(0, body)
	if len(es.fnRecs) != 0 {
		t.Error("an out-of-range unit records no body")
	}
}

func deoptParen(col int, items ...core.Value) core.Value {
	return deoptLit(core.NewParenExpr(items), col)
}

func deoptCall(seq int, word string, col int, ops ...EmitOperand) EmitEvent {
	return EmitEvent{seq: seq, kind: evCall, call: emitCall{word: word, nout: 1, pos: deoptAt(col), ops: ops}}
}

// TestPlanDeoptsNestedReads pins the statement of a read nested in a
// paren, a literal or a loop body: the top-level token holding it when
// its consumer is inside that token or after it, the consuming word when
// that word collects the token forward, the def word of a `def k (j)`,
// and — for a bare read — the read itself, off the push, when an event
// runs between the read and its stack consumer.
func TestPlanDeoptsNestedReads(t *testing.T) {
	// `(j typeof)`: the consumer inside the paren — the paren's token.
	es, u, rec, _ := deoptUnit(t, []core.Value{deoptParen(43, deoptTok("j", 44), deoptTok("typeof", 46))}, 44,
		deoptCall(3, "typeof", 46, EventOperand(1, 0)))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a paren-nested read starts at the paren: %+v", rec.deopts)
	}
	// `(j) typeof`: the consumer after the paren — still the paren's token.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptParen(43, deoptTok("j", 44)), deoptTok("typeof", 47)}, 44,
		deoptCall(3, "typeof", 47, EventOperand(1, 0)))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a word after the paren starts at the paren: %+v", rec.deopts)
	}
	// `typeof (j)`: a forward word collecting the paren — the word.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("typeof", 43), deoptParen(50, deoptTok("j", 51))}, 51,
		deoptCall(3, "typeof", 43, EventOperand(1, 0)))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a forward word before the paren starts at the word: %+v", rec.deopts)
	}
	// `[(j typeof)]`: a paren inside a literal — the literal's token; the
	// literal's own maker follows the consumer and defers nothing.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptLit(core.NewList([]core.Value{deoptParen(44, deoptTok("j", 45), deoptTok("typeof", 47))}), 43)}, 45,
		deoptCall(3, "typeof", 47, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events, EmitEvent{seq: 4, kind: evCall, call: emitCall{word: "[]", nout: 1, pos: deoptAt(43), ops: []EmitOperand{EventOperand(3, 0)}, makeList: true}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a paren inside a literal starts at the literal: %+v", rec.deopts)
	}
	// `for 2 [j]`: a loop event stands at its count — the statement begins
	// at the `for` word before the body.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("for", 43), deoptLit(core.NewInteger(2), 47), deoptLit(core.NewList([]core.Value{deoptTok("j", 50)}), 49)}, 50)
	rec.frag.events = append(rec.frag.events, EmitEvent{seq: 3, kind: evLoop, loop: &emitLoop{
		start: ConstOperand(es.internUnpooled(core.NewInteger(0))), end: ConstOperand(es.internUnpooled(deoptLit(core.NewInteger(2), 47))), step: ConstOperand(es.internUnpooled(core.NewInteger(1))),
		body: &EmitFragment{}, bodyOut: EventOperand(1, 0), hasBodyOut: true, iterSlot: -1, pos: deoptAt(47)}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a loop-body read starts at the for word: %+v", rec.deopts)
	}
	// `def k (j)`: a def whose next token HOLDS the read consumes it — the
	// statement begins at the def word.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("def", 43), deoptTok("k", 47), deoptParen(49, deoptTok("j", 50))}, 50,
		EmitEvent{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "k", srcSeq: 1, pos: deoptAt(47)}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("a def holding the read in its source starts at its word: %+v", rec.deopts)
	}
	// `j (m get "g") add`: the get runs before the read's push — tested
	// before that event, at the read, where the stack is the read's.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptParen(45, deoptTok("m", 46), deoptTok("get", 48), deoptLit(core.NewString("g"), 52)), deoptTok("add", 57)}, 43)
	rec.frag.events = append(rec.frag.events,
		deoptCall(3, "get", 48, localOperand(0), ConstOperand(es.internUnpooled(core.NewString("g")))),
		deoptCall(4, "add", 57, EventOperand(1, 0), EventOperand(3, 0)))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 43 || rec.deopts[0].token != 3 {
		t.Errorf("an event between the read and its consumer moves the test off the push: %+v", rec.deopts)
	}
}

// TestPlanDeoptsAccounting pins deoptDeferred's operand classes: a
// def-bound local, a def's literal, a compound literal, an event result
// (def-bound or not), the residual's inert operands and the consumer's
// own operands — each admitted where the compiled stack holds the value
// at the test and declined where it does not.
func TestPlanDeoptsAccounting(t *testing.T) {
	// A later event reading the produced value's own frame local (its
	// source seated in a slot): the def binding it is its consumption.
	es, u, rec, j := deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45), deoptTok("j", 52), deoptTok("add", 54)}, 43,
		deoptCall(3, "typeof", 45, EventOperand(1, 0)),
		deoptCall(4, "add", 54, localOperand(1), EventOperand(3, 0)))
	u.localByID[j.ID] = 1
	es.NoteLocalRead(j.ID, deoptAt(52))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 {
		t.Errorf("a def-bound local read after the start is not deferred: %+v", rec.deopts)
	}
	// A literal a later event reads through a def (`def y 5 … j typeof  y
	// add`): read after the start, nothing is deferred; read before it too
	// (`… y  j typeof  y add`), the compiled stack lacks it — decline.
	five := deoptLit(core.NewInteger(5), 49)
	five.ID = "lit-five"
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("def", 43), deoptTok("y", 47), five, deoptTok("j", 52), deoptTok("typeof", 54), deoptTok("y", 61), deoptTok("add", 63)}, 52,
		EmitEvent{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1, val: five, pos: deoptAt(47)}},
		deoptCall(4, "typeof", 54, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events, deoptCall(5, "add", 63, ConstOperand(es.internUnpooled(five)), EventOperand(4, 0)))
	es.NoteLocalRead(five.ID, deoptAt(61))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !rec.deopts[0].atPush {
		t.Errorf("a def's literal read after the start is not deferred: %+v", rec.deopts)
	}
	five = deoptLit(core.NewInteger(5), 49)
	five.ID = "lit-five"
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("def", 43), deoptTok("y", 47), five, deoptTok("y", 51), deoptTok("j", 53), deoptTok("typeof", 55), deoptTok("y", 62), deoptTok("add", 64)}, 53,
		EmitEvent{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1, val: five, pos: deoptAt(47)}},
		deoptCall(4, "typeof", 55, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events, deoptCall(5, "add", 64, ConstOperand(es.internUnpooled(five)), EventOperand(4, 0)))
	es.NoteLocalRead(five.ID, deoptAt(51))
	es.NoteLocalRead(five.ID, deoptAt(62))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a def's literal read before the start declines: %+v", rec.deopts)
	}
	// A compound literal a later event consumes: written after the start
	// it is pushed there; written before, the compiled stack lacks it.
	lst := deoptLit(core.NewList([]core.Value{core.NewInteger(1)}), 52)
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45), lst, deoptTok("size", 56)}, 43, deoptCall(3, "typeof", 45, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events, deoptCall(4, "size", 56, ConstOperand(es.internUnpooled(lst))))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 {
		t.Errorf("a compound literal after the start is not deferred: %+v", rec.deopts)
	}
	lst = deoptLit(core.NewList([]core.Value{core.NewInteger(1)}), 43)
	es, u, rec, _ = deoptUnit(t, []core.Value{lst, deoptTok("j", 49), deoptTok("typeof", 51), deoptTok("size", 58)}, 49, deoptCall(3, "typeof", 51, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events, deoptCall(4, "size", 58, ConstOperand(es.internUnpooled(lst))))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a compound literal before the start declines: %+v", rec.deopts)
	}
	// The fold re-mints a compound const without its position (`[1 2]  (j
	// typeof)  drop` pushes it fresh, last of all): counted by canon
	// against the body's compound tokens before the start — decline; the
	// same const with its token after the start is not deferred.
	es, u, rec, _ = deoptUnit(t, []core.Value{lst, deoptParen(49, deoptTok("j", 50), deoptTok("typeof", 52)), deoptTok("drop", 58)}, 50, deoptCall(3, "typeof", 52, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events, deoptCall(4, "drop", 58, EventOperand(3, 0)))
	rec.outOps = []EmitOperand{ConstOperand(es.internUnpooled(core.NewList([]core.Value{core.NewInteger(1)})))}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a re-minted compound written before the start declines: %+v", rec.deopts)
	}
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptParen(43, deoptTok("j", 44), deoptTok("typeof", 46)), deoptLit(core.NewList([]core.Value{core.NewInteger(1)}), 52)}, 44, deoptCall(3, "typeof", 46, EventOperand(1, 0)))
	rec.outOps = []EmitOperand{EventOperand(3, 0), ConstOperand(es.internUnpooled(core.NewList([]core.Value{core.NewInteger(1)})))}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 {
		t.Errorf("a re-minted compound written after the start lands after the island: %+v", rec.deopts)
	}
	// An event result no def consumed, produced before a residual read and
	// consumed by an event after it: the island's stack lacks it — decline.
	// Def-bound (`def n (m size)  j  n typeof`), the island reads it by name.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptParen(43, deoptTok("m", 44), deoptTok("size", 46)), deoptTok("j", 52), deoptTok("typeof", 54)}, 52,
		deoptCall(3, "size", 46, localOperand(0)), deoptCall(4, "typeof", 54, EventOperand(3, 0)))
	rec.outOps = []EmitOperand{EventOperand(1, 0), EventOperand(4, 0)}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("an unbound event result before the read declines: %+v", rec.deopts)
	}
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("def", 36), deoptTok("n", 40), deoptParen(42, deoptTok("m", 43), deoptTok("size", 45)), deoptTok("j", 52), deoptTok("n", 54), deoptTok("typeof", 56)}, 52,
		deoptCall(3, "size", 45, localOperand(0)),
		EmitEvent{seq: 4, kind: evDynBind, dyn: &emitDynBind{name: "n", srcSeq: 3, pos: deoptAt(40)}},
		deoptCall(5, "typeof", 56, EventOperand(3, 0)))
	rec.outOps = []EmitOperand{EventOperand(1, 0), EventOperand(5, 0)}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].atPush || rec.deopts[0].start.Col != 52 {
		t.Errorf("a def-bound result is read by name in the island: %+v", rec.deopts)
	}
	// The residual's own inert operand written before the start (`5 j
	// typeof`) is pushed last of all — decline; written after it (`j
	// typeof 5`), it lands after the island.
	five = deoptLit(core.NewInteger(5), 43)
	es, u, rec, _ = deoptUnit(t, []core.Value{five, deoptTok("j", 45), deoptTok("typeof", 47)}, 45, deoptCall(3, "typeof", 47, EventOperand(1, 0)))
	rec.outOps = []EmitOperand{ConstOperand(es.internUnpooled(five)), EventOperand(3, 0)}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a residual literal before the start declines: %+v", rec.deopts)
	}
	five = deoptLit(core.NewInteger(5), 52)
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45), five}, 43, deoptCall(3, "typeof", 45, EventOperand(1, 0)))
	rec.outOps = []EmitOperand{EventOperand(3, 0), ConstOperand(es.internUnpooled(five))}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 {
		t.Errorf("a residual literal after the start lands after the island: %+v", rec.deopts)
	}
	// A def of a literal consumed its literal (`def y 5  j typeof  5 add`):
	// the later `5` is the pooled const's own token, pushed after the start.
	five = deoptLit(core.NewInteger(5), 49)
	five.ID = "lit-five"
	five2 := deoptLit(core.NewInteger(5), 61)
	five2.ID = "lit-five-2"
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("def", 43), deoptTok("y", 47), five, deoptTok("j", 52), deoptTok("typeof", 54), five2, deoptTok("add", 63)}, 52,
		EmitEvent{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1, val: five, pos: deoptAt(47)}},
		deoptCall(4, "typeof", 54, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events, deoptCall(5, "add", 63, ConstOperand(es.internUnpooled(five2)), EventOperand(4, 0)))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 {
		t.Errorf("a literal a def consumed is not deferred: %+v", rec.deopts)
	}
	// A point tested before its consumer (`5 (j) add`): the consumer's own
	// literal written before the paren is deferred — decline; after the
	// paren (`(j) 5 add`) the island pushes it.
	five = deoptLit(core.NewInteger(5), 43)
	es, u, rec, _ = deoptUnit(t, []core.Value{five, deoptParen(45, deoptTok("j", 46)), deoptTok("add", 49)}, 46)
	rec.frag.events = append(rec.frag.events, deoptCall(3, "add", 49, ConstOperand(es.internUnpooled(five)), EventOperand(1, 0)))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a consumer's literal before the start declines: %+v", rec.deopts)
	}
	five = deoptLit(core.NewInteger(5), 47)
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptParen(43, deoptTok("j", 44)), five, deoptTok("add", 49)}, 44)
	rec.frag.events = append(rec.frag.events, deoptCall(3, "add", 49, EventOperand(1, 0), ConstOperand(es.internUnpooled(five))))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].start.Col != 43 {
		t.Errorf("a consumer's literal after the start is the island's to push: %+v", rec.deopts)
	}
}

// TestPlanDeoptsStartDeclines pins the starts no body token can anchor.
func TestPlanDeoptsStartDeclines(t *testing.T) {
	// A branch consumer over a read no body token contains.
	arm := deoptLit(core.NewList([]core.Value{deoptTok("j", 52)}), 51)
	es, u, rec, _ := deoptUnit(t, []core.Value{deoptTok("if", 43), deoptTok("true", 46), arm, deoptLit(core.NewList([]core.Value{core.NewInteger(0)}), 55)}, 0,
		EmitEvent{seq: 3, kind: evBranch, br: &emitBranch{cond: localOperand(0), then: &EmitFragment{}, thenOut: EventOperand(1, 0), hasThenOut: true, els: &EmitFragment{}}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a read outside every body token declines: %+v", rec.deopts)
	}
	// A loop body with no `for` word within reach of it.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("x", 43), deoptLit(core.NewInteger(2), 47), deoptLit(core.NewList([]core.Value{deoptTok("j", 50)}), 49)}, 50)
	rec.frag.events = append(rec.frag.events, EmitEvent{seq: 3, kind: evLoop, loop: &emitLoop{start: localOperand(0), end: localOperand(0), step: localOperand(0), body: &EmitFragment{}, bodyOut: EventOperand(1, 0), hasBodyOut: true, iterSlot: -1, pos: deoptAt(47)}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a loop body with no for word declines: %+v", rec.deopts)
	}
	// A def consumer whose name follows no word.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptLit(core.NewInteger(5), 43), deoptTok("k", 47), deoptTok("j", 49)}, 49,
		EmitEvent{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "k", srcSeq: 1, pos: deoptAt(47)}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a def name after no word declines: %+v", rec.deopts)
	}
	// A nested read no body token contains, under a plain call consumer.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptTok("typeof", 45)}, 0,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(45), ops: []EmitOperand{EventOperand(1, 0)}}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a read outside every body token declines under a call too: %+v", rec.deopts)
	}
	// A nested read whose consumer stands nowhere (no position) and makes
	// no literal.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptParen(43, deoptTok("j", 44))}, 44,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, ops: []EmitOperand{EventOperand(1, 0)}}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a consumer with no position declines: %+v", rec.deopts)
	}
	// A top-level read whose consumer stands INSIDE an earlier token (a
	// paren before the read): not after the read, inside the body, and at
	// no top-level token — no statement start to place.
	es, u, rec, _ = deoptUnit(t, []core.Value{deoptParen(43, deoptTok("typeof", 44)), deoptTok("j", 49)}, 49,
		EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(44), ops: []EmitOperand{EventOperand(1, 0)}}})
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a consumer nested before the read declines: %+v", rec.deopts)
	}
}

// TestEmitDeoptsBeforeStackHome pins the lowerer's deopt over a value
// with no frame slot: tested at its depth on the simulated stack, dropped
// when it has no home there either, and kept while its statement is
// still ahead.
func TestEmitDeoptsBeforeStackHome(t *testing.T) {
	cf := &CompiledFn{}
	lw := &lowerer{es: NewEmitState(), p: &Program{}, code: &cf.Code, debug: &cf.Debug, deoptTable: &cf.Deopts,
		vm:     []vmSlot{{seq: 7, idx: 0}, {seq: 8, idx: 0}},
		deopts: []deoptPoint{{seq: 7, slot: -1, name: "j", start: deoptAt(3), token: 1}, {seq: 9, slot: -1, name: "k", start: deoptAt(5), token: 2}, {seq: 7, slot: -1, name: "j", start: deoptAt(9), token: 4}}}
	lw.emitDeoptsBefore(deoptAt(6))
	if len(cf.Deopts) != 1 || cf.Deopts[0].Slot != -1 || cf.Deopts[0].Depth != 1 || cf.Deopts[0].Token != 1 || len(cf.Code) != 1 || cf.Code[0].Op != OpDeoptIfFn {
		t.Errorf("a stack-resident value tests at its depth: %+v %+v", cf.Deopts, cf.Code)
	}
	if len(lw.deopts) != 1 || lw.deopts[0].token != 4 {
		t.Errorf("a later statement's point waits: %+v", lw.deopts)
	}
	lw.emitDeoptsBefore(core.SrcPos{})
	if len(cf.Deopts) != 2 || cf.Deopts[1].Depth != 1 || len(lw.deopts) != 0 {
		t.Errorf("the zero position flushes every point: %+v %+v", cf.Deopts, lw.deopts)
	}
}

// TestPromoteLateDynBindDeoptNames pins the deopt unit's promotion: a def
// the island reads seats its computed source; one it never reads keeps
// its stack home.
func TestPromoteLateDynBindDeoptNames(t *testing.T) {
	es, _, rec, _ := deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45)}, 43, deoptCall(3, "typeof", 45, EventOperand(1, 0)))
	rec.frag.events = append(rec.frag.events,
		deoptCall(4, "get", 60, localOperand(0), ConstOperand(es.internUnpooled(core.NewString("g")))),
		EmitEvent{seq: 5, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: 4, pos: deoptAt(55)}})
	rec.deoptEnv = true
	rec.deoptNames = map[string]bool{"j": true, "typeof": true}
	es.promoteLateDynBind(rec)
	if _, ok := rec.promoted[1]; !ok {
		t.Errorf("the island's def seats its source: %+v", rec.promoted)
	}
	if _, ok := rec.promoted[4]; ok {
		t.Errorf("a def the island never reads keeps its stack home: %+v", rec.promoted)
	}
}

// TestPlanDeoptsCaptureSeedsParent pins the closure arm (the tenth
// increment): a code body's read of a CAPTURED fn-holding local is a point
// on the capture's slot, and the enclosing unit is asked to bind the name
// (seedParentDeopt) — declined when the closure has no enclosing unit, or
// when the parent's root frame binds the name from no re-pushable source,
// or when a nested open frame of the parent rebinds it.
func TestPlanDeoptsCaptureSeedsParent(t *testing.T) {
	es := NewEmitState()
	m := core.NewCarrier(core.TMap)
	parentUnit, _, ok := es.StartFnCompile("k", "h", nil, []core.Value{m}, []*core.Type{core.TAny}, []string{"m"}, nil, false, core.SrcPos{})
	if !ok {
		t.Fatal("parent unit")
	}
	parent := es.fnRecs[parentUnit]
	j := core.NewDynamicCarrier(core.TAny)
	es.producedBy[j.ID] = producer{seq: 1}
	// The parent's root frame so far: `def j (m get "f")`.
	es.frames[parent.rootFrame] = []EmitEvent{
		{seq: 1, kind: evCall, call: emitCall{word: "get", nout: 1, pos: deoptAt(33), ops: []EmitOperand{localOperand(0)}}},
		{seq: 2, kind: evDynBind, dyn: &emitDynBind{name: "j", srcSeq: 1, pos: deoptAt(28)}},
	}
	// The closure unit `[j typeof]` capturing j.
	cap := core.CapturedBinding{Name: "j", Value: j}
	childUnit, _, ok := es.StartFnCompile("c", "each$body", nil, nil, nil, nil, []core.CapturedBinding{cap}, false, core.SrcPos{})
	if !ok {
		t.Fatal("closure unit")
	}
	u := es.units[len(es.units)-1]
	rec := es.fnRecs[childUnit]
	rec.closure = true
	rec.frag = &EmitFragment{events: []EmitEvent{{seq: 5, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(47), ops: []EmitOperand{localOperand(0)}}}}}
	es.SetUnitBody(childUnit, []core.Value{deoptTok("j", 45), deoptTok("typeof", 47)})
	es.NoteWordRead(j, "j", deoptAt(45))
	es.NoteLocalRead(j.ID, deoptAt(45))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || rec.deopts[0].slot != 0 || !rec.deopts[0].atPush || !parent.deoptNames["j"] || len(parent.deoptChildren) != 1 {
		t.Errorf("a captured read plans on its slot and seeds the parent: %+v parent=%v children=%v", rec.deopts, parent.deoptNames, parent.deoptChildren)
	}
	// The parent, planning no points of its own, keeps the seeded env —
	// and drops the child's points when it cannot bind the name after all
	// (a variadic source).
	parent.frag = &EmitFragment{events: es.frames[parent.rootFrame]}
	pu := es.units[len(es.units)-2]
	es.planDeopts(pu, parent)
	if !parent.deoptEnv || !pu.deoptEnv {
		t.Error("the parent keeps the environment its child seeded")
	}
	es.eventInfo[1] = eventFlags{variadicResult: true}
	parent.deoptEnv, pu.deoptEnv = false, false
	es.planDeopts(pu, parent)
	if parent.deoptEnv || len(rec.deopts) != 0 || rec.deoptEnv {
		t.Errorf("an unbindable name drops the child's points: env=%v child=%+v", parent.deoptEnv, rec.deopts)
	}
	// A def of the captured name inside one of the parent's OPEN NESTED
	// frames (the arm the closure sits in) declines: that bind is popped
	// with the arm, so the island would read an unbound name.
	es3 := NewEmitState()
	m3 := core.NewCarrier(core.TMap)
	p3, _, _ := es3.StartFnCompile("k", "h", nil, []core.Value{m3}, []*core.Type{core.TAny}, []string{"m"}, nil, false, core.SrcPos{})
	parent3 := es3.fnRecs[p3]
	j3 := core.NewDynamicCarrier(core.TAny)
	es3.producedBy[j3.ID] = producer{seq: 1}
	es3.frames[parent3.rootFrame] = []EmitEvent{
		{seq: 1, kind: evCall, call: emitCall{word: "get", nout: 1, pos: deoptAt(33), ops: []EmitOperand{localOperand(0)}}},
		{seq: 2, kind: evDynBind, dyn: &emitDynBind{name: "j", srcSeq: 1, pos: deoptAt(28)}},
	}
	// The arm frame the closure sits in rebinds j.
	es3.frames = append(es3.frames, []EmitEvent{{seq: 7, kind: evDynBind, dyn: &emitDynBind{name: "j", srcSeq: -1, val: core.NewInteger(1), pos: deoptAt(40)}}})
	cap3 := core.CapturedBinding{Name: "j", Value: j3}
	c3, _, _ := es3.StartFnCompile("c", "each$body", nil, nil, nil, nil, []core.CapturedBinding{cap3}, false, core.SrcPos{})
	u3 := es3.units[len(es3.units)-1]
	rec3 := es3.fnRecs[c3]
	rec3.closure = true
	rec3.frag = &EmitFragment{events: []EmitEvent{{seq: 5, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(47), ops: []EmitOperand{localOperand(0)}}}}}
	es3.SetUnitBody(c3, []core.Value{deoptTok("j", 45), deoptTok("typeof", 47)})
	es3.NoteWordRead(j3, "j", deoptAt(45))
	es3.NoteLocalRead(j3.ID, deoptAt(45))
	es3.planDeopts(u3, rec3)
	if len(rec3.deopts) != 0 || parent3.deoptNames["j"] {
		t.Errorf("a rebind in the parent's open arm declines: %+v parent=%v", rec3.deopts, parent3.deoptNames)
	}
	// No enclosing unit: a top-level code body declines.
	es2 := NewEmitState()
	top, _, _ := es2.StartFnCompile("c", "each$body", nil, nil, nil, nil, []core.CapturedBinding{cap}, false, core.SrcPos{})
	u2 := es2.units[len(es2.units)-1]
	rec2 := es2.fnRecs[top]
	rec2.closure = true
	rec2.frag = &EmitFragment{events: []EmitEvent{{seq: 5, kind: evCall, call: emitCall{word: "typeof", nout: 1, pos: deoptAt(47), ops: []EmitOperand{localOperand(0)}}}}}
	es2.SetUnitBody(top, []core.Value{deoptTok("j", 45), deoptTok("typeof", 47)})
	es2.NoteWordRead(j, "j", deoptAt(45))
	es2.NoteLocalRead(j.ID, deoptAt(45))
	es2.planDeopts(u2, rec2)
	if len(rec2.deopts) != 0 {
		t.Errorf("a code body with no enclosing unit declines: %+v", rec2.deopts)
	}
}

// TestPlanDeoptsChildSeededNames pins the unit that plans points of its
// OWN and also carries names a closure child seeded on it: the island's
// name set is the union, so the child's captured def binds too.
func TestPlanDeoptsChildSeededNames(t *testing.T) {
	es, u, rec, _ := deoptUnit(t, []core.Value{deoptTok("j", 43), deoptTok("typeof", 45)}, 43,
		deoptCall(3, "typeof", 45, EventOperand(1, 0)))
	rec.deoptNames = map[string]bool{"seeded": true}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !u.deoptNames["seeded"] || !u.deoptNames["typeof"] {
		t.Errorf("a child's seeded names join the unit's own: %+v names=%v", rec.deopts, u.deoptNames)
	}
}

// TestSeatUnitDeoptsUnpromoted pins the at-push point whose value has
// neither a capture slot nor a promoted source: nothing to key the test
// on, so the read keeps its slot push.
func TestSeatUnitDeoptsUnpromoted(t *testing.T) {
	cf := &CompiledFn{}
	flw := &lowerer{}
	rec := &fnUnitRec{deoptEnv: true, body: []core.Value{deoptTok("j", 1)},
		deopts: []deoptPoint{{seq: 4, slot: -1, name: "j", atPush: true}}}
	seatUnitDeopts(flw, rec, cf, false)
	if len(flw.deoptAtSlot) != 0 || len(flw.deopts) != 0 {
		t.Errorf("an unpromoted at-push point seats nothing: %+v %+v", flw.deoptAtSlot, flw.deopts)
	}
	rec.promoted = map[int]int{4: 2}
	seatUnitDeopts(flw, rec, cf, false)
	if d, ok := flw.deoptAtSlot[2]; !ok || d.name != "j" {
		t.Errorf("a promoted source seats on its slot: %+v", flw.deoptAtSlot)
	}
	flw2 := &lowerer{}
	seatUnitDeopts(flw2, rec, cf, true)
	if flw2.deoptTable != nil {
		t.Error("a diverging body seats no points")
	}
}

// producedInCurrentUnit guards resolveOperand's capture override: the override
// takes the capture slot because the producing event "lives in the parent
// frame", so it must not fire when the event is the OPEN unit's own. Which
// floor answers that depends on when resolution runs — the live fragment floor
// while the body records, rec.frag.startSeq once the finish callback resolves
// the body residual and that frame is closed.
func TestProducedInCurrentUnit(t *testing.T) {
	es := NewEmitState()
	es.producedBy["own"] = producer{seq: 7}
	es.producedBy["outer"] = producer{seq: 2}

	if es.producedInCurrentUnit("own") {
		t.Error("no open unit: nothing is this unit's own")
	}

	// A nil record (a slot reserved before its unit opened) declines.
	es.fnRecs = append(es.fnRecs, nil)
	es.openUnitRecs = []int{0}
	if es.producedInCurrentUnit("own") {
		t.Error("a nil unit record declines")
	}

	// Recording: the live floor is the one beginFragment pushed for this
	// unit's frame. beginFragment appends to es.frames and es.fragFloors
	// together, but es.frames OPENS with a root frame it never pushed a floor
	// for — so frame n's floor is fragFloors[n-1]. Here the unit's frame is 1
	// and its floor is the single entry.
	rec := &fnUnitRec{name: "fnval$body", rootFrame: 1}
	es.fnRecs = append(es.fnRecs, rec)
	es.openUnitRecs = []int{1}
	es.fragFloors = []int{5}
	if !es.producedInCurrentUnit("own") || es.producedInCurrentUnit("outer") {
		t.Error("recording: seq at or above the unit's frame floor is its own, below it is not")
	}
	if es.producedInCurrentUnit("absent") {
		t.Error("a value with no producing event is never this unit's own")
	}

	// Out of range (the frame already popped, no frag yet) declines, and so
	// does frame 0 — the root frame is nobody's unit and has no floor.
	rec.rootFrame = 9
	if es.producedInCurrentUnit("own") {
		t.Error("a rootFrame past the live floors declines")
	}
	rec.rootFrame = 0
	if es.producedInCurrentUnit("own") {
		t.Error("the root frame has no floor of its own")
	}
	rec.rootFrame = 1

	// Finish: the root frame is closed, so the fragment carries the floor.
	rec.frag = &EmitFragment{startSeq: 5}
	if !es.producedInCurrentUnit("own") || es.producedInCurrentUnit("outer") {
		t.Error("finish: the fragment's startSeq is the floor")
	}
}
