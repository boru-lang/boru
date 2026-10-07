package lang

import (
	"fmt"
	"strings"
	"testing"
)

// block_scope_rule_test.go pins phase 2 of design/IMMUTABLE-DEF.1.md on the
// lang surface: a code body a word runs — an `if`/`case` arm, an `if`/`while`
// condition list, a `case` scrutinee, a loop or callback body — is a BLOCK,
// so a `def` inside it ends with the body (lang/spec/block-scopes.tsv has
// the corpus rows). The interpreter's answer is the rule's. The compiled
// lane DECLINES these shapes until the compiler's own block scopes land
// (phase 2's second step): a block-local def over an enclosing binding
// declines under the block gate (core noteBindCensus), and a read after a
// block of a name only the block bound stops the check pass with the
// interpreter's own undefined_word. Either way a decline, never a wrong
// answer — booked in the compile-defect ledger like every other.
func requireBlockRule(t *testing.T, src, interpWant, reason string) {
	t.Helper()
	gotI, errI := mustNew(t).RunInterp(src)
	if sub, isErr := strings.CutPrefix(interpWant, "ERROR:"); isErr {
		if errI == nil || !strings.Contains(errI.Error(), sub) {
			t.Errorf("%q: interpreter %v / %v, want an error containing %q", src, gotI, errI, sub)
		}
	} else if errI != nil || fmt.Sprint(gotI) != interpWant {
		t.Errorf("%q: interpreter %v / %v, want %s", src, gotI, errI, interpWant)
	}
	prog, got, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("%q: CompileCheck: %v", src, cerr)
	}
	if prog != nil {
		t.Errorf("%q: compiled — the compiler's block scopes have landed for this shape; move the row to a parity test", src)
		return
	}
	if !strings.Contains(got, reason) {
		t.Errorf("%q: decline reason %q, want it to contain %q", src, got, reason)
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	requireCompileDefect(t, src, gotC, errC)
	if compiled {
		t.Errorf("%q: RunCompiled reported a compiled run of a declined program", src)
	}
}

