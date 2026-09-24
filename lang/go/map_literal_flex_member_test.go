package lang

import (
	"bytes"
	"fmt"
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

// TestLoopBodyResidualLiteralPending pins NUR197 as it stands: a loop body
// whose residual is a literal bearing a paren group over the loop variable
// — `for 2 [[(i add 1)] i]`, `for 2 [{a:(flex [i])} i]` — is the
// interpreter's `undefined word: i` (the residual literal is evaluated at the
// end of the run, after the loop unbound `i`) and the compiled lane's value
// (the assembly recorded in the body, `i` bound). Found while closing the
// flex-member map, present on main before it; closing it is loud here.
func TestLoopBodyResidualLiteralPending(t *testing.T) {
	for _, c := range []struct{ src, wantC string }{
		{`for 2 [[(i add 1)] i]`, "[[1] 0 [2] 1]"},
		{`for 2 [{a:(i add 1)} i]`, "[{a:1} 0 {a:2} 1]"},
		{`for 2 [{a:(flex [i])} i]`, "[{a:[0]} 0 {a:[1]} 1]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if codeOf(errI) != "undefined_word" || len(gotI) != 0 {
			t.Errorf("%q: the interpreter raises undefined_word on the deferred residual: %v / %v", c.src, gotI, errI)
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != c.wantC {
			t.Errorf("%q: NUR197's compiled value %v / %v (compiled=%v), pinned as %s — closing the divergence must update this pin", c.src, gotC, errC, compiled, c.wantC)
		}
	}
	// Inside a fn the same residual is the fn body's: the interpreter's
	// undefined_word again, where the compiled lane assembles four values
	// and raises the fn's own return-count error — pinned as it stands.
	src := `def f fn [[][List][for 2 [[(i add 1)] i]]] end f`
	a := mustNew(t)
	a.SetOutput(&bytes.Buffer{})
	_, errI := a.RunInterp(src)
	gotC, _, errC := a.RunCompiled(src)
	if codeOf(errI) != "undefined_word" || codeOf(errC) != "type_error" {
		t.Errorf("%q: interp %v, compiled %v / %v — NUR197's fn shape moved; update the pin", src, errI, gotC, errC)
	}
}

// TestDotChainOnMissingMemberPending pins NUR198 as it stands: a dotted read
// through a MISSING member — `m.b.c` over `{a:1}`, where `m.b` is None on both
// lanes — is the interpreter's `undefined word: c` (its `dot` over a run-time
// None leaves the atom unconsumed, which steps as a word) and the compiled
// lane's None. A STATIC None fails to compile at the same undefined_word,
// consistently. Found while closing the flex-member map, present on main
// before it; closing it is loud here.
func TestDotChainOnMissingMemberPending(t *testing.T) {
	for _, src := range []string{
		`def f fn [[m:Map][Any][m.b.c]] end f {a:1}`,
		`def f fn [[m:Map][Any][(m.b).c]] end f {a:1}`,
		`def C class {x:[{y:1}]} end ((make C {}).x get 0).y`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "undefined_word" || len(gotI) != 0 {
			t.Errorf("%q: the interpreter raises undefined_word on the unconsumed atom: %v / %v", src, gotI, errI)
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != "[None]" {
			t.Errorf("%q: NUR198's compiled None %v / %v (compiled=%v) — closing the divergence must update this pin", src, gotC, errC, compiled)
		}
	}
	for _, src := range []string{`def f fn [[v:Any][Any][v.c]] end f none`, `def xs [none] (xs get 0).c`} {
		gotC, compiled, errC, _, errI := runBothEngines(t, src)
		if codeOf(errI) != "undefined_word" || compiled || codeOf(errC) != "compile_failed" || len(gotC) != 0 {
			t.Errorf("%q: a static None is the same undefined_word on both lanes (the compiled lane at the check pass): interp %v, compiled %v / %v", src, errI, gotC, errC)
		}
	}
}
