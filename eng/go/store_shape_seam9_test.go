package eng

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// W9 store_shape.go coverage: the abstract check-mode store-shape payload
// helpers. All pure — driven by direct calls with crafted shapes and values.

func TestW9StoreShapeRecordAndLookupKey(t *testing.T) {
	// nil receiver / empty key are no-ops.
	var nilShape *core.StoreShapeInfo
	nilShape.RecordKey("k", core.NewInteger(1)) // must not panic
	if _, ok := nilShape.LookupKey("k"); ok {
		t.Error("nil shape must not resolve a key")
	}

	s := &core.StoreShapeInfo{}
	s.RecordKey("", core.NewInteger(1))
	if s.KeyTypes != nil {
		t.Error("an empty key must not allocate KeyTypes")
	}

	// First write, then a rewrite that joins.
	s.RecordKey("a", core.NewCarrier(core.TInteger))
	if _, ok := s.LookupKey("a"); !ok {
		t.Error("recorded key must resolve")
	}
	s.RecordKey("a", core.NewCarrier(core.TString))
	joined, ok := s.LookupKey("a")
	if !ok || !joined.Carrier {
		t.Errorf("rewrite should join into a carrier, got %v", joined)
	}
}

func TestW9StoreShapeRecordVal(t *testing.T) {
	// nil receiver and poisoned shape are no-ops.
	var nilShape *core.StoreShapeInfo
	nilShape.RecordVal(core.NewInteger(1)) // must not panic
	poisoned := &core.StoreShapeInfo{ValsPoisoned: true}
	poisoned.RecordVal(core.NewInteger(1))

	// A dispatch-bearing value poisons the unkeyed join.
	s := &core.StoreShapeInfo{}
	s.RecordVal(core.NewFunction(core.FnDefInfo{}))
	if !s.ValsPoisoned {
		t.Error("a function write must poison the unkeyed join")
	}
	if _, ok := s.LookupVals(); ok {
		t.Error("a poisoned join must decline LookupVals")
	}

	// A nil-Parent value also poisons.
	s2 := &core.StoreShapeInfo{}
	s2.RecordVal(core.Value{})
	if !s2.ValsPoisoned {
		t.Error("a nil-Parent write must poison the unkeyed join")
	}

	// First value sets Vals; a second joins.
	s3 := &core.StoreShapeInfo{}
	if _, ok := s3.LookupVals(); ok {
		t.Error("an empty shape has no unkeyed value")
	}
	s3.RecordVal(core.NewCarrier(core.TInteger))
	got, ok := s3.LookupVals()
	if !ok || got.Parent == nil {
		t.Errorf("first unkeyed write should set Vals, got %v (%v)", got, ok)
	}
	s3.RecordVal(core.NewCarrier(core.TString))
	if _, ok := s3.LookupVals(); !ok {
		t.Error("a second unkeyed write should still resolve (joined)")
	}
}

func TestW9StoreShapeCloneNil(t *testing.T) {
	var nilShape *core.StoreShapeInfo
	if nilShape.CloneShape() != nil {
		t.Error("cloning a nil shape must yield nil")
	}
}

func TestW9ContextShapeNilGuards(t *testing.T) {
	var nilCheck *core.CheckState
	if v := nilCheck.ContextShape(nil, 0); !v.Carrier {
		t.Error("nil check should return a bare store carrier")
	}
	c := &core.CheckState{}
	if v := c.ContextShape(nil, 0); !v.Carrier {
		t.Error("nil store should return a bare store carrier")
	}
}

func TestW9AdoptShapeValueXml(t *testing.T) {
	// A concrete Xml value adopts as a bare FlexXml carrier.
	got := check.AdoptShapeValue(core.NewXmlElement("a", nil, nil), 0)
	if !got.Carrier || !got.Parent.ConformsTo(core.TFlexXml) {
		t.Errorf("an Xml field should adopt as a FlexXml carrier, got %v", got)
	}
}

func TestW9ShapeFieldRead(t *testing.T) {
	// A disjunct read keeps its payload as a dynamic carrier value.
	dj := check.ShapeFieldRead(core.NewDisjunct([]core.Value{core.NewString("x")}))
	if !core.IsDynamic(&dj) {
		t.Error("a disjunct read should be dynamic")
	}
	// A function-typed field keeps the dynamic(Any) hatch.
	fn := check.ShapeFieldRead(core.NewFunction(core.FnDefInfo{}))
	if !core.IsDynamic(&fn) {
		t.Error("a function field should read dynamic(Any)")
	}
	// A nil-Parent value (no ValueType) also keeps dynamic(Any).
	none := check.ShapeFieldRead(core.Value{})
	if !core.IsDynamic(&none) {
		t.Error("a typeless value should read dynamic(Any)")
	}
}
