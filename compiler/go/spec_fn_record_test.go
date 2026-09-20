package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordSpeculativeFnDef's arms (the seventieth increment): marks the
// family and hands the def site its placed install — the next RecordDynBind
// of the name stamps its event (specFn; replace for an overlapping
// redefinition) — and declines through the undef site where it cannot place:
// a suspended recording, an arm-resident bracket, an armed loop, a fn body.
func TestRecordSpeculativeFnDefArms(t *testing.T) {
	pos := core.SrcPos{Row: 2, Col: 4}
	decl := core.DeclSite{Pos: core.SrcPos{Row: 2, Col: 9}, File: "f.boru"}
	fnv := core.NewFunction(core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)}), Decl: decl}}})
	lambda := core.NewFunction(core.FnDefInfo{Name: "f", Anonymous: true, Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}})
	var nilES *EmitState
	if nilES.RecordSpeculativeFnDef(nil, "f", core.Value{}, fnv, pos) {
		t.Fatal("a nil recorder places nothing")
	}
	es := NewEmitState()
	if es.RecordSpeculativeFnDef(nil, "", core.Value{}, fnv, pos) || es.pendingSpecFn != nil || len(es.specFnNames) != 0 {
		t.Fatal("an empty name records nothing")
	}
	if !es.RecordSpeculativeFnDef(nil, "f", core.Value{}, fnv, pos) || !es.specFnNames["f"] || es.pendingSpecFn == nil || es.pendingSpecFn.name != "f" || es.pendingSpecFn.replace {
		t.Fatalf("a fresh def: placed, marked, pending, no replace: %v %+v", es.specFnNames, es.pendingSpecFn)
	}
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
	// against a registry — the lang rows pin that; without one, nothing).
	if !es.RecordSpeculativeFnDef(nil, "f", fnv, fnv, pos) || es.pendingSpecFn == nil || !es.pendingSpecFn.replace {
		t.Fatal("an outer fn value makes the install a replace")
	}
	if nilES.compileSpecOuterUnit(core.FnDefInfo{}, 0) != -1 || es.compileSpecOuterUnit(core.FnDefInfo{}, 0) != -1 {
		t.Fatal("no recorder or no registry compiles nothing")
	}
	if fnSigsDeclared(lambda) || fnSigsDeclared(core.NewInteger(1)) || fnSigsDeclared(core.NewFunction(core.FnDefInfo{Name: "n", Signatures: []core.Signature{{}}})) || !fnSigsDeclared(fnv) {
		t.Fatal("only a boru signature with a declaration site is declared")
	}
	// Declined: a FRESH def declines (the join's model is the bug), a
	// REPLACE keeps installDef's own compile failure — for a suspended recording, an
	// arm-resident bracket, an armed loop, a fn body's replace, a lambda as
	// the placed value or as the outer; a closure body compile declines
	// silently either way.
	declines := func(t *testing.T, es *EmitState, what string) {
		t.Helper()
		if es.Compilable || !strings.Contains(es.Reason, "fn `f` defined inside a conditional body where the compiled program cannot place the install") {
			t.Fatalf("%s declines: %v %q", what, es.Compilable, es.Reason)
		}
	}
	declined := func(t *testing.T, es *EmitState, what string) {
		t.Helper()
		if !es.Compilable || es.specFnNames["f"] || es.pendingSpecFn != nil {
			t.Fatalf("%s declines without declining: %v %q", what, es.Compilable, es.Reason)
		}
	}
	es2 := NewEmitState()
	resume := es2.Suspend()
	placed := es2.RecordSpeculativeFnDef(nil, "f", core.Value{}, fnv, pos)
	resume()
	if placed {
		t.Fatal("a suspended recording places nothing")
	}
	declines(t, es2, "a suspended recording's fresh def")
	es3 := NewEmitState()
	es3.armResidentDepth = 1
	if es3.RecordSpeculativeFnDef(nil, "f", fnv, fnv, pos) {
		t.Fatal("an arm-resident bracket places nothing")
	}
	declined(t, es3, "an arm-resident bracket's replace")
	es4 := NewEmitState()
	es4.loopCarried = append(es4.loopCarried, &loopCarriedScope{})
	if es4.RecordSpeculativeFnDef(nil, "f", core.Value{}, fnv, pos) {
		t.Fatal("an armed loop places nothing")
	}
	declines(t, es4, "an armed loop's fresh def")
	es5 := NewEmitState()
	es5.reg, _ = core.NewRegistry()
	es5.reg.Check.FnBodyDepth = 1
	if !es5.RecordSpeculativeFnDef(nil, "f", core.Value{}, fnv, pos) || !es5.specFnNames["f"] {
		t.Fatalf("a fn body's FRESH def is placed (a frame binding the unit's RET unwinds): %q", es5.Reason)
	}
	es5.pendingSpecFn = nil
	if es5.RecordSpeculativeFnDef(nil, "f", fnv, fnv, pos) || !es5.Compilable {
		t.Fatalf("a fn body's replace declines without declining: %q", es5.Reason)
	}
	esL := NewEmitState()
	if esL.RecordSpeculativeFnDef(nil, "f", core.Value{}, lambda, pos) {
		t.Fatal("a conditional lambda def places nothing")
	}
	declines(t, esL, "a conditional lambda def")
	esL2 := NewEmitState()
	if esL2.RecordSpeculativeFnDef(nil, "f", lambda, fnv, pos) {
		t.Fatal("a lambda-valued outer places nothing")
	}
	declined(t, esL2, "a lambda-valued outer's replace")
	// A MODULE's def declines: the registry the def installs into is not
	// the program's (the corpus's finding on #466, third half — es.reg is
	// the last one bound, so it is not the test).
	esM := NewEmitState()
	progReg, _ := core.NewRegistry()
	modReg, _ := core.NewRegistry()
	esM.BindRegistry(progReg)
	esM.BindRegistry(modReg)
	if esM.RecordSpeculativeFnDef(modReg, "f", core.Value{}, fnv, pos) {
		t.Fatal("a module registry's def places nothing")
	}
	declined(t, esM, "a module registry's fresh def")
	esM.reg = modReg
	if !esM.RecordSpeculativeFnDef(progReg, "f", core.Value{}, fnv, pos) || !esM.specFnNames["f"] {
		t.Fatalf("the program registry's def is placed whatever es.reg last bound: %q", esM.Reason)
	}
	// The other two kinds read through the one site.
	es6 := NewEmitState()
	es6.declineUndef("f", specFnUnrouted)
	if es6.Compilable || !strings.Contains(es6.Reason, "dispatch of the conditionally-defined fn `f` cannot route") {
		t.Fatalf("the unrouted compile failure: %q", es6.Reason)
	}
	es7 := NewEmitState()
	es7.declineUndef("f", specFnValueRead)
	if es7.Compilable || !strings.Contains(es7.Reason, "value read of the conditionally-defined fn `f`") {
		t.Fatalf("the value-read compile failure: %q", es7.Reason)
	}
}

