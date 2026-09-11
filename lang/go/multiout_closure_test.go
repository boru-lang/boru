package lang

// Multi-out closure bodies (CallableSpec.BodyOutResidual): `do` returns its
// body's ENTIRE residual, so a multi-value body compiles to ONE closure unit
// whose frameless RET leaves all N values — replacing the baked-const list
// that re-ran through an interpreter sub-engine at run time (classes 3+8 of
// design/DO-STRUCTURE-COMPILATION.0.md). Companion recorder work pinned here:
//   - out-of-order fn-unit residuals (an event above an inert bottom) promote
//     to frame locals and re-push in exact order (Finalize's forceOrder,
//     mirrored per unit);
//   - `error`'s ignore-handler compiles via StripsUnconsumedInput (the
//     runtime identity probe strips the unconsumed error from the residual
//     bottom), instead of islanding;
//   - an EMPTY body's closure residual is its pushed inputs verbatim,
//     matching InvokeBody — fixing the each []/fold []/scan [] miscompile
//     (compiled raised "body produced no result" against the interpreter's
//     identity map).

import (
	"fmt"
	"strings"
	"testing"
)

// compileDisasm compiles src and returns the disassembly, failing on refusal.
func compileDisasm(t *testing.T, src string) string {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected to compile; reason=%q err=%v", src, reason, cerr)
	}
	return prog.Disassemble()
}

// requireEngineParity runs src on both engines and asserts identical rendered
// results; wantCompiled additionally requires the compiled path to have run.
func requireEngineParity(t *testing.T, src string, wantCompiled bool) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
	if wantCompiled && !compiled {
		t.Errorf("%q: expected the compiled path to run", src)
	}
}

// TestDoMultiOutClosure — a multi-value inert body lowers to a TRUE closure
// (PUSH_CLOSURE + a unit RETting all N), not a baked const list run through a
// sub-engine; and the results seat for downstream consumption.
func TestDoMultiOutClosure(t *testing.T) {
	dis := compileDisasm(t, `do [10 20 30]`)
	if !strings.Contains(dis, "PUSH_CLOSURE") {
		t.Errorf("do [10 20 30]: want a PUSH_CLOSURE lowering, got:\n%s", dis)
	}
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("do [10 20 30]: must not island:\n%s", dis)
	}
	for _, src := range []string{
		`do [10 20 30]`,
		`do [10 20 30] add 5`,      // downstream word consumes the residual top
		`do [(1 add 2) (3 mul 4)]`, // two computed results
		`do [1 add 2 10 mul 4]`,    // out-of-order residual (event above literal)
		`def x 5  do [x 1 add 2]`,  // binding read + computed result
		`def x 5  do [x 1 add 2] add 100`,
		`do []`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestDoOutOfOrderResidualPromotes — the unit stores the computed result to a
// frame local and re-pushes the residual in exact order (the forceOrder
// mirror), rather than refusing "result above a literal".
func TestDoOutOfOrderResidualPromotes(t *testing.T) {
	dis := compileDisasm(t, `def x 5  do [x 1 add 2]`)
	if !strings.Contains(dis, "STORE_LOCAL") {
		t.Errorf("out-of-order residual: want a STORE_LOCAL promotion, got:\n%s", dis)
	}
}

// TestErrorStripInputClosure — the error-ignoring handler compiles as a paired
// closure (no island): the runtime strip nets one value from the 2-value
// residual whose bottom is the unconsumed error.
func TestErrorStripInputClosure(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	dis := compileDisasm(t, `do [raise x "e"] error ["fallback"]`)
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("error ignore-handler: must compile as a closure, not island:\n%s", dis)
	}
	for _, src := range []string{
		`do [raise x "e"] error ["fallback"]`,   // ignores the error → strip
		`do [raise x "e"] error [drop 9]`,       // consumes the error
		`do [raise x "e"] error []`,             // empty handler → error passes through
		`do [raise x "e"] error [dup drop "k"]`, // touches then nets one
		`do [5] error ["fb"]`,                   // success pass-through
		`do [1 div 0] error [drop -1]`,          // value-divergent body
	} {
		requireEngineParity(t, src, true)
	}
	// NEGATIVE: a handler netting >1 post-strip declines the closure (the
	// island/fallback owns it — parity still holds, it just isn't a closure).
	requireEngineParity(t, `do [raise x "e"] error ["a" "b"]`, false)
	// NEGATIVE: an all-inert-args deep handler also declines the ISLAND
	// (the baked forward prefix would exceed the barrier) — whole-program
	// fallback, parity intact.
	requireEngineParity(t, `5 error ["a" "b"]`, false)
	// NEGATIVE: a VARIADIC multi-out do body (arm counts differ) fails the
	// whole-residual exactness screen — the count is runtime-variable, so
	// the closure declines and the fallback owns it.
	requireEngineParity(t, `def b true  do [1 2 (if b [] [9 9])]`, false)
	// NEGATIVE: the historical forward-collection miscompile shape
	// (design/EDGE-SPEC-FINDINGS.0.md) must refuse or be correct — never
	// silently wrong.
	requireEngineParity(t, `5 do [raise x "e"] error [drop 9] add 1`, false)
}

