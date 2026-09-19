// Curated bytecode combination matrix (design/legacy/boru-bytecode-plan.0.ignore
// Stage 6 follow-on; the regression net for the residual work). Rather
// than a generator over the full Cartesian product, this is a curated
// set of high-value pairwise/triple feature combinations plus "stranger"
// cases that are regression magnets — nested/chained islands, islands in
// loops with varying threaded inputs, closures in loops, generics
// feeding islands, deep/mutual tail recursion, F4 dynamic-dispatch
// chains, type-operand islands, def reassignment, and every call form ×
// island.
//
// Two assertions per case where applicable:
//   - PARITY: RunCompiled (compiling what it can, falling back otherwise)
//     matches the interpreter on value AND error taxonomy. This is the
//     core soundness guarantee, extended to combinations the .tsv corpus
//     does not reach.
//   - PATH: for a subset, the COMPILATION decision is pinned — a program
//     must island, must lower to pure CALL_NATIVE, or must fall back —
//     so a regression that silently changes the path is caught.
package langspec

import (
	"strings"
	"testing"
)

// comboParity is the curated parity matrix: every row must produce the
// same value and the same error taxonomy through both engines.
var comboParity = []string{
	// --- literal kind × call form (forward / stack / swap / paren) ---
	`add 1 2`, `1 add 2`, `1 2 add`, `add (mul 2 3) 4`,
	`sub 10 3`, `10 sub 3`, `3 10 sub`,
	`mul 2.5 4`, `mul 2 4.0`, // int/float contagion
	`add 'a' 'b'`, // string concat
	`and true false`, `or true false`, `not true`,
	`cmp 1 2`, `eq 1 1`, `lt 1 2`, `gte 3 3`,
	`size [1 2 3]`, `size {a:1 b:2}`,

	// --- control flow × arithmetic ---
	`if (gt 5 2) [10] [20]`, `if (lt 5 2) [10] [20]`,
	`if [1 lt 2] [10] [20]`, // list-form condition
	`if (gt 5 2) [99]`,      // 2-arg if
	`for 5 [i]`, `for 5 [mul i 2]`, `for [2,6] [i]`, `for [6,2,-1] [i]`,
	`for 5 [if (gt i 2) [mul i 10] [i]]`,
	`for 6 [if (eq i 3) [break] [i]]`,
	`for 6 [if (eq i 3) [continue] [i]]`,
	`add 100 (for 4 [add i 1])`, // loop residual consumed? (refuses -> fallback, parity holds)

	// --- user fns: simple / recursive / tail / mutual / closure / generic ---
	`def db fn [[n:Integer] [Integer] [n mul 2]] db 21`,
	`def fac fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fac (n sub 1))]]] fac 6`,
	`def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 100 0`,
	`def od fn [[n:Integer] [Boolean] [if (n eq 0) [false] [ev (n sub 1)]]] def ev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [od (n sub 1)]]] ev 50`,
	`def mk fn [[x:Integer] [Function] [([y:Integer] => [x add y])]] def a5 (mk 5) a5 10`,
	`def idg gen [(T extends Any)] fn [[x:T] [T] [x]] end idg 7`,

	// --- islands: code-body higher-order, baked + threaded ---
	`each [mul 2] [1 2 3]`, `[1 2 3] each [mul 2]`,
	`fold [add] [1 2 3 4] 0`, `scan [add] [1 2 3]`,
	`each [mul 2] (iota 4)`, `scan [add] (iota 4)`, // threaded receiver
	`filter [gt 2] [1 2 3 4]`, `do [add 1 2]`,
	`add 100 (do [mul 6 7])`, `do [1 2 3]`, // single-result do → closure; multi-result do → island (parity holds)
	`each [add 1] (each [mul 2] (iota 3))`, // nested + threaded
	`size (each [mul 2] [1 2 3])`,          // F4 over island result
	`typeof (fold [add] [1 2 3] 0)`,
	`(each [mul 2] [1 2 3]) is List`,
	`flex (each [mul 2] [1 2 3])`,

	// --- islands in loops with varying threaded inputs (reuse safety) ---
	`for 4 [each [mul 2] (iota i)]`,
	`for 3 [size (each [add 1] (iota i))]`,
	`for 3 [do [add 1 2]]`,
	`for 4 [fold [add] (iota i) 100]`,

	// --- closures captured in a loop ---
	`def mk fn [[x:Integer] [Function] [([y:Integer] => [x add y])]] for 3 [(mk i) 10]`,

	// --- code-body closures capturing an enclosing fn's param (plan P2) ---
	`def f fn [[k:Integer] [List] [each [add k] [1 2 3]]] f 10`,
	`def g fn [[k:Integer] [Integer] [fold [add] [1 2 3] k]] g 100`,
	`def h fn [[m:Integer] [List] [each [mul m] (iota 3)]] h 5`,
	`def n 10 each [add n] [1 2 3]`, // concrete module-def bakes as a const

	// --- generics feeding islands ---
	`def idg gen [(T extends Any)] fn [[x:T] [T] [x]] end each [idg] [1 2 3]`,

	// --- def reassignment around islands ---
	`def n 10 each [add 1] [1 2 3] def n 20 each [add 1] [4 5 6]`,

	// --- F4 / type algebra ---
	`is 5 Integer`, `typeof 5`, `typeof 'x'`, `typeof [1 2]`,
	`is 'x' Integer`, // false
	`teq Integer Integer`, `tcmp Integer String`,

	// --- map / record access ---
	`def m {a: {b: {c: 7}}} m.a.b.c`,
	`{a:1 b:2} dot a`,

	// --- F4: integer-keyed get on a dynamic receiver → CALL_NATIVE_POLY
	// (runtime sig match); atom-keyed field get stays islanded ---
	`(flex [10 20 30]) get 1`,
	`(each [add 1] [1 2 3]) get 0`,
	`get 1 (flex [5 6 7])`,
	`def xs [10 20 30] xs get 2`,
	`(flex [1 2 3]) is FlexList`, // typed query on a dynamic result via poly
	// dynamic-INPUT poly: a builtin native over a dynamic operand re-matches
	// its signature at run time (plan P3/P4 widening) instead of refusing.
	`add (do [add 1 2]) 10`,
	`mul 2 (do [mul 2 3])`,
	`size (do [iota 5])`,
	// fn-value-call boundary: a map field that is a method, applied to
	// trailing args — the interpreter auto-applies; compiled must fall
	// back (parity holds, value matches).
	`import "boru:rand" def r (Rand.with-seed 42) r.int 0 100`,
	// CALL_DYNAMIC (plan P4): a fn-value applied to trailing args. A dynamic
	// value that is NOT callable leaves value+args as the residual (parity).
	`(do [iota 3]) 5`,

	// --- F4: class/object `make` — a plain-data class compiles + chains;
	// a class whose computed defaults const-fold (incl. a method field) now
	// compiles too; either way value/error parity holds ---
	`def Point class {x:1, y:2} make Point {x:9}`,
	`def Point class {x:1} (make Point {}) typeof`,
	`def Point class {x:1} (make Point {}) dot x`,
	`def Point class {x:1, y:2} def p (make Point {x:9}) p.y`,
	`def Point class {x:1} (make Point {}) is Point`,
	`def C class {x:1 f:(fn [[][Integer][2]])} (make C {x:5}) dot x`, // method field → fallback

	// --- F4: refine `make` compiles native; predicate/dependent make
	// errors identically in both engines (parity via fallback) ---
	`def Pos refine Integer make Pos 5`,
	`def Big (Integer gt 10) make Big 20`,          // make rejects non-type-literal → error parity
	`def R {x:Integer y:Integer} make R {x:1 y:2}`, // map-shape binding → error parity

	// --- F4: fn-value apply sites — value + error parity via fallback
	// (the Stage-3 dynamic-dispatch frontier, conservatively deferred) ---
	`def m {f:(fn [[a:Integer][Integer][a mul 2]])} (m dot f) 5`,

	// --- error rows: same taxonomy in both engines ---
	`add 1 'x'`, // type_error
	`def f fn [[n:Integer] [String] [n]] f 1`,      // return type_error
	`def r2 fn [[n:Integer] [Integer] [n n]] r2 1`, // return count
	`def m {x:1} unpack [y] m`,                     // unpack_error
	`gen [T]`,                                      // gen_without_constructor
	`size 5`,                                       // no_signature/type
}

