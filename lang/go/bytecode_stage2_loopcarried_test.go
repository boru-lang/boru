package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Stage-2 loop-carried def rebind pins (voxgig zero-refusals plan): a
// pre-loop `def` REBOUND inside a for body (decision.boru eval-table-first's
// `def found true` inside an arm, read as `found not` the NEXT iteration)
// used to refuse "operand of unknown provenance … at not" — the rebind's
// per-round JOIN carrier had no operand home across iterations. The fix is
// loop-persistent frame slots:
//
//  1. NoteLoopCarried (emit.go, driven from AnalyseLoopBody's per-round join)
//     allocates a unit frame slot per rebound pre-loop name, aliases each
//     round's joined-binding ID to the slot (reads resolve to PUSH_LOCAL),
//     and queues the pre-loop value as the slot's init.
//  2. RecordDefRebind (from the def handler) records an evStore at the
//     rebind SITE — inside an if arm the store runs exactly when the arm
//     runs, and break/continue skip it exactly as the interpreter skips
//     the def.
//  3. lowerLoop seeds each carried slot right after FOR_SETUP, so a
//     zero-iteration loop leaves the cell at its pre-loop value.
//
// The differential corpus never sees these off-corpus shapes, so pin them
// (the memory-noted RunCompiledStrict discipline): compile+parity for the
// newly-compiling shapes, sound-fallback for the shapes that stay refused.

// loopCarriedCompilesClean asserts the shape compiles WITHOUT an interpreter
// island (no FALLBACK in the disassembly) and matches the interpreter.
func loopCarriedCompilesClean(t *testing.T, src string) {
	t.Helper()
	stage1aCompiles(t, src)
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || reason != "" || prog == nil {
		t.Fatalf("loop-carried shape did not compile: reason=%q err=%v", reason, cerr)
	}
	if dis := prog.Disassemble(); strings.Contains(dis, "FALLBACK") {
		t.Errorf("expected a full lowering, got an island:\n%s", dis)
	}
}

// The decision.boru eval-table-first shape in miniature: a conditional rebind
// of TWO pre-loop defs inside a nested arm, with `found` read (`found not`)
// on the NEXT iteration and `result` read after the loop. First-match
// semantics: later matches must NOT overwrite.
func TestLoopCarriedRebindReadNextIteration(t *testing.T) {
	loopCarriedCompilesClean(t, `def first-big fn [[xs:List] [Integer] [def found false def result 0 for (xs size) [def idx i def x (xs idx get) if (found not) [if (x 10 gt) [def result x def found true] []] []] end result]]
[(first-big [3 12 40]) (first-big [40 3 12]) (first-big [1 2 3]) (first-big [])]`)
}

// An UNCONDITIONAL rebind read within the loop on the next iteration
// (`acc add acc` doubles per pass) and after the loop.
func TestLoopCarriedRebindUnconditional(t *testing.T) {
	loopCarriedCompilesClean(t, `def doubler fn [[n:Integer] [Integer] [def acc 1 for n [def acc (acc add acc)] end acc]]
[(doubler 3) (doubler 0)]`)
}

// A conditional rebind in ONE arm only, read ONLY after the loop (the
// last-match variant — every matching iteration overwrites).
func TestLoopCarriedRebindConditionalOneArm(t *testing.T) {
	loopCarriedCompilesClean(t, `def last-big fn [[xs:List] [Integer] [def best 0 for (xs size) [def idx i def x (xs idx get) if (x 10 gt) [def best x] []] end best]]
[(last-big [3 12 40 7]) (last-big [1 2]) (last-big [])]`)
}

// A rebind in a NESTED loop: the inner loop reuses the OUTER carried slot
// (a fresh inner slot with its own init would reset the accumulation every
// outer iteration — 3*3 must total 9, not 3).
func TestLoopCarriedRebindNestedLoop(t *testing.T) {
	loopCarriedCompilesClean(t, `def grid fn [[n:Integer] [Integer] [def total 0 for n [def oi i for n [def total (total add 1)]] end total]]
[(grid 3) (grid 1) (grid 0)]`)
}

// A MODULE-scope loop rebind: the carried slot lives in frame 0 and the
// post-loop read is the program residual (the Finalize local-operand branch).
func TestLoopCarriedRebindModuleScope(t *testing.T) {
	loopCarriedCompilesClean(t, `def acc 0
for 3 [def acc (acc add 1)] end acc`)
}

// A PARAM rebound in the loop: the carried cell is a fresh slot seeded from
// the param local; the param slot itself must stay untouched.
func TestLoopCarriedParamRebind(t *testing.T) {
	loopCarriedCompilesClean(t, `def bump fn [[a:Integer n:Integer] [Integer] [for n [def a (a add 2)] end a]]
[(bump 1 4) (bump 7 0)]`)
}

