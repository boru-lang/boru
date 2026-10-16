package check

// Gate test for the computed-segment arm of exprRefsCarrier
// (check_recovery.go:368 — `if ri.Segments[i].Computed {`).
//
// exprRefsCarrier is the const-fold veto: before a folded container expression
// is baked, it walks the expression's tokens looking for a word whose current
// def binding is a CARRIER (a computed value or loop iterator, abstract at
// check time). A Reach node (`m.a`, `m.(k)`) hides its tokens in two places —
// the receiver token sequence and each segment's key — so the walk recurses
// into both.
//
// The arm under test is the SEGMENT-KEY half. Entering it (not merely reaching
// the loop) requires ALL of:
//
//   - the walked value is a Reach — core.IsReach(v) and core.AsReach succeed,
//     so it must be built with core.NewReach(core.ReachInfo{...}), and
//   - the walk has not already found a carrier (found short-circuits the loop
//     body one line above), so the RECEIVER must be carrier-free, and
//   - the segment carries a COMPUTED key: ReachSeg.Computed == true. A literal
//     segment (`m.a`, Computed false, key in KeyLit) is skipped entirely — the
//     key is a name, not an expression, so it is never a carrier reference.
//
// Only a computed segment's KeyExpr is walked, which is what makes `m.(k)`
// with a def-local carrier `k` decline the fold instead of baking the
// carrier's check-time value (AsInteger of a carrier coerces to 0).

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// gate368Reach builds `recv.(key)`-shaped Reach: a receiverless-safe concrete
// receiver plus one segment carrying keyToks either as a computed key
// expression (computed=true) or as a literal key (computed=false).
func gate368Reach(recv []core.Value, computed bool, keyToks []core.Value) core.Value {
	seg := core.ReachSeg{Computed: computed}
	if computed {
		seg.KeyExpr = keyToks
	} else {
		seg.KeyLit = keyToks[0]
	}
	return core.NewReach(core.ReachInfo{
		Receiver: recv,
		Segments: []core.ReachSeg{seg},
		Eval:     true,
	})
}

// TestGaterecovery368ComputedKeyExprFindsCarrier drives the arm: a Reach whose
// receiver is carrier-free but whose COMPUTED segment key names a
// carrier-bound word. The walk must descend into KeyExpr and veto the fold.
func TestGaterecovery368ComputedKeyExprFindsCarrier(t *testing.T) {
	r := newTestRegistry(t)
	defer r.Check.Begin()()
	e := core.NewTop(r)

	// `m` is a concrete map binding (NOT a carrier) so the receiver walk runs
	// to completion without setting found — the loop must be entered with
	// found still false or the arm is short-circuited above.
	r.Defs.Push("m", mapOf("a", core.NewInteger(1)))
	// `k` is the computed key: a carrier, as a loop iterator or any computed
	// value is at check time.
	r.Defs.Push("k", core.NewCarrier(core.TString))

	if bound, ok := r.Defs.Top("m"); !ok || core.IsCarrier(&bound) {
		t.Fatalf("precondition: receiver binding must exist and be carrier-free, got %#v ok=%v", bound, ok)
	}
	if bound, ok := r.Defs.Top("k"); !ok || !core.IsCarrier(&bound) {
		t.Fatalf("precondition: key binding must be a carrier, got %#v ok=%v", bound, ok)
	}

	reach := gate368Reach([]core.Value{core.NewWord("m")}, true, []core.Value{core.NewWord("k")})
	if !exprRefsCarrier(e, []core.Value{reach}) {
		t.Error("m.(k) with a carrier-bound k must report a carrier reference (the fold must decline)")
	}
}

// TestGaterecovery368LiteralSegmentSkipsKey pins the guard's false edge: the
// SAME word in the SAME segment, but as a LITERAL key (Computed false), is not
// an expression and must not be walked — otherwise every `m.k` would veto its
// fold merely because some unrelated def named `k` currently holds a carrier.
func TestGaterecovery368LiteralSegmentSkipsKey(t *testing.T) {
	r := newTestRegistry(t)
	defer r.Check.Begin()()
	e := core.NewTop(r)

	r.Defs.Push("m", mapOf("k", core.NewInteger(1)))
	r.Defs.Push("k", core.NewCarrier(core.TString))

	reach := gate368Reach([]core.Value{core.NewWord("m")}, false, []core.Value{core.NewWord("k")})
	if exprRefsCarrier(e, []core.Value{reach}) {
		t.Error("m.k (literal segment) must not walk its key: the fold stays available")
	}
}

// TestGaterecovery368ComputedKeyExprConcreteBinding pins the other outcome of
// the arm: the computed key IS walked, but names a concrete binding, so no
// carrier is found. This separates "the arm ran" from "the arm found
// something" — both segments below are computed, and the carrier sits in the
// SECOND one, so the loop must keep iterating past a clean computed key.
func TestGaterecovery368ComputedKeyExprConcreteBinding(t *testing.T) {
	r := newTestRegistry(t)
	defer r.Check.Begin()()
	e := core.NewTop(r)

	r.Defs.Push("kc", core.NewString("a"))           // concrete: no veto
	r.Defs.Push("kk", core.NewCarrier(core.TString)) // carrier: veto

	onlyConcrete := core.NewReach(core.ReachInfo{
		Receiver: []core.Value{mapOf("a", core.NewInteger(1))},
		Segments: []core.ReachSeg{{Computed: true, KeyExpr: []core.Value{core.NewWord("kc")}}},
		Eval:     true,
	})
	if exprRefsCarrier(e, []core.Value{onlyConcrete}) {
		t.Error("a computed key naming a concrete binding must not veto the fold")
	}

	secondSegment := core.NewReach(core.ReachInfo{
		Receiver: []core.Value{mapOf("a", core.NewInteger(1))},
		Segments: []core.ReachSeg{
			{Computed: true, KeyExpr: []core.Value{core.NewWord("kc")}},
			{Computed: true, KeyExpr: []core.Value{core.NewWord("kk")}},
		},
		Eval: true,
	})
	if !exprRefsCarrier(e, []core.Value{secondSegment}) {
		t.Error("a carrier in a LATER computed segment must still veto the fold")
	}
}