func TestCompiledCombinationParity(t *testing.T) {
	t.Parallel()
	var diverge int
	for _, src := range comboParity {
		ac := newDifferentialInstance(t)
		gotC, _, errC := ac.RunCompiled(src)
		// A program that does not compile, or compiles and then bails, has
		// no compiled answer to hold beside the interpreter's. This harness
		// used to perform the fallback itself so it had one; both classes
		// are booked as the defects they are instead.
		if errCode(errC) == "compile_failed" || compiledDefect(t, "combo:"+src, src, errC) {
			continue
		}
		ai := newDifferentialInstance(t)
		gotI, errI := ai.RunInterp(src)

		if cdC, cdI := errCode(errC), errCode(errI); cdC != cdI {
			diverge++
			t.Errorf("%q: error divergence compiled=[%s]%v interp=[%s]%v", src, cdC, errC, cdI, errI)
			continue
		}
		if errC != nil {
			continue
		}
		if renderAny(gotC) != renderAny(gotI) {
			diverge++
			t.Errorf("%q: value divergence compiled=%q interp=%q", src, renderAny(gotC), renderAny(gotI))
		}
	}
	t.Logf("combination parity: %d cases, %d divergences", len(comboParity), diverge)
}

// pathOf classifies how a program compiles: "fallback" (whole-program),
// "island" (compiles with an OpFallback island), or "native" (compiles,
// no island).
func pathOf(t testing.TB, src string) string {
	t.Helper()
	a := newDifferentialInstance(t)
	prog, _, _, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		return "fallback"
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		return "island"
	}
	return "native"
}

