package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// named_fn_candidates_test.go pins the recorder-side helpers of the named fn
// value's candidates (2026-09-23: NUR186 closed, NUR187's fn-unit and
// word-after halves, NUR188 and NUR189 found and closed): the landing note's
// what-follows and values-beneath facts and the op argument they lower to,
// the unit-tail test for a concrete named fn, the member-read and wreckage
// arms of the poly decline, the word-after crossing rule and the trailing
// arm's placed check.

func nfState() (*EmitState, core.Value) {
	es := NewEmitState()
	es.frames = [][]EmitEvent{{{kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: 5}}, seq: 1}}}
	es.producedBy = map[string]producer{"v": {seq: 1, idx: 0}}
	v := core.NewCarrier(core.TFunction)
	v.ID = "v"
	return es, v
}

func TestNoteLandingNextGatesAndMerge(t *testing.T) {
	es, v := nfState()
	es.eventInfo = map[int]eventFlags{1: {variadicResult: true}}
	es.NoteLandingNext(v, core.LandingNextWord, false, core.Value{})
	if len(es.landingNext) != 0 {
		t.Error("a variadic result has no landing to describe: its paths deliver different tails")
	}
	es.eventInfo = nil
	// Repeated notes (a paren's collapse re-steps what its group parked):
	// a word at any step wins, then the tape's end, then a boundary; values
	// beneath at any step stay noted.
	for _, tc := range []struct {
		name  string
		notes []core.LandingNext
		want  core.LandingNext
	}{
		{"boundary then word", []core.LandingNext{core.LandingNextBoundary, core.LandingNextWord}, core.LandingNextWord},
		{"word then end", []core.LandingNext{core.LandingNextWord, core.LandingNextEnd}, core.LandingNextWord},
		{"end then boundary", []core.LandingNext{core.LandingNextEnd, core.LandingNextBoundary}, core.LandingNextEnd},
		{"boundary then end", []core.LandingNext{core.LandingNextBoundary, core.LandingNextEnd}, core.LandingNextEnd},
		{"boundary twice", []core.LandingNext{core.LandingNextBoundary, core.LandingNextBoundary}, core.LandingNextBoundary},
		{"value then word", []core.LandingNext{core.LandingNextValue, core.LandingNextWord}, core.LandingNextWord},
		{"value then end", []core.LandingNext{core.LandingNextValue, core.LandingNextEnd}, core.LandingNextEnd},
		{"boundary then value", []core.LandingNext{core.LandingNextBoundary, core.LandingNextValue}, core.LandingNextValue},
		{"value twice", []core.LandingNext{core.LandingNextValue, core.LandingNextValue}, core.LandingNextValue},
	} {
		es.landingNext, es.landingBeneath = nil, nil
		for _, n := range tc.notes {
			es.NoteLandingNext(v, n, false, core.Value{})
		}
		if got := es.landingNext[1]; got != tc.want {
			t.Errorf("%s: merged note = %v, want %v", tc.name, got, tc.want)
		}
	}
	es.landingNext, es.landingBeneath = nil, nil
	es.NoteLandingNext(v, core.LandingNextWord, true, core.Value{})
	es.NoteLandingNext(v, core.LandingNextWord, false, core.Value{})
	if !es.landingBeneath[1] {
		t.Error("values beneath at any step are the residual arms' apply")
	}
}

