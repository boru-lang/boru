package native

import (
	"strings"
	"testing"
)

// arrayTestReg returns a registry with the boru:array module words
// registered as bare words, so these handler unit tests can exercise
// them directly even though shape/reshape/where/etc. are no longer part
// of the global default registry (they now live in the boru:array module).
// The core array words (iota, range, each, …) are present regardless.
func arrayTestReg() *Registry {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	for _, n := range ArrayModuleNatives {
		r.RegisterNativeFunc(n)
	}
	return r
}

// --- iota ---

func TestIota(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{NewWord("iota"), NewInteger(5)})
	list, _ := AsList(result[0])
	if list.Len() != 5 {
		t.Fatalf("iota 5: length = %d, want 5", list.Len())
	}
	for i := 0; i < 5; i++ {
		_as0, _ := AsInteger(list.Get(i))
		if _as0 != int64(i) {
			_as1, _ := AsInteger(list.Get(i))
			t.Errorf("iota 5[%d] = %d, want %d", i, _as1, i)
		}
	}
}

func TestIotaZero(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{NewWord("iota"), NewInteger(0)})
	list, _ := AsList(result[0])
	if list.Len() != 0 {
		t.Errorf("iota 0: length = %d, want 0", list.Len())
	}
}

// --- range ---

// assertIntList checks that result[0] is a list of the wanted integers.
func assertIntList(t *testing.T, label string, result []Value, want []int64) {
	t.Helper()
	list, err := AsList(result[0])
	if err != nil {
		t.Fatalf("%s: result is not a list: %v", label, err)
	}
	if list.Len() != len(want) {
		t.Fatalf("%s: length = %d, want %d", label, list.Len(), len(want))
	}
	for i, w := range want {
		got, _ := AsInteger(list.Get(i))
		if got != w {
			t.Errorf("%s[%d] = %d, want %d", label, i, got, w)
		}
	}
}

func TestRangeStartStop(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{NewWord("range"), NewInteger(2), NewInteger(6)})
	assertIntList(t, "range 2 6", result, []int64{2, 3, 4, 5})
}

// range 0 n 1 must equal iota n.
func TestRangeMatchesIota(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{NewWord("range"), NewInteger(0), NewInteger(5), NewInteger(1)})
	assertIntList(t, "range 0 5 1", result, []int64{0, 1, 2, 3, 4})
}

func TestRangeStep(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{NewWord("range"), NewInteger(0), NewInteger(10), NewInteger(3)})
	assertIntList(t, "range 0 10 3", result, []int64{0, 3, 6, 9})
}

func TestRangeNegativeStep(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{NewWord("range"), NewInteger(5), NewInteger(0), NewInteger(-1)})
	assertIntList(t, "range 5 0 -1", result, []int64{5, 4, 3, 2, 1})
}

// An empty range (start already past stop in the step direction) yields [].
func TestRangeEmpty(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{NewWord("range"), NewInteger(5), NewInteger(5)})
	assertIntList(t, "range 5 5", result, []int64{})

	result = runBoru(t, r, []Value{NewWord("range"), NewInteger(0), NewInteger(10), NewInteger(-1)})
	assertIntList(t, "range 0 10 -1", result, []int64{})
}

// A zero step is rejected rather than looping forever.
func TestRangeZeroStepErrors(t *testing.T) {
	r := arrayTestReg()
	if err := runBoruError(t, r, []Value{NewWord("range"), NewInteger(0), NewInteger(10), NewInteger(0)}); err == nil {
		t.Fatalf("range 0 10 0: expected error, got none")
	}
}

// --- shape ---

func TestShapeFlat(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
		NewWord("shape"),
	})
	list, _ := AsList(result[0])
	_as2, _ := AsInteger(list.Get(0))
	if list.Len() != 1 || _as2 != 3 {
		t.Errorf("shape [1,2,3] = %v, want [3]", result[0])
	}
}

