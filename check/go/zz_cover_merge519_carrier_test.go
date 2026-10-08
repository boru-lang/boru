package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestMerge519JoinedElementCarrierTypeLiteral pins elementJoinCarrier's type
// arm (NUR323): a type literal among a concrete list's mixed elements joins as
// a Type carrier (core.ValueCarrier), never as a carrier of its Parent — the
// Integer node's Parent is Number, and a Number element would commit a body's
// arithmetic over what the run holds as a type. The negative twin is the same
// list with a plain Number-family value in the type's place, which joins to
// the numeric lattice with no Type alternative.
func TestMerge519JoinedElementCarrierTypeLiteral(t *testing.T) {
	typeLit := core.NewTypeLiteral(core.TInteger)
	if !core.IsTypeLiteral(typeLit) || typeLit.Parent.Equal(core.TInteger) {
		t.Fatalf("precondition: an Integer type literal is parented off Integer, got %v", typeLit.Parent)
	}

	if got := elementJoinCarrier(typeLit); !got.Parent.Equal(core.TType) || got.Parent.Equal(typeLit.Parent) {
		t.Errorf("a type literal element joins as its Type carrier, got %v", got)
	}

	mixed := core.NewList([]core.Value{core.NewInteger(1), typeLit})
	joined, ok := joinedElementCarrier(mixed)
	if !ok {
		t.Fatal("[1 Integer] is a mixed concrete list: the join must run")
	}
	if !joinMentions(joined, core.TType) {
		t.Errorf("[1 Integer] must join a Type alternative, got %v", joined)
	}

	// Negative twin: a Float in the type's place joins numerics only.
	plain := core.NewList([]core.Value{core.NewInteger(1), core.NewFloat(2.5)})
	joined, ok = joinedElementCarrier(plain)
	if !ok {
		t.Fatal("[1 2.5] is a mixed concrete list: the join must run")
	}
	if joinMentions(joined, core.TType) {
		t.Errorf("[1 2.5] must not join a Type alternative, got %v", joined)
	}
}

// joinMentions reports whether a joined carrier stands for want, or has an
// alternative that does (core.TypeNodeOf reads a carrier's Parent and a
// type literal's own node alike).
func joinMentions(v core.Value, want *core.Type) bool {
	if n := core.TypeNodeOf(v); n != nil && n.Equal(want) {
		return true
	}
	if di, ok := v.Data.(core.DisjunctInfo); ok {
		for _, alt := range di.Alternatives {
			if joinMentions(alt, want) {
				return true
			}
		}
	}
	return false
}
