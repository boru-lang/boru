package basic

import (
	"strings"
	"testing"
)

// A BARE NODE head with no schema behind it (`P of [Integer]` at run
// time, past the check pass): the head resolves through the bare-node
// arm — CanonicalType + SchemaInfoOf — and declines as "not a generic
// schema". The check pass flags the same program statically, so the
// runtime arm is driven directly here.
func TestOfHandlerBareNodeHeadNotASchema(t *testing.T) {
	r := newTestRegistry(t)
	prefab := r.Types.MintRefinePrefab(TInteger)
	if err := InstallType(r, "GnP", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("install: %v", err)
	}
	node := r.LookupTypeName("GnP")
	if node == nil {
		t.Fatal("GnP not minted")
	}
	args := []Value{NewList([]Value{NewTypeLiteral(TInteger)}), NewTypeLiteral(node)}
	if _, err := OfHandler(args, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not a generic schema") {
		t.Fatalf("a plain named type's node must decline instantiation, got %v", err)
	}
}
