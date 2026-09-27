package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNoteClosureReStep pins the closure call's re-step flag (NUR317): a
// whole-residual word (`do`) whose compiled body may leave a fn value that
// none of the dispatch's modelled outputs may be — the check pass already
// stepped it inside the body, so nothing recorded after the call applies it.
func TestNoteClosureReStep(t *testing.T) {
	resid := &core.Signature{Callable: &core.CallableSpec{BodyOut: core.BodyOutResidual}}
	one := &core.Signature{Callable: &core.CallableSpec{BodyOut: 1}}
	es := NewEmitState()
	es.fnRecs = append(es.fnRecs, &fnUnitRec{mayReturnFn: true}, &fnUnitRec{})
	ints := []core.Value{core.NewInteger(7), core.NewInteger(5)}
	fn := []core.Value{core.NewFunction(core.FnDefInfo{Name: "g", Signatures: []core.Signature{{}}})}
	for seq, c := range []struct {
		why  string
		sig  *core.Signature
		unit int
		outs []core.Value
		want bool
	}{
		{"a do whose body may leave a fn the model stepped", resid, 0, ints, true},
		{"a word with no code body", &core.Signature{}, 0, ints, false},
		{"a word that is not whole-residual", one, 0, ints, false},
		{"no unit", resid, -1, ints, false},
		{"an unknown unit", resid, 9, ints, false},
		{"a body that leaves no fn", resid, 1, ints, false},
		{"a modelled fn output the program re-steps itself", resid, 0, fn, false},
	} {
		es.noteClosureReStep(c.sig, c.unit, c.outs, seq)
		if got := es.eventInfo[seq].reStepResults; got != c.want {
			t.Errorf("%s: reStepResults = %v, want %v", c.why, got, c.want)
		}
	}
}

// TestValueMayBeFn pins the modelled-value test the re-step flag reads: a fn
// value, a fn-typed carrier, a gradual value whose bound admits one, a union
// with a Function alternative — and not plain data.
func TestValueMayBeFn(t *testing.T) {
	dyn := core.NewCarrier(core.TAny)
	dyn.Dynamic = true
	union := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TFunction)})
	union.Carrier = true
	for _, c := range []struct {
		why  string
		v    core.Value
		want bool
	}{
		{"a fn value", core.NewFunction(core.FnDefInfo{Name: "g", Signatures: []core.Signature{{}}}), true},
		{"a fn-typed carrier", core.NewCarrier(core.TFunction), true},
		{"a gradual Any", dyn, true},
		{"a union with a Function alternative", union, true},
		{"an Integer carrier", core.NewCarrier(core.TInteger), false},
		{"a literal", core.NewInteger(5), false},
	} {
		if got := valueMayBeFn(c.v); got != c.want {
			t.Errorf("%s: valueMayBeFn = %v, want %v", c.why, got, c.want)
		}
	}
}

// TestBodyReStepNote pins the closure probe's note for the dyn-body backstop:
// read once, keyed by the body value.
func TestBodyReStepNote(t *testing.T) {
	es := NewEmitState()
	body := core.NewList([]core.Value{core.NewInteger(1)})
	body.ID = "body"
	if es.takeBodyReStep(body) {
		t.Fatal("no note yet")
	}
	es.noteBodyReStep(body)
	if !es.takeBodyReStep(body) || es.takeBodyReStep(body) {
		t.Error("the note is read once")
	}
}

// TestReStepsResults pins the lowering's decisions (SigRef.ReStep): a
// flagged call re-steps, keeping its seat count unless its result is a run;
// a run the region plan seats beneath its prefix with outputs that may hold
// a fn is a candidate the seat itself flags.
func TestReStepsResults(t *testing.T) {
	if (&lowerer{}).reStepsResults(1) || (&lowerer{}).regionReStepCandidate(1) {
		t.Error("no recorder, no re-step")
	}
	es := NewEmitState()
	es.eventInfo[1] = eventFlags{reStepResults: true}
	es.eventInfo[2] = eventFlags{outsMayBeFn: true}
	es.eventInfo[3] = eventFlags{variadicResult: true}
	lw := &lowerer{es: es}
	if !lw.reStepsResults(1) || lw.reStepsResults(2) || lw.reStepsResults(3) {
		t.Error("only the flagged call re-steps")
	}
	if lw.regionReStepCandidate(2) {
		t.Error("no region plan, no candidate")
	}
	lw.regionPrefixSeq = 2
	if !lw.regionReStepCandidate(2) || lw.regionReStepCandidate(1) {
		t.Error("the planned run whose outputs may be a fn is the candidate")
	}
	if got := lw.reStepOut(1, 2); got != 2 {
		t.Errorf("a fixed call keeps its seat count, got %d", got)
	}
	if got := lw.reStepOut(2, 2); got != -1 {
		t.Errorf("the prefix's region takes any count, got %d", got)
	}
	if got := lw.reStepOut(3, 2); got != -1 {
		t.Errorf("a variadic result takes any count, got %d", got)
	}
}

// TestDisassembleReStep: a `do` whose results the VM re-steps says so in the
// disassembly, and a plain call of the same word does not.
func TestDisassembleReStep(t *testing.T) {
	sig := &core.Signature{Args: []*core.Type{core.TList}}
	p := &Program{
		Code: []Instr{{Op: OpCallNative, Arg: 0}, {Op: OpCallNative, Arg: 1}},
		Sigs: []SigRef{{Word: "do", Sig: sig, ReStep: true, ReStepOut: 2}, {Word: "do", Sig: sig}},
	}
	lines := strings.Split(p.Disassemble(), "\n")
	const note = " [results re-stepped]"
	if !strings.HasSuffix(lines[0], "; do (List)"+note) || strings.Contains(lines[1], note) {
		t.Errorf("want the note on the re-stepped call alone, got %q / %q", lines[0], lines[1])
	}
}
