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
		// The divergence: a do body assigning a loop-carried var (a do body
		// inside a loop body binds in the BODY's block — design §2.1 — so a
		// def there is the iteration's own; block-scopes.tsv pins that).
		`var t 0 end for 3 [do [var t 5]] end t`,
		`var t 0 end for 3 [do [var t 5]] t end`,
		`var t 0 end for 3 [do [var t 5]] end do [t]`,
		`var t 0 end for 3 [do [var t (t add 1)]] end t`,
		`var t 0 end for 3 [do [var t (i add 1)]] end t`,
		`var i 0 end while [i lt 3] [do [var i (i add 1)]] end i`,
		`var u 0 end for 3 [do [var u (i mul 2)]] end u`,
		// The root leak, read back.
		`def t 0 end do [def t (t add 1)] end t`,
		`def t 0 end do [def t (t add 1)] end t add 1`,
		`def t 0 end do [def t 5] end t`,
		`do [def t 1] end do [def t 2] end t`,
		`def t 0 end do [def t (t add 1)] end do [def t (t add 1)] end t`,
		`do [def _ 5 def t (1 add 2)] end t`,
		`var t 0 end do [if true [var t 9] []] end t`,
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
		`do [var s 0 for 3 [var s (s add i)] s]`,
		`var s 0 end do [for 3 [var s (s add i)]] end s`,
		`var t 0 end for 2 [do [var t (t add 1)] t] end`,
		`var xs (flex []) end for 3 [do [var xs (xs append i)]] end xs`,
		// Leak then raise: the def the body made before the raise stays
		// bound, the error trapped by `do` — a `do` body is not a frame,
		// so its defs are the caller's own (a CALLEE's def under the same
		// raise is torn down with its frame: NUR201, below).
		`def t 0 end do [def t 5 raise 'x'] end t`,
		`def t 0 end do [def t 5 raise 'x'] error [drop 1] end t`,
		// The MULTI-RUN bodies (each, fold, scan, var): the leak per
		// element, read after the body — the last element's install, or at
		// zero iterations the miss the interpreter raises — inside a loop
		// (NUR200) and inside a fn frame.
		`var t 0 end each [var t (t add 1) t] [1 2 3] drop end t`,
		`var t 0 end each [var t 5 t] [1 2 3] drop end t`,
		`var t 0 end each [var t 5 1] [1 2] drop end t`,
		`[] each [def x 5] x`,
		`var x 0 end [1 2] each [var x 5] x add 1`,
		`var x 0 end fold [ var x 5 ] [10 20] 0  x add 1`,
		`var x 0 end scan [ var x 5 ] [10 20]  x add 1`,
		`[10 20] each [ def x end x ]`,
		`var t 0 end for 3 [[1] each [var t 5] drop] end t`,
		`var t 0 end for 3 [[1 2] each [var t (t add i)] drop] end t`,
		`var k 5  def f fn [[] [Integer] [k add 2]]  f  [1] each [var k 9  k]  f`,
		// A lambda callback is a frame of its own: no leak on either lane.
		`def t 0 end each ([x:Integer] => [def t x x]) [1 2] drop end t`,
		// A var pair inside the body nets to nothing: its def half never
		// installs or leaks, and a lambda's own param of the leaked name is
		// its own binding — never a live read of the registry (the frontier
		// row `xs each ([a] => [(a comp)])`, measured 2026-09-24).
		`def a 7 end [1 2] each ([a] => [a]) end a`,
		`import module [ def use fn [[comp:Function xs:List] [List] [ xs each [ ([a] => [(a comp)]) apply ] ]] export "S" {use: use/v} ] end S.use ([a:Integer] => [a mul 2]) [1 2 3]`,
		`def a 7 end def f fn [[][Integer][do [(([a] => [a add 1]) 1)]]] end f end a`,
		`def a 7 end for 2 [do [(([a] => [a add 1]) 1)]] end a`,
		`def f fn [[][Integer][do [(([a] => [a add 1]) 1)]]] end f`,
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
	// A FRAME's var assigned from a callback body: the body unit's
	// assignment does not reach the frame's cell yet, so the compiled lane
	// declines the shape (core assignVarUnitGate) until the compiler's
	// block scopes land; the interpreter's block assigns the frame's cell.
	for _, src := range []string{
		`def f fn [[][Integer][var t 0 [1 2] each [var t (t add 1)] drop t]] end f`,
		`def t 0 end def f fn [[][Integer][var s 0 [1 2] each [var s 5] drop s]] end f end do [t]`,
	} {
		requireEngineParity(t, src, false)
	}

	// The lowering: the do body's assignment runs on the VM, no island.
	dis := compileDisasm(t, `var t 0 end for 3 [do [var t (t add 1)]] end t`)
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("the keep-defs body must not island:\n%s", dis)
	}
	// And no unattributed interpreter entry: the whole shape runs on the VM.
	if entries, _ := unattributedEntries(t, `var t 0 end for 3 [do [var t (t add 1)]] end t`); len(entries) != 0 {
		t.Errorf("the keep-defs loop shape entered the interpreter: %v", entries)
	}

	// The fn frame whose RESULT is the var after the loop: both lanes
	// answer 5 (it answered 0 before NUR199, and declined "result is a
	// variadic loop value" between).
	requireEngineParity(t, `def f fn [[][Integer][var t 0 for 3 [do [var t 5]] t]] end f`, true)
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
	// The each body ASSIGNS the loop-carried var (a def there is the
	// element's block local since phase 2; block-scopes.tsv pins it).
	src := `var t 0 end for 3 [[1] each [var t 5] drop] end t`
	requireEngineParity(t, src, true)
	if dis := compileDisasm(t, src); strings.Contains(dis, "FALLBACK") {
		t.Errorf("the each body's assignment must not island:\n%s", dis)
	}
}

