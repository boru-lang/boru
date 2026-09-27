package lang

import (
	"fmt"
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
