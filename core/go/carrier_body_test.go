package core

// Standalone-suite coverage for carrier_body.go and record_typed_def.go,
// moved down from the check piece at the 2026-08-08 ADR-013 amendment.
//
// The four RunCarrierBody* entries differ only in whether the body's defs
// roll back (a conditional arm) or leak (`do`), and whether the run raises
// CondBodyDepth (a condition fragment runs unconditionally exactly once,
// so it does not). The decline arms are pinned separately in
// carrier_body_gate_test.go; what is proved here is the RUN.

import (
	"testing"
)

// bodyProbeWords registers the two natives the body tests drive: one that
// pushes a def (so the net-additions accounting has something to report)
// and one that raises (so the branch_error arm is reachable).
func bodyProbeWords(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "cbind",
		Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				reg.Defs.Push("cbound", args[0])
				return nil, nil
			}, RunInCheck()),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "cboom",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				return nil, reg.BoruError("probe_error", "cboom always raises", "cboom")
			}, RunInCheck()),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
}

func bodyProbeReg(t *testing.T) *Registry {
	t.Helper()
	return covRegistry(t, bodyProbeWords)
}

func TestRunCarrierBodyRollsBackDefs(t *testing.T) {
	r := bodyProbeReg(t)
	defer r.Check.Begin()()

	body := NewList([]Value{NewInteger(7), NewWord("cbind")})
	stk, adds := RunCarrierBodyWithDefs(r, body)
	_ = stk

	// The body's net def ADDITION is reported…
	got, ok := adds["cbound"]
	if !ok {
		t.Fatalf("adds = %v, want the body's cbound binding", adds)
	}
	if n, err := AsInteger(got); err != nil || n != 7 {
		t.Errorf("reported binding = %v (%v), want 7", got, err)
	}
	// …and popped, so the caller decides whether to re-push, join or drop.
	if _, still := r.Defs.Top("cbound"); still {
		t.Error("a rolled-back body must not leave its binding installed")
	}

	// The depths it raised are restored.
	if r.Check.NestedBodyDepth != 0 || r.Check.CondBodyDepth != 0 {
		t.Errorf("depths after run: nested=%d cond=%d, want 0/0",
			r.Check.NestedBodyDepth, r.Check.CondBodyDepth)
	}
}

func TestRunCarrierBodyKeepDefsLeaks(t *testing.T) {
	r := bodyProbeReg(t)
	defer r.Check.Begin()()

	// `do`'s check-mode twin: body defs LEAK to the enclosing scope,
	// exactly as the runtime leaves them.
	RunCarrierBodyKeepDefs(r, NewList([]Value{NewInteger(3), NewWord("cbind")}))
	v, ok := r.Defs.Top("cbound")
	if !ok {
		t.Fatal("keep-defs mode must leave the body's binding installed")
	}
	if n, err := AsInteger(v); err != nil || n != 3 {
		t.Errorf("leaked binding = %v (%v), want 3", v, err)
	}
}

func TestRunCarrierCondBodyIsCondDepthExempt(t *testing.T) {
	r := bodyProbeReg(t)
	defer r.Check.Begin()()

	// A condition fragment runs unconditionally exactly once BEFORE the
	// branch decision, so an in-place redefinition there is not
	// path-dependent and must not raise CondBodyDepth. Observe the depth
	// from inside the body.
	var sawCond, sawNested int
	r.RegisterNativeFunc(NativeFunc{
		Name: "cdepth",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				sawCond, sawNested = reg.Check.CondBodyDepth, reg.Check.NestedBodyDepth
				return nil, nil
			}, RunInCheck()),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}

	RunCarrierCondBody(r, NewList([]Value{NewWord("cdepth")}))
	if sawCond != 0 {
		t.Errorf("condition fragment raised CondBodyDepth to %d, want 0", sawCond)
	}
	if sawNested != 1 {
		t.Errorf("condition fragment NestedBodyDepth = %d, want 1", sawNested)
	}

	// A branch arm (keep=false, condFrag=false) DOES raise it — its
	// bindings are conditional, so an in-place redefinition there is
	// unsound to compile.
	sawCond, sawNested = -1, -1
	RunCarrierBodyWithDefs(r, NewList([]Value{NewWord("cdepth")}))
	if sawCond != 1 {
		t.Errorf("branch arm CondBodyDepth = %d, want 1", sawCond)
	}
}

func TestRunCarrierBodyRecordsBranchError(t *testing.T) {
	r := bodyProbeReg(t)
	defer r.Check.Begin()()

	if stk := RunCarrierBody(r, NewList([]Value{NewWord("cboom")})); len(stk) != 0 {
		t.Errorf("a raising body must yield no residual, got %v", stk)
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "branch_error" {
			found = true
		}
	}
	if !found {
		t.Errorf("a raising body must record branch_error, got %+v", r.Check.Diagnostics)
	}
	// The depths are still unwound after the error path.
	if r.Check.NestedBodyDepth != 0 || r.Check.CondBodyDepth != 0 {
		t.Errorf("depths after error: nested=%d cond=%d, want 0/0",
			r.Check.NestedBodyDepth, r.Check.CondBodyDepth)
	}
}

func TestRunCarrierBodyEmptyList(t *testing.T) {
	r := bodyProbeReg(t)
	defer r.Check.Begin()()
	// An empty (but concrete) list runs to an empty residual rather than
	// taking either decline arm.
	if stk := RunCarrierBody(r, NewList(nil)); len(stk) != 0 {
		t.Errorf("empty body residual = %v, want empty", stk)
	}
}

// --- record_typed_def.go ---------------------------------------------
