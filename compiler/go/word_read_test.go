package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// wordReadUnit opens a user-fn unit over one fn-typed param `g` and one
// gradual `x:Any` param on an active state, returning the state, the unit's
// record and the two param carriers (the frame locals a body reads).
func wordReadUnit(t *testing.T) (*EmitState, *fnUnitRec, core.Value, core.Value) {
	t.Helper()
	es := NewEmitState()
	g := core.NewCarrier(core.TFunction)
	x := core.NewDynamicCarrier(core.TAny)
	unit, _, ok := es.StartFnCompile("k", "f", nil, []core.Value{g, x}, []*core.Type{core.TAny}, []string{"g", "x"}, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	return es, es.fnRecs[unit], g, x
}

// TestNoteWordReadArms pins the recorder's counting (NUR123): an inactive
// state, an empty id or name, no open unit and a non-local value all note
// nothing; a fn-typed local counts strictly, a gradual one only names
// itself; a `/v` read counts on the unit.
func TestNoteWordReadArms(t *testing.T) {
	es := NewEmitState()
	es.NoteWordRead(core.NewCarrier(core.TFunction), "g", core.SrcPos{})
	es.NoteValRead("id", "n")
	if len(es.fnRecs) != 0 {
		t.Fatal("no open unit: nothing to record on")
	}
	es, rec, g, x := wordReadUnit(t)
	es.NoteWordRead(core.Value{}, "g", core.SrcPos{})
	es.NoteWordRead(g, "", core.SrcPos{})
	es.NoteWordRead(core.NewCarrier(core.TFunction), "other", core.SrcPos{})
	es.NoteValRead("", "n")
	if rec.wordReadNames != nil || rec.valReads != nil {
		t.Errorf("empty ids/names and a non-local value note nothing: %v %v", rec.wordReadNames, rec.valReads)
	}
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 7})
	es.NoteWordRead(x, "x", core.SrcPos{Row: 1, Col: 9})
	if rec.wordReads[g.ID] != 1 || rec.wordReads[x.ID] != 0 {
		t.Errorf("only the fn-typed read is accounted strictly: %v", rec.wordReads)
	}
	if rec.wordReadNames[x.ID] != "x" || rec.wordReadPos[g.ID].Col != 7 {
		t.Errorf("both reads keep their name and position: %v %v", rec.wordReadNames, rec.wordReadPos)
	}
	es.NoteValRead(g.ID, "n")
	if rec.valReads[g.ID] != 1 {
		t.Errorf("a /v read counts: %v", rec.valReads)
	}
	es.Compilable = false
	es.NoteWordRead(g, "g", core.SrcPos{})
	es.NoteValRead(g.ID, "n")
	if rec.wordReads[g.ID] != 1 || rec.valReads[g.ID] != 1 {
		t.Error("an inactive state records nothing")
	}
}

// TestCreditWordReadArms pins the apply-lowering credit: a nil state, an
// empty id, no open unit and an id never read bare credit nothing; a read
// id credits once per lowering (RecordDynApply and RegisterTrailingApply
// both route here).
func TestCreditWordReadArms(t *testing.T) {
	var nilES *EmitState
	nilES.creditWordRead("x")
	es := NewEmitState()
	es.creditWordRead("x")
	es, rec, g, _ := wordReadUnit(t)
	es.creditWordRead("")
	es.creditWordRead(g.ID)
	if rec.wordReadCredit != nil {
		t.Errorf("an id never read bare takes no credit: %v", rec.wordReadCredit)
	}
	es.NoteWordRead(g, "g", core.SrcPos{})
	es.RegisterTrailingApply(g.ID, 1)
	if _, ok := es.RecordDynApply([]core.Value{core.NewInteger(5)}, g, core.NewCarrier(core.TInteger), core.SrcPos{}); !ok {
		t.Fatal("a plain paren apply over a carrier lead records")
	}
	if rec.wordReadCredit[g.ID] != 2 {
		t.Errorf("each apply lowering credits one read: %v", rec.wordReadCredit)
	}
}

