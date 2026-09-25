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
		// The MULTI-RUN bodies (each, fold, scan, var): the leak per
		// element, read after the body — the last element's install, or at
		// zero iterations the miss the interpreter raises — inside a loop
		// (NUR200) and inside a fn frame.
		`def t 0 end each [def t (t add 1) t] [1 2 3] drop end t`,
		`def t 0 end each [def t 5 t] [1 2 3] drop end t`,
		`each [def t 5 1] [1 2] drop end t`,
		`[] each [def x 5] x`,
		`[1 2] each [def x 5] x add 1`,
		`fold [ def x 5 ] [10 20] 0  x add 1`,
		`scan [ def x 5 ] [10 20]  x add 1`,
		`[10 20] each [ var [[r] def x r x] ] end x`,
		`def t 0 end for 3 [[1] each [def t 5] drop] end t`,
		`def t 0 end for 3 [[1 2] each [def t (t add i)] drop] end t`,
		`def f fn [[][Integer][def t 0 [1 2] each [def t (t add 1)] drop t]] end f`,
		`def t 0 end def f fn [[][Integer][[1 2] each [def t 5] drop t]] end f end do [t]`,
		`def k 5  def f fn [[] [Integer] [k add 2]]  f  [1] each [def k 9  k]  f`,
		// A lambda callback is a frame of its own: no leak on either lane.
		`def t 0 end each ([x:Integer] => [def t x x]) [1 2] drop end t`,
		// A var pair inside the body nets to nothing: its def half never
		// installs or leaks, and a lambda's own param of the leaked name is
		// its own binding — never a live read of the registry (the frontier
		// row `xs each [var [[a] (a comp)]]`, measured 2026-09-24).
		`def a 7 end [1 2] each [var [[a] a]] end a`,
		`import module [ def use fn [[comp:Function xs:List] [List] [ xs each [ var [[a] (a comp)] ] ]] export "S" {use: use/v} ] end S.use ([a:Integer] => [a mul 2]) [1 2 3]`,
		`def a 7 end def f fn [[][Integer][do [var [[[a 1]] a add 1]]]] end f end a`,
		`def a 7 end for 2 [do [var [[[a 1]] a add 1]]] end a`,
		`def f fn [[][Integer][do [var [[[a 1]] a add 1]]]] end f`,
		// A flex a multi-run body MUTATES, read back after it: the pass holds
		// a re-modelled carrier with no compiled home, so the read seats live
		// on the registry's cell (code-bodies.tsv L190, module-composition L92).
		`def acc (flex []) end for-each [acc swap append drop] [1 2 3] end acc`,
		`def acc (flex []) end each [acc swap append drop 1] [1 2 3] drop end acc`,
		`def acc (flex []) end for-each [acc swap append drop] [1 2 3] end acc size`,
		`def f fn [[][List][def acc (flex []) for-each [acc swap append drop] [1 2 3] acc]] end f`,
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

	// The fn frame whose RESULT is the leaked name after the loop: the
	// post-loop read resolves to the carried slot the do call refreshed (a
	// read with a compiled home never seats live — readHasHome), so the
	// fn's residual seats and both lanes answer 5 (it answered 0 before
	// NUR199, and declined "result is a variadic loop value" between).
	requireEngineParity(t, `def f fn [[][Integer][def t 0 for 3 [do [def t 5]] t]] end f`, true)
}

