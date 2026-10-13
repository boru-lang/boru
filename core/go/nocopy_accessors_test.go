package core

import (
	"testing"
	"unsafe"
)

// Value.Is runs the default match in place for a behavior that carries the
// matchesAsDefault marker and never calls that behavior's Match: the marker
// promises its Match IS the default match. Pin the promise for every marked
// behavior, so a Match edited to do something else fails here instead of
// being silently bypassed.
func TestMarkedBehaviorsMatchByDefault(t *testing.T) {
	marked := []TypeBehavior{
		objectConvertBehavior{}, storeConvertBehavior{}, errorConvertBehavior{},
		reachConvertBehavior{}, tableConvertBehavior{}, xmlBehavior{},
		listFormatBehavior{}, mapFormatBehavior{}, moduleTypeBehavior{},
	}
	vals := []Value{NewInteger(1), NewString("s"), NewList([]Value{NewInteger(1)}), NewMap(NewOrderedMap())}
	types := []*Type{TInteger, TString, TList, TMap, TAny}
	for _, b := range marked {
		if _, ok := b.(defaultMatcher); !ok {
			t.Errorf("%T lost its matchesAsDefault marker", b)
		}
		for i := range vals {
			for _, ty := range types {
				if got, want := b.Match(vals[i], ty), defaultMatch(&vals[i], ty); got != want {
					t.Errorf("%T.Match(%s, %s) = %v; the default match says %v", b, vals[i].String(), IDOf(ty), got, want)
				}
			}
		}
	}
}

// The no-copy readers mirror their copying twins: TopRef answers what Top
// answers, without the copy, and a nil table binds nothing.
func TestDefTableTopRefMirrorsTop(t *testing.T) {
	var none *DefTable
	if e, ok := none.TopRef("x"); ok || e != nil {
		t.Error("a nil DefTable binds nothing")
	}
	dt := NewDefTable()
	dt.Push("x", NewInteger(7))
	e, ok := dt.TopRef("x")
	if !ok || e == nil {
		t.Fatal("TopRef must find a bound name")
	}
	if n, _ := AsInteger(e.Body); n != 7 {
		t.Errorf("TopRef(x).Body = %v, want 7", e.Body)
	}
	if _, ok := dt.TopRef("y"); ok {
		t.Error("TopRef of an unbound name must report false")
	}
}

// Tape.Ref out of range is the tape's zero cell, as At's is the zero Value,
// and it is cleared on every such read.
func TestTapeRefOutOfRangeIsAZeroCell(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1)}, 0)
	for _, i := range []int{-1, 1, 5} {
		r := tp.Ref(i)
		if r == nil || r.Parent != nil || r.Data != nil {
			t.Fatalf("Ref(%d) = %v, want the zero cell", i, r)
		}
		SetQuoted(r, true) // a reader that wrote through it…
		if IsQuoted(tp.Ref(i)) {
			t.Fatalf("Ref(%d): the zero cell must be cleared on every read", i)
		}
	}
	if n, _ := AsInteger(*tp.Ref(0)); n != 1 {
		t.Errorf("Ref(0) = %v, want the cell", tp.Ref(0))
	}
}

// SetFrom is Set reading the new value where it lives: the same cell write,
// the same forward bookkeeping, and an out-of-range index ignored.
func TestTapeSetFromIsSet(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1), NewInteger(2)}, 0)
	fwd := NewForward(ForwardInfo{FuncName: "f"})
	tp.SetFrom(1, &fwd)
	if !IsForward(tp.Ref(1)) || !tp.hasForward() {
		t.Fatal("SetFrom must place the value and count the forward it places")
	}
	one := NewInteger(3)
	tp.SetFrom(1, &one)
	if n, _ := AsInteger(*tp.Ref(1)); n != 3 || tp.hasForward() {
		t.Fatalf("SetFrom over a forward must uncount it: %v %v", tp.Ref(1), tp.hasForward())
	}
	for _, i := range []int{-1, 2} {
		tp.SetFrom(i, &fwd)
	}
	if tp.Len() != 2 || tp.hasForward() {
		t.Error("an out-of-range SetFrom must change nothing")
	}
}

