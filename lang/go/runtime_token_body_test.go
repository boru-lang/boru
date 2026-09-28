package lang

import (
	"fmt"
	"testing"
)

// runtime_token_body_test.go pins S3's first slice (2026-09-24): a TOKEN
// body reaching the InvokeBody seam at run time — a quoted list read from a
// flex, returned by a fn, selected by a branch, passed as a List param, built
// by `push` — runs as a run-time-stamped unit hosted by the VM (eng
// vm_token_body.go over compiler.StampTokenBody), where the seam stepped it
// on a pooled sub-engine once per application (the interp-entry census's
// code-bodies.tsv, twenty of its twenty-one rows, and four rows elsewhere).
// The unit is memoised by the body (its ID, or its tokens with positions
// when it has none) and the input shape (count and concrete types); a free
// word rebound between applications recompiles (the JIT re-stamp); a body
// the stamp declines — a flow sentinel, an empty body, `args` — keeps the
// interpreter, byte-identically.

var runtimeTokenBodyRows = []struct {
	label, src, want string
	native           bool
}{
	{"a flex member body (L122)", `def bodies (flex {inc:(quote [add 1])}) end each bodies.inc [1 2]`, "[[2 3]]", true},
	{"a flex get body (L123)", `def s (flex {}) end s set 'body' (quote [add 1]) drop end each (s get 'body') [1 2 3]`, "[[2 3 4]]", true},
	{"a flex dot body (L124)", `def s (flex {}) end s set 'body' (quote [add 1]) drop end each s.body [1 2 3]`, "[[2 3 4]]", true},
	{"do over a fn's result (L136)", `def mk fn [[n:Integer][List][quote [1 add 2]]] end do (mk 0)`, "[3]", true},
	{"if over a fn's result (L137)", `def mk fn [[n:Integer][List][quote [1 add 2]]] end if true (mk 0) [0]`, "[3]", true},
	{"fold over a fn's result, the accumulator left beneath (L138)", `def mk fn [[n:Integer][List][quote [add 1]]] end fold (mk 0) [1 2 3] 0`, "[4]", true},
	{"filter over a fn's result (L139)", `def mk fn [[n:Integer][List][quote [gt 1]]] end filter (mk 0) [1 2 3]`, "[[2 3]]", true},
	{"scan over a fn's result (L140)", `def mk fn [[n:Integer][List][quote [add]]] end scan (mk 0) [1 2 3]`, "[[1 3 6]]", true},
	{"each over a 0-arg fn's result (L143)", `def mkb fn [[][List][quote [add 1]]] end each (mkb) [1 2 3]`, "[[2 3 4]]", true},
	{"a body a branch selects inside a fn (L144)", `def pickbody fn [[f:Boolean][List][if f [(quote [add 1])] [(quote [mul 2])]]] end each (pickbody true) [1 2 3]`, "[[2 3 4]]", true},
	{"a body a branch selects at the top (L145)", `def b (if true [(quote [add 1])] [(quote [mul 2])]) end each b [1 2 3]`, "[[2 3 4]]", true},
	{"do over a List param (L148)", `def f fn [[b:List][Any][do b]] end f (quote [1 add 2])`, "[3]", true},
	{"if over a List param (L149)", `def f fn [[b:List][Any][if true b [0]]] end f (quote [1 add 2])`, "[3]", true},
	{"filter over a List param (L150)", `def f fn [[b:List xs:List][List][filter b xs]] end f (quote [gt 1]) [1 2 3]`, "[[2 3]]", true},
	{"fold over a List param (L151)", `def f fn [[b:List xs:List][Integer][fold b xs 0]] end f (quote [add]) [1 2 3]`, "[6]", true},
	{"each over a List param, the body second (L153)", `def f fn [[xs:List b:List][List][each b xs]] end f [1 2] (quote [add 1])`, "[[2 3]]", true},
	{"each over a List param (L154)", `def f fn [[b:List][List][each b [1 2 3]]] end f (quote [add 1])`, "[[2 3 4]]", true},
	{"a map member body through a Map param (L156)", `def m {f:(quote [add 1])} end def g fn [[c:Map][List][each c.f [1 2 3]]] end g m`, "[[2 3 4]]", true},
	{"a do body over a frame local (L166)", `def f fn [[n:Integer][List][def xs (do [[n n]]) each [add 1] xs]] end f 1`, "[[2 2]]", true},
	{"a body built by push, no identity (L219)", `do (push (quote 2) [1 (quote add)])`, "[1 add 2]", true},
	{"do over a body fetched from a list (control L82)", `def i 0  def ops [quote [1 add 2]]  do (ops get (i add 0))`, "[3]", true},
	{"a word splice (word-splice L126)", `def xs [add 1 2]  do [word xs]`, "[3]", true},
	{"a fn-local named fn as a fold body (fold-map-filter L239)", `def runner fn [[xs:List][Integer][def step fn [[a:Integer e:Integer][Integer][a add e]] 0 fold [step] xs]]  runner [1 2 3]`, "[6]", true},
	{"a generic element's fold body (generics-fn L55)", `def Box gen [T] class {value:T} def sumvals gen [T] fn [[bs:[:T]] [Integer] [0 fold [dot value add] bs]] def xs [(make (Box of [Integer]) {value:10}) (make (Box of [Integer]) {value:20})] sumvals xs`, "[30]", true},
	// A body that exists only at run time (a fn's returned quotation is a
	// fresh clone per call, so the text key names it) meets its unit
	// again; a dependency rebound in between recompiles (the JIT re-stamp),
	// and one rebound past the re-stamp budget takes the interpreter, which
	// resolves the binding live — the budget's whole point.
	{"a free word rebound between applications recompiles", `def k 1 def mk fn [[][List][quote [add k]]] end each (mk) [1] def k 10 each (mk) [1]`, "[[2] [11]]", true},
	{"a called fn redefined between applications recompiles", `def inc fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [inc]]] end each (mk) [1] def inc fn [[n:Integer][Integer][n add 100]] end each (mk) [1]`, "[[2] [101]]", true},
	{"a compound binding rebound between applications recompiles", `def c [9] def mk fn [[][List][quote [c size]]] end each (mk) [1] def c [9 9] each (mk) [1]`, "[[1] [2]]", true},
	{"a dependency rebound past the re-stamp budget takes the interpreter (open, by design)", `def inc fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [inc]]] end each (mk) [1] def inc fn [[n:Integer][Integer][n add 2]] end each (mk) [1] def inc fn [[n:Integer][Integer][n add 3]] end each (mk) [1] def inc fn [[n:Integer][Integer][n add 4]] end each (mk) [1] def inc fn [[n:Integer][Integer][n add 5]] end each (mk) [1]`, "[[2] [3] [4] [5] [6]]", false},
	{"a def-bound body is lowered by the program, not the seam", `def k 1 def b (quote [add k]) each b [1] def k 10 each b [1]`, "[[2] [11]]", true},
	// A break or continue raised INSIDE a hosted token body with no loop in
	// its own frames is the enclosing run's to resolve, as the seam's
	// sub-engine handed it back (vmContext.flowEscapes): the `for` ends, or
	// steps on, on both lanes. The rows enter elsewhere (the loop body's
	// frame replay islands at its `i`), not at this seam.
	{"a break through a called fn ends the enclosing loop", `def f fn [[x:Integer] [Integer] [break 7]] def b (quote [f 1]) for 5 [do b i] 99`, "[99]", false},
	{"a break through two called fns ends the enclosing loop", `def f fn [[x:Integer] [Integer] [break 7]] def g fn [[x:Integer] [Integer] [f x]] def b (quote [g 1]) for 5 [do b i] 99`, "[99]", false},
	{"a break through a recursion ends the enclosing loop", `def r fn [[n:Integer] [Integer] [if (n lte 0) [break 0] [r (n sub 1)]]] def b (quote [r 2]) for 5 [do b i] 99`, "[99]", false},
	{"a continue through a called fn steps the enclosing loop", `def f fn [[x:Integer] [Integer] [continue 7]] def b (quote [f 1]) for 3 [do b i] 99`, "[99]", false},
	// A flow sentinel INSIDE the computed body, under a native inside a
	// loop: the flag the body run leaves set is read after the native
	// returns (NUR195's close), so the `for` ends or steps on.
	{"a break inside an each body ends the enclosing loop (NUR195)", `def mk fn [[][List][quote [break]]] end for 3 [each (mk) [1 2 3] i] 99`, "[99]", false},
	{"a continue inside an each body steps the enclosing loop (NUR195)", `def mk fn [[][List][quote [continue]]] end for 3 [each (mk) [1 2 3] i] 99`, "[99]", false},
	// A body carrying a reference value is keyed by its ID (a fn's returned
	// quotation has one); built by `push` at run time it has none, and the
	// seam keeps the interpreter rather than name it by text.
	{"a body with a map literal, keyed by its identity", `def mk fn [[][List][quote [{a:1} size]]] end do (mk)`, "[1]", true},
	{"a body with a map inside a nested list, keyed by its identity", `def mk fn [[][List][quote [[{a:1}] size]]] end do (mk)`, "[1]", true},
	{"a pushed body with a map, no identity to key by (open)", `do (push (quote size) [{a:1}])`, "[{a:1} size]", false},
	{"the inputs keep their stack order", `def mk fn [[][List][quote [sub]]] end 10 fold (mk) [1 2 3]`, "[4]", true},
	{"a heterogeneous collection compiles once per input type", `def mk fn [[][List][quote [typeof]]] end each (mk) [1 "a" [2]]`, "[[Integer ProperString List]]", true},
	{"one body under two seam arities", `def mk fn [[][List][quote [add 1]]] end def b (mk) each b [1 2] fold b [1 2] 0`, "[[2 3] 3]", true},
	// A token body that is one map literal bearing paren groups compiles
	// in-frame since NUR256's close (NUR209 until the merge of main's #511; the closure records the assembly), so
	// the dyn-scope rescue's family no longer keeps the interpreter here.
	{"a map literal with paren groups compiles in-frame (was open, L77)", `do [{a:(1 add 2) b:(2 mul 3)}]`, "[{a:3 b:6}]", true},
	// Open, each keeping the interpreter as before: an empty body and `args`
	// read inside a token body. A body with a flow sentinel is declined
	// by the stamp too (as the lazy stamp declines a fn body's) and keeps
	// the interpreter (TestComputedBodyFlowSentinelDefers).
	{"an empty body keeps the interpreter (open)", `def mk fn [[][List][quote []]] end do (mk)`, "[]", false},
	{"args inside a token body keeps the interpreter (open, control L83)", `def f fn [[y:Integer] [Any] [do [args]]]  f 7`, "[[7]]", false},
}

