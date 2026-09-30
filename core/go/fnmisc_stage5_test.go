package core

import (
	"testing"
)

// Stage-5 coverage for fnsig.go (FnSigMatchesSpec / FnSigSatisfiesSpec /
// FnUndefMatchesFnDef / FnDefHasSig), the residual fn_capture.go walker
// arms (nested-fn / interp-string / xml-interp / malformed-map skip,
// capture dedup, unbound-word skip, CollectBodyLocalDefs, MergeCaptures)
// and fn_frame.go's AsFrameOpen error arm.

func TestFnMiscStage5FnSigMatchesSpec(t *testing.T) {
	sig := FnSig{Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TInteger}}

	if FnSigMatchesSpec(sig, FnSigSpec{}) {
		t.Error("arity mismatch must not match")
	}
	if FnSigMatchesSpec(sig, FnSigSpec{Params: []FnParam{{Type: TString}}, Returns: []*Type{TInteger}}) {
		t.Error("param-type mismatch must not match")
	}
	if FnSigMatchesSpec(sig, FnSigSpec{Params: []FnParam{{Type: TInteger}}}) {
		t.Error("returns-count mismatch must not match")
	}
	if FnSigMatchesSpec(sig, FnSigSpec{Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TString}}) {
		t.Error("return-type mismatch must not match")
	}
	if !FnSigMatchesSpec(sig, FnSigSpec{Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TInteger}}) {
		t.Error("exact shape must match")
	}
}

func TestFnMiscStage5FnSigSatisfiesSpec(t *testing.T) {
	pat1 := NewInteger(1)
	pat1b := NewInteger(1)
	pat2 := NewInteger(2)

	// Contravariant input (spec Integer ⊆ sig Number), covariant return
	// (sig Integer ⊆ spec Number), optional aligned.
	okSig := FnSig{Params: []FnParam{{Type: TNumber, Optional: true}}, Returns: []*Type{TInteger}}
	okSpec := FnSigSpec{Params: []FnParam{{Type: TInteger, Optional: true}}, Returns: []*Type{TNumber}}
	if !FnSigSatisfiesSpec(okSig, okSpec) {
		t.Error("variance-compatible sig must satisfy the spec")
	}

	if FnSigSatisfiesSpec(okSig, FnSigSpec{}) {
		t.Error("arity mismatch must not satisfy")
	}
	if FnSigSatisfiesSpec(
		FnSig{Params: []FnParam{{Type: TInteger}}},
		FnSigSpec{Params: []FnParam{{Type: TNumber}}},
	) {
		t.Error("contravariance violation must not satisfy")
	}
	if FnSigSatisfiesSpec(
		FnSig{Params: []FnParam{{Type: TNumber}}},
		FnSigSpec{Params: []FnParam{{Type: TInteger, Optional: true}}},
	) {
		t.Error("spec-optional with required candidate must not satisfy")
	}

	// Spec pattern with an unconstrained candidate: satisfied.
	if !FnSigSatisfiesSpec(
		FnSig{Params: []FnParam{{Type: TInteger}}},
		FnSigSpec{Params: []FnParam{{Type: TInteger, Pattern: &pat1}}},
	) {
		t.Error("a pattern-free candidate satisfies a spec pattern")
	}
	// Unifiable patterns: satisfied.
	if !FnSigSatisfiesSpec(
		FnSig{Params: []FnParam{{Type: TInteger, Pattern: &pat1b}}},
		FnSigSpec{Params: []FnParam{{Type: TInteger, Pattern: &pat1}}},
	) {
		t.Error("unifiable patterns must satisfy")
	}
	// Conflicting patterns: not satisfied.
	if FnSigSatisfiesSpec(
		FnSig{Params: []FnParam{{Type: TInteger, Pattern: &pat2}}},
		FnSigSpec{Params: []FnParam{{Type: TInteger, Pattern: &pat1}}},
	) {
		t.Error("conflicting patterns must not satisfy")
	}

	if FnSigSatisfiesSpec(
		FnSig{Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TInteger}},
		FnSigSpec{Params: []FnParam{{Type: TInteger}}},
	) {
		t.Error("returns-count mismatch must not satisfy")
	}
	if FnSigSatisfiesSpec(
		FnSig{Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TNumber}},
		FnSigSpec{Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TInteger}},
	) {
		t.Error("covariance violation must not satisfy")
	}
}

