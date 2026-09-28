package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestMapLiteralFlexMemberCompiles: a map literal whose member is a paren
// group minting a FLEX — `{a:(flex [1])}` — compiles and agrees with the
// interpreter at every position: the program's residual, a native's operand,
// a def-bound value read through `dot`, nested in a map or a list member,
// and the canon round trip `Vm.run` makes of a flex tree (canon.tsv
// L39/L40). The check pass used to accept the member's const-fold for a
// plain literal whatever it held, so the folded flex — a reference minted by
// the fold's scratch run — had no event and the map no compiled home
// ("residual value not statically materialisable"); a folded flex or store
// now keeps the recorded path, where the group runs as an inline region and
// the map assembles from its own event, as a list literal's element always
// did (`[(flex [1])]`).
func TestMapLiteralFlexMemberCompiles(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`{a:(flex [1])}`, "[{a:[1]}]"},
		{`size {a:(flex [1]) b:2}`, "[2]"},
		{`(flex {a:(flex [1])})`, "[{a:[1]}]"},
		{`def m {a:(flex [1]) b:(flex {c:2})} m.b.c`, "[2]"},
		{`def m {a:(flex [1]) b:(1 add 2)} m.b`, "[3]"},
		{`def m {a:(flex [1])} push 2 m.a end m.a`, "[[1 2] [1 2]]"},
		{`def m {x: {y:(flex [1])}} m.x.y`, "[[1]]"},
		{`{a:{b:(flex [1])}}`, "[{a:{b:[1]}}]"},
		{`def m {x: [{y:(flex [1])}]} (m.x get 0).y`, "[[1]]"},
		{`[{a:(flex [1])} {b:(flex [2])}]`, "[[{a:[1]} {b:[2]}]]"},
		{`def m {a:(flex [1])} size {x: m.a}`, "[1]"},
		{`{a:(1 2)}`, "[{a:[1 2]}]"},
		{`{a:() b:1}`, "[{b:1}]"},
		{`size {a:() b:1}`, "[1]"},
		{`size {a:(1 2) b:3}`, "[2]"},
		{`def m {a:(flex [1])} {x:(m.a) y:(1 2)}`, "[{x:[1] y:[1 2]}]"},
		{`import "boru:vm" def v (flex {a:[1]}) (Vm.run (canon v)) deq v`, "[true]"},
		{`import "boru:vm" def v (flex {a:[1]}) typeof (Vm.run (canon v))`, "[FlexMap]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
	// The canon rows' sub-program compiles inside Vm.run now: no unattributed
	// interpreter entry is left (they were the interp-entry census's canon.tsv
	// L39/L40).
	for _, src := range []string{
		`import "boru:vm" def v (flex {a:[1]}) (Vm.run (canon v)) deq v`,
		`import "boru:vm" def v (flex {a:[1]}) typeof (Vm.run (canon v))`,
	} {
		if seen, _ := unattributedEntries(t, src); len(seen) != 0 {
			t.Errorf("%q: unattributed interpreter entries %v", src, seen)
		}
	}
	// A class body keeps its CONCRETE defaults — the check pass's schema
	// needs the value, not the region's carrier — a flex default and a
	// nested map holding one included (the folded value under the region
	// result's identity), and a folded class-instance default keeps the
	// const path (the schema default `make` copies per instance): both lanes.
	for _, c := range []struct{ src, want string }{
		{`def C class {x:(flex [1])} end (make C {}).x`, "[[1]]"},
		{`def C class {x:{y:(flex [1])}} end (make C {}).x.y`, "[[1]]"},
		{`def Bits class {bits:(flex [0 0 0])} def a (make Bits {}) def b (make Bits {}) def _ (set 0 9 a.bits) end b.bits`, "[[0 0 0]]"},
		{`def Foo refine Integer end def S class {x:(make Foo 1)} end (typeof ((make S {}) dot x))`, "[Foo]"},
		{`def Inner class {n:0} def Outer class {i:(make Inner {})} def a (make Outer {}) def b (make Outer {}) set n 9 a.i end b.i.n`, "[0]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled %v/%v (%v) interp %v/%v, want %s on both lanes", c.src, gotC, errC, compiled, gotI, errI, c.want)
		}
	}
}