func TestLandingArg(t *testing.T) {
	var nilES *EmitState
	if nilES.landingArg(1, true) != 0 {
		t.Error("a nil state lands nothing")
	}
	inactive := &EmitState{}
	inactive.NoteLandingNext(core.Value{ID: "v"}, core.LandingNextWord, false, core.Value{})
	if len(inactive.landingNext) != 0 {
		t.Error("an inactive recorder notes nothing")
	}
	es, v := nfState()
	es.NoteLandingNext(core.Value{ID: "unproduced"}, core.LandingNextWord, false, core.Value{})
	if len(es.landingNext) != 0 {
		t.Error("a value with no producing event has no landing to note")
	}
	z := core.NewWord("z")
	for _, tc := range []struct {
		name      string
		next      core.LandingNext
		beneath   bool
		frameTail bool
		word      core.Value
		want      int
	}{
		{"a word follows: the candidate, and the word for the walk", core.LandingNextWord, false, false, z, 3},
		{"a word follows, inside a fn frame", core.LandingNextWord, false, true, z, 3},
		{"a word follows and none was noted: the candidate alone", core.LandingNextWord, false, false, core.Value{}, 1},
		{"the tape ends inside a fn frame: the tail markers", core.LandingNextEnd, false, true, core.Value{}, 1},
		{"the tape ends at the main program", core.LandingNextEnd, false, false, core.Value{}, 0},
		{"a boundary follows", core.LandingNextBoundary, false, true, core.Value{}, 0},
		{"a value-bound word follows: the residual arm's collection", core.LandingNextValue, false, true, core.Value{}, 0},
		{"values beneath: the residual arm's", core.LandingNextWord, true, true, z, 0},
	} {
		es.landingNext, es.landingBeneath, es.landingWord = nil, nil, nil
		es.NoteLandingNext(v, tc.next, tc.beneath, tc.word)
		if got := es.landingArg(1, tc.frameTail); got != tc.want {
			t.Errorf("%s: arg = %d, want %d", tc.name, got, tc.want)
		}
	}
	// The word rides with the note once, under the first note that carries
	// one; a nil state has none.
	es.landingNext, es.landingBeneath, es.landingWord = nil, nil, nil
	es.NoteLandingNext(v, core.LandingNextWord, false, z)
	es.NoteLandingNext(v, core.LandingNextWord, false, core.NewWord("y"))
	if got := es.landingWordAt(1); got.Name != "z" {
		t.Errorf("the first word noted stays: %v", got)
	}
	if nilES.landingWordAt(1).Name != "" {
		t.Error("a nil state notes no word")
	}
}

func TestNamedFnUnmatchedAtTail(t *testing.T) {
	es := NewEmitState()
	oneArg := []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1}}
	inc := core.NewFunction(core.FnDefInfo{Name: "inc", Signatures: oneArg})
	inc.ID = "inc-1"
	if !es.namedFnUnmatchedAtTail(inc) {
		t.Error("a named fn with only an arg-taking overload is the interpreter's raise")
	}
	for name, v := range map[string]core.Value{
		"anonymous": core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: oneArg}),
		"macro":     core.NewFunction(core.FnDefInfo{Name: "m", Macro: true, Signatures: oneArg}),
		"no sigs":   core.NewFunction(core.FnDefInfo{Name: "h"}),
		"a 0-arg":   core.NewFunction(core.FnDefInfo{Name: "z", Signatures: []core.Signature{{BarrierPos: 0}}}),
		"not a fn":  core.NewInteger(1),
		"a carrier": core.NewCarrier(core.TFunction),
	} {
		if es.namedFnUnmatchedAtTail(v) {
			t.Errorf("%s: not the raise", name)
		}
	}
	quoted := inc
	quoted.Quoted = true
	if es.namedFnUnmatchedAtTail(quoted) {
		t.Error("a quoted value is data")
	}
	es.valReadNoted = map[string]bool{"inc-1": true}
	if es.namedFnUnmatchedAtTail(inc) {
		t.Error("a /v read delivers the value inert")
	}
	es.valReadNoted = nil
	es.reg = &core.Registry{Check: &core.CheckState{ParenPlacedFnIDs: map[string]bool{"inc-1": true}}}
	if es.namedFnUnmatchedAtTail(inc) {
		t.Error("a paren-placed value is data")
	}
}