// TopValue is Top read where the binding lives: a value binding's Body, a
// type binding's node itself, nothing for an unbound name; SetTopData
// swaps the payload in place and moves the name's generation.
func TestDefTableTopValueReadsInPlace(t *testing.T) {
	dt := NewDefTable()
	if v, ok := dt.TopValue("x"); ok || v != nil {
		t.Fatal("an unbound name has no value")
	}
	dt.Push("x", NewInteger(1))
	ref, _ := dt.TopRef("x")
	if v, ok := dt.TopValue("x"); !ok || v != &ref.Body {
		t.Fatal("a value binding reads as its own Body")
	}
	dt.PushType("T", TInteger, NewTypeLiteral(TInteger))
	if v, ok := dt.TopValue("T"); !ok || v != TInteger {
		t.Fatal("a type binding reads as its lattice node itself")
	}
	gen := dt.Gen("x")
	if !dt.SetTopData("x", IntPayload{N: 2}) {
		t.Fatal("SetTopData on a bound name")
	}
	if v, _ := dt.TopValue("x"); dt.Gen("x") == gen || v != &ref.Body {
		t.Fatal("SetTopData writes the binding in place and moves its generation")
	}
	if n, _ := AsInteger(ref.Body); n != 2 {
		t.Fatalf("SetTopData payload = %v, want 2", ref.Body)
	}
	if dt.SetTopData("y", IntPayload{N: 1}) || dt.Has("y") {
		t.Error("SetTopData on an unbound name is a no-op")
	}
}

// ReadList.Ref is the element where it lives.
func TestReadListRefIsTheElement(t *testing.T) {
	elems := []Value{NewInteger(1), NewInteger(2)}
	rl := NewReadList(elems)
	if rl.Ref(1) != &elems[1] {
		t.Error("ReadList.Ref must point at the list's own element")
	}
}

// FnDispatchSigs builds a fn value's dispatch view once per identity and
// authored list: the all-forward barrier resolved, the sigs sorted; a second
// read is the same list, a rebuilt authored list (one identity, new sigs)
// rebuilds, and an identity-less payload builds per call.
func TestFnDispatchSigsCachedOnIdentity(t *testing.T) {
	one := Signature{Params: []FnParam{{Type: TInteger}}, BarrierPos: BarrierAllForward}
	fv := NewFunction(FnDefInfo{Signatures: []Signature{one}})
	fd := fv.Data.(*FnDefInfo)
	first := FnDispatchSigs(fd)
	if len(first) != 1 || first[0].BarrierPos != 1 {
		t.Fatalf("the all-forward barrier must resolve to the arg count: %+v", first)
	}
	if fd.Signatures[0].BarrierPos != BarrierAllForward {
		t.Fatal("the authored signature must stay as written")
	}
	if again := FnDispatchSigs(fd); &again[0] != &first[0] {
		t.Error("a second read of one function must hit the cache")
	}
	grown := *fd // one identity, a rebuilt authored list
	grown.Signatures = append([]Signature{{Params: []FnParam{{Type: TString}, {Type: TString}}}}, fd.Signatures...)
	if got := FnDispatchSigs(&grown); len(got) != 2 {
		t.Fatalf("a rebuilt list must rebuild the view, got %d sigs", len(got))
	}
	bare := FnDefInfo{Signatures: []Signature{one}} // no identity
	if a, b := FnDispatchSigs(&bare), FnDispatchSigs(&bare); &a[0] == &b[0] {
		t.Error("an identity-less payload has no cache")
	}
}

// A dynamic record carrier is built whole: dynamic, a carrier, the schema,
// and an ID of its own each time.
func TestNewDynamicRecordCarrier(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("n", NewTypeLiteral(TInteger))
	a, b := NewDynamicRecordCarrier(fields), NewDynamicRecordCarrier(fields)
	if !IsDynamic(&a) || !a.Carrier || !a.Parent.Equal(TMap) {
		t.Fatalf("want a dynamic Map carrier, got %v", a)
	}
	if ri, ok := a.Data.(RecordTypeInfo); !ok || ri.Fields != fields {
		t.Fatalf("the carrier must hold the schema, got %T", a.Data)
	}
	if IDOf(&a) == "" || IDOf(&a) == IDOf(&b) {
		t.Fatalf("each carrier needs its own ID: %q %q", IDOf(&a), IDOf(&b))
	}
}