// TestLoopBodyResidualLiteralResolves pins NUR197's close: a loop body whose
// residual is a literal bearing the loop variable — `for 2 [[(i add 1)] i]`,
// `for 2 [{a:(flex [i])} i]`, `for 2 [[i]]` — was the interpreter's
// `undefined word: i` (the residual literal was left pending and evaluated at
// the end of the run, after the loop unbound `i`; with an OUTER `i` bound it
// read that one instead) and the compiled lane's value per iteration. The
// interpreter's loop region evaluates a pending residual container when it
// collects the iteration's output, with the iterator still bound
// (Engine.collectLoopRegion), as a fn frame evaluates its body's residual
// in-frame; both lanes answer the value. The do-body twin on the COMPILED
// lane — `for 2 [do [[i]]]`, a single container literal body, baked as a
// const the handler re-ran through the interpreter where the loop's `i` is
// a frame slot the registry never held (`error(undefined word: i)` for the
// interpreter's `[0]`) — closes with it: a token body compiles in-frame
// (recordClosureDispatch's bodyInFrame), so its residual records its
// assembly and the closure unit assembles the list from the captured slot.
func TestLoopBodyResidualLiteralResolves(t *testing.T) {
	for _, src := range []string{
		`for 2 [[(i add 1)] i]`,
		`for 2 [{a:(i add 1)} i]`,
		`for 2 [{a:(flex [i])} i]`,
		`for 2 [[i]]`,
		`for 2 [[i add 1]]`,
		`for 2 [{a:i}]`,
		`for 2 [[(i add 1)]]`,
		`for 2 [[(i add 1)] [i]]`,
		`for 2 [[[i]]]`,
		`for 2 [{a:[i]}]`,
		`for 2 [[{a:i}]]`,
		`for [2 4] [[i] i]`,
		`for 2 [for 2 [[(i add 1)]]]`,
		`for 2 [def k (i mul 2) [k]]`,
		`for 3 [if (i eq 1) [break] [] [i]]`,
		`for 3 [if (i eq 1) [continue] [] [i]]`,
		`for 2 [if true [[(i add 1)]] [[0]]]`,
		`for 0 [[i]]`,
		// An OUTER binding of the loop variable's name: the literal reads
		// the loop's own, never the outer one.
		`def i 9 for 2 [[(i add 1)] i]`,
		// A while body's literal reads the iteration's binding, not the
		// loop's last.
		`def i 0 while [i lt 2] [[(i add 1)] def i (i add 1)]`,
		// Inside a `do` and a fn frame.
		`do [for 2 [[(i add 1)] i]]`,
		`def f fn [[][List][for 1 [[(i add 1)]]]] end f`,
		// The do-body twin: a single container literal body under the loop,
		// and its multi-token neighbours.
		`for 2 [do [[i]]]`,
		`for 2 [do [{a:i}]]`,
		`for 2 [do [[(i add 1)]] i]`,
		`for 2 [do [[i add 1]]]`,
		`for 2 [do [[(i add 1) (i mul 2)]]]`,
		`for 2 [do [[i] 5]]`,
		`for 2 [do [ [i] [i] ]]`,
		`def i 5 do [[(i add 1)]]`,
		`def f fn [[y:Integer][List][do [[(y add 1)]]]] end f 1`,
		`do [[1 2]]`,
		`do [{a:1}]`,
		// A quoted literal stays code-as-data on both lanes, and a consumed
		// one is the word's operand.
		`for 2 [(quote [i])]`,
		`for 2 [[i] size]`,
	} {
		requireEngineParity(t, src, true)
	}
	// The interpreter's own answers.
	for _, c := range []struct{ src, want string }{
		{`for 2 [[(i add 1)] i]`, "[[1] 0 [2] 1]"},
		{`for 2 [{a:(flex [i])} i]`, "[{a:[0]} 0 {a:[1]} 1]"},
		{`def i 9 for 2 [[(i add 1)] i]`, "[[1] 0 [2] 1]"},
		{`def i 0 while [i lt 2] [[(i add 1)] def i (i add 1)]`, "[[2] [3]]"},
		{`for 2 [do [[i]]]`, "[[0] [1]]"},
	} {
		d := mustNew(t)
		got, err := d.RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
	// The fn shape: the loop's four values are the fn's residual, and the
	// fn's own return-count error is the answer on both lanes.
	src := `def f fn [[][List][for 2 [[(i add 1)] i]]] end f`
	a := mustNew(t)
	a.SetOutput(&bytes.Buffer{})
	_, errI := a.RunInterp(src)
	gotC, _, errC := a.RunCompiled(src)
	if codeOf(errI) != "type_error" || codeOf(errC) != "type_error" {
		t.Errorf("%q: interp %v, compiled %v / %v — both lanes raise the fn's count error", src, errI, gotC, errC)
	}
	// The do-body twin's lowering: the body is a closure unit assembling the
	// list from the loop's slot, never a const the handler re-runs.
	dis := compileDisasm(t, `for 2 [do [[i]]]`)
	if !strings.Contains(dis, "PUSH_CLOSURE") || !strings.Contains(dis, "MAKE_LIST") {
		t.Errorf("for 2 [do [[i]]]: the single-literal do body must compile to a closure assembling its list:\n%s", dis)
	}
	if strings.Contains(dis, "word(i)") {
		t.Errorf("for 2 [do [[i]]]: the body must not bake the loop variable as a const word:\n%s", dis)
	}
}

// TestLoopIteratorTornDownOnTrappedRaise pins NUR201's loop twin, found
// closing NUR197: a `for` loop abandoned by a raise the caller traps left
// its iterator INSTALLED on the interpreter — `def i 9  do [for 2 [raise
// 'x']]  i` read 0 (and `[if (i eq 1) [raise 'x'] []]` under `for 3` read
// 1) where the compiled lane's loop keeps `i` in a frame slot and the read
// answers the root's 9. The fault return unwinds every live loop's iterator
// as a break's region discard does (Engine.unwindLiveLoops), before the
// frames — a loop inside a frame installed its iterator after the frame's
// snapshot, so the frame's truncation has nothing left to pop for the name,
// and a loop enclosing a frame keeps its iterator beneath that snapshot.
func TestLoopIteratorTornDownOnTrappedRaise(t *testing.T) {
	for _, src := range []string{
		`def i 9 do [for 2 [raise 'x']] i`,
		`def i 9 do [for 3 [if (i eq 1) [raise 'x'] []]] i`,
		`def i 9 end do [for 2 [def q i raise 'x']] end i`,
		`def i 9 def g fn [[][Integer][for 2 [raise 'x']]] end do [g] end i`,
		`def i 9 def g fn [[][Integer][def i 7 for 2 [raise 'x']]] end do [g] end i`,
		`def i 9 for 1 [do [for 2 [raise 'x']] drop i]`,
		`def i 9 do [for 2 [for 2 [raise 'x']]] i`,
		`def k 9 do [for [0 2] [raise 'x']] k`,
		`def i 9 def f fn [[n:Integer][Integer][raise 'x']] end do [for 2 [f i]] end i`,
		`def i 9 do [for 2 [[1 2] each [raise 'x']]] i`,
		`do [for 2 [raise 'x']] error [drop 1]`,
		// A while body's def is the caller's own and leaks by design.
		`def i 0 do [while [i lt 2] [def i (i add 1) raise 'x']] i`,
	} {
		requireEngineParity(t, src, true)
	}
	for _, c := range []struct{ src, want string }{
		{`def i 9 do [for 2 [raise 'x']] i`, "[error(x) 9]"},
		{`def i 9 do [for 3 [if (i eq 1) [raise 'x'] []]] i`, "[error(x) 9]"},
		{`def i 9 for 1 [do [for 2 [raise 'x']] drop i]`, "[0]"},
		{`do [for 2 [raise 'x']] i`, "ERROR:undefined_word"},
	} {
		d := mustNew(t)
		got, err := d.RunInterp(c.src)
		if strings.HasPrefix(c.want, "ERROR:") {
			if codeOf(err) != strings.TrimPrefix(c.want, "ERROR:") {
				t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, err, c.want)
			}
			continue
		}
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}

// TestDotChainOnMissingMemberResolves pins NUR198's close: a dotted read
// through a MISSING member — `m.b.c` over `{a:1}`, where `m.b` is None on
// both lanes — is None on both lanes, the chained-read propagation the
// accessor's None row always promised for a string or materialised-atom
// key and never gave a BARE-WORD key: the row carried no QuoteArgs, so the
// interpreter never collected `c`, parked the dispatch, and the word stepped
// on its own as `undefined word: c` where the compiled chain answered None
// (and a STATIC None failed to compile at the same undefined_word). The
// atom row quotes the key now, on `dot` and on its strict twin `dotr`, whose
// bare-word read over None raises the family's not_found on both lanes.
func TestDotChainOnMissingMemberResolves(t *testing.T) {
	for _, src := range []string{
		`def f fn [[m:Map][Any][m.b.c]] end f {a:1}`,
		`def f fn [[m:Map][Any][(m.b).c]] end f {a:1}`,
		`def C class {x:[{y:1}]} end ((make C {}).x get 0).y`,
		`def m {a:1} m.b.c`,
		`def m {a:1} m.b.c.d`,
		`def m {a:1} m dot b dot c`,
		`def m {a:1} (m.b) dot c`,
		`def m {a:1} 5 m.b.c`,
		`def m {a:none} m.a.c`,
		`def e (do [raise 'x']) e.nothere.zz`,
		`none.c`,
		`none dot c`,
		`def n none end n.c`,
		`def f fn [[v:Any][Any][v.c]] end f none`,
		`def xs [none] (xs get 0).c`,
		`def m {a:1} m.b.c eq none`,
		`def m {a:1} m.b has c/q`,
		// The strict twin raises the family's not_found, never undefined_word.
		`def m {a:1} m.b!.c`,
		`none dotr c`,
		`none!.c`,
		// `get` still EVALUATES its key: an unbound bare word is undefined on
		// both lanes, a bound one reads its value.
		`def c 'q' def m {a:1} m.b get c`,
	} {
		requireEngineParity(t, src, true)
	}
	for _, c := range []struct{ src, want string }{
		{`def m {a:1} m.b.c`, "[None]"},
		{`none dot c`, "[None]"},
		{`def m {a:1} 5 m.b.c`, "[5 None]"},
		{`def m {a:1} m.b!.c`, "ERROR:not_found"},
		{`def m {a:1} m.b get c`, "ERROR:undefined_word"},
	} {
		d := mustNew(t)
		got, err := d.RunInterp(c.src)
		if strings.HasPrefix(c.want, "ERROR:") {
			if codeOf(err) != strings.TrimPrefix(c.want, "ERROR:") {
				t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, err, c.want)
			}
			continue
		}
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}
