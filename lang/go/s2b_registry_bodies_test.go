package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestCoverSuiteBodyBakesAtModuleScope — Test.cover declares
// CompileRunsBodyOnRegistry (S2b, 2026-09-26): its suite body is tree-walked
// over the enclosing registry in both modes, so at the TOP-LEVEL STATEMENT
// position the dispatch bakes as a plain CALL_NATIVE over the body list —
// the `import` inside the body (which no closure compiles) runs where the
// interpreter runs it, and the compiled program's top-level defs are written
// back for the body to read. Every other position declines loudly: inside a
// fn a body token naming a param, inside a top-level loop one naming the
// iterator, resolved against the registry under the VM and raised a false
// undefined_word (present on main at 3b5db68 — the inert-scope test admitted
// the word-list body; the declared word takes only its own rule now).
func TestCoverSuiteBodyBakesAtModuleScope(t *testing.T) {
	for _, src := range []string{
		`import "boru:test" end Test.cover [ import "boru:cli" end Test.test "t" [1 1 Assert.equal] ] end 7`,
		`import "boru:test" end def x 5 end Test.cover [ Test.test "t" [x 5 Assert.equal] ] end Test.fail-count`,
		`import "boru:test" end Test.cover [ import "boru:string-util" end def spec {a:1} end Test.test "t" [spec.a 1 Assert.equal] ] end Test.fail-count`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `import "boru:test" end Test.cover [ import "boru:cli" end Test.test "t" [1 1 Assert.equal] ] end 7`)
	if !strings.Contains(dis, "CALL_NATIVE") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the suite body must bake as a plain CALL_NATIVE:\n%s", dis)
	}
	// Any nested position declines loudly, with parity through the fallback.
	for _, src := range []string{
		`import "boru:test" end def f fn [[n:Integer][Integer][Test.cover [n] n]] end f 3`,
		`import "boru:test" end for 2 [Test.cover [ def spec {a:i} end Test.test "t" [spec.a i Assert.equal] ]] end Test.fail-count`,
		`import "boru:test" end if true [Test.cover [ Test.test "t" [1 1 Assert.equal] ]] [] end Test.fail-count`,
	} {
		fnValueM2CompileFailure(t, "nested Test.cover: "+src, src, "code-body word test-cover")
	}
}

// TestLoopRangeEventStartCompiles — a counted loop whose range START or STEP
// is an EVENT-produced value (`def a ((rg get 0) sub 1) … for [a b] […]`,
// utils/cut.boru's cut-pick-rng) compiles: the planner promotes the producer
// to a frame local (collectLoopRangeSources joins forceOrder) and the loop's
// FOR_SETUP re-pushes it, exactly as a param read did. Before 2026-09-26 the
// recorder declined "for: computed range start/step (Stage 2 follow-on)".
func TestLoopRangeEventStartCompiles(t *testing.T) {
	for _, src := range []string{
		`def a (1 add 1) end def b (2 add 3) end for [a b] [i]`,
		`def a (1 add 1) end for [a 5 (1 add 1)] [i]`,
		`def f fn [[rg:List][List][def a ((rg get 0) sub 1) def b (rg get 1) def out (flex []) for [a b] [def _p (push i out)] out]] end f [2 4]`,
		`def f fn [[xs:List rg:List out:List][List][def n (size xs) def a ((rg get 0) sub 1) def hi (rg get 1) def b (if (hi gt n) [n] [hi]) if (a gte b) [out] [for [a b] [def _p (push (xs get i) out)] out]]] end f ["a" "b" "c"] [2 3] (flex [])`,
		`def f fn [[xs:List rg:List out:List][List][def n (size xs) def a ((rg get 0) sub 1) def hi (rg get 1) def b (if (hi gt n) [n] [hi]) if (a gte b) [out] [for [a b] [def _p (push (xs get i) out)] out]]] end f ["a" "b" "c"] [3 99] (flex [])`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def a (1 add 1) end def b (2 add 3) end for [a b] [i]`)
	if !strings.Contains(dis, "FOR_SETUP") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("an event-sourced range must lower to the native loop:\n%s", dis)
	}
}

// TestSingleOverloadUserFnOverImpreciseOperandCompiles — a single-overload
// user fn whose sole signature's params are all NOMINAL, called over an
// IMPRECISE carrier (a poly re-match's result: `ds (each [nd] gradual)`,
// kg/ingest.boru's ingest-entity), records the guarded CALL_USER instead of
// declining "unmatched dispatch recovered": the entry check asks the
// interpreter's own nominal question and a missing value raises the same
// no_signature. A CONSTRAINED param (a predicate type) keeps the decline —
// the nominal guard could not enforce it.
func TestSingleOverloadUserFnOverImpreciseOperandCompiles(t *testing.T) {
	pre := `import module [def go fn [[m:Map k:String d:Any][Any][def v (m get k) if (v is None) [d] [v]]] def nd fn [[s:String][String][s]] def ds fn [[xs:List][List][xs]] `
	for _, c := range []struct{ body, call, want string }{
		{`def f fn [[raw:Map][Map][def al (each [nd] (go raw "aliases" [])) {aliases:(ds al)}]]`, `def b (K.f {aliases:["x"]}) end b.aliases`, "[['x']]"},
		{`def f fn [[raw:Map][Map][{aliases:(ds (each [var [[a] nd a]] (go raw "aliases" [])))}]]`, `def b (K.f {aliases:["x" "y"]}) end b.aliases`, "[['x' 'y']]"},
		{`def f fn [[raw:Map][Map][def al (each [nd] (go raw "aliases" [])) {aliases:(ds al)}]]`, `def b (K.f {aliases:[]}) end b.aliases`, "[[]]"},
	} {
		src := pre + c.body + ` export "K" {f: f/v}] end ` + c.call
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled=%v %v / %v, want %s", c.body, compiled, gotC, errC, c.want)
		}
	}
	fnValueM2CompileFailure(t, "a predicate-typed param keeps the decline",
		pre+`def Big (Integer gt 10) def bg fn [[n:Big][Integer][n]] def f fn [[raw:Map][Integer][def al (each [nd] (go raw "aliases" [])) (bg (size al))]] export "K" {f: f/v}] end K.f {aliases:["x"]}`,
		"unmatched dispatch recovered at bg")
}
