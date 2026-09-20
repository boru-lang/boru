package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// Wave-4 coverage for modules/binary.go: the per-argument rejection arm of
// every BinUtil word (the intArg / AsConcreteString guards), the bit-index
// and bit-range range_error arms, the extract / insert width edge cases,
// and the ord / chr / decode rejections — each paired with its accepted
// twin.

// w4BinValid maps a binary-module arg type to a valid value.
func w4BinValid(tp *native.Type) native.Value {
	if tp == native.TString {
		return native.NewString("hi")
	}
	return native.NewInteger(1)
}

// TestBinaryWave4MismatchSweep pins the per-argument rejection arms of
// every BinUtil word: a wrong-typed value at any position is a loud error,
// never a panic.
func TestBinaryWave4MismatchSweep(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	w4MismatchSweep(t, r, binaryModuleNatives, w4BinValid)
}

// w4Bin invokes a BinUtil word's handler with Integer args and returns the
// single Integer result.
func w4Bin(t *testing.T, r *native.Registry, name string, args ...int64) (int64, error) {
	t.Helper()
	n := findNativeWave3(t, binaryModuleNatives, name)
	vals := make([]native.Value, len(args))
	for i, a := range args {
		vals[i] = native.NewInteger(a)
	}
	out, err := w4Handler(t, n, 0, r, vals...)
	if err != nil {
		return 0, err
	}
	got, gerr := out[0].AsConcreteInteger()
	if gerr != nil {
		t.Fatalf("%s returned non-Integer %v", name, out[0])
	}
	return got, nil
}

// TestBinaryWave4BitIndexRange pins the [0, 64) bit-index guard of test /
// set / clear / toggle — in-range accepted, 64 and negative rejected.
func TestBinaryWave4BitIndexRange(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Accepted twins.
	if got, err := w4Bin(t, r, "set", 1, 4); err != nil || got != 6 {
		t.Errorf("set bit 1 of 4 = %d (%v), want 6", got, err)
	}
	if got, err := w4Bin(t, r, "clear", 2, 6); err != nil || got != 2 {
		t.Errorf("clear bit 2 of 6 = %d (%v), want 2", got, err)
	}
	if got, err := w4Bin(t, r, "toggle", 0, 6); err != nil || got != 7 {
		t.Errorf("toggle bit 0 of 6 = %d (%v), want 7", got, err)
	}
	n := findNativeWave3(t, binaryModuleNatives, "test")
	out, terr := w4Handler(t, n, 0, r, native.NewInteger(1), native.NewInteger(6))
	if terr != nil {
		t.Fatalf("test bit 1 of 6: %v", terr)
	}
	if b, _ := out[0].AsConcreteBoolean(); !b {
		t.Error("test bit 1 of 6 = false, want true")
	}
	// Rejected twins: out-of-range index in both directions.
	for _, name := range []string{"test", "set", "clear", "toggle"} {
		for _, idx := range []int64{-1, 64} {
			if _, err := w4Bin(t, r, name, idx, 1); err == nil ||
				!strings.Contains(err.Error(), "range_error") {
				t.Errorf("%s index %d: %v, want range_error", name, idx, err)
			}
		}
	}
}

// TestBinaryWave4ExtractInsert pins extract / insert across the width
// edges: an ordinary field, the zero-width field, the full 64-bit field,
// and the invalid-range rejections.
func TestBinaryWave4ExtractInsert(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// extract args: lo, hi, value.
	if got, err := w4Bin(t, r, "extract", 4, 8, 0xF0); err != nil || got != 0xF {
		t.Errorf("extract [4,8) of 0xF0 = %d (%v), want 15", got, err)
	}
	if got, err := w4Bin(t, r, "extract", 3, 3, 0xF0); err != nil || got != 0 {
		t.Errorf("extract zero-width = %d (%v), want 0", got, err)
	}
	if got, err := w4Bin(t, r, "extract", 0, 64, -1); err != nil || got != -1 {
		t.Errorf("extract full width of -1 = %d (%v), want -1", got, err)
	}
	for _, bad := range [][3]int64{{-1, 4, 0}, {0, 65, 0}, {5, 4, 0}} {
		if _, err := w4Bin(t, r, "extract", bad[0], bad[1], bad[2]); err == nil ||
			!strings.Contains(err.Error(), "range_error") {
			t.Errorf("extract %v: %v, want range_error", bad, err)
		}
	}

	// insert args: lo, hi, bits, value.
	if got, err := w4Bin(t, r, "insert", 4, 8, 0xA, 0); err != nil || got != 0xA0 {
		t.Errorf("insert 0xA at [4,8) = %d (%v), want 160", got, err)
	}
	if got, err := w4Bin(t, r, "insert", 3, 3, 0xF, 7); err != nil || got != 7 {
		t.Errorf("insert zero-width = %d (%v), want the value unchanged", got, err)
	}
	if got, err := w4Bin(t, r, "insert", 0, 64, -1, 0); err != nil || got != -1 {
		t.Errorf("insert full width = %d (%v), want -1", got, err)
	}
	for _, bad := range [][4]int64{{-1, 4, 0, 0}, {0, 65, 0, 0}, {5, 4, 0, 0}} {
		if _, err := w4Bin(t, r, "insert", bad[0], bad[1], bad[2], bad[3]); err == nil ||
			!strings.Contains(err.Error(), "range_error") {
			t.Errorf("insert %v: %v, want range_error", bad, err)
		}
	}
}

