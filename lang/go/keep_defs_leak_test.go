package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestDoBodyDefLeaksToTheEnclosingScope pins the KEEP-DEFS closure unit
// (NUR199): a `do` body runs ONCE in the caller's frame on the interpreter
// and its defs LEAK to the enclosing scope, so the compiled body unit
// installs every value def through a kept OpBindDynScope (the VM leaves it
// standing past the unit's own RET — CompiledFn.KeepsDefs), the enclosing
// loop's carried slot is refreshed from the registry right after the call,
// and a later read of the leaked name seats live at its token.
//
// The first group is the divergence itself: `def t 0  for 3 [do [def t 5]]
// t` answered 0 (the loop-carried slot kept the pre-loop value — the do's
// def was frame-local to its closure and never reached the slot) for the
// interpreter's 5. The rest are the shapes around it: the computed def
// (code-bodies.tsv rows 180 and 186, compile failures until now), the root
// leak read back as a residual and as an operand, the undef interplay (the
// adopted twin is marked written back, so the runtime install is the ONE
// level `undef` pops), a def inside a branch arm of the body, a fn def
// (whose adopted twin still replays it), and the fn frame, where the leak
// is torn down with the frame as the interpreter's def-cleanup tears it
// down — `do [t]` after the call reads the root binding on both lanes.
func TestDoBodyDefLeaksToTheEnclosingScope(t *testing.T) {
	for _, src := range []string{
		// The divergence: a do body rebinding a loop-carried name.
		`def t 0 end for 3 [do [def t 5]] end t`,
		`def t 0 end for 3 [do [def t 5]] t end`,
		`def t 0 end for 3 [do [def t 5]] end do [t]`,
		`def t 0 end for 3 [do [def t (t add 1)]] end t`,
		`def t 0 end for 3 [do [def t (i add 1)]] end t`,
		`def i 0 end while [i lt 3] [do [def i (i add 1)]] end i`,
		`for 3 [do [def u (i mul 2)]] end u`,
		// The root leak, read back.
		`def t 0 end do [def t (t add 1)] end t`,
		`def t 0 end do [def t (t add 1)] end t add 1`,
		`def t 0 end do [def t 5] end t`,
		`do [def t 1] end do [def t 2] end t`,
		`def t 0 end do [def t (t add 1)] end do [def t (t add 1)] end t`,
		`do [def _ 5 def t (1 add 2)] end t`,
		`def t 0 end do [if true [def t 9] []] end t`,
		`do [def f fn [[][Integer][7]] f]`,
		// A value the install cannot re-push — a `word` splice marker —
		// keeps the pre-keep lowering (nothing) and the adopted twin's
		// replay (emitDynBind.keepSkip).
		`do [def dbl word [dup add]] end 5 dbl`,
		`do [def _ 5] end 1`,
		// One install per def: undef pops exactly the levels the
		// interpreter stacked.
		`def t 0 end do [def t 1 def t 2] end t end undef t end t`,
		`def t 0 end do [def t 5] end undef t end t`,
		// A loop inside the body, and the body inside a loop reading the
		// leaked name on the same iteration.
		`do [def s 0 for 3 [def s (s add i)] s]`,
		`def s 0 end do [for 3 [def s (s add i)]] end s`,
		`def t 0 end for 2 [do [def t (t add 1)] t] end`,
		`def xs (flex []) end for 3 [do [def xs (xs append i)]] end xs`,
		// Leak then raise: the def the body made before the raise stays
		// bound, the error trapped by `do` (the interpreter's raise skips
		// the frame's cleanup; a CALLEE's own def under the same raise is
		// NUR201, below).
		`def t 0 end do [def t 5 raise 'x'] end t`,
		`def t 0 end do [def t 5 raise 'x'] error [drop 1] end t`,
		// The fn frame: the leak lives as long as the frame.
		`def f fn [[x:Integer][Integer][def t 0 do [def t (x add 1)] t]] end f 4`,
		`def t 0 end def f fn [[][Integer][do [def t 5] t]] end f end do [t]`,
	} {
		requireEngineParity(t, src, true)
	}

	// The lowering: the body unit's def is a BIND_DYN_SCOPE, the enclosing
	// loop refreshes its carried slot from the registry after the call
	// (LOOKUP_DYN_SCOPE then STORE_LOCAL), and the post-loop read is a live
	// lookup — never the stale slot.
	dis := compileDisasm(t, `def t 0 end for 3 [do [def t (t add 1)]] end t`)
	if !strings.Contains(dis, "BIND_DYN_SCOPE") || !strings.Contains(dis, "LOOKUP_DYN_SCOPE") {
		t.Errorf("the do body's def must install through BIND_DYN_SCOPE and the loop re-read the name:\n%s", dis)
	}
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("the keep-defs body must not island:\n%s", dis)
	}
	// And no unattributed interpreter entry: the whole shape runs on the VM.
	if entries, _ := unattributedEntries(t, `def t 0 end for 3 [do [def t (t add 1)]] end t`); len(entries) != 0 {
		t.Errorf("the keep-defs loop shape entered the interpreter: %v", entries)
	}

	// The remaining edge, counted: inside a fn whose RESULT is the leaked
	// name after the loop, the loop's count-agnostic body (a `do` returns
	// its whole residual) leaves the fn's residual a variadic loop value
	// the unit's RET cannot seat, and the program declines — where it
	// answered 0 for the interpreter's 5 before this change. Parity by the
	// counted defect (compile_defect_test.go's ledger).
	requireEngineParity(t, `def f fn [[][Integer][def t 0 for 3 [do [def t 5]] t]] end f`, false)
}

