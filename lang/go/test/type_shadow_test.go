package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// --- type shadowing + undef ---
//
// *Type bindings now stack like `def` bindings. `def Foo X; def Foo
// Y` pushes Y on top so subsequent uses see Y; `undef Foo` pops Y
// and X becomes active again. Once the stack empties, the name is
// unbound. Mirrors `def` / `undef` semantics so users have a single
// scoping mental model across the value and type namespaces.

// Shadowing a type binding swaps the active definition without
// error. The most recent binding wins.
func TestTypeShadow_Push(t *testing.T) {
	got := runOne(t, `def Foo Integer
def Foo String
42 is Foo
"hi" is Foo`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "false" {
		t.Errorf("42 is Foo (now String) = %v, want false", got[0])
	}
	if got[1] != "true" {
		t.Errorf("\"hi\" is Foo (now String) = %v, want true", got[1])
	}
}

// `undef Foo` pops the most recent binding. The previous binding
// becomes active again.
func TestTypeShadow_Pop(t *testing.T) {
	got := runOne(t, `def Foo Integer
def Foo String
undef Foo
42 is Foo
"hi" is Foo`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "true" {
		t.Errorf("42 is Foo (popped to Integer) = %v, want true", got[0])
	}
	if got[1] != "false" {
		t.Errorf("\"hi\" is Foo (popped to Integer) = %v, want false", got[1])
	}
}

// Untype-ing the only binding makes the name unbound. Subsequent
// references error as undefined.
func TestTypeShadow_PopToEmpty(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def Foo Integer
undef Foo
42 is Foo`)
	if err == nil {
		t.Fatalf("expected error after untype-ing the last binding")
	}
	if !strings.Contains(err.Error(), "Foo") {
		t.Errorf("error %q does not mention Foo", err)
	}
}

// Untype-ing a name with no binding errors with a clear message.
func TestTypeShadow_UntypeUnbound(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`undef Nonexistent`)
	if err == nil {
		t.Fatalf("expected error untyping a nonexistent name")
	}
	if !strings.Contains(err.Error(), "no such type binding") {
		t.Errorf("error %q does not say 'no such type binding'", err)
	}
}

// `undef foo` (lowercase) is a value-namespace unbind under the
// universal `undef`: it does not touch a capitalised type binding and
// does not error when the lowercase name is unbound.
func TestTypeShadow_UndefLowercaseIsValueUnbind(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	got, err := a.Run(`def Foo Integer
undef foo
5 is Foo`)
	if err != nil {
		t.Fatalf("undef foo should be a harmless value-unbind: %v", err)
	}
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("type binding Foo should survive lowercase undef; got %v, want [true]", got)
	}
}

// Shadow with a richer type: predicate type over a concrete type
// literal. `is` should consult the active binding.
func TestTypeShadow_PredicateOverLiteral(t *testing.T) {
	got := runOne(t, `def Foo Integer
def Foo fnpred x:Any [if (x is String) [x] [None]]
42 is Foo
"hi" is Foo
undef Foo
42 is Foo
"hi" is Foo`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	want := []string{"false", "true", "true", "false"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Shadow with a DepScalar: the type-stack interacts cleanly with the
// dependent-scalar machinery.
func TestTypeShadow_DepScalar(t *testing.T) {
	got := runOne(t, `def Bound (Integer gt 10)
def Bound (Integer gt 100)
50 is Bound
200 is Bound
undef Bound
50 is Bound
200 is Bound`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	want := []string{"false", "true", "true", "true"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Multiple shadows + multiple untypes — verify deep-stack behaviour.
func TestTypeShadow_DeepStack(t *testing.T) {
	got := runOne(t, `def T Integer
def T String
def T Boolean
true is T
undef T
"hi" is T
undef T
42 is T`)
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 results", got)
	}
	want := []string{"true", "true", "true"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("level %d: %v, want %v", i, got[i], w)
		}
	}
}
