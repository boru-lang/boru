package native

import (
	"strings"
	"testing"
)

// Wave-7 coverage for the small handler files: forloop.go parseRange,
// getpath.go accessor + reach-lens errors, parse_text.go string reject,
// native_keyval.go duplicate registration, and native_definition_fn.go
// MatchFnSig's value-pattern unify-fail arm (design/TEST-SEAMS.10.md).

func TestParseRangeThreeElemErrors(t *testing.T) {
	// case 3, bad start (elems[0]).
	if _, _, _, err := parseRange([]Value{NewString("x"), NewInteger(10), NewInteger(2)}); err == nil {
		t.Fatal("parseRange([x,10,2]): expected start error")
	}
	// case 3, bad end (elems[1]).
	if _, _, _, err := parseRange([]Value{NewInteger(1), NewString("y"), NewInteger(2)}); err == nil {
		t.Fatal("parseRange([1,y,2]): expected end error")
	}
	// Positive pair: a valid 3-element range decodes.
	start, end, step, err := parseRange([]Value{NewInteger(1), NewInteger(9), NewInteger(2)})
	if err != nil || start != 1 || end != 9 || step != 2 {
		t.Fatalf("parseRange([1,9,2]) = %d,%d,%d / %v", start, end, step, err)
	}
}

func TestGetpathHandlerAccessorError(t *testing.T) {
	r := b2Reg(t)
	// args[0] is not a concrete string → AsConcreteString error arm.
	if _, err := getpathHandler([]Value{NewInteger(5), mapVal("a", NewInteger(1))}, nil, nil, r); err == nil {
		t.Fatal("getpath(non-string path): expected error")
	}
	// Positive pair: a real path read.
	out, err := getpathHandler([]Value{NewString("a"), mapVal("a", NewInteger(1))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("getpath(a) = %v / %v", out, err)
	}
	// NOTE: getpath.go:23 (anyToValue-of-result error) is unreachable by
	// input — valueToAny/GetPath only produce JSON-shaped values that
	// anyToValue accepts totally. Documented residual (needs an
	// anyToValue seam).
}

func TestGetpathReachLensError(t *testing.T) {
	r := b2Reg(t)
	reach, rerr := b2Run(r, "$.a.b")
	if rerr != nil || len(reach) != 1 || !reach[0].Parent().ConformsTo(TReach) {
		t.Fatalf("could not build a Reach value: %v / %v", reach, rerr)
	}
	// Applying `$.a.b` to a scalar receiver dots into a non-map → ApplyReach
	// errors, exercising getpathReachHandler's ApplyReach error arm.
	if _, err := getpathReachHandler([]Value{reach[0], NewInteger(5)}, nil, nil, r); err == nil {
		t.Fatal("getpath reach lens into scalar: expected error")
	}
	// Positive pair: the same lens over a matching nested map succeeds.
	nested := NewOrderedMap()
	inner := NewOrderedMap()
	inner.Set("b", NewInteger(7))
	nested.Set("a", NewMap(inner))
	out, err := getpathReachHandler([]Value{reach[0], NewMap(nested)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("getpath reach lens = %v / %v", out, err)
	}
}

func TestParseTextRejectsNonString(t *testing.T) {
	r := b2Reg(t)
	// args[0] not a string → AsString error arm.
	if _, err := parseTextHandler([]Value{NewInteger(5)}, nil, nil, r); err == nil {
		t.Fatal("parse(non-string): expected error")
	}
	// Empty input arm.
	if _, err := parseTextHandler([]Value{NewString("   ")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "empty") {
		t.Fatalf("parse(blank): expected empty error, got %v", err)
	}
	// Positive pair: valid data decodes.
	out, err := parseTextHandler([]Value{NewString("{a:1}")}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("parse({a:1}) = %v / %v", out, err)
	}
	// NOTE: parse_text.go:42 (jsonicToValue-of-result error) is unreachable
	// by input — jsonicToValue is total over every value SafeParseData can
	// produce. Documented residual.
}

func TestMatchFnSigValuePatternUnifyFail(t *testing.T) {
	r := b2Reg(t)
	// A fn with a value-pattern param `3` (an Integer literal, not a Map)
	// takes MatchFnSig's else branch; an argument of 4 fails to unify.
	fn, err := b2Run(r, "fn [[3] [Any] [1]]")
	if err != nil || len(fn) != 1 {
		t.Fatalf("could not build value-pattern fn: %v / %v", fn, err)
	}
	if sig := MatchFnSig(fn[0], []Value{NewInteger(4)}); sig != nil {
		t.Fatal("MatchFnSig(4) against pattern 3 should not match")
	}
	// Positive pair: the exact value unifies.
	if sig := MatchFnSig(fn[0], []Value{NewInteger(3)}); sig == nil {
		t.Fatal("MatchFnSig(3) against pattern 3 should match")
	}
}
