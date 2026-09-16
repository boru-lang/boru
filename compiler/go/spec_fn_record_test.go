package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordSpeculativeFnDef's arms (the seventieth increment): marks the
// family and hands the def site its placed install — the next RecordDynBind
// of the name stamps its event (specFn; replace for an overlapping
// redefinition) — and refuses through the undef site where it cannot place:
// a suspended recording, an arm-resident bracket, an armed loop, a fn body.
func TestRecordSpeculativeFnDefArms(t *testing.T) {
	pos := core.SrcPos{Row: 2, Col: 4}
	var nilES *EmitState
	nilES.RecordSpeculativeFnDef("f", core.Value{}, pos)
	es := NewEmitState()
	es.RecordSpeculativeFnDef("", core.Value{}, pos)
	if es.pendingSpecFn != nil || len(es.specFnNames) != 0 {
		t.Fatal("an empty name records nothing")
	}
	es.RecordSpeculativeFnDef("f", core.Value{}, pos)
	if !es.specFnNames["f"] || es.pendingSpecFn == nil || es.pendingSpecFn.name != "f" || es.pendingSpecFn.replace {
		t.Fatalf("a fresh def: marked, pending, no replace: %v %+v", es.specFnNames, es.pendingSpecFn)
	}
	fnv := core.NewFunction(core.FnDefInfo{Name: "f"})
	es.RecordDynBind("g", core.NewInteger(1), pos)
	if es.pendingSpecFn == nil {
		t.Fatal("another name's def leaves the hand-off pending")
	}
	es.RecordDynBind("f", fnv, pos)
	if es.pendingSpecFn != nil {
		t.Fatal("the def site consumes the hand-off")
	}
	var d *emitDynBind
	for _, ev := range es.frames[0] {
		if ev.kind == evDynBind && ev.dyn.name == "f" {
			d = ev.dyn
		}
	}
	if d == nil || !d.specFn || d.replace {
		t.Fatalf("the def site's event is the placed install: %+v", d)
	}
	// An overlapping redefinition: replace (the outer's units compile only
	// against a registry — the lang rows pin that).
	es.RecordSpeculativeFnDef("f", fnv, pos)
	if es.pendingSpecFn == nil || !es.pendingSpecFn.replace {
		t.Fatal("an outer fn value makes the install a replace")
	}
	// Refused where it cannot be placed.
	refuses := func(t *testing.T, es *EmitState, what string) {
		t.Helper()
		if es.Compilable || !strings.Contains(es.Reason, "fn `f` defined inside a conditional body where the compiled program cannot place the install") {
			t.Fatalf("%s refuses: %v %q", what, es.Compilable, es.Reason)
		}
	}
	es2 := NewEmitState()
	resume := es2.Suspend()
	es2.RecordSpeculativeFnDef("f", core.Value{}, pos)
	resume()
	refuses(t, es2, "a suspended recording")
	es3 := NewEmitState()
	es3.armResidentDepth = 1
	es3.RecordSpeculativeFnDef("f", core.Value{}, pos)
	refuses(t, es3, "an arm-resident bracket")
	es4 := NewEmitState()
	es4.loopCarried = append(es4.loopCarried, &loopCarriedScope{})
	es4.RecordSpeculativeFnDef("f", core.Value{}, pos)
	refuses(t, es4, "an armed loop")
	es5 := NewEmitState()
	es5.reg, _ = core.NewRegistry()
	es5.reg.Check.FnBodyDepth = 1
	es5.RecordSpeculativeFnDef("f", core.Value{}, pos)
	if !es5.Compilable || !es5.specFnNames["f"] {
		t.Fatalf("a fn body's FRESH def is placed (a frame binding the unit's RET unwinds): %q", es5.Reason)
	}
	es5.RecordSpeculativeFnDef("f", fnv, pos)
	refuses(t, es5, "a fn body's replace")
	// The other two kinds read through the one site.
	es6 := NewEmitState()
	es6.refuseUndef("f", specFnUnrouted)
	if es6.Compilable || !strings.Contains(es6.Reason, "dispatch of the conditionally-defined fn `f` cannot route") {
		t.Fatalf("the unrouted refusal: %q", es6.Reason)
	}
	es7 := NewEmitState()
	es7.refuseUndef("f", specFnValueRead)
	if es7.Compilable || !strings.Contains(es7.Reason, "value read of the conditionally-defined fn `f`") {
		t.Fatalf("the value-read refusal: %q", es7.Reason)
	}
}

