package core

// Stage-5 coverage tests for clone.go: the payload arms CloneValue had no
// unit coverage for (nil-Data literal, Map, FlexList, Store, class
// instance, Table, Error, ExtensionPayload) plus the cloner's identity-map
// (seen) arms and the nil guards of cloneSlice / cloneOrderedMap /
// cloneStore. Every value is built through the kernel constructors so the
// sealed-payload contract holds.

import (
	"errors"
	"testing"
)

func TestStage5CloneTypeLiteralSharesValue(t *testing.T) {
	lit := NewTypeLiteral(TInteger)
	cl := CloneValue(lit)
	if !IsBareTypeNode(&cl) || !ValueType(cl).Equal(TInteger) {
		t.Errorf("clone of a type literal = %v, want the literal back", cl)
	}
	if IDOf(&cl) != IDOf(&lit) {
		t.Error("a payload-free literal is shared, not re-minted")
	}
}

func TestStage5CloneMapDeepAndMeta(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	om.Implicit = true
	om.Meta = map[string]any{"k": "v"}
	m := NewMap(om)

	cl := CloneValue(m)
	cm, err := AsMutableMap(cl)
	if err != nil {
		t.Fatalf("clone is not a map: %v", err)
	}
	if cm == om {
		t.Fatal("clone must duplicate the OrderedMap")
	}
	if !cm.Implicit {
		t.Error("Implicit flag must survive the clone")
	}
	if got, ok := cm.Meta["k"]; !ok || got != "v" {
		t.Errorf("Meta not copied: %v", cm.Meta)
	}
	if v, ok := cm.Get("a"); !ok {
		t.Fatal("clone lost key a")
	} else if n, err := AsInteger(v); err != nil || n != 1 {
		t.Errorf("clone a = %v (%v), want 1", v, err)
	}
	cm.Set("b", NewInteger(2))
	if _, ok := om.Get("b"); ok {
		t.Error("writing the clone must not touch the original")
	}
	if IDOf(&cl) == IDOf(&m) {
		t.Error("a container clone gets a fresh ID")
	}
}

func TestStage5CloneSharedOrderedMapClonesOnce(t *testing.T) {
	shared := NewOrderedMap()
	shared.Set("x", NewInteger(1))
	lst := NewList([]Value{NewMap(shared), NewMap(shared)})

	cl := CloneValue(lst)
	elems, err := AsMutableList(cl)
	if err != nil || len(elems) != 2 {
		t.Fatalf("clone list = %v (%v)", cl, err)
	}
	m0, _ := AsMutableMap(elems[0])
	m1, _ := AsMutableMap(elems[1])
	if m0 == nil || m0 != m1 {
		t.Error("a map shared by two branches must clone to ONE shared map")
	}
	if m0 == shared {
		t.Error("the shared clone must still be a copy of the original")
	}
}

func TestStage5CloneFlexListSharedAndNil(t *testing.T) {
	fl := NewFlexList([]Value{NewInteger(1)})
	pair := NewList([]Value{fl, fl})

	cl := CloneValue(pair)
	elems, err := AsMutableList(cl)
	if err != nil || len(elems) != 2 {
		t.Fatalf("clone list = %v (%v)", cl, err)
	}
	fd0, err0 := AsFlexList(elems[0])
	fd1, err1 := AsFlexList(elems[1])
	if err0 != nil || err1 != nil {
		t.Fatalf("clone elems are not flex lists: %v / %v", err0, err1)
	}
	if fd0 != fd1 {
		t.Error("a flex list appearing twice must clone to ONE shared store")
	}
	orig, _ := AsFlexList(fl)
	if fd0 == orig {
		t.Error("the clone must not alias the original flex store")
	}
	if n, err := AsInteger(fd0.Elems[0]); err != nil || n != 1 {
		t.Errorf("clone elem = %v (%v), want 1", fd0.Elems[0], err)
	}

	// A flex list with a nil element slice exercises cloneSlice's nil arm.
	nl := CloneValue(NewFlexList(nil))
	nd, err := AsFlexList(nl)
	if err != nil {
		t.Fatalf("nil-elems flex clone: %v", err)
	}
	if nd.Elems != nil {
		t.Error("cloneSlice(nil) must stay nil")
	}
}