// The occurrence record is the holder's: a setter repoints the one header
// it is called on, another copy keeps the record it had, and clearing the
// last field drops the record (nil, no allocation kept).
func TestOccurrenceRecordIsTheHolders(t *testing.T) {
	v := NewInteger(1)
	v.SetDynFrom("x")
	SetID(&v, "id-1")
	kept := v
	v.SetDynFrom("")
	if v.DynFrom() != "" || kept.DynFrom() != "x" || IDOf(&v) != "id-1" {
		t.Fatalf("a clear must reach this holder only: %q %q %q", v.DynFrom(), kept.DynFrom(), IDOf(&v))
	}
	v.SetDynFrom("") // nothing to clear: a no-op
	SetID(&v, "")
	if v.occ != nil {
		t.Fatal("an empty record must not stay allocated")
	}
	if key := IdentityOnly("k"); IDOf(&key) != "k" || key.Parent != nil || key.Data != nil {
		t.Fatalf("IdentityOnly carries the identity and nothing else: %v", key)
	}
	if empty := IdentityOnly(""); !empty.IsUnboundSlot() {
		t.Fatal("IdentityOnly of no identity is the zero Value")
	}
}

// The header only shrinks (design/VALUE-NO-COPY.0.md §6 phase 3): every
// byte of it rides on each by-value copy the ratchet still allows, so a new
// field belongs in the occurrence record, the type-node metadata or a
// payload, not here.
func TestValueHeaderSize(t *testing.T) {
	if got := unsafe.Sizeof(Value{}); got > 72 {
		t.Fatalf("Value header is %d bytes, want at most 72", got)
	}
}

// A type literal names its node (decision 5): NodeOf answers the node for
// the node itself and for any copy of it, needs no registry, and leaves an
// ad-hoc *Type no table registered as it is.
func TestNodeOfNamesTheNodeFromAnyCopy(t *testing.T) {
	if NodeOf(nil) != nil {
		t.Fatal("NodeOf(nil) must be nil")
	}
	r := newTestRegistry(t)
	minted := r.Types.MintType("P3Node", TInteger)
	lit := NewTypeLiteral(minted)
	if NodeOf(minted) != minted || NodeOf(&lit) != minted || NodeOf(addrOf(NewTypeLiteral(TInteger))) != TInteger {
		t.Fatal("a node and every copy of it name the node")
	}
	bare := &Type{Parent: TInteger}
	adhoc := addrOf(withID(Type{tmeta: &typeMeta{Name: "P3AdHoc"}, Parent: TInteger}, "P3AdHoc"))
	if NodeOf(bare) != bare || NodeOf(adhoc) != adhoc {
		t.Fatal("an ad-hoc *Type no table registered stands for itself")
	}
	// Adopting an ad-hoc node registers it as its metadata's node; adopting
	// a copy of a node registers the node, never the copy.
	dyn := NewDynamicTypeTable()
	dyn.Adopt(adhoc)
	if NodeOf(addrOf(*adhoc)) != adhoc || dyn.LookupByID("P3AdHoc") != adhoc {
		t.Fatal("an adopted ad-hoc node becomes the node its copies name")
	}
	dyn.Adopt(&lit)
	if dyn.LookupByID(IDOf(minted)) != minted {
		t.Fatal("adopting a literal registers its node")
	}
}

// The helpers that hand out a type literal's *Type hand out its node; only
// an ad-hoc literal, which has none, still yields a copy of itself.
func TestLiteralTypeHelpersReturnTheNode(t *testing.T) {
	lit := NewTypeLiteral(TInteger)
	if ValueType(lit) != TInteger || TypeNodeOf(lit) != TInteger || denotedType(lit) != TInteger {
		t.Fatal("a registered literal's type is its node")
	}
	if c := CarrierOfLiteral(lit); c.Parent != TInteger || !c.Carrier {
		t.Fatalf("CarrierOfLiteral = %v, want a carrier of the node", c)
	}
	adhoc := withID(Type{tmeta: &typeMeta{Name: "P3Lit"}, Parent: TInteger}, "P3Lit")
	for name, got := range map[string]*Type{
		"ValueType": ValueType(adhoc), "TypeNodeOf": TypeNodeOf(adhoc),
		"denotedType": denotedType(adhoc), "CarrierOfLiteral": CarrierOfLiteral(adhoc).Parent,
	} {
		if got == nil || got.Name() != "P3Lit" || IDOf(got) != "P3Lit" {
			t.Errorf("%s of an ad-hoc literal = %v, want a copy of it", name, got)
		}
	}
	if carrier := NewCarrier(TString); TypeNodeOf(carrier) != TString {
		t.Error("a carrier's node is its Parent")
	}
	var sink *Type
	if n := testing.AllocsPerRun(100, func() { sink = ValueType(lit) }); n != 0 || sink != TInteger {
		t.Errorf("ValueType of a registered literal allocated %.0f times; its node costs nothing", n)
	}
}
