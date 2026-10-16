package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A KeyVal is a Map subtype: it IS a KeyVal, it IS a Map, and its type leaf is
// "KeyVal".
func TestKeyValIsMapSubtype(t *testing.T) {
	e := NewKeyVal("a", NewInteger(1), 0, 3)
	if !e.Is(TKeyVal) {
		t.Errorf("NewKeyVal value is not a KeyVal")
	}
	if !e.Is(TMap) {
		t.Errorf("KeyVal is not a Map (it must be a Map subtype)")
	}
	if got := ParentOf(&e).Leaf(); got != "KeyVal" {
		t.Errorf("type leaf = %q, want KeyVal", got)
	}
}

// The four fields are readable by name with their declared types.
func TestKeyValFields(t *testing.T) {
	e := NewKeyVal("name", NewString("Ada"), 2, 5)
	m, err := AsMap(e)
	if err != nil {
		t.Fatalf("AsMap: %v", err)
	}
	for k, want := range map[string]string{"k": "name", "v": "Ada"} {
		v, ok := m.Get(k)
		if !ok {
			t.Errorf("missing field %q", k)
			continue
		}
		if got, _ := AsString(v); got != want {
			t.Errorf("field %q = %q, want %q", k, got, want)
		}
	}
	for k, want := range map[string]int64{"i": 2, "n": 5} {
		v, ok := m.Get(k)
		if !ok {
			t.Errorf("missing field %q", k)
			continue
		}
		if got, _ := AsInteger(v); got != want {
			t.Errorf("field %q = %d, want %d", k, got, want)
		}
	}
}

// A KeyVal renders as a map.
func TestKeyValRendersAsMap(t *testing.T) {
	got := NewKeyVal("a", NewInteger(1), 0, 3).String()
	want := "{k:'a' v:1 i:0 n:3}"
	if got != want {
		t.Errorf("KeyVal render = %q, want %q", got, want)
	}
}

// Matching is nominal: only a value the constructor tags is a KeyVal. A plain
// map carrying the same fields is NOT one, and a non-map is neither.
func TestKeyValNominal(t *testing.T) {
	om := NewOrderedMap()
	om.Set("k", NewString("a"))
	om.Set("v", NewInteger(1))
	om.Set("i", NewInteger(0))
	om.Set("n", NewInteger(3))
	if addrOf(NewMap(om)).Is(TKeyVal) {
		t.Errorf("a plain map with k/v/i/n fields must NOT be a KeyVal")
	}
	if addrOf(NewInteger(1)).Is(TKeyVal) {
		t.Errorf("an Integer must not be a KeyVal")
	}
}

// The type name is registered in the builtin leaf index, so source can resolve
// it — the name can be used in annotations and `is` checks (e.g.
// `[e:KeyVal] => …`, `e is KeyVal`). Verified end-to-end with `boru do 'KeyVal'`.
func TestKeyValNameResolves(t *testing.T) {
	got := core.Builtin.LookupBuiltinByName("KeyVal")
	if got == nil {
		t.Fatalf("type name %q is not registered for source resolution", "KeyVal")
	}
	if got.Leaf() != "KeyVal" {
		t.Errorf("KeyVal resolves to leaf %q, want KeyVal", got.Leaf())
	}
}

// No-panic guard: the constructor and the rendering/match paths must be
// defensive even with a type-literal value (Any) as the entry value.
func TestKeyValNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	e := NewKeyVal("", NewTypeLiteral(TMap), 0, 0)
	_ = e.String()
	_ = e.Is(TKeyVal)
	_ = e.Is(TMap)
}
