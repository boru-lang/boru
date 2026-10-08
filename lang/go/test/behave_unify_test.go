package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// behave unify/q installs a custom Unifier on a user-defined type.
// The kernel's dispatchUnifier walks the LCA chain looking for a
// Unifier capability and dispatches to it when found. This test pins
// the full flow:
//
//  1. Mint a refine subtype.
//  2. Install a Unifier on it via `behave unify/q`.
//  3. Verify the body fires when two values of that type are unified
//     via the boru `unify` word.

func TestBehaveUnify_BodyFires(t *testing.T) {
	got := runOne(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] [
  a b add
]])
def x:Item 3
def y:Item 4
x unify y`)
	// Expect [7, true] — the Unifier body returns a+b.
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(got), got)
	}
	if got[0] != int64(7) {
		t.Errorf("user unifier should return a+b=7, got %v", got[0])
	}
	if got[1] != "true" {
		t.Errorf("unify ok flag should be true, got %v", got[1])
	}
}

// A Unifier that returns None signals failure. The kernel converts
// this into the same shape any other unification failure produces.
func TestBehaveUnify_NoneSignalsFailure(t *testing.T) {
	got := runOne(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] [
  None
]])
def x:Item 3
def y:Item 4
x unify y`)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(got), got)
	}
	if got[1] != "false" {
		t.Errorf("Unifier returning None must fail unification, got %v", got)
	}
}

// Sibling refine subtypes share their parent (Item). The kernel's
// dispatchUnifier walk starts from the more specific side; when
// neither is a subtype of the other, it falls back to the LCA. A
// Unifier installed on the parent fires for sibling values.
func TestBehaveUnify_ParentUnifierFiresForSiblings(t *testing.T) {
	got := runOne(t, `def Item refine Integer
def Foo refine Item
def Bar refine Item
behave unify/q (fn [[a:Item b:Item] [Any] [
  99
]])
def x:Foo 3
def y:Bar 4
x unify y`)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(got), got)
	}
	if got[0] != int64(99) {
		t.Errorf("parent Unifier should fire for siblings, got %v", got[0])
	}
}

// validateUnifySig rejects fn shapes that don't match the protocol.
func TestBehaveUnify_RejectsBadShape(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "wrong arity",
			src:  `def Item refine Integer; behave unify/q (fn [[a:Item] [Item] [a]])`,
			want: "must take 2 args",
		},
		{
			name: "mismatched param types",
			src:  `def Item refine Integer; def Other refine Integer; behave unify/q (fn [[a:Item b:Other] [Item] [a]])`,
			want: "same type",
		},
		{
			name: "mismatched return type",
			src:  `def Item refine Integer; behave unify/q (fn [[a:Item b:Item] [Integer] [a]])`,
			want: "return type must match",
		},
		{
			name: "wrong return type",
			src:  `def Item refine Integer; behave unify/q (fn [[a:Item b:Item] [String] [a]])`,
			want: "return type must match",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			err = runReferenceErr(t, a, c.src)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q did not contain %q", err.Error(), c.want)
			}
		})
	}
}
