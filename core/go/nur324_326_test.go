package core

import (
	"strings"
	"testing"
)

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
	// A None literal the return contract refuses is named, not `<nil>`.
	if detail, _ := ReturnTypeErrorText("f", 1, TInteger, noneLit); !strings.Contains(detail, "got None") {
		t.Errorf("return text %q must name the None literal", detail)
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
		{"typed list over a carrier child", NewTypedList(NewCarrier(TType)), true},
		{"typed list of a type", NewTypedList(NewTypeLiteral(TInteger)), false},
	} {
		if got := AnnotationRunDependent(c.v); got != c.want {
			t.Errorf("%s: AnnotationRunDependent = %v, want %v", c.name, got, c.want)
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
	// A typed container whose paren child the pass evaluates to a carrier
	// notes the building word run-dependent too (NUR327).
	if _, err := ResolveChildTypeExpr(r, NewTypedList(NewParenExpr([]Value{NewWord("zzsum")}))); err != nil || rec.dependents != 2 {
		t.Fatalf("a run-dependent child notes the word: %v %d", err, rec.dependents)
	}
	got, err = EvalSigTypeExpr(r, NewParenExpr([]Value{NewWord("zzneg")}), "x")
	if err != nil || got.Carrier || !IsNegation(got) || rec.dependents != 2 {
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

// TestInstallTypeResolvesAParenChild pins NUR327's kernel half: a typed
// container body whose child is a paren expression evaluates it at the
// install, as a fn parameter's annotation does; a child that cannot run is
// the install's error, not a raw paren no element satisfies.
func TestInstallTypeResolvesAParenChild(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := NewTypedList(NewParenExpr([]Value{NewWord("Integer")}))
	if err := InstallType(r, "Zints", body); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	got, ok := r.TopTypeBody("Zints")
	ci, cerr := AsChildType(got)
	if !ok || cerr != nil || IsParenExpr(ci.Child) || !(&ci.Child).Equal(TInteger) {
		t.Fatalf("the installed child is Integer, not a paren: %v %v", got, cerr)
	}
	if err := InstallType(r, "Zbad", NewTypedList(NewParenExpr([]Value{NewWord("nosuchword")}))); err == nil {
		t.Error("a child that cannot run is the install's error")
	}
}

// TestParamAdmitsAndTypeCarriers pins NUR328's kernel half: a parameter
// admits a value as the interpreter's dispatch does (the type match and its
// type-literal rule), and the check pass's Type and None carriers — its
// stand-ins for a type value and the None literal — are Type members.
func TestParamAdmitsAndTypeCarriers(t *testing.T) {
	for _, c := range []struct {
		name string
		v    Value
		t    *Type
		want bool
	}{
		{"a value", NewInteger(5), TInteger, true},
		{"its type", NewTypeLiteral(TInteger), TInteger, false},
		{"a type at Type", NewTypeLiteral(TInteger), TType, true},
		{"the None literal at None", NewTypeLiteral(TNone), TNone, true},
		{"a List node at List", NewTypeLiteral(TList), TList, false},
		{"a String at Integer", NewString("s"), TInteger, false},
	} {
		if got := ParamAdmits(c.v, c.t); got != c.want {
			t.Errorf("%s: ParamAdmits = %v, want %v", c.name, got, c.want)
		}
	}
	for _, c := range []struct {
		name string
		v    Value
		want bool
	}{
		{"Type carrier", NewCarrier(TType), true},
		{"None carrier", NewCarrier(TNone), true},
		{"gradual Type", NewDynamicCarrier(TType), false},
		{"Integer carrier", NewCarrier(TInteger), false},
		{"type literal", NewTypeLiteral(TInteger), true},
		{"a value", NewInteger(5), false},
	} {
		if got := TypeMembership(c.v); got != c.want {
			t.Errorf("%s: TypeMembership = %v, want %v", c.name, got, c.want)
		}
	}
}
