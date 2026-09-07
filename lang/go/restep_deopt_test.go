package lang

import (
	"fmt"
	"strings"
	"testing"
)

// restep_deopt_test.go pins NUR124's TIMING axis (measured 2026-09-07). A
// native word's results go back onto the tape and the interpreter steps
// them, so an unquoted fn among them dispatches WHERE IT LANDS — after the
// shuffle that put it on top, before the next word runs: `[g/v] each [5 swap
// drop]` applies g to 5 and then drops the 15, an each_error. The check
// pass steps a fn-typed carrier past as data, so the compiled body ran drop
// over g and answered [5] on the DEFAULT lane, exit 0. The engine now notes
// each such result (NoteFnResultReStep), the unit plans a RE-STEP point
// right after the call (planReStepDeopts), and the VM hands the results as
// TOKENS plus the rest of the body to the interpreter when one is a fn
// (DEOPT_IF_FN with Results) — with the unit's unpushed unnamed inputs seated
// beneath the region, the interpreter's own frame bottom. A fn-typed note no
// point serves refuses instead of miscompiling.

const rsG = `def g fn [[x:Integer][Integer][x mul 3]]`

// TestReStepDeoptParity pins the shapes that now agree on both lanes, value
// and error alike, and COMPILE (the deopt is a lowering, not an island of
// the whole word).
func TestReStepDeoptParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{rsG + `  [g/v] each [5 swap drop]`, "each_error — g applied at the swap, drop empties the body; was [5]"},
		{rsG + `  [g/v] each [5 swap drop 9]`, "[9] — was [9] for the wrong reason"},
		{rsG + `  [g/v] each [5 swap drop 9 swap]`, "cannot call `swap` — the 15 was dropped; was the NUR124 refusal"},
		{rsG + `  def m {f: g/v}  [5] each [m get "f" drop]`, "a gradual map read re-stepped: each_error; was [5]"},
		{rsG + `  def m {f: g/v}  [5] each [m get "f" drop 7]`, "[7]"},
		{rsG + `  [5] each [{f: g/v} get "f" drop]`, "an inline map's fn member: each_error; was [5]"},
		{rsG + `  def f fn [[h:Function][Any][[h/v] each [5 swap drop]]]  f g/v`, "inside a fn body: each_error; was [5]"},
		{rsG + `  [1 g/v] each [5 swap drop]`, "a mixed list: element 1 errors"},
		// controls: plain data costs the test and nothing else.
		{rsG + `  def m {f: 1}  [5] each [m get "f" drop]`, "[5] — a data member"},
		{rsG + `  def m {f: g/v}  [5 6] each [m get "f" 1 add]`, "[8 9] — the fn collects the written 1"},
		{`[1 2] each [5 swap drop 9]`, "[9 9] — no fn anywhere"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestReStepDeoptLowering pins the op: the each body carries ONE DEOPT_IF_FN
// right after the swap, over its two results; a body no fn-typed result
// re-steps carries none.
func TestReStepDeoptLowering(t *testing.T) {
	a, _ := New()
	prog, reason, _, err := a.CompileCheck(rsG + `  [g/v] each [5 swap drop 9]`)
	if err != nil || prog == nil {
		t.Fatalf("refused: %q %v", reason, err)
	}
	dis := prog.Disassemble()
	if strings.Count(dis, "DEOPT_IF_FN") != 1 || !strings.Contains(dis, "re-step 2 result(s) on the interpreter if one is a fn") {
		t.Errorf("one re-step point over swap's two results:\n%s", dis)
	}
	if i, j := strings.Index(dis, "swap (Any, Any)"), strings.Index(dis, "DEOPT_IF_FN"); i < 0 || j < i {
		t.Errorf("the point follows the swap:\n%s", dis)
	}
	b, _ := New()
	prog, reason, _, err = b.CompileCheck(`[1 2] each [5 swap drop 9]`)
	if err != nil || prog == nil {
		t.Fatalf("refused: %q %v", reason, err)
	}
	if dis := prog.Disassemble(); strings.Contains(dis, "DEOPT_IF_FN") {
		t.Errorf("no fn-typed result, no point:\n%s", dis)
	}
}

// TestReStepDeoptSiblingsUnchanged pins the neighbours the note must leave
// alone: a fn re-stepped with nothing plain after it belongs to the residual
// arms (the each islands, as before), a concrete fn element is dispatched by
// the pass itself, and the interpreter's own dispatch-at-the-shuffle rows
// keep agreeing.
func TestReStepDeoptSiblingsUnchanged(t *testing.T) {
	rows := []string{
		rsG + `  [g/v] each [5 swap]`,
		rsG + `  [g/v] each [5 swap 7]`,
		rsG + `  [g/v] each [5 tuck]`,
		rsG + `  [g/v] each [5 swap 6 swap]`,
		rsG + `  [g/v] each [dup drop 5]`,
		rsG + `  [5] each [[g/v] get 0]`,
	}
	for _, src := range rows {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

// TestClosureValueReStepParity pins NUR124's PAYLOAD axis (the
// twenty-fifth increment): a produced CLOSURE the body shuffles is re-stepped
// exactly as the interpreter's own fn value — by the island's sub-engine
// (the interpreter's execFnDefLiteral bridges a ClosurePayload to a
// dispatchable fn through CompiledRuntime.ClosureAsFnDef) and by the
// compiled unit's re-step deopt, whose test now admits a closure — while a
// 0-arg anonymous closure nothing calls parks as the interpreter's lambda
// does, and a closure whose argument does not match stays data.
func TestClosureValueReStepParity(t *testing.T) {
	const mk = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  `
	const mk0 = `def mk0 fn [[][Function][([] => [42])]]  `
	rows := []struct{ src, note string }{
		{mk + `[(mk 3)] each [5 swap]`, "[15] — was [fn (Integer)]"},
		{mk + `[(mk 3)] each [5 over]`, "[45] — was [fn (Integer)]"},
		{mk + `[(mk 3)] each [5 swap drop]`, "each_error — was [5]"},
		{mk + `[(mk 3)] each [5 swap drop 9]`, "[9]"},
		{mk + `[(mk 3)] each [5 swap 7]`, "[21] — the closure collects the written 7; was [7]"},
		{mk + `[(mk 3) (mk 4)] each [5 swap]`, "[15 20] — was [fn fn]"},
		{mk + `[(mk 3)] each [dup drop 5]`, "[5] — the control"},
		{mk + `[(mk 3)] each ["x" swap]`, "[fn (Integer)] — no Integer to collect, data on both lanes"},
		{mk0 + `[(mk0)] each [dup drop]`, "[fn] — a 0-arg lambda VALUE parks (ADR-016)"},
		{mk0 + `[(mk0)] each [5 swap]`, "[fn] — parks above the 5 too"},
		{mk0 + `[(mk0)] each [5 swap drop]`, "[5]"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestReStepDeoptOpenShapes pins, as MEASURED, the neighbours these
// increments leave open, so a change in any of them is noticed: a fold of a
// static list index hands the fn-typed LOCAL itself to the re-step (no event
// to test after, so nothing is noted), and a fn-typed value re-stepped at
// the MAIN program (no unit body to resume into) keeps the optimistic model
// when its note is gradual.
func TestReStepDeoptOpenShapes(t *testing.T) {
	rows := []struct{ src, interp, compiled string }{
		{rsG + `  def f fn [[h:Function][Any][[5 h/v] get 1 drop 7]]  f g/v`, "uncalled_function", "[7]"},
		{rsG + `  def m {f: g/v}  m get "f" drop 7`, "uncalled_function", "[7]"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled", c.src)
			continue
		}
		if errI == nil || !strings.Contains(errI.Error(), c.interp) {
			t.Errorf("%q: interpreter: want %s, got %v/%v", c.src, c.interp, gotI, errI)
		}
		if errC != nil || fmt.Sprint(gotC) != c.compiled {
			t.Errorf("%q: compiled (measured, open): want %s, got %v/%v", c.src, c.compiled, gotC, errC)
		}
	}
}
