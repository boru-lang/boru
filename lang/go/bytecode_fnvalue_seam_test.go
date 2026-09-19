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
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) {
			t.Errorf("%s: compiled/interp disagree\n  compiled %v / [%s] %s\n  interp   %v / [%s] %s\n  %s",
				tc.label, gotC, codeOf(errC), detailOf(errC), gotI, codeOf(errI), detailOf(errI), tc.src)
		}
	}
}