// TestCalleeDefTornDownOnTrappedRaise pins NUR201's close: a fn body's def
// followed by a raise the caller traps — `def g fn [[][Integer][def t 9
// raise 'x']]  do [g]  t` — left the def BOUND on the interpreter (the
// raise abandoned the tape with the frame's def-cleanup tail unstepped, so
// the callee's `t` leaked: `[error(x) 9]`), where the compiled lane's
// frame unwinds with the error and the read answers the root binding
// (`[error(x) 0]`). The interpreter's fault return now tears down every
// frame the error leaves open on the tape (core Engine.faultReturn), so
// both lanes answer `[error(x) 0]`: the callee's locals, params and args
// list are gone when the trap resumes. The rows are the shape and its
// neighbours — a param of the leaked name, nested callee frames, the raise
// inside a paren group, a callback body, a residual container and a native
// error in place of the raise, a lambda value and an applied fn value, the
// trap inside a fn frame, a loop, and an `error` handler. A `do` body's OWN
// def under the same raise still leaks on both lanes (NUR199's row above):
// a `do` body is not a frame.
func TestCalleeDefTornDownOnTrappedRaise(t *testing.T) {
	const g = `def t 0 end def g fn [[][Integer][def t 9 raise 'x']] end `
	for _, src := range []string{
		g + `do [g] end t`,
		g + `do [g] end do [t]`,
		g + `do [(g)] end t`,
		g + `do [g/v apply] end t`,
		g + `do [g] error [drop 1] end t`,
		g + `def f fn [[][Integer][do [g] drop t]] end f`,
		g + `[1 2] each [do [g] drop] t`,
		g + `for 2 [do [g] drop] t`,
		`def t 0 end def g fn [[t:Integer][Integer][raise 'x']] end do [g 9] end t`,
		`def t 0 end def h fn [[][Integer][def t 7 g]] end def g fn [[][Integer][def t 9 raise 'x']] end do [h] end t`,
		`def t 0 end def g fn [[][Integer][def t 9 [1] each [raise 'x'] 1]] end do [g] end t`,
		`def t 0 end def g fn [[][Integer][def t 9 (1 div 0)]] end do [g] end t`,
		`def t 0 end def g fn [[][Map][def t 9 {a: (raise 'x')}]] end do [g] end t`,
		`def t 0 end def g ([] => [def t 9 raise 'x']) end do [g] end t`,
	} {
		requireEngineParity(t, src, true)
	}
	// The interpreter's answer on its own: the callee's def is gone when
	// the trap resumes, and so are its param and its args list — the frame
	// beneath the raise (`f`) still reads its own.
	for _, c := range []struct{ src, want string }{
		{g + `do [g] end t`, "[error(x) 0]"},
		{`def t 0 end def g fn [[t:Integer][Integer][raise 'x']] end do [g 9] end t`, "[error(x) 0]"},
		{`def g fn [[n:Integer][Integer][raise 'x']] end def f fn [[a:Integer][Integer][do [g a] drop args size]] end f 1`, "[1]"},
		{`def g fn [[n:Integer][Integer][raise 'x']] end def f fn [[a:Integer][Integer][do [g 5] drop a]] end f 1`, "[1]"},
		{`def g fn [[n:Integer][Integer][def u n raise 'x']] end do [g 5] end u`, "ERROR:undefined_word"},
	} {
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		got, errI := d.RunInterp(c.src)
		if strings.HasPrefix(c.want, "ERROR:") {
			if errI == nil || !strings.Contains(errI.Error(), strings.TrimPrefix(c.want, "ERROR:")) {
				t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, errI, c.want)
			}
			continue
		}
		if errI != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, errI, c.want)
		}
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
	// A FRAME's var assigned from the token body: the compiled lane
	// declines the shape until its block scopes land (core
	// assignVarUnitGate), the interpreter's block assigns the frame's cell.
	for _, src := range []string{
		`def f fn [[xs:List][List][var t 0 each [var t (t add 1) t] xs]] end f [1 2 3]`,
		`def f fn [[xs:List][List][var t 0 each [var t (t add 1) t] xs]] end f [1 2 3 4 5 6 7]`,
		`def f fn [[xs:List][Integer][var t 0 each [var t (t add 1) t] xs drop t]] end f [1 2 3]`,
		`def f fn [[xs:List][Integer][var t 0 each [var t (t add 1)] xs drop t]] end f [1 2 3]`,
		`def f fn [[b:List xs:List][List][var t 0 each b xs]] end f (quote [var t (t add 1) t]) [1 2 3 4 5 6 7]`,
	} {
		requireEngineParity(t, src, false)
	}
	for _, src := range []string{
		`def f fn [[xs:List][List][each [] xs]] end f [1 2 3]`,
		// An input a stack word RE-PRODUCES at its own position is no longer
		// untouched (the gate keeps its shapes for it); these carry no fn
		// value, so they compile either way.
		`def f fn [[xs:List][List][each [dup drop] xs]] end f [1 2 3]`,
		`def f fn [[xs:List][Integer][0 fold [swap swap drop] xs]] end f [1 2 3]`,
		// A body that exists only at run time: the stamped unit runs as a
		// block, as InvokeBody does — its assignment reaches the module's
		// var, and a root read after it is live.
		`var t 0 end def b (quote [var t (t add 1) t]) end each b [1 2 3] drop end t`,
		`var t 0 end def b (quote [var t (t add 1) add]) end fold b [1 2 3] 0 drop end t`,
		// A lambda callback is a frame of its own: no leak on either lane.
		`def f fn [[xs:List][Integer][def t 0 each ([x:Integer] => [def t 9 x]) xs drop t]] end f [1 2]`,
	} {
		requireEngineParity(t, src, true)
	}
	// The leak is torn down with the fn frame on both lanes: the read after
	// the call is the interpreter's undefined_word, and the check pass
	// mirrors it (an erroring program, not a compiler defect — so it is not
	// booked in the compile-defect ledger).
	src := `def f fn [[xs:List][List][def t 0 each [def t 5 t] xs]] end f [1 2 3] end t`
	gotC, compiled, errC, _, errI := runBothEngines(t, src)
	if codeOf(errI) != "undefined_word" || compiled || len(gotC) != 0 || errC == nil || !strings.Contains(errC.Error(), "undefined word: t") {
		t.Errorf("%q: the fn's local must not outlive its frame on either lane: interp %v; compiled=%v %v / %v", src, errI, compiled, gotC, errC)
	}
	// No interpreter entry: the run-time body stamps once and the whole
	// list runs on the VM (a module var the body assigns is a live cell on
	// both lanes).
	for _, src := range []string{
		`var t 0 end each [var t (t add 1) t] [1 2 3 4 5 6 7] drop end t`,
		`var t 0 end def b (quote [var t (t add 1) t]) end each b [1 2 3 4 5 6 7] drop end t`,
	} {
		if entries, _ := unattributedEntries(t, src); len(entries) != 0 {
			t.Errorf("%q: the keep-defs body entered the interpreter: %v", src, entries)
		}
	}
}

