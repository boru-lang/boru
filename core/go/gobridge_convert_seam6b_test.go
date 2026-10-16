package core

// Seam-6b cluster B5: unit tests exercising previously-unreached blocks
// in gobridge.go (ToNative / FromNative edges), convert_ideal.go (the
// IdealConverter walk and the per-family converter method stubs), and
// nodify.go (the Nodifier walk). Tests that touch the bytes-bridge
// package hooks save and restore them via t.Cleanup and do not run in
// parallel (design/TEST-SEAMS.10.md).

import (
	"math"
	"testing"
)

// --- gobridge.go -------------------------------------------------------------

func TestS6b5ToNativeEdges(t *testing.T) {
	// A zero Value (nil Parent) maps to Go nil.
	if got := ToNative(Value{}); got != nil {
		t.Errorf("ToNative(zero Value) = %v, want nil", got)
	}

	// Map-typed value with an unreadable payload: textual fallback.
	badMap := MakeValue(TMap, ListPayload{})
	if got := ToNative(badMap); got != "{}" {
		t.Errorf("ToNative(bad map payload) = %v, want {}", got)
	}

	// List-typed value with an unreadable payload: textual fallback.
	badList := MakeValue(TList, MapPayload{M: NewOrderedMap()})
	if got, ok := ToNative(badList).(string); !ok {
		t.Errorf("ToNative(bad list payload) = %v, want a string fallback", got)
	}

	// A Word conforms to none of the data families: stringified fallback.
	saveFrom, saveTo := bytesFromNativeHook, bytesToNativeHook
	t.Cleanup(func() { bytesFromNativeHook, bytesToNativeHook = saveFrom, saveTo })
	bytesFromNativeHook, bytesToNativeHook = nil, nil
	if got := ToNative(NewWord("s6b5w")); got != "word(s6b5w)" {
		t.Errorf("ToNative(word) = %v, want word(s6b5w)", got)
	}
}

func TestS6b5FromNativeEdges(t *testing.T) {
	// A Value passes through unchanged.
	v := NewInteger(9)
	if got := FromNative(v); !ValuesEqual(got, v) {
		t.Errorf("FromNative(Value) = %v, want pass-through", got)
	}

	// The fixed-width integer lifts.
	cases := []struct {
		in   any
		want int64
	}{
		{int32(5), 5},
		{uint(6), 6},
		{uint32(7), 7},
		{uint64(8), 8},
	}
	for _, c := range cases {
		n, err := AsInteger(FromNative(c.in))
		if err != nil || n != c.want {
			t.Errorf("FromNative(%T %v) = %v (%v), want %d", c.in, c.in, n, err, c.want)
		}
	}

	// float32 routes through floatToValue (integral → Integer).
	if n, err := AsInteger(FromNative(float32(3))); err != nil || n != 3 {
		t.Errorf("FromNative(float32 3) = %v (%v), want 3", n, err)
	}

	// []byte with no bytes bridge installed falls back to String.
	saveFrom, saveTo := bytesFromNativeHook, bytesToNativeHook
	t.Cleanup(func() { bytesFromNativeHook, bytesToNativeHook = saveFrom, saveTo })
	bytesFromNativeHook, bytesToNativeHook = nil, nil
	if s, err := AsString(FromNative([]byte("s6b5"))); err != nil || s != "s6b5" {
		t.Errorf("FromNative([]byte) = %q (%v), want s6b5", s, err)
	}
}

func TestS6b5FloatToValueNonFinite(t *testing.T) {
	got := floatToValue(math.NaN())
	if ParentOf(&got) == nil || !ParentOf(&got).Equal(TFloat) {
		t.Errorf("floatToValue(NaN) parent = %v, want Float", ParentOf(&got))
	}
	f, err := AsFloat(got)
	if err != nil || !math.IsNaN(f) {
		t.Errorf("floatToValue(NaN) = %v (%v), want NaN payload", f, err)
	}
}

// --- convert_ideal.go ---------------------------------------------------------

// s6b5DeclineConverter is a TypeBehavior that structurally satisfies
// IdealConverter but declines both conversions, so the parent-chain
// walk must continue past it to the Ideal root's fallback.
type s6b5DeclineConverter struct{}

func (s6b5DeclineConverter) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (s6b5DeclineConverter) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (s6b5DeclineConverter) Format(v Value) string       { return DefaultBehavior.Format(v) }
func (s6b5DeclineConverter) ToMap(Value) (Value, error)  { return Value{}, ErrNoConverter }
func (s6b5DeclineConverter) ToList(Value) (Value, error) { return Value{}, ErrNoConverter }