// TestWordReadAccounting pins the unit-finish rule: no reads pass; a read
// consumed nowhere the replay or an apply lowering saw refuses (a container
// member, an arm residual); a credited read passes; a read seated in the
// replay window passes; an id read both bare and by /v refuses.
func TestWordReadAccounting(t *testing.T) {
	es, rec, g, x := wordReadUnit(t)
	if r := es.wordReadAccounting(rec); r != "" {
		t.Errorf("no reads: %q", r)
	}
	es.NoteWordRead(g, "g", core.SrcPos{})
	if r := es.wordReadAccounting(rec); !strings.Contains(r, "consumed where the interpreter dispatches it") {
		t.Errorf("an unseated fn-typed read refuses: %q", r)
	}
	es.creditWordRead(g.ID)
	if r := es.wordReadAccounting(rec); r != "" {
		t.Errorf("a credited read passes: %q", r)
	}
	es.NoteWordRead(g, "g", core.SrcPos{})
	rec.outOpsVals = []core.Value{x, g}
	rec.dynFrameW = 2
	rec.dynFrameWords = []DynFrameWord{{}, {Name: "g"}}
	if r := es.wordReadAccounting(rec); r != "" {
		t.Errorf("a read seated in the replay window passes: %q", r)
	}
	es.NoteValRead(g.ID, "n")
	if r := es.wordReadAccounting(rec); !strings.Contains(r, "read both bare and by /v") {
		t.Errorf("a mixed read refuses: %q", r)
	}
}

// TestWordReadReplayArms pins wordReadName, dynFrameWordsFor,
// replayValueApplicables and noteWordReadReplay: a nil record, a quoted
// value and an apply-pending id are not word reads; a residual with no
// word read arms nothing; a gradual read the window cannot seat keeps the
// slot push (true, unarmed); a fn-typed one refuses (false); a seatable
// read arms the replay with the word table.
func TestWordReadReplayArms(t *testing.T) {
	es, rec, g, x := wordReadUnit(t)
	u := es.units[len(es.units)-1]
	if es.wordReadName(nil, g) != "" || es.wordReadName(rec, g) != "" {
		t.Error("a nil record and an un-noted value are not word reads")
	}
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 5})
	es.NoteWordRead(x, "x", core.SrcPos{Row: 1, Col: 8})
	quoted := g
	quoted.Quoted = true
	if es.wordReadName(rec, quoted) != "" {
		t.Error("a quoted value is data")
	}
	u.pendingApply = append(u.pendingApply, pendingApply{id: x.ID})
	if es.wordReadName(rec, x) != "" {
		t.Error("an apply-pending id is the apply word's")
	}
	u.pendingApply = nil
	if es.dynFrameWordsFor(u, rec, []core.Value{core.NewInteger(1)}) != nil {
		t.Error("a window without a word read has no table")
	}
	if !es.noteWordReadReplay(u, rec, []core.Value{core.NewInteger(1)}) || rec.dynFrameW != 0 {
		t.Error("no word read: nothing to arm, nothing to refuse")
	}
	// A window whose events run AFTER the read is not a body tail: a fragment
	// event positioned past the read.
	rec.frag = &EmitFragment{events: []EmitEvent{{seq: 1, kind: evCall, call: emitCall{pos: core.SrcPos{Row: 2, Col: 1}}}}}
	if !es.noteWordReadReplay(u, rec, []core.Value{x}) || rec.dynFrameW != 0 {
		t.Error("a gradual read the window cannot seat keeps the slot push, unarmed")
	}
	if es.noteWordReadReplay(u, rec, []core.Value{g}) {
		t.Error("a fn-typed read the window cannot seat refuses")
	}
	rec.frag = &EmitFragment{}
	names := es.dynFrameWordsFor(u, rec, []core.Value{core.NewInteger(1), g})
	if replayValueApplicables([]core.Value{core.NewInteger(1), g}, names) != 0 {
		t.Error("a word-read entry is not a value-semantics applicable")
	}
	if replayValueApplicables([]core.Value{core.NewCarrier(core.TFunction), g}, names) != 1 {
		t.Error("a value-read fn beside it is")
	}
	if !es.noteWordReadReplay(u, rec, []core.Value{core.NewInteger(1), g}) || rec.dynFrameW != 2 || !rec.retReplay ||
		rec.dynFrameWords[1].Name != "g" || rec.dynFrameWords[1].Pos.Col != 5 || rec.dynFrameWords[0].Name != "" {
		t.Errorf("a seatable read arms the replay with its word table: w=%d words=%v", rec.dynFrameW, rec.dynFrameWords)
	}
	// Two value-semantics applicables beside a word read decline.
	rec.dynFrameW = 0
	if es.noteWordReadReplay(u, rec, []core.Value{core.NewCarrier(core.TFunction), core.NewCarrier(core.TFunction), g}) {
		t.Error("two value applicables cannot be ordered by the flat re-step")
	}
	// An unnamed-param prefix swallowing the read leaves no window.
	rec.nUnnamed, rec.nParams = 2, 2
	if es.noteWordReadReplay(u, rec, []core.Value{g}) {
		t.Error("a read the prefix swallows has no window to seat in")
	}
}

