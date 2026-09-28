package core

import "testing"

// TestNoneLiteralIsAUnionMember pins NUR324's kernel half: the None literal —
// what a missing member reads as, and a VALUE at dispatch — is a member of a
// union that names None, as `none` is, at the union's Match and at the
// slot's type-literal rule; a type literal stays no member.
func TestNoneLiteralIsAUnionMember(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	maybe, err := r.DefineType("Maybe", NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TNone)}))
	if err != nil {
		t.Fatal(err)
	}
	noneLit := NewTypeLiteral(TNone)
	if !noneLit.Is(maybe) || !NewNone().Is(maybe) || !NewInteger(5).Is(maybe) {
		t.Error("the None literal, none and 5 are all members of (Integer tor None)")
	}
	if NewTypeLiteral(TInteger).Is(maybe) || NewString("s").Is(maybe) {
		t.Error("a type literal and a String are no members")
	}
	if rejectsTypeLiteral(noneLit, maybe) || !rejectsTypeLiteral(NewTypeLiteral(TInteger), maybe) {
		t.Error("the slot admits the None literal and refuses the Integer literal")
	}
	sig := []Signature{{Args: []*Type{maybe}, BarrierPos: -1}}
	if MatchSignature(sig, []Value{noneLit}, WordInfo{ArgCount: -1}) == nil {
		t.Error("a Maybe slot takes the None literal")
	}
	if MatchSignature(sig, []Value{NewTypeLiteral(TInteger)}, WordInfo{ArgCount: -1}) != nil {
		t.Error("a Maybe slot refuses a type literal")
	}
}

// TestAnnotationRunDependent pins NUR325's predicate: a carrier with no type
// content is a value only the run computes, alone or as a union's
// alternative; a carrier holding its type (tnot's negation) is the type.
func TestAnnotationRunDependent(t *testing.T) {
	neg := NegateType(NewTypeLiteral(TInteger))
	neg.Carrier = true
	for _, c := range []struct {
		name string
		v    Value
		want bool
	}{
		{"scalar carrier", NewCarrier(TInteger), true},
		{"type carrier", NewCarrier(TType), true},
		{"negation carrier", neg, false},
		{"union over a carrier", NewDisjunct([]Value{NewTypeLiteral(TString), NewCarrier(TType)}), true},
		{"union of types", NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TInteger)}), false},
		{"value", NewInteger(3), false},
		{"type", NewTypeLiteral(TInteger), false},
	} {
		if got := annotationRunDependent(c.v); got != c.want {
			t.Errorf("%s: annotationRunDependent = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestEvalSigTypeExprAnalysedAnnotations pins NUR325 and NUR326 at the
// annotation's evaluation under an analysis pass: a run-dependent value
// notes the building word run-dependent (it declines), and tnot's exact
// negation comes back as the type itself, not a carrier a match admits
// anything at.
func TestEvalSigTypeExprAnalysedAnnotations(t *testing.T) {
	r, rec := analysingRegistry(t)
	// The pass's values for `(1 add 2)` and `(tnot Integer)`, bound as the
	// annotation's word reads them (core runs no word's analysis half).
	neg := NegateType(NewTypeLiteral(TInteger))
	neg.Carrier = true
	r.Defs.Push("zzsum", NewCarrier(TInteger))
	r.Defs.Push("zzneg", neg)
	got, err := EvalSigTypeExpr(r, NewParenExpr([]Value{NewWord("zzsum")}), "x")
	if err != nil || !got.Carrier || rec.dependents != 1 {
		t.Fatalf("a carrier annotation notes the word run-dependent: %v %v %d", got, err, rec.dependents)
	}
	got, err = EvalSigTypeExpr(r, NewParenExpr([]Value{NewWord("zzneg")}), "x")
	if err != nil || got.Carrier || !IsNegation(got) || rec.dependents != 1 {
		t.Fatalf("tnot's negation is the type itself and notes nothing: %v %v %d", got, err, rec.dependents)
	}
	kind, pat, err := ResolveSigType(r, got)
	if err != nil || kind != TAny || pat == nil || !IsNegation(*pat) {
		t.Fatalf("an inline negation constrains through its pattern: %v %v %v", kind, pat, err)
	}
	if _, ok := Unify(NewInteger(5), *pat); ok {
		t.Error("the pattern refuses 5")
	}
	if _, ok := Unify(NewString("s"), *pat); !ok {
		t.Error("the pattern admits 's'")
	}
}
