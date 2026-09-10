package langspec

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestVariadicSpreadOracle pins the soundness-oracle contract for a
// variadic-spread carrier (the recursion.tsv:53 void-recursion residual;
// `while`'s trip count; await's winner-takes-all residual, NUR067): it absorbs
// ANY number of runtime entries (count flexibility) but each must still be a
// member of the element type (type strictness) — a wrong-typed leak is still
// flagged. Pairs the positives with the negatives at EVERY position the spread
// can take: alone, with a fixed run above it (the recursive-fn residual), with
// a fixed run BELOW it (`99 await {mode:'first'} [[1 2 3]]` — checked
// [Integer, spread] against a runtime [99 1 2 3]), and with fixed runs on both
// sides. The position mattered: the oracle used to read the spread only at
// checked[0], and rejected the below-a-fixed-entry shape outright.
func TestVariadicSpreadOracle(t *testing.T) {
	varInt := core.NewVariadicCarrier(core.NewTypeLiteral(core.TInteger))
	i := core.NewInteger(6)
	s := core.NewString("x")
	strCarrier := core.NewCarrier(core.TString)

	cases := []struct {
		name    string
		checked []core.Value
		actual  []core.Value
		want    bool
	}{
		{"three integers covered", []core.Value{varInt}, []core.Value{i, i, i}, true},
		{"zero trailing covered", []core.Value{varInt}, nil, true},
		{"string leak rejected", []core.Value{varInt}, []core.Value{i, s, i}, false},
		{"fixed prefix + integer spread", []core.Value{varInt, strCarrier}, []core.Value{i, i, s}, true},
		{"fixed prefix wrong type", []core.Value{varInt, strCarrier}, []core.Value{i, i, i}, false},
		{"missing fixed prefix", []core.Value{varInt, strCarrier}, nil, false},

		// The spread ABOVE a fixed entry — await's `99 await …` shape. The
		// fixed entry aligns with the BOTTOM of the runtime stack and the
		// spread absorbs everything above it.
		{"fixed bottom + integer spread", []core.Value{strCarrier, varInt}, []core.Value{s, i, i, i}, true},
		{"fixed bottom, zero absorbed", []core.Value{strCarrier, varInt}, []core.Value{s}, true},
		{"fixed bottom wrong type", []core.Value{strCarrier, varInt}, []core.Value{i, i, i}, false},
		{"leak above a fixed bottom", []core.Value{strCarrier, varInt}, []core.Value{s, i, s}, false},
		{"fixed bottom missing", []core.Value{strCarrier, varInt}, nil, false},

		// Fixed runs on BOTH sides: bottom-aligned below, top-aligned above,
		// absorbed in between — and a leak in the absorbed middle is still
		// flagged even though both fixed ends match.
		{"fixed both sides", []core.Value{strCarrier, varInt, strCarrier}, []core.Value{s, i, i, s}, true},
		{"fixed both sides, zero absorbed", []core.Value{strCarrier, varInt, strCarrier}, []core.Value{s, s}, true},
		{"fixed both sides, middle leak", []core.Value{strCarrier, varInt, strCarrier}, []core.Value{s, i, s, s}, false},
		{"fixed both sides, top wrong", []core.Value{strCarrier, varInt, strCarrier}, []core.Value{s, i, i, i}, false},
		{"fixed both sides too short", []core.Value{strCarrier, varInt, strCarrier}, []core.Value{s}, false},
	}
	for _, c := range cases {
		if got := stackTypeCovered(c.checked, c.actual); got != c.want {
			t.Errorf("%s: stackTypeCovered = %v, want %v", c.name, got, c.want)
		}
	}
}