// TestBinaryWave4Rotates pins rotl / rotr (count, value) including a
// negative count (rotates the other way).
func TestBinaryWave4Rotates(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := w4Bin(t, r, "rotl", 4, 1); err != nil || got != 16 {
		t.Errorf("rotl 4 of 1 = %d (%v), want 16", got, err)
	}
	if got, err := w4Bin(t, r, "rotr", 4, 16); err != nil || got != 1 {
		t.Errorf("rotr 4 of 16 = %d (%v), want 1", got, err)
	}
	if got, err := w4Bin(t, r, "rotl", -4, 16); err != nil || got != 1 {
		t.Errorf("rotl -4 of 16 = %d (%v), want 1", got, err)
	}
}

// TestBinaryWave4MaskEdges pins mask's clamp arms: non-positive → 0,
// 64-or-more → -1, ordinary widths in between.
func TestBinaryWave4MaskEdges(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ n, want int64 }{{-3, 0}, {0, 0}, {8, 255}, {64, -1}, {99, -1}}
	for _, c := range cases {
		if got, err := w4Bin(t, r, "mask", c.n); err != nil || got != c.want {
			t.Errorf("mask %d = %d (%v), want %d", c.n, got, err, c.want)
		}
	}
}

// TestBinaryWave4OrdChr pins ord / chr and their rejections: the empty
// string has no codepoint, and out-of-range codepoints are declined.
func TestBinaryWave4OrdChr(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ord := findNativeWave3(t, binaryModuleNatives, "ord")
	out, oerr := w4Handler(t, ord, 0, r, native.NewString("Ax"))
	if oerr != nil {
		t.Fatalf("ord: %v", oerr)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 65 {
		t.Errorf("ord 'Ax' = %v, want 65", out[0])
	}
	if _, err := w4Handler(t, ord, 0, r, native.NewString("")); err == nil ||
		!strings.Contains(err.Error(), "range_error") {
		t.Errorf("ord '': %v, want range_error", err)
	}

	chr := findNativeWave3(t, binaryModuleNatives, "chr")
	out, cerr := w4Handler(t, chr, 0, r, native.NewInteger(65))
	if cerr != nil {
		t.Fatalf("chr: %v", cerr)
	}
	if s, _ := out[0].AsConcreteString(); s != "A" {
		t.Errorf("chr 65 = %v, want A", out[0])
	}
	for _, bad := range []int64{-1, 0x110000} {
		if _, err := w4Handler(t, chr, 0, r, native.NewInteger(bad)); err == nil ||
			!strings.Contains(err.Error(), "range_error") {
			t.Errorf("chr %d: %v, want range_error", bad, err)
		}
	}
}

// TestBinaryWave4Encodings pins the base64 / hex round trips and the
// malformed-input decode_error arms.
func TestBinaryWave4Encodings(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	str := func(name, in string) (string, error) {
		n := findNativeWave3(t, binaryModuleNatives, name)
		out, herr := w4Handler(t, n, 0, r, native.NewString(in))
		if herr != nil {
			return "", herr
		}
		return out[0].AsConcreteString()
	}
	if got, err := str("base64-encode", "hi"); err != nil || got != "aGk=" {
		t.Errorf("base64-encode hi = %q (%v)", got, err)
	}
	if got, err := str("base64-decode", "aGk="); err != nil || got != "hi" {
		t.Errorf("base64-decode = %q (%v)", got, err)
	}
	if _, err := str("base64-decode", "!!!"); err == nil ||
		!strings.Contains(err.Error(), "decode_error") {
		t.Errorf("base64-decode garbage: %v, want decode_error", err)
	}
	if got, err := str("hex-encode", "hi"); err != nil || got != "6869" {
		t.Errorf("hex-encode hi = %q (%v)", got, err)
	}
	if got, err := str("hex-decode", "6869"); err != nil || got != "hi" {
		t.Errorf("hex-decode = %q (%v)", got, err)
	}
	if _, err := str("hex-decode", "zz"); err == nil ||
		!strings.Contains(err.Error(), "decode_error") {
		t.Errorf("hex-decode garbage: %v, want decode_error", err)
	}
}

// TestBinaryWave4Unary pins the unary counting words (paired against the
// sweep's rejections): popcount / clz / ctz / bitlen / parity / reverse /
// swap keep their documented values.
func TestBinaryWave4Unary(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := w4Bin(t, r, "popcount", 7); err != nil || got != 3 {
		t.Errorf("popcount 7 = %d (%v), want 3", got, err)
	}
	if got, err := w4Bin(t, r, "clz", 1); err != nil || got != 63 {
		t.Errorf("clz 1 = %d (%v), want 63", got, err)
	}
	if got, err := w4Bin(t, r, "ctz", 8); err != nil || got != 3 {
		t.Errorf("ctz 8 = %d (%v), want 3", got, err)
	}
	if got, err := w4Bin(t, r, "bitlen", 255); err != nil || got != 8 {
		t.Errorf("bitlen 255 = %d (%v), want 8", got, err)
	}
	n := findNativeWave3(t, binaryModuleNatives, "parity")
	out, perr := w4Handler(t, n, 0, r, native.NewInteger(7))
	if perr != nil {
		t.Fatalf("parity: %v", perr)
	}
	if b, _ := out[0].AsConcreteBoolean(); !b {
		t.Error("parity 7 = false, want true (three bits)")
	}
	if got, err := w4Bin(t, r, "reverse", 1); err != nil || got != -9223372036854775808 {
		t.Errorf("reverse 1 = %d (%v), want min-int64", got, err)
	}
	if got, err := w4Bin(t, r, "swap", 1); err != nil || got != 72057594037927936 {
		t.Errorf("swap 1 = %d (%v), want 1<<56", got, err)
	}
}
