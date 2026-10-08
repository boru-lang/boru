package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// NUR037: a fn declared inside another fn's body and named as a
// higher-order body word (`for-each [step] xs`) resolved under the
// interpreter but died with undefined_word under the DEFAULT compiled
// mode — the compile pass admitted the body (the check-time registry
// resolves `step`) and baked the NAME, which the VM's runtime registry
// never binds (the enclosing body's `def step fn` is compiled away).
// The first resolution was a COMPILE FAILURE: bodyRefsFnLocalFn (check/go's
// carrier.go) detects the shape at compile admission and marked the
// program uncompilable, so the interpreter owned the whole run — "slow,
// not wrong" restored.
//
// The seventy-second increment narrowed the compile failure to a CAPTURING local
// fn: a capture-free local fn's def is PLACED as a registry-visible
// install for the frame (compiler/go's placeFnLocalDef — `PUSH_CONST fn;
// BIND_DYN_SCOPE step`, torn down at RET), so the body resolves it where
// the interpreter does and the program compiles. The recorded repro's
// `step` captures `acc`, a closure the placement cannot bake, and keeps
// the compile failure; the `each` variant below is capture-free and compiles.
//
// The declining case lives here rather than in lang/spec because the
// spec corpus enforces failureCeiling = 0 (every spec value row must
// compile) — the same placement precedent as fn_triple_compiled_test.go.
// The module-scope twins (which must KEEP compiling) are pinned in
// lang/spec/fn-value.tsv §7 where the differential gate holds both
// engines to them.

// nur037Repro is the recorded repro: a fn-local fn as a for-each body
// word, accumulating into a captured flex map — the CAPTURING shape,
// which still declines.
const nur037Repro = `
def collect fn [[xs:List] [Any] [
  def acc (flex {})
  def step fn [[e:String] [Any] [ acc set (e) true ]]
  for-each [step] xs
  acc
]]
collect ["x" "y"]
`

// nur037Each is the `each` variant — the same fn-local-fn shape across
// the higher-order family (it leaked through the island path where
// for-each leaked through the CALL_NATIVE const-bake). Its `step` is
// capture-free, so the seventy-second increment places its def and the
// program compiles.
const nur037Each = `
def collect fn [[xs:List] [Any] [
  def step fn [[e:Integer] [Integer] [e add 1]]
  each [step] xs
]]
collect [1 2]
`

// runBothEngines runs src through the default compiled entry point and
// the interpreter, returning (defaultOut, wasCompiled, interpOut). A
// compile_failed from the library is handled the way the CLI handles
// it (warn-and-fall-back): the compile failure guarantees no observable effect
// escaped, so the default-mode result is an explicit interpreter re-run
// on a fresh instance, with wasCompiled=false.
func runBothEngines(t *testing.T, src string) (string, bool, string) {
	t.Helper()
	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	comp, wasCompiled, cerr := ac.RunCompiled(src)
	if cerr != nil {
		if !strings.Contains(cerr.Error(), "compile_failed") {
			t.Fatalf("default (compiled-mode) run errored: %v", cerr)
		}
		af, ferr := lang.New()
		if ferr != nil {
			t.Fatalf("lang.New: %v", ferr)
		}
		comp, cerr = af.RunInterp(src)
		if cerr != nil {
			t.Fatalf("fallback interpreter run errored: %v", cerr)
		}
		wasCompiled = false
	}
	ai, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	interp, ierr := ai.RunInterp(src)
	if ierr != nil {
		t.Fatalf("interpreter run errored: %v", ierr)
	}
	return fmt.Sprint(comp), wasCompiled, fmt.Sprint(interp)
}

func TestNur037FnLocalFnForEachAgrees(t *testing.T) {
	comp, wasCompiled, interp := runBothEngines(t, nur037Repro)
	if wasCompiled {
		t.Error("the CAPTURING fn-local-fn for-each shape must DECLINE compilation (a closure the frame placement cannot bake)")
	}
	if comp != interp {
		t.Errorf("default mode = %q diverges from interpreter = %q", comp, interp)
	}
	if !strings.Contains(interp, "x:true") || !strings.Contains(interp, "y:true") {
		t.Errorf("interpreter result = %q, want the {x:true y:true} accumulator", interp)
	}
}

func TestNur037FnLocalFnEachAgrees(t *testing.T) {
	comp, wasCompiled, interp := runBothEngines(t, nur037Each)
	if !wasCompiled {
		t.Error("the capture-free fn-local-fn each shape must COMPILE (the seventy-second increment places its def for the frame)")
	}
	if comp != interp {
		t.Errorf("default mode = %q diverges from interpreter = %q", comp, interp)
	}
	if interp != "[[2 3]]" {
		t.Errorf("interpreter result = %q, want [[2 3]]", interp)
	}
}

func TestNur037CompileFailureReasonNamesTheShape(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	_, wasCompiled, reason, rerr := a.RunCompiledReason(nur037Repro)
	if wasCompiled {
		t.Fatal("the capturing repro must decline compilation")
	}
	// Stage J: a genuine performance compile failure surfaces as compile_failed
	// (the CLI warns and falls back); the reason names the shape.
	if rerr != nil && !strings.Contains(rerr.Error(), "compile_failed") {
		t.Fatalf("run: %v", rerr)
	}
	if !strings.Contains(reason, "fn-local fn `step`") {
		t.Errorf("compile failure reason = %q, want the fn-local-fn reason naming step", reason)
	}
}

func TestNur037CheckStaysClean(t *testing.T) {
	// The compile failure (and the placement) is a compile-admission decision,
	// not a check error: both programs are valid and `boru check` must
	// stay clean on them.
	for _, src := range []string{nur037Repro, nur037Each} {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		res, cerr := a.Check(src)
		if cerr != nil {
			t.Fatalf("check errored: %v", cerr)
		}
		if res.Summary.Errors != 0 || res.Summary.Warnings != 0 {
			t.Errorf("check: %d error(s), %d warning(s); want clean (compile failure is not a check error)",
				res.Summary.Errors, res.Summary.Warnings)
		}
	}
}

func TestNur037ModuleScopeCallbackStillCompiles(t *testing.T) {
	// The negative contract: the SAME callback hoisted to module scope
	// (the utils-corpus house-rule shape) must KEEP compiling and agree.
	src := `
def step fn [[e:Integer] [Integer] [e add 1]]
each [step] [1 2]
`
	comp, wasCompiled, interp := runBothEngines(t, src)
	if !wasCompiled {
		t.Error("a module-scope callback must keep compiling (the compile failure must not over-match)")
	}
	if comp != interp || comp != "[[2 3]]" {
		t.Errorf("compiled = %q, interpreted = %q; want both [[2 3]]", comp, interp)
	}
}

func TestNur037FnLocalValueDefStillCompilesClosure(t *testing.T) {
	// A fn-local VALUE def read by the body (the lexical-capture shape)
	// is the closure path's territory and must not be caught by the
	// fn-local-FN compile failure: the program still runs identically in both
	// modes, whatever the compile decision for the enclosing unit.
	src := `
def f fn [[xs:List] [Any] [
  def n 10
  each [n add] xs
]]
f [1 2]
`
	comp, _, interp := runBothEngines(t, src)
	if comp != interp || comp != "[[11 12]]" {
		t.Errorf("compiled = %q, interpreted = %q; want both [[11 12]]", comp, interp)
	}
}
