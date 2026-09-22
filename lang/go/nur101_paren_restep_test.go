package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestParenReStepRule is the standing measurement behind
// design/PAREN-RESTEP-RULE.0.md: for every shape the rule classifies, the
// compiled lane either AGREES with the tree-walking interpreter or DECLINES.
// It never answers differently.
//
// The rule: a Function a paren PLACED is re-stepped into a CALL exactly when
// it leads two or more survivors of an enclosing group that closes with a
// paren rewind — a user paren, an fn frame, or an `if` / `for` / `do` body.
// The program top level, list literals and map literals do not rewind.
//
// Five rows here were SILENT MISCOMPILES before 2026-08-27, in both
// directions, and all five were invisible to the suite because the tests that
// covered them used `Run` as their interpreter oracle (NUR106).
func TestParenReStepRule(t *testing.T) {
	const mk = `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] `
	const inc = `def inc fn [[n:Integer] [Integer] [n add 1]] `

	for _, c := range []struct {
		src  string
		want string // the INTERPRETER's answer — the only oracle
		why  string
	}{
		{mk + `(mk 1) 2`, "[fn (Integer) 2]", "program residual: nothing rewinds, so the carrier is placed"},
		{mk + `((mk 1) 2)`, "[3]", "the outer paren rewinds onto the carrier and dispatches it"},
		{mk + `[(mk 1) 2]`, "[[fn (Integer) 2]]", "a list literal does not rewind: two elements"},
		{mk + `[((mk 1) 2)]`, "[[3]]", "the inner paren rewinds: one element (declined — see below)"},
		{mk + `if true [(mk 1) 2]`, "[3]", "the arm's frame rewinds"},
		{mk + `for 2 [(mk 1) 2]`, "[3 3]", "the loop body's frame rewinds, once per iteration"},
		{mk + `do [(mk 1) 2]`, "[3]", "the do body's frame rewinds"},
		{mk + `case 1 [1] [(mk 1) 2]`, "[1 [fn (Integer) 2]]", "a case arm is a literal, not a frame"},
		{inc + `(inc/v) 7`, "[fn inc(Integer) 7]", "one survivor: a reference places, per NUR073 BROAD"},
		{inc + `((inc/v) 7)`, "[8]", "two survivors: the outer paren dispatches the placed reference"},
	} {
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil {
			t.Errorf("%q: interp errored: %v", c.src, errI)
			continue
		}
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp = %v, want %s (%s)", c.src, gotI, c.want, c.why)
			continue
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled {
			continue // a compile failure is sound; the per-shape fences below pin which ones
		}
		if errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: DIVERGENCE — compiled=%v (%v) interp=%v (%s)", c.src, gotC, errC, gotI, c.why)
		}
	}
}

