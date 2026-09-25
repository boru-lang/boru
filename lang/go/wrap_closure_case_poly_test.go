package lang

import (
	"fmt"
	"testing"
)

// TestWrapWordsBridgeACompiledClosure pins NUR158's close: the four
// dispatch-modifier words bridge a compiled closure (a capturing lambda read
// out of a container) to its fn definition before asserting the payload.
func TestWrapWordsBridgeACompiledClosure(t *testing.T) {
	const m = "def mk fn [[k:Integer] [Function] [([a:Integer b:Integer] => [(a sub b) add k])]] end def mk2 fn [[] [Map] [{a:(mk 100)}]] end def m (mk2) end "
	for _, src := range []string{
		m + "m.a/u 10 3",
		m + "usurp (m.a) 10 3",
		m + "10 3 m.a/s",
		m + "m.a/f 10 3",
		m + "m.a/2 10 3",
		m + "force-arity 2 (m.a) 10 3",
		m + "m.a 10 3",
	} {
		requireSameVerdict(t, src)
	}
	for _, c := range []struct{ src, want string }{
		{m + "m.a/u 10 3", "[93]"},
		{m + "m.a/2 10 3", "[107]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}

// TestSamePredicateReferencesUnify pins NUR157's close: two references to
// one predicate type unify as the type under a registry. The `unify` word
// is uncompilable, so the compiled lane answers by fallback (parity).
func TestSamePredicateReferencesUnify(t *testing.T) {
	const pos = "def Pos fnpred n:Integer [n gt 0] "
	for _, c := range []struct{ src, want string }{
		{pos + "([:Pos] unify [:Pos])", "[[:fn (Integer)] true]"},
		{pos + "def T fnsig [[xs:[:Pos]] [Boolean]] ((fn [[xs:[:Pos]] [Boolean] [true]]) unify T)", "[fn (List) true]"},
		{pos + "def Neg fnpred n:Integer [n lt 0] ([:Pos] unify [:Neg])", "[~unify-fail false]"},
		{pos + "([1 2] unify [:Pos])", "[[1 2] true]"},
		{pos + "([1 -2] unify [:Pos])", "[~unify-fail false]"},
		{pos + "(5 unify Pos)", "[5 true]"},
		{pos + "(-5 unify Pos)", "[~unify-fail false]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
		requireEngineParity(t, c.src, false)
	}
	// The stack spelling leaves no dynamic lead beside a value inside a
	// paren, so it compiles, and the two lanes agree (fnpred.tsv's rows).
	for _, src := range []string{
		pos + "[:Pos] [:Pos] unify",
		pos + "def Neg fnpred n:Integer [n lt 0] [:Neg] [:Pos] unify",
		pos + "[1 -2] [:Pos] unify",
	} {
		requireEngineParity(t, src, true)
	}
}

// TestCaseComputedClauseListDeclines pins NUR154's close as a sound decline:
// a `case` over a clause list a fn returns declines the compile (the check
// pass no longer traps it) and the fallback answers as the interpreter.
func TestCaseComputedClauseListDeclines(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"def mk fn [[n:Integer] [List] [quote [1 'one' 'many']]] end case 1 (mk 0)", "[one]"},
		{`import module [def cl fn [[][List][[1 'one' 2 'two' 'many']]] export "M" {cl: cl/v}] end case 2 M.cl`, "[two]"},
	} {
		requireEngineParity(t, c.src, false)
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
	for _, src := range []string{
		"case 1 [1 'one' 'many']",
		"case 1 5",
		"def cl [1 'one' 'many'] case 1 cl",
	} {
		requireSameVerdict(t, src)
	}
}

// TestPolySeatRetriesOtherArities pins NUR147's close: a poly seat whose
// recorded count the run refutes retries the word's other overload arities
// over the same stack top before it defers.
func TestPolySeatRetriesOtherArities(t *testing.T) {
	for _, src := range []string{
		"def svc (service {}) end add {} ([r:Map state:Any] => [1]) svc call {} svc call {} svc",
		"def svc (service {}) end add {} ([r:Map state:Any] => [1]) svc call {} svc",
		"def svc (service {}) end add {} ([r:Map state:Any] => [1]) svc call {} svc {}",
	} {
		requireSameVerdict(t, src)
	}
	got, err := mustNew(t).RunInterp("def svc (service {}) end add {} ([r:Map state:Any] => [1]) svc call {} svc call {} svc")
	if err != nil || fmt.Sprint(got) != "[1 1]" {
		t.Errorf("interpreter = %v / %v, want [1 1]", got, err)
	}
}
