package native

import (
	"strings"
	"testing"
)

// Direct seam coverage for the guaranteed-runtime-error mirror helpers
// (design/legacy/CHECKER-COMPLETION.0.ignore): the guard arms a full check pass
// cannot reach — malformed arg shapes, unresolvable schemas — so the
// mirrors stay total under any caller.

func mirrorReg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Check.Begin())
	return r
}

func TestClassSchemaOfGuards(t *testing.T) {
	r := mirrorReg(t)

	// A zero Value (nil Parent) resolves no schema.
	if _, ok := classSchemaOf(r, Value{}); ok {
		t.Error("nil-Parent receiver resolved a class schema")
	}
	// A receiver whose type name is bound to a NON-class body resolves
	// no schema (AsClassType declines).
	if _, ok := classSchemaOf(r, NewCarrier(TInteger)); ok {
		t.Error("non-class type binding resolved a class schema")
	}
}

func TestSetClassInstanceReturnsGuards(t *testing.T) {
	r := mirrorReg(t)

	// Short arg slice: no diagnostic, the declared no-residual model.
	if out := setClassInstanceReturns([]Value{NewAtom("x")}, r); len(out) != 0 {
		t.Errorf("short-args residual = %v, want empty", out)
	}
	// Non-concrete key: silent (a computed key proves nothing).
	if out := setClassInstanceReturns([]Value{NewCarrier(TAtom), NewInteger(1), NewCarrier(TClass)}, r); len(out) != 0 {
		t.Errorf("carrier-key residual = %v, want empty", out)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("guard arms emitted diagnostics: %+v", r.Check.Diagnostics)
	}
}

func TestSetListIndexReturnsGuards(t *testing.T) {
	r := mirrorReg(t)
	// Short arg slice: the List model without a bounds check.
	out := setListIndexReturns([]Value{NewInteger(0)}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TList) {
		t.Errorf("short-args residual = %v, want one List carrier", out)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("short-args emitted diagnostics: %+v", r.Check.Diagnostics)
	}
}

func TestGetrObjectReturnsUnresolvableSchema(t *testing.T) {
	r := mirrorReg(t)
	// A COMPUTED key proves nothing — the guard arm returns the base
	// result with no miss claimed.
	if out := getrObjectReturns([]Value{NewCarrier(TAtom), NewCarrier(TClass)}, r); len(out) != 1 {
		t.Fatalf("computed-key residual = %v, want one value", out)
	}
	// A class-typed receiver whose type binding is gone: getObjectReturns'
	// result flows through unchanged and no miss is claimed.
	recv := NewCarrier(TClass)
	out := getrObjectReturns([]Value{NewAtom("zzz"), recv}, r)
	if len(out) != 1 {
		t.Fatalf("residual = %v, want one value", out)
	}
	for _, d := range r.Check.Diagnostics {
		if d.Code == "not_found" {
			t.Errorf("unresolvable schema wrongly claimed a static miss: %+v", d)
		}
	}
}

func TestGetrNodeReturnsGuards(t *testing.T) {
	r := mirrorReg(t)
	// A concrete map whose AsMap read succeeds and whose key is PRESENT:
	// no diagnostic, the field narrowing flows through.
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	out := getrNodeReturns([]Value{NewString("a"), NewMap(m)}, r)
	if len(out) != 1 {
		t.Fatalf("residual = %v, want one value", out)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("present key emitted diagnostics: %+v", r.Check.Diagnostics)
	}
}

func TestRaiseReturnsGuards(t *testing.T) {
	r := mirrorReg(t)
	// Zero args (defensive): silent, empty residual.
	if out := raiseReturns(nil, r); len(out) != 0 {
		t.Errorf("nil-args residual = %v, want empty", out)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("nil-args emitted diagnostics: %+v", r.Check.Diagnostics)
	}
}

func TestReturnsDivModNonZeroDelegates(t *testing.T) {
	r := mirrorReg(t)
	out := returnsDivMod("division by zero")([]Value{NewInteger(2), NewInteger(10)}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TInteger) {
		t.Errorf("non-zero divisor residual = %v, want Integer carrier", out)
	}
}

func TestCheckBigFloatMixGuards(t *testing.T) {
	r := mirrorReg(t)
	// Bare type nodes are excluded (numLeaf would misclassify a literal).
	checkBigFloatMix(r, []Value{NewTypeLiteral(TFloat), NewTypeLiteral(TBigInteger)})
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("type-literal operands wrongly flagged: %+v", r.Check.Diagnostics)
	}
}

func TestUnpackSourceProvenRecord(t *testing.T) {
	// The record branch reports proven=true; strings.Contains sanity keeps
	// the import used if assertions change shape later.
	r := mirrorReg(t)
	fields := NewOrderedMap()
	fields.Set("k", NewInteger(9))
	_, keys, proven, err := unpackSource(NewRecordType(fields), r)
	if err != nil || !proven || len(keys) != 1 || !strings.Contains(keys[0], "k") {
		t.Fatalf("record source: keys=%v proven=%v err=%v", keys, proven, err)
	}
}

func TestGetrObjectReturnsNonClassBinding(t *testing.T) {
	// The receiver's type name resolves to a NON-class body (a refine
	// prefab): AsClassType declines and no static miss is claimed.
	r := mirrorReg(t)
	minted := r.Types.MintTypeWithBehavior("NotACls", TMap, nil)
	r.Defs.PushType("NotACls", minted, NewTypeLiteral(minted))
	out := getrObjectReturns([]Value{NewAtom("zzz"), NewCarrier(minted)}, r)
	if len(out) != 1 {
		t.Fatalf("residual = %v, want one value", out)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("non-class binding wrongly claimed a miss: %+v", r.Check.Diagnostics)
	}
}

func TestSetClassInstanceReturnsNamelessSchema(t *testing.T) {
	// A class schema whose Name is empty falls back to the receiver's
	// type-node name in the sealed_field text — mirroring the runtime
	// handler's identical fallback.
	r := mirrorReg(t)
	minted := r.Types.MintTypeWithBehavior("AnonCls", TClass, nil)
	info := ClassTypeInfo{Fields: NewOrderedMap()}
	r.Defs.PushType("AnonCls", minted, NewClassType(minted, info))
	out := setClassInstanceReturns([]Value{NewAtom("zzz"), NewInteger(1), NewCarrier(minted)}, r)
	if len(out) != 0 {
		t.Fatalf("residual = %v, want empty", out)
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "sealed_field" && strings.Contains(d.Detail, "AnonCls") {
			found = true
		}
	}
	if !found {
		t.Errorf("nameless schema did not fall back to the type-node name: %+v", r.Check.Diagnostics)
	}
}

func TestIsStaticZeroIntDivisorBigDecimalGuard(t *testing.T) {
	// A BigDecimal-typed value whose payload is not a decimal (defensive
	// seam — the sealed-payload rules make this a hand-built shape only)
	// answers false rather than claiming a zero divisor.
	v := Value{Parent: TBigDecimal, Data: IntPayload{N: 0}}
	if isStaticZeroIntDivisor(v) {
		t.Error("non-decimal payload wrongly classified as a zero divisor")
	}
}