func TestShapeNested(t *testing.T) {
	r := arrayTestReg()
	input := NewList([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewList([]Value{NewInteger(3), NewInteger(4)}),
		NewList([]Value{NewInteger(5), NewInteger(6)}),
	})
	result := runBoru(t, r, []Value{input, NewWord("shape")})
	list, _ := AsList(result[0])
	_as4, _ := AsInteger(list.Get(0))
	_as3, _ := AsInteger(list.Get(1))
	if list.Len() != 2 || _as4 != 3 || _as3 != 2 {
		t.Errorf("shape [[1,2],[3,4],[5,6]] = %v, want [3,2]", result[0])
	}
}

// --- rank ---

func TestRank(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewWord("rank"),
	})
	_as5, _ := AsInteger(result[0])
	if _as5 != 1 {
		_as6, _ := AsInteger(result[0])
		t.Errorf("rank [1,2] = %d, want 1", _as6)
	}

	input := NewList([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewList([]Value{NewInteger(3), NewInteger(4)}),
	})
	result = runBoru(t, r, []Value{input, NewWord("rank")})
	_as7, _ := AsInteger(result[0])
	if _as7 != 2 {
		_as8, _ := AsInteger(result[0])
		t.Errorf("rank [[1,2],[3,4]] = %d, want 2", _as8)
	}
}

// --- reshape ---

func TestReshape(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("reshape"),
		NewList([]Value{NewInteger(2), NewInteger(3)}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4), NewInteger(5), NewInteger(6)}),
	})
	outer, _ := AsList(result[0])
	if outer.Len() != 2 {
		t.Fatalf("reshape rows = %d, want 2", outer.Len())
	}
	row0, _ := AsList(outer.Get(0))
	_as12, _ := AsInteger(row0.Get(0))
	_as11, _ := AsInteger(row0.Get(2))
	if row0.Len() != 3 || _as12 != 1 || _as11 != 3 {
		t.Errorf("reshape row 0 = %v, want [1,2,3]", outer.Get(0))
	}
}

// --- flatten -1 (deep flatten, a core flatten depth) ---

// Deep flatten is `flatten -1` on the core flatten word — not a separate
// array word (ADR-001). A nested list collapses fully.
func TestFlattenFull(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	input := NewList([]Value{
		NewList([]Value{NewInteger(1), NewList([]Value{NewInteger(2), NewList([]Value{NewInteger(3)})})}),
		NewInteger(4),
	})
	result := runBoru(t, r, []Value{NewWord("flatten"), NewInteger(-1), input})
	list, _ := AsList(result[0])
	if list.Len() != 4 {
		t.Fatalf("flatten -1 length = %d, want 4", list.Len())
	}
	for i := 0; i < 4; i++ {
		got, _ := AsInteger(list.Get(i))
		if got != int64(i+1) {
			t.Errorf("flatten -1 [%d] = %d, want %d", i, got, i+1)
		}
	}
}

// Default flatten still removes only one level (the negative depth is the
// only "fully flatten" form).
func TestFlattenDefaultOneLevel(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	input := NewList([]Value{
		NewList([]Value{NewInteger(1), NewList([]Value{NewInteger(2)})}),
	})
	result := runBoru(t, r, []Value{NewWord("flatten"), input})
	list, _ := AsList(result[0])
	// [[1,[2]]] flatten -> [1,[2]] : one level removed, inner [2] survives.
	if list.Len() != 2 {
		t.Fatalf("flatten (default) length = %d, want 2", list.Len())
	}
	if inner, err := AsList(list.Get(1)); err != nil || inner.Len() != 1 {
		t.Errorf("flatten (default) should keep inner list, got %v", result[0])
	}
}

// --- transpose ---

func TestTranspose(t *testing.T) {
	r := arrayTestReg()
	input := NewList([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
		NewList([]Value{NewInteger(4), NewInteger(5), NewInteger(6)}),
	})
	result := runBoru(t, r, []Value{input, NewWord("transpose")})
	outer, _ := AsList(result[0])
	if outer.Len() != 3 {
		t.Fatalf("transpose rows = %d, want 3", outer.Len())
	}
	// First column: [1,4]
	col0, _ := AsList(outer.Get(0))
	_as16, _ := AsInteger(col0.Get(0))
	_as15, _ := AsInteger(col0.Get(1))
	if _as16 != 1 || _as15 != 4 {
		t.Errorf("transpose col 0 = %v, want [1,4]", outer.Get(0))
	}
}

