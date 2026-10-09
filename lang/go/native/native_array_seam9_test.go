package native

import (
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Coverage for the error/edge arms of the array words (native_array.go):
// shape/reshape helpers, take/shed boundary maths, the arrCompareValues
// error fallback, cross-collection redirects, the reach-form lens error
// arms, and the inner/eachrank/foldaxis body-run failure arms. Handlers
// and their pure helpers are driven directly with crafted args.

func w9AddBody() Value { return w9List(NewWord("add")) }

// w9Reach parses a receiverless reach (`$.foo`) and returns the Reach value.
func w9Reach(t *testing.T) Value {
	t.Helper()
	toks, err := parser.Parse("$.foo")
	if err != nil {
		t.Fatalf("parse reach: %v", err)
	}
	for _, tk := range toks {
		if _, e := AsReach(tk); e == nil {
			return tk
		}
	}
	t.Fatalf("no reach token in parse of $.foo")
	return Value{}
}

func TestW9ShapeHelpers(t *testing.T) {
	// computeShape: nil list, empty list, and a ragged sub-list.
	if s := computeShape(NewTypeLiteral(TList)); s != nil {
		t.Fatalf("computeShape(nil list) = %v, want nil", s)
	}
	// An explicitly non-nil empty list (variadic w9List() would pass a nil
	// slice, which reads back as IsNil and hits the nil arm instead).
	if s := computeShape(NewList([]Value{})); len(s) != 1 || s[0] != 0 {
		t.Fatalf("computeShape(empty) = %v, want [0]", s)
	}
	ragged := w9List(w9List(NewInteger(1), NewInteger(2)), w9List(NewInteger(3)))
	if s := computeShape(ragged); len(s) != 1 || s[0] != 2 {
		t.Fatalf("computeShape(ragged) = %v, want [2]", s)
	}

	// flattenList: nil list and the nested-recursion branch.
	if f := flattenList(NewTypeLiteral(TList)); f != nil {
		t.Fatalf("flattenList(nil) = %v, want nil", f)
	}
	if f := flattenList(w9List(w9List(NewInteger(1), NewInteger(2)))); len(f) != 2 {
		t.Fatalf("flattenList(nested) len = %d, want 2", len(f))
	}

	// buildNested: rank-0 with one element unwraps; rank-0 with many wraps.
	if v := buildNested([]Value{NewInteger(5)}, nil); !v.Is(TInteger) {
		t.Fatalf("buildNested single = %v, want Integer", v)
	}
	if v := buildNested([]Value{NewInteger(1), NewInteger(2)}, nil); !v.Parent.ConformsTo(TList) {
		t.Fatalf("buildNested many = %v, want List", v)
	}

	// reshape with a zero dimension exercises the product=0 short-circuit.
	if _, err := reshapeHandler([]Value{w9List(NewInteger(0)), w9List()}, nil, nil, w9Reg(t)); err != nil {
		t.Fatalf("reshape [0] []: %v", err)
	}

	// listDepth breaks on an empty spine list.
	if d := listDepth(w9List()); d != 1 {
		t.Fatalf("listDepth(empty) = %d, want 1", d)
	}

	// transposeListOfLists of no rows returns nil.
	if c := transposeListOfLists(NewReadList(nil)); c != nil {
		t.Fatalf("transpose(empty) = %v, want nil", c)
	}
}

func TestW9TakeShedWhereUnique(t *testing.T) {
	r := w9Reg(t)
	list := w9List(NewInteger(1), NewInteger(2), NewInteger(3))

	// take negative with |n| > length clamps to the whole list.
	if _, err := takeHandler([]Value{NewInteger(-5), list}, nil, nil, r); err != nil {
		t.Fatalf("take -5: %v", err)
	}
	// shed positive with n > length clamps start to length (empty result).
	if _, err := shedHandler([]Value{NewInteger(5), list}, nil, nil, r); err != nil {
		t.Fatalf("shed 5: %v", err)
	}
	// shed negative with |n| > length clamps abs to length.
	if _, err := shedHandler([]Value{NewInteger(-5), list}, nil, nil, r); err != nil {
		t.Fatalf("shed -5: %v", err)
	}
	// where with no truthy element returns an empty (non-nil) list.
	out, err := whereHandler([]Value{w9List(NewBoolean(false), NewBoolean(false))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("where all-false: out=%v err=%v", out, err)
	}
	// unique over an empty list returns an empty list.
	out, err = uniqueHandler([]Value{w9List()}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("unique empty: out=%v err=%v", out, err)
	}
}

func TestW9ArrCompareValuesFallback(t *testing.T) {
	// Values with a nil Parent make CompareValues error, forcing the
	// string-comparison fallback with distinguishable string forms.
	a := NewString("aaa")
	a.Parent = nil
	b := NewString("bbb")
	b.Parent = nil
	if got := arrCompareValues(a, b); got != -1 {
		t.Fatalf("arrCompareValues(a,b) = %d, want -1", got)
	}
	if got := arrCompareValues(b, a); got != 1 {
		t.Fatalf("arrCompareValues(b,a) = %d, want 1", got)
	}
	if got := arrCompareValues(a, a); got != 0 {
		t.Fatalf("arrCompareValues(a,a) = %d, want 0", got)
	}
}

func TestW9ListLengthMismatches(t *testing.T) {
	r := w9Reg(t)
	two := w9List(NewInteger(1), NewInteger(2))
	one := w9List(NewInteger(9))

	// sortby: non-concrete data list rejected.
	if _, err := sortbyHandler([]Value{two, NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatalf("sortby non-list data: expected error")
	}
	// group (two-arg): non-concrete values list rejected.
	if _, err := groupTwoHandler([]Value{two, NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatalf("group non-list values: expected error")
	}
	// group (two-arg): length mismatch.
	if _, err := groupTwoHandler([]Value{two, one}, nil, nil, r); err == nil {
		t.Fatalf("group length mismatch: expected error")
	}
	// replicate: length mismatch.
	if _, err := replicateHandler([]Value{two, one}, nil, nil, r); err == nil {
		t.Fatalf("replicate length mismatch: expected error")
	}
}

func TestW9CrossCollectionRedirects(t *testing.T) {
	r := w9Reg(t)
	m := w9Map("a", NewInteger(1))

	// each / fold / scan handlers redirect to their Map twin when the
	// runtime collection turns out to be a concrete Map.
	if _, err := eachHandler([]Value{w9List(), m}, nil, nil, r); err != nil {
		t.Fatalf("eachHandler map redirect: %v", err)
	}
	if _, err := foldNoInitHandler([]Value{w9List(), m}, nil, nil, r); err != nil {
		t.Fatalf("foldNoInitHandler map redirect: %v", err)
	}
	if _, err := scanHandler([]Value{w9List(), m}, nil, nil, r); err != nil {
		t.Fatalf("scanHandler map redirect: %v", err)
	}
}

func TestW9ReachLensErrors(t *testing.T) {
	r := w9Reg(t)
	reach := w9Reach(t)
	scalars := w9List(NewInteger(1), NewInteger(2))

	// Applying `$.foo` to scalar elements errors (no dot on an Integer).
	if _, err := eachReachHandler([]Value{reach, scalars}, nil, nil, r); err == nil {
		t.Fatalf("each reach over scalars: expected error")
	}
	if _, err := sortbyReachHandler([]Value{reach, scalars}, nil, nil, r); err == nil {
		t.Fatalf("sortby reach over scalars: expected error")
	}
}

func TestW9InnerBodyFailures(t *testing.T) {
	r := w9Reg(t)
	drive := w9Driven(r)
	left1 := w9List(NewInteger(1), NewInteger(2))
	right1 := w9List(NewInteger(3), NewInteger(4))
	left2 := w9List(w9List(NewInteger(1), NewInteger(2)))
	right2 := w9List(w9List(NewInteger(3), NewInteger(4)), w9List(NewInteger(5), NewInteger(6)))

	// 1D: pair-op error, then agg-op error.
	if _, err := drive(innerHandler([]Value{w9ErrBody(), w9AddBody(), left1, right1}, nil, nil, r)); err == nil {
		t.Fatalf("inner 1D pair error: expected error")
	}
	if _, err := drive(innerHandler([]Value{w9AddBody(), w9ErrBody(), left1, right1}, nil, nil, r)); err == nil {
		t.Fatalf("inner 1D agg error: expected error")
	}

	// 2D: pair error, pair no-result, agg error, agg no-result.
	if _, err := drive(innerHandler([]Value{w9ErrBody(), w9AddBody(), left2, right2}, nil, nil, r)); err == nil {
		t.Fatalf("inner 2D pair error: expected error")
	}
	if _, err := drive(innerHandler([]Value{w9Drop2Body(), w9AddBody(), left2, right2}, nil, nil, r)); err == nil {
		t.Fatalf("inner 2D pair no-result: expected error")
	}
	if _, err := drive(innerHandler([]Value{w9AddBody(), w9ErrBody(), left2, right2}, nil, nil, r)); err == nil {
		t.Fatalf("inner 2D agg error: expected error")
	}
	if _, err := drive(innerHandler([]Value{w9AddBody(), w9Drop2Body(), left2, right2}, nil, nil, r)); err == nil {
		t.Fatalf("inner 2D agg no-result: expected error")
	}
}

func TestW9EachRankFoldAxisArms(t *testing.T) {
	r := w9Reg(t)
	drive := w9Driven(r)

	// eachrank: non-integer rank rejected.
	if _, err := eachrankHandler([]Value{NewTypeLiteral(TInteger), w9AddBody(), w9List(NewInteger(1))}, nil, nil, r); err == nil {
		t.Fatalf("eachrank non-int rank: expected error")
	}
	// The walk runs on the tape (eachrankDriver): a body error at a leaf,
	// and a non-list cell where a list is expected — met lazily, after the
	// cell before it ran — both reach the run as the word's error.
	ar := arrayTestReg()
	if _, err := w3Run(t, ar, "eachrank 0 [nope_word_xyz] [5]"); err == nil || !strings.Contains(err.Error(), "eachrank: ") {
		t.Fatalf("eachrank body error: got %v", err)
	}
	if _, err := w3Run(t, ar, "eachrank 0 [dup] [[1] 2]"); err == nil || !strings.Contains(err.Error(), "rank exceeds nesting depth") {
		t.Fatalf("eachrank depth exceeds: got %v", err)
	}

	// foldaxis: non-integer axis rejected.
	if _, err := drive(foldaxisHandler([]Value{NewTypeLiteral(TInteger), w9AddBody(), w9List(w9List(NewInteger(1)))}, nil, nil, r)); err == nil {
		t.Fatalf("foldaxis non-int axis: expected error")
	}
	// foldaxis: a lane body that errors.
	matrix := w9List(w9List(NewInteger(1), NewInteger(2)), w9List(NewInteger(3), NewInteger(4)))
	if _, err := drive(foldaxisHandler([]Value{NewInteger(1), w9ErrBody(), matrix}, nil, nil, r)); err == nil {
		t.Fatalf("foldaxis lane error: expected error")
	}
}

func TestW9BodyAnalysisFailures(t *testing.T) {
	r := w9Reg(t)

	// analyseHigherOrderBodyVals records a diagnostic and returns nil when
	// the body's sub-engine run errors (here an undefined word — the
	// analyser is driven outside check mode so the error is not swallowed).
	base := len(r.Check.Diagnostics)
	if stk := analyseHigherOrderBodyVals(r, w9ErrBody(), NewCarrier(TInteger)); stk != nil {
		t.Fatalf("analyseHigherOrderBodyVals error body: got %v, want nil", stk)
	}
	if len(r.Check.Diagnostics) != base+1 {
		t.Fatalf("expected a body_error diagnostic, have %d", len(r.Check.Diagnostics))
	}

	// The fold / scan accumulator fixed points fall back to the bare
	// carrier when the body is not analysable.
	nonEmpty := w9List(NewInteger(1))
	if out := foldNoInitReturnsFn([]Value{w9ErrBody(), nonEmpty}, r); len(out) != 1 {
		t.Fatalf("foldNoInitReturnsFn fallback: %v", out)
	}
	if out := scanReturnsFn([]Value{w9ErrBody(), nonEmpty}, r); len(out) != 1 {
		t.Fatalf("scanReturnsFn fallback: %v", out)
	}
	// foldaxis's analysing ReturnsFn (NUR115) shares the fixed point and the
	// fallback: an unanalysable lane body answers the bare List carrier.
	if out := foldaxisReturnsFn([]Value{NewInteger(1), w9ErrBody(), w9List(w9List(NewInteger(1)))}, r); len(out) != 1 || !out[0].Parent.ConformsTo(TList) {
		t.Fatalf("foldaxisReturnsFn fallback: %v", out)
	}
}