func TestRuntimeTokenBodyParity(t *testing.T) {
	for _, row := range runtimeTokenBodyRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != row.want {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}

// TestRuntimeTokenBodyRaisesAlike: a raise inside a run-time-stamped body,
// and a no-match inside one, are the interpreter's errors — named, detailed
// and rendered alike, position included.
func TestRuntimeTokenBodyRaisesAlike(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[][List][quote [raise bad_input "boom"]]] end each (mk) [1]`,
		`def mk fn [[][List][quote [0 div]]] end each (mk) [1]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || errI == nil || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || fmt.Sprint(errC) != fmt.Sprint(errI) || len(gotC) != 0 || len(gotI) != 0 {
			t.Errorf("%q: compiled %v [%s] %v (%v), interp %v [%s] %v", src, gotC, codeOf(errC), errC, compiled, gotI, codeOf(errI), errI)
		}
	}
}

// TestComputedBodyFlowSentinelDefers pins NUR195's close (2026-09-24, the
// escaped flow after a native call): a flow sentinel inside a COMPUTED body
// under `each` or `fold` with no enclosing loop is the interpreter's
// `flow_error: break outside loop`, and the compiled lane — which answered
// `[[1 2 3]]` silently, the flag a native's body run left set never read —
// now reads the flag after every native call and takes the loop-less
// flow's designed path: the internal error RunCompiled's callers defer to
// the interpreter on (a bail in the lang ledger, loud, never a value). With
// an enclosing loop the lanes agree outright (the `for` rows in the parity
// table).
func TestComputedBodyFlowSentinelDefers(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[][List][quote [break]]] end each (mk) [1 2 3]`,
		`def mk fn [[][List][quote [continue]]] end each (mk) [1 2 3]`,
		`def mk fn [[][List][quote [break]]] end fold (mk) [1 2 3] 0`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "flow_error" || len(gotI) != 0 {
			t.Fatalf("%q: the interpreter's answer moved: %v / %v", src, gotI, errI)
		}
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if !compiled || codeOf(errC) != "flow_error" || len(gotC) != 0 {
			t.Errorf("%q: compiled %v / %v (%v) — neither the interpreter's raise nor the loop-less deferral", src, gotC, errC, compiled)
		}
	}
}

// TestLiteralBodyFlowThroughFnResolves pins NUR196's close (2026-09-24,
// the escaped body ends the iteration): a `break` raised by a fn CALLED
// from a LITERAL each body — `def f fn [[x:Integer] [Integer] [break]] end
// each [f] [1 2 3]` — is the interpreter's `flow_error: break outside
// loop`, and under `for 3 [… i] 99` its [99]; the compiled lane raised
// each's own `body produced no result` on both, the fn's island having
// returned nothing on the escape before the flag could be read. The
// iterating natives end their iteration on an escaped body now
// (core.BodyEscaped) and return no result, so the run resolves the flag on
// both lanes: [99] under the loop, and with no loop at all the
// interpreter's raise against the compiled lane's loop-less deferral bail
// (as NUR195's witnesses).
func TestLiteralBodyFlowThroughFnResolves(t *testing.T) {
	const f = `def f fn [[x:Integer][Integer][break]] end `
	for _, src := range []string{
		f + `for 3 [each [f] [1 2 3] i] 99`,
		f + `for 3 [each [f] {a:1} i] 99`,
		`def g fn [[x:Integer][Integer][continue]] end for 3 [each [g] [1 2 3] i] 99`,
		f + `for 3 [scan [f] [1 2 3] i] 99`,
		f + `for 3 [filter [f] [1 2 3] i] 99`,
		f + `for 3 [filter [f] {a:1} i] 99`,
		f + `for 3 [outer [f] [1 2] [3 4] i] 99`,
		f + `for 3 [inner [f] [add] [1 2] [3 4] i] 99`,
		`def h fn [[x:Integer y:Integer][Integer][break]] end for 3 [inner [add] [h] [1 2] [3 4] i] 99`,
		f + `for 3 [inner [f] [add] [[1 2] [3 4]] [[5 6] [7 8]] i] 99`,
		`def h fn [[x:Integer y:Integer][Integer][break]] end for 3 [inner [add] [h] [[1 2] [3 4]] [[5 6] [7 8]] i] 99`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != "[99]" || fmt.Sprint(gotI) != "[99]" {
			t.Errorf("%q: compiled %v/%v (%v) interp %v/%v, want [99] on both lanes", src, gotC, errC, compiled, gotI, errI)
		}
	}
	// The natives the compiler still declines (a code-body word, a fold
	// whose branch leaves extra values): the interpreter's [99] is the
	// contract, and the compiled lane reaches it by fallback or by parity.
	for _, src := range []string{
		f + `for 3 [[1 2] for-each [f] i] 99`,
		f + `for 3 [0 fold [f] [1 2 3] i] 99`,
		`import "boru:array-util" ` + f + `for 3 [ArrayUtil.eachrank 0 [f] [[1 2] [3 4]] i] 99`,
		`import "boru:array-util" def fl fn [[x:List][Integer][break]] end for 3 [ArrayUtil.eachrank 1 [fl] [[1 2] [3 4]] i] 99`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if errI != nil || fmt.Sprint(gotI) != "[99]" {
			t.Errorf("%q: interp %v/%v, want [99]", src, gotI, errI)
		}
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != "[99]" {
			t.Errorf("%q: compiled %v/%v (%v), want [99]", src, gotC, errC, compiled)
		}
	}
	for _, src := range []string{
		f + `each [f] [1 2 3]`,
		f + `0 fold [f] [1 2 3]`,
		f + `scan [f] [1 2 3]`,
		f + `filter [f] [1 2 3]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "flow_error" || len(gotI) != 0 {
			t.Fatalf("%q: the interpreter must raise the flow error, not read the escaped residual: %v / %v", src, gotI, errI)
		}
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if !compiled || codeOf(errC) != "flow_error" || len(gotC) != 0 {
			t.Errorf("%q: compiled %v / %v (%v) — neither the interpreter's raise nor the loop-less deferral", src, gotC, errC, compiled)
		}
	}
}
