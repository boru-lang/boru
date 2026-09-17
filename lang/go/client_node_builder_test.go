package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// errCountFor is the shared check-mode error counter for the client node-builder
// regressions below (radix / tst edge-splitter + node-rebuild patterns from
// design/legacy/CLIENT-FIXES-2026-06-24.ignore).
func errCountFor(t *testing.T, src string) int {
	t.Helper()
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

// A binary word with DISJOINT-typed overloads (add: Number+Number, String+Scalar,
// the temporal Date/DateTime/Instant forms) dispatched on TWO fully-unknown
// (dynamic-Any) operands must widen its result to the gradual union including
// Any, not commit to the single matched overload. radix-delete's merged label
// `(lab (gpair get 0) add)` — lab and the sibling label are both get-results of
// statically-unknown type — committed to `Instant`, which is disjoint from the
// `set-edge` `newlabel:String` slot it then fed, so set-edge failed no_signature.
func TestAddTwoUnknownOperandsGradual(t *testing.T) {
	// add over two dynamic-Any operands, result consumed by a String param.
	clean := `def af fn [[x:Any] [Any] [x]] end
def g fn [[s:String] [Integer] [1]] end
def f fn [[a:Any b:Any] [Integer] [ (((af a) (af b) add) g) ]] end`
	if n := errCountFor(t, clean); n != 0 {
		t.Errorf("add of two unknowns -> String param: expected 0 errors, got %d", n)
	}
	// Same result consumed by an Integer param (the Number overload) — also clean.
	clean2 := `def af fn [[x:Any] [Any] [x]] end
def g fn [[n:Integer] [Integer] [1]] end
def f fn [[a:Any b:Any] [Integer] [ (((af a) (af b) add) g) ]] end`
	if n := errCountFor(t, clean2); n != 0 {
		t.Errorf("add of two unknowns -> Integer param: expected 0 errors, got %d", n)
	}
	// NEGATIVE: a CONCRETE String operand pins add to String concat, so the
	// result is String — an Integer consumer of THAT must still be flagged
	// (the widening is gated to the fully-unknown case, not partial-concrete).
	bad := `def af fn [[x:Any] [Any] [x]] end
def g fn [[n:Integer] [Integer] [1]] end
def f fn [[a:Any] [Integer] [ (((af a) "x" add) g) ]] end`
	if n := errCountFor(t, bad); n == 0 {
		t.Errorf("add(unknown, concrete String) is String concat -> Integer consumer must still be flagged")
	}
}

// The optional / builder sentinel `if (nd eq none) [build-node] [nd]` merges a
// freshly-built node (Map) with the still-`none` receiver. When the receiver is
// a CONCRETE none (a direct call `none key val insert`, e.g. tst's `TstMap.set`
// on an empty map), the merge must stay GRADUAL — a strict Disjunct(None|Map)
// makes every downstream node-rebuild `get` fail no_signature because None does
// not conform to Node. The same code reached through a gradual `:Any` param
// already merged dynamically; this pins the concrete-none path to match.
func TestNoneBuilderMergeGradual(t *testing.T) {
	src := `def mk fn [[v:Any nd:Any] [Map] [ do {val: [v], kid: [nd]} ]] end
def step fn [[v:Any nd:Any] [Map] [
  def nd (if (nd eq none) [ none none mk ] [nd])
  (nd "val" get) v mk
]] end
def go fn [[] [Map] [ none 1 step ]] end`
	if n := errCountFor(t, src); n != 0 {
		t.Errorf("builder over a concrete-none seed: expected 0 errors, got %d", n)
	}
}

// A 1-arm `if` (`if cond [v]`) models a VARIADIC 0-or-1 result by merging the
// then-value with a SYNTHETIC None pad. That pad must NOT trigger the
// None-vs-value gradual widening (only a genuine value-vs-none branch does), or
// the chained-variadic-if compile path breaks. Guards the gate that the
// builder-merge widening is scoped to both-arms-real positions.
func TestVariadicIfStaysPrecise(t *testing.T) {
	// Two chained 1-arm ifs (both conditions false) then add — compiles & runs.
	src := `def n 5 if (n eq 0) [98] if (n eq 0) [99] add 1 2`
	a, _ := lang.New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	if prog == nil {
		t.Fatalf("expected native compile, refused: %s", reason)
	}
}

// A `none` value threaded as a dynamic carrier into a fn body must not crash the
// fn-body memo keying: a None root node has a nil Parent (it IS its own lattice
// node), and FnAnalysisKey / the generalised-args build dereferenced it. tst's
// `none entries [… tst-insert] fold` reached this path. Assert no panic and a
// clean (or at worst diagnostic, never crash) result.
func TestNoneDynamicCarrierNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on none dynamic carrier into fn body: %v", r)
		}
	}()
	src := `def ins fn [[v:Any nd:Any] [Any] [ if (nd eq none) [ do {v: [v]} ] [nd] ]] end
def build fn [[entries:List] [Any] [
  none entries [ var [[e acc] (acc (e get 0) ins) ] ] fold
]] end`
	// Must not panic; the build helper is a clean gradual builder.
	if n := errCountFor(t, src); n != 0 {
		t.Errorf("none-seeded fold builder: expected 0 errors, got %d", n)
	}
}
