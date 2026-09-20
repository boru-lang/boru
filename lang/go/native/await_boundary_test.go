package native

import "testing"

// Unit tests for `await`'s branch-boundary check (native_temporal_await.go).
// The end-to-end compile failures live in lang/go/await_branch_isolation_test.go;
// these pin the two predicates directly, because the interesting arms are
// container kinds that are awkward to construct from boru source and easy
// to get silently wrong.
//
// Why a type-tag switch and not `send`'s payload-keyed sendableViolation:
// the two words ask different questions. `send` asks "can this be COPIED
// across?" and deep-copies what it admits, so it declines only what a copy
// cannot preserve. `await` copies nothing and must decline anything mutated
// IN PLACE. A FlexMap is the case that separates them — it is
// MapPayload{*OrderedMap} with a TFlexMap tag, payload-identical to an
// immutable Map, so a payload switch reads it as a plain map.

func TestSharedMutableKindClassifiesEveryContainer(t *testing.T) {
	om := NewOrderedMap()
	om.Set("k", NewInteger(1))

	mutable := []struct {
		name string
		v    Value
		want string
	}{
		{"FlexMap", NewFlexMap(om), "FlexMap"},
		{"FlexList", NewFlexList([]Value{NewInteger(1)}), "FlexList"},
		{"Store", NewStore(TStore), "Store"},
		// Table and a class instance are the two remaining in-place-mutable
		// kinds. Both are reachable as await-branch bindings, and both are
		// caught by a different arm of the classifier — Table by its type
		// tag, an Object by its ClassInstanceInfo payload.
		{"Table", NewValueRaw(TTable, TableData{}), "Table"},
		{"Object", NewValueRaw(TAny, ClassInstanceInfo{}), "Object"},
	}
	for _, c := range mutable {
		t.Run(c.name, func(t *testing.T) {
			if got := sharedMutableKind(c.v); got != c.want {
				t.Errorf("sharedMutableKind = %q, want %q — an in-place-mutable "+
					"container must never cross a branch boundary", got, c.want)
			}
		})
	}

	// NEGATIVE, and the half that matters most: over-declining breaks
	// working programs. An immutable Map/List returns a copy from `set`,
	// so it was never the hazard.
	immutable := []struct {
		name string
		v    Value
	}{
		{"Integer", NewInteger(1)},
		{"String", NewString("x")},
		{"plain Map", NewMap(om)},
		{"plain List", NewList([]Value{NewInteger(1), NewString("s")})},
		{"empty List", NewList(nil)},
		{"type literal", NewTypeLiteral(TInteger)},
		{"None", NewNone()},
	}
	for _, c := range immutable {
		t.Run("immutable/"+c.name, func(t *testing.T) {
			if got := sharedMutableKind(c.v); got != "" {
				t.Errorf("sharedMutableKind = %q, want \"\" — declining this would "+
					"break programs that share it legitimately", got)
			}
		})
	}

	// A Value with no Parent (a zero Value reaching the predicate through
	// an unbound or malformed binding) must not panic — ADR-005.
	if got := sharedMutableKind(Value{}); got != "" {
		t.Errorf("zero Value = %q, want \"\"", got)
	}
}

// TestSharedMutableKindFindsNestedContainers pins the recursion. A
// container is just as shared when it is reached through an immutable
// wrapper, so the walk descends plain List and Map payloads — and the
// compile failure then names the enclosing binding, which is the one the author
// has to change.
func TestSharedMutableKindFindsNestedContainers(t *testing.T) {
	inner := NewOrderedMap()
	flex := NewFlexMap(inner)

	t.Run("inside a plain List", func(t *testing.T) {
		v := NewList([]Value{NewInteger(1), flex})
		if got := sharedMutableKind(v); got != "FlexMap" {
			t.Errorf("got %q, want FlexMap", got)
		}
	})
	t.Run("inside a plain Map", func(t *testing.T) {
		outer := NewOrderedMap()
		outer.Set("nested", flex)
		if got := sharedMutableKind(NewMap(outer)); got != "FlexMap" {
			t.Errorf("got %q, want FlexMap", got)
		}
	})
	t.Run("two levels down", func(t *testing.T) {
		mid := NewOrderedMap()
		mid.Set("deep", NewList([]Value{NewFlexList([]Value{})}))
		if got := sharedMutableKind(NewMap(mid)); got != "FlexList" {
			t.Errorf("got %q, want FlexList", got)
		}
	})
	t.Run("all-immutable nesting stays clean", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", NewList([]Value{NewInteger(1), NewString("s")}))
		if got := sharedMutableKind(NewMap(m)); got != "" {
			t.Errorf("got %q, want \"\" — nothing mutable is in here", got)
		}
	})
}

// TestBranchTokensHandlesBothShapes pins the seam that made this check
// ship INERT on the default path the first time. A branch body arrives
// either as the raw token list (interpreted) or as a synthetic fn-value
// carrying the tokens in its BoruImpl body (compiled). Reading only the
// first leaves the boundary unguarded exactly where most programs run.
func TestBranchTokensHandlesBothShapes(t *testing.T) {
	toks := []Value{NewWord("m"), NewWord("set")}

	if got := branchTokens(NewList(toks)); len(got) != 2 {
		t.Errorf("list shape: got %d tokens, want 2", len(got))
	}

	fn := NewFunction(FnDefInfo{
		Name:       "b",
		Signatures: []FnSig{{Impl: &BoruImpl{Body: toks}}},
	})
	if got := branchTokens(fn); len(got) != 2 {
		t.Errorf("compiled fn-value shape: got %d tokens, want 2 — this is the "+
			"shape the default path uses", len(got))
	}

	// A fn-value with no boru body yields nothing rather than panicking,
	// and a non-list, non-fn element (a bare value branch) is simply not
	// a body to walk.
	empty := NewFunction(FnDefInfo{Name: "e"})
	if got := branchTokens(empty); got != nil {
		t.Errorf("fn with no boru sig: got %v, want nil", got)
	}
	if got := branchTokens(NewInteger(7)); got != nil {
		t.Errorf("non-body element: got %v, want nil", got)
	}
	if got := branchTokens(NewTypeLiteral(TList)); got != nil {
		t.Errorf("bare List type literal: got %v, want nil", got)
	}
}
