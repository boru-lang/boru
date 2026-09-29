package lang

import (
	"fmt"
	"testing"
)

// census_items3_test.go pins the four mechanisms that took eleven rows off the
// interp-entry census on 2026-09-29 (full compilation's item 3), each against
// the interpreter and with no interpreter entry, beside the windows each must
// leave to the island (pinned open, still at parity):
//
//   - `args` in a TOKEN body reads the live args stack (compiler
//     EmitState.ArgsReadLive, eng invokeTokenBody): `do [args]` inside a fn
//     stamps at run time instead of stepping the body on a pooled sub-engine,
//     and the seam pushes no args frame of its own, as RunResolved pushes
//     none (callbacks.tsv L167–L171, control.tsv L83);
//   - a NAMED fn value that takes no argument runs over nothing, its results
//     on top of the window it never touched (eng vm_fnvalue_zeroarg.go):
//     user-types.tsv L368 at the token seam, fn-value.tsv L319/L320 at
//     CALL_DYNAMIC;
//   - a run-time-minted fn value applied at a shaped method gets its unit
//     from the lazy stamp (eng dynApplyForeign): fn-value.tsv L334;
//   - CALL_DYNAMIC_MIXED's window bound by the interpreter's own plan
//     (core.PlanMatch) for a single-signature, named-param boru fn value
//     (eng vm_mixed_plan.go): fn-value.tsv L19, `3 m.f 2`.
//
// Two message fixes ride along, each a divergence measured on the way: an
// applied nameless VERBOSE fn reports as `<fn>` and a nameless lambda
// unnamed (eng applyFrameName), where the frame path said `` for the one
// and the token seam `<fn>` for the other.

