package native

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The three kernel type-kinds are registered as Ideals on every
// DefaultRegistry, and `type`'s dispatch resolves through them.
func TestIdeals_KernelKindsRegistered(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	for _, name := range []string{"Object", "Record", "Table"} {
		if r.Ideals.Get(name) == nil {
			t.Errorf("kernel Ideal %q is not registered", name)
		}
	}
	if id := r.Ideals.For(NewTypeLiteral(TRecord)); id == nil || id.Name != "Record" {
		t.Errorf("For(Record literal) = %v, want Record", id)
	}
}

// A host-registered Ideal is dispatched by the real `type` word — the
// dynamic extensibility the Ideal registry exists to provide.
func TestIdeals_CustomKindDispatchesThroughType(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	called := false
	r.Ideals.Register(&core.Ideal{
		Name:    "Stringy",
		Enabled: true,
		Accepts: func(base Value) bool {
			return IsBareTypeNode(&base) && (&base).Equal(TString)
		},
		Construct: func(base, arg Value, r *Registry) ([]Value, error) {
			called = true
			return []Value{NewTypeLiteral(TString)}, nil
		},
	})
	// `refine String {}` — no kernel kind claims a String base, so the
	// custom Ideal handles it.
	runBoru(t, r, []Value{
		NewWord("refine"), NewTypeLiteral(TString), NewMap(NewOrderedMap()),
	})
	if !called {
		t.Error("custom Ideal's Construct was not invoked by `type`")
	}
}

// A host-registered Ideal's Instantiate is dispatched by the real
// `make` word.
func TestIdeals_CustomKindInstantiatesThroughMake(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	called := false
	r.Ideals.Register(&core.Ideal{
		Name:    "Listy",
		Enabled: true,
		Accepts: func(v Value) bool {
			return IsBareTypeNode(&v) && (&v).Equal(TList)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			called = true
			return []Value{NewInteger(0)}, nil
		},
	})
	// `make List 7` — no kernel kind claims a bare List type literal,
	// so the custom Ideal's Instantiate handles it.
	runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TList), NewInteger(7),
	})
	if !called {
		t.Error("custom Ideal's Instantiate was not invoked by `make`")
	}
}

// Every registered Ideal satisfies the descriptor contract: a name
// that round-trips through the registry and an Accepts predicate. The
// three kernel kinds additionally carry both halves of the type
// pipeline — Construct (for `type`) and Instantiate (for `make`).
func TestIdealConformance(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	kernelKind := map[string]bool{"Object": true, "Record": true, "Table": true}
	names := r.Ideals.Names()
	if len(names) == 0 {
		t.Fatal("DefaultRegistry registered no Ideals")
	}
	for _, name := range names {
		id := r.Ideals.Get(name)
		if id == nil {
			t.Errorf("Names() reported %q but Get returned nil", name)
			continue
		}
		if id.Name != name {
			t.Errorf("Ideal keyed %q reports Name %q", name, id.Name)
		}
		if id.Accepts == nil {
			t.Errorf("Ideal %q has no Accepts predicate", name)
		}
		if kernelKind[name] {
			if id.Construct == nil {
				t.Errorf("kernel Ideal %q has no Construct", name)
			}
			if id.Instantiate == nil {
				t.Errorf("kernel Ideal %q has no Instantiate", name)
			}
		}
	}
	for _, name := range []string{"Object", "Record", "Table"} {
		if r.Ideals.Get(name) == nil {
			t.Errorf("kernel Ideal %q is missing from DefaultRegistry", name)
		}
	}
}

// Disabling an Ideal removes it from dispatch (For) but not from the
// registry (Match still reports it, so a caller can tell a disabled
// kind apart from an unknown base).
func TestIdealConformance_DisabledKind(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	rec := r.Ideals.Get("Record")
	if rec == nil {
		t.Fatal("Record Ideal not registered")
	}
	rec.Enabled = false
	base := NewTypeLiteral(TRecord)
	if id := r.Ideals.For(base); id != nil {
		t.Errorf("For(Record) with Record disabled = %v, want nil", id)
	}
	if id := r.Ideals.Match(base); id == nil || id.Name != "Record" {
		t.Errorf("Match(Record) with Record disabled = %v, want Record", id)
	}
}

// `type` against a disabled kind fails with a clear "not available"
// error rather than silently falling through to the unknown-base
// message.
func TestIdeals_DisabledKindErrorsFromType(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	rec := r.Ideals.Get("Record")
	if rec == nil {
		t.Fatal("Record Ideal not registered")
	}
	rec.Enabled = false
	err = runBoruError(t, r, []Value{
		NewWord("refine"), NewTypeLiteral(TRecord), NewMap(NewOrderedMap()),
	})
	if err == nil {
		t.Fatal("refine Record with Record disabled: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error = %q, want it to mention the kind is not available", err.Error())
	}
}