// A COMPUTED pre-loop init (`def result (do {…})` — a map event, the
// decision.boru shape): the init operand rides the promoted value-def local.
func TestLoopCarriedComputedInit(t *testing.T) {
	loopCarriedCompilesClean(t, `def tally fn [[xs:List] [Map] [def result (do {ok: false}) def found false for (xs size) [def idx i if (found not) [if ((xs idx get) 10 gt) [def result (do {ok: true}) def found true] []] []] end result]]
[(tally [2 15 3]) (tally [1 2])]`)
}

// Zero iterations leave the pre-loop value (the "loop may run zero times"
// join): the carried slot's init must land BEFORE the first FOR_NEXT.
func TestLoopCarriedZeroIterationsKeepInit(t *testing.T) {
	loopCarriedCompilesClean(t, `def keep fn [[n:Integer] [Integer] [def acc 42 for n [def acc 0] end acc]]
(keep 0)`)
}

// SOUND-fallback pins: shapes adjacent to the feature that stay refused (or
// compile) must match the interpreter either way — never diverge.

// A value-producing loop body that ALSO rebinds: per-iteration residual plus
// carried stores.
func TestLoopCarriedRebindValueLoopSound(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	stage1aSound(t, `def vals fn [[n:Integer] [List] [def acc 0 [(for n [def acc (acc add 1) acc])]]]
(vals 3)`)
}

// A read of the rebound name IN THE SAME ITERATION after a computed rebind
// (the value is consumed by the carried store; the read must still see it).
func TestLoopCarriedSameIterationReadAfterRebindSound(t *testing.T) {
	stage1aSound(t, `def echo fn [[xs:List] [Integer] [def cur 0 for (xs size) [def idx i def cur (xs idx get) def _u (cur add 0)] end cur]]
(echo [4 9 2])`)
}

// NEGATIVE: `undef` of a loop-carried name inside the body exposes the
// previous binding while the cell holds the rebound value — must refuse
// (fall back), never read the stale slot.
func TestLoopCarriedUndefStaysSound(t *testing.T) {
	stage1aSound(t, `def flip fn [[n:Integer] [Integer] [def acc 5 for n [def acc (acc add 1) undef acc] end acc]]
(flip 2)`)
}

// NEGATIVE: a rebind to a FUNCTION value inside a loop body REFUSES (fn
// values do not ride carried slots in Stage 2). The loop-body `def h`
// overlap-removes the enclosing `h` in place — the def depth is unchanged,
// so the loop rollback cannot restore it — and compiled resolution statically
// bakes the loop's `add 2` overload. The interpreter keeps the pre-loop
// `add 1` when the loop runs ZERO times, so `(pickfn 0)` silently miscompiled
// to 12 (should be 11) before this refusal landed; `(pickfn 2)` coincidentally
// agreed at 12 because the loop runs. Refuse — compiled == interpreter at every
// n (slow, not wrong). See the conditional-fn-shadow divergence fix.
func TestLoopCarriedFnValueRebindStaysSound(t *testing.T) {
	base := `def pickfn fn [[n:Integer] [Integer] [def h ([x:Integer] => [x add 1]) for n [def h ([x:Integer] => [x add 2])] end (h 10)]]`
	// The DEFINITION carries the unsound loop rebind, so every call refuses.
	mustRefuseWithParity(t, base+"\n(pickfn 0)", "redefined inside a conditional body")
	mustRefuseWithParity(t, base+"\n(pickfn 2)", "redefined inside a conditional body")
	// The interpreter is the source of truth the refusal falls back to: the
	// zero-iteration case (11) is exactly what the compiled bake got wrong.
	for _, tc := range []struct {
		n    int
		want string
	}{{0, "[11]"}, {1, "[12]"}, {2, "[12]"}} {
		a, _ := New()
		got, err := a.RunInterp(fmt.Sprintf("%s\n(pickfn %d)", base, tc.n))
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("(pickfn %d): interp=%v err=%v, want %s", tc.n, got, err, tc.want)
		}
	}
}

// NEGATIVE: break skips the rebinds after it exactly as the interpreter
// skips the defs (stores sit at the def SITE, not an iteration epilogue).
func TestLoopCarriedRebindAfterBreakSound(t *testing.T) {
	stage1aSound(t, `def til fn [[xs:List] [Integer] [def seen 0 for (xs size) [def idx i if ((xs idx get) 0 lt) [break] [] def seen (seen add 1)] end seen]]
(til [1 2 -1 4])`)
}
