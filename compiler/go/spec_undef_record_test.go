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
	d := &RegionDesc{Slots: []SlotDesc{
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
// site, consuming nothing; a live operand carrying its read position is
// pushed at that position, and one without it at the consumer's.
func TestLowerSpeculativeUndefAndLiveReadPosition(t *testing.T) {
	undefAt := core.SrcPos{Row: 2, Col: 3}
	readAt := core.SrcPos{Row: 4, Col: 5}
	consumerAt := core.SrcPos{Row: 4, Col: 9}
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
	idx := es.intern(core.NewString("k"))
	op := dynScopeOperand(idx)
	op.pos = readAt
	lw.pushOperand(op, consumerAt)
	lw.pushOperand(dynScopeOperand(idx), consumerAt)
	if len(cf.Code) != 3 || cf.Code[1].Op != OpLookupDynScope || cf.Debug[1] != readAt || cf.Code[2].Op != OpLookupDynScope || cf.Debug[2] != consumerAt {
		t.Fatalf("a live read carries its own position, a plain one the consumer's: code=%v debug=%v", cf.Code, cf.Debug)
	}
	// Finalize hands the placed names to the Program.
	es2 := NewEmitState()
	es2.specUndefNames = map[string]bool{"k": true}
	p, reason, ok := es2.Finalize(nil)
	if !ok || p == nil || !p.SpecUndefNames["k"] {
		t.Fatalf("the program carries the placed names: ok=%v reason=%q", ok, reason)
	}
	if _, ok := p.Disassemble(), true; !ok || !strings.Contains(cf.Code[0].Op.String(), "UNDEF_DYN_SCOPE") {
		t.Fatal("the opcode names itself")
	}
}
