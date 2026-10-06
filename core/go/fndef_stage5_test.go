package core

import (
	"strings"
	"testing"
)

// Stage-5 coverage for fn_def.go: the ParseFnDef happy paths (triple
// walking, input-sig wrapping, literal return constraints) and the
// ParseFnUndefSpec pair walker. The signature-context classifiers this
// file also covered (OutputSigIsConcreteReturns / IsSigTypeValue /
// isSigTypeName / OutputSigValues) went with the return-by-value sugar
// they served — an output sig is now always types, so there is nothing
// left to classify.

func fndefStage5List(elems ...Value) Value { return NewList(elems) }

func TestFnDefStage5ParseFnDefTriples(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	list := []Value{
		// Triple 1: list input, type-word output, list body.
		fndefStage5List(NewWord("n:Integer")), NewWord("Integer"), fndefStage5List(NewWord("n")),
		// Triple 2: NON-list input (auto-wrapped to a one-element list),
		// single LITERAL output (a value is a type — the return must
		// match 42), non-list body.
		NewWord("Integer"), NewInteger(42), NewWord("zz-b"),
		// Triple 3: a LIST of two literal return constraints.
		fndefStage5List(NewWord("m:Integer")), fndefStage5List(NewInteger(1), NewString("ok")), fndefStage5List(NewWord("m")),
	}
	info, perr := ParseFnDef(r, list)
	if perr != nil {
		t.Fatalf("ParseFnDef: %v", perr)
	}
	if len(info.Signatures) != 3 {
		t.Fatalf("want 3 sigs, got %d", len(info.Signatures))
	}

	s0 := info.Signatures[0]
	if len(s0.Params) != 1 || s0.Params[0].Name != "n" || !s0.Params[0].Type.Equal(TInteger) {
		t.Errorf("sig0 params = %+v", s0.Params)
	}
	if len(s0.Returns) != 1 || !s0.Returns[0].Equal(TInteger) {
		t.Errorf("sig0 returns = %v", s0.Returns)
	}
	if len(s0.Body()) != 1 {
		t.Errorf("sig0 body = %v", s0.Body())
	}

	// Triple 2: the wrapped input yields one unnamed Integer param. The
	// literal output declares ONE return of the literal's kind, carrying
	// the literal as the return PATTERN — nothing is spliced onto the
	// body, which stays exactly as written.
	s1 := info.Signatures[1]
	if len(s1.Params) != 1 || s1.Params[0].Name != "" || !s1.Params[0].Type.Equal(TInteger) {
		t.Errorf("sig1 params = %+v", s1.Params)
	}
	b1 := s1.Body()
	if len(b1) != 1 {
		t.Fatalf("sig1 body = %v (want [zz-b], unspliced)", b1)
	}
	if len(s1.Returns) != 1 || !s1.Returns[0].Equal(TInteger) {
		t.Errorf("sig1 returns = %v (want [Integer])", s1.Returns)
	}
	if len(s1.ReturnPatterns) != 1 || s1.ReturnPatterns[0] == nil {
		t.Fatalf("sig1 return patterns = %v", s1.ReturnPatterns)
	}
	if n, aerr := AsInteger(*s1.ReturnPatterns[0]); aerr != nil || n != 42 {
		t.Errorf("sig1 return pattern = %v (%v)", *s1.ReturnPatterns[0], aerr)
	}

	// Triple 3: two literal constraints, one per declared return. The
	// String literal names no type, so it lands as a literal pattern
	// rather than a failed type-name lookup.
	s2 := info.Signatures[2]
	b2 := s2.Body()
	if len(b2) != 1 {
		t.Fatalf("sig2 body = %v (want [m], unspliced)", b2)
	}
	if len(s2.Returns) != 2 || !s2.Returns[0].Equal(TInteger) || !s2.Returns[1].Equal(TString) {
		t.Errorf("sig2 returns = %v (want [Integer String])", s2.Returns)
	}
	if len(s2.ReturnPatterns) != 2 || s2.ReturnPatterns[0] == nil || s2.ReturnPatterns[1] == nil {
		t.Fatalf("sig2 return patterns = %v", s2.ReturnPatterns)
	}
	if str, aerr := AsString(*s2.ReturnPatterns[1]); aerr != nil || str != "ok" {
		t.Errorf("sig2 return pattern 1 = %v (%v)", *s2.ReturnPatterns[1], aerr)
	}

	// A capitalised parameter word that names no type is a ParseFnParams
	// error (a lowercase one is an untyped param).
	_, perr = ParseFnDef(r, []Value{
		fndefStage5List(NewWord("ZzNotAType")), NewWord("Integer"), fndefStage5List(),
	})
	if perr == nil || !strings.Contains(perr.Error(), "invalid type") {
		t.Errorf("bad param type must error, got %v", perr)
	}

	// An empty list parses to an empty FnDefInfo.
	empty, perr := ParseFnDef(r, nil)
	if perr != nil || len(empty.Signatures) != 0 {
		t.Errorf("empty list: %+v, %v", empty, perr)
	}
}

func TestFnDefStage5ParseFnUndefSpec(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	info, uerr := ParseFnUndefSpec(r, []Value{
		// Pair 1: NON-list input (auto-wrapped).
		NewWord("Integer"), NewWord("Integer"),
		// Pair 2: named param, list output.
		fndefStage5List(NewWord("s:String")), fndefStage5List(NewWord("String")),
	})
	if uerr != nil {
		t.Fatalf("ParseFnUndefSpec: %v", uerr)
	}
	if len(info.Sigs) != 2 {
		t.Fatalf("want 2 sig specs, got %d", len(info.Sigs))
	}
	if len(info.Sigs[0].Params) != 1 || !info.Sigs[0].Params[0].Type.Equal(TInteger) ||
		len(info.Sigs[0].Returns) != 1 || !info.Sigs[0].Returns[0].Equal(TInteger) {
		t.Errorf("spec0 = %+v", info.Sigs[0])
	}
	if len(info.Sigs[1].Params) != 1 || info.Sigs[1].Params[0].Name != "s" ||
		len(info.Sigs[1].Returns) != 1 || !info.Sigs[1].Returns[0].Equal(TString) {
		t.Errorf("spec1 = %+v", info.Sigs[1])
	}

	// The ParseFnReturns error arm.
	_, uerr = ParseFnUndefSpec(r, []Value{
		fndefStage5List(NewWord("Integer")), NewWord("zz-not-a-type"),
	})
	if uerr == nil {
		t.Error("an unresolvable output type must error")
	}

	// The ParseFnParams error arm: a capitalised name that is no type (a
	// lowercase bare word is an untyped param, design/IMMUTABLE-DEF.1.md §2.3).
	_, uerr = ParseFnUndefSpec(r, []Value{
		fndefStage5List(NewWord("ZzNotAType")), NewWord("Integer"),
	})
	if uerr == nil {
		t.Error("an unresolvable input type must error")
	}
}
