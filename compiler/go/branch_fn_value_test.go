package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// branch_fn_value_test.go pins the recorder-side helpers NUR159 and NUR187
// added (2026-09-23): the branch event's may-be-fn flags and the seam over
// them (MayBeFn, mayBeFnArgsOf, mayBeFnUnsettled, fnLikeResidual), the
// arg-taking test (fnValueMayTakeArgs), the read poison (noteMayBeFnRead),
// the poly decline, the branch landing's lowering (emitBranchLanding), and
// the statement-boundary note and its crossing test.

// bfState builds a recorder with two branch events: seq 1 produced "br"
// (a fn arm that takes arguments), seq 2 produced "br0" (only-0-arg arms).
func bfState() (*EmitState, core.Value, core.Value) {
	es := NewEmitState()
	es.frames = [][]EmitEvent{{
		{kind: evBranch, br: &emitBranch{pos: core.SrcPos{Row: 1, Col: 5}}, seq: 1},
		{kind: evBranch, br: &emitBranch{pos: core.SrcPos{Row: 1, Col: 30}}, seq: 2},
	}}
	es.producedBy = map[string]producer{"br": {seq: 1, idx: 0}, "br0": {seq: 2, idx: 0}}
	es.eventInfo[1] = eventFlags{mayBeFn: true, mayBeFnArgs: true}
	es.eventInfo[2] = eventFlags{mayBeFn: true}
	br := core.NewCarrier(core.TAny)
	br.ID = "br"
	br0 := core.NewCarrier(core.TAny)
	br0.ID = "br0"
	return es, br, br0
}

func TestFnValueMayTakeArgs(t *testing.T) {
	zero := core.NewFunction(core.FnDefInfo{Name: "one", Signatures: []core.Signature{{BarrierPos: 0}}})
	one := core.NewFunction(core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1}}})
	both := core.NewFunction(core.FnDefInfo{Name: "h", Signatures: []core.Signature{{BarrierPos: 0}, {Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1}}})
	if fnValueMayTakeArgs(zero) {
		t.Error("an only-0-arg fn is settled by the landing")
	}
	if !fnValueMayTakeArgs(one) || !fnValueMayTakeArgs(both) {
		t.Error("a fn with any parameterised overload may take arguments")
	}
	if !fnValueMayTakeArgs(core.NewCarrier(core.TFunction)) {
		t.Error("a carrier's overloads are unknown: it may take arguments")
	}
	// A branch arm's value with no recoverable producer shape falls back to
	// the value's own overloads (the factory case rides the lang rows).
	es := NewEmitState()
	if es.branchArmMayTakeArgs(zero) || !es.branchArmMayTakeArgs(one) {
		t.Error("an unproduced arm value is classified by its own overloads")
	}
}

func TestMayBeFnHelpers(t *testing.T) {
	var nilES *EmitState
	if nilES.MayBeFn("br") || nilES.mayBeFnArgsOf("br") || nilES.mayBeFnUnsettled(core.Value{ID: "br"}) || nilES.crossesBoundary(core.Value{ID: "br"}, nil) {
		t.Error("a nil state answers false everywhere")
	}
	es, br, br0 := bfState()
	if es.MayBeFn("") || es.MayBeFn("nothing") || es.mayBeFnArgsOf("") || es.mayBeFnArgsOf("nothing") {
		t.Error("an empty or unproduced id is no branch result")
	}
	if !es.MayBeFn("br") || !es.MayBeFn("br0") {
		t.Error("both branch results may be fns")
	}
	if !es.mayBeFnArgsOf("br") || es.mayBeFnArgsOf("br0") {
		t.Error("only the arg-taking arm's result is unsettled")
	}
	if !es.mayBeFnUnsettled(br) || es.mayBeFnUnsettled(br0) {
		t.Error("mayBeFnUnsettled follows the args flag")
	}
	if !es.fnLikeResidual(br) || es.fnLikeResidual(br0) || es.fnLikeResidual(core.NewInteger(1)) {
		t.Error("the residual arms read an unsettled branch result as a fn-like entry, a settled one and a literal as data")
	}
	if !es.fnLikeResidual(core.NewCarrier(core.TFunction)) {
		t.Error("a fn-typed carrier is fn-like as before")
	}
	quoted := br
	quoted.Quoted = true
	if es.mayBeFnUnsettled(quoted) {
		t.Error("a quoted value is data")
	}
	// A paren placed it and no enclosing paren re-stepped it: settled.
	es.reg = &core.Registry{Check: &core.CheckState{ParenPlacedFnIDs: map[string]bool{"br": true}}}
	if es.mayBeFnUnsettled(br) {
		t.Error("a paren-placed result is the interpreter's own park")
	}
	es.reg.Check.ParenReSteppedFnIDs = map[string]bool{"br": true}
	if !es.mayBeFnUnsettled(br) {
		t.Error("an enclosing paren's re-step undoes the placement")
	}
	// A `/v` read delivered it inert: settled.
	es.reg = nil
	es.valReadNoted = map[string]bool{"br": true}
	if es.mayBeFnUnsettled(br) {
		t.Error("a /v read delivers the value inert")
	}
}