var censusItems3Rows = []struct {
	label, src string
	native     bool
}{
	// --- args in a token body.
	{"args: do [args] in a fn", `def f fn [[y:Integer] [Any] [do [args]]]  f 7`, true},
	{"args: an index read", `def f fn [[y:Integer] [Any] [do [args.0]]]  f 7`, true},
	{"args: two index reads", `def f fn [[y:Integer z:Integer] [Any] [do [args.1 add args.0]]]  f 7 8`, true},
	{"args: an index past the end", `def f fn [[y:Integer] [Any] [do [args.5]]]  f 7`, true},
	{"args: the list consumed after the body", `def f fn [[y:Integer] [Any] [do [args] size]]  f 7`, true},
	{"args: a callback fn value", `def g fn [[n:Integer] [Any] [do [args]]] end each g/v [1 2]`, true},
	{"args: a def-bound lambda callback", `def g ([n:Integer] => [do [args]]) end each g/v [1 2]`, true},
	{"args: a callback's own call args", `def h fn [[a:Integer] [Any] [do [args]]] end def w fn [[x:Integer] [Any] [each h/v [x 5]]] end w 7`, true},
	{"args: the enclosing fn's, per element", `def w fn [[x:Integer] [Any] [each [do [args]] [x 5]]] end w 7`, true},
	{"args: a quoted body", `def b quote [args] def w fn [[x:Integer] [Any] [do b]] end w 7`, true},
	{"args: a return contract over the list", `def f fn [[y:Integer] [Integer] [do [args]]]  f 7`, true},
	{"args: a module fn", `import module [def f fn [[y:Integer] [Any] [do [args]]] export "M" {f: f/v}] end M.f 4`, true},
	{"args: no enclosing fn", `do [args]`, true},
	// The seam pushed the element as the body's args frame and the dynamic
	// body read it: [7] then [5] for the interpreter's [7] twice.
	{"args: a computed body's dynamic body", `def mk fn [[] [List] [quote [do [args]]]] end def w fn [[x:Integer] [Any] [def b (mk) each b [x 5]]] end w 7`, true},
	// `__pa` pops the frame: still a context-dependent decline.
	{"args: __pa in the body (open)", `def f fn [[y:Integer] [Any] [do [args __pa]]]  f 7`, false},

	// --- a named zero-argument fn value.
	{"zero-arg: a callback", `def g fn [[] [Integer] [7]] end each g/v [1 2]`, true},
	{"zero-arg: a fold callback", `def g fn [[] [Integer] [7]] end fold g/v [1 2] 0`, true},
	{"zero-arg: over container elements", `def g fn [[] [Integer] [7]] end each g/v [[1] {a:1}]`, true},
	{"zero-arg: a callback minting a type", `def g fn [[] [Integer] [def T (class {}) undef T 1]] end each g/v [1 2]`, true},
	{"zero-arg: a count error", `def g fn [[] [Integer] [7 8]] end each g/v [1 2]`, true},
	{"zero-arg: a type error", `def g fn [[] [Integer] ['x']] end each g/v [1 2]`, true},
	{"zero-arg: a raise", `def g fn [[] [Integer] [raise oops 'boom']] end each g/v [1 2]`, true},
	{"zero-arg: an empty body at a trailing apply", `def h fn [[] [] []] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end 5 m.f`, true},
	{"zero-arg: an empty body read by get", `def h fn [[] [] []] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end 5 m get 'f'`, true},
	{"zero-arg: a result on top of the arg", `def h fn [[] [Integer] [9]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end 5 m.f`, true},
	{"zero-arg: a raise at a trailing apply", `def h fn [[] [Integer] ['x']] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end 5 m.f`, true},
	{"zero-arg: an overload taking an argument (dispatches it)", `def g fn [[] [Integer] [7] [a:Integer] [Integer] [a]] end each g/v [1 2]`, true},
	// An empty body declaring a return: the count error is the island's.
	{"zero-arg: an empty body declaring a return (open)", `def g fn [[] [Integer] []] end each g/v [1 2]`, false},
	{"zero-arg: an empty body declaring a return at an apply (open)", `def h fn [[] [Integer] []] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end 5 m.f`, false},
	// An anonymous lambda with nothing to take parks: not this arm's.
	{"zero-arg: an anonymous lambda parks (open)", `def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end 5 m.f`, false},

	// --- the lazy stamp at a shaped method.
	{"lazy: a def-bound factory lambda", `def mk fn [[k:Integer][Function][([s:String] => [s])]] end def f (mk 1) end do [f 'z']`, true},
	{"lazy: in a fn body", `def mk fn [[k:Integer][Function][([s:String] => [s])]] end def f (mk 1) end def g fn [[][Any][f 'q']] end g`, true},
	{"lazy: a count error named for the binding", `def mk fn [[k:Integer][Function][([s:String] => [s s])]] end def f (mk 1) end do [f 'z']`, true},
	{"lazy: a raise", `def mk fn [[k:Integer][Function][([s:String] => [s raise oops 'x'])]] end def f (mk 1) end do [f 'z']`, true},
	{"lazy: a nameless fn's contract error is <fn>'s", `def mk fn [[][Map][{f: (fn [[s:String][Integer][s]])}]] end def m (mk) m.f 'z'`, true},
	{"lazy: a nameless fn's count error", `def mk fn [[][Map][{f: (fn [[s:String][Integer][s s]])}]] end def m (mk) m.f 'z'`, true},
	{"lazy: a lambda's count error is unnamed", `def mk fn [[k:Integer][Map][{f: ([s:String] => [s s])}]] end def m (mk 1) end m.f 'z'`, true},
	{"frame: a nameless fn's contract error is <fn>'s", `def m {f: (fn [[s:String][Integer][s]])} m.f 'z'`, true},
	{"frame: a factory's nameless fn applied in a paren", `def mk fn [[][Function][(fn [[s:String][Integer][s]])]] end ((mk) 'z')`, true},
	// No signature admits the arg: the island raises the named no-match.
	{"lazy: a rejected arg (open)", `def mk fn [[k:Integer][Function][([s:String] => [s])]] end def f (mk 1) end do [f 5]`, false},

	// --- the mixed window bound by the interpreter's plan.
	{"mixed: the split rule", mixedLam + `3 m.f 2`, true},
	{"mixed: untaken values either side", mixedLam + `9 3 m.f 2 7`, true},
	{"mixed: one param, the stack value untaken", `def m {f: (fn [[a:Integer][Integer][a mul 100]])}  3 m.f 2`, true},
	{"mixed: three params, two forward", `def m {f: (fn [[a:Integer b:Integer c:Integer][Integer][(a mul 100) add (b mul 10) add c]])}  3 m.f 2 1`, true},
	{"mixed: three params, two beneath", `def m {f: (fn [[a:Integer b:Integer c:Integer][Integer][(a mul 100) add (b mul 10) add c]])}  4 3 m.f 2`, true},
	{"mixed: a type-directed split", `def m {f: (fn [[a:Integer b:String][Any][[a b]]])}  'x' m.f 3`, true},
	{"mixed: a barrier", `def m {f: (fn [[a:Integer | b:Integer][Integer][(a mul 100) add b]])}  3 m.f 2`, true},
	{"mixed: a lambda", `def m {f: ([a:Integer b:Integer] => [(a mul 100) add b])}  3 m.f 2`, true},
	{"mixed: a named fn value", `def g fn [[a:Integer b:Integer][Integer][(a mul 100) add b]] end def m {f: g/v}  3 m.f 2`, true},
	{"mixed: a factory's value", `def mk fn [[][Map][{f: (fn [[a:Integer b:Integer][Integer][(a mul 100) add b]])}]] end def m (mk) 3 m.f 2`, true},
	{"mixed: a return type error", `def m {f: (fn [[a:Integer b:Integer][String][a add b]])}  3 m.f 2`, true},
	{"mixed: a return count error", `def m {f: (fn [[a:Integer b:Integer][Integer][a add b a]])}  3 m.f 2`, true},
	{"mixed: a lambda's count error", `def m {f: ([a:Integer b:Integer] => [a b])}  3 m.f 2`, true},
	{"mixed: a raise", `def m {f: (fn [[a:Integer b:Integer][Integer][raise oops 'no']])}  3 m.f 2`, true},
	// The plan picks nothing: the island parks the value (open).
	{"mixed: no pick parks (open)", `def m {f: (fn [[a:Integer b:String][Any][[a b]]])}  3 m.f 'x'`, false},
	// Unnamed params and overloads keep the island (open).
	{"mixed: unnamed params (open)", `def m {f: (fn [[Integer Integer][Integer][mul]])}  3 m.f 2`, false},
	{"mixed: two overloads (open)", `def m {f: (fn [[a:Integer b:Integer][Integer][a sub b] [a:String][String][a]])}  3 m.f 2`, false},
	// A stack-only barrier takes both from the stack: [3 fn 2] parks.
	{"mixed: a stack-only barrier (open)", `def m {f: (fn [[| a:Integer b:Integer][Integer][(a mul 100) add b]])}  3 m.f 2`, false},
}

const mixedLam = `def m {f: (fn [[a:Integer b:Integer][Integer][(a mul 100) add b]])}  `

func TestCensusItems3ParityAndNoEntry(t *testing.T) {
	for _, row := range censusItems3Rows {
		gotI, errI := mustNew(t).RunInterp(row.src)
		b := mustNew(t)
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" && !ev.CheckMode {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if !compiled || fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s:\n  compiled %v [%v] (compiled=%v)\n  interp   %v [%v]\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}