func TestFnMiscStage5FnUndefMatchesFnDef(t *testing.T) {
	fnVal := NewFunction(FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Type: TNumber}}, Returns: []*Type{TInteger},
	}}})
	okUndef := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{
		Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TNumber},
	}}})
	badUndef := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{
		Params: []FnParam{{Type: TString}}, Returns: []*Type{TNumber},
	}}})

	if !FnUndefMatchesFnDef(okUndef, fnVal) {
		t.Error("a satisfiable constraint must match")
	}
	if FnUndefMatchesFnDef(badUndef, fnVal) {
		t.Error("an unsatisfiable constraint must not match")
	}
	if FnUndefMatchesFnDef(NewInteger(1), fnVal) {
		t.Error("a non-FnUndef constraint must not match")
	}
	if FnUndefMatchesFnDef(okUndef, NewInteger(1)) {
		t.Error("a non-function candidate must not match")
	}
}

func TestFnMiscStage5WalkBodyValueSkipArms(t *testing.T) {
	var names []string
	cb := func(w WordInfo, _ Value) { names = append(names, w.Name) }

	body := []Value{
		// Nested FnDefInfo: opaque, never descended.
		NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{{
			Impl: Boru([]Value{NewWord("zz-hidden")}), BarrierPos: BarrierAllForward,
		}}}),
		// Interp string: expression parts are walked.
		NewInterpString([]InterpPart{{Lit: "a"}, {Expr: []Value{NewWord("zz-in-interp")}}}),
		// Interpolated XML skeleton: attr and child exprs are walked.
		NewXmlInterp(XmlTmpl{
			Tag:  "p",
			Attr: []XmlAttrTmpl{{Name: "c", Parts: []InterpPart{{Expr: []Value{NewWord("zz-in-attr")}}}}},
			Cren: []XmlCren{{Kind: XmlCrenExpr, Expr: []Value{NewWord("zz-in-xml")}}},
		}),
		// A TMap-typed value with a non-map payload: skipped, no panic.
		NewValueRaw(TMap, StrPayload{S: "not-a-map"}),
	}
	WalkBodyWords(body, cb)

	want := []string{"zz-in-interp", "zz-in-attr", "zz-in-xml"}
	if len(names) != len(want) {
		t.Fatalf("walked words = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("walked[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestFnMiscStage5ComputeCapturesDupAndUnbound(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("zz-cap", NewInteger(5))
	defer r.Defs.Pop("zz-cap")
	r.PushFnBaseline(map[string]int{})
	defer r.PopFnBaseline()

	sig := &FnSig{Impl: Boru([]Value{
		NewWord("zz-cap"), NewWord("zz-cap"), // duplicate reference: deduped
		NewWord("zz-unbound"), // unbound: skipped
	})}
	caps := ComputeCaptures(r, sig)
	if len(caps) != 1 || caps[0].Name != "zz-cap" {
		t.Fatalf("captures = %+v, want [zz-cap]", caps)
	}
	if n, aerr := AsInteger(caps[0].Value); aerr != nil || n != 5 {
		t.Errorf("captured value = %v (%v), want 5", caps[0].Value, aerr)
	}
}

func TestFnMiscStage5CollectBodyLocalDefs(t *testing.T) {
	locals := map[string]bool{}
	CollectBodyLocalDefs([]Value{
		NewWord("def"), NewWord("j"), NewInteger(1),
		NewWord("def"), NewInteger(9), // def followed by a non-word: no local
		NewWord("var"), NewList([]Value{
			NewList([]Value{NewWord("a"), NewWord("b"), NewInteger(0)}),
			NewWord("zz-body"),
		}),
		NewList([]Value{NewWord("def"), NewWord("k")}),      // list recursion
		NewParenExpr([]Value{NewWord("def"), NewWord("p")}), // paren recursion
		NewWord("def"), // trailing def with no name token
	}, locals)

	for _, name := range []string{"j", "a", "b", "k", "p"} {
		if !locals[name] {
			t.Errorf("local %q not collected: %v", name, locals)
		}
	}
	if locals["zz-body"] || locals["def"] || locals["var"] {
		t.Errorf("unexpected locals collected: %v", locals)
	}
}

func TestFnMiscStage5MergeCaptures(t *testing.T) {
	one := []CapturedBinding{{Name: "a", Value: NewInteger(1)}}
	got := MergeCaptures([][]CapturedBinding{one})
	if len(got) != 1 || got[0].Name != "a" {
		t.Errorf("single-list merge = %+v", got)
	}
	if MergeCaptures(nil) != nil {
		t.Error("no lists must merge to nil")
	}
	if MergeCaptures([][]CapturedBinding{nil, nil}) != nil {
		t.Error("all-empty lists must merge to nil")
	}
}

func TestFnMiscStage5AsFrameOpenError(t *testing.T) {
	if _, err := AsFrameOpen(NewInteger(1)); err == nil {
		t.Error("a non-frame value must error")
	}
	meta := &FnFrameMeta{}
	info, err := AsFrameOpen(NewFrameOpen(meta))
	if err != nil || info.Meta != meta {
		t.Errorf("frame-open roundtrip = %+v, %v", info, err)
	}
}
