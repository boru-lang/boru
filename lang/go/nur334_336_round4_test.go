package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur334_336_round4_test.go sweeps the edges of the 2026-09-29/30 NUR334 /
// NUR336 islands — the call-result island (compiler call_result_island.go),
// the late start and the seated late values of a statement island
// (landing_restart.go lateStart, deoptPoint.lits), the per-iteration loop
// continuation (loopContPlan, LoopCont) and the spliced word's live point
// (spliceBody) — over the shapes each declines. Every program must answer as
// the interpreter does compiled, or defer loudly (a designed defer or a
// compile decline): never a silent wrong answer.

// requireSoundOrLoud asserts the compiled lane either agrees with the
// interpreter byte for byte or fails loudly (a compile decline or a compiled
// runtime defect report), and that the interpreter answers want (an
// ERROR:-prefixed want names a substring of its error; empty skips it).
func requireSoundOrLoud(t *testing.T, src, want string) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if sub, isErr := strings.CutPrefix(want, "ERROR:"); isErr {
		if errI == nil || !strings.Contains(errI.Error(), sub) {
			t.Errorf("%s: interpreter %v / %v, want an error containing %q", src, gotI, errI, sub)
		}
	} else if want != "" && (errI != nil || fmt.Sprint(gotI) != want) {
		t.Errorf("%s: interpreter %v / %v, want %s", src, gotI, errI, want)
	}
	if compiled && fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) && (!isBailDefect(errC) || len(gotC) != 0) {
		t.Errorf("%s: compiled %v / %v silently, interpreter %v / %v", src, gotC, errC, gotI, errI)
	}
}

// TestNUR334CallResultEdges: call-result islands a unit may couple through
// (a call of a unit that calls one), and the calls no island takes.
func TestNUR334CallResultEdges(t *testing.T) {
	const f = `def f fn [[b:List][Any][def t 0 do b drop t/v]] end `
	const call = `f (quote [def t word [1 2] 1])`
	for _, c := range []struct{ src, want string }{
		// A unit coupling through another's call.
		{f + `def g fn [[][Any][` + call + ` end]] end 9 (g)`, "ERROR:expected 1 return value(s), got 2"},
		{f + `def g fn [[][Any][` + call + ` end]] end [(g) 3]`, "ERROR:expected 1 return value(s), got 2"},
		// A recursive unit is seen once.
		{`def r fn [[n:Integer][Any][if (n gt 0) [r (n sub 1)] [n]]] end 9 r 3`, "[9 0]"},
		// A program ending in a terminal trap plans none.
		{f + `9 ` + call + ` end 1 add "a" nosuchword`, "ERROR:undefined word"},
		// A statement the island cannot take over: a call in a list with a
		// later read, a stack operand from an earlier statement.
		{f + `(quote [def t word [1 2] 1]) end f`, "[1 2]"},
		{f + `def g fn [[b:List][Any][def z 5 end z f b]] end g (quote [def t word [7] 1])`, "ERROR:expected 1 return value(s), got 2"},
		{f + `def g fn [[b:List][Any][4 end f b]] end g (quote [def t word [7] 1])`, "ERROR:expected 1 return value(s), got 2"},
		{f + `def g fn [[b:List][Any][print "a" f b 1]] end g (quote [def t word [7] 1])`, "ERROR:expected 1 return value(s), got 2"},
	} {
		requireSoundOrLoud(t, c.src, c.want)
	}
}