// TestSeatDynFrameWords pins the unit-side seat: an empty table records
// nothing, a table keys the pc and allocates the map once.
func TestSeatDynFrameWords(t *testing.T) {
	var cf CompiledFn
	seatDynFrameWords(&cf, 3, nil)
	if cf.DynFrameWords != nil {
		t.Error("no words, no table")
	}
	seatDynFrameWords(&cf, 3, []DynFrameWord{{Name: "g"}})
	seatDynFrameWords(&cf, 7, []DynFrameWord{{Name: "x"}})
	if len(cf.DynFrameWords) != 2 || cf.DynFrameWords[3][0].Name != "g" || cf.DynFrameWords[7][0].Name != "x" {
		t.Errorf("each replay keys its own pc: %v", cf.DynFrameWords)
	}
}

// TestLamParamContract pins the closure unit's declared param contract: a
// nil signature carries none; a typed param keeps its type; a pattern-only
// param declares Any with its pattern.
func TestLamParamContract(t *testing.T) {
	if lamParamContract(nil) != nil {
		t.Error("a nil signature has no contract")
	}
	pat := core.NewInteger(7)
	sig := &core.Signature{Params: []core.FnParam{{Name: "n", Type: core.TInteger}, {Name: "p", Pattern: &pat}}}
	ps := lamParamContract(sig)
	if ps == nil || len(ps.Types) != 2 || !ps.Types[0].Equal(core.TInteger) || !ps.Types[1].Equal(core.TAny) ||
		ps.Patterns[0] != nil || ps.Patterns[1] == nil {
		t.Errorf("the contract is the declared types, Any for a pattern-only param, patterns kept: %+v", ps)
	}
}

// TestFnResidualReplayReasonArms pins the shared refusal site's three
// verdicts: a closure unit and a trailing apply take none; a fn-typed
// word read the window cannot seat (an event after the read) refuses with
// the NUR123 reason; a seated read passes the accounting.
func TestFnResidualReplayReasonArms(t *testing.T) {
	es, rec, g, _ := wordReadUnit(t)
	u := es.units[len(es.units)-1]
	rec.returns = []*core.Type{core.TAny}
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 5})
	op, _ := es.resolveOperand(g)
	ops := []EmitOperand{op}
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 1); r != "" {
		t.Errorf("a trailing apply takes no verdict here: %q", r)
	}
	rec.closure = true
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 0); r != "" {
		t.Errorf("a closure unit takes no verdict here: %q", r)
	}
	rec.closure = false
	rec.frag = &EmitFragment{events: []EmitEvent{{seq: 1, kind: evCall, call: emitCall{pos: core.SrcPos{Row: 2, Col: 1}}}}}
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 0); !strings.Contains(r, "cannot seat (NUR123)") {
		t.Errorf("a fn-typed read after which the body still runs events cannot seat: %q", r)
	}
	rec.frag = &EmitFragment{}
	if r := es.fnResidualReplayReason(u, rec, []core.Value{g}, ops, 0); r != "" || rec.dynFrameW != 1 {
		t.Errorf("a seated read arms the replay and passes the accounting: %q w=%d", r, rec.dynFrameW)
	}
}

