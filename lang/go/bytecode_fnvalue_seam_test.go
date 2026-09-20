package lang

import (
	"fmt"
	"strings"
	"testing"
)

// bytecode_fnvalue_seam_test.go pins S1b's first increment
// (design/FULL-COMPILATION-REPLAN.0.md §5, the review's §3.3 "unit half"):
// a fn VALUE applied through a runtime seam runs its compiled unit on the
// VM, stamped at first application when nothing stamped it earlier, and the
// seam never enters the interpreter for it.
//
// Two seams carry the value. The TOKEN seam — a higher-order word's list arm
// handing the value to InvokeBody, where the VM's invoker used to step it on
// a pooled sub-engine (the interp-entry census's RunResolved rows, fifty-one
// of them after S1a) — now dispatches it natively (eng/go/vm_fnvalue_seam.go):
// the interpreter's own match, in the seam's top-down order, the unit hosted
// at the value's home, the value's own return contract enforced with the
// token seam's discipline. The fn-VALUE seam — InvokeCallbackFn, the map arm,
// filter, walk — stamps lazily (core.CompiledRuntime.LazyStamp) and takes
// the VM path it already had for a stamped value.
//
// The rows are the seam semantics measured on the interpreter on 2026-09-19,
// before the change: every one must agree on both lanes (value, error code,
// error detail — the corpus compares details), and the rows marked native
// must make NO unattributed interpreter entry on the compiled lane.

type fnValueSeamRow struct {
	label  string
	src    string
	native bool // the compiled lane must not enter the interpreter unattributed
}