func TestPolyCallDeclineReasonMemberReadAndWreckage(t *testing.T) {
	es, v := nfState()
	inc := core.NewFunction(core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1}}})
	es.memberFnReads = map[string]core.Value{"v": inc}
	after := core.SrcPos{Row: 1, Col: 9}
	before := core.SrcPos{Row: 1, Col: 2}
	if got := es.polyCallDeclineReason("typeof", []core.Value{v}, nil, after); !strings.Contains(got, "NUR188") {
		t.Errorf("a member read written before the word is re-stepped first: %q", got)
	}
	if got := es.polyCallDeclineReason("typeof", []core.Value{v}, nil, before); got != "" {
		t.Errorf("a member read written after the word is its operand: %q", got)
	}
	if got := es.polyCallDeclineReason("typeof", []core.Value{v}, nil, core.SrcPos{}); got != "" {
		t.Errorf("an unknown word position proves nothing: %q", got)
	}
	quoted := v
	quoted.Quoted = true
	if got := es.polyCallDeclineReason("typeof", []core.Value{quoted}, nil, after); got != "" {
		t.Errorf("a quoted member read is data: %q", got)
	}
	es.valReadNoted = map[string]bool{"v": true}
	if got := es.polyCallDeclineReason("typeof", []core.Value{v}, nil, after); got != "" {
		t.Errorf("a /v-read member is data: %q", got)
	}
	es.valReadNoted = nil
	es.reg = &core.Registry{Check: &core.CheckState{ParenPlacedFnIDs: map[string]bool{"v": true}}}
	if got := es.polyCallDeclineReason("typeof", []core.Value{v}, nil, after); got != "" {
		t.Errorf("a paren-placed member read is data the word collects (`(m dot a) eq (m dot a)`): %q", got)
	}
	es.reg.Check.ParenReSteppedFnIDs = map[string]bool{"v": true}
	if got := es.polyCallDeclineReason("typeof", []core.Value{v}, nil, after); !strings.Contains(got, "NUR184") {
		t.Errorf("an enclosing paren's re-step undoes the placement and is NUR184's arm: %q", got)
	}
	es.reg = nil
	wreck := core.NewInteger(1)
	wreck.FailedDispatch = true
	if got := es.polyCallDeclineReason("add", []core.Value{wreck}, nil, after); !strings.Contains(got, "NUR186") {
		t.Errorf("dispatch wreckage declines: %q", got)
	}
}

func TestCrossesBoundaryWordNext(t *testing.T) {
	es, v := nfState()
	rest := []core.Value{{ID: "later"}}
	if es.crossesBoundary(v, rest) {
		t.Error("nothing noted: no crossing")
	}
	es.NoteLandingNext(v, core.LandingNextWord, true, core.Value{})
	if !es.crossesBoundary(v, rest) {
		t.Error("a word after the value: every later entry was pushed after its re-step")
	}
	if !es.wordFollowsLanding(v) {
		t.Error("the word-after fact on its own")
	}
	if es.crossesStatementEnd(v, rest) {
		t.Error("the positional rule alone sees no statement boundary: the lead arm keeps applying (NUR190, pending)")
	}
	if es.crossesBoundary(v, nil) {
		t.Error("nothing above the value: nothing to cross")
	}
	es.landingNext, es.landingBeneath = nil, nil
	es.NoteLandingNext(v, core.LandingNextEnd, false, core.Value{})
	if es.crossesBoundary(v, rest) {
		t.Error("the tape's end is not a word")
	}
	es.landingNext, es.landingBeneath = nil, nil
	es.NoteLandingNext(v, core.LandingNextValue, false, core.Value{})
	if es.crossesBoundary(v, rest) {
		t.Error("a value-bound word is collected, not a boundary: `7 m.f k` is [7 6] on both lanes")
	}
	// A def-bound READ carries the value's note but was written where the
	// name is: neither rule applies to it.
	es.landingNext, es.landingBeneath = nil, nil
	es.NoteLandingNext(v, core.LandingNextWord, false, core.Value{})
	es.defReads = map[string]string{"v": "x"}
	if es.crossesBoundary(v, rest) {
		t.Error("a def read is excluded from the word rule")
	}
}

