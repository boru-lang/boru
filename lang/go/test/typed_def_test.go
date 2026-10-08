package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// Surface-syntax tests for `def name:*Type value`.
//
// At the top level, jsonic parses `name:*Type` as a single-pair map,
// so the new [Map, Any] sig on `def` picks it up and uses the map's
// only key as the name and its only value as the type constraint.
// The body must unify with the constraint or the def errors and is
// not installed.

func runOne(t *testing.T, src string) []any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	result, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return result
}

// `def x:Integer 1; x` → 1
func TestTypedDefIntegerLiteralSurface(t *testing.T) {
	got := runOne(t, "def x:Integer 1\nx")
	if len(got) != 1 || got[0] != int64(1) {
		t.Errorf("got %v, want [1]", got)
	}
}

// `def x:Integer "no"` → unify error.
func TestTypedDefIntegerLiteralSurfaceFailure(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def x:Integer "nope"`)
	if err == nil {
		t.Fatal("expected unify error for `def x:Integer \"nope\"`, got nil")
	}
}

// `def x:String "hi"; x` → "hi"
func TestTypedDefStringSurface(t *testing.T) {
	got := runOne(t, `def x:String "hi"
x`)
	if len(got) != 1 || got[0] != "hi" {
		t.Errorf("got %v, want [\"hi\"]", got)
	}
}

// `def G10 (Integer gt 10); def n:G10 11; n` → 11
func TestTypedDefNamedDepTypeSurface(t *testing.T) {
	got := runOne(t, `def G10 (Integer gt 10)
def n:G10 11
n`)
	if len(got) != 1 || got[0] != int64(11) {
		t.Errorf("got %v, want [11]", got)
	}
}

// `def G10 (Integer gt 10); def n:G10 5` → unify error.
func TestTypedDefNamedDepTypeSurfaceFailure(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def G10 (Integer gt 10)
def n:G10 5`)
	if err == nil {
		t.Fatal("expected unify error for `def n:G10 5` (5 not gt 10), got nil")
	}
}

// Multiple typed defs in sequence work. Float comes back through
// lang.Run's catch-all default branch as a stringified value, so we
// just check non-nil + the integer/string entries.
func TestTypedDefMultipleBindings(t *testing.T) {
	got := runOne(t, `def a:Integer 1
def b:String "two"
def c:Float 3.5
a b c`)
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 results", got)
	}
	if got[0] != int64(1) {
		t.Errorf("a = %v, want 1", got[0])
	}
	if got[1] != "two" {
		t.Errorf("b = %v, want \"two\"", got[1])
	}
	if got[2] != "3.5" {
		t.Errorf("c = %v, want \"3.5\"", got[2])
	}
}

// `def Foo refine Integer` mints a fresh user subtype of Integer.
// `typeof Foo` walks one step up the lattice (Foo's parent), so it
// reports the base type Foo refines.
func TestRefineBareSubtypeTypeofParent(t *testing.T) {
	got := runOne(t, `def Foo refine Integer
typeof Foo is Integer`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("got %v, want [true]", got)
	}
}

// A typed-def against a bare-refine subtype reparents the value to
// the minted lattice node so `typeof x` reports the subtype, not the
// base. The value still satisfies `is` for the base type via the
// ancestry walk.
func TestRefineBareSubtypeReparent(t *testing.T) {
	got := runOne(t, `def Foo refine Integer
def x:Foo 1
typeof x
x is Foo
x is Integer
x is Number`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	if got[0] != "Foo" {
		t.Errorf("typeof x = %v, want \"Foo\"", got[0])
	}
	if got[1] != "true" {
		t.Errorf("x is Foo = %v, want true", got[1])
	}
	if got[2] != "true" {
		t.Errorf("x is Integer = %v, want true", got[2])
	}
	if got[3] != "true" {
		t.Errorf("x is Number = %v, want true", got[3])
	}
}

// `def Foo Integer` (no `refine`) is the alias path: Foo's body is
// the Integer type literal verbatim, so `42 is Foo` resolves Foo
// to Integer and succeeds. The two surfaces must stay distinct —
// alias for `def Foo Integer`, subtype for `def Foo refine Integer`.
func TestRefineBareDistinctFromAlias(t *testing.T) {
	got := runOne(t, `def Foo Integer
42 is Foo
def Bar refine Integer
42 is Bar`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "true" {
		t.Errorf("42 is Foo (alias) = %v, want true", got[0])
	}
	if got[1] != "false" {
		t.Errorf("42 is Bar (subtype, untagged 42) = %v, want false", got[1])
	}
}

// A user comparator installed on a shared user-minted parent (Item)
// dispatches for both its concrete inhabitants (Foo, Bar values) and
// its sibling subtype literals. Locks in the canonical-lattice-pointer
// pipeline through fn-sig type resolution → behave install → LCA walk:
// without canonical identity at every hop the LCA walk would miss the
// installed Behavior and fall through to numberCompareBehavior, which
// would order y (Bar=2) and x (Foo=1) by numeric magnitude (2 > 1)
// instead of by type-rank-then-name (Bar < Foo).
func TestUserComparatorOnRefineParent(t *testing.T) {
	got := runOne(t, `def Item refine Integer;
def Foo refine Item;
def Bar refine Item;
def x:Foo 1;
def y:Bar 2;
behave compare/q (fn [[a:Item b:Item] [Integer] [
  if (a is Type) [
    (a inspect/s 'struct' get) (b inspect/s 'struct' get) cmp
  ] [
    (a inspect/s 'type' get) (b inspect/s 'type' get) cmp
  ]
]]);
y lt x;
Bar lt Foo;
1 lt 2;
y cmp x`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	if got[0] != "true" {
		t.Errorf("y lt x = %v, want true (Bar value < Foo value via comparator)", got[0])
	}
	if got[1] != "true" {
		t.Errorf("Bar lt Foo = %v, want true (sibling type literals)", got[1])
	}
	if got[2] != "true" {
		t.Errorf("1 lt 2 = %v, want true (plain integer compare)", got[2])
	}
	if got[3] != int64(-1) {
		t.Errorf("y cmp x = %v, want -1", got[3])
	}
}
