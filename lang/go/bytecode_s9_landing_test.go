package lang

import (
	"fmt"
	"testing"
)

// COMPILE FAILURE-CLOSURE §9 landing battery — written FAILING-FIRST (the goal's
// test-driven discipline): every subtest asserts the TARGET compile-parity
// behavior of one §9 item and fails until its landing flips it. The `want`
// values are the interpreter's (probe-pinned before any implementation).

const s9DocMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] def boom fn [[x:Any] [Any] [ raise bad_input "always" ]] export "M" {dec: dec/v, boom: boom/v} ] end `

func TestS9LoopCarriedVariadicStore(t *testing.T) { // §9.2a — LANDED
	// A loop-carried def of a variadic loop region is the §5 first-value
	// split one frame down. Two pieces landed it: (1) the split gate admits
	// NestedBodyDepth == LoopBodyDepth (every enclosing body analysis is an
	// unconditional-per-iteration LOOP body — a branch arm keeps declining,
	// the P1-b leak fence); (2) the bind's depth stamp subtracts the one
	// analysis-only shadow the final AnalyseLoopBody round carries (the
	// inflated depth made the runtime SetAt a silent no-op and the in-loop
	// read resolved the kept check-time carrier — the historical [5 0 5 0]).
	// The statically-counted rest rides as the iteration's residual
	// (splitBound → multiOut), and reads resolve via OpLookupDynScope.
	mustCompileWithParity(t, `for 2 [ def acc (for 2 [5]) acc ]`, "[5 5 5 5]")
	mustCompileWithParity(t, `for 3 [ def acc (for 2 [1]) ]`, "[1 1 1]")
	mustCompileWithParity(t, `for 2 [ def acc (for 3 [7]) acc ]`, "[7 7 7 7 7 7]")
	mustCompileWithParity(t, `for 2 [ def acc (for 2 [5]) 9 ]`, "[5 9 5 9]")
	mustCompileWithParity(t, `for 2 [ def acc (for 1 [7 "x"]) acc ]`, "[x 7 x 7]")
	mustCompileWithParity(t, `for 2 [ def acc (for 2 [5]) ] acc`, "[5 5 5]")

	// Decline fences, each parity-faithful: a DYNAMIC inner count (no static
	// region), a BRANCH between the loop and the def (conditionally-reached
	// split — the P1-b leak), and a NESTED loop pair (depth-2 fence).
	mustFailToCompileWithParity(t,
		`def m {n: 2} for 2 [ def acc (for (m get "n") [5]) acc ]`, "")
	mustFailToCompileWithParity(t,
		`for 2 [ if true [ def acc (for 2 [5]) acc ] [0] ]`, "")
	mustFailToCompileWithParity(t,
		`for 2 [ for 2 [ def acc (for 2 [5]) acc ] ]`, "")

	// PR #280 review reachability fences: the depth equality proves the BODY
	// runs per iteration, not that the SPLIT SITE is reached with its
	// residual intact — so LoopBodyDepth now stamps only under a STATIC
	// >= 1-trip count and a sentinel-free body (AnalyseLoopBody provenTrips).
	// A COMPUTED trip count declines (a runtime count of 0 leaked the
	// analysis-only binding: compiled 0 vs interp undefined_word), as does an
	// upstream `continue` (the bind is bypassed: compiled 0 vs undefined_word)
	// and a downstream `break` (a discarded iteration's spill survived:
	// compiled [5 5] vs interp [5]).
	mustFailToCompileWithParity(t,
		`def m {n:0} for (m get "n") [ def acc (for 2 [5]) ] acc`, "consumes loop results")
	mustFailToCompileWithParity(t,
		`def m {n:1} for (m get "n") [ def acc (for 2 [5]) ] acc`, "consumes loop results")
	mustFailToCompileWithParity(t,
		`for 1 [if true [continue] [] def acc (for 2 [5])] acc`, "consumes loop results")
	mustFailToCompileWithParity(t,
		`for 3 [def acc (for 2 [5]) break] acc`, "consumes loop results")
}

func TestS9SpliceComputedPayload(t *testing.T) { // §9.2b
	mustCompileWithParity(t, `def xs (range 1 3) def d word xs d`, "[1 2]")
}

func TestS9XmlInterpComputed(t *testing.T) { // §9.2c
	mustCompileWithParity(t, `def n 7 <p>${n add 1}</p>`, "[<p>8</p>]")
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p>${x}</p>]] f 5`, "[<p>5</p>]")
}