func TestTrailingApplyPlacedDeclines(t *testing.T) {
	es, v := nfState()
	lw := &lowerer{vm: []vmSlot{{seq: 1, idx: 0}}}
	residual := []core.Value{core.NewInteger(7), v}
	if rot, ok := es.trailingApply(lw, residual); !ok || len(rot) != 2 || rot[0].ID != "v" {
		t.Fatalf("a trailing fn-typed carrier over one literal is the rotated apply, got %v %v", rot, ok)
	}
	es.reg = &core.Registry{Check: &core.CheckState{ParenPlacedFnIDs: map[string]bool{"v": true}}}
	if _, ok := es.trailingApply(lw, residual); ok {
		t.Error("a paren-placed value is data on both lanes (NUR189)")
	}
	es.appliedByWord = map[string]bool{"v": true}
	if _, ok := es.trailingApply(lw, residual); !ok {
		t.Error("an `apply` word after the placed value dispatches it on purpose")
	}
	es.appliedByWord = nil
	es.reg.Check.ParenReSteppedFnIDs = map[string]bool{"v": true}
	if _, ok := es.trailingApply(lw, residual); !ok {
		t.Error("an enclosing paren's re-step undoes the placement")
	}
}

// TestClosureResidualUnappliedFnSkipsPlaced: a value a user paren placed in
// a closure body's residual, or a user fn's returned closure parked where
// it landed, is data (the park rule) and no unapplied apply; a fn-typed
// carrier over a value beneath still is.
func TestClosureResidualUnappliedFnSkipsPlaced(t *testing.T) {
	es := NewEmitState()
	carrier := core.NewCarrier(core.TFunction)
	carrier.ID = "c"
	elem := core.NewInteger(1)
	if !es.closureResidualHasUnappliedFn([]core.Value{elem, carrier}, nil) {
		t.Error("a fn-typed carrier over the element is an unapplied apply")
	}
	es.reg = &core.Registry{Check: &core.CheckState{ParenPlacedFnIDs: map[string]bool{"c": true}}}
	if es.closureResidualHasUnappliedFn([]core.Value{elem, carrier}, nil) {
		t.Error("a paren-placed value is data")
	}
	es.reg.Check.ParenReSteppedFnIDs = map[string]bool{"c": true}
	if !es.closureResidualHasUnappliedFn([]core.Value{elem, carrier}, nil) {
		t.Error("an enclosing paren's re-step undoes the placement")
	}
	es.reg = nil
	es.frames = [][]EmitEvent{{{kind: evCallUser, uc: emitUserCall{nout: 1}, seq: 1}}}
	es.producedBy = map[string]producer{"c": {seq: 1, idx: 0}}
	if es.closureResidualHasUnappliedFn([]core.Value{elem, carrier}, nil) {
		t.Error("a user fn's returned closure parked where it landed is data")
	}
	// At a unit's finish the producing event sits in the captured fragment,
	// not on the frame stack.
	es.frames = nil
	frag := &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{nout: 1}, seq: 1}}}
	if es.closureResidualHasUnappliedFn([]core.Value{elem, carrier}, frag) {
		t.Error("the fragment's user call parks its returned closure")
	}
	es.reg = &core.Registry{Check: &core.CheckState{ParenReSteppedFnIDs: map[string]bool{"c": true}}}
	if !es.closureResidualHasUnappliedFn([]core.Value{elem, carrier}, frag) {
		t.Error("an enclosing paren's re-step applies the parked closure (`[add (2 (mk 1))]`)")
	}
	// A closure a paren-apply produced (a call event, not a user fn's
	// return) is re-stepped again over what follows: the placement record
	// alone does not make it data (the module decliner's chain).
	frag = &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{word: wordDynApply, nout: 1}, seq: 1}}}
	es.reg = &core.Registry{Check: &core.CheckState{ParenPlacedFnIDs: map[string]bool{"c": true}}}
	if !es.closureResidualHasUnappliedFn([]core.Value{elem, carrier}, frag) {
		t.Error("a paren-apply's produced closure is an apply over the element")
	}
}
