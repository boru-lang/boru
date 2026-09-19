package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// --- Function signatures as types ---
//
// `def Mapper fnsig [[Integer] [Integer]]` installs `Mapper` as a
// function-shape type — a FnUndef value carrying input + output sig
// lists but no body. Mapper can then be used in the typed-def form
// `def n:Mapper somefn` to constrain n to a function value whose
// signatures structurally match Mapper's.

// A function whose sole sig matches Mapper unifies and is bound.
// The `(quote double)` form passes the function as a value rather than
// invoking it — same idiom boru already uses for higher-order calls.
func TestTypeFnSig_DefBindMatchingFunction(t *testing.T) {
	got := runOne(t, `def Mapper fnsig [[Integer] [Integer]]
def double fn [[Integer] [Integer] [1 add]]
def m:Mapper (quote double)
double 5`)
	// double 5 → 6 (we just use double directly here; the test point
	// is that `def m:Mapper (quote double)` did not error).
	if len(got) != 1 || got[0] != int64(6) {
		t.Errorf("got %v, want [6]", got)
	}
}

// A non-function value fails the typed binding.
func TestTypeFnSig_DefBindRejectsNonFunction(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def Mapper fnsig [[Integer] [Integer]]
def m:Mapper 42`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper 42` (42 is not a function), got nil")
	}
}

// A function whose input types differ from Mapper's fails.
func TestTypeFnSig_DefBindRejectsWrongInputType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def Mapper fnsig [[Integer] [Integer]]
def stringy fn [[String] [Integer] [length]]
def m:Mapper (quote stringy)`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper (quote stringy)` (String != Integer input), got nil")
	}
}

// A function whose return types differ from Mapper's fails.
func TestTypeFnSig_DefBindRejectsWrongReturnType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def Mapper fnsig [[Integer] [Integer]]
def stringer fn [[Integer] [String] [convert String]]
def m:Mapper (quote stringer)`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper (quote stringer)` (returns String != Integer), got nil")
	}
}

// A function with a different arity fails.
func TestTypeFnSig_DefBindRejectsWrongArity(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def Mapper fnsig [[Integer] [Integer]]
def two-arg fn [[Integer Integer] [Integer] [add]]
def m:Mapper (quote two-arg)`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper (quote two-arg)` (arity 2 vs 1), got nil")
	}
}

// Different bound names: a second function-shape type and a function
// that satisfies it; ensures the constraint store is per-name.
func TestTypeFnSig_DistinctNamedShapes(t *testing.T) {
	got := runOne(t, `def Mapper fnsig [[Integer] [Integer]]
def Predicate fnsig [[Integer] [Boolean]]
def double fn [[Integer] [Integer] [1 add]]
def positive fn [[Integer] [Boolean] [n:Integer 0 gt]]
def m:Mapper (quote double)
def p:Predicate (quote positive)
0`)
	// Just confirm both bindings install without error.
	if len(got) != 1 || got[0] != int64(0) {
		t.Errorf("got %v, want [0]", got)
	}
}

// --- Predicate-as-type: fn that returns None on fail / the unified value on ok ---
//
// `def Bbd fn [x:Any Any [if ((x is String) and (x gte "b") and (x lte "d")) [x] [None]]]`
// installs Bbd as a *predicate* type. `def p:Bbd v` calls the predicate
// with `v`; on a non-None return the def installs with the *returned*
// value (which may be a transformed version of v); on a None return
// the def errors and is not installed.

const bbdSource = `def Bbd fn [x:Any Any [if ((x is String) and (x gte "b") and (x lte "d")) [x] [None]]]
`

func TestTypeFnPredicate_DefBindWithinRange(t *testing.T) {
	got := runOne(t, bbdSource+`def p:Bbd "c"
p`)
	if len(got) != 1 || got[0] != "c" {
		t.Errorf("got %v, want [\"c\"]", got)
	}
}

func TestTypeFnPredicate_DefBindBoundary(t *testing.T) {
	for _, val := range []string{"b", "c", "d"} {
		got := runOne(t, bbdSource+`def p:Bbd "`+val+`"
p`)
		if len(got) != 1 || got[0] != val {
			t.Errorf("Bbd boundary %q: got %v, want [%q]", val, got, val)
		}
	}
}

func TestTypeFnPredicate_DefBindOutOfRange(t *testing.T) {
	for _, val := range []string{"a", "e", "z"} {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, err = a.Run(bbdSource + `def q:Bbd "` + val + `"`)
		if err == nil {
			t.Errorf("Bbd out-of-range %q: expected unify error, got nil", val)
		}
	}
}

func TestTypeFnPredicate_DefBindWrongType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	// Integer 99 is not a String, so (x is String) is false → predicate
	// returns None → def errors.
	_, err = a.Run(bbdSource + `def q:Bbd 99`)
	if err == nil {
		t.Fatal("Bbd with Integer 99: expected unify error, got nil")
	}
}

// Predicate types are NOT independently callable. Resolving Bbd by
// name pushes the FnDef value with Quoted=true so the engine treats
// it as data, not a call site — so `Bbd "c"` leaves the FnDef and
// "c" on the stack rather than invoking the predicate. This is the
// "type-defining functions are only used for type operations" rule:
// they participate in def name:T body, v is T, inspect T — never as
// free-standing calls.
func TestTypeFnPredicate_NotIndependentlyCallable(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	result, err := runReference(t, a, bbdSource+`Bbd "c"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// The residual stack should hold the FnDef (rendered by String())
	// and the string "c", not the predicate's return value.
	if len(result) != 2 {
		t.Fatalf("got %v results, want 2 (FnDef + string)", result)
	}
	if result[1] != "c" {
		t.Errorf("residual[1] = %v, want \"c\"", result[1])
	}
}

// A predicate that *transforms* the value: success returns a
// different value (here, the upper-cased form). The def installs
// with the transformed value, not the original input.
func TestTypeFnPredicate_TransformsOnSuccess(t *testing.T) {
	got := runOne(t, `def Up fn [x:Any Any [if (x is String) [x upper] [None]]]
def shout:Up "hello"
shout`)
	if len(got) != 1 || got[0] != "HELLO" {
		t.Errorf("got %v, want [\"HELLO\"]", got)
	}
}

// A predicate over Integer values (ranged constraint expressed as a fn).
func TestTypeFnPredicate_IntegerRange(t *testing.T) {
	got := runOne(t, `def Mid fn [n:Any Any [if ((n is Integer) and (n gte 10) and (n lte 20)) [n] [None]]]
def x:Mid 15
x`)
	if len(got) != 1 || got[0] != int64(15) {
		t.Errorf("got %v, want [15]", got)
	}
}

func TestTypeFnPredicate_IntegerRangeFail(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def Mid fn [n:Any Any [if ((n is Integer) and (n gte 10) and (n lte 20)) [n] [None]]]
def x:Mid 25`)
	if err == nil {
		t.Fatal("Mid with 25: expected unify error, got nil")
	}
}