// --- indices: list-membership lookup (boru:array-util) ---

// indices is an array word (ArrayUtil.indices), distinct from the
// string word indexof: for each needle, its index in the haystack, or
// -1 when absent. Forward form is `indices <needles> <haystack>` — the
// haystack (the larger reference collection) is the final argument.
func TestIndicesListLookup(t *testing.T) {
	r := arrayTestReg() // seeds ArrayModuleNatives as bare words
	result := runBoru(t, r, []Value{
		NewWord("indices"),
		NewList([]Value{NewInteger(20), NewInteger(99), NewInteger(10)}),
		NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30)}),
	})
	// 20→1, 99→absent→-1, 10→0.
	assertIntList(t, "indices [20,99,10] [10,20,30]", result, []int64{1, -1, 0})
}

// indexof is now string-only — the list form no longer dispatches here.
func TestIndexofStringStillWorks(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Haystack-last: forward form is `indexof needle haystack`.
	result := runBoru(t, r, []Value{
		NewWord("indexof"), NewString("ll"), NewString("hello"),
	})
	got, _ := AsInteger(result[0])
	if got != 2 {
		t.Errorf(`indexof "ll" "hello" = %d, want 2`, got)
	}
}

// indexof no longer accepts two lists — the list form moved to
// ArrayUtil.indices, so the [List, List] call must NOT match a string sig.
func TestIndexofListFormRemoved(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{
		NewWord("indexof"),
		NewList([]Value{NewInteger(20)}),
		NewList([]Value{NewInteger(10), NewInteger(20)}),
	})
	if err == nil {
		t.Fatalf("indexof [20] [10,20] should error (no list overload), got nil")
	}
}

// --- compress ---

func TestCompress(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("compress"),
		NewList([]Value{NewBoolean(true), NewBoolean(false), NewBoolean(true)}),
		NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30)}),
	})
	assertIntList(t, "compress [t,f,t] [10,20,30]", result, []int64{10, 30})
}

func TestCompressMismatchErrors(t *testing.T) {
	r := arrayTestReg()
	err := runBoruError(t, r, []Value{
		NewWord("compress"),
		NewList([]Value{NewBoolean(true), NewBoolean(false)}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
	})
	if err == nil {
		t.Fatalf("compress length mismatch: expected error, got none")
	}
}

// --- eachrank ---

// eachrank 0 targets each scalar leaf; the body doubles it.
func TestEachrankScalar(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("eachrank"), NewInteger(0),
		NewList([]Value{NewWord("mul"), NewInteger(2)}),
		NewList([]Value{
			NewList([]Value{NewInteger(1), NewInteger(2)}),
			NewList([]Value{NewInteger(3), NewInteger(4)}),
		}),
	})
	// [[2,4],[6,8]]
	outer, _ := AsList(result[0])
	row0, _ := AsList(outer.Get(0))
	a, _ := AsInteger(row0.Get(0))
	b, _ := AsInteger(row0.Get(1))
	if a != 2 || b != 4 {
		t.Errorf("eachrank 0 [mul 2] row0 = %v, want [2,4]", outer.Get(0))
	}
}

func TestEachrankOverRankErrors(t *testing.T) {
	r := arrayTestReg()
	err := runBoruError(t, r, []Value{
		NewWord("eachrank"), NewInteger(5),
		NewList([]Value{NewWord("reverse")}),
		NewList([]Value{NewInteger(1), NewInteger(2)}),
	})
	if err == nil {
		t.Fatalf("eachrank rank exceeding data: expected error, got none")
	}
}

// --- foldaxis ---

func TestFoldaxisColumns(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("foldaxis"), NewInteger(0),
		NewList([]Value{NewWord("add")}),
		NewList([]Value{
			NewList([]Value{NewInteger(1), NewInteger(2)}),
			NewList([]Value{NewInteger(3), NewInteger(4)}),
		}),
	})
	assertIntList(t, "foldaxis 0 [add]", result, []int64{4, 6})
}

