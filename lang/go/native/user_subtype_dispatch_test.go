package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// User-defined refinement subtypes must dispatch like system types — a
// fn parameter declared `[f:Foo]` (where Foo is `def Foo class
// {...}`) accepts ONLY Foo instances and REJECTS unrelated values, just
// as `[f:Integer]` does.
//
// **End-to-end dispatch tests live in lang/spec/user-types.tsv** —
// run via test/go/langspec. They exercise the actual parser, dispatcher,
// and rendering pipeline through boru source code (the right level for
// a *language* test).
//
// The Go tests below are unit-level: they probe internal Value-shape
// invariants and the `DefEntry.TypeDef` flag path that the TSV runner
// can't reach. Keep them tight — anything user-observable belongs in
// the TSV.

// Documents the architectural distinction the fix relies on:
//
//   - The codebase HAS a dedicated "this is a type" flag:
//     DefEntry.TypeDef *Type (see eng/go/deftable.go). It's set
//     non-nil exactly when a capitalised def installs a type
//     binding. r.LookupTypeName(name) consults it directly and
//     returns the lattice node.
//
//   - Before the fix, ResolveSigType only used that flag as a
//     narrow special-case for predicate-fn bodies, and reached
//     ResolveDefType for everything else. ResolveDefType then
//     INFERRED type-ness from body payload shape: `Data == nil`
//     (lattice-only body), IsRecordType, IsOptionsType — anything
//     else fell through to `return TAny`. ObjectType, Disjunct,
//     DepScalar and others silently became wildcards.
//
//   - After the fix, ResolveSigType consults DefEntry.TypeDef
//     first via r.LookupTypeName, and per-kind Behavior wrappers
//     (disjunctUnifier, depScalarUnifier, bareRefineUnifier) drive
//     the actual Match the dispatcher consults. Payload-shape
//     inspection is no longer the discriminator for "is this a
//     type"; the dedicated flag is.
//
// Confirms the shape data the fix relies on: ObjectType bodies carry
// their payload in Data, and the lattice node lives at body.Parent.
func TestUserTypeBindingShape_ObjectRefinement(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	registerIOWords(r)
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("Foo"),
		NewOpenParen(),
		NewWord("class"), NewMap(NewOrderedMap()),
		NewCloseParen(),
	})

	// 1. The dedicated flag: r.LookupTypeName returns the lattice
	//    node directly. This is the authoritative path.
	def := r.LookupTypeName("Foo")
	if def == nil {
		t.Fatal("LookupTypeName(Foo) returned nil — DefEntry.TypeDef flag not set")
	}
	if def.Leaf() != "Foo" {
		t.Errorf("LookupTypeName(Foo).Leaf() = %q, want %q", def.Leaf(), "Foo")
	}

	// 2. The body in r.TopTypeBody is a POPULATED body — its Data
	//    carries the ClassTypeInfo payload; the lattice node is at
	//    body.Parent.
	body, ok := r.TopTypeBody("Foo")
	if !ok {
		t.Fatal("Foo not installed as a type")
	}
	if !IsConcrete(&body) {
		t.Error("ObjectType body should have Data != nil")
	}
	if body.Parent == nil || body.Parent.Leaf() != "Foo" {
		t.Errorf("body.Parent should be Foo lattice node, got %v", body.Parent)
	}

	// 3. ResolveSigType (the dispatcher's entry point) consults the
	//    dedicated flag and returns the lattice node, NOT TAny.
	tp, _, err := core.ResolveSigType(r, NewWord("Foo"))
	if err != nil {
		t.Fatalf("ResolveSigType: %v", err)
	}
	if tp.Equal(TAny) {
		t.Error("ResolveSigType returned TAny for Foo — user subtype silently wildcard")
	}
	if tp.Leaf() != "Foo" {
		t.Errorf("ResolveSigType leaf = %q, want %q", tp.Leaf(), "Foo")
	}
}

// Catalogue what each refinement shape produces as a body and confirms
// ResolveSigType resolves each to the user-named lattice node. This is
// the structural invariant that anchors the dispatch-matrix tests in
// lang/spec/user-types.tsv — if ResolveSigType ever started returning
// TAny again for any of these shapes, the TSV happy-path rows would
// still pass (TAny accepts everything) but the TSV negative-path rows
// would fail. This Go test catches the regression at the resolution
// layer where it originates.
func TestResolveSigType_AllUserTypeKinds(t *testing.T) {
	cases := []struct {
		name    string
		defTail []Value
	}{
		{
			name: "class {}",
			defTail: []Value{
				NewWord("class"), NewMap(NewOrderedMap()),
			},
		},
		{
			name: "refine Integer (bare scalar)",
			defTail: []Value{
				NewWord("refine"), NewWord("Integer"),
			},
		},
		{
			name: "Disjunct (Integer tor none)",
			defTail: []Value{
				NewWord("Integer"), NewWord("tor"), NewWord("none"),
			},
		},
		{
			name: "DepScalar (Integer gt 10)",
			defTail: []Value{
				NewWord("Integer"), NewWord("gt"), NewInteger(10),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := DefaultRegistry()
			registerIOWords(r)
			input := append([]Value{
				NewWord("def"), NewWord("X"),
				NewOpenParen(),
			}, tc.defTail...)
			input = append(input, NewCloseParen())
			runBoru(t, r, input)

			// The dedicated flag must be set.
			def := r.LookupTypeName("X")
			if def == nil {
				t.Fatalf("LookupTypeName(X) = nil — DefEntry.TypeDef flag not set for %s", tc.name)
			}

			// And ResolveSigType must use it — not fall through to TAny.
			tp, _, err := core.ResolveSigType(r, NewWord("X"))
			if err != nil {
				t.Fatalf("ResolveSigType: %v", err)
			}
			if tp == nil {
				t.Fatal("ResolveSigType returned nil")
			}
			if tp.Equal(TAny) {
				t.Errorf("ResolveSigType returned TAny for %s — user subtype silently wildcard", tc.name)
			}
			if tp.Leaf() != "X" {
				t.Errorf("ResolveSigType leaf = %q, want %q", tp.Leaf(), "X")
			}
		})
	}
}

