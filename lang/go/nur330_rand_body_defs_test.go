package lang

import "testing"

// TestNUR330RandGeneratorBodyDefs pins NUR330's close. Rand.list-of and
// Rand.map-from run their generator bodies on the shared registry with no
// def cleanup, so a body `def` rebinds the name for the next run and for the
// rest of the program; the compiled program read the binding it had recorded
// before the call.
//
// list-of COMPILES and agrees: it is each's shape (a body run per element on
// the shared registry), so it carries BodyMultiRunKeepsDefs and a ReturnsFn
// that runs the body in the pass's model as each's does — the body's defs are
// kept installs in the compiled unit and the reads after it seat live.
func TestNUR330RandGeneratorBodyDefs(t *testing.T) {
	const rnd = `import "boru:rand"  `
	for _, c := range []struct{ src, want string }{
		{rnd + `var k 5 Rand.list-of [var k 1 k] 1 k`, "[[1] 1]"},
		{rnd + `var k 5 Rand.list-of [var k 1 k] 2 k`, "[[1 1] 1]"},
		{rnd + `var k 5 Rand.list-of [var k (k add 1) k] 3 k`, "[[6 7 8] 8]"},
		{rnd + `var k 5 Rand.list-of [var k (k add 1) k] 3 [k]`, "[[6 7 8] [8]]"},
		{rnd + `var k 5 def g fn [[] [Any] [k]] end Rand.list-of [var k 1 k] 1 g`, "[[1] 1]"},
		{rnd + `var k 5 for 2 [Rand.list-of [var k (k add 1) k] 1] k`, "[[6] [7] 7]"},
		{rnd + `var x "a" Rand.list-of [var x 1 x] 1 x add 1`, "[[1] 2]"},
		{rnd + `def r (Rand.with-seed 3) end var k 5 r.list-of [var k 1 k] 1 k`, "[[1] 1]"},
		// Negative: a body that runs zero times binds nothing, and one that
		// binds another name leaves k alone.
		{rnd + `var k 5 Rand.list-of [var k 1 k] 0 k`, "[[] 5]"},
		{rnd + `def k 5 Rand.list-of [def j 1 j] 1 k`, "[[1] 5]"},
		{rnd + `def k 5 Rand.list-of [k add 1] 2 k`, "[[6 6] 5]"},
		// map-from bodies that bind nothing still compile.
		{rnd + `def k 5 Rand.map-from {b:[k add 1]} k`, "[{b:6} 5]"},
		{rnd + `Rand.map-from {a:[Rand.int 0 10]} keys`, "[['a']]"},
		{rnd + `Rand.map-from {a:[1 add "x"]}`, "[{a:'1x'}]"},
		{rnd + `Rand.map-from {a:[raise bad_input "boom"]}`, "ERROR:boom"},
		{rnd + `Rand.map-from {a:[nosuchword]}`, "ERROR:undefined word: nosuchword"},
		{rnd + `Rand.map-from {a:5}`, "ERROR:value is not a list"},
		{rnd + `def s {a:[1]} end Rand.map-from s`, "[{a:1}]"},
		// A bound schema literal's lists ran when the def bound them.
		{rnd + `def s {a:[def k 1 k]} end def k 5 Rand.map-from s k`, "[{a:1} 5]"},
		{rnd + `def f fn [[b:List][Any][def s {a: b} Rand.map-from s]] end f [2]`, "[{a:2}]"},
		{rnd + `def f fn [[m:Map][Any][Rand.map-from m]] end f {a:[1]}`, "[{a:1}]"},
		// The argument list ran at the call (a word-context list), so the
		// body the schema holds is its value and the def happened there.
		{rnd + `def k 5 def f fn [[b:List][Any][def s {a: b} Rand.map-from s]] end f [def k 1 k] k`, "[{a:1} 1]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A body's own def is the body's (phase 2): the read after the call of a
	// name only the body bound is undefined_word, and the compiled lane
	// declines at the check.
	ruleOrDecline(t, rnd+`Rand.list-of [def k 1 k] 1 k`, "ERROR:undefined word: k")
	// Inside a fn the body assigns the frame's var: the closure path declines
	// the shape (core assignVarUnitGate) and the backstop declines the
	// code-body word, so the row is the interpreter's.
	ruleOrDecline(t, rnd+`def f fn [[n:Integer] [Any] [var k 5 Rand.list-of [var k (k add n) k] 2 k]] end f 3`, "ERROR:expected 1 return value(s), got 2 — [[8 11] 11]")
	// map-from DECLINED a body that binds a name: the compiled program runs
	// its bodies as run-time token bodies it never joins into its model, so
	// the binding had no placement in the compiled stream and the twin regime
	// refused the program — a sound, loud refusal where the read after the
	// call answered the stale binding ([{b:1} 5] for the interpreter's
	// [{b:1} 1]).
	//
	// Since phase 2 the body is a BLOCK, so a `def` inside it is the body's
	// own and k after the call is the module's 5; the body ASSIGNS the var
	// to update it, and a name only the body bound is unbound after. Each
	// row is the interpreter's answer with the compiled lane agreeing or
	// declining (ruleOrDecline).
	for _, c := range []struct{ src, want string }{
		{rnd + `def k 5 Rand.map-from {b:[def k 1 k]} k`, "[{b:1} 5]"},
		{rnd + `var k 5 Rand.map-from {b:[var k 1 k]} k`, "[{b:1} 1]"},
		{rnd + `var k 5 Rand.map-from {b:[var k 1 k] c:[k]} k`, "[{b:1 c:1} 1]"},
		{rnd + `var k 5 [(Rand.map-from {b:[var k 1 k]}) k]`, "[[{b:1} 1]]"},
		{rnd + `var k 5 Rand.map-from {b:[do [var k 1] k]} k`, "[{b:1} 1]"},
		{rnd + `def r (Rand.with-seed 3) end var k 5 r.map-from {b:[var k 1 k]} k`, "[{b:1} 1]"},
		{rnd + `def k 5 Rand.map-from {b:[def j 1 j]} j`, "ERROR:undefined word: j"},
		{rnd + `var k 5 def m {a: (quote [var k 1 k])} end Rand.map-from m k`, "[{a:1} 1]"},
	} {
		ruleOrDecline(t, c.src, c.want)
	}
}