// TestParenReStepPlacedLayoutCompiles ratchets what the placed record BUYS,
// not just what it prevents. A residual whose lead a user paren placed and no
// enclosing paren re-stepped is inert on both lanes, so the layout may simply
// lay it out — where before it declined, reading the absence of a record as
// evidence of a hazard.
//
// These graduated 2026-08-27 with Stage 3's first increment. Pinned as
// POSITIVE so the coverage cannot quietly regress to a compile failure: the rule test
// above tolerates compile failures by design, which is what makes it safe to extend
// and useless as a ratchet.
func TestParenReStepPlacedLayoutCompiles(t *testing.T) {
	for _, c := range []struct{ src, want, why string }{
		{`def inc fn [[n:Integer] [Integer] [n add 1]] (inc/v) 7`,
			"[fn inc(Integer) 7]",
			"a CONCRETE fn value placed by a user paren — the record's fourth shape"},
		{`def tbl {double:(a:Integer => [mul 2 a])} def k 'double' (tbl get k) 5`,
			"[fn (Integer) 5]",
			"a DYNAMIC maybe-callable placed by a user paren"},
		{`def f fn [[x:Any] [Any] [x]] end def m {p: f/v} end (m.p 5) (m.p 7)`,
			"[5 7]",
			"the graduated NUR038 seal row: each paren applies, and two placed results are not a hazard"},
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: REGRESSED to a compile failure (%s) — %s", c.src, reason, c.why)
			continue
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil || errI != nil {
			t.Errorf("%q: compiled=%v errC=%v errI=%v", c.src, compiled, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v, want %s on both — %s", c.src, gotC, gotI, c.want, c.why)
		}
	}
}

// TestForeignClosureCompilesInItsOwnRegistry pins the cross-registry closure
// unit (Stage 3, §6.3): a fn value from another module reaching a native
// callback slot compiles NATIVELY and resolves its free words where it was
// written.
//
// This row islanded until 2026-08-27 — one `FALLBACK` instruction for the
// whole `filter` call, the right answer by the mechanism the mission forbids.
// The lowering declined because compiling the body against the CALLER bakes
// the caller's `lim` (100) and answers `[]`.
//
// Both halves of the fix are load-bearing, and the negative half is why this
// test asserts the DISASSEMBLY and not just the value:
//
//   - compile against the DEFINING registry, so `lim` is module A's (2), and
//     StartFnCompile stamps CompiledFn.Reg for the VM's curReg swap;
//   - share the CALLER's CheckState onto it, so params, carriers and the
//     recorder stay local and the unit lands in the caller's program.
//
// With the registry alone the body const-folds to `PUSH_CONST false; RET` —
// a silent wrong answer. A value-only assertion would pass the day someone
// re-islands this, so the island check is the point.
func TestForeignClosureCompilesInItsOwnRegistry(t *testing.T) {
	const src = `import module [def lim fn [[n:Integer] [Integer] [2]] def big fn [[e:Map] [Boolean] [(e dot value) gt (lim 0)]] export "A" {big: big/v}] end def lim fn [[n:Integer] [Integer] [100]] filter A.big [1 2 3 4]`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("foreign closure did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("foreign closure ISLANDED — the compile is the point, not the answer:\n%s", dis)
	}
	if !strings.Contains(dis, "PUSH_CLOSURE") {
		t.Errorf("expected a closure unit for the foreign predicate:\n%s", dis)
	}
	if strings.Contains(dis, "false (Boolean)") {
		t.Errorf("the predicate CONST-FOLDED — the caller's CheckState is not shared:\n%s", dis)
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil || errI != nil {
		t.Fatalf("run: compiled=%v errC=%v errI=%v", compiled, errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[[3 4]]" {
		t.Errorf("compiled=%v interp=%v, want [[3 4]] on both — `lim` must be module A's 2, "+
			"not the caller's 100 (which answers [[]])", gotC, gotI)
	}
}

// TestForeignClosureCaptureResolvesInItsOwnRegistry is the negative half of
// the cross-registry closure unit, and it caught a SIXTH silent miscompile in
// the same family as NUR101's five.
//
// A foreign body's MODULE-SCOPE mutable captures — a flex cell, a class
// instance, anything moduleScopeMutableCaptures rides as a closure slot — must
// be looked up in the registry that WROTE the body, exactly like its free
// words. Look them up in the CALLER and a name collision silently swaps the
// cell: here both modules bind `acc`, module A's holds three elements and the
// caller's is empty, so the predicate keeps only 4 interpreted and everything
// compiled.
//
// Measured, on a build with the lookup against the caller:
//
//	compiled [[1 2 3 4]]   interpreted [[4]]
//	0007 PUSH_CLOSURE f0   ; closure filter$body/2   <- capturing l0, the
//	                                                    CALLER's flex cell
//
// With the lookup in fd.Registry the capture is module A's cell, which has no
// producing event in the CALLER's emit tables, so resolveOperand declines and
// the call falls back — sound, and the answers agree. Compiling it needs a
// registry-tagged operand for a foreign module-scope instance (the follow-up
// the frontier ledger names); the fence here is parity, which holds either way.
func TestForeignClosureCaptureResolvesInItsOwnRegistry(t *testing.T) {
	const src = `import module [def acc (flex [1 2 3]) def big fn [[e:Map] [Boolean] [(size acc) lt (e dot value)]] export "A" {big: big/v}] end def acc (flex []) filter A.big [1 2 3 4]`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil || errI != nil {
		t.Fatalf("run: compiled=%v errC=%v errI=%v", compiled, errC, errI)
	}
	if fmt.Sprint(gotI) != "[[4]]" {
		t.Fatalf("interpreter answered %v, want [[4]] — the oracle moved, re-derive this fence", gotI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("compiled=%v interp=%v — the closure captured the CALLER's `acc` (empty) "+
			"instead of module A's (3 elements); moduleScopeMutableCaptures must be looked "+
			"up in fd.Registry for a foreign body", gotC, gotI)
	}
}

// TestListFoldCallbackOrderPin was a FENCE and is now a GRADUATION PIN, and it
// asserts the DISASSEMBLY as well as the value — deliberately. Its parity half
// passed for the whole time list fold/scan ISLANDED, because an island answers
// with the interpreter: a value-only assertion here proves nothing and would go
// on passing the day someone re-islands the callback.
//
// THE CONVENTION. A MAP fold takes (accumulator, entry); a LIST fold takes
// (element, accumulator). Measured with AMBIGUOUS param types —
// `fold ([x:Any y:Any] => [x]) {a:1} 0` answers the seed and
// `fold ([x:Any y:Any] => [x]) [7] 0` answers the element — so it is a real
// ordering convention, not a by-type assignment that resembles one.
//
// WHY THEY DIFFER, which is the part that took three readings to get right.
// BOTH handlers hand (accumulator, element). The MAP path calls the lambda
// POSITIONALLY (mapBody.callLambda -> InvokeCallbackFn, args bound in sig order), so
// sig[0] is the accumulator. The LIST path goes through InvokeBody, whose
// interpreter arm runs the inputs as a STACK (RunResolved) where
// MatchSignature fills from the TOP DOWN, so sig[0] is the element. Same order
// in, opposite assignment out — one path is positional, the other is a stack.
//
// A compiled closure is positional like the map path, so the list pair arrived
// swapped (123 interpreted, 60 positional) and the lowering declined, leaving
// the whole call an island. ClosureInStackPair reverses at the bind, and these
// compile native.
//
// Carriers were never the lever: swapping the carrier order changed nothing,
// because carriers TYPE the body while the slot each input lands in comes from
// the handler's push order. The rows below use SAME-TYPED params on purpose —
// a typed probe cannot see positional order at all, since the matcher
// reassigns by type, and that is what made the first reading wrong.
func TestListFoldCallbackOrderPin(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`fold ([e:Integer a:Integer] => [a mul 10 add e]) [1 2 3] 0`, "[123]"},
		{`scan ([e:Integer a:Integer] => [a mul 10 add e]) [1 2 3]`, "[[1 12 123]]"},
		{`def f fn [[e:Integer a:Integer] [Integer] [a mul 10 add e]]  fold f/v [1 2 3] 0`, "[123]"},
		{`def n 2  fold ([e:Integer a:Integer] => [a add (e mul n)]) [1 2 3] 0`, "[12]"},
		// The unseeded twin: the first element seeds, so both slots carry the
		// element type rather than a separate seed type.
		{`def f fn [[e:Integer a:Integer] [Integer] [a mul 10 add e]]  fold f/v [1 2 3]`, "[123]"},
		// The MAP twin, unchanged and here as the control: it agreed before
		// this work and must still, so a permutation applied to the wrong
		// container fails loudly instead of quietly.
		{`fold ([a:Integer kv:KeyVal] => [a mul 10 add (kv.v)]) {x:1 y:2 z:3} 0`, "[123]"},
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(tc.src)
		if cerr != nil || prog == nil {
			t.Fatalf("%s: declined: %s (err %v)", tc.src, reason, cerr)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%s: the callback ISLANDED — the parity below would pass anyway, "+
				"because an island answers with the interpreter:\n%s", tc.src, prog.Disassemble())
		}
		gotC, ran, errC := mustNew(t).RunCompiled(tc.src)
		gotI, errI := mustNew(t).RunInterp(tc.src)
		if noteCompileDefect(t, tc.src, gotC, errC) {
			continue
		}
		if !ran || errC != nil || errI != nil {
			t.Fatalf("%s: ran=%v errC=%v errI=%v", tc.src, ran, errC, errI)
		}
		if fmt.Sprint(gotI) != tc.want {
			t.Fatalf("%s: interpreter answered %v, want %s — the oracle moved, re-derive this pin",
				tc.src, gotI, tc.want)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled=%v interp=%v — the callback's (accumulator, element) pair "+
				"arrived SWAPPED. The compiled closure binds positionally; the interpreter's "+
				"list path fills from the stack top down. ClosureInStackPair is what reconciles "+
				"them, and it is per-word: check the shape this word's lowering records",
				tc.src, gotC, gotI)
		}
	}
}