// TestNoteWordReadBodyLocalProducer pins the producer admission (NUR123's
// leftover, 2026-09-05): a value an event of THIS unit produced and a
// body-local `def` bound (`def j (m get "f")  j`) resolves through the
// event, not a slot, and is the unit's word read all the same — named,
// and gradual (never strict); a value neither local nor produced here, or
// produced but bound in an ENCLOSING scope, is not this unit's to seat; a
// binding read both bare and by `/v` is never seated (wordReadName) — that
// mix is the accounting's refusal.
func TestNoteWordReadBodyLocalProducer(t *testing.T) {
	es, rec, _, _ := wordReadUnit(t)
	u := es.units[len(es.units)-1]
	j := core.NewDynamicCarrier(core.TAny)
	es.NoteWordRead(j, "j", core.SrcPos{Row: 1, Col: 40})
	if rec.wordReadNames[j.ID] != "" {
		t.Error("a value neither local nor produced here is not this unit's read")
	}
	es.producedBy[j.ID] = producer{seq: 3}
	es.NoteWordRead(j, "j", core.SrcPos{Row: 1, Col: 40})
	if rec.wordReadNames[j.ID] != "j" || rec.wordReads[j.ID] != 0 || rec.wordReadPos[j.ID].Col != 40 {
		t.Errorf("a body-local producer's value is named at its read, gradual: %v %v %v", rec.wordReadNames, rec.wordReads, rec.wordReadPos)
	}
	k := core.NewDynamicCarrier(core.TAny)
	es.producedBy[k.ID] = producer{seq: 2}
	if u.enclosingBindIDs == nil {
		u.enclosingBindIDs = map[string]bool{}
	}
	u.enclosingBindIDs[k.ID] = true
	es.NoteWordRead(k, "k", core.SrcPos{Row: 1, Col: 44})
	if rec.wordReadNames[k.ID] != "" {
		t.Error("an enclosing-scope binding's value is not this unit's to seat")
	}
	if es.wordReadName(rec, j) != "j" {
		t.Error("the produced value reads as its word")
	}
	es.NoteValRead(j.ID, "n")
	if es.wordReadName(rec, j) != "" {
		t.Error("a binding read both bare and by /v is the accounting's, never a seat")
	}
}

// TestReplayIsBodyTailWordReadAnchor pins the read anchor (NUR123): a
// word-read entry orders by its READ position whatever its producer's seq —
// an event between the producer and the read ran before the dispatch on
// both lanes and is admitted (`def j (m get "f")  def y 1  j`); an event
// after the read reorders and declines (`… j  def y 1`); a produced value
// beside the read keeps its own seq anchor.
func TestReplayIsBodyTailWordReadAnchor(t *testing.T) {
	es := NewEmitState()
	evAt := func(seq, col int) EmitEvent {
		return EmitEvent{seq: seq, kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: col}}}
	}
	j := core.NewDynamicCarrier(core.TAny)
	es.producedBy[j.ID] = producer{seq: 1}
	j.SetPos(core.SrcPos{Row: 1, Col: 50})
	frag := &EmitFragment{events: []EmitEvent{evAt(1, 33), evAt(2, 45)}}
	if !es.replayIsBodyTailAnchored(frag, []core.Value{j}, []bool{true}) {
		t.Error("an event before the read is admitted whatever its seq")
	}
	if es.replayIsBodyTail(frag, []core.Value{j}) {
		t.Error("the same window anchored at its producer declines the later event")
	}
	frag = &EmitFragment{events: []EmitEvent{evAt(1, 33), evAt(2, 60)}}
	if es.replayIsBodyTailAnchored(frag, []core.Value{j}, []bool{true}) {
		t.Error("an event after the read reorders — decline")
	}
	r := core.NewDynamicCarrier(core.TAny)
	es.producedBy[r.ID] = producer{seq: 2}
	frag = &EmitFragment{events: []EmitEvent{evAt(1, 33), evAt(2, 45), evAt(3, 48)}}
	if !es.replayIsBodyTailAnchored(frag, []core.Value{r, j}, []bool{false, true}) {
		t.Error("an event after the produced value's seq but before the read is admitted")
	}
	frag = &EmitFragment{events: []EmitEvent{evAt(1, 33), evAt(2, 45), evAt(3, 55)}}
	if es.replayIsBodyTailAnchored(frag, []core.Value{r, j}, []bool{false, true}) {
		t.Error("an event after both anchors declines")
	}
}

