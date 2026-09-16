package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordSpeculativeUndef's arms (the sixty-eighth increment): placed as an
// undef-half dyn-bind event where the recorder is live, the name joining
// specUndefNames; refused through the undef site where it cannot be seated
// (a suspended recording, an arm-resident bracket, a carried name); exempt
// inside a closure body compile; a no-op on a nil recorder.
func TestRecordSpeculativeUndefArms(t *testing.T) {
	pos := core.SrcPos{Row: 3, Col: 7}
	es := NewEmitState()
	es.RecordSpeculativeUndef("k", pos)
	if !es.Compilable || !es.specUndefNames["k"] || len(es.frames[0]) != 1 {
		t.Fatalf("a live root recording places the undef: compilable=%v names=%v events=%d", es.Compilable, es.specUndefNames, len(es.frames[0]))
	}
	if d := es.frames[0][0].dyn; es.frames[0][0].kind != evDynBind || d == nil || !d.speculative || !d.undef || d.bindsValue() || d.name != "k" || d.pos != pos || d.residentTwin != -1 {
		t.Fatalf("the placed event is a speculative undef half at its site: %+v", es.frames[0][0])
	}
	// Suspended: refused, the program's.
	es2 := NewEmitState()
	resume := es2.Suspend()
	es2.RecordSpeculativeUndef("k", pos)
	resume()
	if es2.Compilable || !strings.Contains(es2.Reason, "undef of the enclosing binding `k`") {
		t.Fatalf("a suspended recording refuses: %v %q", es2.Compilable, es2.Reason)
	}
	// An arm-resident bracket: refused (the bridge pairs events to twins).
	es3 := NewEmitState()
	es3.armResidentDepth = 1
	es3.RecordSpeculativeUndef("k", pos)
	if es3.Compilable {
		t.Fatal("an arm-resident bracket refuses")
	}
	// A carried name — by a live loop's slot, or by any loop so far.
	es4 := NewEmitState()
	es4.loopCarried = append(es4.loopCarried, &loopCarriedScope{unitDepth: 1, slots: map[string]int{"k": 0}})
	es4.RecordSpeculativeUndef("k", pos)
	if es4.Compilable {
		t.Fatal("a name a live loop carries refuses")
	}
	es5 := NewEmitState()
	es5.carriedNames = map[string]bool{"k": true}
	es5.RecordSpeculativeUndef("k", pos)
	if es5.Compilable {
		t.Fatal("a name a loop has carried refuses")
	}
	if !es5.nameCarried("k") || es5.nameCarried("z") || es4.nameCarried("z") {
		t.Fatal("nameCarried reads both channels")
	}
	// A closure body compile: exempt, nothing placed.
	es6 := NewEmitState()
	es6.fnRecs = append(es6.fnRecs, &fnUnitRec{closure: true})
	es6.openUnitRecs = append(es6.openUnitRecs, 0)
	es6.RecordSpeculativeUndef("k", pos)
	if !es6.Compilable || len(es6.frames[0]) != 0 || es6.specUndefNames["k"] {
		t.Fatalf("a closure body compile is exempt: %q %d", es6.Reason, len(es6.frames[0]))
	}
	// An empty name and a nil recorder are no-ops.
	es7 := NewEmitState()
	es7.RecordSpeculativeUndef("", pos)
	if len(es7.frames[0]) != 0 {
		t.Fatal("an empty name places nothing")
	}
	var none *EmitState
	none.RecordSpeculativeUndef("k", pos)
}

// The def-after-undef guard: a `def` of a generalised name inside a
// rolled-back region of the CURRENT unit refuses through the undef site;
// outside any region, or inside a region an enclosing unit's call site sits
// in, the def records as before.
func TestRecordDynBindRefusesDefAfterSpecUndef(t *testing.T) {
	pos := core.SrcPos{Row: 1, Col: 1}
	es := NewEmitState()
	es.specUndefNames = map[string]bool{"k": true}
	if es.inRolledBackRegion() {
		t.Fatal("no open fragment: not in a region")
	}
	es.RecordDynBind("k", core.NewInteger(6), pos)
	if !es.Compilable || len(es.frames[0]) != 1 {
		t.Fatalf("a def outside any region records: %q events=%d", es.Reason, len(es.frames[0]))
	}
	// A fragment this unit opened: in the region.
	es.fragUnits = append(es.fragUnits, len(es.units))
	if !es.inRolledBackRegion() {
		t.Fatal("a fragment of the current unit is the region")
	}
	es.RecordDynBind("k", core.NewInteger(6), pos)
	if es.Compilable || !strings.Contains(es.Reason, "def of `k` inside the region that undefs it") {
		t.Fatalf("a def of the generalised name inside the region refuses: %v %q", es.Compilable, es.Reason)
	}
	// A fragment an ENCLOSING unit opened: this unit's body is not that region.
	es2 := NewEmitState()
	es2.specUndefNames = map[string]bool{"k": true}
	es2.fragUnits = append(es2.fragUnits, len(es2.units)-1)
	if es2.inRolledBackRegion() {
		t.Fatal("an enclosing unit's fragment is not this unit's region")
	}
	es2.RecordDynBind("k", core.NewInteger(6), pos)
	if !es2.Compilable {
		t.Fatalf("a def under an enclosing unit's fragment records: %q", es2.Reason)
	}
	// A different name is untouched.
	es3 := NewEmitState()
	es3.specUndefNames = map[string]bool{"k": true}
	es3.fragUnits = append(es3.fragUnits, len(es3.units))
	es3.RecordDynBind("j", core.NewInteger(6), pos)
	if !es3.Compilable {
		t.Fatalf("a def of another name records: %q", es3.Reason)
	}
	// beginFragment pushes and pops the unit depth beside the fragment id.
	es4 := NewEmitState()
	end := es4.beginFragment()
	if len(es4.fragUnits) != 1 || es4.fragUnits[0] != 1 || !es4.inRolledBackRegion() {
		t.Fatalf("beginFragment notes the unit depth: %v", es4.fragUnits)
	}
	end()
	if len(es4.fragUnits) != 0 || es4.inRolledBackRegion() {
		t.Fatalf("the close pops it: %v", es4.fragUnits)
	}
}

