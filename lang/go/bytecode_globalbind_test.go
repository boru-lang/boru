package lang

import (
	"fmt"
	"strings"
	"testing"
)

// OpBindGlobal — the cross-request persistence twin of a top-level computed
// `def` (design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore, the 2026-07-15
// flip composite's root cause). A compiled request's check pass installs the
// def binding as a CARRIER and keep-on-compile persists it; without the
// write-back, the NEXT request (either engine) resolved a type literal where
// the interpreter had bound the runtime value — `def h (Model.new …)` then
// `Model.stop h` raised model_bad_handle, and `def n (add 1 2)` then
// `n add 1` silently computed the WRONG value. Each pin below runs request 1
// compiled, then reads the binding in request 2 on the SAME instance.

// runCompiledRequest runs src as one compiled-by-default request and fails
// the test on refusal or error — the pins below need the COMPILED path (a
// fallback would bind via the interpreter and prove nothing).
func runCompiledRequest(t *testing.T, a *Boru, src string) {
	t.Helper()
	_, ran, reason, err := a.RunAutoValues(src)
	if err != nil || !ran {
		t.Fatalf("%q: compiled request: ran=%v reason=%q err=%v", src, ran, reason, err)
	}
}

// readBack reads src in a follow-up interpreter request and returns the
// rendering of the result stack.
func readBack(t *testing.T, a *Boru, src string) string {
	t.Helper()
	out, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("%q: next-request read: %v", src, err)
	}
	return fmt.Sprint(out)
}

func TestGlobalBindPersistsComputedDefs(t *testing.T) {
	// A computed scalar: the check pass binds an Integer carrier; the
	// write-back must land the folded 3 so the next request computes 4 —
	// pre-fix this silently returned the WRONG value.
	a := mustNew(t)
	runCompiledRequest(t, a, `def n (add 1 2)`)
	if got := readBack(t, a, `n add 1`); got != "[4]" {
		t.Errorf("computed scalar def: next request = %v, want [4]", got)
	}

	// A computed list (the ListPayload class).
	b := mustNew(t)
	runCompiledRequest(t, b, `def xs (range 1 4)`)
	if got := readBack(t, b, `xs get 0`); got != "[1]" {
		t.Errorf("computed list def: next request = %v, want [1]", got)
	}

	// A runtime handle (the FlexList Extension class — the model/service
	// shape without needing a module import): pre-fix the next request
	// raised "cannot access property on type literal".
	c := mustNew(t)
	runCompiledRequest(t, c, `def fx (flex [1 2 3])`)
	if got := readBack(t, c, `fx get 0`); got != "[1]" {
		t.Errorf("flex handle def: next request = %v, want [1]", got)
	}

	// The branch-merge producer (the corpus edge-quote-4 row): the merge is
	// peeked in place by the fast path — no promotion machinery needed.
	d := mustNew(t)
	runCompiledRequest(t, d, `def r (if false [0] [quote [add 1 2]]) r size`)
	if got := readBack(t, d, `r size`); got != "[3]" {
		t.Errorf("branch-merge def: next request = %v, want [3]", got)
	}

	// A DEAD def (never read in-program) of a USER-fn call result: the
	// producer-site dead-drop is suppressed and the bind consumes the value
	// (lowerUserCall's bindConsumes arm); the binding still persists.
	f := mustNew(t)
	runCompiledRequest(t, f, `def uf fn [[] [Integer] [5]] def q (uf) 99`)
	if got := readBack(t, f, `q`); got != "[5]" {
		t.Errorf("dead user-call def: next request = %v, want [5]", got)
	}

	// A DEAD branch-merge def: the dead-arm drop is skipped and the bind
	// consumes the merge (the lowerBranch bindConsumes arm).
	g := mustNew(t)
	runCompiledRequest(t, g, `def qb (if true [1] [2]) 99`)
	if got := readBack(t, g, `qb`); got != "[1]" {
		t.Errorf("dead branch def: next request = %v, want [1]", got)
	}

	// A DEAD def of a USER-POLY call result (a multi-overload fn over a
	// dynamic operand — CALL_USER_POLY's bindConsumes arm).
	h := mustNew(t)
	runCompiledRequest(t, h,
		`def pf fn [[a:Integer] [Integer] [1] [a:String] [Integer] [2]] def m (flex [5]) def d (m get 0) def q (pf d) 99`)
	if got := readBack(t, h, `q`); got != "[1]" {
		t.Errorf("dead user-poly def: next request = %v, want [1]", got)
	}

	// The next-request read must also work COMPILED (the REPL's steady
	// state: every line compiled).
	e := mustNew(t)
	runCompiledRequest(t, e, `def m (do [add 20 22])`)
	vals, ran, reason, err := e.RunAutoValues(`m`)
	if err != nil || !ran {
		t.Fatalf("compiled next-request read: ran=%v reason=%q err=%v", ran, reason, err)
	}
	if fmt.Sprint(convertResults(vals)) != "[42]" {
		t.Errorf("compiled next-request read = %v, want [42]", vals)
	}
}