func TestFoldaxisRows(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("foldaxis"), NewInteger(1),
		NewList([]Value{NewWord("add")}),
		NewList([]Value{
			NewList([]Value{NewInteger(1), NewInteger(2)}),
			NewList([]Value{NewInteger(3), NewInteger(4)}),
		}),
	})
	assertIntList(t, "foldaxis 1 [add]", result, []int64{3, 7})
}

func TestFoldaxisBadAxisErrors(t *testing.T) {
	r := arrayTestReg()
	err := runBoruError(t, r, []Value{
		NewWord("foldaxis"), NewInteger(2),
		NewList([]Value{NewWord("add")}),
		NewList([]Value{NewList([]Value{NewInteger(1)})}),
	})
	if err == nil {
		t.Fatalf("foldaxis bad axis: expected error, got none")
	}
}

// --- reverse ---

func TestReverse(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
		NewWord("reverse"),
	})
	list, _ := AsList(result[0])
	_as19, _ := AsInteger(list.Get(0))
	_as18, _ := AsInteger(list.Get(1))
	_as17, _ := AsInteger(list.Get(2))
	if _as19 != 3 || _as18 != 2 || _as17 != 1 {
		t.Errorf("reverse [1,2,3] = %v, want [3,2,1]", result[0])
	}
}

// --- take ---

func TestTake(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("take"), NewInteger(2),
		NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30), NewInteger(40)}),
	})
	list, _ := AsList(result[0])
	_as21, _ := AsInteger(list.Get(0))
	_as20, _ := AsInteger(list.Get(1))
	if list.Len() != 2 || _as21 != 10 || _as20 != 20 {
		t.Errorf("take 2 = %v, want [10,20]", result[0])
	}
}

func TestTakeNegative(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("take"), NewInteger(-2),
		NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30), NewInteger(40)}),
	})
	list, _ := AsList(result[0])
	_as23, _ := AsInteger(list.Get(0))
	_as22, _ := AsInteger(list.Get(1))
	if list.Len() != 2 || _as23 != 30 || _as22 != 40 {
		t.Errorf("take -2 = %v, want [30,40]", result[0])
	}
}

// --- shed ---

func TestShed(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("shed"), NewInteger(1),
		NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30), NewInteger(40)}),
	})
	list, _ := AsList(result[0])
	_as24, _ := AsInteger(list.Get(0))
	if list.Len() != 3 || _as24 != 20 {
		t.Errorf("shed 1 = %v, want [20,30,40]", result[0])
	}
}

// --- where ---

func TestWhere(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewBoolean(true), NewBoolean(false), NewBoolean(true), NewBoolean(false), NewBoolean(true)}),
		NewWord("where"),
	})
	list, _ := AsList(result[0])
	_as27, _ := AsInteger(list.Get(0))
	_as26, _ := AsInteger(list.Get(1))
	_as25, _ := AsInteger(list.Get(2))
	if list.Len() != 3 || _as27 != 0 || _as26 != 2 || _as25 != 4 {
		t.Errorf("where = %v, want [0,2,4]", result[0])
	}
}

// --- unique ---

func TestUnique(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(3), NewInteger(1), NewInteger(4), NewInteger(1), NewInteger(5)}),
		NewWord("unique"),
	})
	list, _ := AsList(result[0])
	if list.Len() != 4 {
		t.Fatalf("unique length = %d, want 4", list.Len())
	}
	expected := []int64{3, 1, 4, 5}
	for i, want := range expected {
		_as28, _ := AsInteger(list.Get(i))
		if _as28 != want {
			_as29, _ := AsInteger(list.Get(i))
			t.Errorf("unique[%d] = %d, want %d", i, _as29, want)
		}
	}
}

// --- grade ---

func TestGrade(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(30), NewInteger(10), NewInteger(40), NewInteger(20)}),
		NewWord("grade"),
	})
	list, _ := AsList(result[0])
	// Sorted order: 10(1), 20(3), 30(0), 40(2)
	expected := []int64{1, 3, 0, 2}
	for i, want := range expected {
		_as30, _ := AsInteger(list.Get(i))
		if _as30 != want {
			_as31, _ := AsInteger(list.Get(i))
			t.Errorf("grade[%d] = %d, want %d", i, _as31, want)
		}
	}
}

// --- at ---