// TestDynApplyHeadNameSeat pins the trailing apply's head-name seam
// (CompiledFn.DynApplyName, the nineteenth increment): openUnitRec reports
// the innermost open record, dynApplyHeadName pairs a BARE READ's name with
// its position and answers zero for anything else, and the lowerer's seat
// keys the emission target's own pc.
func TestDynApplyHeadNameSeat(t *testing.T) {
	es, rec, g, x := wordReadUnit(t)
	if es.openUnitRec() == nil {
		t.Error("a unit is open, so openUnitRec has a record")
	}
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 37})
	if w := es.dynApplyHeadName(rec, g, nil); w.Name != "g" || w.Pos.Col != 37 {
		t.Errorf("a bare read carries its name and the READ's position: %+v", w)
	}
	if w := es.dynApplyHeadName(rec, x, nil); w.Name != "" {
		t.Errorf("an un-noted head seats nothing: %+v", w)
	}
	if w := es.dynApplyHeadName(nil, g, nil); w.Name != "" {
		t.Errorf("no record, no name: %+v", w)
	}

	// The seat itself. The main code has no table pointer at all.
	var cf CompiledFn
	main := &lowerer{code: &cf.Code, debug: &cf.Debug}
	main.emit(OpRet, 0, core.SrcPos{})
	main.seatDynApplyName(DynApplyHead{Name: "g"})
	if cf.DynApplyName != nil {
		t.Error("the main code names no frame binding")
	}
	unit := &lowerer{code: &cf.Code, debug: &cf.Debug, dynApplyName: &cf.DynApplyName}
	unit.seatDynApplyName(DynApplyHead{})
	if cf.DynApplyName != nil {
		t.Error("no bare-read head, no table")
	}
	unit.seatDynApplyName(DynApplyHead{Name: "g", Pos: core.SrcPos{Row: 1, Col: 37}})
	unit.emit(OpRet, 0, core.SrcPos{})
	unit.seatDynApplyName(DynApplyHead{Name: "h"})
	if len(cf.DynApplyName) != 2 || cf.DynApplyName[1].Name != "g" || cf.DynApplyName[1].Pos.Col != 37 || cf.DynApplyName[2].Name != "h" {
		t.Errorf("each apply keys its own pc: %v", cf.DynApplyName)
	}
}