// TestEmptyBodyClosureParity — an empty body's compiled closure returns its
// pushed inputs verbatim, matching InvokeBody: each/fold/scan identity-map
// behaviour is preserved under compilation (this DIVERGED before: the
// RET-nothing unit made the handlers raise "body produced no result").
func TestEmptyBodyClosureParity(t *testing.T) {
	for _, src := range []string{
		`[1 2] each []`,
		`[1 2 3] fold [] 0`,
		`[1 2 3] scan [] 0`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestDoSentinelBodyStaysUncompiled — a body carrying a flow-control sentinel
// still declines the closure (the sentinel targets an enclosing loop the VM
// cannot reach across the call boundary); the fallback owns it, and parity
// holds.
func TestDoSentinelBodyStaysUncompiled(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	src := `for 3 [ do [break] drop ]`
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
}

// TestDynBodyVariadicAndSpliceShapes — Phase E increment 2 pins:
//   - a NESTED multi-out dyn-body do inside a closure unit seats its
//     contiguous variadic residual (eventRunThenInert) and compiles;
//   - a variadic run followed by an INERT value seats it too (the
//     fifty-ninth increment: the suffix is PUSHED after the run, not indexed
//     past it), while a run followed by another EVENT still declines;
//   - a splice over a COMPUTED payload inside a do body compiles via the
//     dyn-body backstop with the interpreter's SPREAD semantics (the
//     captured-carrier closure previously baked identity — [7 8] against
//     the interpreter's 7 8, a miscompile);
//   - the bare top-level splice-of-computed refuses (recording poisoned)
//     with fallback parity.
func TestDynBodyVariadicAndSpliceShapes(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	compiles := []string{
		`def b true  do [do [1 2 (if b [] [9 9])]]`,
		`def mk fn [[] [List] [[7 8]]]  def xs (mk)  do [word xs]`,
		`def b true  do [1 2 (if b [] [9 9])]`,
		`def i 0  def ops [quote [1 add 2]]  do (ops get (i add 0))`,
		`def xs [add 1 2]  do [word xs]`,
		// A LOOP inside a do body: the unrolled model repeats ONE value
		// ([1 1 1] as the same Value), so tryRecordDynBody's intra-event ID
		// de-collision mints fresh result identities — without it, producedBy
		// collapsed to the last index and the residual refused "call results
		// reordered". Value above / below the run, and a computed range.
		`do [for 3 [1]]`,
		`do [for 3 [1] 7]`,
		`do [7 for 3 [1]]`,
		// The fifty-ninth increment: the run-then-inert body COMPILES now
		// rather than riding as a List const the handler interprets. Two
		// values above the run, and a branch region rather than a loop.
		`do [for 3 [1] 7 8]`,
		`def b true  do [(if b [] [9 9]) 1 2]`,
		// Still the dyn-body strategy, and both must keep answering: a
		// second region above the first, and a call result above the run.
		`do [for 3 [1] for 2 [9]]`,
		`do [for 3 [1] (1 add 2)]`,
		`def n 3  do [for n [1]]`,
		// forceOrder ∪ dynBind-source merge (planValueDefLocals): the SECOND
		// do's closure unit has an out-of-order residual (inert x below the
		// computed result — residualForceOrder) AND a dyn-bound computed
		// body-local (`def q (1 add 2)` under the DynEnv armed by the first
		// do) — both promotion triggers drive one merged set.
		`def ys [mul 2 3]  do [word ys]  def x 5  do [def q (1 add 2)  x  q add 10]`,
	}
	for _, src := range compiles {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: must compile; reason=%q err=%v", src, reason, cerr)
			continue
		}
		requireEngineParity(t, src, true)
	}
	// The variadic-run-then-value body also compiles: the closure declines
	// (exactness screen) and the dyn-body CALL_NATIVE takes the whole body.
	requireEngineParity(t, `def b true  do [do [1 2 (if b [] [9 9])] 7]`, true)
	// GRADUATED (REFUSAL-CLOSURE §9.2b, 2026-07-17): the computed-payload
	// splice compiles to OpSpliceDyn — a DATA payload spreads verbatim, a
	// code-bearing one defers to the interpreter at run time. Full family
	// pinned in bytecode_splicedyn_test.go.
	requireEngineParity(t, `def mk fn [[] [List] [[7 8]]]  def xs (mk)  word xs`, true)
	// GRADUATED by the fifty-ninth increment, and it had been the pin for
	// the sentence that was wrong: `def f fn [[] [] [for 3 [1] 7]]  f` was
	// refused as "a variadic loop value MID-residual … the RET cannot seat a
	// runtime-variable count with a fixed value above it". Above it is
	// exactly where a fixed value CAN go — it is pushed after the run — and
	// the shape that genuinely cannot seat is a fixed value BENEATH the run,
	// which is what the mark plan exists for. A no-contract fn RETs whatever
	// the body leaves, so the run and the 7 above it leave together.
	requireEngineParity(t, `def f fn [[] [] [for 3 [1] 7]]  f`, true)
	refusals := []string{
		// What still refuses at that screen, and the honest witness for it:
		// an EVENT above the run. A call result is not pushed after the run,
		// it is already on the simulated stack in its own production order,
		// so seating it above a length nothing knows is the indexing problem
		// the inert suffix does not have.
		`def f fn [[] [] [for 3 [1] (1 add 2)]]  f`,
	}
	for _, src := range refusals {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", src, cerr)
		}
		if prog != nil || reason == "" {
			t.Errorf("%q: must refuse with a named reason; got prog=%v reason=%q", src, prog != nil, reason)
		}
		requireEngineParity(t, src, false)
	}
}