func TestAt(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("at"),
		NewList([]Value{NewInteger(2), NewInteger(0), NewInteger(1)}),
		NewList([]Value{NewString("a"), NewString("b"), NewString("c")}),
	})
	list, _ := AsList(result[0])
	_as34, _ := AsString(list.Get(0))
	_as33, _ := AsString(list.Get(1))
	_as32, _ := AsString(list.Get(2))
	if _as34 != "c" || _as33 != "a" || _as32 != "b" {
		t.Errorf("at [2,0,1] = %v, want [c,a,b]", result[0])
	}
}

// --- sortby ---

func TestSortby(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("sortby"),
		NewList([]Value{NewInteger(3), NewInteger(1), NewInteger(2)}),
		NewList([]Value{NewString("c"), NewString("a"), NewString("b")}),
	})
	list, _ := AsList(result[0])
	_as37, _ := AsString(list.Get(0))
	_as36, _ := AsString(list.Get(1))
	_as35, _ := AsString(list.Get(2))
	if _as37 != "a" || _as36 != "b" || _as35 != "c" {
		t.Errorf("sortby = %v, want [a,b,c]", result[0])
	}
}

// --- member ---

func TestMember(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("member"),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
		NewList([]Value{NewInteger(2), NewInteger(4), NewInteger(6)}),
	})
	list, _ := AsList(result[0])
	_as38, _ := AsBoolean(list.Get(1))
	if !_as38 {
		t.Error("member: 2 should be in [2,4,6]")
	}
	_as39, _ := AsBoolean(list.Get(0))
	if _as39 {
		t.Error("member: 1 should NOT be in [2,4,6]")
	}
}

// --- window ---

func TestWindow(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("window"), NewInteger(2),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}),
	})
	list, _ := AsList(result[0])
	if list.Len() != 3 {
		t.Fatalf("window 2: length = %d, want 3", list.Len())
	}
	w0, _ := AsList(list.Get(0))
	_as41, _ := AsInteger(w0.Get(0))
	_as40, _ := AsInteger(w0.Get(1))
	if _as41 != 1 || _as40 != 2 {
		t.Errorf("window[0] = %v, want [1,2]", list.Get(0))
	}
}

// --- pairs ---

func TestPairs(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}),
		NewWord("pairs"),
	})
	list, _ := AsList(result[0])
	if list.Len() != 3 {
		t.Fatalf("pairs: length = %d, want 3", list.Len())
	}
}

// --- replicate ---

func TestReplicate(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("replicate"),
		NewList([]Value{NewInteger(2), NewInteger(0), NewInteger(3)}),
		NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30)}),
	})
	list, _ := AsList(result[0])
	// [10,10,30,30,30]
	if list.Len() != 5 {
		t.Fatalf("replicate length = %d, want 5", list.Len())
	}
	expected := []int64{10, 10, 30, 30, 30}
	for i, want := range expected {
		_as42, _ := AsInteger(list.Get(i))
		if _as42 != want {
			_as43, _ := AsInteger(list.Get(i))
			t.Errorf("replicate[%d] = %d, want %d", i, _as43, want)
		}
	}
}

// --- group ---

// Keys are Strings only (NUR030): an Atom key is declined, which is what
// keeps the record's collision case — the type literal `Integer` and the
// atom `Integer/q`, both rendering "Integer" — from re-opening.
func TestGroupTwoArgs(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("group"),
		NewList([]Value{NewString("a"), NewString("b"), NewString("a")}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
	})
	m, _ := AsMap(result[0])
	aVal, ok := m.Get("a")
	if !ok {
		t.Fatal("group: key 'a' not found")
	}
	aList, _ := AsList(aVal)
	_as45, _ := AsInteger(aList.Get(0))
	_as44, _ := AsInteger(aList.Get(1))
	if aList.Len() != 2 || _as45 != 1 || _as44 != 3 {
		t.Errorf("group a = %v, want [1,3]", aVal)
	}
}

// --- each (higher-order) ---

func TestEach(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("each"),
		NewList([]Value{NewWord("mul"), NewInteger(2)}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
	})
	list, _ := AsList(result[0])
	expected := []int64{2, 4, 6}
	for i, want := range expected {
		_as46, _ := AsInteger(list.Get(i))
		if _as46 != want {
			_as47, _ := AsInteger(list.Get(i))
			t.Errorf("each[%d] = %d, want %d", i, _as47, want)
		}
	}
}