// TestRefineNewtypeMembershipIsNominal pins the bare-refine (newtype)
// membership rule on the dispatch-side lattice node: `def Pos refine
// Integer` mints a nominal subtype, so an untagged base value is NOT a
// member — `42 Is Pos` is false through both the minted node
// (LookupTypeName, what dispatch sees) and the def-table body. See
// design/REFINE-NEWTYPE-VS-SUBSET.10.md.
func TestRefineNewtypeMembershipIsNominal(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("Pos"),
		NewWord("refine"), NewWord("Integer"),
	})

	body, ok := r.TopTypeBody("Pos")
	if !ok {
		t.Fatal("Pos not installed")
	}
	if !IsBareTypeNode(&body) {
		t.Errorf("bare-refine body should be a bare type node, got Data=%T", body.Data)
	}
	def := r.LookupTypeName("Pos")
	if def == nil {
		t.Fatal("LookupTypeName(Pos) = nil")
	}
	if addrOf(NewInteger(42)).Is(def) {
		t.Error("42.Is(Pos lattice node) = true — newtype membership must be nominal, a plain Integer is not a Pos")
	}
	bNode := body
	if addrOf(NewInteger(42)).Is(&bNode) {
		t.Error("42.Is(&body) = true — the def-table body must not admit untagged base values either")
	}
}

// TestClassInstanceTaggedWithCanonicalLatticeNode pins the identity
// chain for class instances: `make Foo {}` tags the instance with the
// SAME minted lattice node that LookupTypeName returns, so membership
// (`Is`, ConformsTo) holds through that node. The def-table body is a
// different value (the class schema payload, its own ID) and is NOT a
// dispatchable stand-in — which is exactly why dispatch must resolve
// names via LookupTypeName / NodeOf, never via `&body`. See
// eng/go/CLAUDE.md "Canonical *Type Pointers".
func TestClassInstanceTaggedWithCanonicalLatticeNode(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("Foo"),
		NewOpenParen(),
		NewWord("class"), NewMap(NewOrderedMap()),
		NewCloseParen(),
	})

	result := runBoru(t, r, []Value{
		NewOpenParen(),
		NewWord("make"), NewWord("Foo"), NewMap(NewOrderedMap()),
		NewCloseParen(),
	})
	if len(result) != 1 {
		t.Fatalf("make Foo: expected 1 result, got %d", len(result))
	}
	inst := result[0]

	def := r.LookupTypeName("Foo")
	if def == nil {
		t.Fatal("LookupTypeName(Foo) = nil")
	}
	if IDOf(inst.Parent) != IDOf(def) {
		t.Errorf("instance tag %q is not the minted lattice node %q", IDOf(inst.Parent), IDOf(def))
	}
	if !inst.Is(def) {
		t.Error("instance.Is(Foo lattice node) = false — class membership broken")
	}
	if !inst.Parent.ConformsTo(def) {
		t.Error("instance.Parent.ConformsTo(Foo lattice node) = false")
	}

	// The def-table body (the schema payload) is a distinct value with
	// its own ID — a non-canonical stand-in that membership checks must
	// not be routed through.
	body, _ := r.TopTypeBody("Foo")
	if IDOf(inst.Parent) == IDOf(&body) {
		t.Errorf("def-table body unexpectedly IS the lattice node (%q) — canonical-pointer docs need updating", IDOf(&body))
	}
}

// TestDisjunctMembershipRequiresCanonicalNode pins the disjunct
// (`def Maybe (Integer tor none)`) membership rule and the canonical-
// pointer asymmetry that motivates it: the minted node from
// LookupTypeName carries the disjunct unifier, so an Integer is
// admitted (and a String rejected) through it — while `&body` from the
// def table is a raw DisjunctInfo value with no unifier Behavior and
// admits nothing. Membership must go through the canonical node.
func TestDisjunctMembershipRequiresCanonicalNode(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("Maybe"),
		NewOpenParen(),
		NewWord("Integer"), NewWord("tor"), NewWord("none"),
		NewCloseParen(),
	})

	def := r.LookupTypeName("Maybe")
	if def == nil {
		t.Fatal("LookupTypeName(Maybe) = nil")
	}
	if !addrOf(NewInteger(42)).Is(def) {
		t.Error("42.Is(Maybe) = false — the Integer alternative must be admitted")
	}
	if addrOf(NewString("x")).Is(def) {
		t.Error(`"x".Is(Maybe) = true — String is not an alternative and must be rejected`)
	}

	// The orphan `&body` pointer carries no unifier: it must not admit
	// members. If this changes, dispatch through bodies has been made
	// canonical — update eng/go/CLAUDE.md "Canonical *Type Pointers".
	body, _ := r.TopTypeBody("Maybe")
	bNode := body
	if addrOf(NewInteger(42)).Is(&bNode) {
		t.Error("42.Is(&body) = true — non-canonical body pointer unexpectedly dispatches membership")
	}
}