var fnValueSeamRows = []fnValueSeamRow{
	// The token seam binds the sig from the stack top down: a is the ELEMENT,
	// e the accumulator — ((1-10) → 2-(-9) → 3-11) = -8, not 4.
	{"fold's list arm binds top-down", `fold ([a:Integer e:Integer] => [a sub e]) [1 2 3] 10`, true},
	// The map arm binds handler order: a is the accumulator — 10-1-2-3 = 4.
	{"fold's map arm binds handler order", `fold ([a:Integer e:KeyVal] => [a sub e.v]) {x:1 y:2 z:3} 10`, true},
	{"a def-bound fn value, reading a module-scope def", `def k 5 end def f fn [[n:Integer][Integer][n add k]] end each f/v [1 2]`, true},
	{"a class-field callback (S1a's callbacks.tsv:57)", `def Handler class {cb: Function} def h (make Handler {cb: (fn [[n:Integer][Integer][n add 1]])}) each h.cb [1 2 3]`, true},
	{"a factory-built capturing lambda", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end each (mk 10) [1 2 3]`, true},
	{"a module export applied from main", `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end each M.inc [1 2 3]`, true},
	{"an unnamed-param fn leaves its input at the frame bottom", `def u fn [[Integer][Integer][add 1 0]] end each u/v [1 2]`, true},
	{"a lambda over a gradual collection that is a List (S1a)", `def f fn [[c:Any][Any][each ([x:Integer] => [x add 1]) c]] end f [1 2]`, true},
	{"a KeyVal lambda at the map arm", `each ([kv:KeyVal] => [kv.v add 1]) {a:1 b:2}`, true},
	{"a named fn at the map arm", `def inc fn [[kv:KeyVal][Integer][kv.v add 1]] end each inc/v {a:1 b:2}`, true},
	{"a lambda at filter", `filter ([p:Any] => [p.value gt 1]) [1 2 3]`, true},
	// The token seam's return discipline is __RC's: the count is enforced
	// over the residual, with the value's name in the error ("" for a lambda).
	{"a lambda's count contract", `each ([x:Integer] => [x 1]) [1 2]`, true},
	{"a named fn's count contract", `def two fn [[n:Integer][Integer][n 1]] end each two/v [1 2]`, true},
	{"a named fn's return type", `def cbad fn [[n:Integer][Boolean][n]] end each cbad/v [1 2]`, true},
	// The map arm's discipline is CallBoru's: a two-value body is trimmed.
	{"the map arm trims a two-value lambda body", `each ([x:KeyVal] => [x.v 1]) {a:1}`, true},
	// No matching own sig: the stepping path decides — an anonymous lambda
	// stays on the stack as the element's result (NUR155's interpreter rule),
	// a named fn raises uncalled_function. The seam declines, so these rows
	// still enter the interpreter, deliberately.
	{"an anonymous lambda that matches no element stays data", `each ([x:Integer] => [x add 1]) ['a' 2]`, false},
	{"a named fn that matches no element raises", `def inc fn [[n:Integer][Integer][n add 1]] end each inc/v ['a' 2]`, false},
	{"a named fn the map arm cannot match raises", `def two fn [[n:Integer][Integer][n 1]] end each two/v {a:1}`, false},
	// S1b-2: a COMPUTED fn value def-bound at the top level — a factory's
	// result, whose analysis-pass binding is the fn-carrier side table, not
	// Defs — read at a higher-order word's forward slot, bare or through
	// `/v`. The collection seat resolves the table (Engine.DefTop), so the
	// dispatch matches the Function overload and the value rides as the
	// STORE_LOCAL's operand; the seam then runs it natively. Each shape was
	// "unmatched dispatch recovered at <word>" before (callbacks.tsv:82,
	// :154, each-variants.tsv:203, fold-map-filter.tsv:73, :227, :229,
	// module-composition.tsv:95).
	{"a factory-built value at each, /v", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def a5 (mk 5) end each a5/v [1 2 3]`, true},
	{"a factory-built value at each, bare", `def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f (mk 10) end each f [1 2 3]`, true},
	{"a factory-built value at fold", `def mk fn [[k:Integer][Function][([a:Integer e:Integer] => [a add e add k])]] end def f (mk 10) end 0 fold f [1 2]`, true},
	{"a factory-built value at filter", `def mk fn [[k:Integer][Function][([p:Map] => [p.value gt k])]] end def f (mk 1) end filter f [1 2 3]`, true},
	{"a branch-chosen value at each", `def choose fn [[b:Boolean][Function][if b [(fn [[n:Integer][Integer][n add 1]])] [(fn [[n:Integer][Integer][n sub 1]])]]] end def f (choose false) end each f [1 2 3]`, true},
	{"a module factory's value at each", `import module [def mk fn k:Integer Function [([n:Integer] => [n add k])] export "M" {mk: mk/v}] end def a5 (M.mk 5) end each a5/v [1 2 3]`, true},
	// FnUtil.compose's result is a fn-util wrapper whose body applies its
	// captured values through the module's own seam — still stepped, one of
	// the fn values S1b owes (the handoff's "wrappers"); compiles with
	// parity today.
	{"a composed value at each", `import "boru:fn-util" end def inc x:Integer => [add 1 x] end def dbl x:Integer => [mul 2 x] end def h (FnUtil.compose inc/v dbl/v) end each h/v [1 2 3]`, false},
	{"a factory-built value at the map arm", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def f (mk 1) end each f/v {a:1 b:2}`, true},
	{"a rebound factory-built value reads the latest bind", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def f (mk 1) end def f (mk 100) end each f/v [1 2 3]`, true},
	{"a factory-built value at each inside a loop body", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def f (mk 1) end for 2 [each f/v [1 2 3]]`, true},
	{"a factory-built value at each inside a fn body", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def run fn [[xs:List][List][def f (mk 2) each f/v xs]] end run [1 2 3]`, true},
	// A def-bound CAPTURING value that matches no element takes the
	// interpreter's data fork on both lanes, with the def's name on the
	// value; the NON-capturing twin (`[s:String] => [s]`, a const fn value)
	// prints anonymous on the compiled lane where the interpreter prints
	// `fn f(String)` — NUR168, not pinned here.
	{"a factory-built value whose sig matches no element", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def f (mk 1) end each f/v ['a' 2]`, false},
	// S1b-2, the fn-VALUE CLOSURE convention: a CAPTURING `fn` / `=>`
	// literal minted at run time is a compiled closure (PUSH_CLOSURE), and
	// every callback seam matches it against its own signature the way the
	// interpreter matches the value — the token seam top down, the map arm
	// over the KeyVal, filter over its entry — instead of running the unit
	// blind over whatever the handler pushed. Every row here was a compile failure
	// on `main` ("function-valued operand at <word>"), released by S1a and
	// miscomputed on the S1a/S1b-1 head; measured on the interpreter.
	{"a capturing closure that matches no element stays data", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end each (mk 1) ['a' 2]`, false},
	{"a capturing closure at the map arm is handed the KeyVal", `def mk fn [[k:Integer][Function][([kv:KeyVal] => [kv.v add k])]] end each (mk 1) {a:1 b:2}`, true},
	{"a capturing closure the map arm cannot match raises", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end each (mk 1) {a:1 b:2}`, true},
	{"a capturing closure fold's map arm cannot match raises", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end fold (mk 1) {a:1 b:2} 0`, true},
	{"a capturing closure at fold's map arm", `def mk fn [[k:Integer][Function][([a:Integer e:KeyVal] => [a add e.v add k])]] end fold (mk 1) {a:1 b:2} 0`, true},
	{"a capturing closure at fold's list arm binds top-down", `def mk fn [[k:Integer][Function][([a:Integer e:Integer] => [a sub e add k])]] end fold (mk 0) [1 2 3] 10`, true},
	{"a capturing closure filter cannot match raises", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end filter (mk 1) [1 2]`, true},
	{"a capturing closure at filter's map form", `def mk fn [[k:Integer][Function][([kv:KeyVal] => [kv.v gt k])]] end filter (mk 1) {a:1 b:2}`, true},
	// The two-value body reads the capture last, which islands the unit
	// (vm:island-resolved) — a pre-existing island shape, not the seam's.
	{"a capturing closure's count contract at the map arm", `def mk fn [[k:Integer][Function][([kv:KeyVal] => [kv.v k])]] end each (mk 1) {a:1}`, false},
	{"a capturing closure's list param arrives quoted", `def mk fn [[k:Integer][Function][([xs:List] => [size xs add k])]] end each (mk 1) [[1 2] [3]]`, true},
	// The token body over a list holding the closure islands (a pre-existing
	// shape); the row pins the identity the bridge carries.
	{"a bridged closure is eq to itself", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end [(mk 1)] each [dup eq]`, false},
}

