package test

import (
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
