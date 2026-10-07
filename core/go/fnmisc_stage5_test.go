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
	// An enclosing fn's VAR is captured as one (CapturedBinding.Var): the
	// closure reads its value, and the var word refuses to assign it.
	InstallVar(r, "zz-var", NewInteger(7), nil)
	defer r.Defs.Pop("zz-var")

	sig := &FnSig{Impl: Boru([]Value{
		NewWord("zz-cap"), NewWord("zz-cap"), // duplicate reference: deduped
		NewWord("zz-unbound"), // unbound: skipped
		NewWord("zz-var"),
	})}
	caps := ComputeCaptures(r, sig)
	if len(caps) != 2 || caps[0].Name != "zz-cap" || caps[1].Name != "zz-var" {
		t.Fatalf("captures = %+v, want [zz-cap zz-var]", caps)
	}
	if n, aerr := AsInteger(caps[0].Value); aerr != nil || n != 5 || caps[0].Var {
		t.Errorf("captured def = %v (%v) var=%v, want 5, not a var", caps[0].Value, aerr, caps[0].Var)
	}
	// A CODE body's capture of the var stays unmarked (the body is the frame
	// that wrote it); a fn VALUE's is marked — its own frame.
	if n, aerr := AsInteger(caps[1].Value); aerr != nil || n != 7 || caps[1].Var {
		t.Errorf("code-body capture of the var = %v (%v) var=%v, want 7, unmarked", caps[1].Value, aerr, caps[1].Var)
	}
	vcaps := ComputeFnValueCaptures(r, sig)
	if len(vcaps) != 2 || vcaps[0].Var || !vcaps[1].Var {
		t.Errorf("fn-value captures = %+v, want zz-cap plain and zz-var marked Var", vcaps)
	}
}

// TestInstallCapturedBinding: a capture enters the callee frame as a frame
// binding; a captured VAR is marked Var as well, the Frame-and-Var pair the
// var word refuses to assign (a var of another frame, §2.3). A plain
// capture is Frame only.
func TestInstallCapturedBinding(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	InstallCapturedBinding(r, CapturedBinding{Name: "zz-cv", Value: NewInteger(1), Var: true})
	if e, ok := r.Defs.TopEntry("zz-cv"); !ok || !e.Frame || !e.Var || e.VarType != nil {
		t.Errorf("a captured var must be Frame and Var: %+v (%v)", e, ok)
	}
	InstallCapturedBinding(r, CapturedBinding{Name: "zz-cd", Value: NewInteger(2)})
	if e, ok := r.Defs.TopEntry("zz-cd"); !ok || !e.Frame || e.Var {
		t.Errorf("a captured def must be Frame only: %+v (%v)", e, ok)
	}
}

func TestFnMiscStage5CollectBodyLocalDefs(t *testing.T) {
	locals := map[string]bool{}
	CollectBodyLocalDefs(nil, []Value{
		NewWord("def"), NewWord("j"), NewInteger(1),
		NewWord("def"), NewInteger(9), // def followed by a non-word: no local
		NewWord("var"), NewWord("a"), NewInteger(0), // a var the body declares binds a name like def
		NewWord("var"), NewWord("b"), NewWord("zz-body"),
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

	// `var NAME …` over a var VISIBLE on the registry is bindVar's assignment
	// of that cell, not a binding of the body's own: it is no local (the
	// each body `[var s (s add 1)]` assigns the enclosing fn's `s`, var.tsv
	// L36). A `def` of the same name still is — def rebinds — and a var the
	// registry knows only as a def is a declaration, as is one it does not
	// know at all. The nil registry above knows no vars.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	InstallVar(r, "s", NewInteger(0), nil)
	r.Defs.Push("d", NewInteger(1))
	locals = map[string]bool{}
	CollectBodyLocalDefs(r, []Value{
		NewWord("var"), NewWord("s"), NewInteger(1), // assigns the visible var: no local
		NewWord("var"), NewWord("d"), NewInteger(2), // shadows a def with a new var: local
		NewWord("var"), NewWord("t"), NewInteger(3), // declares: local
		NewList([]Value{NewWord("var"), NewWord("s"), NewInteger(4)}), // nested assignment: no local
		NewWord("def"), NewWord("s"), NewInteger(5), // def rebinds: local
	}, locals)
	if locals["s"] != true || !locals["d"] || !locals["t"] || len(locals) != 3 {
		t.Errorf("registry-aware locals = %v, want d, t and s (via def) only", locals)
	}
	locals = map[string]bool{}
	CollectBodyLocalDefs(r, []Value{NewWord("var"), NewWord("s"), NewInteger(1)}, locals)
	if len(locals) != 0 {
		t.Errorf("a lone assignment of the visible var collected %v, want nothing", locals)
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
