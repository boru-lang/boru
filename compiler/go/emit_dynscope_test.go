package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// emit_dynscope_test.go pins the dynamic-scope emit/lowering arms the corpus
// rows don't reach: the fragment-tree bind detector's loop arm, the rescue's
// FnNameStack reader fallback, and lowerDynBind's promoted / literal-bake /
// non-inert arms.

func dsBindEvent(name string) EmitEvent {
	return EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: name, srcSeq: -1, residentTwin: -1}}
}

func TestEventsBindDynScopeWalksFragments(t *testing.T) {
	names := map[string]bool{"dsn": true}
	// Loop body fragment carries the bind.
	loopEv := EmitEvent{kind: evLoop, loop: &emitLoop{
		body: &EmitFragment{events: []EmitEvent{dsBindEvent("dsn")}},
	}}
	if !eventsBindDynScope([]EmitEvent{loopEv}, names) {
		t.Error("a bind inside a loop body fragment must be detected")
	}
	// Branch arm fragment carries it.
	brEv := EmitEvent{kind: evBranch, br: &emitBranch{
		els: &EmitFragment{events: []EmitEvent{dsBindEvent("dsn")}},
	}}
	if !eventsBindDynScope([]EmitEvent{brEv}, names) {
		t.Error("a bind inside a branch arm fragment must be detected")
	}
	// A bind of an UNREAD name is invisible.
	if eventsBindDynScope([]EmitEvent{dsBindEvent("other")}, names) {
		t.Error("a bind of a name no lookup reads must not be detected")
	}
	if eventsBindDynScope([]EmitEvent{loopEv}, nil) {
		t.Error("an empty name set detects nothing")
	}
}

func TestDynScopeRescueReaderFallback(t *testing.T) {
	r := seam7Reg(t)
	r.Check.Mode = true
	// Binder model: fn "binder" binds "dsv" and reaches fn "reader".
	r.Check.FnBinders = map[string]map[string]bool{"dsv": {"binder": true}}
	r.Check.FnCallGraph = map[string]map[string]bool{"binder": {"reader": true}}
	es := NewEmitState()
	es.BindRegistry(r)
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}},
		&emitUnit{localByID: map[string]int{}})
	// unitNames stays EMPTY (the fallback under test); the check's
	// FnNameStack carries the reader.
	r.Check.FnNameStack = []string{"reader"}
	v := core.NewCarrier(core.TAny)
	es.NoteDefRead(v.ID, "dsv")
	op, ok := es.dynScopeRescue(v)
	if !ok || op.kind != opDynScope {
		t.Fatalf("rescue via FnNameStack fallback: op=%+v ok=%v", op, ok)
	}
	if !es.dynScopeNames["dsv"] {
		t.Error("the rescued name must join dynScopeNames")
	}
}

func TestLowerDynBindArms(t *testing.T) {
	names := map[string]bool{"dsl": true}
	newLW := func(es *EmitState, promoted map[int]int) (*lowerer, *CompiledFn) {
		cf := &CompiledFn{}
		return &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
			sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: promoted}, cf
	}
	es := NewEmitState()
	es.dynScopeNames = names

	// Promoted computed value: pushes the promoted local, then the bind.
	lw, cf := newLW(es, map[int]int{7: 3})
	ev := EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: 7, residentTwin: -1}}
	if reason := lw.lowerDynBind(&ev); reason != "" {
		t.Fatalf("promoted bind declined: %s", reason)
	}
	if len(cf.Code) != 2 || cf.Code[0].Op != OpPushLocal || cf.Code[0].Arg != 3 || cf.Code[1].Op != OpBindDynScope {
		t.Errorf("promoted bind code = %v, want PUSH_LOCAL 3 + BIND_DYN_SCOPE", cf.Code)
	}

	// Unpromoted computed value: declines.
	lw, _ = newLW(es, map[int]int{})
	ev = EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: 9, residentTwin: -1}}
	if reason := lw.lowerDynBind(&ev); !strings.Contains(reason, "unpromoted computed value") {
		t.Errorf("unpromoted bind reason = %q", reason)
	}

	// Literal binding: bakes the recorded value verbatim (unpooled).
	lw, cf = newLW(es, nil)
	ev = EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: -1, val: core.NewInteger(5), residentTwin: -1}}
	if reason := lw.lowerDynBind(&ev); reason != "" {
		t.Fatalf("literal bind declined: %s", reason)
	}
	if len(cf.Code) != 2 || cf.Code[0].Op != OpPushConst || cf.Code[1].Op != OpBindDynScope {
		t.Errorf("literal bind code = %v, want PUSH_CONST + BIND_DYN_SCOPE", cf.Code)
	}

	// A non-inert (tape-coupled) value declines.
	lw, _ = newLW(es, nil)
	ev = EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: -1, val: core.NewWord("live"), residentTwin: -1}}
	if reason := lw.lowerDynBind(&ev); !strings.Contains(reason, "unknown provenance") {
		t.Errorf("non-inert bind reason = %q", reason)
	}

	// A name no lookup reads lowers to nothing.
	lw, cf = newLW(es, nil)
	ev = EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "unread", srcSeq: -1, residentTwin: -1}}
	if reason := lw.lowerDynBind(&ev); reason != "" || len(cf.Code) != 0 {
		t.Errorf("unread bind must be a no-op: reason=%q code=%v", reason, cf.Code)
	}
}

