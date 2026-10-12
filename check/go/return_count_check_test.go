package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Direct kernel pins for checkBodyReturnConformance's COUNT mirror (the
// static twin of the __RC arity rule): a residual provably longer than the
// declaration + unnamed-arg allowance flags the runtime's byte-identical
// "expected N return value(s), got M" (returnCountErrorText), while every
// count the analysis cannot know exactly — a variadic spread, a possibly-
// unapplied fn value — stays with the runtime  The corpus twins are
// the recursion.tsv:59 / user-types.tsv:287 ERROR rows.

func countCheckRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Check.Begin())
	return r
}

func countDiags(r *core.Registry, detail string) int {
	n := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "type_error" && d.Detail == detail {
			n++
		}
	}
	return n
}

func TestReturnCountConformanceFlagsExcess(t *testing.T) {
	r := countCheckRegistry(t)
	stk := []core.Value{core.NewInteger(1), core.NewInteger(2)}
	checkBodyReturnConformance(r, "r2", []*core.Type{core.TInteger}, nil, 0, true, stk, core.SrcPos{Row: 1, Col: 1}, core.SrcPos{})

	// The values ride in the detail, and the detail is the dedup
	// key — so the slice passed here must be the same one the checker
	// renders (stk minus the unnamed allowance, 0 here).
	want := core.ReturnCountErrorText("r2", 1, 2, stk)
	if countDiags(r, want) != 1 {
		t.Fatalf("excess residual not flagged: diagnostics = %+v", r.Check.Diagnostics)
	}

	// Same shape again: deduped by detail (one analysed shape, many sites).
	checkBodyReturnConformance(r, "r2", []*core.Type{core.TInteger}, nil, 0, true, stk, core.SrcPos{Row: 9, Col: 9}, core.SrcPos{})
	if countDiags(r, want) != 1 {
		t.Error("duplicate count diagnostic not deduped by detail")
	}
}

func TestReturnCountConformanceUnnamedAllowance(t *testing.T) {
	// One unnamed param: an unconsumed arg sits at the residual bottom and
	// the runtime DISCARDS it (extra ≤ UnnamedCount) — no count error, and
	// the type check aligns against the TOP declared-count values.
	r := countCheckRegistry(t)
	stk := []core.Value{core.NewInteger(7), core.NewInteger(42)}
	checkBodyReturnConformance(r, "okun", []*core.Type{core.TInteger}, nil, 1, true, stk, core.SrcPos{}, core.SrcPos{})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("allowance-covered residual wrongly flagged: %+v", r.Check.Diagnostics)
	}

	// Two extras over a one-arg allowance: got reports len-unnamedCount,
	// exactly the runtime's returnCountError arithmetic.
	stk = []core.Value{core.NewInteger(7), core.NewInteger(8), core.NewInteger(42)}
	checkBodyReturnConformance(r, "okun", []*core.Type{core.TInteger}, nil, 1, true, stk, core.SrcPos{}, core.SrcPos{})
	if countDiags(r, core.ReturnCountErrorText("okun", 1, 2, stk[1:])) != 1 {
		t.Fatalf("beyond-allowance residual not flagged: %+v", r.Check.Diagnostics)
	}
}

func TestReturnCountConformanceSkipsUnknowableCounts(t *testing.T) {
	r := countCheckRegistry(t)

	// A variadic spread models 0-or-more values — count unknown, skip.
	stk := []core.Value{core.NewVariadicCarrier(core.NewTypeLiteral(core.TNever)), core.NewInteger(1)}
	checkBodyReturnConformance(r, "vr", []*core.Type{core.TInteger}, nil, 0, true, stk, core.SrcPos{}, core.SrcPos{})

	// A Function value in the residual may be an unapplied fn-value call the
	// static model over-counts (emit.go cluster E) — skip.
	fnv := core.NewFunction(core.FnDefInfo{Name: "inner"})
	stk = []core.Value{fnv, core.NewInteger(1)}
	checkBodyReturnConformance(r, "fv", []*core.Type{core.TInteger}, nil, 0, true, stk, core.SrcPos{}, core.SrcPos{})

	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("unknowable-count residuals wrongly flagged: %+v", r.Check.Diagnostics)
	}
}

func TestReturnCountConformanceSkipsDynamicResiduals(t *testing.T) {
	// A DYNAMIC value in the residual marks a modelling seam (a mid-body
	// apply, a gradual branch join) where the static count may diverge
	// from the runtime one — the count mirror must skip it.
	r := countCheckRegistry(t)
	dyn := core.NewCarrier(core.TAny)
	core.SetDynamic(&dyn, true)
	stk := []core.Value{dyn, core.NewInteger(1)}
	checkBodyReturnConformance(r, "dy", []*core.Type{core.TInteger}, nil, 0, true, stk, core.SrcPos{}, core.SrcPos{})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("dynamic residual wrongly count-flagged: %+v", r.Check.Diagnostics)
	}
}