// specUndefFwdSlot: the first forward WORD slot naming a generalised name,
// scanning every slot (the live operand stops the record's claim short);
// nothing for a nil descriptor, an empty set, or slots that are not words.
func TestSpecUndefFwdSlot(t *testing.T) {
	es := NewEmitState()
	d := &RegionDesc{NFwd: 3, Slots: []SlotDesc{
		{Token: core.NewInteger(1)},
		{Source: SlotWordRef, Token: core.NewWord("j")},
		{Source: SlotWordRef, Token: core.NewWord("k")},
	}}
	if es.specUndefFwdSlot(d) != "" || es.specUndefFwdSlot(nil) != "" {
		t.Fatal("no generalised name: nothing")
	}
	es.specUndefNames = map[string]bool{"k": true}
	if got := es.specUndefFwdSlot(d); got != "k" {
		t.Fatalf("the generalised word slot is named: %q", got)
	}
	// Only a CLAIMED slot counts: beyond the claim the token is not this
	// dispatch's.
	d.NFwd = 2
	if got := es.specUndefFwdSlot(d); got != "" {
		t.Fatalf("a slot beyond the claim is not named: %q", got)
	}
	d.NFwd = 3
	if es.specUndefFwdSlot(nil) != "" {
		t.Fatal("a nil descriptor names nothing")
	}
	// The refusal reads through the one undef site.
	es.refuseUndef("k", fwdReadAfterSpecUndef)
	if es.Compilable || !strings.Contains(es.Reason, "forward-slot read of `k` after a placed undef") {
		t.Fatalf("the forward-slot refusal: %v %q", es.Compilable, es.Reason)
	}
}