func TestS6b5ConvertIdealDecliningBehaviorWalks(t *testing.T) {
	child := &Type{tmeta: &typeMeta{Name: "S6b5Declineon", Behavior: s6b5DeclineConverter{}}, Parent: TIdeal}
	v := NewValueRaw(child, IntPayload{N: 1})

	m, err := ConvertIdealToMap(v)
	if err != nil {
		t.Fatalf("ConvertIdealToMap: %v", err)
	}
	rm, err := AsMap(m)
	if err != nil || rm.Len() != 0 {
		t.Errorf("declined ToMap should fall to the empty-map root, got %v (%v)", m, err)
	}

	l, err := ConvertIdealToList(v)
	if err != nil {
		t.Fatalf("ConvertIdealToList: %v", err)
	}
	rl, err := AsList(l)
	if err != nil || rl.Len() != 0 {
		t.Errorf("declined ToList should fall to the empty-list root, got %v (%v)", l, err)
	}
}

func TestS6b5IdealConvertBehaviorFallbacks(t *testing.T) {
	b := idealRootBehavior{}
	m, err := b.ToMap(NewInteger(1))
	if err != nil {
		t.Fatalf("ToMap: %v", err)
	}
	if rm, merr := AsMap(m); merr != nil || rm.Len() != 0 {
		t.Errorf("base ToMap = %v, want empty map", m)
	}
	l, err := b.ToList(NewInteger(1))
	if err != nil {
		t.Fatalf("ToList: %v", err)
	}
	if rl, lerr := AsList(l); lerr != nil || rl.Len() != 0 {
		t.Errorf("base ToList = %v, want empty list", l)
	}
}

func TestS6b5ConverterBehaviorMethodStubs(t *testing.T) {
	a, b := NewInteger(2), NewInteger(2)

	if !(objectConvertBehavior{}).Equal(a, b) {
		t.Error("objectConvertBehavior.Equal must delegate to the default")
	}
	if !(storeConvertBehavior{}).Equal(a, b) {
		t.Error("storeConvertBehavior.Equal must delegate to the default")
	}
	if got := (storeConvertBehavior{}).Format(NewInteger(3)); got != "3" {
		t.Errorf("storeConvertBehavior.Format = %q, want 3", got)
	}
	if !(reachConvertBehavior{}).Equal(a, b) {
		t.Error("reachConvertBehavior.Equal must delegate to the default")
	}
	if !(tableConvertBehavior{}).Match(NewInteger(1), TInteger) {
		t.Error("tableConvertBehavior.Match must delegate to the default")
	}
	if !(tableConvertBehavior{}).Equal(a, b) {
		t.Error("tableConvertBehavior.Equal must delegate to the default")
	}
}

func TestS6b5TableConvertToMapSkipsNonMapRow(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("c", NewTypeLiteral(TInteger))
	td := TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{NewInteger(1)}, // not a map: the column scan skips it
	}
	v := NewValueRaw(TList, td)
	m, err := (tableConvertBehavior{}).ToMap(v)
	if err != nil {
		t.Fatalf("ToMap: %v", err)
	}
	rm, err := AsMap(m)
	if err != nil {
		t.Fatalf("AsMap: %v", err)
	}
	col, ok := rm.Get("c")
	if !ok {
		t.Fatal("columnar map must still carry the schema column")
	}
	if rl, lerr := AsList(col); lerr != nil || rl.Len() != 0 {
		t.Errorf("non-map rows must contribute no values, got %v", col)
	}
}

// --- nodify.go ----------------------------------------------------------------

// s6b5DeclineNodifier holds the Nodifier slot but declines, so the
// NodifyValue walk continues past it.
type s6b5DeclineNodifier struct{}

func (s6b5DeclineNodifier) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (s6b5DeclineNodifier) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (s6b5DeclineNodifier) Format(v Value) string       { return DefaultBehavior.Format(v) }
func (s6b5DeclineNodifier) Nodify(Value) (Value, error) { return Value{}, ErrNoNodifier }

func TestS6b5NodifyValueEdges(t *testing.T) {
	// nil Parent: identity.
	out, err := NodifyValue(Value{})
	if err != nil || ParentOf(&out) != nil {
		t.Errorf("NodifyValue(zero) = %v (%v), want identity", out, err)
	}

	// A declining Nodifier on the chain: the walk continues and, with no
	// other Nodifier registered, returns the value unchanged.
	typ := &Type{tmeta: &typeMeta{Name: "S6b5Nodon", Behavior: s6b5DeclineNodifier{}}}
	v := NewValueRaw(typ, IntPayload{N: 4})
	out, err = NodifyValue(v)
	if err != nil {
		t.Fatalf("NodifyValue: %v", err)
	}
	if n, nerr := AsInteger(out); nerr != nil || n != 4 {
		t.Errorf("declined nodify must return the value unchanged, got %v", out)
	}
}