func TestS9CurriedFactory(t *testing.T) { // §9.2d
	// Still [3], but no longer by accident: the residual lowering used to
	// apply EVERY leading carrier, which is right here and wrong for the
	// unwrapped `(mk 1) 2` (NUR101, design/PAREN-RESTEP-RULE.0.md). The
	// re-step record separates them.
	mustCompileWithParity(t,
		`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] ((mk 1) 2)`, "[3]")
}

func TestS9ParenBoundedLeadingApply(t *testing.T) { // §9.2e
	// The stamp-suite shape: the apply inside an FN BODY (top-level already
	// compiles via RecordDynApply).
	interpOnlyWithCompileFailure(t,
		`def m {f: ([y:Integer] => [y add 1])} def h fn [[x:Integer] [Integer] [ add 1 (x (m get "f") apply) ]] h 7`, "[9]")
}

func TestS9SurfaceTypedDispatch(t *testing.T) { // §9.2f
	mustCompileWithParity(t,
		`def Comparable surface {cmp: (fnsig [[Self Self] [Integer]])} Integer exposes Comparable def g gen [(T extends Comparable)] fn [[a:T b:T] [Integer] [cmp a b]] g 3 5`,
		"[1]")
}

func TestS9FnComputedOperand(t *testing.T) { // §9.2g — DESIGNED KEEP
	// A fn constructed over a COMPUTED input sig (`fn (2 add 3) [Integer]
	// [7]`) is runtime-value-dependent: the param PATTERN is the runtime
	// value (5), so a compiled unit would bake the check-time carrier where
	// the live value belongs. No runtime op short of re-constructing the
	// FnDefInfo per evaluation fixes it, and that machinery is unjustified
	// for so exotic a shape — a designed opt-out like §8. Pinned declining
	// with interpreter parity.
	mustFailToCompileWithParity(t, `def f fn (2 add 3) [Integer] [7] f 5`,
		"triple-form construction over a computed")
	mustFailToCompileWithParity(t, `(2 add 3) afn [7]`,
		"construction over a computed")
}