// TestCompiledCombinationPath pins the compilation DECISION for
// representative shapes, so a regression that silently changes the path
// (e.g. islanding a concrete dispatch, or refusing a shape that used to
// compile) is caught even when the result stays correct.
func TestCompiledCombinationPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src  string
		want string
	}{
		// Straight-line native compute — pure CALL_NATIVE, never an island.
		{`add 1 (mul 2 3)`, "native"},
		{`size [1 2 3]`, "native"},
		{`for 5 [mul i 2]`, "native"},
		{`def db fn [[n:Integer] [Integer] [n mul 2]] db 21`, "native"},
		// Code-body higher-order words island.
		// Code-body higher-order words compile their body to a CLOSURE unit
		// and run it through the VM (plan P2) — pure CALL_NATIVE, no island.
		{`each [mul 2] [1 2 3]`, "native"},
		{`each [mul 2] (iota 4)`, "native"}, // threaded data
		{`do [add 1 2]`, "native"},          // single-result do body → closure
		// F4 over a now-native each result is itself native (no dynamic island).
		{`size (each [mul 2] [1 2 3])`, "native"},
		{`flex (each [mul 2] [1 2 3])`, "native"},
		// F4 INTEGER-keyed (sequence index) get on a dynamic receiver runtime-
		// dispatches via CALL_NATIVE_POLY (plan P3) — never returns a method.
		{`(flex [10 20 30]) get 1`, "native"},
		{`def xs [10 20 30] xs get 1`, "native"},
		// An ATOM-keyed field get now polys too: a data field is returned
		// directly, a named 0-arg method result is auto-applied VM-native, and
		// a method needing args flows to CALL_DYNAMIC (plan P3+P4).
		{`{a:1 b:2} dot a`, "native"},
		{`def m {a:{b:7}} m.a.b`, "native"},
		// Class `make` with plain-data fields compiles (the body bakes as a
		// const); a class with a method field must fall back.
		{`def Point class {x:1, y:2} make Point {x:9}`, "native"},
		{`def C class {x:1 f:(fn [[][Integer][2]])} make C {}`, "native"}, // computed defaults (incl. a method field) const-fold
		// is / typeof on a make-result: the type operand shares the make
		// result's ID, but the type-operand ID-collision guard resolves the
		// `Point` literal to its own type, not the make event — so it compiles
		// instead of failing stack discipline.
		{`def Point class {x:1} (make Point {}) is Point`, "native"},
		{`def Point class {x:1} (make Point {}) typeof`, "native"},
		// fn-value-call boundary, fully native now: the atom-keyed get polys
		// (the method stays a value, no 0-arg sig), then OpCallDynamic applies
		// it to 0 100 VM-native (a trivial-delegation method) — plan P3+P4.
		{`import "boru:rand" def r (Rand.with-seed 42) r.int 0 100`, "native"},
		// Sentinel inside a closure body must NOT compile (falls back so the
		// interpreter can unwind break/continue across the boundary).
		{`for 3 [each [break] [1 2]]`, "fallback"},
		// A body reading a CONCRETE module-def bakes it as a const (native).
		{`def n 10 each [add n] [1 2 3]`, "native"},
		// A higher-order body capturing an enclosing fn's param compiles as a
		// closure that threads the capture (plan P2 closure-capture).
		{`def f fn [[k:Integer] [List] [each [add k] [1 2 3]]] f 10`, "native"},
		{`def g fn [[k:Integer] [Integer] [fold [add] [1 2 3] k]] g 7`, "native"},
	}
	for _, c := range cases {
		if got := pathOf(t, c.src); got != c.want {
			t.Errorf("%q: compilation path = %s, want %s", c.src, got, c.want)
		}
	}
}
