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
	"errors"
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

// TestRunCarrierCondBodyKeepDefsKeeps pins the KEPT condition run (NUR212):
// an `if` condition runs unconditionally, once, before the branch decision,
// so the binding it makes stands after the run — unlike RunCarrierCondBody,
// which rolls it back — and, not being truncated, the run does not raise
// RolledBackBodyDepth (its installs ledger like any straight-line one) nor
// CondBodyDepth (it is not conditional).
func TestRunCarrierCondBodyKeepDefsKeeps(t *testing.T) {
	r := bodyProbeReg(t)
	defer r.Check.Begin()()

	var sawRolled, sawCond, sawNested int
	r.RegisterNativeFunc(NativeFunc{
		Name: "cdepths",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				sawRolled, sawCond, sawNested = reg.Check.RolledBackBodyDepth, reg.Check.CondBodyDepth, reg.Check.NestedBodyDepth
				return nil, nil
			}, RunInCheck()),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}

	stk := RunCarrierCondBodyKeepDefs(r, NewList([]Value{NewInteger(9), NewWord("cbind"), NewWord("cdepths"), NewBoolean(true)}))
	if len(stk) != 1 {
		t.Fatalf("residual = %v, want the one condition value", stk)
	}
	v, ok := r.Defs.Top("cbound")
	if !ok {
		t.Fatal("a kept condition run must leave its binding installed")
	}
	if n, err := AsInteger(v); err != nil || n != 9 {
		t.Errorf("kept binding = %v (%v), want 9", v, err)
	}
	if sawRolled != 0 || sawCond != 0 || sawNested != 1 {
		t.Errorf("depths inside the kept condition: rolled=%d cond=%d nested=%d, want 0/0/1",
			sawRolled, sawCond, sawNested)
	}

	// The negative twin: the rolled-back condition run (the `case`
	// scrutinee's count run) reports the binding and removes it.
	r.Defs.Truncate("cbound", 0)
	_, adds := RunCarrierCondBody(r, NewList([]Value{NewInteger(4), NewWord("cbind"), NewWord("cdepths"), NewBoolean(true)}))
	if _, still := r.Defs.Top("cbound"); still {
		t.Error("a rolled-back condition run must not leave its binding installed")
	}
	if _, reported := adds["cbound"]; !reported {
		t.Errorf("adds = %v, want the rolled-back binding reported", adds)
	}
	if sawRolled != 1 || sawCond != 0 {
		t.Errorf("depths inside the rolled-back condition: rolled=%d cond=%d, want 1/0", sawRolled, sawCond)
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

// probeEmit is an EmitRecorder that reports ACTIVE and captures the calls
// recorded through it. Everything it does not override delegates to the
// shared inactive recorder, so it stays a few lines wide as the interface
// grows.
type probeEmit struct {
	EmitRecorder
	words []string
	outs  [][]Value
}

func (p *probeEmit) Active() bool { return true }

func (p *probeEmit) RecordCall(word string, _ *Signature, _ []Value, out []Value, _ SrcPos, _, _ bool) {
	p.words = append(p.words, word)
	p.outs = append(p.outs, out)
}

// makeWordReg registers a `make` word carrying the `[Ideal Map]` overload
// a typed-def object construction would have dispatched.
func makeWordReg(t *testing.T, withIdealMap bool) *Registry {
	t.Helper()
	r := w8reg(t)
	sigs := []Signature{{
		Args: []*Type{TInteger},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
		Returns: []*Type{TAny}, BarrierPos: -1,
	}}
	if withIdealMap {
		sigs = append(sigs, Signature{
			Args: []*Type{TIdeal, TMap},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			Returns: []*Type{TAny}, BarrierPos: -1,
		})
	}
	r.RegisterNativeFunc(NativeFunc{Name: "make", Signatures: sigs})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	return r
}

func TestObjectMakeSig(t *testing.T) {
	// No `make` word at all.
	if got := objectMakeSig(w8reg(t)); got != nil {
		t.Error("a registry without make must yield nil")
	}
	// A `make` word with no [Ideal Map] overload.
	if got := objectMakeSig(makeWordReg(t, false)); got != nil {
		t.Error("make without an [Ideal Map] overload must yield nil")
	}
	// The overload is found by ARG SHAPE, so it tracks the registered
	// native rather than a fabricated sig.
	sig := objectMakeSig(makeWordReg(t, true))
	if sig == nil {
		t.Fatal("the [Ideal Map] overload must be found")
	}
	if !SigArgType(sig, 0).Equal(TIdeal) || !SigArgType(sig, 1).Equal(TMap) {
		t.Errorf("found sig = %v/%v, want [Ideal Map]",
			SigArgType(sig, 0), SigArgType(sig, 1))
	}
}

func TestRecordTypedDefMake(t *testing.T) {
	probe := MintTestType("Ideal/TypedDefProbe")
	typeArg := NewTypeLiteral(probe)
	body := NewMap(NewOrderedMap())

	// A nil registry declines.
	if _, ok := RecordTypedDefMake(nil, "", typeArg, body, SrcPos{}); ok {
		t.Error("a nil registry must decline")
	}

	// An INACTIVE recorder declines — outside emit mode the caller binds
	// the concrete value instead.
	r := makeWordReg(t, true)
	if _, ok := RecordTypedDefMake(r, "b", typeArg, body, SrcPos{}); ok {
		t.Error("an inactive recorder must decline")
	}

	// Active recorder but NO [Ideal Map] overload to attribute the call to.
	r = makeWordReg(t, false)
	r.Check.Emit = &probeEmit{EmitRecorder: TheInactiveEmit}
	if _, ok := RecordTypedDefMake(r, "b", typeArg, body, SrcPos{}); ok {
		t.Error("a missing make overload must decline")
	}

	// The recording path: a make event over [typeArg, body], returning the
	// fresh instance carrier the caller binds in place of the concrete
	// result, so the binding carries make-equivalent provenance.
	r = makeWordReg(t, true)
	pe := &probeEmit{EmitRecorder: TheInactiveEmit}
	r.Check.Emit = pe
	carrier, ok := RecordTypedDefMake(r, "b", typeArg, body, SrcPos{Row: 2, Col: 4})
	if !ok {
		t.Fatal("an active recorder with a make overload must record")
	}
	if !carrier.Carrier || !carrier.Parent.Equal(probe) {
		t.Errorf("recorded carrier = %v, want an abstract %v", carrier, probe)
	}
	if len(pe.words) != 1 || pe.words[0] != "make" {
		t.Errorf("recorded calls = %v, want one make", pe.words)
	}
	if len(pe.outs) != 1 || len(pe.outs[0]) != 1 || !ValuesEqual(pe.outs[0][0], carrier) {
		t.Errorf("recorded output = %v, want the returned carrier", pe.outs)
	}
}

// typedDefMakeSig runs make's own handler and wraps ONLY its failure as the
// typed-def handler does (`def <name>: <err>`); a success passes through
// untouched, and the original signature is left as it was.
func TestTypedDefMakeSigWrapsOnlyTheFailure(t *testing.T) {
	boom := errors.New("make: missing field \"spec\" for Entity")
	orig := &Signature{Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if len(args) == 0 {
			return nil, boom
		}
		return args, nil
	})}
	w := typedDefMakeSig(orig, "e")
	if w == orig {
		t.Fatal("the wrap must be a copy, not make's own signature")
	}
	_, err := w.DispatchHandler()(nil, nil, nil, nil)
	if err == nil || err.Error() != "def e: "+boom.Error() || !errors.Is(err, boom) {
		t.Errorf("failure = %v, want the def-wrapped make error", err)
	}
	in := []Value{NewInteger(7)}
	if got, err := w.DispatchHandler()(in, nil, nil, nil); err != nil || len(got) != 1 || !ValuesEqual(got[0], in[0]) {
		t.Errorf("success = %v / %v, want the handler's own result", got, err)
	}
	if _, err := orig.DispatchHandler()(nil, nil, nil, nil); err != boom {
		t.Errorf("make's own signature changed: %v", err)
	}
}