// The active-token MAP const gate: inside a compiled fn frame a map operand
// bearing a word (autoEvalMap re-evaluates it per dispatch against the LIVE
// frame) must not bake — it routes to the dyn-scope rescue and, with no read
// provenance, declines. Lists stay bakeable (code-as-data), and module scope
// keeps the map bake.
func TestActiveTokenMapConstGate(t *testing.T) {
	r := seam7Reg(t)
	// Begin (not a bare Mode write) so the values this test mints carry
	// compile identities — runtime mints elide IDs (value.go
	// checkPassDepth), and an identity-less compound is deliberately NOT
	// bakeable inside a fn unit.
	defer r.Check.Begin()()
	mkMap := func() core.Value {
		om := core.NewOrderedMap()
		om.Set("line", core.NewWord("src"))
		return core.NewMap(om)
	}
	es := NewEmitState()
	es.BindRegistry(r)
	// NewEmitState seeds the module unit; one more puts us inside a fn unit.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	if _, ok := es.resolveOperand(mkMap()); ok {
		t.Error("an active-token map const must not bake inside a fn unit")
	}
	lst := core.NewList([]core.Value{core.NewWord("dup"), core.NewWord("mul")})
	if _, ok := es.resolveOperand(lst); !ok {
		t.Error("a code-as-data LIST const must stay bakeable inside a fn unit")
	}
	// Module scope (the seeded single unit) keeps the map bake.
	es2 := NewEmitState()
	es2.BindRegistry(r)
	if _, ok := es2.resolveOperand(mkMap()); !ok {
		t.Error("module scope must keep the active-token map bake")
	}
}

func TestBearsActiveTokensArms(t *testing.T) {
	if !core.BearsActiveTokens(core.NewList([]core.Value{core.NewInteger(1), core.NewWord("w")})) {
		t.Error("a list member word must count as an active token")
	}
	if core.BearsActiveTokens(core.NewMap(nil)) {
		t.Error("a nil-backed map bears nothing")
	}
	om := core.NewOrderedMap()
	om.Set("k", core.NewInteger(2))
	if core.BearsActiveTokens(core.NewMap(om)) {
		t.Error("an all-data map bears no active tokens")
	}
}

// The rescue's ID-ELIDED enclosing read: a detached stamp forked the
// runtime def table, so the mutable ref's ID was never minted, and the read
// arrives with an empty ID and a DynFrom tag. It commits by NAME against the
// unit's enclosingBindNames snapshot (the by-name twin of enclosingBindIDs);
// a name absent from the snapshot falls to the reachability model as
// before. Driven directly since the seventy-first increment: a stored-ref
// unit's own read of the module value is seated live before it could reach
// this arm, and the elided-ID shape is the detached stamp's alone.
func TestDynScopeRescueElidedIDByName(t *testing.T) {
	r := seam7Reg(t)
	r.Check.Mode = true
	es := NewEmitState()
	es.BindRegistry(r)
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}},
		&emitUnit{localByID: map[string]int{}, enclosingBindIDs: map[string]bool{}, enclosingBindNames: map[string]bool{"files": true}})
	v := core.NewCarrier(core.TAny)
	v.ID = ""
	v.SetDynFrom("files")
	op, ok := es.dynScopeRescue(v)
	if !ok || op.kind != opDynScope || !es.dynScopeNames["files"] {
		t.Fatalf("an elided-ID DynFrom read commits by name: op=%+v ok=%v names=%v", op, ok, es.dynScopeNames)
	}
	w := core.NewCarrier(core.TAny)
	w.ID = ""
	w.SetDynFrom("other")
	if _, ok := es.dynScopeRescue(w); ok {
		t.Fatal("a name outside the snapshot, with no fn binder, is declined as before")
	}
}
