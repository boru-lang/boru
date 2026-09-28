package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// zz_cover_merge519_emit_test.go drives recordGuardedTrap's declines
// directly (the merged ADR-008 gate on the #519 merge). The end-to-end rows
// are lang/go/zz_cover_merge519_emit_test.go; these pin the arms a program
// reaches only through shapes that decline elsewhere first.

// guardedTrapState is a recording state whose check pass has published an
// optimistic outer match (core CheckState.OptimisticOuter, NUR264).
func guardedTrapState(t *testing.T, outer *core.OuterMatch) *EmitState {
	t.Helper()
	es, r := memoState(t)
	r.Check.OptimisticOuter = outer
	return es
}

// A trap met under an optimistic match is the outer word's rematch, carrying
// the trap as OnMatch; the rematch declines — and so does the trap, leaving
// the caller's failure to stand — for a speculative fn family's word, a
// render tuple that does not index the window, and a window value with no
// compiled home.
func TestRecordGuardedTrapDeclines(t *testing.T) {
	win := func(vals ...core.Value) *core.OuterMatch {
		written := make([]int, len(vals))
		for i := range written {
			written[i] = i
		}
		return &core.OuterMatch{Word: "each", Vals: vals, Written: written, Pos: core.SrcPos{Row: 1, Col: 1}}
	}

	// The positive control: a concrete window resolves, the trap is the
	// word's rematch with the trap's own error on a match.
	es := guardedTrapState(t, win(core.NewInteger(5)))
	if !es.RecordTrap("illegal_ref", "d", "x", "", core.SrcPos{Row: 1, Col: 9}) || es.trapAt == 0 {
		t.Fatal("a resolvable window records the guarded trap")
	}
	var ev *EmitEvent
	for i := range es.frames[0] {
		if es.frames[0][i].seq == es.trapAt {
			ev = &es.frames[0][i]
		}
	}
	if ev == nil || ev.trap.rematchWord != "each" || ev.trap.rematchOnMatch == nil || ev.trap.rematchOnMatch.Code != "illegal_ref" {
		t.Fatalf("the trap is each's rematch carrying illegal_ref: %+v", ev)
	}

	for _, c := range []struct {
		name  string
		outer *core.OuterMatch
		spec  bool
	}{
		{"a speculative fn family's word", win(core.NewInteger(5)), true},
		{"a render tuple that indexes nothing", &core.OuterMatch{Word: "each", Vals: []core.Value{core.NewInteger(5)}}, false},
		{"a window value with no compiled home", win(core.NewCarrier(core.TList)), false},
	} {
		es := guardedTrapState(t, c.outer)
		if c.spec {
			es.specFnNames = map[string]bool{"each": true}
		}
		if es.RecordTrap("illegal_ref", "d", "x", "", core.SrcPos{}) {
			t.Errorf("%s: the guarded trap declines", c.name)
		}
		if es.RecordTrapErr(&core.BoruError{Code: "signature_error"}, core.SrcPos{}) {
			t.Errorf("%s: RecordTrapErr's guarded trap declines too", c.name)
		}
		if es.trapAt != 0 {
			t.Errorf("%s: a declined trap records nothing", c.name)
		}
	}
}

// bindCarrier widens a type VALUE to a Type carrier (NUR323), never to a
// carrier of its Parent: the Integer node's Parent is Number.
func TestBindCarrierTypeValue(t *testing.T) {
	c := bindCarrier(core.NewTypeLiteral(core.TInteger))
	if !c.Carrier || c.Parent != core.TType {
		t.Errorf("a type value's carrier is a Type carrier, got %v (parent %v)", c, c.Parent)
	}
}
