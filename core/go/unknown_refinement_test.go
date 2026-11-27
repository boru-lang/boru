package core

import (
	"errors"
	"strings"
	"testing"
)

// passNode mints what the analysis pass mints for `def name (Integer gt
// (size s))`: a node whose refinement's bound is the pass's carrier.
func passNode(r *Registry, name string) *Type {
	placeholder := NewDepScalar(DepGT, NewCarrier(TInteger))
	di, _ := placeholder.AsDepScalar()
	n := r.Types.MintType(name, TInteger)
	installDepScalarUnifier(n, TInteger, di, name)
	n.SetTypeBody(placeholder)
	return n
}

// TestNegationRenders pins a negation's rendering: a simple inner follows
// `tnot` bare, a compound one is parenthesised so it cannot misread.
func TestNegationRenders(t *testing.T) {
	if got := NewNegation(NewTypeLiteral(TInteger)).String(); got != "tnot Integer" {
		t.Errorf("simple inner: %q", got)
	}
	inner := NewNegation(NewTypeLiteral(TInteger))
	if got := NewNegation(inner).String(); got != "tnot (tnot Integer)" {
		t.Errorf("compound inner must be parenthesised: %q", got)
	}
}

// TestCombineOverUnknownBounds pins the intersection of refinements when a
// bound is one the pass does not know: which side is tighter is the run's,
// the unknown bound is kept (so the result stays a type the run installs),
// and emptiness is decided only over known bounds — `(Integer lt (size s))
// tand (Integer gt 5)` was Never at compile time whatever s held.
func TestCombineOverUnknownBounds(t *testing.T) {
	unk := func() *DepBound { return &DepBound{Value: NewCarrier(TInteger)} }
	kn := func(n int64) *DepBound { return &DepBound{Value: NewInteger(n)} }
	for _, tc := range []struct {
		name   string
		a, b   DepScalarInfo
		loUnk  bool
		hiUnk  bool
		nonNil bool
	}{
		{"unknown lo first", DepScalarInfo{Lo: unk()}, DepScalarInfo{Lo: kn(1)}, true, false, false},
		{"unknown lo second", DepScalarInfo{Lo: kn(1)}, DepScalarInfo{Lo: unk()}, true, false, false},
		{"unknown hi first", DepScalarInfo{Hi: unk()}, DepScalarInfo{Hi: kn(9)}, false, true, false},
		{"unknown hi second", DepScalarInfo{Hi: kn(9)}, DepScalarInfo{Hi: unk()}, false, true, false},
		{"unknown upper over known lower", DepScalarInfo{Hi: unk()}, DepScalarInfo{Lo: kn(5)}, false, true, true},
	} {
		out, ok := combineDepScalars(tc.a, tc.b)
		if !ok {
			t.Errorf("%s: an interval over an unknown bound is never proved empty", tc.name)
			continue
		}
		if tc.loUnk && (out.Lo == nil || depBoundKnown(out.Lo)) {
			t.Errorf("%s: the unknown lower bound is kept: %+v", tc.name, out.Lo)
		}
		if tc.hiUnk && (out.Hi == nil || depBoundKnown(out.Hi)) {
			t.Errorf("%s: the unknown upper bound is kept: %+v", tc.name, out.Hi)
		}
		if tc.nonNil && (out.Lo == nil || out.Hi == nil) {
			t.Errorf("%s: both sides survive: %+v", tc.name, out)
		}
	}
	if _, ok := combineDepScalars(DepScalarInfo{Hi: kn(2)}, DepScalarInfo{Lo: kn(5)}); ok {
		t.Error("over known bounds, (lt 2) tand (gt 5) is still empty")
	}
}

// TestNegationOverUnknownAdmits pins the complement's half of "the pass
// decides no membership over an unknown bound": the refinement admits every
// value, so its complement would refuse every value — the pass admits, and
// the run decides. Over a known bound the complement still decides.
func TestNegationOverUnknownAdmits(t *testing.T) {
	if _, ok := Unify(NewInteger(9), NewNegation(NewDepScalar(DepGT, NewCarrier(TInteger)))); !ok {
		t.Error("tnot over an unknown bound admits at check time")
	}
	if _, ok := Unify(NewInteger(9), NewNegation(NewDepScalar(DepGT, NewInteger(3)))); ok {
		t.Error("tnot (Integer gt 3) refuses 9")
	}
}

// TestMakeFieldErrorIsStructured pins NUR310: make's field refusal is a
// type_error on both lanes — a plain error was the interpreter's raise, and a
// compiled run books a plain error as a compiler defect. A refusal already
// structured keeps its own code.
func TestMakeFieldErrorIsStructured(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewDepScalar(DepGT, NewInteger(100)))
	src := NewOrderedMap()
	src.Set("x", NewInteger(3))
	_, err := MakeRecordR(RecordTypeInfo{Fields: fields}, NewMap(src), false, nil)
	var ae *BoruError
	if !errors.As(err, &ae) || ae.Code != "type_error" || !strings.HasPrefix(ae.Detail, `make: field "x": `) {
		t.Errorf("a field refusal is a type_error: %v", err)
	}
	inner := &BoruError{Code: "raise_error", Detail: "boom"}
	wrapped := makeFieldError("y", inner)
	if !errors.As(wrapped, &ae) || ae != inner || !strings.Contains(wrapped.Error(), `make: field "y"`) {
		t.Errorf("a structured refusal keeps its code: %v", wrapped)
	}
}
