package eng

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// errTestW9 is a plain Go error (no BoruError Code / Data).
var errTestW9 = errors.New("plain boom")

// W9 clone.go coverage: the mutable-payload clone arms not reached by the
// existing seam8 object test — TableData, ErrorInfo (with/without Data),
// the ExtensionPayload no-DeepCloner share arm, and the nil / already-seen
// / Meta arms of cloneSlice and cloneOrderedMap.

func TestW9CloneTableData(t *testing.T) {
	rows := []core.Value{core.NewInteger(1), core.NewInteger(2)}
	tv := core.NewValueRaw(core.TTable, core.TableData{
		Record:    core.RecordTypeInfo{},
		Rows:      rows,
		TableName: "t",
	})
	cl := core.CloneValue(tv)
	td, ok := cl.Data().(core.TableData)
	if !ok {
		t.Fatalf("clone Data = %T, want TableData", cl.Data())
	}
	if td.TableName != "t" {
		t.Errorf("clone lost TableName: %q", td.TableName)
	}
	if len(td.Rows) != 2 {
		t.Fatalf("clone rows = %d, want 2", len(td.Rows))
	}
	// Rows must be duplicated, not aliased.
	td.Rows[0] = core.NewInteger(99)
	if n, _ := core.AsInteger(rows[0]); n != 1 {
		t.Error("clone leaked: mutating the clone's rows changed the original")
	}
}

func TestW9CloneErrorWithData(t *testing.T) {
	data := core.NewOrderedMap()
	data.Set("k", core.NewInteger(7))
	ae := &core.BoruError{Code: "user_error", Detail: "boom", Data: data}
	ev := core.NewError(ae)
	cl := core.CloneValue(ev)
	ci, ok := cl.Data().(core.ErrorInfo)
	if !ok {
		t.Fatalf("clone Data = %T, want ErrorInfo", cl.Data())
	}
	if ci.Code != "user_error" || ci.Message != "boom" {
		t.Errorf("clone lost immutable fields: %+v", ci)
	}
	if ci.Data == data {
		t.Error("cloneError must duplicate the Data map, not share it")
	}
	// Mutating the clone's Data must not touch the original.
	ci.Data.Set("k", core.NewInteger(0))
	if got, _ := data.Get("k"); func() int64 { n, _ := core.AsInteger(got); return n }() != 7 {
		t.Error("clone leaked: mutating the clone's Data changed the original")
	}
}

func TestW9CloneErrorNilData(t *testing.T) {
	// A plain Go error has nil Data: the clone keeps Data nil.
	ev := core.NewError(errTestW9)
	cl := core.CloneValue(ev)
	ci, ok := cl.Data().(core.ErrorInfo)
	if !ok {
		t.Fatalf("clone Data = %T, want ErrorInfo", cl.Data())
	}
	if ci.Data != nil {
		t.Error("cloneError with nil Data should keep Data nil")
	}
}

// extNoCloneW9 is a host body that does NOT implement DeepCloner.
type extNoCloneW9 struct{ n int }

func TestW9CloneExtensionNoDeepCloner(t *testing.T) {
	body := &extNoCloneW9{n: 5}
	ev := core.NewExtension(core.TAny, body)
	cl := core.CloneValue(ev)
	ep, ok := cl.Data().(core.ExtensionPayload)
	if !ok {
		t.Fatalf("clone Data = %T, want ExtensionPayload", cl.Data())
	}
	if ep.Body != any(body) {
		t.Error("an ExtensionPayload with no DeepCloner must be shared, not copied")
	}
}

func TestW9CloneNilSliceAndMap(t *testing.T) {
	// ListPayload with a nil slice: cloneSlice(nil) returns nil.
	lv := core.NewList(nil)
	cl := core.CloneValue(lv)
	lp, ok := cl.Data().(core.ListPayload)
	if !ok {
		t.Fatalf("clone Data = %T, want ListPayload", cl.Data())
	}
	if lp.Elems != nil {
		t.Error("cloneSlice(nil) should stay nil")
	}
	// MapPayload with a nil map: cloneOrderedMap(nil) returns nil.
	mv := core.NewMap(nil)
	clm := core.CloneValue(mv)
	mp, ok := clm.Data().(core.MapPayload)
	if !ok {
		t.Fatalf("clone Data = %T, want MapPayload", clm.Data())
	}
	if mp.M != nil {
		t.Error("cloneOrderedMap(nil) should stay nil")
	}
}

func TestW9CloneSharedMapDedup(t *testing.T) {
	// A single *OrderedMap referenced by two list elements: cloneOrderedMap
	// clones it once (the second call hits the seen map) so the clone keeps
	// the sharing.
	shared := core.NewOrderedMap()
	shared.Set("x", core.NewInteger(1))
	m1 := core.NewMap(shared)
	m2 := core.NewMap(shared)
	lv := core.NewList([]core.Value{m1, m2})
	cl := core.CloneValue(lv)
	lp := cl.Data().(core.ListPayload)
	c1 := lp.Elems[0].Data().(core.MapPayload).M
	c2 := lp.Elems[1].Data().(core.MapPayload).M
	if c1 != c2 {
		t.Error("shared map should clone once and stay shared in the clone")
	}
	if c1 == shared {
		t.Error("clone should duplicate the shared map, not alias the original")
	}
}

func TestW9CloneMapMeta(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("x", core.NewInteger(1))
	m.Meta = map[string]any{"src": "test"}
	cl := core.CloneValue(core.NewMap(m))
	cm := cl.Data().(core.MapPayload).M
	if cm.Meta == nil || cm.Meta["src"] != "test" {
		t.Errorf("clone lost Meta: %+v", cm.Meta)
	}
	// The Meta map must be duplicated, not aliased.
	cm.Meta["src"] = "changed"
	if m.Meta["src"] != "test" {
		t.Error("clone leaked: mutating the clone's Meta changed the original")
	}
}
