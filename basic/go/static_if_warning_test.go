package basic

import "testing"

// static_if_warning_test.go pins the dead-arm warning of a bare concrete-
// Boolean `if` condition, split out of ReduceStaticIf so BOTH analysis passes
// make it (warnStaticIfDeadArm), and the case desugar's filter that keeps the
// synthesized `if` tokens of its own chain from making it
// (dropSynthesizedDeadArmWarnings). The recording pass itself is not linked
// in this module; lang's diagnostic_parity_fixes_test.go pins both passes
// end to end.

func deadArmWarnings(r *Registry) []CheckDiagnostic {
	var out []CheckDiagnostic
	for _, d := range r.Check.Diagnostics {
		if d.Code == "unreachable_branch" {
			out = append(out, d)
		}
	}
	return out
}

func TestWarnStaticIfDeadArm(t *testing.T) {
	for _, c := range []struct {
		name    string
		cond    Value
		hasElse bool
		want    string // "" = no warning
	}{
		{"false: the then-arm is dead", NewBoolean(false), true, "if condition is a constant false; then-branch is unreachable"},
		{"false, no else: the then-arm is still dead", NewBoolean(false), false, "if condition is a constant false; then-branch is unreachable"},
		{"true with an else: the else-arm is dead", NewBoolean(true), true, "if condition is a constant true; else-branch is unreachable"},
		// Negatives: nothing is dead, or nothing is known.
		{"true, no else: no dead arm exists", NewBoolean(true), false, ""},
		{"a non-Boolean condition", NewInteger(1), true, ""},
		{"a Boolean carrier (unknown value)", NewCarrier(TBoolean), true, ""},
		{"a list-form condition", NewList([]Value{NewBoolean(true)}), true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTestRegistry(t)
			r.Check.CurCallPos = SrcPos{Row: 2, Col: 7}
			warnStaticIfDeadArm(r, c.cond, c.hasElse)
			got := deadArmWarnings(r)
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no warning, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Detail != c.want || got[0].Row != 2 || got[0].Col != 7 {
				t.Fatalf("want one %q at 2:7, got %+v", c.want, got)
			}
		})
	}
}

// A body analysed for ONE call shape does not speak for the code
// (EmitUnreachableBranch's CallShapeDepth gate) — the split-out warning
// inherits that.
func TestWarnStaticIfDeadArmSilentInCallShape(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.CallShapeDepth = 1
	warnStaticIfDeadArm(r, NewBoolean(false), true)
	if got := deadArmWarnings(r); len(got) != 0 {
		t.Fatalf("a call-shape specialisation must not warn, got %+v", got)
	}
}
