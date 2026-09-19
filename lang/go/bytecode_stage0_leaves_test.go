package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Stage-0 leaf pins (voxgig zero-refusals plan): two emitter/dispatch fixes
// whose exact shapes the langspec differential does not cover.
//
//  1. Options-carrier param match (signature.go): a Map-typed CARRIER matches
//     an `opts:Options` slot — it is check mode's stand-in for a value that IS
//     a concrete map at run time, and the runtime rule accepts that value.
//     Without it the check-mode dispatch refused what the interpreter runs
//     (template `{…} render` → Options-arm → tpl-render-opts refusal).
//  2. Nested zeroOut branch (emit.go RecordBranch): a both-arms-void `if`
//     nested in another arm registers a phantom (None) result; the arm
//     residual must strip it (as the program/fn-body residuals do) or the
//     lowerer refuses "branch leaves extra values" (the stats welford-push
//     min/max guard shape).

func stage0Sound(t *testing.T, src string) {
	t.Helper()
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, gerr) {
		return
	}
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
}

// stage0Compiles asserts the source force-compiles (no interpreter fallback)
// AND matches the interpreter — the flip claims of the two fixes.
func stage0Compiles(t *testing.T, src string) {
	t.Helper()
	stage0Sound(t, src)
	a, _ := New()
	if _, reason, _, err := a.CompileCheck(src); err != nil || reason != "" {
		t.Fatalf("expected the shape to force-compile, got refusal: %v / %q\n  src: %s", err, reason, src)
	}
}

// A piped concrete map dispatching a 2-overload fn whose Options arm calls a
// single-overload Options helper — the tpl-render-opts shape. The helper's
// opts param is a Map carrier at check time; it must match the Options slot.
func TestOptionsCarrierParamMatchCompiles(t *testing.T) {
	stage0Compiles(t, `def helper fn [ [opts:Options] [String] [ (opts get "a") ] ]
def disp fn [
  [cdata:Any c:String] [String] [ c ]
  [opts:Options]       [String] [ helper opts ]
]
({a:'hi'} disp)`)
}

// NEGATIVE: a NON-map carrier must still refuse the Options slot — the match
// is scoped to carriers whose static type conforms to Map, not all carriers.
func TestOptionsCarrierNonMapStillRefuses(t *testing.T) {
	src := `def helper fn [ [opts:Options] [String] [ "x" ] ]
def f fn [[n:Integer] [String] [ helper n ]]
(f 3)`
	a, _ := New()
	res, _ := a.Check(src)
	flagged := false
	for _, d := range res.Diagnostics {
		if d.Severity == "error" || strings.Contains(d.Detail, "no signature matches") {
			flagged = true
			break
		}
	}
	if !flagged {
		t.Error("an Integer arg into an Options param must stay flagged")
	}
}

// The welford min/max guard shape: a both-arms-void nested `if` inside the
// else-arm of a side-effect `if`, in a class-instance-returning fn. Must
// force-compile and match the interpreter.
func TestNestedZeroOutBranchCompiles(t *testing.T) {
	stage0Compiles(t, `def Box class { v: 0.0 lo: 0.0 }
def poke fn [
  [b:Box x:Float] [Box] [
    set v (x) b end
    if (x lt 1.0) [
      set lo (x) b end
    ] [
      if (x gt 2.0) [set lo (2.0) b end] []
    ]
    b
  ]
]
(poke (make Box {}) 0.5)`)
}

// Both branch paths of the nested guard, exercised at run time (compile ==
// interpret on each path, including the untaken-nested-guard path).
func TestNestedZeroOutBranchAllPathsSound(t *testing.T) {
	for _, x := range []string{"0.5", "1.5", "2.5"} {
		stage0Sound(t, `def Box class { v: 0.0 lo: 0.0 }
def poke fn [
  [b:Box x:Float] [Box] [
    if (x lt 1.0) [ set lo (x) b end ] [ if (x gt 2.0) [set lo (2.0) b end] [] ]
    b
  ]
]
(poke (make Box {}) `+x+`)`)
	}
}