// The seats a speculative fn family refuses at: a `/v` read (NoteValRead),
// the user-poly record, the rematch record; and Finalize hands the names
// to the Program.
func TestSpecFnRefusingSeatsAndFinalize(t *testing.T) {
	pos := core.SrcPos{Row: 1, Col: 1}
	es := NewEmitState()
	es.specFnNames = map[string]bool{"f": true}
	es.NoteValRead("id", "f")
	if es.Compilable || !strings.Contains(es.Reason, "value read of the conditionally-defined fn `f`") {
		t.Fatalf("a /v read refuses: %q", es.Reason)
	}
	es2 := NewEmitState()
	es2.specFnNames = map[string]bool{"f": true}
	es2.RecordUserPolyCall("f", nil, nil, nil, nil, nil, []core.Value{core.NewInteger(1)}, nil, pos, "f", pos)
	if es2.Compilable || !strings.Contains(es2.Reason, "dispatch of the conditionally-defined fn `f` cannot route") {
		t.Fatalf("the user-poly seat refuses: %q", es2.Reason)
	}
	es3 := NewEmitState()
	es3.specFnNames = map[string]bool{"f": true}
	if es3.RecordDispatchRematchValues("f", []core.Value{core.NewInteger(1)}, 0, 1, pos) || es3.Compilable {
		t.Fatalf("the rematch seat refuses: %q", es3.Reason)
	}
	es4 := NewEmitState()
	es4.specFnNames = map[string]bool{"f": true}
	p, reason, ok := es4.Finalize(nil)
	if !ok || p == nil || !p.SpecFnNames["f"] {
		t.Fatalf("Finalize hands the names to the Program: ok=%v reason=%q", ok, reason)
	}
}

// The placed install lowers to the fn value and OpBindResident at its site
// (no twin, the value popped); a speculative fn family's dispatch routes at
// root with no word slot at all, and nothing else does.
func TestLowerSpecFnBindAndRouteAdmission(t *testing.T) {
	at := core.SrcPos{Row: 3, Col: 9}
	cf := &CompiledFn{}
	es := NewEmitState()
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	fnv := core.NewFunction(core.FnDefInfo{Name: "f"})
	if reason := lw.lowerDynBind(&EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "f", srcSeq: -1, residentTwin: -1, specFn: true, root: true, val: fnv, pos: at}}); reason != "" {
		t.Fatalf("a placed fn install lowers: %s", reason)
	}
	if len(cf.Code) != 2 || cf.Code[0].Op != OpPushConst || cf.Code[1].Op != OpBindResident || cf.Debug[1] != at || len(lw.vm) != 0 {
		t.Fatalf("the value then BIND_RESIDENT at the site, nothing left on the sim: code=%v vm=%v", cf.Code, lw.vm)
	}
	if rb := lw.p.ResidentBinds; len(rb) != 1 || rb[0].Name != "f" || rb[0].Twin != -1 || !rb[0].Pop || rb[0].Undef || rb[0].TypeInstall {
		t.Fatalf("the resident spec: %+v", rb)
	}
	if !core.IsAppliableFn(es.consts[cf.Code[0].Arg]) {
		t.Fatal("the const is the fn value")
	}
	// Inside a unit (root false) the install is the frame-scoped
	// OpBindDynScope, naming the binding.
	if reason := lw.lowerDynBind(&EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "f", srcSeq: -1, residentTwin: -1, specFn: true, root: false, val: fnv, pos: at}}); reason != "" {
		t.Fatalf("a placed fn install in a unit lowers: %s", reason)
	}
	if len(cf.Code) != 4 || cf.Code[3].Op != OpBindDynScope || len(lw.vm) != 0 {
		t.Fatalf("BIND_DYN_SCOPE inside a unit: code=%v vm=%v", cf.Code, lw.vm)
	}
	if name, err := es.consts[cf.Code[3].Arg].AsConcreteString(); err != nil || name != "f" {
		t.Fatalf("the op names the binding: %v %v", name, err)
	}
	// Route admission at root: a speculative lead routes with no slot; any
	// other slot-less lead does not.
	es2 := NewEmitState()
	es2.specFnNames = map[string]bool{"f": true}
	if !es2.routeRegion(&RegionDesc{Lead: LeadWord, Word: "f"}) {
		t.Fatal("a speculative fn family's slot-less dispatch routes at root")
	}
	if es2.routeRegion(&RegionDesc{Lead: LeadWord, Word: "g"}) {
		t.Fatal("another slot-less lead keeps its committed call")
	}
	// A frame-local lead (LeadLocal) routes only for a speculative family.
	if !es2.routeRegion(&RegionDesc{Lead: LeadWord, Word: "f", LeadLocal: true}) || es2.routeRegion(&RegionDesc{Lead: LeadWord, Word: "g", LeadLocal: true}) {
		t.Fatal("a speculative frame-local lead routes; any other frame-local lead does not")
	}
}
