package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The guarded fast paths over generic dispatch (the review of discarded
// type information, 2026-09-27): each keeps a precise fact for the common
// case, checks it where it is used, and takes the generic path otherwise.
// Every shape here must answer what the interpreter answers.

func requireCompiledParity(t *testing.T, src string) {
	t.Helper()
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if !compiled || fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q:\n  compiled %v [%v] (compiled=%v)\n  interp   %v [%v]", src, gotC, errC, compiled, gotI, errI)
	}
}

// #1 the poly inline cache: a site whose operand tags change between
// executions re-matches and picks the other overload; a site whose word
// gains an overload between executions re-matches too.
func TestPolyInlineCacheParity(t *testing.T) {
	for _, src := range []string{
		// size over a List, then a String, then a List again, at one site
		`def f fn [[m:Map][Integer][size m.a]]  [(f {a:[1 2 3]}) (f {a:"xy"}) (f {a:[1]})]`,
		// get over a List then a Map at one site
		`def f fn [[m:Map][Any][m.b get m.k]]  [(f {b:[10 20] k:1}) (f {b:{x:5} k:"x"})]`,
		// the same site in a loop, monomorphic
		`def f fn [[m:Map][Integer][m.a add m.b]]  def v {a:1 b:2}  0 fold [drop (f v) add] (range 0 50)`,
		// a no-match after a hit still raises the interpreter's error
		`def f fn [[m:Map][Integer][size m.a]]  [(f {a:"xy"}) (f {a:5})]`,
	} {
		requireCompiledParity(t, src)
	}
}

// #2 the seeded poly pick: the checker's own overload, dispatched directly
// while the runtime operands carry the checked tags (or match a word's only
// overload); a FlexMap/FlexList where a Map/List was checked misses the
// guard and re-matches.
func TestPolySeedParity(t *testing.T) {
	for _, src := range []string{
		`def f fn [[m:Map][Any][m.a]]  [(f {a:1}) (f (flex {a:2}))]`,
		`def f fn [[m:Node][Any][m get "a"]]  [(f {a:1}) (f (flex {a:2}))]`,
		`def f fn [[xs:List][Any][xs get 0]]  [(f [1 2]) (f (flex [3 4]))]`,
		`def f fn [[m:Map][Any][m.a is None]]  [(f {a:1}) (f {b:2})]`,
	} {
		prog, reason, _, err := mustNew(t).CompileCheck(src)
		if prog == nil || err != nil {
			t.Fatalf("%q: %s %v", src, reason, err)
		}
		seeds := 0
		for _, pr := range prog.PolyRefs {
			if pr.Seed != nil {
				seeds++
			}
		}
		if seeds == 0 {
			t.Errorf("%q: want a seeded poly site", src)
		}
		requireCompiledParity(t, src)
	}
}

// #4 shape specialisation: a plain Map to an untyped `m:Map` param compiles
// a unit keyed on the map's exact shape, whose field reads are strict and
// whose dispatches over them commit; another shape (an extra key, another
// tag, a FlexMap) fails the entry guard and runs the fn itself.
func TestShapeSpecialisation(t *testing.T) {
	src := `def f fn [[m:Map][Integer][m.a add m.b]]  def v {a:1 b:2}  0 fold [drop (f v) add] (range 0 50)`
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog == nil || err != nil {
		t.Fatalf("%s %v", reason, err)
	}
	body := unitBody(prog.Disassemble(), "spec [l0=record{a:Integer b:Integer}]")
	if !strings.Contains(body, "CALL_NATIVE s") || strings.Contains(body, "POLY") {
		t.Errorf("the shape unit's reads and add commit:\n%s", body)
	}
	requireCompiledParity(t, src)
	for _, src := range []string{
		`def f fn [[m:Map][Any][m.a]]  [(f {a:1}) (f (flex {a:2})) (f {a:5})]`,
		`def f fn [[m:Map][Integer][size m]]  [(f {a:1}) (f {a:1 b:2})]`,
		`def f fn [[m:Map][Any][m has "z"]]  [(f {a:1}) (f {a:1 z:2})]`,
		`def f fn [[m:Map][Any][m.a]]  def g fn [[x:Map][Any][f x]]  [(f {a:1}) (g {a:"s"}) (g {a:1 b:2})]`,
		// List shapes: exact length and element tags
		`def f fn [[xs:List][Integer][(xs get 0) add (xs get 1)]]  def l [1 2 3]  0 fold [drop (f l) add] (range 0 50)`,
		`def f fn [[xs:List][Any][xs get 5]]  [(f [1 2]) (f [1 2 3 4 5 6 7])]`,
		`def f fn [[xs:List][Any][size xs]]  [(f [1 2]) (f [1 2 3])]`,
		`def f fn [[xs:List][Any][xs get 0]]  [(f [1 2]) (f (flex [3 4])) (f ["a" "b"])]`,
		`def f fn [[xs:List][Any][each [mul 2] xs]]  [(f [1 2]) (f [3 4 5])]`,
		// shapes the generic path pins as declines / stored-sig poly
		`def wrapfn fn [[m:Map] [Integer] [def helper fn [[a:Integer] [Integer] [a mul 2] [b:String] [Integer] [7]] helper (m get k/q)]] wrapfn {k:3}`,
		`def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]]  def f fn [[m:Map][Integer][((mk 1) m.x) mul 10]]  f {x: 2}`,
		// a behaviour-bearing field (a reach) keeps the generic path
		`def add2 a:Integer => [b:Integer => [add a b]]  def w fn [[m:Map x:Integer][Any][x (m get "f") apply]]  w {f: 3} 4`,
		// record-typed params (the entry contract still checks R)
		`def R {a:Integer b:Integer}  def f fn [[m:R][Integer][m.a add m.b]]  def v {a:1 b:2}  0 fold [drop (f v) add] (range 0 50)`,
		`def R {a:Integer b:Integer}  def f fn [[m:R][Integer][m.a add m.b]]  [(f {a:1 b:2}) (f {a:1 b:2 c:"x"})]`,
		`def R {a:Integer}  def f fn [[m:R][Any][m.a]]  [(f {a:1}) (f {a:"s"})]`,
	} {
		requireCompiledParity(t, src)
	}
}

// #5 a declared return the body proves narrower (`[Any]` over `x add 1`,
// `[List]` over a list of Integers) types the caller's use of the result.
func TestProvenNarrowerReturnParity(t *testing.T) {
	src := `def g fn [[x:Integer][Any][x add 1]]  (g 3) mul 2`
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog == nil || err != nil {
		t.Fatalf("%s %v", reason, err)
	}
	if strings.Contains(prog.Disassemble(), "POLY") {
		t.Errorf("the [Any] result proven Integer commits the mul:\n%s", prog.Disassemble())
	}
	for _, src := range []string{
		src,
		`def mk fn [[n:Integer][List][[n (n add 1)]]]  ((mk 3) get 0) mul 2`,
		`def g fn [[x:Integer][Number][x add 1]]  (g 3) mul 2`,
		`def g fn [[x:Any][Any][x]]  [((g 3) add 1) ((g "a") add "b")]`,
		`def g fn [[x:Integer][Any][if (x gt 0) [x] ["neg"]]]  [(g 3) (g -1)]`,
		`def g fn [[x:Integer][Any][x add 1]]  0 fold [drop ((g 3) mul 2) add] (range 0 20)`,
	} {
		requireCompiledParity(t, src)
	}
}