func TestStage5CloneStoreGraph(t *testing.T) {
	proto := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     map[string]Value{"base": NewString("b")},
	}
	child := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     map[string]Value{"inner": NewInteger(42)},
	}
	s1 := &StoreInstanceInfo{
		TypeName:  "Object/Store",
		Data:      map[string]Value{"x": NewInteger(1), "child": NewStoreValue(nil, child)},
		Deleted:   map[string]bool{"gone": true},
		Prototype: proto,
	}
	s2 := &StoreInstanceInfo{
		TypeName:  "Object/Store",
		Data:      map[string]Value{"y": NewInteger(2)},
		Prototype: proto,
	}

	om := NewOrderedMap()
	om.Set("s1", NewStoreValue(nil, s1))
	om.Set("s2", NewStoreValue(nil, s2))

	cl := CloneValue(NewMap(om))
	cm, err := AsMutableMap(cl)
	if err != nil {
		t.Fatalf("clone is not a map: %v", err)
	}
	v1, _ := cm.Get("s1")
	v2, _ := cm.Get("s2")
	c1, ok1 := DataOf(&v1).(*StoreInstanceInfo)
	c2, ok2 := DataOf(&v2).(*StoreInstanceInfo)
	if !ok1 || !ok2 {
		t.Fatal("clone lost the store payloads")
	}
	if c1 == s1 || c2 == s2 {
		t.Fatal("stores must be duplicated, not shared")
	}
	// Shared prototype clones ONCE (identity map).
	if c1.Prototype == nil || c1.Prototype != c2.Prototype {
		t.Error("the shared prototype must clone to one shared node")
	}
	if c1.Prototype == proto {
		t.Error("the cloned prototype must not alias the original")
	}
	// The prototype chain terminates: cloneStore(nil) = nil.
	if c1.Prototype.Prototype != nil {
		t.Error("chain end must stay nil")
	}
	// Tombstones travel with the layer.
	if !c1.Deleted["gone"] {
		t.Error("tombstones must be copied")
	}
	c1.Deleted["extra"] = true
	if s1.Deleted["extra"] {
		t.Error("clone tombstone map must be independent")
	}
	// The nested store's COW back-link is re-established onto the CLONE.
	cc, ok := DataOf(addrOf(c1.Data["child"])).(*StoreInstanceInfo)
	if !ok {
		t.Fatal("nested store lost in the clone")
	}
	if cc == child {
		t.Error("nested store must be duplicated")
	}
	if cc.Parent != c1 || cc.ParentKey != "child" {
		t.Errorf("nested back-link = (%p, %q), want the cloned parent", cc.Parent, cc.ParentKey)
	}
	if n, err := AsInteger(c1.Data["x"]); err != nil || n != 1 {
		t.Errorf("clone x = %v (%v), want 1", c1.Data["x"], err)
	}
	// The clone is fully detached from the original: writes don't leak.
	c1.Data["x"] = NewInteger(99)
	if n, _ := AsInteger(s1.Data["x"]); n != 1 {
		t.Error("clone writes must not reach the original store")
	}
}

func TestStage5CloneClassInstance(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("f", NewInteger(1))
	ct := &ClassTypeInfo{Name: "Ideal/Stage5K"}
	iv := NewClassInstance(TClass, ClassInstanceInfo{TypeRef: ct, Fields: fields})

	cl := CloneValue(iv)
	ci, ok := DataOf(&cl).(ClassInstanceInfo)
	if !ok {
		t.Fatalf("clone payload = %T, want ClassInstanceInfo", DataOf(&cl))
	}
	if ci.TypeRef != ct {
		t.Error("TypeRef is a shared immutable descriptor")
	}
	if ci.Fields == fields {
		t.Fatal("instance fields must be duplicated")
	}
	if v, ok := ci.Fields.Get("f"); !ok {
		t.Fatal("clone lost field f")
	} else if n, err := AsInteger(v); err != nil || n != 1 {
		t.Errorf("clone f = %v (%v), want 1", v, err)
	}
}

