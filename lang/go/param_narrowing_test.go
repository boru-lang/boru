package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// A fn body is analysed against its DECLARED param types, not the (possibly
// gradual) arg shapes a call threads in. A mutual-recursion cycle that passes a
// `get`-result `dynamic(Any)` into a `ch:List` param must still see `ch` as a
// List inside the body, so `each` over it matches the LIST overload (→ List)
// rather than the map-each overload (→ Map) — otherwise a downstream
// `all`/`any`/`[List]` consumer fails no_signature against the spurious Map.
// This is the decision client's `eval-pred-all` ↔ `eval-pred` cycle.
func TestParamNarrowingMutualCycle(t *testing.T) {
	errCount := func(src string) int {
		a, _ := lang.New()
		cr, _ := a.Check(src)
		n := 0
		for _, d := range cr.Diagnostics {
			if d.Severity == "error" {
				n++
			}
		}
		return n
	}

	// eval-pred-all calls each+all over eval-pred results; eval-pred recurses
	// back into eval-pred-all, threading a get-result as the children:List arg.
	clean := `
def eval-cond fn [[c:Map input:Map] [Boolean] [true]] end
def eval-pred-all fn [[children:List input:Map] [Boolean] [(children each [input swap eval-pred]) all]] end
def eval-pred-any fn [[children:List input:Map] [Boolean] [(children each [input swap eval-pred]) any]] end
def eval-pred fn [[pred:Map input:Map] [Boolean] [
  if ((pred get "kind") "group" eq)
    [input (pred get "children") eval-pred-all]
    [input pred eval-cond]
]] end`
	if n := errCount(clean); n != 0 {
		t.Errorf("mutual-cycle eval-pred-all/eval-pred: expected 0 errors, got %d", n)
	}

	// NEGATIVE: passing a genuinely wrong concrete type to a List param is still
	// flagged — narrowing only applies to a gradual arg that PASSED the match.
	bad := `def f fn [[xs:List] [Integer] [xs size]] end (f 5)`
	if n := errCount(bad); n == 0 {
		t.Errorf("passing a concrete Integer to a List param must still be flagged")
	}

	// A STRICT Any arg threaded into a declared-`Any` param must be analysed
	// gradually (the param is declared Any → ParamInputCarrier gives a dynamic
	// carrier). The fold-accumulator-as-node shape: an accumulator that started
	// `none` and was rebuilt into a Map is threaded into a `nd:Any` insert
	// receiver, whose body then reads it with `get`/rebuilds it. (tst/radix's
	// `none entries [(acc … ins)] fold`.)
	accAsNode := `
def mk fn [[a:Any b:Any] [Map] [do {x: [a], y: [b]}]] end
def wev fn [[fin:Any nd:Map] [Map] [(nd "x" get) (nd "y" get) mk]] end
def ins fn [[val:Any key:String nd:Any] [Any] [
  def nd (if (nd eq none) [ none none mk ] [nd])
  if ((key size) eq 0) [ (nd val wev) ] [nd]
]] end
def from-entries fn [[entries:List] [Any] [
  none entries [ ([e acc] => [(acc (e get 0) (e get 1) ins)]) apply ] fold
]] end`
	if n := errCount(accAsNode); n != 0 {
		t.Errorf("fold-accumulator-as-node: expected 0 errors, got %d", n)
	}
}

// A DYNAMIC arg whose bound is DISJOINT from (a sibling of, not a supertype of)
// the declared concrete param is analysed at the PARAM's declared type, not the
// arg's bound. The param annotation is the authoritative contract for body
// analysis, and a dynamic arg is "statically unknown, optimistically anything".
// radix's edge splitter feeds `(lab (gpair get 0) add)` / a `slice` result of
// statically-unknown shape — modelled as a dynamic List — into `set-edge`'s
// `newlabel:String`; without re-binding the body's view to String, the body's
// `newlabel slice 0 1` stayed List and failed `set`'s String-key Map overload.
func TestParamNarrowingDisjointDynamicArg(t *testing.T) {
	errCount := func(src string) int {
		a, _ := lang.New()
		cr, _ := a.Check(src)
		n := 0
		for _, d := range cr.Diagnostics {
			if d.Severity == "error" {
				n++
			}
		}
		return n
	}

	// The tst node-rebuild shape: `(nd "ch" get)` reads a node field that is a
	// dynamic carrier with a NON-Any (non-String) bound and feeds it to
	// mk-tnode's `ch:String` param. The body must see `ch` at its declared
	// String type so mk-tnode dispatches; otherwise it fails no_signature.
	clean := `
def mk-tnode fn [[lo:Any val:Any ch:String] [Map] [ do {ch: [ch], val: [val], lo: [lo]} ]] end
def with-lo fn [[lo:Any nd:Map] [Map] [ (mk-tnode (nd "lo" get) (nd "val" get) (nd "ch" get)) ]] end
def ins fn [[val:Any ch:String nd:Any] [Any] [
  def nd2 (if (nd eq none) [ (mk-tnode none none ch) ] [nd])
  (with-lo (nd2 "lo" get) nd2)
]] end
def go fn [[] [Any] [ (ins 1 "k" none) ]] end`
	if n := errCount(clean); n != 0 {
		t.Errorf("disjoint dynamic field -> String param: expected 0 errors, got %d", n)
	}

	// NEGATIVE: a CONCRETE List arg to a String param is a genuine type error
	// and must still be flagged — only a gradual (dynamic) arg is re-bound.
	bad := `
def use fn [[s:String] [Integer] [ 1 ]] end
def f fn [[] [Integer] [ [1 2 3] use ]] end`
	if n := errCount(bad); n == 0 {
		t.Errorf("a concrete List passed to a String param must still be flagged")
	}
}