func TestFnValueSeamParityAndNoEntry(t *testing.T) {
	for _, row := range fnValueSeamRows {
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
		if noteCompileDefect(t, row.src, gotC, errC) {
			continue
		}
		disarm()
		if !compiled {
			t.Errorf("%s: must take the compiled lane\n  %s", row.label, row.src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) {
			t.Errorf("%s: compiled/interp disagree\n  compiled %v / [%s] %s\n  interp   %v / [%s] %s\n  %s",
				row.label, gotC, codeOf(errC), detailOf(errC), gotI, codeOf(errI), detailOf(errI), row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
	}
}

// detailOf is the error's first line — the detail the corpus's
// compile-or-fallback gate compares between lanes — or "" for nil.
func detailOf(err error) string {
	if err == nil {
		return ""
	}
	return strings.SplitN(err.Error(), "\n", 2)[0]
}

// TestFnValueSeamStampsOnce pins the memo: a container-held value applied
// twice compiles its unit once — the second application finds the ref on
// the value's shared impl (the stamp ledger records one stamp for the body).
func TestFnValueSeamStampsOnce(t *testing.T) {
	src := `def hold {f: (fn [[n:Integer][Integer][n add 1]])} end [(each hold.f [1 2]) (each hold.f [3 4])]`
	b, _ := New()
	got, compiled, err := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, err) {
		return
	}
	if err != nil || !compiled || fmt.Sprint(got) != "[[[2 3] [4 5]]]" {
		t.Fatalf("got %v compiled=%v err=%v", got, compiled, err)
	}
	stamps := 0
	for _, ev := range b.StampReport() {
		if ev.Stamped {
			stamps++
		}
	}
	if stamps != 1 {
		t.Errorf("stamp ledger records %d stamps, want exactly 1 (the memo)\n  %v", stamps, b.StampReport())
	}
}

// TestFnValueSeamDeclinesKeepStepping pins the values the seam hands back to
// the stepping path, each with the interpreter's own answer on both lanes: a
// native word's value (its sigs carry Go handlers — nothing to stamp), a body
// with a flow sentinel (never stampable), and a container-held body whose
// detached stamp declines (a `receive` body: result of unknown provenance),
// applied once and twice. The decline's memo — one compile, remembered on
// the value — is pinned where it can be counted, compiler/go
// (TestLazyStampFnSigRemembersDecline): the stamp ledger a whole program
// leaves also carries the compile pass's own per-site attempts.
func TestFnValueSeamDeclinesKeepStepping(t *testing.T) {
	const once = `def hold {f: (fn [[n:Integer][Integer][n receive [[x:Integer] [x]] after 0 [0]]])} end do [(each hold.f [1])] error [dot code]`
	const twice = `def hold {f: (fn [[n:Integer][Integer][n receive [[x:Integer] [x]] after 0 [0]]])} end [(do [(each hold.f [1])] error [dot code]) (do [(each hold.f [1])] error [dot code])]`
	for _, tc := range []struct{ label, src string }{
		{"a native word's value", `each typeof/v [1 2]`},
		{"a body with a flow sentinel", `def f fn [[n:Integer][Integer][break]] end do [(each f/v [1])] error [dot code]`},
		{"a container-held body whose stamp declines", once},
		{"the same body applied twice", twice},
	} {
		a, _ := New()
		gotI, errI := a.RunInterp(tc.src)
		b, _ := New()
		gotC, _, errC := b.RunCompiled(tc.src)
		if noteCompileDefect(t, tc.src, gotC, errC) {
			continue
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) {
			t.Errorf("%s: compiled/interp disagree\n  compiled %v / [%s] %s\n  interp   %v / [%s] %s\n  %s",
				tc.label, gotC, codeOf(errC), detailOf(errC), gotI, codeOf(errI), detailOf(errI), tc.src)
		}
	}
}