// TestDynamicKeepDefsBodyLeaksToTheFn pins NUR203's close: a keep-defs word
// over a DYNAMIC body (a List param) inside a fn leaks the body's defs into
// the fn's frame, and the fn's later reads see them on both lanes. Every name
// the unit value-defs before the dispatch is seated live at its later reads
// (noteDynKeepDefsLeak), and the kept-defs latch (compiler kept_defs.go,
// main's NUR210) lets such a live read through; the read's value is a
// carrier, so a literal over it is assembled from the lookup, never folded
// from the pre-call binding (`… drop [t]` answered [[0]] for [[3]] when the
// read stayed concrete — the reason the merge of main's #514 first kept the
// latch's decline). Both lanes answer 3; the compiled lane used to read the
// pre-call 0.
func TestDynamicKeepDefsBodyLeaksToTheFn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def f fn [[b:List xs:List][Integer][var t 0 each b xs drop t]] end f (quote [var t (t add 1) t]) [1 2 3]`, "[3]"},
		{`def f fn [[b:List xs:List][Integer][var t 0 fold b xs 0 drop t]] end f (quote [var t (t add 1) add]) [1 2 3]`, "[3]"},
		{`def f fn [[b:List xs:List][Any][var t 0 each b xs drop [t]]] end f (quote [var t (t add 1) t]) [1 2 3]`, "[[3]]"},
		{`def f fn [[b:List xs:List][Any][var t 0 each b xs drop {a: t}]] end f (quote [var t (t add 1) t]) [1 2 3]`, "[{a:3}]"},
	} {
		if gotI, errI := mustNew(t).RunInterp(c.src); errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interpreted %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		// The frame's var assigned from a computed body: the compiled
		// lane declines the shape until its block scopes land (compiler
		// recordDynBodyCall's frame-vars fence); agree where it runs.
		requireEngineParity(t, c.src, false)
	}
}