func TestPolyCallDeclineReasonMayBeFn(t *testing.T) {
	es, br, br0 := bfState()
	want := "a dispatch collected a branch result whose fn arm the interpreter re-steps first (NUR159)"
	for _, v := range []core.Value{br, br0} {
		if got := es.polyCallDeclineReason("add", []core.Value{core.NewInteger(1), v}, nil, core.SrcPos{}); got != want {
			t.Errorf("%s: a branch result under a poly declines, got %q", v.ID, got)
		}
	}
	quoted := br
	quoted.Quoted = true
	if got := es.polyCallDeclineReason("add", []core.Value{quoted}, nil, core.SrcPos{}); got != "" {
		t.Errorf("a quoted operand is data: %q", got)
	}
	es.units[0].pendingApply = []pendingApply{{id: "br"}}
	if got := es.polyCallDeclineReason("apply", []core.Value{br}, nil, core.SrcPos{}); got != "" {
		t.Errorf("an apply-owned operand is the apply word's: %q", got)
	}
	if got := es.polyCallDeclineReason("add", []core.Value{core.NewInteger(1), core.NewInteger(2)}, nil, core.SrcPos{}); got != "" {
		t.Errorf("plain operands: %q", got)
	}
}

func TestNoteMayBeFnRead(t *testing.T) {
	var nilES *EmitState
	nilES.noteMayBeFnRead("br", "x") // no panic: an unknown id is nothing to poison
	es, _, _ := bfState()
	es.noteMayBeFnRead("br0", "r")
	if es.armReadCompileFailure != "" {
		t.Errorf("a settled (only-0-arg) result's read is fine: %q", es.armReadCompileFailure)
	}
	es.noteMayBeFnRead("br", "x")
	if !strings.Contains(es.armReadCompileFailure, "read of `x`") || !strings.Contains(es.armReadCompileFailure, "NUR159") {
		t.Errorf("an arg-taking result's read poisons the placement gate: %q", es.armReadCompileFailure)
	}
	es.noteMayBeFnRead("br", "y")
	if !strings.Contains(es.armReadCompileFailure, "read of `x`") {
		t.Error("the first poison stands")
	}
	es, _, _ = bfState()
	es.trapAt = 3
	es.noteMayBeFnRead("br", "x")
	if es.armReadCompileFailure != "" {
		t.Error("after a terminal trap the read is unreachable: no poison")
	}
}

