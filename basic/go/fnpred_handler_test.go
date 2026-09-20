package basic

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// FnpredHandler's two defensive arms, neither reachable from the language
// surface: a bare `List` type literal never matches the TList slot (dispatch
// declines first with signature_error), and the spec-list rows in
// lang/spec/fnpred.tsv cover the malformed-shape arm from source. Both are
// live paths for a HOST calling the handler directly — the same reason
// FnsigHandler carries the identical concrete-list guard — so they are
// tested here rather than pragma'd away.

func TestFnpredHandlerNonConcreteList(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// A bare List TYPE LITERAL: Parent is List, Data is nil.
	_, err = FnpredHandler([]core.Value{core.NewTypeLiteral(core.TList)}, nil, nil, r)
	if err == nil {
		t.Fatal("a non-concrete list must be declined")
	}
	if !strings.Contains(err.Error(), "concrete list") {
		t.Errorf("want a concrete-list compile failure, got %v", err)
	}
}

func TestFnpredHandlerConstructError(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// A well-shaped PAIR whose input names a type that does not exist:
	// the spec list is fine, so the failure comes out of FnConstruct.
	spec := core.NewList([]core.Value{
		core.NewList([]core.Value{core.NewWord("n:NoSuchTypeXyz")}),
		core.NewList([]core.Value{core.NewInteger(1)}),
	})
	if _, err := FnpredHandler([]core.Value{spec}, nil, nil, r); err == nil {
		t.Fatal("an unknown param type must surface FnConstruct's error")
	}
}
