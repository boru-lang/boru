package native

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Stage 0 of design/legacy/TYPE-REPRESENTATION.1.ignore — the catch-all InstallType
// branch fixes (NUR093) and the typed-def constraint guard, exercised
// through the full word surface.

// `undef` retires only a node the popped binding MINTED. An alias
// binding adopts an existing canonical node, so popping it must leave
// the adopted identity in the lattice.
func TestUndefAliasKeepsAdoptedNode(t *testing.T) {
	r := w8Reg(t)
	if _, err := w8RunOn(t, r, "def NsA refine Integer"); err != nil {
		t.Fatalf("refine: %v", err)
	}
	node := r.LookupTypeName("NsA")
	if node == nil {
		t.Fatal("NsA must resolve to its minted node")
	}
	if _, err := w8RunOn(t, r, "def NsB NsA  undef NsB"); err != nil {
		t.Fatalf("alias+undef: %v", err)
	}
	if r.Types.LookupByID(node.ID) == nil {
		t.Fatal("undef of the alias must not retire NsA's minted node")
	}
	// The minted binding itself still retires on its own undef.
	if _, err := w8RunOn(t, r, "undef NsA"); err != nil {
		t.Fatalf("undef NsA: %v", err)
	}
	if r.Types.LookupByID(node.ID) != nil {
		t.Fatal("undef of the minting binding must retire the node")
	}
}

// The typed-def refine-reparent arm keeps its hands off nodes whose kind
// enforces membership through a constraint Unify (core.HasConstraintUnify):
// with a node-valued constraint (the Stage 2 evaluation state, simulated
// here by rebinding the name to its node literal), a dependent scalar
// still runs its bounds check instead of being nominally reparented.
func TestTypedDefNodeConstraintRunsDepScalar(t *testing.T) {
	r := w8Reg(t)
	if _, err := w8RunOn(t, r, "def NsBig (Integer gt 10)"); err != nil {
		t.Fatalf("depscalar: %v", err)
	}
	node := r.LookupTypeName("NsBig")
	if node == nil || !core.HasConstraintUnify(node) {
		t.Fatal("NsBig must carry a constraint Unify")
	}
	// Simulate the flip: the name evaluates to the minted node.
	r.Defs.PushType("NsBig", node, core.NewTypeLiteral(node))
	if _, err := w8RunOn(t, r, "def nsx:NsBig 5"); err == nil ||
		!strings.Contains(err.Error(), "does not unify") {
		t.Fatalf("a non-member must be declined through the constraint, got %v", err)
	}
	out, err := w8RunOn(t, r, "def nsy:NsBig 50  nsy")
	if err != nil {
		t.Fatalf("member bind: %v", err)
	}
	if len(out) != 1 || out[0].String() != "50" {
		t.Fatalf("the member must bind the VALUE (not the node literal), got %v", out)
	}
}

// A singleton node constraint likewise binds the value, not the node
// literal (the unifySameOrSubtype swap hazard BindingBodyUnifier.Unify
// closes).
func TestTypedDefNodeConstraintSingleton(t *testing.T) {
	r := w8Reg(t)
	if _, err := w8RunOn(t, r, "def NsOne 1"); err != nil {
		t.Fatalf("singleton: %v", err)
	}
	node := r.LookupTypeName("NsOne")
	r.Defs.PushType("NsOne", node, core.NewTypeLiteral(node))
	out, err := w8RunOn(t, r, "def nsz:NsOne 1  nsz")
	if err != nil {
		t.Fatalf("member bind: %v", err)
	}
	if len(out) != 1 || out[0].String() != "1" {
		t.Fatalf("the singleton member must bind the value, got %v", out)
	}
	if _, err := w8RunOn(t, r, "def nsw:NsOne 2"); err == nil {
		t.Fatal("a non-member of the singleton must decline")
	}
}

// The NUR093 surface, end to end: alias and singleton parameters
// dispatch, wrong values still decline, `is` unchanged.
func TestAliasSingletonDispatch(t *testing.T) {
	out, err := w8Run(t, "def Foo Integer  def f fn [[x:Foo] [Integer] [7]]  f 42")
	if err != nil || len(out) != 1 || out[0].String() != "7" {
		t.Fatalf("alias param must dispatch, got %v / %v", out, err)
	}
	out, err = w8Run(t, "def One 1  def g fn [[x:One] [Integer] [9]]  g 1")
	if err != nil || len(out) != 1 || out[0].String() != "9" {
		t.Fatalf("singleton param must dispatch, got %v / %v", out, err)
	}
	if _, err = w8Run(t, "def One 1  def g fn [[x:One] [Integer] [9]]  g 2"); err == nil ||
		!strings.Contains(err.Error(), "no signature matches") {
		t.Fatalf("a non-member must still decline, got %v", err)
	}
	out, err = w8Run(t, "def Foo Integer  42 is Foo")
	if err != nil || len(out) != 1 || out[0].String() != "true" {
		t.Fatalf("`is` must stay transparent for the alias, got %v / %v", out, err)
	}
}
