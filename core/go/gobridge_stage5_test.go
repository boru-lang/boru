package core

// Stage-5 coverage tests for gobridge.go — the ToNative / FromNative
// bridge hosts use to round-trip data between boru Values and plain Go
// values. The bytes-bridge hooks are package globals: the test that
// installs them saves and restores via t.Cleanup and does not run in
// parallel (matching gobridge_convert_seam6b_test.go).

import (
	"reflect"
	"testing"
)

func TestStage5ToNativeScalars(t *testing.T) {
	if got := ToNative(NewNone()); got != nil {
		t.Errorf("ToNative(none) = %v, want nil", got)
	}
	if got := ToNative(NewString("s5")); got != "s5" {
		t.Errorf("ToNative(string) = %v, want s5", got)
	}
	if got := ToNative(NewInteger(7)); got != int64(7) {
		t.Errorf("ToNative(integer) = %v, want int64 7", got)
	}
	if got := ToNative(NewFloat(2.5)); got != 2.5 {
		t.Errorf("ToNative(float) = %v, want 2.5", got)
	}
	if got := ToNative(NewBoolean(true)); got != true {
		t.Errorf("ToNative(boolean) = %v, want true", got)
	}
	if got := ToNative(NewAtom("s5atom")); got != "s5atom" {
		t.Errorf("ToNative(atom) = %v, want s5atom", got)
	}
}

func TestStage5ToNativeContainers(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	om.Set("b", NewString("x"))
	wantM := map[string]any{"a": int64(1), "b": "x"}
	if got := ToNative(NewMap(om)); !reflect.DeepEqual(got, wantM) {
		t.Errorf("ToNative(map) = %#v, want %#v", got, wantM)
	}

	lst := NewList([]Value{NewInteger(1), NewString("y")})
	wantL := []any{int64(1), "y"}
	if got := ToNative(lst); !reflect.DeepEqual(got, wantL) {
		t.Errorf("ToNative(list) = %#v, want %#v", got, wantL)
	}
}

func TestStage5BytesBridgeHooks(t *testing.T) {
	saveFrom, saveTo := bytesFromNativeHook, bytesToNativeHook
	t.Cleanup(func() { bytesFromNativeHook, bytesToNativeHook = saveFrom, saveTo })

	RegisterBytesBridge(
		func(b []byte) Value { return NewString("bridged:" + string(b)) },
		func(v Value) ([]byte, bool) { return []byte("hooked"), true },
	)

	// ToNative: a word conforms to no data family, so the bytes hook gets
	// its chance and the []byte answer is returned as-is.
	got := ToNative(NewWord("s5w"))
	b, ok := got.([]byte)
	if !ok || string(b) != "hooked" {
		t.Errorf("ToNative via bytes hook = %#v, want []byte(hooked)", got)
	}

	// FromNative: []byte routes through the installed lift.
	if s, err := AsString(FromNative([]byte("zz"))); err != nil || s != "bridged:zz" {
		t.Errorf("FromNative([]byte) via hook = %q (%v), want bridged:zz", s, err)
	}
}

func TestStage5FromNativeScalars(t *testing.T) {
	if v := FromNative(nil); v.Parent() == nil || !v.Parent().ConformsTo(TNone) {
		t.Errorf("FromNative(nil) = %v, want None", v)
	}
	if s, err := AsString(FromNative("hi")); err != nil || s != "hi" {
		t.Errorf("FromNative(string) = %q (%v), want hi", s, err)
	}
	if b, err := AsBoolean(FromNative(true)); err != nil || !b {
		t.Errorf("FromNative(bool) = %v (%v), want true", b, err)
	}
	if n, err := AsInteger(FromNative(int(3))); err != nil || n != 3 {
		t.Errorf("FromNative(int) = %v (%v), want 3", n, err)
	}
	if n, err := AsInteger(FromNative(int64(4))); err != nil || n != 4 {
		t.Errorf("FromNative(int64) = %v (%v), want 4", n, err)
	}

	// float64: integral promotes to Integer, fractional stays Float.
	if n, err := AsInteger(FromNative(float64(6))); err != nil || n != 6 {
		t.Errorf("FromNative(6.0) = %v (%v), want integer 6", n, err)
	}
	frac := FromNative(2.5)
	if frac.Parent() == nil || !frac.Parent().ConformsTo(TFloat) {
		t.Fatalf("FromNative(2.5) parent = %v, want Float", frac.Parent())
	}
	if f, err := AsFloat(frac); err != nil || f != 2.5 {
		t.Errorf("FromNative(2.5) = %v (%v), want 2.5", f, err)
	}

	// Unknown shapes fall back to the stringified form.
	if s, err := AsString(FromNative(int16(9))); err != nil || s != "9" {
		t.Errorf("FromNative(int16) = %q (%v), want fallback 9", s, err)
	}
}

func TestStage5FromNativeContainers(t *testing.T) {
	lv := FromNative([]any{int64(1), "a"})
	rl, err := AsList(lv)
	if err != nil || rl.Len() != 2 {
		t.Fatalf("FromNative([]any) = %v (%v), want a 2-list", lv, err)
	}
	if n, _ := AsInteger(rl.Get(0)); n != 1 {
		t.Errorf("list[0] = %v, want 1", rl.Get(0))
	}
	if s, _ := AsString(rl.Get(1)); s != "a" {
		t.Errorf("list[1] = %v, want a", rl.Get(1))
	}

	mv := FromNative(map[string]any{"k": int64(5)})
	rm, err := AsMap(mv)
	if err != nil || rm.Len() != 1 {
		t.Fatalf("FromNative(map) = %v (%v), want a 1-map", mv, err)
	}
	got, ok := rm.Get("k")
	if !ok {
		t.Fatal("FromNative(map) lost key k")
	}
	if n, _ := AsInteger(got); n != 5 {
		t.Errorf("map[k] = %v, want 5", got)
	}
}