// --- for-each (§7.4): side-effecting iteration that discards results ---

// for-each runs the body once per element for its side effects and
// produces nothing, leaving the stack clean.
func TestForEachProducesNothing(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
		NewWord("for-each"),
		NewList([]Value{NewWord("drop")}),
	})
	if len(result) != 0 {
		t.Errorf("for-each should leave the stack empty, got %v", result)
	}
}

// for-each tolerates a body that leaves the stack empty (a None-producing
// / purely-mutating body) — the case where `each` errors. This is the
// §7.4 sentinel-free mutating loop.
func TestForEachAllowsEmptyBody(t *testing.T) {
	r := arrayTestReg()
	// Body `drop` consumes the element and pushes nothing.
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(10), NewInteger(20)}),
		NewWord("for-each"),
		NewList([]Value{NewWord("drop")}),
	})
	if len(result) != 0 {
		t.Fatalf("for-each empty body: expected clean stack, got %v", result)
	}
}

// Contrast: `each` with the same empty-producing body must still error,
// pinning the behavioural difference between each and for-each.
func TestEachStillRequiresResult(t *testing.T) {
	r := arrayTestReg()
	_, err := New(r).Run([]Value{
		NewList([]Value{NewInteger(1)}),
		NewWord("each"),
		NewList([]Value{NewWord("drop")}),
	})
	if err == nil || !strings.Contains(err.Error(), "body produced no result") {
		t.Fatalf("each with empty body should error, got %v", err)
	}
}

// for-each surfaces an error raised inside the body.
func TestForEachPropagatesBodyError(t *testing.T) {
	r := arrayTestReg()
	_, err := New(r).Run([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewWord("for-each"),
		NewList([]Value{NewWord("definitely-undefined-word")}),
	})
	if err == nil {
		t.Fatal("expected for-each to propagate the body's undefined-word error")
	}
}

// --- fold ---

func TestFoldSum(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("fold"),
		NewList([]Value{NewWord("add")}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}),
	})
	_as48, _ := AsInteger(result[0])
	if _as48 != 10 {
		t.Errorf("fold [add] = %v, want 10", result[0])
	}
}

func TestFoldProduct(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("fold"),
		NewList([]Value{NewWord("mul")}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}),
	})
	_as49, _ := AsInteger(result[0])
	if _as49 != 24 {
		t.Errorf("fold [mul] = %v, want 24", result[0])
	}
}

// --- scan ---

func TestScan(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("scan"),
		NewList([]Value{NewWord("add")}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}),
	})
	list, _ := AsList(result[0])
	expected := []int64{1, 3, 6, 10}
	for i, want := range expected {
		_as50, _ := AsInteger(list.Get(i))
		if _as50 != want {
			_as51, _ := AsInteger(list.Get(i))
			t.Errorf("scan[%d] = %d, want %d", i, _as51, want)
		}
	}
}

// --- outer ---

func TestOuterMul(t *testing.T) {
	r := arrayTestReg()
	result := runBoru(t, r, []Value{
		NewWord("outer"),
		NewList([]Value{NewWord("mul")}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}),
	})
	// [[1,2,3,4],[2,4,6,8],[3,6,9,12]]
	outer, _ := AsList(result[0])
	if outer.Len() != 3 {
		t.Fatalf("outer rows = %d, want 3", outer.Len())
	}
	row0, _ := AsList(outer.Get(0))
	_as52, _ := AsInteger(row0.Get(3))
	if _as52 != 4 {
		_as53, _ := AsInteger(row0.Get(3))
		t.Errorf("outer[0][3] = %d, want 4", _as53)
	}
	row2, _ := AsList(outer.Get(2))
	_as54, _ := AsInteger(row2.Get(2))
	if _as54 != 9 {
		_as55, _ := AsInteger(row2.Get(2))
		t.Errorf("outer[2][2] = %d, want 9", _as55)
	}
}

// --- inner (matrix multiply) ---

