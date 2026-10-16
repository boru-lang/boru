package check

// Unit coverage for the effectful-word mirror seam (drypass.go's
// DeepConcreteOptions / MirrorReturns). The corpus drives these end to
// end through boru:net; these tests own the gate's decline arms and the
// diagnostic's code/detail resolution, which no corpus row can reach
// selectively.

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func optsMapOf(k string, v core.Value) core.Value {
	m := core.NewOrderedMap()
	m.Set(k, v)
	return core.NewMap(m)
}

func TestDeepConcreteOptionsGate(t *testing.T) {
	concrete := optsMapOf("tcp", core.NewInteger(8080))

	// The happy shape: a deep-concrete options map at position 0.
	if !DeepConcreteOptions([]core.Value{concrete}) {
		t.Error("a deep-concrete options map must pass the gate")
	}
	// A later STRICT carrier is fine — the validator does not read it.
	if !DeepConcreteOptions([]core.Value{concrete, core.NewCarrier(core.TMap)}) {
		t.Error("a strict carrier at a later position must not close the gate")
	}

	// The decline that keeps the false-positive pin at zero: a carrier
	// FIELD inside a concrete map. The field accessors read it as absent
	// or malformed, so a validator would flag a correct program.
	computed := optsMapOf("tcp", core.NewCarrier(core.TInteger))
	if !core.IsConcrete(&computed) {
		t.Fatal("fixture: a map with a carrier field is still IsConcrete — that is the trap")
	}
	if DeepConcreteOptions([]core.Value{computed}) {
		t.Error("a computed option field must close the gate")
	}
	// A DYNAMIC later operand closes it too: the match was optimistic, so
	// the run may never reach the error being mirrored.
	if DeepConcreteOptions([]core.Value{concrete, core.NewDynamicCarrier(core.TAny)}) {
		t.Error("a dynamic later operand must close the gate")
	}
	// The AT variant gates the argument the validator actually reads —
	// open's options are args[1], not args[0].
	atOne := DeepConcreteOptionsAt(1)
	if !atOne([]core.Value{core.NewCarrier(core.TString), concrete}) {
		t.Error("a computed operand at 0 must not close a gate reading position 1")
	}
	if atOne([]core.Value{concrete, computed}) {
		t.Error("a computed options map at the READ position must close the gate")
	}
	if atOne([]core.Value{concrete}) {
		t.Error("an index past the operands closes the gate")
	}
	// A DYNAMIC operand at ANY position closes it: the runtime may take a
	// different overload than the one being modelled.
	if atOne([]core.Value{core.NewDynamicCarrier(core.TAny), concrete}) {
		t.Error("a dynamic operand at position 0 must close the gate")
	}

	// Degenerate shapes.
	if DeepConcreteOptions(nil) {
		t.Error("no operands: nothing to validate")
	}
	if DeepConcreteOptions([]core.Value{core.NewCarrier(core.TMap)}) {
		t.Error("a carrier options map must close the gate")
	}
}

func TestMirrorReturnsDiagnostics(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	base := ReturnsStatic(core.TString)
	args := []core.Value{optsMapOf("tcp", core.NewInteger(1))}

	// A CODED validator error is reported verbatim — code and detail both.
	coded := MirrorReturns("listen", DeepConcreteOptions,
		func(_ []core.Value, _ *core.Registry) error {
			return &core.BoruError{Code: "net_error", Detail: "listen: options need tcp: (or unix:)"}
		}, base)
	out := coded(args, r)
	if len(out) != 1 || !core.ParentOf(&out[0]).Equal(core.TString) {
		t.Fatalf("residual = %v, want the base model's String", out)
	}
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("a coded validator error must be reported once, got %+v", r.Check.Diagnostics)
	}
	if d := r.Check.Diagnostics[0]; d.Code != "net_error" || d.Detail != "listen: options need tcp: (or unix:)" {
		t.Errorf("diagnostic = %q/%q, want the runtime's own code and detail", d.Code, d.Detail)
	}

	// An UNCODED validator error falls back to type_error, keeping the
	// message as its detail.
	n := len(r.Check.Diagnostics)
	uncoded := MirrorReturns("prepare", DeepConcreteOptions,
		func(_ []core.Value, _ *core.Registry) error {
			return errors.New("prepare: missing required \"spec\" field")
		}, base)
	uncoded(args, r)
	if len(r.Check.Diagnostics) != n+1 {
		t.Fatalf("an uncoded validator error must still be reported, got %+v", r.Check.Diagnostics)
	}
	if d := r.Check.Diagnostics[n]; d.Code != "type_error" {
		t.Errorf("uncoded error code = %q, want the type_error fallback", d.Code)
	}

	// A validator that PASSES adds nothing.
	n = len(r.Check.Diagnostics)
	clean := MirrorReturns("listen", DeepConcreteOptions,
		func(_ []core.Value, _ *core.Registry) error { return nil }, base)
	if out := clean(args, r); len(out) != 1 {
		t.Errorf("clean residual = %v, want one value", out)
	}
	if len(r.Check.Diagnostics) != n {
		t.Error("a passing validator must add no diagnostic")
	}

	// A CLOSED gate never runs the validator at all — the guarantee that
	// keeps an effectful word's handler out of the analysis pass.
	ran := false
	gated := MirrorReturns("listen", DeepConcreteOptions,
		func(_ []core.Value, _ *core.Registry) error { ran = true; return nil }, base)
	gated([]core.Value{core.NewCarrier(core.TMap)}, r)
	if ran {
		t.Error("the validator ran behind a closed gate")
	}
}
