package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// TestDynamicContextGetGradualMatch demonstrates the bounded dynamic(T)
// modality end to end (design/legacy/dynamic-modality-report.10.ignore): `context
// get` on a statically-untracked key is an escape hatch that emits
// dynamic(Any) — optimistically compatible — instead of strict
// Carry<Any>. That dynamic carrier then matches a typed slot under
// `boru check` (here add's Number) where strict Carry<Any> would not.
//
// `add` has only a [Number, Number] signature (no Any catch-all), so a
// successful match is the gradual not-disjoint rule at work, not a
// fallback. Strict Carry<Any> would fail it (proven directly in
// eng/go/dynamic_match_test.go).
func TestDynamicContextGetGradualMatch(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check(`context get "k" 1 add`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Fatalf("dynamic(Any) from context dot should match the Number slot; got no_signature: %+v", res.Diagnostics)
		}
	}
	// The result is itself dynamic (contagion): the modality flows through
	// add, and the residual stack renders it as dynamic(<bound>).
	if len(res.Stack) != 1 || (res.Stack[0] != "dynamic(Float)" && res.Stack[0] != "dynamic(Integer)") {
		t.Fatalf("expected a dynamic numeric result from the gradual match, got stack=%v", res.Stack)
	}
}

// TestDynamicCarrierRendersInCheckStack pins that a dynamic carrier is
// surfaced as dynamic(<bound>) in the `boru check` residual stack, so the
// gradual modality is visible rather than masquerading as a strict type.
func TestDynamicCarrierRendersInCheckStack(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`context get "k"`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "dynamic(Any)" {
		t.Errorf("expected dynamic(Any) in the residual stack, got %v", res.Stack)
	}
}

// TestDynamicDoComputedBody pins the `do` escape hatch: running a
// computed body the checker can't analyze statically yields a bounded
// gradual dynamic(Any), while a concrete body is analyzed normally.
func TestDynamicDoComputedBody(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	computed, err := a.Check(`do (iota 3)`)
	if err != nil {
		t.Fatalf("check computed: %v", err)
	}
	if len(computed.Stack) != 1 || computed.Stack[0] != "dynamic(Any)" {
		t.Errorf("do on a computed body should be dynamic(Any), got %v", computed.Stack)
	}
	concrete, err := a.Check(`do [1 add 2]`)
	if err != nil {
		t.Fatalf("check concrete: %v", err)
	}
	if len(concrete.Stack) != 1 || concrete.Stack[0] != "Integer" {
		t.Errorf("do on a concrete body should be analyzed (Integer), got %v", concrete.Stack)
	}
}

// hasDiag reports whether a check produced a diagnostic with the given
// code.
func hasDiag(diags []lang.CheckDiagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

// TestDynamicContagionFlows pins gradual contagion: the dynamic modality
// flows through a dispatch chain instead of dying after one hop. `get`
// on a dynamic value yields a dynamic result, which then matches add's
// Number slot — where a strict Carry<Any> from `get` would have failed.
func TestDynamicContagionFlows(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`context get "k" "x" get 1 add`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if hasDiag(res.Diagnostics, "no_signature") {
		t.Fatalf("dynamic should flow through dot and match add; got no_signature: %+v", res.Diagnostics)
	}
}

// TestDynamicNarrowingThroughUse pins narrowing-through-use: a typed use
// of a dynamic binding tightens it to dynamic(bound ∩ slot), so a later
// provably-disjoint use of the same name is caught WITHOUT an explicit
// guard. Branch analysis must scope the narrowing — it can't leak into a
// sibling branch or past the `if`.
func TestDynamicNarrowingThroughUse(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Linear: used as Number, then as a List (join) → contradiction caught.
	contra, err := a.Check(`def x (context get "k") end  x 1 add  x "s" join`)
	if err != nil {
		t.Fatalf("check contra: %v", err)
	}
	if !hasDiag(contra.Diagnostics, "no_signature") {
		t.Errorf("a Number-then-String use of a dynamic binding should be flagged, got %+v", contra.Diagnostics)
	}

	// Valid: used as Number twice → no contradiction.
	valid, err := a.Check(`def x (context get "k") end  x 1 add  x 2 add`)
	if err != nil {
		t.Fatalf("check valid: %v", err)
	}
	if hasDiag(valid.Diagnostics, "no_signature") {
		t.Errorf("two Number uses must not be flagged, got %+v", valid.Diagnostics)
	}

	// Branch-scoped: the then-branch narrows x to Number, but the
	// else-branch uses x as a List — the narrowing must NOT leak, so no
	// false positive. (Non-constant condition so both branches analyze.)
	scoped, err := a.Check(`def c (context get "f") end  def x (context get "k") end  if [c] [x 1 add] [x "s" join]`)
	if err != nil {
		t.Fatalf("check scoped: %v", err)
	}
	if hasDiag(scoped.Diagnostics, "no_signature") {
		t.Errorf("then-branch narrowing must not leak to the else-branch, got %+v", scoped.Diagnostics)
	}

	// And it must not leak PAST the if either.
	after, err := a.Check(`def x (context get "k") end  if [true false or] [x 1 add] [0] end  x "s" join`)
	if err != nil {
		t.Fatalf("check after: %v", err)
	}
	if hasDiag(after.Diagnostics, "no_signature") {
		t.Errorf("branch narrowing must not leak past the if, got %+v", after.Diagnostics)
	}
}

// TestDynamicGuardDischarge pins the bridge back to strict typing: a
// guard on a dynamic binding discharges the modality — inside the
// then-branch the value is strictly its guarded type, so a provably
// disjoint use is flagged (which the bare dynamic value would have
// admitted), and a valid use passes.
func TestDynamicGuardDischarge(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Without a guard: dynamic(Any) is optimistically compatible with
	// join's slot, so the disjoint use is admitted (no no_signature).
	bare, err := a.Check(`def x (context get "k") end x "s" join`)
	if err != nil {
		t.Fatalf("check bare: %v", err)
	}
	if hasDiag(bare.Diagnostics, "no_signature") {
		t.Fatalf("bare dynamic(Any) should match join optimistically, got no_signature: %+v", bare.Diagnostics)
	}

	// Guarded: in the then-branch x is strictly Integer (discharged), so
	// `x "s" join` (Integer into a List slot) is provably disjoint and
	// flagged.
	disjoint, err := a.Check(`def x (context get "k") end if [x is Integer] [x "s" join] [0]`)
	if err != nil {
		t.Fatalf("check disjoint: %v", err)
	}
	if !hasDiag(disjoint.Diagnostics, "no_signature") {
		t.Fatalf("guard should discharge to strict Integer and flag the disjoint join, got: %+v", disjoint.Diagnostics)
	}

	// Guarded + valid use: strict Integer into add's Number slot passes.
	valid, err := a.Check(`def x (context get "k") end if [x is Integer] [x 1 add] [0]`)
	if err != nil {
		t.Fatalf("check valid: %v", err)
	}
	if hasDiag(valid.Diagnostics, "no_signature") {
		t.Fatalf("guarded Integer should match add cleanly, got: %+v", valid.Diagnostics)
	}
}
