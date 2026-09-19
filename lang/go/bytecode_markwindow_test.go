package lang

import (
	"fmt"
	"testing"
)

// The mark-window island (plan Phase 5, L-DO part 2b): a fallible do-catch
// residual — 1 caught Error vs N values at run time — plus the values above
// it lowers as ONE verbatim window: Finalize's markWindowShape opens an
// OpStackMark before the region-starting do event, nothing re-pushes
// (verifyMarkWindow pins the residual IS the lowered stack), and
// OpCallDynMixedFromMark re-steps stack[mark:] through the island exactly as
// the interpreter — auto-apply hazard included.

const mwDocMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] def boom fn [[x:Any] [Any] [ raise bad_input "always" ]] export "M" {dec: dec/v, boom: boom/v} ] end `

// mwParityCompiled asserts the row force-compiles and runs COMPILED with
// value/error parity against the interpreter.
func mwParityCompiled(t *testing.T, src string) {
	t.Helper()
	a := mustNew(t)
	if _, reason, _, err := a.CompileCheck(src); err != nil || reason != "" {
		t.Fatalf("the shape must compile, got refusal %q / err %v\n  src: %s", reason, err, src)
	}
	b := mustNew(t)
	outC, ran, errC := b.RunCompiled(src)
	if !ran {
		t.Fatalf("the shape must run COMPILED, fell back (err %v)", errC)
	}
	c := mustNew(t)
	outI, errI := c.RunInterp(src)
	if (errC == nil) != (errI == nil) || fmt.Sprint(outC) != fmt.Sprint(outI) {
		t.Errorf("parity: compiled %v/%v != interp %v/%v\n  src: %s", outC, errC, outI, errI, src)
	}
}

// mwRefusedWithParity asserts the row still refuses and the fallback matches
// the interpreter exactly.
func mwRefusedWithParity(t *testing.T, src, wantReason string) {
	t.Helper()
	a := mustNew(t)
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog != nil {
		t.Fatalf("the shape GRADUATED (it now compiles) — move it into the compiles battery and graduate its ledger row\n  src: %s", src)
	}
	if wantReason != "" && reason != wantReason {
		t.Errorf("refusal reason drifted: %q, want %q — re-diagnose", reason, wantReason)
	}
	b := mustNew(t)
	outC, ran, errC := b.RunCompiled(src)
	if ran {
		t.Fatal("refused program must fall back")
	}
	c := mustNew(t)
	outI, errI := c.RunInterp(src)
	if (errC == nil) != (errI == nil) || fmt.Sprint(outC) != fmt.Sprint(outI) {
		t.Errorf("fallback parity: compiled %v/%v != interp %v/%v", outC, errC, outI, errI)
	}
}

func TestMarkWindowDoCatchCompiles(t *testing.T) {
	rows := []string{
		// The always-raising module fn: the caught path (1 Error in the
		// region, "x" never pushed at run time).
		mwDocMod + `do [(M.boom 5) "x"] error [dot code]`,
		// The [Any]-returning user fn twin.
		`def f fn [[x:Any] [Any] [raise bad_input "nope"]]  do [(f 5) 2] error [dot code]`,
		// The branch-arm nesting: the raise sits inside a taken if arm.
		mwDocMod + `do [(if true [M.boom 5] [7]) 8] error [dot code]`,
		// The dry-pass-proven raising constant (the StructUtil chained leaf).
		`import "boru:struct-util"  def g StructUtil.parse/v  do [(g "") 2] error [dot code]`,
	}
	for i, src := range rows {
		t.Run(fmt.Sprintf("row-%d", i), func(t *testing.T) {
			mwParityCompiled(t, src)
		})
	}
}

// TestMarkWindowDeclinesKeepParity — the shapes the window rightly declines:
// a PROMOTED def read (`def msg (…) msg` — the def popped the result to a
// frame slot, so it is not live in the window). Its ledger row stays in
// frontier-do-catch.tsv; this pin fires when a later widening graduates it.
// (The module-export-in-region sibling graduated 2026-07-17 — §9.1.)
func TestMarkWindowDeclinesKeepParity(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// Re-diagnosed 2026-07-20 (PR #280 review): the promotion itself is now
	// the refusal — a variadic catch region's stores would pop success-arity
	// values the raising run never produced — caught at lowerCall's
	// store-prologue gate, one stage before the window's own "residual shape
	// beyond Stage 1 (call result above a literal)" decline this row used to
	// surface. Same refusal, earlier and truer diagnosis.
	//
	// Re-diagnosed again 2026-07-30 (design/legacy/FN-VALUE-DISPATCH.0.ignore): the
	// region's `M.dec` call fails dispatch, and that is now an error-severity
	// check diagnostic in the model-undermining class (dispatch did not
	// resolve, so there is nothing to compile), so the pipeline refuses on the
	// diagnostics before reaching the promotion gate. The gate is still what a
	// widening would have to graduate — see the ledger note in
	// frontier-do-catch.tsv — but this row can no longer reach it. Parity is
	// what this test actually guards, and it holds either way.
	mwRefusedWithParity(t,
		mwDocMod+`def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`,
		"check diagnostics")
	// GRADUATED 2026-07-17 (§9.1): the module-export-in-region row compiles —
	// the identity-less ExtensionPayload out mints an ID at the dyn-body
	// record, restoring its event linkage, so the mark window owns the
	// region. Pinned by its frontier spec row (frontier-do-catch.tsv:18), whose
	// ledger entry graduated.
	// The 0-arg-lambda wrap: the do-catch runs inside a FN UNIT, whose finish
	// seats its residual through the RET reconciliation — the program-level
	// window arms on the variadic CALL event but the lowered stack is the
	// call's seated results, not the window's event slots, so verifyMarkWindow
	// pins the mismatch and the program falls back whole.
	//
	// Re-diagnosed 2026-08-02 (NUR037): this row's `f` is a fn-local fn —
	// declared inside the `wrap` lambda's body and then named from the `do`
	// code body — which a compiled unit could not resolve at all, so the
	// admission predicate refused one stage before the mark window ever
	// armed. Same refusal, earlier and truer diagnosis (the third such
	// re-diagnosis of this row).
	//
	// Re-diagnosed 2026-09-16 (the seventy-second increment): the local fn's
	// def is placed as a registry-visible install for the frame now, so the
	// body resolves it and this row falls to the SAME refusal as its hoisted
	// sibling below (NUR120's count contract) — the fifth diagnosis, one
	// stage later. Parity is what this test guards and it holds: the program
	// falls back whole and answers exactly as the interpreter does.
	mwRefusedWithParity(t,
		`def wrap ([] => [def f fn [[x:Any] [Any] [raise bad_input "nope"]]  do [(f 5) 2] error [dot code]]) wrap`,
		"fn wrap: unapplied fn-value in body residual (dynamic apply not compiled in a fn body)")

	// The same shape with `f` hoisted to MODULE scope: NUR037's admission
	// predicate does not fire (a module-scope callback compiles fine), so
	// the program reaches the wrap unit's own finish.
	//
	// Re-diagnosed 2026-09-05 (NUR120): the `=>` lambda now carries its
	// placeholder's COUNT contract (check.LambdaCountContract — the
	// interpreter enforces it at the named call), so the unit's finish sees a
	// 2-value model residual against 1 declared, with a dynamic value that
	// may be an unapplied fn in it, and refuses on the count path one stage
	// before the mark window's verify. Same refusal, earlier and truer
	// diagnosis (the fourth for this row). Parity is what this test guards
	// and it holds.
	mwRefusedWithParity(t,
		`def f fn [[x:Any] [Any] [raise bad_input "nope"]]  def wrap ([] => [do [(f 5) 2] error [dot code]])  wrap`,
		"fn wrap: unapplied fn-value in body residual (dynamic apply not compiled in a fn body)")

	// A CONTRACT-FREE twin (a named fn declaring no returns — no count
	// check at its finish) is the row that keeps verifyMarkWindow's own
	// decline exercised: the program-level window arms on the variadic CALL
	// event but the lowered stack is the unit's seated results, not the
	// window's event slots, so the verify pins the mismatch and the program
	// falls back whole.
	mwRefusedWithParity(t,
		`def f fn [[x:Any] [Any] [raise bad_input "nope"]]  def wrap fn [[] [] [do [(f 5) 2] error [dot code]]]  wrap`,
		"mark-window residual does not match the lowered stack")
}