func TestStage5CloneTable(t *testing.T) {
	tv := NewValueRaw(TTable, TableData{
		Record:    RecordTypeInfo{Fields: NewOrderedMap()},
		Rows:      []Value{NewInteger(1)},
		TableName: "stage5t",
	})
	cl := CloneValue(tv)
	td, ok := DataOf(&cl).(TableData)
	if !ok {
		t.Fatalf("clone payload = %T, want TableData", DataOf(&cl))
	}
	if td.TableName != "stage5t" {
		t.Errorf("TableName = %q", td.TableName)
	}
	if len(td.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(td.Rows))
	}
	if n, err := AsInteger(td.Rows[0]); err != nil || n != 1 {
		t.Errorf("row 0 = %v (%v), want 1", td.Rows[0], err)
	}
	if IDOf(&cl) == IDOf(&tv) {
		t.Error("a table clone gets a fresh ID")
	}
}

func TestStage5CloneError(t *testing.T) {
	// A raise-style error with a payload map: the map is deep-cloned.
	data := NewOrderedMap()
	data.Set("hint", NewString("h"))
	ev := NewValueRaw(TError, ErrorInfo{Message: "m", Code: "user_error", Data: data})

	cl := CloneValue(ev)
	ei, err := AsError(cl)
	if err != nil {
		t.Fatalf("clone is not an error: %v", err)
	}
	if ei.Message != "m" || ei.Code != "user_error" {
		t.Errorf("message/code = %q/%q", ei.Message, ei.Code)
	}
	if ei.Data == data {
		t.Fatal("raise payload map must be duplicated")
	}
	if v, ok := ei.Data.Get("hint"); !ok {
		t.Fatal("clone lost the payload key")
	} else if s, err := AsString(v); err != nil || s != "h" {
		t.Errorf("hint = %v (%v)", v, err)
	}

	// A plain Go error has no payload map — the nil arm.
	plain := CloneValue(NewError(errors.New("boom")))
	pi, err := AsError(plain)
	if err != nil {
		t.Fatalf("plain clone: %v", err)
	}
	if pi.Data != nil {
		t.Error("nil payload must stay nil")
	}
}

// stage5CloneBody implements DeepCloner for the ExtensionPayload arm.
type stage5CloneBody struct{ N int }

func (b *stage5CloneBody) DeepClone() any { return &stage5CloneBody{N: b.N} }

// stage5OpaqueBody has no DeepCloner: the clone shares it.
type stage5OpaqueBody struct{ N int }

func TestStage5CloneExtensionPayload(t *testing.T) {
	body := &stage5CloneBody{N: 7}
	v := NewValueRaw(TAny, ExtensionPayload{Body: body})
	cl := CloneValue(v)
	ep, ok := DataOf(&cl).(ExtensionPayload)
	if !ok {
		t.Fatalf("clone payload = %T, want ExtensionPayload", DataOf(&cl))
	}
	nb, ok := ep.Body.(*stage5CloneBody)
	if !ok {
		t.Fatalf("clone body = %T", ep.Body)
	}
	if nb == body {
		t.Error("DeepCloner body must be duplicated")
	}
	if nb.N != 7 {
		t.Errorf("clone body N = %d, want 7", nb.N)
	}

	// No DeepCloner: the whole value is shared as-is.
	ov := NewValueRaw(TAny, ExtensionPayload{Body: stage5OpaqueBody{N: 3}})
	ocl := CloneValue(ov)
	if IDOf(&ocl) != IDOf(&ov) {
		t.Error("an opaque host body is shared, not re-minted")
	}
}

func TestStage5CloneOrderedMapNil(t *testing.T) {
	c := &cloner{seen: map[any]any{}}
	if c.cloneOrderedMap(nil) != nil {
		t.Error("cloneOrderedMap(nil) must be nil")
	}
}
