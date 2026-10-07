package lang

import "testing"

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
	// Since phase 2 (design/IMMUTABLE-DEF.1.md §2.1) the loop body and the
	// arm are BLOCKS: the module an inline `import` binds inside them ends
	// with the body's run, so a read after a loop that ran zero times or an
	// arm that did not run is the interpreter's undefined_word, and the
	// compiled lane stops at the check. A RUN-TIME read of the module inside
	// the body — a flex export the body mutates is read live — declines at
	// the lowerer (core.NoteBlockImport → blockImportRead): the check pass's
	// install is the only one the compiled program has, and the block retired
	// it, where the lookup bailed `dynamic-scope read miss` before. The
	// compiler's block scopes land with phase 2's second step.
	requireBlockRule(t, `for 2 [import module [def acc (flex []) export "M" {acc: acc}] end M.acc push 1 end size M.acc]`, "[[1] 1 [1] 1]", "block-local import `M` read at run time")
	requireBlockRule(t, `if true [import module [def acc (flex []) export "M" {acc: acc}] end size M.acc] [0]`, "[0]", "block-local import `M` read at run time")
	for _, tc := range []struct{ src, name string }{
		{`def n (0 add 0) end for n [import module [def a 1 export "M" {a: a}] end] M.a`, "M"},
		{`while [false] [import module [def a 1 export "M" {a: a}] end] M.a`, "M"},
		{`def c (1 gt 2) end if c [import module [def a 1 export "M" {a: a}] end] [] M.a`, "M"},
		{`def c (1 gt 2) end if c [import "boru:math-util" end] [] MathUtil.$name`, "MathUtil"},
	} {
		requireBlockRule(t, tc.src, "ERROR:undefined word: "+tc.name, "check diagnostics")
	}
}

// TestModuleBindReplayStandsWhereItIsTheBind pins NUR205's edges: where
// the check pass's read of the module FOLDS — a cached `boru:` import's
// constant, a constant export — the program needs no run-time binding and
// compiles with parity inside a loop body or an arm too.
func TestModuleBindReplayStandsWhereItIsTheBind(t *testing.T) {
	for _, src := range []string{
		`for 2 [import "boru:math-util" end MathUtil.$name]`,
		`if true [import "boru:math-util" end MathUtil.$name] [0]`,
		`if false [0] [import module [def a 1 export "M" {a: a}] end M.a]`,
		`if true [import module [def a 1 export "M" {a: a}] end M.a add 1] [0]`,
	} {
		requireEngineParity(t, src, true)
	}
	ruleOrDecline(t, `for 2 [import module [def a 1 export "M" {a: a}] end M.a]`, "[1 1]")
}

// TestModuleBindOutsideABlockCompiles pins the note's edge: an import at
// module level or inside a FN body (a frame, not a block) is untouched —
// the check pass's install stands for the program's read, mutable module
// state included.
func TestModuleBindOutsideABlockCompiles(t *testing.T) {
	for _, src := range []string{
		`import module [def acc (flex []) export "M" {acc: acc}] end M.acc push 1 end size M.acc`,
		`import "boru:math-util" end MathUtil.$name`,
		`def f fn [[][Any][import module [def a 1 export "M" {a: a}] end M.a]] end f`,
	} {
		requireEngineParity(t, src, true)
	}
}
