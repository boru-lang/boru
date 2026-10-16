package core

// Seam-6b wave B0: guard arms in core_flex.go — FlexDeepCopy /
// NodeDeepCopy / containsFlex / AdoptIntoFlex / MakeNodeHandler on
// type-body payloads (record/table types share the Map/List families
// but carry no readable container) and nil-backed flex nodes.

import (
	"strings"
	"testing"
)

func s6b0RecordTypeVal() Value {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	return NewRecordType(fields)
}

func s6b0TableTypeVal() Value {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	return NewTableType(RecordTypeInfo{Fields: fields})
}

func s6b0AttrMap(k string, v Value) *OrderedMap {
	m := NewOrderedMap()
	m.Set(k, v)
	return m
}

// --- FlexDeepCopy ------------------------------------------------------------

func TestS6b0FlexDeepCopyTypeBodyErrors(t *testing.T) {
	// A record type body shares the Map family but has no readable
	// container payload.
	if _, err := FlexDeepCopy(s6b0RecordTypeVal()); err == nil ||
		!strings.Contains(err.Error(), "flex: cannot make a flex copy") {
		t.Errorf("record type body: %v", err)
	}
	// Same for the List family via a table type body.
	if _, err := FlexDeepCopy(s6b0TableTypeVal()); err == nil ||
		!strings.Contains(err.Error(), "flex: cannot make a flex copy") {
		t.Errorf("table type body: %v", err)
	}
}

func TestS6b0FlexDeepCopyNestedTypeBodyErrors(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", s6b0RecordTypeVal())
	if _, err := FlexDeepCopy(NewMap(m)); err == nil {
		t.Error("nested record type body in a map must fail the flex copy")
	}
	if _, err := FlexDeepCopy(NewList([]Value{s6b0RecordTypeVal()})); err == nil {
		t.Error("nested record type body in a list must fail the flex copy")
	}
}

func TestS6b0FlexDeepCopyXmlArms(t *testing.T) {
	// A concrete Xml-family value without element parts.
	bad := MakeValue(TXml, StrPayload{S: "x"})
	if _, err := FlexDeepCopy(bad); err == nil ||
		!strings.Contains(err.Error(), "flex: cannot make a flex copy") {
		t.Errorf("non-element xml payload: %v", err)
	}
	// Attribute value that cannot flex-copy.
	withAttr := NewXmlElement("a", s6b0AttrMap("k", s6b0RecordTypeVal()), nil)
	if _, err := FlexDeepCopy(withAttr); err == nil {
		t.Error("unflexable attribute value must fail")
	}
	// Child that cannot flex-copy.
	withChild := NewXmlElement("a", nil, []Value{s6b0RecordTypeVal()})
	if _, err := FlexDeepCopy(withChild); err == nil {
		t.Error("unflexable child must fail")
	}
}

// --- NodeDeepCopy ------------------------------------------------------------

func TestS6b0NodeDeepCopyNilBackedFlexList(t *testing.T) {
	if _, err := NodeDeepCopy(NewFlexList(nil)); err == nil ||
		!strings.Contains(err.Error(), "node: cannot convert") {
		t.Errorf("nil-backed flex list: %v", err)
	}
}

func TestS6b0NodeDeepCopyNestedFailures(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewFlexList(nil))
	if _, err := NodeDeepCopy(NewMap(m)); err == nil {
		t.Error("nil-backed flex list inside a map must fail node conversion")
	}
	if _, err := NodeDeepCopy(NewList([]Value{NewFlexList(nil)})); err == nil {
		t.Error("nil-backed flex list inside a list must fail node conversion")
	}
	withAttr := NewXmlElement("a", s6b0AttrMap("k", NewFlexList(nil)), nil)
	if _, err := NodeDeepCopy(withAttr); err == nil {
		t.Error("nil-backed flex list in an xml attribute must fail node conversion")
	}
	withChild := NewXmlElement("a", nil, []Value{NewFlexList(nil)})
	if _, err := NodeDeepCopy(withChild); err == nil {
		t.Error("nil-backed flex list in an xml child must fail node conversion")
	}
}

// --- containsFlex --------------------------------------------------------------

func TestS6b0ContainsFlexProbes(t *testing.T) {
	if containsFlex(NewCarrier(TList)) {
		t.Error("a carrier is not concrete and cannot contain flex")
	}
	m := NewOrderedMap()
	m.Set("a", NewFlexMap(NewOrderedMap()))
	if !containsFlex(NewMap(m)) {
		t.Error("map containing a flex node must report flex")
	}
	withAttr := NewXmlElement("a", s6b0AttrMap("k", NewFlexMap(NewOrderedMap())), nil)
	if !containsFlex(withAttr) {
		t.Error("xml attribute flex must be found")
	}
	withChild := NewXmlElement("a", nil, []Value{NewFlexMap(NewOrderedMap())})
	if !containsFlex(withChild) {
		t.Error("xml child flex must be found")
	}
	plain := NewXmlElement("a", s6b0AttrMap("k", NewString("v")), []Value{NewString("t")})
	if containsFlex(plain) {
		t.Error("fully immutable xml must not report flex")
	}
}

// --- AdoptIntoFlex ---------------------------------------------------------------

func TestS6b0AdoptIntoFlexUnreadableListPassesThrough(t *testing.T) {
	tt := s6b0TableTypeVal()
	out, err := AdoptIntoFlex(tt)
	if err != nil {
		t.Fatalf("AdoptIntoFlex: %v", err)
	}
	if DataOf(&out) != DataOf(&tt) {
		t.Error("a list-family type body must pass through unchanged")
	}
}

// --- MakeNodeHandler ---------------------------------------------------------------

func TestS6b0MakeNodeHandlerUnavailableKind(t *testing.T) {
	r := newTestRegistry(t)
	r.Ideals.Register(&Ideal{
		Name:    "S6b0NodeKind",
		Enabled: false,
		Accepts: func(v Value) bool { return IsBareTypeNode(&v) && v.Equal(TBoolean) },
	})
	_, err := MakeNodeHandler([]Value{NewTypeLiteral(TBoolean), NewInteger(1)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "S6b0NodeKind type-kind is not available") {
		t.Fatalf("expected unavailable-kind error, got %v", err)
	}
}

func TestS6b0MakeNodeHandlerConversionErrors(t *testing.T) {
	flexFail := func() Value {
		m := NewOrderedMap()
		m.Set("a", s6b0RecordTypeVal())
		return NewMap(m)
	}()
	nodeFailMap := func() Value {
		m := NewOrderedMap()
		m.Set("a", NewFlexList(nil))
		return NewMap(m)
	}()

	cases := []struct {
		name   string
		target *Type
		src    Value
	}{
		{"FlexMap flex error", TFlexMap, flexFail},
		{"FlexList flex error", TFlexList, NewList([]Value{s6b0RecordTypeVal()})},
		{"Map node error", TMap, nodeFailMap},
		{"List node error", TList, NewList([]Value{NewFlexList(nil)})},
		{"FlexXml flex error", TFlexXml, NewXmlElement("a", nil, []Value{s6b0RecordTypeVal()})},
		{"Xml node error", TXml, NewXmlElement("a", nil, []Value{NewFlexList(nil)})},
	}
	for _, tc := range cases {
		_, err := MakeNodeHandler([]Value{NewTypeLiteral(tc.target), tc.src}, nil, nil, nil)
		if err == nil {
			t.Errorf("%s: expected the conversion error to surface", tc.name)
		}
	}
}
