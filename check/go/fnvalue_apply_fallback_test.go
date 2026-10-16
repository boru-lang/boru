package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fakeApplyRec overrides just the name-route apply on the inactive
// recorder, so the fallback's success tail is testable without a real
// EmitState (compiler is above this module).
type fakeApplyRec struct {
	core.EmitRecorder
	ok bool
}

func (f fakeApplyRec) RecordDynApplyName(string, []core.Value, core.Value, *core.Value, core.SrcPos) bool {
	return f.ok
}

// TestRecordFnValueApplyFallbackArms pins the §4.3 fallback's guards: the
// arity/out-count gates, the carrier-out requirement (a concrete out must
// not be freshened away), the non-concrete-capture requirement, the
// binding-shape gates (anonymous, unquoted, single own sig), and the
// success tail's freshened carrier.
func TestRecordFnValueApplyFallbackArms(t *testing.T) {
	r, _ := core.NewRegistry()
	rec := fakeApplyRec{EmitRecorder: core.TheInactiveEmit, ok: true}
	arg := []core.Value{core.NewInteger(5)}
	carrierOut := []core.Value{core.NewCarrier(core.TAny)}
	caps := []core.CapturedBinding{{Name: "g", Value: core.NewCarrier(core.TFunction)}}

	if _, ok := recordFnValueApplyFallback(rec, r, "h", caps, nil, carrierOut, core.SrcPos{}); ok {
		t.Error("a 0-arg call must decline")
	}
	if _, ok := recordFnValueApplyFallback(rec, r, "h", caps, arg, nil, core.SrcPos{}); ok {
		t.Error("a 0-out call must decline")
	}
	if _, ok := recordFnValueApplyFallback(rec, r, "h", caps, arg, []core.Value{core.NewInteger(9)}, core.SrcPos{}); ok {
		t.Error("a concrete out must decline (freshening would erase its value)")
	}
	concreteCaps := []core.CapturedBinding{{Name: "g", Value: core.NewInteger(1)}}
	if _, ok := recordFnValueApplyFallback(rec, r, "h", concreteCaps, arg, carrierOut, core.SrcPos{}); ok {
		t.Error("all-concrete captures keep the unit call")
	}
	if _, ok := recordFnValueApplyFallback(rec, r, "nope", caps, arg, carrierOut, core.SrcPos{}); ok {
		t.Error("an unbound name must decline")
	}
	core.InstallDef(r, "notfn", core.NewInteger(3))
	if _, ok := recordFnValueApplyFallback(rec, r, "notfn", caps, arg, carrierOut, core.SrcPos{}); ok {
		t.Error("a non-fn binding must decline")
	}
	named := core.NewFunction(core.FnDefInfo{Name: "n", Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
	}}})
	r.Defs.Push("named", named)
	if _, ok := recordFnValueApplyFallback(rec, r, "named", caps, arg, carrierOut, core.SrcPos{}); ok {
		t.Error("a named (non-anonymous) binding must decline")
	}
	multi := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{
		{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1},
		{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TAny}, BarrierPos: -1},
	}})
	r.Defs.Push("multi", multi)
	if _, ok := recordFnValueApplyFallback(rec, r, "multi", caps, arg, carrierOut, core.SrcPos{}); ok {
		t.Error("a multi-own-sig binding must decline")
	}
	anon := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
	}}})
	r.Defs.Push("h", anon)
	fresh, ok := recordFnValueApplyFallback(rec, r, "h", caps, arg, carrierOut, core.SrcPos{})
	if !ok {
		t.Fatal("the qualifying shape must record")
	}
	if !core.IsCarrier(&fresh) || core.IDOf(&fresh) == core.IDOf(&carrierOut[0]) {
		t.Errorf("the out must be a FRESH carrier (one identity per call site): %v", fresh)
	}
	// The recorder declining leaves the caller on the unit-call path.
	recNo := fakeApplyRec{EmitRecorder: core.TheInactiveEmit, ok: false}
	if _, ok := recordFnValueApplyFallback(recNo, r, "h", caps, arg, carrierOut, core.SrcPos{}); ok {
		t.Error("a declining recorder must return false")
	}
}
