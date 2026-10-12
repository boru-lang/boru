package eng

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in fnsig.go — the pattern-compatibility arms of
// FnSigSatisfiesSpec, its return-arity guard, and the FnUndefMatchesFnDef
// payload / empty-constraint guards. Pure calls with crafted specs. Per
// design/TEST-SEAMS.10.md.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestW8FnSigSatisfiesSpecPatternAbsentOnCandidate(t *testing.T) {
	pat := core.NewTypeLiteral(core.TInteger)
	spec := core.FnSigSpec{Params: []core.FnParam{{Type: core.TInteger, Pattern: &pat}}}
	sig := core.FnSig{Params: []core.FnParam{{Type: core.TInteger}}} // sg.Pattern == nil
	if !core.FnSigSatisfiesSpec(sig, spec) {
		t.Fatal("a candidate without a pattern satisfies a spec that has one")
	}
}

func TestW8FnSigSatisfiesSpecPatternUnifyFail(t *testing.T) {
	spat := core.NewTypeLiteral(core.TInteger)
	gpat := core.NewTypeLiteral(core.TString)
	spec := core.FnSigSpec{Params: []core.FnParam{{Type: core.TAny, Pattern: &spat}}}
	sig := core.FnSig{Params: []core.FnParam{{Type: core.TAny, Pattern: &gpat}}}
	if core.FnSigSatisfiesSpec(sig, spec) {
		t.Fatal("incompatible patterns must not satisfy")
	}
}

func TestW8FnSigSatisfiesSpecReturnArity(t *testing.T) {
	spec := core.FnSigSpec{Returns: []*core.Type{core.TInteger}}
	sig := core.FnSig{} // zero returns vs one expected
	if core.FnSigSatisfiesSpec(sig, spec) {
		t.Fatal("return-arity mismatch must not satisfy")
	}
}

func TestW8FnUndefMatchesFnDefGuards(t *testing.T) {
	// undef value is not a FnUndefInfo.
	if core.FnUndefMatchesFnDef(core.NewInteger(1), core.NewValueRaw(core.TFunction, &core.FnDefInfo{})) {
		t.Fatal("non-FnUndef constraint value must not match")
	}
	// fn value is not a FnDefInfo.
	undef := core.NewFnUndef(core.FnUndefInfo{Sigs: []core.FnSigSpec{{Returns: []*core.Type{core.TInteger}}}})
	if core.FnUndefMatchesFnDef(undef, core.NewInteger(1)) {
		t.Fatal("non-FnDef candidate value must not match")
	}
	// Empty constraint trivially matches any function.
	emptyUndef := core.NewFnUndef(core.FnUndefInfo{})
	if !core.FnUndefMatchesFnDef(emptyUndef, core.NewValueRaw(core.TFunction, &core.FnDefInfo{})) {
		t.Fatal("an empty FnUndef constraint matches any function")
	}
}
