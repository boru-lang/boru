package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The 2-arg `if` and its ZERO-VALUE then arm — the one branch shape the
// recorder registers WITHOUT resolving a merge value (RecordBranch's
// `!HasElse && len(ThenStk) == 0` arm, emit.go). Because that arm never runs
// resolveArm, it carries its own check that the then FRAGMENT was captured;
// these two tests are the two sides of it.
//
// The refusing side is a documented program, not a contrived one. `boru
// describe if` lists the 2-arg VALUE form — `if 5 6 ;# 6` — and a plain-value
// arm is not a code body, so core's body runner (runCarrierBodyDefsAdds)
// returns on its AsList failure BEFORE installing the guard that would record
// the arm into a fragment. If2ReturnsFn therefore hands the recorder a nil
// fragment over an empty then-stack, and the recorder refuses the PROGRAM
// rather than emitting a branch with nothing to lower on its true path. The
// interpreter still answers, which is the point of a refusal: it is a
// fallback, not a change of behaviour.
//
// The refusal is load-bearing, not tidiness: with the recorder's check
// removed, the nil fragment reaches lowerFragment and `if 5 6` PANICS inside
// CompileCheck on a nil dereference. This test is what stands between that
// and a release.
//
// The accepting side is the same recorder arm reached with a real `[…]` body
// that happens to leave nothing — `if true []`. It must keep compiling, or
// the guard above has been widened into a refusal of working programs.
//
// compiler/go/if2_zero_arm_guard_test.go is the seam half, which pins where
// the refusal lands inside RecordBranch.

// TestIf2ValueThenArmRefusesToCompile pins the REFUSAL: a 2-arg `if` whose
// then arm is an already-evaluated value has no fragment to lower, so the
// program falls back to the interpreter with this exact reason.
func TestIf2ValueThenArmRefusesToCompile(t *testing.T) {
	const want = "if: then-branch not captured"
	cases := []struct{ name, src, interp string }{
		// The worked example `boru describe if` publishes for the 2-arg form.
		{"literal condition, literal value arm", `if 5 6`, "[6]"},
		{"boolean condition, literal value arm", `if true 99`, "[99]"},
		// A COMPUTED condition takes the same path: the arm, not the
		// condition, is what was never captured.
		{"computed condition, literal value arm", `if (1 lt 2) 99`, "[99]"},
		// A def-bound condition, and a residual after the if, so the refusal
		// is not an artifact of the if being the whole program.
		{"def-bound condition, value arm, trailing residual",
			`def a 1 if (a gt 0) 99 end 7`, "[99 7]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck: %v", err)
			}
			if prog != nil {
				t.Fatalf("a value then arm has no fragment to lower; it must not compile (got %s)", prog.Disassemble())
			}
			if reason != want {
				t.Errorf("refusal reason = %q, want %q", reason, want)
			}
			// The refusal is a FALLBACK: the program still runs, and still
			// gives the documented answer.
			got, err := a.RunInterp(c.src)
			if err != nil {
				t.Fatalf("RunInterp: %v", err)
			}
			if fmt.Sprint(got) != c.interp {
				t.Errorf("interpreter = %v, want %s", got, c.interp)
			}
		})
	}
}

// TestIf2CapturedZeroValueArmCompiles is the TWIN: the same RecordBranch arm
// (2-arg if, then nets nothing) reached with a real `[…]` body, which IS
// captured. These must keep compiling natively — a guard that refused them
// too would be refusing the shape it was written to admit.
func TestIf2CapturedZeroValueArmCompiles(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"empty body arm", `if true []`, "[]"},
		{"empty body arm, trailing residual", `if true [] end 7`, "[7]"},
		{"computed condition, empty body arm", `if (1 lt 2) [] end 7`, "[7]"},
		// A body that RUNS and nets nothing — `set` on a class instance is an
		// in-place field write that returns nothing — is the other way into
		// the same zero-value arm, and the one that proves the arm's EFFECTS
		// survive the missing merge slot: the branch emits no result, but the
		// body still lowers and still runs on the true path.
		{"0-value word arm writes through the branch",
			`def C class {n: Integer} def o (make C {n: 1}) if true [o set "n" 2] end (o get "n")`, "[2]"},
		{"0-value word arm on the false path leaves the field alone",
			`def C class {n: Integer} def o (make C {n: 1}) if false [o set "n" 2] end (o get "n")`, "[1]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck: %v", err)
			}
			if prog == nil {
				t.Fatalf("a captured 0-value then arm must compile, refused: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("must compile native (no interpreter island):\n%s", prog.Disassemble())
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			ref, err := b.RunInterp(c.src)
			if err != nil {
				t.Fatalf("RunInterp: %v", err)
			}
			if fmt.Sprint(got) != fmt.Sprint(ref) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, ref)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