// The lowering of a placed speculative undef is OpUndefDynScope at its
// site, consuming nothing; a live read seated as an event lowers to the
// lookup at the read's own token with one result on the sim; and a read
// the placement did not seat refuses at the rescue.
func TestLowerSpeculativeUndefAndLiveRead(t *testing.T) {
	undefAt := core.SrcPos{Row: 2, Col: 3}
	readAt := core.SrcPos{Row: 4, Col: 5}
	cf := &CompiledFn{}
	es := NewEmitState()
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	if reason := lw.lowerDynBind(&EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "k", srcSeq: -1, residentTwin: -1, undef: true, speculative: true, pos: undefAt}}); reason != "" {
		t.Fatalf("a speculative undef lowers: %s", reason)
	}
	if len(cf.Code) != 1 || cf.Code[0].Op != OpUndefDynScope || cf.Debug[0] != undefAt || len(lw.vm) != 0 {
		t.Fatalf("UNDEF_DYN_SCOPE at its site, no stack effect: code=%v debug=%v vm=%v", cf.Code, cf.Debug, lw.vm)
	}
	if name, err := es.consts[cf.Code[0].Arg].AsConcreteString(); err != nil || name != "k" {
		t.Fatalf("the op names the binding: %v %v", name, err)
	}
	// The live read: the recorder seats it with its own identity and event.
	es.specUndefNames = map[string]bool{"k": true}
	v := core.NewCarrier(core.TInteger)
	before := v.ID
	es.NoteLiveRead(&v, "k", readAt)
	pr, ok := es.producedBy[v.ID]
	if v.ID == before || !ok || es.defReads[v.ID] != "k" || len(es.frames[0]) != 1 || !es.frames[0][0].call.live || es.frames[0][0].call.nout != 1 {
		t.Fatalf("a live read gets its own identity and a one-result live event: id changed=%v produced=%v events=%d", v.ID != before, ok, len(es.frames[0]))
	}
	ev := es.frames[0][len(es.frames[0])-1]
	if ev.seq != pr.seq {
		t.Fatalf("the read's producer is the live event: %d vs %d", ev.seq, pr.seq)
	}
	if reason := lw.lowerCall(&ev); reason != "" {
		t.Fatalf("the live event lowers: %s", reason)
	}
	if len(cf.Code) != 2 || cf.Code[1].Op != OpLookupDynScope || cf.Debug[1] != readAt || len(lw.vm) != 1 || lw.vm[0].seq != ev.seq {
		t.Fatalf("the lookup sits at the read token with one result on the sim: code=%v debug=%v vm=%v", cf.Code, cf.Debug, lw.vm)
	}
	if op, ok := es.resolveOperand(v); !ok || op.kind != opEvent {
		t.Fatalf("the read resolves to its event: %+v %v", op, ok)
	}
	// Other names, a nil value and an inactive recorder: nothing seated.
	w := core.NewCarrier(core.TInteger)
	es.NoteLiveRead(&w, "z", readAt)
	es.NoteLiveRead(nil, "k", readAt)
	resume := es.Suspend()
	es.NoteLiveRead(&w, "k", readAt)
	resume()
	if len(es.frames[0]) != 1 {
		t.Fatalf("only a generalised name's live read is seated: events=%d", len(es.frames[0]))
	}
	// A read of the name that reaches the rescue instead refuses.
	es3 := NewEmitState()
	es3.reg, _ = core.NewRegistry()
	es3.specUndefNames = map[string]bool{"k": true}
	c := core.NewCarrier(core.TInteger)
	es3.defReads = map[string]string{c.ID: "k"}
	if _, ok := es3.dynScopeRescue(c); ok || es3.Compilable || !strings.Contains(es3.Reason, "read of `k` after a placed undef the placement did not seat") {
		t.Fatalf("an unseated read refuses: ok=%v %q", ok, es3.Reason)
	}
	// Finalize hands the placed names to the Program.
	es2 := NewEmitState()
	es2.specUndefNames = map[string]bool{"k": true}
	p, reason, ok := es2.Finalize(nil)
	if !ok || p == nil || !p.SpecUndefNames["k"] {
		t.Fatalf("the program carries the placed names: ok=%v reason=%q", ok, reason)
	}
	if !strings.Contains(cf.Code[0].Op.String(), "UNDEF_DYN_SCOPE") {
		t.Fatal("the opcode names itself")
	}
}

// twinInstalls mirrors core.ApplyBindTwin's push rule: a concrete (or bare
// type node) value entry that is not written back installs; a carrier's
// entry, a written-back one, a type entry and no twin at all do not. A root
// def whose twin installs emits no BIND_DYN_SCOPE beside the replay.
func TestTwinInstallsAndRootDynBindSkip(t *testing.T) {
	es := NewEmitState()
	es.dynScopeNames = map[string]bool{"k": true}
	cf := &CompiledFn{}
	p := &Program{
		BindTwins:       []core.BindTransition{{Kind: core.BindDef, Name: "k"}, {Kind: core.BindDef, Name: "c"}, {Kind: core.BindDef, Name: "w", WrittenBack: true}, {Kind: core.BindTypeInstall, Name: "T"}},
		BindTwinEntries: []core.DefEntry{{Body: core.NewInteger(5)}, {Body: core.NewCarrier(core.TInteger)}, {Body: core.NewInteger(5)}, {Body: core.NewTypeLiteral(core.TInteger), TypeDef: core.TInteger}},
	}
	lw := &lowerer{es: es, p: p, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	if !lw.twinInstalls(0) || lw.twinInstalls(1) || lw.twinInstalls(2) || lw.twinInstalls(3) || lw.twinInstalls(-1) || lw.twinInstalls(9) {
		t.Fatal("twinInstalls: concrete yes; carrier, written-back, type, none: no")
	}
	pos := core.SrcPos{Row: 1, Col: 5}
	// The root def of k, paired with its concrete twin: no second install.
	lw.noteTwin(0)
	if reason := lw.lowerDynBind(&EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "k", srcSeq: -1, residentTwin: -1, root: true, depth: 1, val: core.NewInteger(5), pos: pos}}); reason != "" || len(cf.Code) != 0 {
		t.Fatalf("a root def its twin replays emits nothing: %q code=%v", reason, cf.Code)
	}
	// The same def with a CARRIER twin keeps its BIND_DYN_SCOPE.
	es.dynScopeNames["c"] = true
	lw.noteTwin(1)
	if reason := lw.lowerDynBind(&EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "c", srcSeq: -1, residentTwin: -1, root: true, depth: 1, val: core.NewInteger(5), pos: pos}}); reason != "" || len(cf.Code) != 2 || cf.Code[1].Op != OpBindDynScope {
		t.Fatalf("a root def whose twin is skipped keeps the install: %q code=%v", reason, cf.Code)
	}
}
