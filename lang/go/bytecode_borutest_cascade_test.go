package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Off-corpus regressions for force-compiling the recursive boru:test framework
// cascade (run-spec → test-describe closure → run-cases → run-case → test-record,
// plus the recursive `for subs [subspec run-spec]`). The langspec differential is
// blind to these harness shapes — and the failures here were SILENT miscompiles
// (vacuous "all green" / wrong case COUNTS), so each pins RunCompiled==Run on
// Test.summary, the count being the soundness signal. Covers four kernel fixes:
//   - FnSummaries delete-key (a discarded closure-probe summary left run-case an
//     empty stub unit → 0 cases),
//   - poly registry threading (test-record's sub-registry PolyRef.Reg → deferral),
//   - genArgs-keyed fn units (a recursive call allocated a SECOND, empty unit →
//     sub-specs silently skipped),
//   - 0-return no-provenance residual drop (run-spec's test-describe residual).

func cascadeSound(t *testing.T, src string) {
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

// A flat spec with a PASSING case: it compiles to a native lowering (no FALLBACK
// island) and the summary matches.
func TestBoruTestCascade_FlatPassing(t *testing.T) {
	const src = `import "boru:test" end
def double fn [[n:Integer] [Integer] [n mul 2]]
def s {name:"d" subject:double/q cases:[{name:"a" in:[3] out:6}] subs:[]}
s Test.run-spec end Test.summary`
	cascadeSound(t, src)
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("did not compile: reason=%q err=%v", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("expected a native lowering of the run-spec cascade, got an island")
	}
}

// A flat spec with a FAILING case: the failure MUST be counted (the empty-stub
// miscompile reported {total:0} here while interpret reported {total:1}).
func TestBoruTestCascade_FlatFailingCounted(t *testing.T) {
	const src = `import "boru:test" end
def double fn [[n:Integer] [Integer] [n mul 2]]
def s {name:"d" subject:double/q cases:[{name:"a" in:[3] out:999}] subs:[]}
s Test.run-spec end Test.summary`
	cascadeSound(t, src)
}

// Multiple cases, mixed pass/fail.
func TestBoruTestCascade_MultiCase(t *testing.T) {
	const src = `import "boru:test" end
def double fn [[n:Integer] [Integer] [n mul 2]]
def s {name:"d" subject:double/q cases:[{name:"a" in:[3] out:6} {name:"b" in:[0] out:1} {name:"c" in:[5] out:10}] subs:[]}
s Test.run-spec end Test.summary`
	cascadeSound(t, src)
}

// SUB-SPEC RECURSION: the recursive `subspec run-spec` must run the child's cases
// too (the empty-f4 bug ran the top case only — {total:1} vs interpret {total:2}).
func TestBoruTestCascade_SubSpecRecursion(t *testing.T) {
	const src = `import "boru:test" end
def inc fn [[n:Integer] [Integer] [n add 1]]
def child {name:"c" subject:inc/q cases:[{name:"a" in:[1] out:2}] subs:[]}
def top {name:"t" subject:inc/q cases:[{name:"b" in:[2] out:3}] subs:[child]}
top Test.run-spec end Test.summary`
	cascadeSound(t, src)
}

// Deeper / wider sub-spec tree: recursion at two levels and multiple siblings,
// with a failing leaf, so the count is non-trivial.
func TestBoruTestCascade_NestedSubSpecs(t *testing.T) {
	const src = `import "boru:test" end
def inc fn [[n:Integer] [Integer] [n add 1]]
def gc {name:"gc" subject:inc/q cases:[{name:"x" in:[9] out:10} {name:"y" in:[0] out:99}] subs:[]}
def c1 {name:"c1" subject:inc/q cases:[{name:"a" in:[1] out:2}] subs:[gc]}
def c2 {name:"c2" subject:inc/q cases:[{name:"b" in:[2] out:3}] subs:[]}
def top {name:"t" subject:inc/q cases:[{name:"r" in:[5] out:6}] subs:[c1 c2]}
top Test.run-spec end Test.summary`
	cascadeSound(t, src)
}

// A STRING subject (dotted, the real _spec-file shape) alongside the atom form, to
// pin the disjunct subject:(Atom tor String) path through test-invoke.
func TestBoruTestCascade_StringSubject(t *testing.T) {
	const src = `import "boru:test" end
def double fn [[n:Integer] [Integer] [n mul 2]]
def s {name:"d" subject:"double" cases:[{name:"a" in:[3] out:6}] subs:[]}
s Test.run-spec end Test.summary`
	cascadeSound(t, src)
}
