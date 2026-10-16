package native

import (
	"fmt"
	"testing"
)

// --- native_accessor.go: hasObjectHandler map-container branch ---

func TestW8HasObjectMapContainer(t *testing.T) {
	r := w8Reg(t)
	m := NewOrderedMap()
	m.Set("k", NewInteger(1))
	// container is a concrete (mutable) map -> the AsMutableMap branch.
	out, err := hasObjectHandler([]Value{NewString("k"), NewMap(m)}, nil, nil, r)
	if err != nil {
		t.Fatalf("hasObject: %v", err)
	}
	if b, _ := AsBoolean(out[0]); !b {
		t.Error("hasObject should report the present key as found")
	}
	// Absent key on the same map path.
	out, _ = hasObjectHandler([]Value{NewString("missing"), NewMap(m)}, nil, nil, r)
	if b, _ := AsBoolean(out[0]); b {
		t.Error("hasObject should report an absent key as not found")
	}
}

// --- setpath.go: non-reach path arg that fails AsString ---

func TestW8SetpathPathNotConcreteString(t *testing.T) {
	r := w8Reg(t)
	dm := NewOrderedMap()
	dm.Set("a", NewInteger(1))
	// A value that conforms to TString (so it is selected as the path arg)
	// but has nil Data, so AsString fails.
	pathArg := ValueOf(&Datum{Parent: TString})
	if _, err := setpathHandler([]Value{NewMap(dm), pathArg, NewInteger(9)}, nil, nil, r); err == nil {
		t.Fatal("setpath with a non-concrete String path must error")
	}
}

// --- native_storage.go: setFlexListHandler non-integer index ---

func TestW8SetFlexListNonIntIndex(t *testing.T) {
	r := w8Reg(t)
	out, err := w8RunOn(t, r, `flex [1 2 3]`)
	if err != nil || len(out) != 1 {
		t.Fatalf("build flexlist: err=%v len=%d", err, len(out))
	}
	flexVal := out[0]
	// args: index, newVal, container. A non-integer index errors.
	_, herr := setFlexListHandler([]Value{NewString("notint"), NewInteger(9), flexVal}, nil, nil, r)
	if herr == nil {
		t.Fatal("set on a FlexList with a non-integer index must error")
	}
}

// --- native_temporal_await.go: parallel branch result is a single Error ---

func TestW8RunParallelBranchSingleError(t *testing.T) {
	r := w8Reg(t)
	// A body whose sub-program leaves a single Error VALUE on the stack
	// (not a raised runtime error) marks the branch as errored.
	elem := NewList([]Value{NewError(fmt.Errorf("boom"))})
	res := runParallelBranch(r, elem)
	if !res.err {
		t.Error("a branch returning a single error value should be marked errored")
	}
}

// --- parse_text.go: jsonicToValue error after a successful parse ---

func TestW8ParseTextConversionError(t *testing.T) {
	r := w8Reg(t)
	// The source parses as a valid number token, but the integer is out of
	// int64 range, so jsonicToValue's number conversion errors.
	huge := "999999999999999999999999999999999999999"
	if _, err := parseTextHandler([]Value{NewString(huge)}, nil, nil, r); err == nil {
		t.Fatal("parse of an out-of-range integer must error at conversion")
	}
}

// --- setpath.go: a Reach-typed path arg whose Data is not a ReachInfo ---

func TestW8SetpathReachBadData(t *testing.T) {
	r := w8Reg(t)
	dm := NewOrderedMap()
	dm.Set("a", NewInteger(1))
	// IsReach gates on Parent==TReach; AsReach gates on the ReachInfo
	// payload. A TReach-typed value with no ReachInfo passes the first and
	// fails the second.
	badReach := ValueOf(&Datum{Parent: TReach})
	if _, err := setpathHandler([]Value{NewMap(dm), badReach, NewInteger(9)}, nil, nil, r); err == nil {
		t.Fatal("setpath with a malformed reach path must error")
	}
}

// --- native_control.go: doListReturnsFn resolves a word body ---

func TestW8DoListReturnsFnWordBody(t *testing.T) {
	r := w8Reg(t)
	if _, err := w8RunOn(t, r, `def foo [1 2]`); err != nil {
		t.Fatalf("def foo: %v", err)
	}
	// A bare-word body is resolved to its bound value (the IsWord + live
	// binding branch of the check-mode return shape).
	out := doListReturnsFn([]Value{NewWord("foo")}, r)
	if len(out) == 0 {
		t.Fatal("doListReturnsFn should return a residual shape")
	}
}

// --- native_map_iter.go: invokeBodyTop propagates a body error ---

func TestW8InvokeBodyTopError(t *testing.T) {
	r := w8Reg(t)
	// The body references an undefined word, so InvokeBody returns an error.
	body := NewList([]Value{NewWord("w8_undefined_word_xyz")})
	if _, _, err := invokeBodyTop(r, body, []Value{NewInteger(1)}); err == nil {
		t.Fatal("invokeBodyTop should propagate the body error")
	}
}
