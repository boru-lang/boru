package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestDynamicCarrierMatch pins the not-disjoint matching rule for the
// bounded dynamic(T) modality (design/legacy/dynamic-modality-report.10.ignore): a
// dynamic carrier matches a signature slot unless its bound is PROVABLY
// disjoint from the slot, the optimistic dual of strict ConformsTo. The
// contrast block proves the modality is what flips the behaviour — a
// non-dynamic Carry<Any> still fails an Integer slot.
func TestDynamicCarrierMatch(t *testing.T) {
	dynDisjunct := core.NewDynamicCarrierValue(
		core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString)}),
	)

	cases := []struct {
		name string
		v    core.Value
		slot *core.Type
		want bool
	}{
		// dynamic(Any) — classic gradual any: compatible with every slot.
		{"dyn(Any) vs Integer", core.NewDynamicCarrier(core.TAny), core.TInteger, true},
		{"dyn(Any) vs String", core.NewDynamicCarrier(core.TAny), core.TString, true},
		{"dyn(Any) vs Atom", core.NewDynamicCarrier(core.TAny), core.TAtom, true},
		{"dyn(Any) vs List", core.NewDynamicCarrier(core.TAny), core.TList, true},
		{"dyn(Any) vs Any", core.NewDynamicCarrier(core.TAny), core.TAny, true},

		// dynamic(Integer) — matches overlapping slots, fails disjoint ones.
		{"dyn(Integer) vs Integer", core.NewDynamicCarrier(core.TInteger), core.TInteger, true},
		{"dyn(Integer) vs Number", core.NewDynamicCarrier(core.TInteger), core.TNumber, true},
		{"dyn(Integer) vs Scalar", core.NewDynamicCarrier(core.TInteger), core.TScalar, true},
		{"dyn(Integer) vs Any", core.NewDynamicCarrier(core.TInteger), core.TAny, true},
		{"dyn(Integer) vs String", core.NewDynamicCarrier(core.TInteger), core.TString, false},
		{"dyn(Integer) vs Atom", core.NewDynamicCarrier(core.TInteger), core.TAtom, false},
		{"dyn(Integer) vs Boolean", core.NewDynamicCarrier(core.TInteger), core.TBoolean, false},
		{"dyn(Integer) vs List", core.NewDynamicCarrier(core.TInteger), core.TList, false},

		// dynamic(Number) overlaps Integer (a dynamic Number could be one).
		{"dyn(Number) vs Integer", core.NewDynamicCarrier(core.TNumber), core.TInteger, true},

		// dynamic(Integer tor String) — union bound, member-wise overlap.
		{"dyn(Int|Str) vs Number", dynDisjunct, core.TNumber, true},
		{"dyn(Int|Str) vs String", dynDisjunct, core.TString, true},
		{"dyn(Int|Str) vs Integer", dynDisjunct, core.TInteger, true},
		{"dyn(Int|Str) vs Atom", dynDisjunct, core.TAtom, false},
		{"dyn(Int|Str) vs List", dynDisjunct, core.TList, false},

		// Contrast: a NON-dynamic carrier keeps strict ConformsTo. This is
		// the whole point — strict Carry<Any> fails Integer, the footgun
		// dynamic exists to fix; and Carry<Integer> still fails String.
		{"strict Carry<Any> vs Integer", core.NewCarrier(core.TAny), core.TInteger, false},
		{"strict Carry<Integer> vs Integer", core.NewCarrier(core.TInteger), core.TInteger, true},
		{"strict Carry<Integer> vs Number", core.NewCarrier(core.TInteger), core.TNumber, true},
		{"strict Carry<Integer> vs String", core.NewCarrier(core.TInteger), core.TString, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.SigTypeMatches(tc.v, tc.slot); got != tc.want {
				t.Errorf("sigTypeMatches = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDynamicResultContagion pins gradual contagion and its refinement: a
// result derived from a dynamic carrier is dynamic EXCEPT when its declared
// return is a CONCRETE, single-reachable CONTAINER (List/Map) — that type is
// fixed by the word's contract regardless of which runtime value flowed in,
// so a downstream `each`/`fold`/accessor can commit on it (the cross-module
// element-typing flip). A concrete SCALAR return (Integer) still rides
// dynamic (keeping it strict gains no consumer and can trip the forward/stack
// split detector), as does a genuinely input-dependent Any return. Strict
// args always yield a strict result.
func TestDynamicResultContagion(t *testing.T) {
	r, _ := core.NewRegistry()
	listSig := &core.Signature{Returns: []*core.Type{core.TList}}

	strict := CarrierResults(r, "w", listSig, []core.Value{core.NewCarrier(core.TString)}, core.SrcPos{}, nil, false)
	if len(strict) != 1 || strict[0].Dynamic {
		t.Errorf("strict args must yield a strict result, got Dynamic=%v", strict[0].Dynamic)
	}

	// A dynamic input + a CONCRETE, single-reachable CONTAINER return: the
	// result type is statically known (List), so it stays STRICT — a
	// downstream `each` over such a declared-List return can now commit
	// instead of declining "dynamic input at each".
	cont := CarrierResults(r, "w", listSig, []core.Value{core.NewDynamicCarrier(core.TAny)}, core.SrcPos{}, nil, false)
	if len(cont) != 1 || cont[0].Dynamic {
		t.Fatalf("a concrete container return stays strict over a dynamic input, got Dynamic=%v", cont[0].Dynamic)
	}
	if !cont[0].Parent.Equal(core.TList) {
		t.Errorf("result bound = %s, want the declared return List", cont[0].Parent)
	}

	// A dynamic input + a concrete SCALAR return stays dynamic (contagion is
	// scoped to containers; a strict scalar gains nothing and can trip the
	// forward/stack split detector).
	intSig := &core.Signature{Returns: []*core.Type{core.TInteger}}
	sc := CarrierResults(r, "w", intSig, []core.Value{core.NewDynamicCarrier(core.TAny)}, core.SrcPos{}, nil, false)
	if len(sc) != 1 || !sc[0].Dynamic {
		t.Fatalf("a dynamic input + scalar return must stay dynamic, got Dynamic=%v", sc[0].Dynamic)
	}

	// A dynamic input + a declared ANY return IS input-dependent (the type
	// is genuinely unknown), so the modality flows downstream as dynamic.
	anySig := &core.Signature{Returns: []*core.Type{core.TAny}}
	dyn := CarrierResults(r, "w", anySig, []core.Value{core.NewDynamicCarrier(core.TAny)}, core.SrcPos{}, nil, false)
	if len(dyn) != 1 || !dyn[0].Dynamic {
		t.Fatalf("a dynamic input + Any return must stay dynamic, got Dynamic=%v", dyn[0].Dynamic)
	}
}

// TestDynamicFirstMatchPartition pins the first-match partition for the
// dispatch RESULT: when a dynamic bound reaches several overloads with
// divergent returns, the result is the UNION of those returns (sound),
// not just the first match's (too narrow). No production word has
// return-divergent overloads spanned by a dynamic bound, so this is
// verified with a synthetic word.
func TestDynamicFirstMatchPartition(t *testing.T) {
	r, _ := core.NewRegistry()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "wdiv",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TBoolean}},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TAtom}},
		},
	})
	intStr := core.NewDynamicCarrierValue(core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString)}))

	// dynamic(Integer|String) reaches BOTH overloads → union {Boolean, Atom}.
	if rets := dynamicReachableReturns(r, "wdiv", []core.Value{intStr}); len(rets) != 2 {
		t.Fatalf("expected 2 reachable returns for dynamic(Integer|String), got %v", rets)
	}

	// The dispatch result is dynamic(Atom tor Boolean) — a union whose terms
	// print in canonical tcmp order (disjuncts are sorted at construction).
	sig := &core.Signature{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TBoolean}}
	out := CarrierResults(r, "wdiv", sig, []core.Value{intStr}, core.SrcPos{}, nil, false)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("expected a single dynamic result, got %v", out)
	}
	if got := out[0].String(); got != "dynamic(Atom tor Boolean)" {
		t.Errorf("partition result = %q, want dynamic(Atom tor Boolean)", got)
	}

	// A dynamic bound reaching only ONE overload → no refinement (the
	// matched return is already correct).
	if got := dynamicReachableReturns(r, "wdiv", []core.Value{core.NewDynamicCarrier(core.TInteger)}); got != nil {
		t.Errorf("dynamic(Integer) reaches one overload; expected no union, got %v", got)
	}
}

