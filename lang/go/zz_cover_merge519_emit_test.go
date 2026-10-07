package lang

import (
	"fmt"
	"strings"
	"testing"
)

// zz_cover_merge519_emit_test.go reaches the compiler arms the merged
// ADR-008 profile left uncovered on the #519 merge (compiler/go emit.go,
// bytecode.go, callable_words.go, unit_memo.go), each through a program whose
// compiled answer is held to the interpreter's (RunInterp is the oracle).

// TestMerge519RootResidualReadGuards: a root read of a gradual def-bound
// value that the program RESIDUAL holds (NUR207, seatRootResidualReads) is an
// island only when the interpreter's state at the read is the laid-out
// residual. A root event written after the read (`def zz 8`), or a second
// read an event consumes (`def k j`), leaves no island: the read is a GUARD.
//   - Leading the residual's dynamic apply over a static window, the guard
//     bails on a no-match alone (NoMatchOnly), and the disassembly says so.
//   - Otherwise it reads the value's own promoted slot.
//
// Data at run time passes through the guard; a fn whose window matches is
// the apply's own answer; a fn the window does not match is the designed
// loud defer, never a different answer.
func TestMerge519RootResidualReadGuards(t *testing.T) {
	const (
		mk7 = `def mk fn [[][Any][7]] end def j (mk) end `
		idf = `def mk fn [[x:Any][Any][x/v]] end `
	)
	const noMatchGuard = "bail if the read holds a fn its window does not match (guard)"
	for _, c := range []struct {
		src, want   string
		noMatchOnly bool
	}{
		// A root event after the read: no island (rootResidualIsland's
		// later-event arm), the leading guard is NoMatchOnly.
		{mk7 + `j 5 def zz 8`, "[7 5]", true},
		{idf + `def r (mk 7) end r 5 def zz 8`, "[7 5]", true},
		{idf + `def lam ([n:Integer] => [n add 1]) end def r (mk lam/v) end r 5 def zz 8`, "[6]", true},
		// A second read a def consumes: the residual holds one of the two
		// reads (rootResidualIsland's read-count arm).
		{mk7 + `j 5 def k j end`, "[7 5]", true},
		// ... and read back at the end, the residual read is no longer the
		// apply's lead: the guard reads the promoted slot.
		{mk7 + `j 5 def k j end k`, "[7 5 7]", false},
	} {
		agreeOnBothLanes(t, c.src, c.want)
		if dis := compileDisasm(t, c.src); strings.Contains(dis, noMatchGuard) != c.noMatchOnly {
			t.Errorf("%s: want the no-match-only guard %v, got:\n%s", c.src, c.noMatchOnly, dis)
		}
	}
	// A fn the window does not match: the interpreter's word dispatch raises
	// its no-match; the compiled guard defers loudly rather than answer.
	requireLoudDefer(t, idf+`def lam ([s:String] => [s]) end def r (mk lam/v) end r 5 def zz 8`,
		"could not re-step (NUR123)", "ERROR:cannot call `r`")
}

