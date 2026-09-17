package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// --- Naming rule: capitalisation selects type vs value binding ---
//
// `def` is the universal binder (design/legacy/TYPE-UNIFORM.10.ignore).
// The *name's capitalisation* selects what is bound: a capitalised
// name is a TYPE binding, a lowercase name is a VALUE binding.

// def accepts a capitalised name as a type binding.

func TestNameCase_TypeUpperOK(t *testing.T) {
	got := runOne(t, `def Mid Integer
def n:Mid 5
n`)
	if len(got) != 1 || got[0] != int64(5) {
		t.Errorf("got %v, want [5]", got)
	}
}

// def accepts non-capitalised names.

func TestNameCase_DefLowerOK(t *testing.T) {
	got := runOne(t, `def x 1
x`)
	if len(got) != 1 || got[0] != int64(1) {
		t.Errorf("got %v, want [1]", got)
	}
}

func TestNameCase_DefHyphenOK(t *testing.T) {
	got := runOne(t, `def my-thing 42
my-thing`)
	if len(got) != 1 || got[0] != int64(42) {
		t.Errorf("got %v, want [42]", got)
	}
}

// def with a capitalised name is a TYPE binding — equivalent to
// `type`. `def Mid Integer` installs Mid as a type, usable in a
// type position exactly like `def Mid Integer` would.

func TestNameCase_DefUpperIsTypeBinding(t *testing.T) {
	got := runOne(t, `def Mid Integer
def n:Mid 5
n`)
	if len(got) != 1 || got[0] != int64(5) {
		t.Errorf("got %v, want [5]", got)
	}
}

// A capitalised def binding an object type absorbs the lattice-minting
// that `type` does: typeof reports the bound name.
func TestNameCase_DefUpperObjectMints(t *testing.T) {
	got := runOne(t, `def Acct (class {bal:Number})
make Acct {bal:1} typeof`)
	if len(got) != 1 || got[0] != "Acct" {
		t.Errorf("got %v, want [Acct]", got)
	}
}

// undef is the universal unbinder — the symmetric completion of the
// universal `def`. `undef Foo` (capitalised) pops the type binding,
// exactly as the legacy `untype` does; a lowercase `undef` pops a
// value binding.

func TestNameCase_UndefUpperPopsTypeShadow(t *testing.T) {
	got := runOne(t, `def Foo Integer
def Foo String
undef Foo
5 is Foo`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("undef should pop String, revealing Integer; got %v, want [true]", got)
	}
}

func TestNameCase_UndefUpperEmptiesType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	if _, err := a.Run("def Foo Integer\nundef Foo\nFoo"); err == nil {
		t.Fatal("expected error — Foo undefined after undef, got nil")
	}
}

// Typed-def shares the same rule.

func TestNameCase_TypedDefUpperRejected(t *testing.T) {
	expectError(t, `def X:Integer 1`, "must not start with a capital letter")
}

func TestNameCase_TypedDefLowerOK(t *testing.T) {
	got := runOne(t, `def x:Integer 1
x`)
	if len(got) != 1 || got[0] != int64(1) {
		t.Errorf("got %v, want [1]", got)
	}
}