// blockRuleOrParity asserts the interpreter's answer (a value list, or
// "ERROR:<substring>") and, on the compiled lane, either parity — the shape
// compiled — or the block gate's decline, booked.
func blockRuleOrParity(t *testing.T, src, interpWant string) {
	t.Helper()
	gotI, errI := mustNew(t).RunInterp(src)
	if sub, isErr := strings.CutPrefix(interpWant, "ERROR:"); isErr {
		if errI == nil || !strings.Contains(errI.Error(), sub) {
			t.Errorf("%q: interpreter %v / %v, want an error containing %q", src, gotI, errI, sub)
		}
	} else if errI != nil || fmt.Sprint(gotI) != interpWant {
		t.Errorf("%q: interpreter %v / %v, want %s", src, gotI, errI, interpWant)
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if !compiled {
		if !strings.Contains(fmt.Sprint(errC), "block-local") {
			t.Errorf("%q: declined for a reason other than the block gate: %v", src, errC)
		}
		requireCompileDefect(t, src, gotC, errC)
		return
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: compiled %v / %v, interpreter %v / %v", src, gotC, errC, gotI, errI)
	}
}

func TestBlockScopeRuleOnTheLangSurface(t *testing.T) {
	const gate = "block-local def `"
	const stop = "check diagnostics"
	for _, c := range []struct{ src, want, reason string }{
		// arms
		{`if true [def w 1] [def w 2] w`, "ERROR:undefined word: w", stop},
		{`def x 1 if true [def x 9] [] x`, "[1]", gate + "x`"},
		{`def c true if c [def op 1] [0] op`, "ERROR:undefined word: op", stop},
		{`def g fn [[n:Integer] [Boolean] [n gt 5]] if (g 9) [def op 1] [0] op`, "ERROR:undefined word: op", stop},
		{`def f fn [[b:Boolean] [Integer] [if b [def z 9] [] z]] f true`, "ERROR:undefined word: z", stop},
		{`def two fn [[][Integer Integer][1 2]] def c true if c [def x (two)] [] x`, "ERROR:undefined word: x", stop},
		{`if [true] [def y 9 5] [0] y`, "ERROR:undefined word: y", stop},
		{`def g fn [[x:Any] [Integer] [x add 100]] if true [def g fn [[x:Any] [Integer] [x add 1]]] g 1`, "[101]", gate + "g`"},
		{`def m {e: true} def g fn [[][Integer][if (m "e" get) [def f fn [[x:Integer][Integer][x add 1]]] [] do [f 5]]] g`, "ERROR:undefined word: f", stop},
		{`def body (quote [def zz 7 zz]) def n 5 (if (n eq 0) [99] body) zz add 1`, "ERROR:undefined word: zz", stop},
		// conditions and scrutinees
		{`def x 1 if [def x 5 true] [2] [3] x`, "[2 1]", gate + "x`"},
		{`if [def y 5 true] [y] [3] y`, "ERROR:undefined word: y", stop},
		{`def x 1 case [def x 5 x] [5 "five" "other"] x`, "[five 1]", gate + "x`"},
		{`case [def y 7 y] [7 "seven" "other"] y`, "ERROR:undefined word: y", stop},
		{`if [def f fn [[][Integer][7]] true] [f] [3] f`, "ERROR:undefined word: f", stop},
		// loop bodies
		{`def n 0 for 3 [def n (n add 1)] n`, "[0]", gate + "n`"},
		{`for 2 [def x 9] x`, "ERROR:undefined word: x", stop},
		{`def t 0 for 3 [do [def t 5]] t`, "[0]", gate + "t`"},
		{`for 2 [ def acc (for 2 [5]) ] acc`, "ERROR:undefined word: acc", stop},
		{`def i 0 for 3 [def i 9] i`, "[0]", gate + "i`"},
		{`def t 0 each [def t (t add 1) t] [1 2 3] t`, "[[1 1 1] 0]", gate + "t`"},
		// a parser bound in an arm (bytecode_m3m4_test.go's neighbour)
		{`import "boru:parselang" def p (fn [[source:String opts:Map] [Any] [7]]) def c false if c [def s 'inc'] [] parse p s`, "ERROR:undefined word: s", stop},
	} {
		requireBlockRule(t, c.src, c.want, c.reason)
	}
}

// TestCallbackBodyBlockOnTheVM pins the compiled lane's half of a callback
// body's block (eng invokeClosureOn): a def the body makes is retired with
// the element's run on BOTH lanes, measured on the NEXT request — the read
// after the program raises undefined_word on each instance — and a `var`
// of the enclosing scope assigned in the body is the one cell both lanes
// read after the loop (the declaration's twin replays a var, core
// InstallVar). Before the bracket the compiled lane left `x` three deep
// after `[1 2 3] each [def x 5]`; before the var mark moved, `var x 0 [1 2]
// each [var x 5] x add 1` compiled 1 for the interpreter's 6.
func TestCallbackBodyBlockOnTheVM(t *testing.T) {
	for _, src := range []string{
		`[1 2 3] each [def x 5]`,
		`fold [ def x 5 ] [10 20] 0`,
		`[1 2] each [ ([r] => [def x r x]) apply ]`,
	} {
		interp, compiled := mustNew(t), mustNew(t)
		if _, err := interp.RunInterp(src); err != nil {
			t.Fatalf("%q: interpreter: %v", src, err)
		}
		if _, ran, err := compiled.RunCompiled(src); err != nil || !ran {
			t.Fatalf("%q: compiled run (ran=%v): %v", src, ran, err)
		}
		for lane, b := range map[string]*Boru{"interp": interp, "compiled": compiled} {
			if got, err := b.RunInterp("x"); err == nil || !strings.Contains(err.Error(), "undefined word: x") {
				t.Errorf("%q: %s instance still binds x after the program: %v / %v", src, lane, got, err)
			}
		}
	}
	for _, c := range []struct{ src, want string }{
		{`var x 0 [1 2] each [var x 5] x add 1`, "[[1 2] 6]"},
		{`var x 0 fold [ var x 5 ] [10 20] 0  x add 1`, "[20 6]"},
		{`var x 0 [1 2] each [var x (x add 1)] x`, "[[1 2] 2]"},
		// (The typed form, `var n:Integer 0 … [var n (n add 1)] n`, declines
		// compiled on this shape and on a `for` body alike — "check-mode
		// suppressed a runtime error", measured on the phase-1 commit too:
		// NUR389, not this landing's.)
	} {
		interp, compiled := mustNew(t), mustNew(t)
		gotI, errI := interp.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		gotC, ran, errC := compiled.RunCompiled(c.src)
		if errC != nil || !ran || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled %v / %v (ran=%v), want %s", c.src, gotC, errC, ran, c.want)
		}
		// One cell on both lanes on the next request: the assignment
		// replaced the declaration's cell, it did not declare beside it.
		for lane, b := range map[string]*Boru{"interp": interp, "compiled": compiled} {
			var stack []string
			for i := 0; i < 4; i++ {
				out, err := b.RunInterp("x")
				if err != nil {
					break
				}
				stack = append(stack, fmt.Sprint(out))
				if _, err := b.RunInterp("undef x"); err != nil {
					t.Fatalf("%q: %s undef x: %v", c.src, lane, err)
				}
			}
			if c.src[4] == 'x' && len(stack) != 1 {
				t.Errorf("%q: %s instance's install stack of x after the program is %v, want one cell", c.src, lane, stack)
			}
		}
	}
}