// The one dispatch of a speculative fn family RecordUserCall still declines
// after an undrivable window takes the slot-less descriptor: a callee with
// captures (the captures ride as trailing CALL_USER operands the routed op
// has no plumbing for) — driven over a real capture as
// TestRecordUserCallDeclinesCapturesAndLocalLeads drives its decline.
func TestRecordUserCallDoesNotLowerCapturedSpecCallee(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()
	caps := []core.CapturedBinding{{Name: "c", Value: core.NewInteger(3)}}
	kv, b := core.NewInteger(5), core.NewInteger(1)
	reg.Defs.Push("k", kv)
	reg.Register("f", core.Signature{Args: []*core.Type{core.TAny, core.TAny}})
	pos := capture(t, es, reg, "f", core.NewWord("k"), b)
	unit, _, ok := es.StartFnCompile("f", "f", nil, []core.Value{core.NewInteger(0), core.NewInteger(0)},
		[]*core.Type{core.TAny, core.TAny}, []string{"x", "y"}, caps, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	es.specFnNames = map[string]bool{"f": true}
	es.RecordUserCall(unit, "f", []core.Value{kv, b}, nil, core.SrcPos{Row: 1, Col: 3}, pos)
	if es.Compilable || !strings.Contains(es.Reason, "dispatch of the conditionally-defined fn `f` cannot route") {
		t.Fatalf("a captured speculative callee declines: %v %q", es.Compilable, es.Reason)
	}
}

// The seats a speculative fn family declines at: a `/v` read (NoteValRead),
// the user-poly record, the rematch record; and Finalize hands the names
// to the Program.
func TestSpecFnFailingToCompileSeatsAndFinalize(t *testing.T) {
	pos := core.SrcPos{Row: 1, Col: 1}
	es := NewEmitState()
	es.specFnNames = map[string]bool{"f": true}
	es.NoteValRead("id", "f")
	if es.Compilable || !strings.Contains(es.Reason, "value read of the conditionally-defined fn `f`") {
		t.Fatalf("a /v read declines: %q", es.Reason)
	}
	es2 := NewEmitState()
	es2.specFnNames = map[string]bool{"f": true}
	es2.RecordUserPolyCall("f", nil, nil, nil, nil, nil, []core.Value{core.NewInteger(1)}, nil, pos, "f", pos)
	if es2.Compilable || !strings.Contains(es2.Reason, "dispatch of the conditionally-defined fn `f` cannot route") {
		t.Fatalf("the user-poly seat declines: %q", es2.Reason)
	}
	es3 := NewEmitState()
	es3.specFnNames = map[string]bool{"f": true}
	if es3.RecordDispatchRematchValues("f", []core.Value{core.NewInteger(1)}, 0, 1, pos) || es3.Compilable {
		t.Fatalf("the rematch seat declines: %q", es3.Reason)
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
