package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestModuleBindInLoopOrArmDeclines pins NUR205's close. An `import` is a
// compile-time word: the check pass runs it, and the compiled program
// replays the one bind it made at the construct's position (the loop's or
// the branch's join twin). Wherever that replay is not the interpreter's
// bind the join's twin now keeps no placement — noted with the recorder
// suspended — so the program DECLINES at the twin regime's full-placement
// gate, and the interpreter answers:
//
//   - an inline `import module […]` inside a loop body runs its body again
//     per iteration on the interpreter, so state it mints is fresh each
//     time — `[[1] 1 [1] 1]` for the compiled lane's shared
//     `[[1 1] 1 [1 1] 2]`;
//   - a loop that may run zero times, and a branch arm that may not run,
//     leave the name UNBOUND on the interpreter (`undefined_word`), where
//     the replay bound it anyway.
func TestModuleBindInLoopOrArmDeclines(t *testing.T) {
	for _, tc := range []struct{ src, interp string }{
		{`for 2 [import module [def acc (flex []) export "M" {acc: acc}] end M.acc push 1 end size M.acc]`, "[[1] 1 [1] 1]"},
		{`for 2 [import module [def a 1 export "M" {a: a}] end M.a]`, "[1 1]"},
		{`def n (0 add 0) end for n [import module [def a 1 export "M" {a: a}] end] M.a`, "undefined_word"},
		{`while [false] [import module [def a 1 export "M" {a: a}] end] M.a`, "undefined_word"},
		{`def c (1 gt 2) end if c [import module [def a 1 export "M" {a: a}] end] [] M.a`, "undefined_word"},
		{`def c (1 gt 2) end if c [import "boru:math-util" end] [] MathUtil.$name`, "undefined_word"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(tc.src)
		// The one twin the replay cannot stand for keeps no placement, so
		// the program declines at the twin regime's full-placement gate.
		if !noteCompileDefect(t, tc.src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), "twin regime") {
			t.Errorf("%q: want the twin regime's decline, got %v compiled=%v err=%v", tc.src, gotC, compiled, errC)
		}
		gotI, errI := mustNew(t).RunInterp(tc.src)
		if got := fmt.Sprint(gotI); errI != nil {
			got = codeOf(errI)
			if got != tc.interp {
				t.Errorf("%q: interp raised %v, want %s", tc.src, errI, tc.interp)
			}
		} else if got != tc.interp {
			t.Errorf("%q: interp %s, want %s", tc.src, got, tc.interp)
		}
	}
}

// TestModuleBindReplayStandsWhereItIsTheBind pins NUR205's edges: where
// the one replay IS the interpreter's bind, the program still compiles and
// agrees — a module the loader caches (a `boru:` import answers the same
// instance every time) inside a loop that provably runs, and the arm a
// decided condition takes.
func TestModuleBindReplayStandsWhereItIsTheBind(t *testing.T) {
	for _, src := range []string{
		`for 2 [import "boru:math-util" end MathUtil.$name]`,
		`if true [import "boru:math-util" end MathUtil.$name] [0]`,
		`if false [0] [import module [def a 1 export "M" {a: a}] end M.a]`,
	} {
		requireEngineParity(t, src, true)
	}
}