// TestMerge519ApplyWindowFitsGradualWindow: a paren-bounded fn-value apply
// whose window cannot be PROVED to fit its lead (NUR246, applyWindowFits) —
// a dynamic argument, or a static argument that is no concrete value at a
// VALUE-pattern parameter — is a variadic region: it fires when the run's
// value matches and parks the window and the fn when it does not, as the
// interpreter does.
func TestMerge519ApplyWindowFitsGradualWindow(t *testing.T) {
	const inc = `def lam ([x:Integer] => [x add 1]) end `
	for _, c := range []struct{ src, want string }{
		// A dynamic (declared-Any) argument.
		{inc + `def mk fn [[][Any][0]] end ((mk) lam/v)`, "[1]"},
		{inc + `def mk fn [[][Any]['s']] end ((mk) lam/v)`, "[s fn lam(Integer)]"},
		// A computed Map at a map-pattern parameter.
		{`def mk fn [[][Map][{a:3}]] end def lam ([x:{a:Integer}] => [x.a]) end ((mk) lam/v)`, "[3]"},
		{`def mk fn [[][Map][{a:'s'}]] end def lam ([x:{a:Integer}] => [x.a]) end ((mk) lam/v)`, "[{a:'s'} fn lam(Map)]"},
		{`def mk fn [[][Map][{a:3}]] end def lam ([x:{a:3}] => [9]) end ((mk) lam/v)`, "[9]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestMerge519RecordTrapUnderAnOptimisticMatch: NUR264's rematch for a trap
// RecordTrap records (not only RecordTrapErr's): `x/u` over a non-fn binding
// is check-lenient and records a terminal illegal_ref trap, met here while an
// OPTIMISTICALLY matched word's data list is evaluated. The trap becomes the
// outer word's rematch: over a live value the word does not take, its
// no-match; over one it takes, the recorded illegal_ref — the full report
// identical to the interpreter's on either path.
func TestMerge519RecordTrapUnderAnOptimisticMatch(t *testing.T) {
	const x = `def x 5 end `
	for _, c := range []struct{ src, word string }{
		{x + `def mk fn [[][Any][5]] end each (mk) [x/u]`, "signature_error"},
		{x + `def mk fn [[][Any][[1 2]]] end each (mk) [x/u]`, "illegal_ref"},
		{x + `def mk fn [[][Any][[1 2]]] end def f fn [[a:List b:List][Any][b]] end f (mk) [x/u]`, "illegal_ref"},
		{x + `def mk fn [[][Any][[1 2]]] end def v (mk) end each v [x/u]`, "illegal_ref"},
		{x + `def mk fn [[][Any][[1 2]]] end each ((mk) dup drop) [x/u]`, "illegal_ref"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || len(gotC) != 0 || len(gotI) != 0 || codeOf(errI) != c.word {
			t.Errorf("%q: want %s on both lanes, got compiled=%v %v %v, interp %v %v", c.src, c.word, compiled, gotC, errC, gotI, errI)
			continue
		}
		if fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: compiled\n%v\ninterpreted\n%v", c.src, errC, errI)
		}
		if dis := compileDisasm(t, c.src); !strings.Contains(dis, "DISPATCH_REMATCH") || strings.Contains(dis, "TRAP ") {
			t.Errorf("%q: the trap is the outer word's rematch, not a bare TRAP; got:\n%s", c.src, dis)
		}
	}
	// The outer word is a speculative fn family (both arms of an undecided
	// `if` define it): the rematch has no single fn to plan, so the guarded
	// trap declines (recordGuardedTrap). The program must never answer
	// differently from the interpreter — today it declines loudly.
	src := x + `def mk fn [[][Any][[1 2]]] end def b fn [[][Boolean][true]] end ` +
		`def f (if (b) [fn [[a:List b:List][Any][b]]] [fn [[a:List b:List][Any][a]]]) end f (mk) [x/u]`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if codeOf(errI) != "illegal_ref" {
		t.Fatalf("%q: the interpreter raises illegal_ref, got %v / %v", src, gotI, errI)
	}
	if compiled && (fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI)) {
		t.Errorf("%q: compiled %v / %v, interpreted %v / %v", src, gotC, errC, gotI, errI)
	}
	if !compiled && codeOf(errC) != "compile_failed" {
		t.Errorf("%q: a declined program reports compile_failed, got %v", src, errC)
	}
}

// TestMerge519TypeValueSeedAndRebind: NUR323's type VALUE (a type literal,
// whose Parent is not its type) reaching two check-model carriers the
// compiler builds — a seeded map fold's accumulator (callable_words
// closureInputs) and a multi-run body's JOIN of a rebound name's start and
// end bindings (unit_memo bindCarrier). Both lanes read the type as the
// type it is.
func TestMerge519TypeValueSeedAndRebind(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The seeded map fold's accumulator is a type value.
		{`fold ([a:Any kv:Any] => [a]) {x: 1} Integer`, "[Integer]"},
		{`fold ([a:Type kv:KeyVal] => [a]) {x: 1 y: 2} Integer`, "[Integer]"},
		{`fold ([a:Any kv:KeyVal] => [typeof a]) {x: 1} Integer`, "[Number]"},
		{`fold ([a:Any kv:KeyVal] => [a is Number]) {x: 1} Integer`, "[true]"},
		{`fold ([a:Any kv:KeyVal] => [a add 1]) {x: 1} Integer`, "ERROR:cannot call `add`"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A multi-run body's def over a name whose binding is a type value: the
	// body's own since phase 2 (each element's run shadows and retires it),
	// so the start binding is what every element and the read after see;
	// the compiled lane declines the shadow until its block scopes land.
	for _, c := range []struct{ src, want string }{
		{`def t Integer end [1 2] each [drop typeof t def t 5] t`, "[[Number Number] Integer]"},
		{`def t Integer end [1 2] each [drop def u (typeof t) def t 5 u] t`, "[[Number Number] Integer]"},
		{`def t Integer end fold [drop typeof t def t 5] [10 20] 0`, "[Number]"},
		{`def t Integer end scan [def t 5] [10 20] t`, "[[10 20] Integer]"},
	} {
		ruleOrDecline(t, c.src, c.want)
	}
}
