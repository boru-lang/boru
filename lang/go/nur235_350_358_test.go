package lang

import (
	"fmt"
	"strings"
	"testing"
)

// requireInterpDeclined pins a shape the compiled lane declines loudly (no
// wrong answer) with the interpreter's answer want ("ERROR:<substring>" for
// an error).
func requireInterpDeclined(t *testing.T, src, want string) {
	t.Helper()
	_, compiled, errC, gotI, errI := runBothEngines(t, src)
	if compiled || errC == nil || !strings.Contains(errC.Error(), "compil") {
		t.Errorf("%s: must decline to compile, got compiled=%v err=%v", src, compiled, errC)
	}
	if sub, isErr := strings.CutPrefix(want, "ERROR:"); isErr {
		if errI == nil || !strings.Contains(errI.Error(), sub) {
			t.Errorf("%s: interpreter %v / %v, want an error containing %q", src, gotI, errI, sub)
		}
	} else if errI != nil || fmt.Sprint(gotI) != want {
		t.Errorf("%s: interpreter %v / %v, want %s", src, gotI, errI, want)
	}
}

// TestNUR358ListLiteralIsNoLoop pins NUR358's verdict: a list (or map)
// literal is not a loop. A break/continue escaping its elements belongs to
// the run holding the literal — the enclosing loop breaks or continues, the
// iteration's partial values dropped, and with no loop the run raises
// `<ctrl> outside loop` at the escaped run's position. No frame marker leaks
// into the value, and a call taking the literal is abandoned.
func TestNUR358ListLiteralIsNoLoop(t *testing.T) {
	const f = `def f fn [[] [Any] [break]] end `
	for _, c := range []struct{ src, want string }{
		// the record's repros
		{`[do (quote [break])]`, "ERROR:break outside loop"},
		{f + `[f]`, "ERROR:break outside loop"},
		{`def f fn [[b:List][Any][[do b]]] end f (quote [1 break])`, "ERROR:break outside loop\n  --> 1:48"},
		{`def f fn [[b:List][Any][size [do b]]] end f (quote [1 break])`, "ERROR:break outside loop\n  --> 1:53"},
		{`[do (quote [continue])]`, "ERROR:continue outside loop"},
		{`{a: 1 b: (do (quote [break]))}`, "ERROR:break outside loop"},
		{`def f fn [[][Any][[1 break]]] end f`, "ERROR:break outside loop"},
		// inside a loop the literal's escape breaks / continues the loop
		{`for 2 [[do (quote [break])]]`, "[]"},
		{`for 3 [[if (i eq 1) [break] [i]]]`, "[[0]]"},
		{`for 3 [[(if (i eq 1) [break] [i])]]`, "[[0]]"},
		{`for 3 [[i (if (i eq 1) [continue] [i])] 7]`, "[[0 0] 7 [2 2] 7]"},
		{`for 3 [[do (quote [continue])] 7]`, "[]"},
		{`for 3 [size [do (quote [break])]]`, "[]"},
		{`[for 3 [[if (i eq 1) [break] [i]]]]`, "[[[0]]]"},
		{f + `for 3 [[i f]]`, "[]"},
		{`def f fn [[] [Any] [for 3 [[if (i eq 1) [break] [i]]]]] end f`, "[[0]]"},
		// negatives: a literal that raises nothing is untouched
		{`for 3 [[i add 1]]`, "[[1] [2] [3]]"},
		{`[1 [2 3]]`, "[[1 [2 3]]]"},
		{`for 2 [print "a" (break) print "b"]`, "[]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The interpreter's answers where the compiled lane declines (a break
	// outside a compiled loop): the escape raises, never swallowed.
	for _, c := range []struct{ src, want string }{
		{`[break]`, "ERROR:break outside loop"},
		{`[1 break 2]`, "ERROR:break outside loop\n  --> 1:10"},
		{`[1 [2 break]]`, "ERROR:break outside loop"},
		{`{a: (break)}`, "ERROR:break outside loop"},
		{`print [1 break]`, "ERROR:break outside loop"},
	} {
		requireInterpDeclined(t, c.src, c.want)
	}
}

// TestNUR358WhileAndMarkers pins the while loop's collection of an escaped
// literal and the frame teardown of a literal's escaped fn call (no marker
// in any value; the frame's per-call state is popped).
func TestNUR358WhileAndMarkers(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def n 0 while [n lt 3] [def n (n add 1) [n (if (n eq 2) [break] [n])]] n`, "[[1 1] 2]"},
		{`def n 0 while [n lt 3] [def n (n add 1) [n (if (n eq 2) [continue] [n])]] n`, "[[1 1] [3 3] 3]"},
		// f's frame, spliced inside the literal, is torn down: g's args
		// list is the one `args` reads after the loop
		{`def f fn [[x:Integer] [Any] [break]] end def g fn [[y:Integer][Any][for 2 [[(f 1)]] args]] end g 9`, "[[9]]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}

// TestNUR358EscapeSeats pins the remaining seats a literal's escape can
// leave: a map member before others, a callback's residual literal, a
// module fn's operand, a pattern-evaluated operand, a def-form constructor's
// operand (whose gen spec is dropped with it), and an evaluation error in a
// loop's collection.
func TestNUR358EscapeSeats(t *testing.T) {
	const mod = `import module [def w fn [[x:List][Any][x]] end def v fn [[x:Map][Any][x]] end export "M" {w:w/v v:v/v}] end `
	for _, c := range []struct{ src, want string }{
		{`{a: (do (quote [break])) b: 2}`, "ERROR:break outside loop"},
		{`for 2 [{a: (do (quote [break])) b: 2}]`, "[]"},
		{`for 2 [each ([x:Any] => [[1 (break)]]) [1 2]]`, "[]"},
		{`[each ([x:Any] => [[1 (do (quote [break]))]]) [1 2]]`, "ERROR:break outside loop"},
		{mod + `for 2 [M.w [1 (break)]]`, "[]"},
		{mod + `for 2 [M.v {a: (break)}]`, "[]"},
		{mod + `M.w [1 (1 add 1)]`, "[[1 2]]"},
		{`def h ([m:{f:Integer}] => [m.f]) end def m {g: h/v} end for 2 [m.g {f: (break)}]`, "[]"},
		{`def P gen [T] refine Record [v: (do (quote [break]))]`, "ERROR:break outside loop"},
		// a handler-less `fn` value stack-matched from a member
		// (ExecFnDefSigStackMatch → execFnDefSig)
		{`def m {f: (fn [[xs:List][Any][xs]])} end for 2 [[1 (break)] m.f]`, "[]"},
		{`def m {f: (fn [[xs:Map][Any][xs]])} end for 2 [{a: (break)} m.f]`, "[]"},
		{`def m {f: (fn [[xs:List][Any][xs]])} end [1 (1 add 1)] m.f`, "[[1 2]]"},
		// a named fn value stack-matched from a member
		{`def g fn [[xs:List][Any][xs]] end def m {f: g/v} end for 2 [[1 (break)] m.f]`, "[]"},
		{`def g fn [[xs:Map][Any][xs]] end def m {f: g/v} end for 2 [{a: (break)} m.f]`, "[]"},
		{`def g fn [[xs:List][Any][xs]] end def m {f: g/v} end [1 (1 add 1)] m.f`, "[[1 2]]"},
		{`def g fn [[xs:Map][Any][xs]] end def m {f: g/v} end {a: (1 add 1)} m.f`, "[{a:2}]"},
		// a callback run on its own (the map-iteration each's CallBoru):
		// its residual literal's escape raises inside the callback, as
		// its own tokens' does — the caller's loop is not on its run
		{`for 2 [each ([kv:Any] => [[1 (break)]]) {a:1 b:2}]`, `ERROR:each: key "a": [boru/flow_error]: break outside loop`},
		{`for 2 [each ([kv:Any] => [1 break]) {a:1 b:2}]`, `ERROR:each: key "a": [boru/flow_error]: break outside loop`},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want string }{
		{`for 2 [[nosuch]]`, "ERROR:undefined word: nosuch"},
		{`def h fn [[xs:[:Integer]][Any][xs]] end for 2 [h [(break)]]`, "[]"},
		{`def P gen [T] refine Record [v:T w:[(do (quote [break]))]]`, "ERROR:break outside loop"},
		{`for 1 [def P gen [T] refine Record [v:T w:[(do (quote [break]))]]] 5`, "[5]"},
		// an evaluation error at a pattern slot raises (a second patterned
		// candidate does not evaluate the literal again), on a word and on
		// a fn value
		{`def h fn [[m:{f:Integer}][Any]["i"] [m:{f:String}][Any]["s"]] end h {f: (nosuch 1)}`, "ERROR:undefined word: nosuch"},
		{`def h fn [[xs:[:Integer]][Any][xs]] end h [(nosuch 1)]`, "ERROR:undefined word: nosuch"},
		{`def h ([m:{f:Integer}] => [m.f]) end def m {g: h/v} end m.g {f: (nosuch 1)}`, "ERROR:undefined word: nosuch"},
		{`def h ([m:{f:Integer}] => [m.f]) end (h/v {f: (nosuch 1)})`, "ERROR:undefined word: nosuch"},
	} {
		requireInterpDeclined(t, c.src, c.want)
	}
}

// TestNUR235PatternJudgesTheValue pins NUR235's verdict: a pending map or
// list literal at a slot whose pattern reads its contents is matched as the
// value the call evaluates it to — `{f: (1 add 1)}` against `m:{f:Integer}`
// is `{f: 2}`, a bound word member resolves, a typed list likewise — on
// both lanes, and a refusal's notes render values, or the literal as
// written, never internal forms.
func TestNUR235PatternJudgesTheValue(t *testing.T) {
	const h = `def h fn [[m:{f:Integer}][Any][m.f]] end `
	const hl = `def h fn [[xs:[:Integer]][Any][xs]] end `
	for _, c := range []struct{ src, want string }{
		{h + `h {f: (1 add 1)}`, "[2]"},
		{h + `{f: (1 add 1)} h`, "[2]"},
		{h + `def x 2 end h {f: x}`, "[2]"},
		{h + `[(h {f: (1 add 1)}) (h {f: 3})]`, "[[2 3]]"},
		{h + `def g fn [[x:Integer][Any][h {f: (x add 1)}]] end g 4`, "[5]"},
		{h + `def g fn [[x:Any][Any][h {f: x}]] end [(g 1) (g 2)]`, "[[1 2]]"},
		{`def h fn [[m:{f:Integer}][Any]["int"] [m:{f:String}][Any]["str"]] end [(h {f: (1 add 1)}) (h {f: ("a" add "b")})]`, "[['int' 'str']]"},
		{`def h fn [[m:{f:Integer}][Any]["int"] [m:Map][Any]["map"]] end [(h {f: (1 add 1)}) (h {f: ("a" add "b")})]`, "[['int' 'map']]"},
		{`def h fn [[k:Integer m:{f:Integer}][Any][m.f add k]] end h 10 {f: (1 add 1)}`, "[12]"},
		{`def h fn [[m:{f:{g:Integer}}][Any][m.f.g]] end h {f: {g: (1 add 1)}}`, "[2]"},
		{`def h fn [[m:{f:[:Integer]}][Any][m.f]] end h {f: [(1 add 1)]}`, "[[2]]"},
		{`def h ([m:{f:Integer}] => [m.f]) end h {f: (1 add 1)}`, "[2]"},
		{`def h fn [[m:{f:Function}][Any][m.f 3]] end h {f: ([x:Integer] => [x add 1])}`, "[4]"},
		{`def P refine Record [x:Integer] end def h fn [[p:P][Any][p.x]] end h {x: (1 add 1)}`, "[2]"},
		{hl + `h [(1 add 1) 3]`, "[[2 3]]"},
		{hl + `def y 3 h [y 4]`, "[[3 4]]"},
		{hl + `def g fn [[a:Any][Any][h [a]]] end [(g 1) (g 2)]`, "[[[1] [2]]]"},
		// negatives: a member whose value misses the pattern is refused,
		// the note naming the value
		{hl + `h [("a" add "b")]`, "ERROR:['ab'] does not satisfy its declared pattern [:Integer]"},
		{h + `h {g: (1 add 1)}`, "ERROR:{g:2} does not satisfy its declared pattern {f:Integer}"},
		// a failed match that never judged the literal renders it as written
		{`def h fn [[m:{f:Integer} k:Integer][Any][m.f add k]] end h {f: (1 add 1)} "x"`, "ERROR:the arguments were {f:(1 add 1)} (a Map) and 'x' (a ProperString)\n  = note: candidate `h (Map, Integer)` — argument 2"},
		{`def h fn [[k:Integer][Any][k]] end h {f: (1 add 1)}`, "ERROR:got {f:(1 add 1)} (a Map)"},
		{`def h fn [[k:Integer][Any][k]] end h [(1 add 1)]`, "ERROR:got [(1 add 1)] (a List)"},
		{h + `h [(1 add 1)]`, "ERROR:expected Map, got [(1 add 1)] (a List)"},
		// a check-pass recovery that evaluated the literal still traps the
		// interpreter's report, which renders it as written
		{`def h fn [[m:Map k:Integer][Any][k] [m:Map k:Boolean][Any][k]] end def x "q" end h {f: (1 add 1)} x`, "ERROR:the argument was {f:(1 add 1)} (a Map)"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// Refused shapes the check pass stops on (a definite record-shape
	// mismatch, an undefined member): the interpreter's refusal, evaluated.
	for _, c := range []struct{ src, want string }{
		{h + `h {f: "a"}`, "ERROR:{f:'a'} does not satisfy its declared pattern {f:Integer}"},
		{h + `h {f: ("a" add "b")}`, "ERROR:{f:'ab'} does not satisfy"},
		{h + `def x "s" end h {f: x}`, "ERROR:{f:'s'} does not satisfy"},
		{h + `h {f: (nosuch 1)}`, "ERROR:undefined word: nosuch"},
		{h + `for 3 [h {f: (if (i eq 1) [break] [i])}]`, "[0]"},
		{h + `h {f: (do (quote [break]))}`, "ERROR:break outside loop"},
		// over a gradual operand the report would be rebuilt from the
		// evaluated literal at run time: declined, never a divergent note
		{`def h fn [[m:Map k:Integer][Any][k] [m:Map k:Boolean][Any][k]] end def g fn [[x:String][Any][h {f: (1 add 1)} x]] end g "q"`, "ERROR:the argument was {f:(1 add 1)} (a Map)"},
		{`def h fn [[m:Map k:Integer][Any][k] [m:Map k:Boolean][Any][k]] end def g fn [[x:String][Any][{f: (1 add 1)} x h]] end g "q"`, "ERROR:the arguments were 'q' (a ProperString) and {f:(1 add 1)} (a Map)"},
	} {
		requireInterpDeclined(t, c.src, c.want)
	}
}

// TestNUR350ForkedFrameArgs pins NUR350's fork edge: a frame run in an await
// branch (a forked registry) holds its call's real args on both lanes — the
// VM used to push the elided empty list there.
func TestNUR350ForkedFrameArgs(t *testing.T) {
	agreeOnBothLanes(t, `import "boru:time-util" def w fn [[x:Integer][Any][m]] end def m word [args] end TimeUtil.await {mode:'all'} [[w 3] [w 4]]`, "[[[3] [4]]]")
	agreeOnBothLanes(t, `def w fn [[Integer y:Integer][Any][args]] end w 3 4`, "[[3 4]]")
	agreeOnBothLanes(t, `def w fn [[x:Integer][Any][x m]] end def m word [args] end w 3`, "ERROR:expected 1 return value(s), got 2 — [3 [3]]")
}

// TestNUR365ParenIsNoLoop pins NUR365's verdict (NUR358's rule for a
// paren): a break/continue escaping a paren a word collects abandons the
// word and belongs to the enclosing loop — continue included, nested
// parens, a user fn's operand, a map member, a fn body — and with no loop
// the run raises `outside loop` where the group's run stood; a fn frame the
// group spliced is torn down with it.
func TestNUR365ParenIsNoLoop(t *testing.T) {
	const h = `def h fn [[a:Integer][Any][a]] end `
	const f = `def f fn [[x:Integer][Any][break]] end `
	for _, c := range []struct{ src, want string }{
		{`for 2 [def x (1 break) end print "b"]`, "[]"},
		{`for 2 [def x (1 continue) end print "b"]`, "[]"},
		{`for 2 [def x ((1 break)) end print "b"]`, "[]"},
		{`for 2 [def x ((1 continue)) end print "b"]`, "[]"},
		{`for 2 [def x (2 add (1 break)) end print "b"]`, "[]"},
		{h + `for 2 [h (1 break) print "b"]`, "[]"},
		{h + `for 3 [h (if (i eq 1) [continue] [i])]`, "[0 2]"},
		{h + `for 3 [h (if (i eq 1) [break] [i])]`, "[0]"},
		{`for 2 [size (1 break)]`, "[]"},
		{`for 2 [{a: (def x (1 break) end x)}]`, "[]"},
		{`for 2 [i def x (1 break) end]`, "[]"},
		{`for 3 [def x (if (i eq 1) [break] [i]) end x]`, "[0]"},
		{`for 3 [def x (if (i eq 1) [continue] [i]) end x]`, "[0 2]"},
		{`def g fn [[][Any][for 2 [def x (1 break) end print "b"] 7]] end g`, "[7]"},
		{f + `def g fn [[y:Integer][Any][for 2 [def z (f 1) end] args]] end g 9`, "[[9]]"},
		{`def f fn [[x:Integer][Any][continue]] end def g fn [[y:Integer][Any][for 2 [def z (f 1) end] args]] end g 9`, "[[9]]"},
		// no loop: the run raises where the group's run stood
		{`def g fn [[][Any][def x (1 break) end 7]] end g`, "ERROR:break outside loop\n  --> source position unknown"},
		{`def g fn [[][Any][def x (1 break 2) end 7]] end g`, "ERROR:break outside loop\n  --> 1:34"},
		{`def g fn [[b:List][Any][def x (do b) end 7]] end g (quote [1 break 2])`, "ERROR:break outside loop\n  --> 1:60"},
		// negative: a quiet paren operand is the word's argument
		{`for 2 [def x (1 add 1) end x]`, "[2 2]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// At the root the compiled lane declines (a break outside a compiled
	// loop); the interpreter raises at the group's run, never a
	// signature_error on the abandoned word.
	for _, c := range []struct{ src, want string }{
		{`def x (1 break) end`, "ERROR:break outside loop\n  --> source position unknown"},
		{`def x (1 break 2) end`, "ERROR:break outside loop\n  --> 1:16"},
		{`def x (1 continue) end`, "ERROR:continue outside loop"},
		{h + `h (1 break 2)`, "ERROR:break outside loop\n  --> 1:47"},
		{`{a: (def x (1 break) end x)}`, "ERROR:break outside loop"},
	} {
		requireInterpDeclined(t, c.src, c.want)
	}
	// A loop over a fn frame the group spliced, interpreter-only (a
	// variadic loop result): the callee's args list does not outlive it.
	requireInterpDeclined(t, f+`def g fn [[y:Integer][Any][for 2 [size (f 1)] args]] end g 9`, "[[9]]")
}

// TestNUR365LoopInsideParen: a loop the collected paren itself holds is the
// nearest loop — its break/continue stays inside the group, and the word
// collects the loop's results (Codex on #526: `size (for 2 [break])` raised
// `break outside loop`, and inside another loop broke that one). A def
// binding the first value of a loop whose iteration a break/continue may
// cut short declines: the static count no longer sizes its region (the
// compiled run underflowed BIND_GLOBAL).
func TestNUR365LoopInsideParen(t *testing.T) {
	const f = `def f fn [[][Any][break]] end `
	for _, c := range []struct{ src, want string }{
		{`def g fn [[][Any][def x (for 3 [if (i eq 1) [break] [i]]) end x]] end g`, "[0]"},
		{`def g fn [[][Any][def x (for 3 [if (i gt 0) [continue] [i]]) end x]] end g`, "[0]"},
		// negative: a loop no signal cuts short keeps the static split
		{`def x (for 3 [i]) end x`, "[1 2 0]"},
		{`for 2 [def x (for 1 [i]) end x]`, "[0 0]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want string }{
		{`size (for 2 [break])`, "ERROR:argument expression produced no value for size"},
		{`size (for 2 [continue])`, "ERROR:argument expression produced no value for size"},
		{`for 3 [size (for 2 [break])]`, "ERROR:argument expression produced no value for size"},
		{`1 add (for 3 [if (i eq 1) [break] [i]])`, "[1]"},
		{`1 add (for 3 [if (i lt 2) [continue] [i]])`, "[3]"},
		{`1 add (for 3 [if (i eq 1) [(break)] [i]])`, "[1]"},
		{`1 add ((for 3 [if (i eq 1) [break] [i]]))`, "[1]"},
		{`for 2 [1 add (for 3 [if (i eq 1) [break] [i]])]`, "[1 1]"},
		{`for 2 [def x (for 3 [if (i eq 1) [break] [i]]) end x]`, "[0 0]"},
		{`def x (for 3 [if (i eq 1) [break] [i]]) end x`, "[0]"},
		{`def x (for 3 [if (i eq 2) [break] [i]]) end x`, "[1 0]"},
		{`def x (for 3 [if (i gt 0) [continue] [i]]) end x`, "[0]"},
		{`def x (for 3 [if (i eq 1) [(break)] [i]]) end x`, "[0]"},
		{`def x (for 3 [(for 2 [i break]) i]) end x`, "[1 2 0]"},
		{f + `def x (for 3 [if (i eq 1) [f] [i]]) end x`, "[0]"},
	} {
		requireInterpDeclined(t, c.src, c.want)
	}
}