// TestVarDeclarationInstallsAVarCellOnTheVM pins the compiled lane's var
// DECLARATION installs: a fn frame's `var t 0` that a hosted body assigns
// lowers to BIND_DYN_SCOPE_VAR (the cell marked a var, core InstallVar), so
// the body's `var t (t add 1)` replaces it and the frame's read after the
// body sees the sum on both lanes. Before the opcode the frame's install was
// a plain def's, and the body's assignment declared a second cell the body's
// block then retired: compiled 0 for the interpreter's 3.
func TestVarDeclarationInstallsAVarCellOnTheVM(t *testing.T) {
	src := `def f fn [[xs:List][Integer][var t 0 each [var t (t add 1)] xs drop t]] f [1 2 3]`
	if dis := compileDisasm(t, src); !strings.Contains(dis, "BIND_DYN_SCOPE_VAR") {
		t.Errorf("the frame's var declaration does not lower to BIND_DYN_SCOPE_VAR:\n%s", dis)
	}
	agreeOnBothLanes(t, src, "[3]")
	// (The typed declaration, `var t:Integer 0`, declines compiled on this
	// shape — NUR389.)
	agreeOnBothLanes(t, `def f fn [[b:List xs:List][Integer][var t 0 each b xs drop t]] f (quote [var t (t add 1) t]) [1 2 3]`, "[3]")
}

// ruleOrDecline asserts the interpreter's answer under the block rule (a
// value list, or "ERROR:<substring>") and, on the compiled lane, either
// parity or a DECLINE — a compile failure, for whatever reason, counted in
// the compile-defect ledger — never a wrong answer. It is the pin for the
// NUR regression tests whose programs the rule re-reads: an arm's or a
// body's def read after it is the block's own now, so the compiler
// machinery those tests exercised (arm installs, speculative fn families,
// split rebinds) is mooted, and phase 4 retires it (design/IMMUTABLE-
// DEF.1.md §5). A row that compiles must agree; one that declines is
// booked; a wrong answer or a run-time bail fails the row.
func ruleOrDecline(t *testing.T, src, interpWant string) {
	t.Helper()
	gotI, errI := mustNew(t).RunInterp(src)
	if sub, isErr := strings.CutPrefix(interpWant, "ERROR:"); isErr {
		if errI == nil || !strings.Contains(errI.Error(), sub) {
			t.Errorf("%q: interpreter %v / %v, want an error containing %q", src, gotI, errI, sub)
		}
	} else if errI != nil || fmt.Sprint(gotI) != interpWant {
		t.Errorf("%q: interpreter %v / %v, want %s", src, gotI, errI, interpWant)
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if !compiled {
		requireCompileDefect(t, src, gotC, errC)
		return
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || firstErrLine(errC) != firstErrLine(errI) {
		t.Errorf("%q: compiled %v / %v, interpreter %v / %v", src, gotC, errC, gotI, errI)
	}
}