// TestNUR336LateStartEdges: statements whose run before the stop took a value
// from beneath, where no late start serves, and late values no island seats.
func TestNUR336LateStartEdges(t *testing.T) {
	const mk = `def mk fn [[] [Map] [{f: 5}]] end `
	const m = mk + `def m (mk) end `
	for _, c := range []struct{ src, want string }{
		// The stop at the statement's first token after a taking word in an
		// earlier paren of the same statement.
		{m + `(1 add 2) end (drop) (m.f 7)`, "ERROR:cannot call `drop`"},
		{m + `(1 add 2) end drop 4 print (m.f 7)`, "[4 7]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end (drop) (q.f 7) drop drop]] end g (mk)`, "ERROR:cannot call `drop`"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop print (q.f 7)]] end g (mk)`, "[7]"},
		// A literal consumed before the start as well as written before it.
		{mk + `def g fn [[q:Map][Any][4 end 4 add 1 end (q.f 7) drop drop add]] end g (mk)`, "[9]"},
		{mk + `def g fn [[q:Map][Any][def z 3 end z end z add 1 end (q.f 7) drop drop add]] end g (mk)`, "[7]"},
		{mk + `def g fn [[q:Map][Any][def z (1 add 2) end z end z add 1 end (q.f 7) drop drop add]] end g (mk)`, "[7]"},
		// A param read before the start, one read of it consumed there.
		{mk + `def g fn [[q:Map n:Integer][Any][n end n add 1 end (q.f 7) drop drop add]] end g (mk) 4`, "[9]"},
		// A type value read before the start.
		{mk + `def g fn [[q:Map][Any][def z Integer end z end (q.f 7) drop drop]] end g (mk)`, "[Integer]"},
		// A literal seated beneath a later frame entry.
		{mk + `def g fn [[q:Map][Any][4 end (1 add 2) end (q.f 7) drop drop]] end g (mk)`, "ERROR:expected 1 return value(s), got 2"},
		// A value whose position does not split the frame's entries.
		{mk + `def g fn [[q:Map][Any][(1 add 2) end 4 end swap end (q.f 7) drop drop drop]] end g (mk)`, "[4]"},
		// A root statement whose paren the pass told no stack for.
		{m + `(1 add 2) end drop [(m.f 7)]`, "[[5 7]]"},
		{m + `(1 add 2) end drop ((m.f 7))`, "[5 7]"},
	} {
		requireSoundOrLoud(t, c.src, c.want)
	}
}

// TestNUR336LoopContinuationEdges: loops the per-iteration continuation
// declines — a condition loop, a carried def, a body def, a loop not at its
// statement's start, values beneath it the walk cannot seat — and nested
// loops.
func TestNUR336LoopContinuationEdges(t *testing.T) {
	const two = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end def y fn [[] [Integer] [42]] end `
	const unit = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def y fn [[] [Integer] [42]] end `
	for _, c := range []struct{ src, want string }{
		{two + `def n (flex [0]) end while [(n.0 lt 2)] [n set 0 (n.0 add 1) drop (m.f y) 9 drop]`, "[]"},
		{two + `def a 0 end for 2 [def a (a add 1) (m.f y) 9 drop] a`, "[2]"},
		{two + `for 2 [def u 1 (m.f y) 9 drop]`, "[]"},
		{two + `5 for 2 [(m.f y) 9 drop]`, "[5]"},
		{two + `for 2 [for 2 [(m.f y) 9 drop]]`, "[]"},
		{two + `for 2 [[(m.f y) 9]]`, "[[33] [33]]"},
		{two + `for 2 [i end (m.f y) 9 drop]`, "[0 1]"},
		{two + `(1 add 2) end for 2 [(m.f y) 9 drop] end 7`, "[3 7]"},
		{two + `def k (1 add 2) end k end for 2 [(m.f y) 9 drop]`, "[3]"},
		{unit + `def g fn [[q:Map][Any][4 end for 2 [(q.f y) 9 drop]]] end g (mk)`, "[4]"},
		{unit + `def g fn [[q:Map][Any][def z (1 add 2) end z end for 2 [(q.f y) 9 drop]]] end g (mk)`, "[3]"},
		{unit + `def g fn [[q:Map Integer][Any][for 2 [(q.f y) 9 drop]]] end g (mk) 4`, "[4]"},
		{unit + `def g fn [[q:Map][Any][for 2 [(q.f y) 9 drop] 5]] end g (mk)`, "[5]"},
		{unit + `def g fn [[q:Map][Any][5 for 2 [(q.f y) 9 drop]]] end g (mk)`, "[5]"},
	} {
		requireSoundOrLoud(t, c.src, c.want)
	}
}

// TestNUR334SplicedWordEdges: spliced words the live point declines — a name
// read twice or nested, a splice fired before its def, a def made again.
func TestNUR334SplicedWordEdges(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{mkSplice + `def w word [do (mk) end x] end w end [w]`, "[1 2 [1 2]]"},
		{mkSplice + `def w word [do (mk) end x] end (w)`, "[1 2]"},
		{mkSplice + `def w word [do (mk) end x] end def w word [x] end w`, "[0]"},
		{mkSplice + `def w word [do (mk) end x]`, "[]"},
		{mkSplice + `def w [do (mk) end x] end w`, "[[1 2]]"},
		{mkSplice + `quote w word [do (mk) end x] end w`, ""},
	} {
		requireSoundOrLoud(t, c.src, c.want)
	}
}
