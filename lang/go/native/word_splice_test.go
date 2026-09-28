package native

import (
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// runSplice parses and runs boru source, returning the canonical render of the
// final stack. Splice (`word` / `def name word value`) is a source-level
// feature (forward collection, def-deref, container auto-eval), so the tests
// exercise it through the real parser rather than hand-built token slices.
func runSplice(t *testing.T, src string) string {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return Canon(out)
}

func runSpliceErr(t *testing.T, src string) error {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, err := parser.Parse(src)
	if err != nil {
		return err
	}
	_, runErr := NewTop(r).Run(toks)
	return runErr
}

func TestWordSplice(t *testing.T) {
	cases := []struct{ src, want string }{
		// --- standalone `word`: scalar splices itself, list splices elements ---
		{"word 42", "42"},
		{"word [1,2,3]", "1 2 3"},
		{"word []", ""},
		// --- macro: spliced words execute against the live stack ---
		{"5 word [dup add]", "10"},
		{"1 2 word [add] 10", "1 12"}, // spliced `add` forward-collects 10: 2 add 10 = 12
		// --- def name word value: binds the splice marker ---
		{"def dbl word [dup add] 5 dbl", "10"},
		{"def xs word [1,2,3] [xs]", "[1 2 3]"},
		{"def n word 42 n add 1", "43"},
		// --- value stored raw/unevaluated; re-evaluated when spliced+run ---
		{"def p word [1 add 2] [p]", "[3]"},
		// --- splices inside containers when the container is evaluated ---
		{"def xs word [1,2,3] [0 xs 4]", "[0 1 2 3 4]"},
		{"def a word [1,2] def b word [a 3] [b]", "[1 2 3]"}, // nested splice flattens
		{"def e word [] [e 9]", "[9]"},                       // empty splice contributes nothing
		// --- def list now binds the evaluated value (implicit splice removed) ---
		{"def xs [1,2,3] size xs", "3"},          // a def'd list is a value, not a splice
		{"def xs [1 add 2] xs", "[3]"},           // evaluated at def time, like a map
		{"size [1,2,3]", "3"},                    // list-as-arg auto-eval unaffected
		{"[1 add 2]", "[3]"},                     // bare list auto-eval unaffected
		{"quote [dup add]", "(quote [dup add])"}, // quote unaffected
	}
	for _, c := range cases {
		if got := runSplice(t, c.src); got != c.want {
			t.Errorf("%q = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestWordSpliceMacroFnCapture pins that a `word`-macro whose expansion
// constructs an inner fn, used inside an outer fn body, still captures the
// outer fn's param. The macro hides the inner `fn` behind a bare word, so the
// frame-state analysis must resolve the splice to keep the enclosing-fn
// baseline (regression guard for the interpreter frame-state optimization).
func TestWordSpliceMacroFnCapture(t *testing.T) {
	src := `def mk word [fn [[x:Integer] [Integer] [x add y]]]
def wrapper fn [[y:Integer] [Function] [mk]]
def f (wrapper 100)
f 5`
	if got := runSplice(t, src); got != "105" {
		t.Errorf("macro-fn capture = %q, want 105", got)
	}
}

func TestWordSpliceErrors(t *testing.T) {
	// A spliced bare word that can't run against the stack still errors —
	// the splice is unevaluated but the resulting tokens are real code.
	if err := runSpliceErr(t, "word [dup]"); err == nil {
		t.Errorf("word [dup] on empty stack: expected error, got none")
	}
	// `word` with no argument has nothing to wrap and must not silently
	// succeed as a bare value.
	if err := runSpliceErr(t, "word"); err == nil {
		t.Errorf("bare `word`: expected error, got none")
	}
}

// TestSpliceValueAPI pins the eng value constructors/accessors directly.
func TestSpliceValueAPI(t *testing.T) {
	inner := NewInteger(7)
	sp := NewSplice(inner)

	if !IsSplice(sp) {
		t.Fatalf("IsSplice(NewSplice(...)) = false, want true")
	}
	if IsSplice(inner) {
		t.Errorf("IsSplice on a plain integer = true, want false")
	}
	info, err := AsSplice(sp)
	if err != nil {
		t.Fatalf("AsSplice: unexpected error %v", err)
	}
	if got, _ := AsInteger(info.Data); got != 7 {
		t.Errorf("AsSplice payload = %v, want 7", got)
	}
	if _, err := AsSplice(inner); err == nil {
		t.Errorf("AsSplice on a non-splice value: expected error, got none")
	}
}

// TestSpliceTypedListSplicesAsValue verifies the "plain lists only" rule:
// a typed list is NOT element-spliced; it splices as a single value (mirroring
// the existing implicit def-list splice guard). Built via the value API since
// a typed list with inline elements has no convenient source literal.
func TestSpliceTypedListSplicesAsValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	typed := NewTypedListWithElements(
		NewTypeLiteral(TInteger),
		[]Value{NewInteger(1), NewInteger(2), NewInteger(3)},
	)
	if !IsTypedList(typed) {
		t.Fatalf("fixture is not a typed list")
	}
	out, err := NewTop(r).Run([]Value{NewSplice(typed)})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// A plain list would have spliced to three stack entries; the typed
	// list must splice as exactly one value.
	if len(out) != 1 {
		t.Fatalf("typed list splice produced %d stack entries, want 1: %q", len(out), Canon(out))
	}
	if !IsTypedList(out[0]) {
		t.Errorf("spliced value is not the original typed list: %q", Canon(out))
	}
}