func TestInnerMatMul(t *testing.T) {
	r := arrayTestReg()
	left := NewList([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewList([]Value{NewInteger(3), NewInteger(4)}),
	})
	right := NewList([]Value{
		NewList([]Value{NewInteger(5), NewInteger(6)}),
		NewList([]Value{NewInteger(7), NewInteger(8)}),
	})
	result := runBoru(t, r, []Value{
		NewWord("inner"),
		NewList([]Value{NewWord("mul")}),
		NewList([]Value{NewWord("add")}),
		left, right,
	})
	// [[1*5+2*7, 1*6+2*8], [3*5+4*7, 3*6+4*8]] = [[19,22],[43,50]]
	outer, _ := AsList(result[0])
	r0, _ := AsList(outer.Get(0))
	r1, _ := AsList(outer.Get(1))
	_as57, _ := AsInteger(r0.Get(0))
	_as56, _ := AsInteger(r0.Get(1))
	if _as57 != 19 || _as56 != 22 {
		_as59, _ := AsInteger(r0.Get(0))
		_as58, _ := AsInteger(r0.Get(1))
		t.Errorf("inner row 0 = [%d,%d], want [19,22]", _as59, _as58)
	}
	_as61, _ := AsInteger(r1.Get(0))
	_as60, _ := AsInteger(r1.Get(1))
	if _as61 != 43 || _as60 != 50 {
		_as63, _ := AsInteger(r1.Get(0))
		_as62, _ := AsInteger(r1.Get(1))
		t.Errorf("inner row 1 = [%d,%d], want [43,50]", _as63, _as62)
	}
}

// --- composition: fold [add] each [dup mul] iota 5 => sum of squares 0+1+4+9+16=30 ---

func TestCompositionSumOfSquares(t *testing.T) {
	r := arrayTestReg()
	// (each [dup mul] (iota 5)) produces [0,1,4,9,16]
	// fold [add] over that produces 30
	result := runBoru(t, r, []Value{
		NewWord("fold"),
		NewList([]Value{NewWord("add")}),
		NewOpenParen(),
		NewWord("each"),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
		NewOpenParen(), NewWord("iota"), NewInteger(5), NewCloseParen(),
		NewCloseParen(),
	})
	_as64, _ := AsInteger(result[0])
	if _as64 != 30 {
		t.Errorf("sum of squares = %v, want 30", result[0])
	}
}

// reshape and iota must bound their allocation against user-supplied
// dimensions: a huge dimension, an overflowing shape product, or a huge
// iota count must return an error rather than reach the panicking
// make([]Value, …) path. Drive the handlers directly so we observe the
// returned error. See ADR-005.
func intList(ns ...int64) Value {
	elems := make([]Value, len(ns))
	for i, n := range ns {
		elems[i] = NewInteger(n)
	}
	return NewList(elems)
}

func TestReshapeBounds(t *testing.T) {
	r := arrayTestReg()
	bad := [][2]Value{
		{intList(4611686018427387904, 4), intList()}, // product overflows to 0
		{intList(99999999, 2), intList()},            // single dim over the cap
		{intList(-1, 2), intList(1, 2)},              // negative dim
	}
	for _, c := range bad {
		if _, err := reshapeHandler([]Value{c[0], c[1]}, nil, nil, r); err == nil {
			t.Errorf("reshape(%v, %v): expected error, got nil", c[0], c[1])
		}
	}
	// A valid reshape still works.
	out, err := reshapeHandler([]Value{intList(2, 3), intList(1, 2, 3, 4, 5, 6)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Errorf("valid reshape = (%v, %v), want one result", out, err)
	}
}

func TestIotaBounds(t *testing.T) {
	r := arrayTestReg()
	if _, err := iotaHandler([]Value{NewInteger(999999999)}, nil, nil, r); err == nil {
		t.Error("iota over cap: expected error, got nil")
	}
	if _, err := iotaHandler([]Value{NewInteger(-1)}, nil, nil, r); err == nil {
		t.Error("iota negative: expected error, got nil")
	}
	if out, err := iotaHandler([]Value{NewInteger(5)}, nil, nil, r); err != nil || len(out) != 1 {
		t.Errorf("iota 5 = (%v, %v), want one list", out, err)
	}
}
