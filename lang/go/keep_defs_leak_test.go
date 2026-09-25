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
		// bound, the error trapped by `do` — a `do` body is not a frame,
		// so its defs are the caller's own (a CALLEE's def under the same
		// raise is torn down with its frame: NUR201, below).
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