// TestEachBodyDefInLoopResolves pins NUR200's close: the MULTI-RUN twin
// of NUR199. An `each` body that rebinds a loop-carried name — `def t 0
// for 3 [[1] each [def t 5] drop]  t` — leaks the binding per element on
// the interpreter (5); the compiled each$body unit is a keep-defs unit
// too now (its unstamped defs install through the kept OpBindDynScope
// where no arm-resident twin is adopted — a loop fragment, a fn body),
// the loop refreshes its carried slot after the call, and the read after
// the body seats live. Both lanes answer 5; the lowering carries the
// kept install, never the stale slot.
func TestEachBodyDefInLoopResolves(t *testing.T) {
	src := `def t 0 end for 3 [[1] each [def t 5] drop] end t`
	requireEngineParity(t, src, true)
	dis := compileDisasm(t, src)
	if !strings.Contains(dis, "BIND_DYN_SCOPE") || !strings.Contains(dis, "LOOKUP_DYN_SCOPE") {
		t.Errorf("the each body's def must install through BIND_DYN_SCOPE and the loop re-read the name:\n%s", dis)
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

// TestKeepDefsTokenBodyOverGradualListCompiles pins NUR202's close: a
// keep-defs token body over a GRADUAL list inside a fn — `def f fn
// [[xs:List][List][def t 0 each [def t (t add 1) t] xs]]  f [1 2 3]` —
// answered `[[1 1 1]]` for the interpreter's `[[1 2 3]]`. Two things
// were wrong. The closure unit declined in its probe ("unapplied fn-value
// in body residual"): the untouched Any ELEMENT beneath `t` read as a
// dynamic value that might auto-apply, where an input enters the frame
// resolved and is never stepped on either lane — the gate exempts a
// unit's own untouched inputs now, so the body compiles to its each$body
// closure (the kept install of NUR200) and the Integer-returning twin
// (code-bodies.tsv L189) compiles with it. And the fallback the decline
// took — the body as a CONST the native runs, stamped at run time as a
// detached unit (StampTokenBody) — unwound its defs at its RET where the
// interpreter's InvokeBody leaks every token body's: a token body's stamp
// is a keep-defs unit now, the host hands its kept installs to the
// enclosing context's trail, and the body's own defs are left out of the
// stamp's dependency snapshot (its rebinding of `t` re-stamped the body
// at every element and, past the budget, ran the rest on the
// interpreter). The run-time bodies below reach that path directly.
func TestKeepDefsTokenBodyOverGradualListCompiles(t *testing.T) {
	for _, src := range []string{
		`def f fn [[xs:List][List][def t 0 each [def t (t add 1) t] xs]] end f [1 2 3]`,
		`def f fn [[xs:List][List][def t 0 each [def t (t add 1) t] xs]] end f [1 2 3 4 5 6 7]`,
		`def f fn [[xs:List][Integer][def t 0 each [def t (t add 1) t] xs drop t]] end f [1 2 3]`,
		`def f fn [[xs:List][Integer][def t 0 each [def t (t add 1)] xs drop t]] end f [1 2 3]`,
		`def f fn [[xs:List][List][each [] xs]] end f [1 2 3]`,
		// An input a stack word RE-PRODUCES at its own position is no longer
		// untouched (the gate keeps its shapes for it); these carry no fn
		// value, so they compile either way.
		`def f fn [[xs:List][List][each [dup drop] xs]] end f [1 2 3]`,
		`def f fn [[xs:List][Integer][0 fold [swap swap drop] xs]] end f [1 2 3]`,
		// A body that exists only at run time: the stamped unit keeps its
		// defs per element, as InvokeBody does — the body's own reads see
		// the previous element's install, and a root read after it is live.
		`def f fn [[b:List xs:List][List][def t 0 each b xs]] end f (quote [def t (t add 1) t]) [1 2 3 4 5 6 7]`,
		`def t 0 end def b (quote [def t (t add 1) t]) end each b [1 2 3] drop end t`,
		`def t 0 end def b (quote [def t (t add 1) add]) end fold b [1 2 3] 0 drop end t`,
		// A lambda callback is a frame of its own: no leak on either lane.
		`def f fn [[xs:List][Integer][def t 0 each ([x:Integer] => [def t 9 x]) xs drop t]] end f [1 2]`,
	} {
		requireEngineParity(t, src, true)
	}
	// The leak is torn down with the fn frame on both lanes: the read after
	// the call is the interpreter's undefined_word, and the check pass
	// mirrors it (an erroring program, not a compiler defect — so it is not
	// booked in the compile-defect ledger).
	src := `def f fn [[xs:List][List][def t 0 each [def t (t add 1) t] xs]] end f [1 2 3] end t`
	gotC, compiled, errC, _, errI := runBothEngines(t, src)
	if codeOf(errI) != "undefined_word" || compiled || len(gotC) != 0 || errC == nil || !strings.Contains(errC.Error(), "undefined word: t") {
		t.Errorf("%q: the fn's leak must not outlive its frame on either lane: interp %v; compiled=%v %v / %v", src, errI, compiled, gotC, errC)
	}
	// No interpreter entry: the run-time body stamps once and the whole
	// list runs on the VM (the stamp's snapshot leaves the body's own defs
	// out, so its rebinding of `t` is no staleness).
	for _, src := range []string{
		`def f fn [[xs:List][List][def t 0 each [def t (t add 1) t] xs]] end f [1 2 3 4 5 6 7]`,
		`def f fn [[b:List xs:List][List][def t 0 each b xs]] end f (quote [def t (t add 1) t]) [1 2 3 4 5 6 7]`,
	} {
		if entries, _ := unattributedEntries(t, src); len(entries) != 0 {
			t.Errorf("%q: the keep-defs body entered the interpreter: %v", src, entries)
		}
	}
}

// TestDynamicKeepDefsBodyLeakInFnPending pins NUR203 as it stands: a
// keep-defs word over a DYNAMIC body (a List param, a def-bound quoted
// list) inside a fn — `def f fn [[b:List xs:List][Integer][def t 0 each b
// xs drop t]]  f (quote [def t (t add 1) t]) [1 2 3]` — leaks the body's
// def per element on the interpreter (3), and the compiled lane's later
// read of `t` in the fn answers the pre-call value (0): the run-time
// stamped body installs the leak in the registry (NUR202's close), but the
// compile pass cannot know which names a body it never sees will rebind, so
// the fn's later read keeps its compile-time home instead of seating live
// (NoteKeepDefsLeak names only a compiled unit's defs). The same shape at
// the root agrees (a root read is live). Closing it must update this pin.
func TestDynamicKeepDefsBodyLeakInFnPending(t *testing.T) {
	for _, src := range []string{
		`def f fn [[b:List xs:List][Integer][def t 0 each b xs drop t]] end f (quote [def t (t add 1) t]) [1 2 3]`,
		`def f fn [[b:List xs:List][Integer][def t 0 fold b xs 0 drop t]] end f (quote [def t (t add 1) add]) [1 2 3]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if errI != nil || fmt.Sprint(gotI) != "[3]" {
			t.Errorf("%q: the interpreter leaks the dynamic body's def into the fn's frame: %v / %v", src, gotI, errI)
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != "[0]" {
			t.Errorf("%q: NUR203's compiled value %v / %v (compiled=%v), pinned as [0] — closing the divergence must update this pin", src, gotC, errC, compiled)
		}
	}
}
