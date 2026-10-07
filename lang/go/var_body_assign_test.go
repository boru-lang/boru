package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestVarAssignedInsideBodyIsNoBodyLocal pins the var word's two readings
// inside a code body on both lanes. `var NAME v` over a var visible in the
// frame ASSIGNS that cell — the each body's `var s (s add 1)` replaces the
// enclosing fn's `s` in place, and the fn's read after the body sees the
// sum — while `var NAME v` over a name the frame does not know as a var
// DECLARES one the body owns. The closure-body compile promotes a body's own
// bindings to slots of its unit (core.CollectBodyLocalDefs), and counting the
// assignment among them gave the each body a fresh `s` the enclosing read
// never saw — `fn f: body result of unknown provenance` where HEAD compiled
// (var.tsv L36, 2026-10-07). The scan is registry-aware now: a var already
// visible is assigned, never a local.
//
// A nested FN or lambda is a frame of its own: it reads the enclosing fn's
// var as the captured value, and its `var s …` over that name is the frame
// rule's var_error on both lanes — the same refusal a fn body meets over a
// module var (var.tsv §3). The capture carries the cell's kind
// (core.CapturedBinding.Var) so the var word can tell it from a captured
// def, which `var` shadows with the closure's own cell.
func TestVarAssignedInsideBodyIsNoBodyLocal(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def f fn [[xs:List] [Integer] [var s 0 def _ (each [var s (s add 1)] xs) s]] f [1 2 3]`, "[3]"},
		{`def g fn [[][Integer][var s 5 (each [var s (s add 1)] [1 2]) drop s]] (g)`, "[7]"},
		{`var s 0 def _ (each [var s (s add 1)] [1 2 3]) s`, "[3]"},
		// A var the body declares is its own: one cell per run, re-declared
		// at the module scope on the second run, assigned there.
		{`def f fn [[][List][(each [var t 1 t] [1 2])]] (f)`, "[[1 1]]"},
		{`(each [var t 1 t] [1 2])`, "[[1 1]]"},
		// A nested fn reads the enclosing var's captured value and never
		// assigns it; a captured def is shadowed by the closure's own var.
		{`def f fn [[][Integer][var s 1 def h fn [[][Integer][s add 1]] (h)]] (f)`, "[2]"},
		{`def g fn [[][List][def s 5 def f fn [[][Integer][var s 0 s]] [(f) s]]] (g)`, "[[0 5]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The refusals. A callback lambda's refusal traps at run time on both
	// lanes (var.tsv §3's `each ([x] => [var acc …])`); a NAMED fn's or a
	// def-bound lambda's body meets it in the check pass's body analysis,
	// which stops the compile at the same var_error (a fn_body_error) where
	// the interpreter raises it at the call — the module-var rows of var.tsv
	// §3 compile the same way, and the stop is counted in the compile-defect
	// ledger as every check stop is (NUR386).
	for _, src := range []string{
		`def g fn [[][List][var s 5 def f fn [[][Integer][var s 0 s]] [(f) s]]] (g)`,
		`def g fn [[][List][var s 5 def f ([] => [var s 0 s]) [(f) s]]] (g)`,
		// The callback lambda compiles and traps at the site; its rendered
		// position differs from the interpreter's by the token it names.
		`def g fn [[][List][var s 5 [(each ([x] => [var s (s add x)]) [1 2]) s]]] (g)`,
	} {
		frameRuleRefused(t, src)
	}
}

// frameRuleRefused asserts the frame rule's var_error on the interpreter and
// the compiled lane's agreement: the same error when the program compiles,
// else a compile stop that names the same var_error, booked as a compile
// defect (noteCompileDefect's ledger).
func frameRuleRefused(t *testing.T, src string) {
	t.Helper()
	const msg = "cannot assign a var of an enclosing frame"
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI == nil || !strings.Contains(errI.Error(), msg) {
		t.Fatalf("%s: interpreter %v / %v, want the frame rule's var_error", src, gotI, errI)
	}
	if compiled {
		if errC == nil || !strings.Contains(errC.Error(), msg) {
			t.Errorf("%s: compiled %v / %v, want the interpreter's var_error", src, gotC, errC)
		}
		return
	}
	requireCompileDefect(t, src, gotC, errC)
	if !strings.Contains(fmt.Sprint(errC), "var_error") || !strings.Contains(fmt.Sprint(errC), msg) {
		t.Errorf("%s: the compile stop must name the frame rule's var_error, got %v", src, errC)
	}
}
