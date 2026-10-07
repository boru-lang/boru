package lang

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The def census end to end (core/go/def_census.go, design/IMMUTABLE-DEF.1.md
// §5 phase 0): the check pass classifies the programs the plan's §2.9 table
// names, through the real `def`, `if`, `for`, `each`, `do`, `undef`, `var`
// and `import` words. Each row states the findings expected, in source
// order, as "class:name"; a row with none pins what the rule leaves alone.

func censusOf(t *testing.T, src string) []core.DefCensusEntry {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := a.Check(src)
	return res.DefCensus
}

func censusClasses(census []core.DefCensusEntry) string {
	parts := make([]string, len(census))
	for i, e := range census {
		parts[i] = string(e.Class) + ":" + e.Name
	}
	return strings.Join(parts, " ")
}

func TestDefCensusPrograms(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{"a first binding", `def a 1 a`, ""},
		{"a module rebind", `def a 1 def a 2 a`, "rebind:a"},
		{"the loop counter: a block shadow that read the name", `def n 0 for 3 [def n (n add 1)] n`, "shadow-rebind:n"},
		{"the callback accumulator", `def t 0 each [def t (t add 1) t] [1 2 3]`, "shadow-rebind:t"},
		{"a while counter", `def n 0 while [n lt 3] [def n (n add 1)] n`, "shadow-rebind:n"},
		{"a block-local def that shadows without reading", `def x 1 for 2 [def x 9] x`, "shadow:x"},
		{"a fresh block def read after the block", `for 2 [def y 9] y`, "leak-read:y"},
		{"a fresh each-body def read after the each", `each [def y 9] [1 2] y`, "leak-read:y"},
		{"an each-body shadow read after the each resolves to the module's", `def y 1 each [def y 9] [1 2] y`, "shadow:y"},
		// (A constant condition analyses the taken arm only.)
		{"arm shadows read after the branch resolve to the frame's", `def f fn [[c:Boolean] [Integer] [def w 0 if c [def w 1] [def w 2] w]] f true`, "shadow:w shadow:w"},
		{"an arm's def read after the branch", `if true [def w 1] [def w 2] w`, "leak-read:w"},
		{"an arm's def read only inside the arm", `if true [def w 1 w] [2]`, ""},
		{"do is transparent: one def", `do [def q 5] q`, ""},
		{"do is transparent: two defs rebind", `do [def q 5] do [def q 6] q`, "rebind:q"},
		{"a fn body's value shadow", `def r 1 def h fn [[] [Integer] [def r 2 r]] (h) r`, "shadow:r"},
		{"a body def over a param", `def k fn [[n:Integer] [Integer] [def n 5 n]] k 1`, "rebind:n"},
		{"an added overload", `def f fn [[x:Integer] [Integer] [1]] def f fn [[x:String] [Integer] [2]] (f 1) (f "s")`, ""},
		{"an overlapping overload", `def f fn [[x:Integer] [Integer] [1]] def f fn [[x:Integer] [Integer] [2]] f 1`, "overlap:f"},
		{"an overload from a fn body", `def f fn [[x:Integer] [Integer] [1]] def g fn [[] [Integer] [def f fn [[x:String] [Integer] [2]] 0]] (g)`, "extend-inner:f"},
		{"a value over a word from a fn body", `def f fn [[x:Integer] [Integer] [1]] def g fn [[] [Integer] [def f 5 f]] (g)`, "shadow:f"},
		// A caller's frame-local fn is visible to the callee but encloses
		// nothing of it: each fn keeps its own `loop`.
		{"a callee's local fn over its caller's", `def g fn [[] [Integer] [def loop fn [[i:Integer] [Integer] [i]] loop 1]] def h fn [[] [Integer] [def loop fn [[i:Integer] [Integer] [i add 1]] (g)]] (h)`, ""},
		{"a callee's local value over its caller's", `def g fn [[] [Integer] [def t 2 t]] def h fn [[] [Integer] [def t 1 (g) add t]] (h)`, ""},
		// A loop body's arm defining a name it reads in the same arm: the
		// model's bookkeeping between analysis rounds is no read of it.
		{"an arm's def read in the arm, in a loop body", `def f fn [[k:Integer] [List] [for 2 [if (k gt 0) [def issue 1 (push issue [])] [[]]]]] f 1`, ""},
		{"an arm's def read in the arm, in a fold lambda", `def f fn [[k:Integer] [Map] [fold ([c acc] => [if (k gt 0) [def issue 1 {issues:issue}] [{issues:1}]]) [1 2] {issues:[]}]] f 1`, ""},
		{"a type alias redefined", `def Foo Integer def Foo String 1 is Foo`, "rebind:Foo"},
		{"a type minted twice", `def Rec {a:Integer} def Rec {b:Integer} 1`, "rebind:Rec"},
		{"undef", `def a 1 undef a`, "undef:a"},
		// A repeat import of one native module is a cache no-op today
		// (edge-modules-1.tsv), as the rule has it; a second module bound to
		// the same namespace is a rebind.
		{"a native module imported twice", `import "boru:math-util" import "boru:math-util" 1`, ""},
		{"two modules on one namespace", `import module [export "M" {a:1}] import module [export "M" {a:2}] 1`, "rebind:M"},
	}
	for _, row := range rows {
		got := censusClasses(censusOf(t, row.src))
		if got != row.want {
			t.Errorf("%s: %s\n  census %q\n  want   %q", row.name, row.src, got, row.want)
		}
	}
}

// The findings carry both sites and the scope.
func TestDefCensusSites(t *testing.T) {
	census := censusOf(t, `def a 1 def a 2 a`)
	if len(census) != 1 {
		t.Fatalf("census %v", census)
	}
	e := census[0]
	if e.Site.Row != 1 || e.Site.Col != 13 || e.Standing.Row != 1 || e.Standing.Col != 5 || e.Scope != core.ScopeModule || e.Note != "value" {
		t.Errorf("the rebind finding: %+v", e)
	}
	census = censusOf(t, `if true [def w 1] [def w 2] w`)
	if len(census) != 1 || census[0].Site.Col != 29 || census[0].Standing.Row != 1 {
		t.Errorf("the leak read names the read and the arm's binding: %+v", census)
	}
	census = censusOf(t, `def n 0 for 3 [def n (n add 1)] n`)
	if len(census) != 1 || census[0].Scope != core.ScopeBlock || census[0].Standing.Col != 5 || census[0].Fn != "" {
		t.Errorf("the shadow-rebind names the block and the shadowed def: %+v", census)
	}
	census = censusOf(t, `def k fn [[n:Integer] [Integer] [def n 5 n]] k 1`)
	if len(census) != 1 || census[0].Class != core.CensusRebind || census[0].Note != "param" || census[0].Fn != "k" || census[0].Scope != core.ScopeFrame {
		t.Errorf("a body def over a param names the fn: %+v", census)
	}
	// The interpreter's run records nothing: the census is the check pass's.
	a, _ := New()
	if _, err := a.RunInterp(`def a 1 def a 2 a`); err != nil {
		t.Fatal(err)
	}
	if n := len(a.NativeRegistry().Check.DefCensus); n != 0 {
		t.Errorf("the interpreter recorded %d findings", n)
	}
}