func TestS9FrontierDefOverCatchRegion(t *testing.T) { // §9.1 rows 1-2 — NARROWED (PR #280 review)
	// A DEF binding the FIRST value of a do-catch's multi-value region:
	// SplitEventRegionBind (the S5/S9.2a split's STATIC-region twin) binds a
	// fresh element carrier via the same splice-at-depth OpBindGlobal
	// (SpliceFromTop = nout-1) and removes the spliced slot from the sim.
	// NARROWED 2026-07-20 (PR #280 review): the split admits only a region
	// the fallibility scan PROVES infallible — a wordless literal region, or
	// one whose fallible call the check collapsed statically (the div rows
	// below). Any callable in the region is fallible (integer overflow made
	// even `add` so — probe-pinned: maxint add 1 through the original
	// designed row underflowed BIND_GLOBAL's splice at run time, there IS no
	// runtime wholesale-defer for a surprise raise), so the original
	// `(1 add 2)` designed rows now DECLINE and fall back; re-landing them
	// needs the variable-arity residual model (OpStackMark/OpDropToMark —
	// the L-DO roadmap in frontier-do-catch.tsv).
	mustCompileWithParity(t,
		`def x (do [10 "x"] error [dot code]) x`, "[x 10]")
	// Double read re-resolves the live binding (OpLookupDynScope).
	mustCompileWithParity(t,
		`def x (do [10 "x"] error [dot code]) x x`, "[x 10 10]")
	// The word-bearing designed rows: compile failure + fallback parity.
	mustFailToCompileWithParity(t,
		`def msg (do [(1 add 2) "no-raise"] error [dot code]) msg`, "unpromoted computed value")
	mustFailToCompileWithParity(t,
		`def msg (do [(1 add 2) "a"] error [dot code]) msg msg`, "unpromoted computed value")
	// The RUNTIME-SURPRISE raise that forced the narrowing, both arities.
	mustFailToCompileWithParity(t,
		`def msg (do [(9223372036854775807 add 1) "x"] error [dot code]) msg`, "unpromoted computed value")
	mustFailToCompileWithParity(t,
		`def x (do [(9223372036854775807 add 1) "a" "b"] error [dot code]) x`, "variadic result promoted")
	// The unflagged-fallible-native twin (getr's not_found — the PR #280
	// review follow-on this taxonomy closes).
	mustFailToCompileWithParity(t,
		`def m {a:1} def x (do [(m getr "zz") "a" "b"] error [dot code]) x`, "variadic result promoted")
	// The fallibility scan descends into NESTED lists: the fallible call
	// buried in the inner list marks the region exactly as a top-level one.
	// The compile failure used to surface at the reorder stage ("residual shape
	// beyond Stage 1") because the def's value, `[Integer]`, read as
	// concrete and the def lowered to nothing; since the write-back is
	// decided by provenance (rootBindWritesBack, the sixty-third
	// increment) a computed compound writes back like its scalar siblings
	// above, and the def declines first — the same compile failure.
	mustFailToCompileWithParity(t,
		`def x (do [[1 add 2] "x"] error [dot code]) x`, "unpromoted computed value")
	// The RAISING region rides the catch path: the compiled run defers and
	// the interpreter owns the catch — value parity through the defer.
	{
		src := `def msg (do [(0 div 0) "x"] error [dot code]) msg`
		a, _ := New()
		gotC, _, errC := a.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			return
		}
		b, _ := New()
		gotI, errI := b.RunInterp(src)
		if errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("catch path: compiled=%v (%v) interp=%v (%v)", gotC, errC, gotI, errI)
		}
	}

	// The doc's M.dec ERROR rows: the region's paren call genuinely fails
	// dispatch — a GUARANTEED-error program, so there is no clean idx-0
	// event for the split to bind (SplitEventRegionBind declines) and the
	// compile failure stands. (The no-def form of the same region declines "check
	// diagnostics" — either compile failure is the sound terminal state for a
	// program that cannot run.)
	//
	// The interpreter's half changed with the loud dispatch contract
	// (design/legacy/FN-VALUE-DISPATCH.0.ignore): the failure is raised AT THE
	// DISPATCH SITE, which is inside this `do [...]`, so the region's own
	// error handler catches it and the program yields the caught code.
	// Under the old residue model the error surfaced from the end-of-run
	// drain instead — outside every handler — so a program that explicitly
	// asked to trap its failures was aborted by one anyway.
	for _, src := range []string{
		s9DocMod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`,
		s9DocMod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`,
	} {
		a, _ := New()
		prog, _, _, _ := a.CompileCheck(src)
		b, _ := New()
		gotI, errI := b.RunInterp(src)
		if prog != nil || errI != nil || fmt.Sprint(gotI) != "[uncalled_function]" {
			t.Errorf("%.50q: want compile failure + the do-catch yielding uncalled_function, got prognil=%v interp %v (%v)",
				src, prog == nil, gotI, errI)
		}
	}

	// Fences (compile failure / runtime defer, parity-faithful): a TWO-split
	// program (the second read is a dynamic value preceding residual args)
	// and a THREE-value region (runtime defer).
	{
		src := `def a (do [(1 add 2) "a"] error [dot code]) def b (do [(3 add 4) "b"] error [dot code]) a b`
		x, _ := New()
		prog, _, _, _ := x.CompileCheck(src)
		if prog != nil {
			t.Errorf("two-split program compiled — graduate this fence")
		}
	}

	// PR #280 review fences. The split seats exactly the TWO-value region
	// (nout != 2 declines — a wider one used to underflow BIND_GLOBAL's
	// splice at run time). A word-bearing region's promotion declines: its
	// event carries the variadic mark (the fallibility latch or the
	// dyn-body record), and lowerCall's store-prologue gate rejects the
	// seat whose raise path delivers ONE caught Error where nout success
	// values were promoted (STORE_LOCAL underflow, probe-pinned on raise,
	// div, add-overflow, and a fallible user fn). The `(1 add 2)` instance
	// cannot ride on its benign operands — fallibility is the word's, not
	// the call site's.
	mustFailToCompileWithParity(t,
		`def x (do [(1 add 2) "a" "b"] error [dot code]) x`, "variadic result promoted")
	mustFailToCompileWithParity(t,
		`def x (do [(0 div 0) "a" "b"] error [dot code]) x`, "variadic result promoted")
	mustFailToCompileWithParity(t,
		`def x (do [(raise aa "m") "a" "b"] error [dot code]) x`, "variadic result promoted")
	mustFailToCompileWithParity(t,
		`def f fn [[n:Integer][Integer][if (n gt 0) [raise aa "m"] [n]]] def x (do [(f 1) "a" "b"] error [dot code]) x`,
		"variadic result promoted")
	// The BARE multi-value raising region keeps native parity — no seat, no
	// stores: the caught Error lands where the handler consumes it.
	mustCompileWithParity(t, `do [(0 div 0) "a" "b"] error [dot code]`, "")
}

func TestS9FrontierModuleExportInRegion(t *testing.T) { // §9.1 row 3
	mustCompileWithParity(t, s9DocMod+`do [M 3] error [dot code]`, "")
}
