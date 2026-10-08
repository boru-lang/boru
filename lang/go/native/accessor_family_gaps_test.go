package native

import (
	"strings"
	"testing"
)

// Direct-handler tests for the NUR021 accessor-family fill: the strict
// Store/Xml readers and the Xml/Module presence rows. The happy paths
// are pinned language-level in lang/spec/accessor.tsv, xml.tsv and
// module-instance.tsv; these cover the defensive arms a spec row cannot
// reach (a type literal never dispatches into the handler — the sig
// match declines first — so the wrong-payload arms need direct calls,
// the same pattern as the array-word handler tests).

func gapsTestReg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return r
}

func TestGetrStoreHandlerArms(t *testing.T) {
	r := gapsTestReg(t)
	// Wrong payload (a plain Integer in the Store slot) errors.
	if _, err := getrStoreHandler([]Value{NewString("k"), NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "getr: expected a Store") {
		t.Errorf("non-store payload: got %v, want 'getr: expected a Store'", err)
	}
	// A real store: miss raises not_found, hit reads the value.
	sv := NewStore(TStore)
	store, err := AsStore(sv)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := getrStoreHandler([]Value{NewString("nope"), sv}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not_found") {
		t.Errorf("miss: got %v, want not_found", err)
	}
	store.Set("k", NewInteger(42))
	out, err := getrStoreHandler([]Value{NewString("k"), sv}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("hit: %v %v", out, err)
	}
	if n, _ := AsInteger(out[0]); n != 42 {
		t.Errorf("hit = %v, want 42", out[0])
	}
}

func TestGetrXmlHandlerArms(t *testing.T) {
	r := gapsTestReg(t)
	// Wrong payload (non-Xml) errors with the getr_error arm.
	if _, err := getrXmlHandler([]Value{NewString("tag"), NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "non-Xml") {
		t.Errorf("non-xml payload: got %v, want the non-Xml error", err)
	}
}

func TestHasXmlHandlerNonXmlIsFalse(t *testing.T) {
	r := gapsTestReg(t)
	out, err := hasXmlHandler([]Value{NewString("tag"), NewInteger(1)}, nil, nil, r)
	if err != nil {
		t.Fatalf("has is total, got error: %v", err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Error("non-Xml payload must answer false")
	}
}

func TestHasModuleHandlersNonConcreteIsFalse(t *testing.T) {
	r := gapsTestReg(t)
	mlit := NewTypeLiteral(TModuleInst)
	out, err := hasModuleInstHandler([]Value{NewString("kind"), mlit}, nil, nil, r)
	if err != nil {
		t.Fatalf("has is total, got error: %v", err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Error("a Module type literal binds nothing — want false")
	}
}