func TestEmitBranchLanding(t *testing.T) {
	(&lowerer{}).emitBranchLanding(&EmitEvent{seq: 7}) // no recorder: nothing to read
	es := NewEmitState()
	var code []Instr
	var debug []core.SrcPos
	lw := &lowerer{es: es, code: &code, debug: &debug}
	lw.emitBranchLanding(&EmitEvent{seq: 7})
	if len(code) != 0 {
		t.Errorf("an unnoted branch emits no landing: %v", code)
	}
	es.landingAfter = map[int]core.SrcPos{7: {Row: 2, Col: 3}}
	lw.emitBranchLanding(&EmitEvent{seq: 7})
	if len(code) != 1 || code[0].Op != OpReStepLanding || debug[0].Row != 2 {
		t.Errorf("a noted branch emits the landing at its position: %v %v", code, debug)
	}
	if _, still := es.landingAfter[7]; still {
		t.Error("the note is consumed")
	}
	lw.emitBranchLanding(&EmitEvent{seq: 7})
	if len(code) != 1 {
		t.Error("a consumed note lands nothing twice")
	}
	// A function word noted after the landing rides beside the op
	// (LandingWords, NUR190) — when the target has a table; the arg carries
	// bit 1 so the VM's landing knows to walk.
	es.landingAfter = map[int]core.SrcPos{7: {Row: 2, Col: 3}}
	es.landingNext = map[int]core.LandingNext{7: core.LandingNextWord}
	es.landingBeneath = map[int]bool{}
	es.landingWord = map[int]LandingWord{7: {Name: "z", Pos: core.SrcPos{Row: 2, Col: 9}}}
	lw.emitBranchLanding(&EmitEvent{seq: 7})
	if len(code) != 2 || code[1].Arg != 3 {
		t.Errorf("the word's landing carries the walk bit: %v", code)
	}
	var tbl map[int]LandingWord
	lw.landingWords = &tbl
	es.landingAfter = map[int]core.SrcPos{7: {Row: 2, Col: 3}}
	lw.emitBranchLanding(&EmitEvent{seq: 7})
	if got := tbl[2]; got.Name != "z" || got.Pos.Col != 9 {
		t.Errorf("the word is seated at the landing's pc: %v", tbl)
	}
	lw.seatLandingWord(7, LandingWord{})
	if len(tbl) != 1 {
		t.Error("no word, nothing seated")
	}
}

func TestStatementBoundaryCrossing(t *testing.T) {
	inactive := &EmitState{}
	inactive.NoteStatementEnd(core.SrcPos{Row: 1, Col: 1})
	if len(inactive.stmtEnds) != 0 {
		t.Error("an inactive recorder notes nothing")
	}
	es := NewEmitState()
	es.NoteStatementEnd(core.SrcPos{})
	if len(es.stmtEnds) != 0 {
		t.Error("an unknown position is nothing to order by")
	}
	es.frames = [][]EmitEvent{{
		{kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: 5}}, seq: 1},
		{kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: 20}}, seq: 2},
		{kind: evCall, call: emitCall{}, seq: 3},
	}}
	es.producedBy = map[string]producer{"a": {seq: 1}, "b": {seq: 2}, "c": {seq: 3}}
	a, b, c := core.Value{ID: "a"}, core.Value{ID: "b"}, core.Value{ID: "c"}
	if es.crossesBoundary(a, []core.Value{b}) {
		t.Error("no boundary recorded: nothing crosses")
	}
	es.NoteStatementEnd(core.SrcPos{Row: 1, Col: 10, Src: ";"})
	if !es.crossesBoundary(a, []core.Value{b}) {
		t.Error("b was written past the boundary that follows a")
	}
	if es.crossesBoundary(b, []core.Value{a}) {
		t.Error("the boundary precedes b: a is not past it")
	}
	if es.crossesBoundary(a, []core.Value{c}) || es.crossesBoundary(c, []core.Value{b}) {
		t.Error("an unknown position proves nothing")
	}
	if es.crossesBoundary(a, []core.Value{c, b}) != true {
		t.Error("one proven crossing among the rest is enough")
	}
	// A def-bound read was written where the NAME is, which is not
	// recorded: unknown, so it never proves a crossing.
	es.defReads = map[string]string{"a": "x"}
	if es.crossesBoundary(a, []core.Value{b}) {
		t.Error("a def read's position is unknown")
	}
	if srcPosBefore(core.SrcPos{Row: 2, Col: 1}, core.SrcPos{Row: 1, Col: 9}) || !srcPosBefore(core.SrcPos{Row: 1, Col: 9}, core.SrcPos{Row: 2, Col: 1}) {
		t.Error("positions order by row, then column")
	}
}
