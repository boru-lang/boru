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
	// Any nested position declines loudly, with parity through the fallback —
	// and so does a body that could CHANGE a binding the program reads after
	// it (a def or undef of a name bound at the dispatch: the check pass
	// never runs the body, so the later read would bake the pre-body
	// binding; a Codex review of #509). A def of a FRESH name stays fine.
	for _, src := range []string{
		`import "boru:test" end def f fn [[n:Integer][Integer][Test.cover [n] n]] end f 3`,
		`import "boru:test" end for 2 [Test.cover [ def spec {a:i} end Test.test "t" [spec.a i Assert.equal] ]] end Test.fail-count`,
		`import "boru:test" end if true [Test.cover [ Test.test "t" [1 1 Assert.equal] ]] [] end Test.fail-count`,
		`import "boru:test" end def x 1 end Test.cover [def x 2] end x`,
		`import "boru:test" end def x 1 end Test.cover [undef x] end x`,
	} {
		fnValueM2CompileFailure(t, "nested Test.cover: "+src, src, "code-body word test-cover")
	}
	// A re-import inside the body binds the SAME loaded module: no rebind.
	requireEngineParity(t, `import "boru:test" import "boru:cli" end def a Cli end Test.cover [import "boru:cli"] end (a eq Cli)`, true)
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
	// A predicate-typed param over the SAME shape: this row declined
	// "unmatched dispatch recovered at bg" until 2026-09-26, when `each` over
	// a dynamic collection stopped committing to a strict Map
	// (TestEachOverDynamicCollectionIsNotAStrictMap). `size al` is then a
	// dynamic Integer that MATCHES bg directly — no recovery — and the
	// CALL_USER entry guard (checkParamContract's SigTypeMatches) runs Big's
	// predicate over the runtime value: the interpreter's signature_error when
	// it fails, its answer when it holds. The failing row compares code and
	// detail, not the rendered caret: the guard's raise inside a unit carries
	// no source position — NUR171's family, pre-existing (a plain top-level
	// `bg (h "s")` already carets the argument, not the word, on main).
	bgSrc := func(bound string) string {
		return pre + `def Big (Integer gt ` + bound + `) def bg fn [[n:Big][Integer][n]] def f fn [[raw:Map][Integer][def al (each [nd] (go raw "aliases" [])) (bg (size al))]] export "K" {f: f/v}] end K.f {aliases:["x"]}`
	}
	requireEngineParity(t, bgSrc("0"), true)
	gotC, compiled, errC, gotI, errI := runBothEngines(t, bgSrc("10"))
	if !compiled || codeOf(errC) != "signature_error" || codeOf(errI) != "signature_error" || len(gotC) != len(gotI) ||
		!strings.Contains(fmt.Sprint(errC), "cannot call `bg`") || !strings.Contains(fmt.Sprint(errI), "cannot call `bg`") {
		t.Errorf("a failing predicate: compiled=%v C=%v/%v I=%v/%v; want the interpreter's `cannot call bg` on both lanes", compiled, gotC, errC, gotI, errI)
	}
	// An UNDER-ARITY call and a QUOTED param are the interpreter's
	// signature_error, never a recovery (a Codex review of #509): the guard
	// would bind a partial window, or evaluate what the matcher captures.
	fnValueM2CompileFailure(t, "an under-arity call keeps the decline",
		pre+`def ds2 fn [[xs:List n:Integer][List][xs]] def f fn [[raw:Map][List][def al (each [nd] (go raw "aliases" [])) al ds2]] export "K" {f: f/v}] end K.f {aliases:["x"]}`,
		"unmatched dispatch recovered at ds2")
	fnValueM2CompileFailure(t, "a quoted param keeps the decline",
		pre+`def dq fn [[xs:List/q][List][[1]]] def f fn [[raw:Map][List][def al (each [nd] (go raw "aliases" [])) (dq al)]] export "K" {f: f/v}] end K.f {aliases:["x"]}`,
		"unmatched dispatch recovered at dq")
	// A GENERIC fn is excluded from the recovery and from the imprecise-tag
	// narrowing (its param is a type variable the generic lane binds per
	// call): generics-fn.tsv L54 compiled through them for one push and ran
	// the call on the interpreter's generic host, an interp-entry census
	// row. It compiles natively through the established arms — a plain
	// user-call unit, no generic dispatch — and both lanes agree.
	gsrc := `def Box gen [T] class {value:T} def unbox gen [T] fn [[b:T] [Any] [b dot value]] end [(make (Box of [Integer]) {value:1})] each [unbox]`
	requireEngineParity(t, gsrc, true)
	if dis := compileDisasm(t, gsrc); strings.Contains(dis, "GENERIC") || strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "CALL_USER") {
		t.Errorf("the generic fn's call must be a plain compiled user call:\n%s", dis)
	}
}
