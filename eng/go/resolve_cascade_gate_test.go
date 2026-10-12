package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Gate tests for the canonical type-name cascade's less-travelled arms
// (ADR-012 rule 4) — each pins an arm the spec suites reach only
// through composed behaviour, paired with its negative.

// ResolveFieldType's builtin arm: a bare `{a:Integer}` field arrives
// as a Word post-opacity and resolves to the canonical literal; an
// unknown name passes through untouched (the caller's concern).
func TestResolveFieldTypeBuiltinArm(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	got := core.ResolveFieldType(r, core.NewWord("Integer"))
	if !core.IsBareTypeNode(&got) || got.ID != core.TInteger.ID {
		t.Fatalf("ResolveFieldType(word(Integer)) = %s, want the Integer literal", got)
	}
	raw := core.NewWord("NoSuchTypeName")
	if got := core.ResolveFieldType(r, raw); !core.IsWord(&got) {
		t.Fatalf("unknown name must pass through, got %s", got)
	}
}

// ResolveTypeLiteralDef's legacy bare-name arm: a Class-shaped value
// installed under a type's user-facing name (the RegisterResource-era
// shape) is picked up from the def store; an unbound name returns the
// literal unchanged.
func TestResolveTypeLiteralDefBareNameClassArm(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	fields := core.NewOrderedMap()
	fields.Set("kind", core.NewTypeLiteral(core.TString))
	cls := core.NewClassType(core.TInteger, core.ClassTypeInfo{Name: "Integer", Fields: fields})
	core.InstallDef(r, "Integer", cls)
	got := core.ResolveTypeLiteralDef(core.NewTypeLiteral(core.TInteger), r)
	if !core.IsClassType(&got) {
		t.Fatalf("bare-name class binding must win, got %s", got)
	}
	// Negative: no binding → the literal passes through.
	if got := core.ResolveTypeLiteralDef(core.NewTypeLiteral(core.TFloat), r); !core.IsBareTypeNode(&got) || got.ID != core.TFloat.ID {
		t.Fatalf("unbound literal must pass through, got %s", got)
	}
}

// ResolveSigChildParam's user-type body arm and the nested-unchanged
// short-circuit.
func TestResolveSigChildParamArms(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if _, err := r.DefineType("Foo", core.NewTypeLiteral(core.TInteger)); err != nil {
		t.Fatalf("DefineType: %v", err)
	}
	// User-type child resolves to the binding's body at sig install.
	got := core.ResolveSigChildParam(r, core.NewTypedList(core.NewWord("Foo")))
	ci, cerr := core.AsChildType(got)
	if cerr != nil || core.IsWord(&ci.Child) {
		t.Fatalf("user-type child must resolve, got %s", got)
	}
	// Nested container whose inner child resolves to nothing: the
	// whole pattern is returned unchanged.
	raw := core.NewTypedList(core.NewTypedList(core.NewWord("NoSuchTypeName")))
	if got := core.ResolveSigChildParam(r, raw); !core.ExactEqual(got, raw) {
		t.Fatalf("unresolvable nested child must pass through, got %s", got)
	}
}
