package lang

import "testing"

// nur357_branch_island_test.go pins NUR357's remainder: a statement that
// opens with an `if` whose arms make defs and leave nothing, ahead of a call
// over a gradual operand the interpreter collects differently. The branch
// ran in the compiled code, and the call's forward-fit island takes the
// statement over after it (compiler leadingDef / fitStartBefore), so the
// compiled run answers the interpreter's collection. Each used to decline
// "(NUR357)", and before that raised keys' no-match.

// TestNUR357IslandPastDefMakingBranch: POSITIVE — in a fn unit and at the
// root, one or two such branches, a def before them, a parameter condition.
func TestNUR357IslandPastDefMakingBranch(t *testing.T) {
	const mk = `def mk fn [[][Any][{a:0}]] end def m (mk) end `
	for _, tc := range []struct{ src, want string }{
		{`def g fn [[m:Map][Any][if true [def q 3] [def q 4] {b:2} keys m.a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map][Any][def q (if true [3] [4]) {b:2} keys m.a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def c true end def g fn [[m:Map][Any][if c [def q 3] [def q 4] {b:2} keys m.a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map c:Boolean][Any][if c [def q 3] [def q 4] {b:2} keys m.a]] end g {a:0} false`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map][Any][def r 1 if true [def q 3] [def q 4] {b:2} keys m.a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map][Any][if true [def q 3] [def q 4] if true [def r 1] [def r 2] {b:2} keys m.a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map][Any][if true [def q 3] [] {b:2} keys m.a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def f fn [[][Any][0]] end def g fn [[][Any][if true [def q 3] [def q 4] {b:2} keys (f)]] end g`, "ERROR:[['b'] 0]"},
		{`def f fn [[][Any][0]] end def g fn [[][Any][def y (f) if true [def q 3] [def q 4] {b:2} keys y]] end g`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map][Any][if true [def q 3] [def q 4] {b:2} keys m.a]] end [g {a:0}]`, "ERROR:[['b'] 0]"},
		{mk + `if true [def q 3] [def q 4] {b:2} keys m.a`, "[['b'] 0]"},
		{mk + `if true [def q 3] [def q 4] {b:2} keys m.a q`, "[['b'] 0 3]"},
		{mk + `def c false end if c [def q 3] [def q 4] {b:2} keys m.a q`, "[['b'] 0 4]"},
		{mk + `if true [def q 3] [def q 4] if true [def r 1] [def r 2] {b:2} keys m.a`, "[['b'] 0]"},
		// The run's value fits: the compiled collection stands.
		{`def g fn [[m:Map][Any][if true [def q 3] [def q 4] {b:2} keys m.a]] end g {a:{c:1}}`, "ERROR:[{b:2} ['c']]"},
		{`def mk fn [[][Any][{a:{z:1}}]] end def m (mk) end if true [def q 3] [def q 4] {b:2} keys m.a`, "[{b:2} ['z']]"},
		// NEGATIVE: nothing beneath fits either — the interpreter's report.
		{`def g fn [[m:Map][Any][if true [def q 3] [def q 4] keys m.a]] end g {a:0}`, "ERROR:cannot call `keys`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
	// The branch's effect runs once, in the compiled code, before the island.
	r := runBothWithOutput(t, `def g fn [[m:Map][Any][if true [def q 3 print "x"] [def q 4] {b:2} keys m.a]] end g {a:0}`)
	if !r.compiled || r.outC != r.outI || r.outI != "x\n" {
		t.Errorf("branch effect: compiled=%v %q, interpreted %q", r.compiled, r.outC, r.outI)
	}
}

// TestNUR357DefMakingBranchUnservedDeclines: NEGATIVE — where the island
// would read a def the branch made in a fn unit, or a 2-operand `if` whose
// arm span the island cannot tell, no island is seated past the branch and
// the program still declines "(NUR357)".
func TestNUR357DefMakingBranchUnservedDeclines(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`def g fn [[m:Map][Any][if true [def q 3] [def q 4] {b:2} keys m.a end q]] end g {a:0}`, "ERROR:[['b'] 0 3]"},
		{`def g fn [[m:Map][Any][if true [def q 3] [def q 4] {b:2} keys m.a q]] end g {a:0}`, "ERROR:[['b'] 0 3]"},
		{`def mk fn [[][Any][{a:0}]] end def m (mk) end if true [def q 3] {b:2} keys m.a`, "ERROR:cannot call `keys`"},
	} {
		declinesWithInterpAnswer(t, tc.src, "(NUR357)", tc.want)
	}
}