// TestReplayLeadApplicables pins the one-lead rule's count (the twenty-sixth
// increment): a data value is nothing; a fn-typed word read is the lead; a
// GRADUAL word read beside it is discounted (it re-steps as the
// interpreter's own word dispatch) but is the lead when it stands alone (a
// gradual body-local's read, NUR123); a second fn-typed value, word read or
// not, and a gradual value that is NOT a word read (an event result) beside
// the lead both count — `f (g x y)` and `(g (x get "k"))` decline on them —
// and with no fn-typed value the count is the original one-applicable rule.
func TestReplayLeadApplicables(t *testing.T) {
	es, rec, g, x := wordReadUnit(t)
	u := es.units[len(es.units)-1]
	es.NoteWordRead(g, "g", core.SrcPos{Row: 1, Col: 5})
	es.NoteWordRead(x, "x", core.SrcPos{Row: 1, Col: 8})
	one := core.NewInteger(1)
	f := core.NewCarrier(core.TFunction)
	e := core.NewDynamicCarrier(core.TAny)
	for _, c := range []struct {
		name   string
		window []core.Value
		want   int
	}{
		{"data alone", []core.Value{one}, 0},
		{"a fn-typed word read is the lead", []core.Value{one, g}, 1},
		{"a gradual word read beside it is discounted", []core.Value{x, g}, 1},
		{"a gradual word read ALONE is the lead (a body-local's read, NUR123)", []core.Value{x, one}, 1},
		{"two gradual word reads compete", []core.Value{x, x}, 2},
		{"a second fn-typed word read competes", []core.Value{g, g}, 2},
		{"a fn-typed value that is no word read competes", []core.Value{f, g}, 2},
		{"a gradual event result beside the lead competes", []core.Value{e, g}, 2},
		{"a gradual event result alone is the lead", []core.Value{e, one}, 1},
		{"a gradual event result and a gradual read compete", []core.Value{e, x}, 2},
	} {
		if got := replayLeadApplicables(c.window, es.dynFrameWordsFor(u, rec, c.window)); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// TestRecordGradualApplyEventDeclines pins recordGradualApplyEvent's
// declines (the twenty-seventh increment): outside a unit, a nil or
// one-arg signature, a wrong arg or out count, a lead that is concrete, not
// gradual or unidentified, a receiver that is itself a fn value, and an
// operand the recorder cannot resolve. The positive arm is the lang rows'
// (gradual_apply_test.go).
func TestRecordGradualApplyEventDeclines(t *testing.T) {
	es, _, g, x := wordReadUnit(t)
	two := &core.Signature{Args: []*core.Type{core.TReach, core.TAny}}
	oneArg := &core.Signature{Args: []*core.Type{core.TFunction}}
	out := []core.Value{core.NewDynamicCarrier(core.TAny)}
	unknown := core.NewDynamicCarrier(core.TAny)
	unknown.ID = "nowhere"
	pos := core.SrcPos{Row: 1, Col: 9}
	if es.recordGradualApplyEvent(nil, []core.Value{x, g}, out, pos) {
		t.Error("a nil signature declines")
	}
	if es.recordGradualApplyEvent(oneArg, []core.Value{x}, out, pos) {
		t.Error("the one-arg overload is the pending apply's, not this event's")
	}
	if es.recordGradualApplyEvent(two, []core.Value{x}, out, pos) || es.recordGradualApplyEvent(two, []core.Value{x, g}, nil, pos) {
		t.Error("a wrong arg or out count declines")
	}
	if es.recordGradualApplyEvent(two, []core.Value{core.NewInteger(1), g}, out, pos) {
		t.Error("a concrete lead declines")
	}
	if es.recordGradualApplyEvent(two, []core.Value{g, x}, out, pos) {
		t.Error("a fn-typed (not gradual) lead is the pending apply's")
	}
	noID := core.NewDynamicCarrier(core.TAny)
	noID.ID = ""
	if es.recordGradualApplyEvent(two, []core.Value{noID, g}, out, pos) {
		t.Error("an unidentified lead declines")
	}
	if es.recordGradualApplyEvent(two, []core.Value{x, g}, out, pos) {
		t.Error("a fn-value receiver declines")
	}
	if es.recordGradualApplyEvent(two, []core.Value{unknown, core.NewInteger(1)}, out, pos) {
		t.Error("an unresolvable lead declines")
	}
	if es.recordGradualApplyEvent(two, []core.Value{x, unknown}, out, pos) {
		t.Error("an unresolvable receiver declines")
	}
	// The positive arm: a gradual param lead over a literal receiver
	// records the event, lowered to OpCallDynApplyOne.
	before := len(es.frames[len(es.frames)-1])
	if !es.recordGradualApplyEvent(two, []core.Value{x, core.NewInteger(1)}, out, pos) {
		t.Fatal("a gradual lead over a resolvable receiver records")
	}
	evs := es.frames[len(es.frames)-1]
	if len(evs) != before+1 || !evs[len(evs)-1].call.dynApplyOne || !evs[len(evs)-1].call.dynApplyUnquote || evs[len(evs)-1].call.pos != pos {
		t.Errorf("the event carries the one-result and unquote flavours at the apply word's position: %+v", evs[len(evs)-1].call)
	}
	// Outside a unit nothing records.
	top := NewEmitState()
	if top.recordGradualApplyEvent(two, []core.Value{x, core.NewInteger(1)}, out, pos) {
		t.Error("the main program has no single-consumer window")
	}
}