// TestEachBodyDefInLoopPending pins NUR200 as it stands: the MULTI-RUN
// twin of NUR199. An `each` body that rebinds a loop-carried name —
// `def t 0  for 3 [[1] each [def t 5] drop]  t` — leaks the binding per
// element on the interpreter (5), while the compiled each$body unit keeps
// the def frame-local: no arm-resident twin is adopted inside a loop
// fragment (AdoptResidentTwins' root fence) and the carried slot keeps the
// pre-loop value (0). The keep-defs mechanism of NUR199 is the `do` body's
// (once-run, one install); a per-element install rides the resident-twin
// bridge, which is fenced to the root stream today. Closing it must update
// this pin.
func TestEachBodyDefInLoopPending(t *testing.T) {
	src := `def t 0 end for 3 [[1] each [def t 5] drop] end t`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[5]" {
		t.Errorf("%q: the interpreter leaks the each body's def: %v / %v", src, gotI, errI)
	}
	if errC != nil || !compiled || fmt.Sprint(gotC) != "[0]" {
		t.Errorf("%q: NUR200's compiled value %v / %v (compiled=%v), pinned as [0] — closing the divergence must update this pin", src, gotC, errC, compiled)
	}
}

// TestCalleeDefSurvivesTrappedRaisePending pins NUR201 as it stands: a fn
// body's def followed by a raise the caller traps — `def g fn
// [[][Integer][def t 9 raise 'x']]  do [g]  t` — leaves the def BOUND on
// the interpreter (a raise skips the frame's def-cleanup tail, so the
// callee's `t` leaks: `[error(x) 9]`), where the compiled lane's frame
// installs nothing the trap could keep and the read answers the root
// binding (`[error(x) 0]`). Present on main before the keep-defs body
// (3768c46); the same shape with the def in the `do` body itself is
// NUR199's leak-then-raise row above, which agrees. Closing it must update
// this pin.
func TestCalleeDefSurvivesTrappedRaisePending(t *testing.T) {
	src := `def t 0 end def g fn [[][Integer][def t 9 raise 'x']] end do [g] end t`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[error(x) 9]" {
		t.Errorf("%q: the interpreter keeps the callee's def past the trapped raise: %v / %v", src, gotI, errI)
	}
	if errC != nil || !compiled || fmt.Sprint(gotC) != "[error(x) 0]" {
		t.Errorf("%q: NUR201's compiled value %v / %v (compiled=%v), pinned as [error(x) 0] — closing the divergence must update this pin", src, gotC, errC, compiled)
	}
}