// TestParenReStepListElementCompileFailure pins the one shape in the rule table that
// the compiler DECLINES rather than answers, and why the compile failure is not the
// lazy reading.
//
// `[((mk 1) 2)]` is `[3]` interpreted: the inner paren leaves two survivors,
// the park declines, and the rewind dispatches the carrier. It compiled to
// `[[fn (Integer) 2]]` until 2026-08-27 — NUR101's original symptom, silent,
// exit 0, `boru check` clean.
//
// It cannot be fixed by testing the lead's SHAPE, because `[(mk 1) 2]` reaches
// RecordMakeList with byte-identical `[carrier, 2]` elements and really is two
// elements on both lanes. Only the re-step record taken at the collapse
// separates them — which is why that row above still compiles and this one
// declines.
//
// GRADUATED 2026-09-22 (the curried chain): the inner paren records its
// re-stepped produced lead's apply at the collapse, so the list assembles
// ONE element — the event's result — and the re-step guard in
// RecordMakeListInner never meets the pair. A parity row since.
func TestParenReStepListElementCompileFailure(t *testing.T) {
	const src = `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] [((mk 1) 2)]`
	mustCompileWithParity(t, src, "[[3]]")

	// The twin that MUST keep compiling: no inner rewind, so two elements.
	const placed = `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] [(mk 1) 2]`
	gotC, compiled, errC := mustNew(t).RunCompiled(placed)
	gotI, errI := mustNew(t).RunInterp(placed)
	if noteCompileDefect(t, placed, gotC, errC) {
		return
	}
	if !compiled || errC != nil || errI != nil {
		t.Fatalf("placed list twin: compiled=%v errC=%v errI=%v", compiled, errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[[fn (Integer) 2]]" {
		t.Errorf("placed list twin: compiled=%v interp=%v, want [[fn (Integer) 2]] on both", gotC, gotI)
	}
}
