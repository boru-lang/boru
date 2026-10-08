package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// w8Store builds a concrete Store Ideal value with the given fields.
func w8Store(fields map[string]native.Value) native.Value {
	return native.NewStoreValue(native.TStore, &native.StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     fields,
	})
}

// TestW8QueryDocToAnyIdealMap drives docToAny's default-branch Ideal→map
// projection (minilang_query.go:58/60/64): a concrete Store projects to a
// non-empty map.
func TestW8QueryDocToAnyIdealMap(t *testing.T) {
	out := docToAny(w8Store(map[string]native.Value{"x": native.NewInteger(1)}))
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("docToAny(Store) = %#v, want map[string]any", out)
	}
	if _, has := m["x"]; !has {
		t.Errorf("docToAny(Store) = %#v, want an x field", m)
	}
}

// TestW8QueryIdealListToAnyError drives idealListToAny's ConvertIdealToList
// error fallback (minilang_query.go:75): a scalar is not list-convertible.
func TestW8QueryIdealListToAnyError(t *testing.T) {
	_ = idealListToAny(native.NewInteger(5)) // must not panic; falls back to ValueToAny
}

// TestW8FormatValueFallbacks drives the AsList / AsMap fallback arms of
// formatTableValue (report.go:130 AsList-error, 143 row AsMap-nil) and
// formatListValue (report.go:213 AsList-error) via extension-backed values
// whose payloads the list/map accessors decline.
func TestW8FormatValueFallbacks(t *testing.T) {
	// A TList value whose payload AsList cannot read (not a table either).
	extList := core.NewExtension(native.TList, "w8-not-a-list")

	// 130: formatTableValue's AsList error fallback.
	if s := formatTableValue(extList); s == "" {
		t.Error("formatTableValue(ext-list) should fall back to a printed form")
	}
	// 213: formatListValue's AsList error fallback.
	if s := formatListValue(extList); s == "" {
		t.Error("formatListValue(ext-list) should fall back to a printed form")
	}
	// 143: a plain list whose first row is a TMap-typed value AsMap declines.
	rowExt := core.NewExtension(native.TMap, "w8-not-a-map")
	if s := formatTableValue(native.NewList([]native.Value{rowExt})); s == "" {
		t.Error("formatTableValue(list of ext-map row) should fall back")
	}
}

// TestW8ProfileRowsSorted drives profileRows's comparator differing-count arm
// (debug.go:749): two words with distinct step counts sort by count desc.
func TestW8ProfileRowsSorted(t *testing.T) {
	out := profileRows(map[string]int{"a": 1, "b": 3})
	lst, err := native.AsList(out)
	if err != nil || lst.Len() != 2 {
		t.Fatalf("profileRows: got %v (%v)", out, err)
	}
	first, _ := native.AsMap(lst.Get(0))
	w, _ := first.Get("word")
	name, _ := w.AsConcreteString()
	if name != "b" {
		t.Errorf("profileRows first row = %q, want b (higher count first)", name)
	}
}
