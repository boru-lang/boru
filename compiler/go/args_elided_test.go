package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestArgsElidedFrameAndSetter pins the recorder half of NUR346: a unit is
// marked args-elided only through SetUnitArgsElided (an index outside the
// table is a no-op), and ArgsElidedFrame answers for the INNERMOST open unit
// alone — false on a nil state, with no unit open, over an open index the
// table does not hold, and for an open unit that pushes the real list.
func TestArgsElidedFrameAndSetter(t *testing.T) {
	var none *EmitState
	if none.ArgsElidedFrame() {
		t.Fatal("a nil state has no args-elided frame")
	}
	es := NewEmitState()
	if es.ArgsElidedFrame() {
		t.Fatal("no unit open, no args-elided frame")
	}
	es.SetUnitArgsElided(-1)
	es.SetUnitArgsElided(0) // no unit recorded yet: both no-ops
	es.fnRecs = append(es.fnRecs, &fnUnitRec{}, &fnUnitRec{})
	es.openUnitRecs = append(es.openUnitRecs, 0)
	if es.ArgsElidedFrame() {
		t.Fatal("an open unit that pushes the real list is no args-elided frame")
	}
	es.SetUnitArgsElided(0)
	if !es.fnRecs[0].argsElided || es.fnRecs[1].argsElided {
		t.Fatal("the setter marks exactly the named unit")
	}
	if !es.ArgsElidedFrame() {
		t.Fatal("the innermost open unit is args-elided")
	}
	es.openUnitRecs = append(es.openUnitRecs, 1)
	if es.ArgsElidedFrame() {
		t.Fatal("a unit opened inside an args-elided one answers for itself")
	}
	es.openUnitRecs = append(es.openUnitRecs, 7)
	if es.ArgsElidedFrame() {
		t.Fatal("an open index the table does not hold answers false")
	}
}

// TestParamSpecArgsElided: a lambda's contract carries its sig's args-list
// decision (lamParamContract), and a token body — no contract — elides
// nothing (it runs in the caller's frame and pushes no list).
func TestParamSpecArgsElided(t *testing.T) {
	if paramSpecArgsElided(nil) {
		t.Fatal("a token body (no contract) elides nothing")
	}
	meta := &core.FnFrameMeta{Name: "lam", ArgsElided: true}
	sig := &core.Signature{Params: []core.FnParam{{Name: "x", Type: core.TInteger}}, Impl: &core.BoruImpl{FnFrame: meta}}
	if ps := lamParamContract(sig); !paramSpecArgsElided(ps) {
		t.Fatal("an elided sig's contract carries the decision")
	}
	meta.ArgsElided = false
	if ps := lamParamContract(sig); paramSpecArgsElided(ps) {
		t.Fatal("a real-args sig's contract does not")
	}
}