func TestGlobalBindShadowAndUndefDepths(t *testing.T) {
	// Same-name shadowing records one write-back per install DEPTH: both
	// slots must hold their own runtime value, so undef reveals the first.
	a := mustNew(t)
	runCompiledRequest(t, a, `def x (add 1 2) def x (add 2 3)`)
	if got := readBack(t, a, `x`); got != "[5]" {
		t.Errorf("shadowed def: next request = %v, want [5]", got)
	}
	if _, err := a.RunInterp(`undef x`); err != nil {
		t.Fatalf("undef: %v", err)
	}
	if got := readBack(t, a, `x`); got != "[3]" {
		t.Errorf("after undef: next request = %v, want [3] (the first install's runtime value)", got)
	}

	// A check-time undef pops the slot before the program runs: the
	// write-back finds no home and must SKIP (the interpreter would have
	// discarded the binding too) — never underflow, never resurrect.
	b := mustNew(t)
	runCompiledRequest(t, b, `def y (add 1 2) undef y 9`)
	if _, err := b.RunInterp(`y`); err == nil || !strings.Contains(err.Error(), "undefined_word") {
		t.Errorf("undef'd computed def must stay unbound next request, got err=%v", err)
	}
}

func TestGlobalBindEnvelope(t *testing.T) {
	// A bare type-node binding is self-representing — the kept check-pass
	// binding IS the runtime value, so no write-back op is emitted and the
	// program stays compiled (the error.tsv `def x None` rows).
	if dis := compileDisasm(t, `def x None 5`); strings.Contains(dis, "BIND_GLOBAL") {
		t.Errorf("bare type-node def must not emit BIND_GLOBAL:\n%s", dis)
	}
	// A concrete literal binding is already faithful — no op.
	if dis := compileDisasm(t, `def x 41 x add 1`); strings.Contains(dis, "BIND_GLOBAL") {
		t.Errorf("concrete literal def must not emit BIND_GLOBAL:\n%s", dis)
	}
	// A computed def DOES emit the twin, and the disassembly names the slot.
	dis := compileDisasm(t, `def n (add 1 2)`)
	if !strings.Contains(dis, "BIND_GLOBAL") || !strings.Contains(dis, "global bind n @depth 1") {
		t.Errorf("computed def must emit BIND_GLOBAL with its slot:\n%s", dis)
	}

	// A fn-BODY def is frame-scoped (DefCleanup tears it down): no
	// write-back, and the name must not leak into the next request.
	a := mustNew(t)
	runCompiledRequest(t, a, `def f fn [[] [Integer] [def zz (add 2 3) zz]] f`)
	if _, err := a.RunInterp(`zz`); err == nil || !strings.Contains(err.Error(), "undefined_word") {
		t.Errorf("fn-body def must not persist cross-request, got err=%v", err)
	}

	// GRADUATED (REFUSAL-CLOSURE S5, 2026-07-17): a def of a STATICALLY-
	// COUNTED variadic loop collect binds the region's first value via the
	// splice-at-depth OpBindGlobal and spills the rest — compiled parity
	// with the interpreter. A DYNAMIC count keeps the refusal (the split
	// needs the static region size) — the zzRefusingRow fixture.
	b := mustNew(t)
	gotC, compiledB, err := b.RunCompiled(`def xs (for 3 [1]) xs`)
	if noteCompileDefect(t, `def xs (for 3 [1]) xs`, gotC, err) {
		return
	}
	if !compiledB || err != nil {
		t.Errorf("S5 static loop def: compiled=%v err=%v, want a compiled run", compiledB, err)
	}
	c := mustNew(t)
	out, ierr := c.RunInterp(`def xs (for 3 [1]) xs`)
	if ierr != nil {
		t.Fatalf("variadic loop def must interpret cleanly: %v", ierr)
	}
	if fmt.Sprint(out) != "[1 1 1]" || fmt.Sprint(gotC) != fmt.Sprint(out) {
		t.Errorf("S5 parity: compiled=%v interp=%v, want [1 1 1] both", gotC, out)
	}
}