// TestDynamicCarrierString pins the trace rendering: a dynamic carrier
// renders as dynamic(<bound>) so the modality is legible, while a strict
// carrier is unchanged.
func TestDynamicCarrierString(t *testing.T) {
	cases := []struct {
		v    core.Value
		want string
	}{
		{core.NewDynamicCarrier(core.TInteger), "dynamic(Integer)"},
		{core.NewDynamicCarrier(core.TAny), "dynamic(Any)"},
		{core.NewDynamicCarrierValue(core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString)})), "dynamic(Integer tor String)"},
		{core.NewCarrier(core.TInteger), "Integer"}, // strict carrier unchanged
	}
	for _, tc := range cases {
		if got := tc.v.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

// TestDynamicCarrierConstructors pins the invariants of the dynamic
// constructors: Dynamic implies Carrier, the bound is preserved, and
// toCarrier never strips a dynamic carrier (which would null its bound).
func TestDynamicCarrierConstructors(t *testing.T) {
	d := core.NewDynamicCarrier(core.TInteger)
	if !d.Dynamic || !d.Carrier {
		t.Fatalf("NewDynamicCarrier: Dynamic=%v Carrier=%v, want both true", d.Dynamic, d.Carrier)
	}
	if !d.Parent.Equal(core.TInteger) {
		t.Errorf("NewDynamicCarrier bound = %s, want Integer", d.Parent)
	}

	// A disjunct promoted to dynamic keeps its alternatives.
	disj := core.NewDynamicCarrierValue(
		core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString)}),
	)
	if !disj.Dynamic || !core.IsDisjunct(disj) {
		t.Errorf("core.NewDynamicCarrierValue: Dynamic=%v IsDisjunct=%v, want both true", disj.Dynamic, core.IsDisjunct(disj))
	}

	// toCarrier must return a dynamic carrier unchanged — same bound,
	// flag intact.
	got := toCarrier(d)
	if !got.Dynamic || !got.Parent.Equal(core.TInteger) {
		t.Errorf("toCarrier(dynamic) = Dynamic %v / %s, want dynamic Integer", got.Dynamic, got.Parent)
	}
}
