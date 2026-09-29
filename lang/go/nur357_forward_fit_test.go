package lang

import (
	"testing"
)

// nur357_forward_fit_test.go pins NUR357, NUR352's last remainder and
// NUR351's note window. The interpreter's forward collection is
// type-directed: a value written after a word fills its slot only where it
// fits, and a candidate whose collection stops there takes the rest of its
// slots from the stack beneath. The pass collected a gradual operand (a
// fn's Any result, a member read of an opaque map) forward wherever its
// carrier was admitted, so `{b:2} keys y` over a y of 0 raised keys'
// no-match compiled where the interpreter answers [['b'] 0]. A read's own
// deopt point now tests the fits too (DeoptSpec.Fits), and every other
// such call takes a statement island at the call (PolyRef.Fit, CallFits).

const nur357Y = `def f fn [[] [Any] [0]] end def y (f) end `
const nur357M = `def mk fn [[] [Map] [{f: 0}]] end def m (mk) end `

// TestNUR357GradualReadCollectedForward: a root def read, a member read, a
// read in a fn body, a user fn's argument, before a trap and beneath an
// earlier residual — each answers the interpreter's collection compiled.
func TestNUR357GradualReadCollectedForward(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{nur357Y + `{b:2} keys y`, "[['b'] 0]"},
		{nur357Y + `{b:2} keys y end 5`, "[['b'] 0 5]"},
		{nur357Y + `7 {b:2} keys y`, "[7 ['b'] 0]"},
		{nur357Y + `[{b:2} keys y]`, "[[['b'] 0]]"},
		{nur357Y + `({b:2} keys y)`, "[['b'] 0]"},
		{nur357Y + `print "a" {b:2} keys y`, "[['b'] 0]"},
		{nur357Y + `5 end {b:2} keys y`, "[5 ['b'] 0]"},
		{nur357Y + `1 2 {b:2} keys y`, "[1 2 ['b'] 0]"},
		{nur357Y + `{b:2} keys y keys`, "ERROR:cannot call `keys`"},
		{nur357Y + `{b:2} keys y add 1`, "[['b'] 1]"},
		{nur357Y + `def z {b:2} end z keys y`, "[['b'] 0]"},
		{nur357Y + `def h fn [[] [Map] [{b:2}]] end h keys y`, "[['b'] 0]"},
		{`def f fn [[] [Any] [[1]]] end def y (f) end 1 add y`, "ERROR:cannot call `add`"},
		{nur357M + `{b:2} keys m.f`, "[['b'] 0]"},
		{nur357M + `print "a" {b:2} keys m.f`, "[['b'] 0]"},
		{nur357M + `{b:2} keys m.f end print "z"`, "[['b'] 0]"},
		{nur357M + `5 end {b:2} keys m.f`, "[5 ['b'] 0]"},
		{nur357M + `[{b:2} keys m.f]`, "[[['b'] 0]]"},
		{nur357M + `def c true end if c [{b:2} keys m.f] [1]`, "[['b'] 0]"},
		{nur357M + `def y m.f end {b:2} keys y`, "[['b'] 0]"},
		{`def mk fn [[] [Map] [{f: "ab"}]] end def m (mk) end [1 2] reverse m.f`, "[[2 1] ab]"},
		{`def g fn [[m:Map][Any][{b:2} keys m.a]] end [(g {a:0})]`, "ERROR:expected 1 return value(s), got 2"},
		{`def g fn [[m:Map][Any][print "p" {b:2} keys m.a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map][Any][def y m.a {b:2} keys y]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{`def g fn [[m:Map l:List][Any][l reverse m.f]] end g {f:"ab"} [1 2]`, "ERROR:[[2 1] 'ab']"},
		{nur357Y + `def h fn [[] [Any] [{b:2} keys y]] end h`, "ERROR:[['b'] 0]"},
		{nur357Y + `def g fn [[m:Map][Any][m keys y]] end [(g {b:2})]`, "ERROR:[['b'] 0]"},
		{nur357Y + `def k fn [[m:Map][List][keys m]] end {b:2} k y`, "[['b'] 0]"},
		{nur357M + `def k fn [[m:Map][List][keys m]] end {b:2} k m.f`, "[['b'] 0]"},
		// The run's value fits: the compiled collection stands.
		{`def f fn [[] [Any] [{a:1}]] end def y (f) end {b:2} keys y`, "[{b:2} ['a']]"},
		{`def mk fn [[] [Map] [{f: {c:1}}]] end def m (mk) end {b:2} keys m.f`, "[{b:2} ['c']]"},
		{`def g fn [[m:Map][Any][{b:2} keys m.a]] end [(g {a:{c:1}})]`, "ERROR:[{b:2} ['c']]"},
		{`def f fn [[] [Any] [5]] end def y (f) end def z (f) end z add y`, "[10]"},
		{`def f fn [[] [Any] ["x"]] end def y (f) end {a:1} get y`, "[None]"},
		// NEGATIVE: nothing beneath to collect, or nothing that fits — the
		// interpreter's own report on both lanes.
		{nur357Y + `keys y`, "ERROR:takes 1 argument, but none were supplied"},
		{nur357M + `keys m.f`, "ERROR:cannot call `keys`"},
		{nur357Y + `5 keys y`, "ERROR:cannot call `keys`"},
		{`def g fn [[m:Map][List][keys m.a]] end g {a:0}`, "ERROR:cannot call `keys`"},
		{nur357Y + `def k fn [[m:Map][List][keys m]] end k y`, "ERROR:cannot call `k`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR357UnservedCollectionDeclines: NEGATIVE — where the run's value
// may miss the very slot the pass collected it into, the stack beneath could
// fill that candidate, and no statement island can take the statement over
// (an effect in the operand's paren, a spliced word's expansion, a def the
// island would make again), the program declines rather than raise keys'
// no-match over a window the interpreter never assembles.
func TestNUR357UnservedCollectionDeclines(t *testing.T) {
	const k = `def k fn [[m:Map][List][keys m]] end `
	for _, tc := range []struct{ src, want string }{
		{nur357M + `{b:2} keys (print "x" m).f`, "[['b'] 0]"},
		{nur357M + `def w word [{b:2} keys m.f] end w`, "[['b'] 0]"},
		{nur357M + `def c true end if c [def q 3] [def q 4] end {b:2} keys m.f`, "[['b'] 0]"},
		{k + nur357M + `{b:2} k (print "x" m).f`, "[['b'] 0]"},
		{`def g fn [[m:Map][Any][{b:2} keys (print "q" m).a]] end g {a:0}`, "ERROR:[['b'] 0]"},
		{k + `def g fn [[m:Map][Any][{b:2} k (print "q" m).a]] end g {a:0}`, "ERROR:[['b'] 0]"},
	} {
		declinesWithInterpAnswer(t, tc.src, "(NUR357)", tc.want)
	}
}

// TestNUR357ReadPointTestsItsFits: the root read's own deopt point carries
// the fits of the dispatch that collected it, and takes the statement's
// island from its first token (DeoptSpec.Fits).
func TestNUR357ReadPointTestsItsFits(t *testing.T) {
	prog, reason, _, err := mustNew(t).CompileCheck(nur357Y + `{b:2} keys y`)
	if prog == nil || err != nil {
		t.Fatalf("must compile: %q %v", reason, err)
	}
	fits := 0
	for _, d := range prog.Deopts {
		fits += len(d.Fits)
	}
	if fits == 0 {
		t.Errorf("want the read's point to test keys' Map slot: %+v", prog.Deopts)
	}
	prog, reason, _, err = mustNew(t).CompileCheck(nur357M + `{b:2} keys m.f`)
	if prog == nil || err != nil {
		t.Fatalf("must compile: %q %v", reason, err)
	}
	islands := 0
	for _, pr := range prog.PolyRefs {
		if pr.Fit != nil {
			islands++
		}
	}
	if islands != 1 {
		t.Errorf("want keys' poly to carry its fit island: %+v", prog.PolyRefs)
	}
}

// TestNUR352IslandSeatsEarlierResiduals: a root gradual def read holding a
// fn, beneath an earlier statement's residual — the statement island seats
// the residual's entries before the statement as its prefix
// (DeoptSpec.Seat): a literal, an event result, a promoted value.
func TestNUR352IslandSeatsEarlierResiduals(t *testing.T) {
	const fnM = `def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def m (mk) end def x m.f end `
	for _, tc := range []struct{ src, want string }{
		{fnM + `5 end 1 x 7 add`, "[5 9]"},
		{fnM + `5 6 end 1 x 7 add`, "[5 6 9]"},
		{fnM + `"s" end 1 x 7 add`, "[s 9]"},
		{fnM + `5 end 1 x 7 add end 6`, "[5 9 6]"},
		{fnM + `def g fn [[] [Any] [4]] end g end 1 x 7 add`, "[4 9]"},
		{fnM + `Integer end 1 x 7 add`, "[Integer 9]"},
		// Data runs on as the model has it.
		{`def mk fn [[] [Map] [{f: 5}]] end def m (mk) end def x m.f end 5 end 1 x 7 add`, "[5 1 12]"},
		// NEGATIVE: the fn matches nothing it could collect, on both lanes.
		{`def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end def x m.f end 5 end 1 x 7 add`, "ERROR:cannot call `x`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR351NoMatchRendersRunValues: a no-match's report names the stack
// the interpreter's collection read — up to four values beneath the word,
// not the word's arity — and the compiled lane renders the run's values
// there, never the pass's carriers: a poly's split reads a value beneath
// where the compiled code keeps it (PolySplit.Live), and a failed window
// with a carrier in its report is the runtime rematch's (withRenderedPrefix).
func TestNUR351NoMatchRendersRunValues(t *testing.T) {
	const fg = `def f fn [[][Any][4]] end def g fn [[][Any][0]] end `
	const f4 = `def f fn [[][Any][4]] end `
	for _, tc := range []struct{ src, want string }{
		{fg + `f g keys`, "ERROR:the arguments were 0 (an Integer) and 4 (an Integer)"},
		{fg + `f g keys end 5`, "ERROR:the arguments were 0 (an Integer) and 4 (an Integer)"},
		{fg + `7 f g keys`, "ERROR:the arguments were 0 (an Integer), 4 (an Integer) and 7 (an Integer)"},
		{fg + `f f f f f g keys`, "ERROR:cannot call `keys`"},
		{fg + `[f g keys]`, "ERROR:the arguments were 0 (an Integer) and 4 (an Integer)"},
		{fg + `def h fn [[][Any][f g keys]] end h`, "ERROR:cannot call `keys`"},
		{f4 + `f end 0 keys`, "ERROR:the arguments were 0 (an Integer) and 4 (an Integer)"},
		{f4 + `f f end 0 keys`, "ERROR:cannot call `keys`"},
		{f4 + `f 7 end 0 keys`, "ERROR:cannot call `keys`"},
		{f4 + `5 f end "a" keys`, "ERROR:cannot call `keys`"},
		{f4 + `f end 0 keys end 9`, "ERROR:the arguments were 0 (an Integer) and 4 (an Integer)"},
		{f4 + `f f f f f end 0 keys`, "ERROR:cannot call `keys`"},
		// A match stands: the run's values fit.
		{`def f fn [[][Any][4]] end def g fn [[][Any][{a:1}]] end f g keys`, "[4 ['a']]"},
		{fg + `f g add`, "[4]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}
