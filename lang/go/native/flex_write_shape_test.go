package native

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
)

// flex_write_shape_test.go pins the check-mode models behind the last five
// non-NUR runtime defers (runtime_defers.tsv, 2026-09-26):
//
//   - getIntKeyReturns hands a quoted list's LIVE word node back verbatim, so
//     the check pass re-steps it as the interpreter does (edge-quote-1.tsv
//     L28, edge-quote-3.tsv L56);
//   - a flex write records into the receiver's shape on the COMPILE pass too
//     (setFlexMapReturns), and a FlexList keeps an element-join shape its
//     writers extend and its index reads surface (flex.tsv L228/L230/L236).

func TestGetIntKeyReturnsLiveWordReSteps(t *testing.T) {
	r := w9Reg(t)
	w := NewWord("add")
	out := getIntKeyReturns([]Value{NewInteger(0), w9List(w, NewInteger(1))}, r)
	if len(out) != 1 || !IsWord(out[0]) || IsBareTypeNode(out[0]) {
		t.Fatalf("a live word element comes back as the token itself: %v", out)
	}
	// A bare type node is a type literal — data, the carrier path.
	lit := NewTypeLiteral(TInteger)
	out = getIntKeyReturns([]Value{NewInteger(0), w9List(lit)}, r)
	if len(out) != 1 || !out[0].Carrier {
		t.Fatalf("a type-literal element reads as a carrier, not a token: %v", out)
	}
}

func TestSetFlexMapReturnsRecordsOnTheCompilePass(t *testing.T) {
	r := w9Reg(t)
	r.Check.Mode = true
	r.Check.Compiling = true
	recv, ok := check.MintFlexShapeCarrier(NewMap(NewOrderedMap()), 0)
	if !ok {
		t.Fatal("mint")
	}
	inner := NewOrderedMap()
	inner.Set("b", NewInteger(1))
	out := setFlexMapReturns([]Value{NewAtom("a"), NewMap(inner), recv}, r)
	if len(out) != 1 || out[0].Data == recv.Data {
		t.Fatalf("the compile pass keeps the legacy fresh carrier: %v", out)
	}
	ss, _ := check.StoreShapeOf(recv)
	v, hit := ss.LookupKey("a")
	if !hit {
		t.Fatal("the compile pass must record the write into the receiver's shape")
	}
	if _, ok := check.StoreShapeOf(v); !ok || !v.Parent.Equal(TFlexMap) {
		t.Errorf("a written plain map adopts to a FlexMap shape: %v", v)
	}
	// The plain pass records AND hands the receiver on (unchanged).
	r.Check.Compiling = false
	if out := setFlexMapReturns([]Value{NewAtom("c"), NewInteger(1), recv}, r); out[0].Data != recv.Data {
		t.Error("the plain pass returns the receiver itself")
	}
}

// A shaped FlexList: `flex [...]` mints it; push / unshift / append / indexed
// set join into its element bound; an index read surfaces the join gradual.
func TestFlexListShapeWritersAndRead(t *testing.T) {
	r := w9Reg(t)
	r.Check.Mode = true
	// flex over an empty concrete list mints a shaped FlexList.
	f := flexReturns([]Value{NewList([]Value{})}, r)[0]
	ss, ok := check.FlexListShapeOf(f)
	if !ok {
		t.Fatalf("flex [] mints a FlexList shape: %v", f)
	}
	// An empty join reads dynamic(Any).
	if out := getIntKeyReturns([]Value{NewInteger(0), f}, r); !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
		t.Fatalf("an empty join reads dynamic(Any): %v", out)
	}
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	flexGrowReturns("push")([]Value{NewMap(m), f}, r)
	out := getIntKeyReturns([]Value{NewInteger(0), f}, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.ConformsTo(TFlexMap) {
		t.Fatalf("a pushed map reads back as a gradual FlexMap: %v", out)
	}
	// Indexed set and a concrete append join too.
	setFlexListReturns([]Value{NewInteger(0), NewInteger(5), f}, r)
	appendListReturns([]Value{w9List(NewString("s")), f}, r)
	if _, hit := ss.LookupVals(); !hit {
		t.Fatal("the writers keep a join")
	}
	// A computed append source widens the join to dynamic(Any).
	appendListReturns([]Value{NewCarrier(TList), f}, r)
	v, _ := ss.LookupVals()
	if !v.Dynamic || !v.Parent.Equal(TAny) {
		t.Errorf("a computed append widens the element join to dynamic(Any): %v", v)
	}
	// A bare (unshaped) FlexList receiver: the writers are no-ops, the read
	// stays dynamic(Any).
	bare := NewCarrier(TFlexList)
	flexGrowReturns("push")([]Value{NewInteger(1), bare}, r)
	if out := getIntKeyReturns([]Value{NewInteger(0), bare}, r); !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
		t.Errorf("an unshaped FlexList reads dynamic(Any): %v", out)
	}
	// A typed list source keeps the D2 carrier (no shape).
	typed := NewTypedListWithElements(NewTypeLiteral(TInteger), []Value{NewInteger(1)})
	if _, ok := check.FlexListShapeOf(flexReturns([]Value{typed}, r)[0]); ok {
		t.Error("a typed list source keeps the bare, tagged carrier")
	}
}
