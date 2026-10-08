package lang

import (
	"strings"
	"testing"
)

// TestLoopBodyVariadicBranchCompiles pins the loop body whose ONE result is
// a 0-or-1 event — a variadic `if` (one arm binding a name, the other
// leaving a value) or a `case` with no matching clause on some element —
// which used to decline "loop results as a branch/body result (Stage 2)"
// (code-bodies.tsv rows 181 and 182). A Stage 2 loop's region is a
// runtime-variable count already and only the program residual absorbs
// it, so a per-iteration count of 0 or 1 costs the region nothing: the
// fragment admits the variadic out for a loop body (lowerFragment's
// loopBody note from lowerLoop), never for a loop condition, and marks the
// loop (lowerer.bodyVariadic) so the S5 first-value bind over it — whose
// static depth assumes one value per iteration — declines instead of
// splicing from a region that has no static shape.
func TestLoopBodyVariadicBranchCompiles(t *testing.T) {
	for _, src := range []string{
		// The corpus rows.
		`def t 0 end for 2 [if true [def t (t add 1)] [0]] end t`,
		`def t 0 end for 2 [case 1 [1 [drop def t (t add 1)] 0]] end t`,
		// The region's count varies per iteration, and the residual absorbs it.
		`for 3 [if (i eq 1) [i] []]`,
		`for 3 [if (i eq 1) [] [i]]`,
		`for 3 [if (i eq 1) [i] []] 9`,
		`for 2 [for 2 [if (i eq 1) [i] []]]`,
		`def t 0 end for 2 [if true [def t 5] [0]] end t`,
		// A while body, and a loop-carried rebind in either arm.
		`def i 0 end while [i lt 3] [if (i eq 1) [def i (i add 1)] [def i (i add 1) i]] end i`,
	} {
		requireEngineParity(t, src, true)
	}
	// Inside a fn the RET's contract judges what the loop left, on both
	// lanes: a single Integer where a List was declared raises the same
	// type_error (the two lanes' renderings of the source excerpt differ,
	// so the codes are compared, not the texts).
	fnSrc := `def f fn [[][List][for 3 [if (i eq 1) [i] []]]] end f`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, fnSrc)
	if !compiled || codeOf(errC) != "type_error" || codeOf(errI) != "type_error" || len(gotC) != 0 || len(gotI) != 0 {
		t.Errorf("%q: want both lanes to raise type_error at the RET; compiled=%v %v/%v interp=%v/%v", fnSrc, compiled, gotC, errC, gotI, errI)
	}

	// The lowering: the loop's body is the variadic `if` and nothing islands.
	dis := compileDisasm(t, `for 3 [if (i eq 1) [i] []]`)
	if !strings.Contains(dis, "FOR_NEXT") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the variadic-body loop must lower natively:\n%s", dis)
	}

	// The fence: a def binding the loop's FIRST value (the S5 split) has no
	// static region to splice from when the body leaves 0 or 1 — a loud
	// decline, counted (compile_defect_test.go's ledger), where the split's
	// static depth would have read the wrong value.
	requireEngineParity(t, `def x (for 3 [if (i eq 1) [i] []]) x`, false)
	// A loop CONDITION keeps its one-value rule (the interpreter raises).
	requireEngineParity(t, `def i 0 end while [if (i lt 2) [true] []] [def i (i add 1) i]`, false)
}