// The TWIN-CARRIER class (the COLLECT oracle's first corpus walk, the
// sixty-second increment; fixed in the sixty-third): a root def of a
// COMPUTED COMPOUND whose check-pass binding is the analysis's MODEL of the
// value — `[Integer]` for `[add 1 2]`, a module prototype with empty fields
// for `Log.counter "x"` — read as concrete by IsConcrete, so no write-back
// was emitted and the twin replayed the model for the rest of the run. The
// bakes hid it (every read of such a def in the corpus resolved to the
// lowering's own value); a LIVE read — the next request's — sees the
// binding itself. The write-back is now decided by provenance
// (rootBindWritesBack): a computed value's binding is exact only for a
// scalar fold.
func TestGlobalBindTwinCarrierClass(t *testing.T) {
	// The list: the lowering pushes the folded [3]; the kept binding was
	// [Integer], and the next request's `get 0` returned the type node.
	dis := compileDisasm(t, `def b [add 1 2] size b`)
	if !strings.Contains(dis, "global bind b @depth 1") {
		t.Errorf("a computed list def must write its runtime value back:\n%s", dis)
	}
	a := mustNew(t)
	runCompiledRequest(t, a, `def b [add 1 2] size b`)
	if got := readBack(t, a, `b get 0 add 1`); got != "[4]" {
		t.Errorf("computed list def: next request = %v, want [4] (the folded element, not its carrier)", got)
	}

	// The module handle: Log.span's check-pass result is the span the
	// ANALYSIS started; the run starts its own, which is the active one. The
	// kept binding was the analysis's, so ending it from the next request
	// was a span-mismatch — the live read the corpus never made (its row
	// ends the span in the same request, through the lowering's own value).
	b := mustNew(t)
	runCompiledRequest(t, b, `import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def s (Log.span "m")`)
	if got := readBack(t, b, `Log.end-span s ; Log.traces size`); got != "[1]" {
		t.Errorf("computed module handle: next request = %v, want [1] (the run's own span, not the analysis's)", got)
	}

	// A computed SCALAR fold stays exact with no write-back needed beyond
	// what the carrier rule already emits: `def n (add 1 2)` is pinned
	// above (TestGlobalBindEnvelope); a literal compound is the value itself.
	if dis := compileDisasm(t, `def xs [1 2] size xs`); strings.Contains(dis, "BIND_GLOBAL") {
		t.Errorf("a literal list def is already faithful — no write-back:\n%s", dis)
	}

	// THE PAIRING, across requests (review of #459): the write-back INSTALLS
	// the runtime value, so the def's twin must not replay the model beside
	// it — or the name holds two levels and an `undef` uncovers the model.
	// Measured before the twin carried the pairing: request 3 answered
	// `[[Integer]]` where the interpreter raises undefined_word.
	c := mustNew(t)
	runCompiledRequest(t, c, `def b [add 1 2] size b`)
	if _, err := c.RunInterp(`undef b`); err != nil {
		t.Fatalf("undef: %v", err)
	}
	if _, err := c.RunInterp(`b`); err == nil || !strings.Contains(err.Error(), "undefined_word") {
		t.Errorf("one install, one undef: b must be unbound, got err=%v", err)
	}

	// A MICRON is inert (immutable) but has fields, and a computed one
	// whose field the check pass could not fold keeps a carrier there
	// (review of #459): `IsInertConst` reads the whole instance as inert, so
	// the scalar exemption must be the payload kinds with no interior.
	// Measured before: request 3 raised signature_error over the carrier
	// field where the interpreter adds.
	d := mustNew(t)
	runCompiledRequest(t, d, `import "boru:time-util" def Stampton refine Micron {n:Integer} def x (make Stampton {n:(TimeUtil.now TimeUtil.to-unix-ms)})`)
	if out, err := d.RunInterp(`(x.n) add 1`); err != nil || len(out) != 1 {
		t.Errorf("computed micron def: next request reads the run's field, got out=%v err=%v", out, err)
	}
}
