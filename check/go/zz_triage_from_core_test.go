package check

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

func TestCheckListIndexBounds(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	list := core.NewList([]core.Value{core.NewInteger(10), core.NewInteger(20)})

	// In-range: silent.
	CheckListIndex(r, core.NewInteger(1), list, "get")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("in-range index flagged: %+v", r.Check.Diagnostics)
	}
	// Provably out of range: flagged.
	CheckListIndex(r, core.NewInteger(9), list, "get")
	if len(r.Check.Diagnostics) == 0 {
		t.Fatal("out-of-range index not flagged")
	}

	n := len(r.Check.Diagnostics)
	// `at` over a list of indices: one bad index flags once.
	CheckAtIndices(r, core.NewList([]core.Value{core.NewInteger(0), core.NewInteger(7)}), list, "at")
	if len(r.Check.Diagnostics) != n+1 {
		t.Errorf("at-indices diagnostics = %d, want %d", len(r.Check.Diagnostics), n+1)
	}
	// Non-concrete indices are skipped without panic.
	CheckAtIndices(r, core.NewTypeLiteral(core.TList), list, "at")
}

func TestReturnsFuncBuilders(t *testing.T) {
	// ReturnsIdentity: repeated sources get fresh IDs.
	idfn := core.ReturnsIdentity(0, 0)
	in := core.NewInteger(5)
	in.ID = core.GenerateID("i_")
	out := idfn([]core.Value{in}, nil)
	if len(out) != 2 {
		t.Fatalf("identity out = %v", out)
	}
	if out[0].ID == out[1].ID {
		t.Error("duplicated identity shares an ID")
	}
	// Out-of-range mapping degrades to Any.
	out = core.ReturnsIdentity(3)([]core.Value{in}, nil)
	if !out[0].Parent.Equal(core.TAny) {
		t.Errorf("out-of-range identity = %v", out)
	}

	// ReturnsStatic.
	out = ReturnsStatic(core.TInteger, core.TString)(nil, nil)
	if len(out) != 2 || !out[0].Parent.Equal(core.TInteger) || !out[1].Parent.Equal(core.TString) {
		t.Errorf("ReturnsStatic = %v", out)
	}

	// ReturnsNumericBinary follows the arithmetic tower.
	nb := ReturnsNumericBinary()
	if out := nb([]core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TInteger)}, nil); !out[0].Parent.Equal(core.TInteger) {
		t.Errorf("int⊕int = %v", out)
	}
	if out := nb([]core.Value{core.NewCarrier(core.TFloat), core.NewCarrier(core.TInteger)}, nil); !out[0].Parent.Equal(core.TFloat) {
		t.Errorf("float⊕int = %v", out)
	}
	if out := nb([]core.Value{core.NewCarrier(core.TBigInteger), core.NewCarrier(core.TInteger)}, nil); !out[0].Parent.Equal(core.TBigInteger) {
		t.Errorf("big⊕int = %v", out)
	}
	if out := nb([]core.Value{core.NewCarrier(core.TBigDecimal), core.NewCarrier(core.TFloat)}, nil); !out[0].Parent.Equal(core.TBigDecimal) {
		t.Errorf("dec⊕float = %v", out)
	}
	if out := nb([]core.Value{core.NewCarrier(core.TInteger)}, nil); !out[0].Parent.Equal(core.TFloat) {
		t.Errorf("degenerate arity = %v", out)
	}

	// ReturnsAddConcat: provable String → String; otherwise gradual Scalar.
	ac := ReturnsAddConcat()
	if out := ac([]core.Value{core.NewString("a"), core.NewCarrier(core.TInteger)}, nil); !out[0].Parent.Equal(core.TString) {
		t.Errorf("string concat = %v", out)
	}
	dynamic := core.NewDynamicCarrier(core.TAny)
	if out := ac([]core.Value{dynamic, core.NewCarrier(core.TInteger)}, nil); !out[0].Dynamic {
		t.Errorf("gradual concat = %v", out)
	}

	// ReturnsFreshInstance: a concrete type-node target yields a fresh
	// value carrier of that type, not the literal.
	fi := ReturnsFreshInstance(0)
	out = fi([]core.Value{core.NewTypeLiteral(core.TPathon), mapOf()}, nil)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(core.TPathon) {
		t.Errorf("fresh instance = %v", out)
	}
	a := fi([]core.Value{core.NewTypeLiteral(core.TPathon), mapOf()}, nil)[0]
	b := fi([]core.Value{core.NewTypeLiteral(core.TPathon), mapOf()}, nil)[0]
	if a.ID == b.ID {
		t.Error("two constructions share an identity")
	}
	// A carrier target keeps identity behaviour; an out-of-range slot is Any.
	out = fi([]core.Value{core.NewCarrier(core.TAny)}, nil)
	if len(out) != 1 {
		t.Errorf("carrier target = %v", out)
	}
	out = ReturnsFreshInstance(4)([]core.Value{core.NewTypeLiteral(core.TPathon)}, nil)
	if !out[0].Parent.Equal(core.TAny) {
		t.Errorf("out-of-range fresh = %v", out)
	}
}

// unifyTyped{List,Map}WithConcrete's !IsConcrete guard (typed flex layer) tags
// an abstract CARRIER gradually. Ordinary dispatch routes flex carriers through
// unifyCarrierVsTyped first (and only the shape-tracked MAP carrier reaches the
// map twin), so the LIST arm is dispatch-unreachable — pin both by direct call.
// paramBodyCarrier (poly-arm + construction-check body-input builder) makes a
// {:T}/[:T] param's body reads narrow. Its typed arms are pinned by direct call
// (the census exercises the compile paths; the branch coverage is here).
func TestParamBodyCarrier(t *testing.T) {
	mapPat := core.NewTypedMap(core.NewTypeLiteral(core.TInteger))
	// A {:T} param carrier must be DYNAMIC — the arg may be flex, so an in-place
	// mutation in the body must runtime-rematch rather than preselect the
	// immutable handler (the compile-vs-interpret divergence fix).
	if v := ParamBodyCarrier(core.FnParam{Pattern: &mapPat, Type: core.TMap}); !core.IsTypedMap(&v) || !v.Dynamic {
		t.Errorf("{:Integer} param → %s (dynamic=%v), want a dynamic typed-map carrier", v.Parent.String(), v.Dynamic)
	}
	listPat := core.NewCarrierTypedListValue(core.NewTypeLiteral(core.TInteger))
	if v := ParamBodyCarrier(core.FnParam{Pattern: &listPat, Type: core.TList}); !core.IsTypedList(&v) || !v.Dynamic {
		t.Errorf("[:Integer] param → %s (dynamic=%v), want a dynamic typed-list carrier", v.Parent.String(), v.Dynamic)
	}
	// A non-typed-container param falls back to ParamInputCarrier (no pattern).
	if v := ParamBodyCarrier(core.FnParam{Type: core.TInteger}); v.Parent == nil || !v.Parent.ConformsTo(core.TInteger) {
		t.Errorf("scalar param → %s, want an Integer carrier", v.Parent.String())
	}
	// A {:Any} pattern (child Any) falls through to ParamInputCarrier(TMap).
	anyPat := core.NewTypedMap(core.NewTypeLiteral(core.TAny))
	if v := ParamBodyCarrier(core.FnParam{Pattern: &anyPat, Type: core.TMap}); v.Parent == nil || !v.Parent.ConformsTo(core.TMap) {
		t.Errorf("{:Any} param → %s, want a Map carrier", v.Parent.String())
	}
}
