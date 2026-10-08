package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// cover_pr505_main_test.go pins three VM-side helpers on inputs their
// contracts name: the lazy first-application stamp without a signature, the
// designed defer lending its position to an unpositioned alt, and the
// closure matcher over a unit that recorded no param contract.

// TestCoverPR505MainLazyStampNeedsASig: the compiled runtime's first-
// application stamp (CompiledRuntime.LazyStamp) answers "no unit" for a nil
// signature rather than handing compiler.LazyStampFnSig a nil to dereference,
// and for a Go-implemented signature, which has no boru body to stamp; a
// boru body on an armed registry stamps, and the sig then carries its unit.
func TestCoverPR505MainLazyStampNeedsASig(t *testing.T) {
	rt := vmCompiledRuntime{}
	r := stampReg(t)
	r.EnableRuntimeStamping()
	defer r.DisableRuntimeStamping()
	if rt.LazyStamp(r, core.FnDefInfo{Name: "lz"}, nil, core.SrcPos{}) {
		t.Error("a nil signature has no unit to stamp")
	}
	goSig := core.Signature{Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, nil
	})}
	native := core.FnDefInfo{Name: "lz", Signatures: []core.Signature{goSig}}
	if rt.LazyStamp(r, native, &native.Signatures[0], core.SrcPos{}) {
		t.Error("a Go-implemented signature has no boru body to stamp")
	}
	boru := core.FnDefInfo{Name: "lz", Signatures: []core.Signature{{
		Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewWord("n")}),
	}}}
	if !rt.LazyStamp(r, boru, &boru.Signatures[0], core.SrcPos{}) || compiler.CompiledRef(&boru.Signatures[0]) == nil {
		t.Error("a boru body on an armed registry stamps, and the sig carries the unit")
	}
}

// TestCoverPR505MainVmDeferAltLendsPosition: a defer site's alt is built over
// values, not tokens, so when it carries no position of its own it takes the
// defer's (the positioned instruction's debug entry) — the interpreter's raise
// points at the token, and so must the answer that replaces it. An alt with
// its own position keeps it, and a defer with no position has none to lend.
func TestCoverPR505MainVmDeferAltLendsPosition(t *testing.T) {
	r := covRegistry(t, nil)
	dbg := []core.SrcPos{{Row: 3, Col: 7, Src: "fold"}}

	alt := &core.BoruError{Code: "signature_error", Detail: "d"}
	err := vmDeferAlt(r, dbg, 0, "vm:poly-no-match", "x", alt)
	if ae, ok := err.(*core.BoruError); !ok || ae.DeferAlt != alt {
		t.Fatalf("the alt must ride the defer, got %v", err)
	}
	if alt.Row != 3 || alt.Col != 7 || alt.Src != "fold" {
		t.Errorf("an unpositioned alt takes the defer's position, got %d:%d %q", alt.Row, alt.Col, alt.Src)
	}

	own := &core.BoruError{Code: "signature_error", Detail: "d", Row: 9, Col: 2, Src: "own"}
	_ = vmDeferAlt(r, dbg, 0, "vm:poly-no-match", "x", own)
	if own.Row != 9 || own.Col != 2 || own.Src != "own" {
		t.Errorf("a positioned alt keeps its own position, got %d:%d %q", own.Row, own.Col, own.Src)
	}

	bare := &core.BoruError{Code: "signature_error", Detail: "d"}
	_ = vmDeferAlt(r, dbg, 5, "vm:poly-no-match", "x", bare)
	if bare.Row != 0 || bare.Col != 0 {
		t.Errorf("a defer past the debug table has no position to lend, got %d:%d", bare.Row, bare.Col)
	}
}

// TestCoverPR505MainClosureMatchesArgsNeedsContract: the closure matcher asks
// a unit's OWN declared signature, so a unit that recorded no param contract
// (a token body) matches nothing — guessing Any would apply where the
// interpreter declines — while a lambda's unit matches by its declared types.
func TestCoverPR505MainClosureMatchesArgsNeedsContract(t *testing.T) {
	token := &compiler.CompiledFn{Name: "each$body", NArgs: 1}
	if closureMatchesArgs(token, []core.Value{core.NewInteger(1)}) {
		t.Error("a unit with no param contract never matches")
	}
	lam := &compiler.CompiledFn{Name: "fnval$body", NArgs: 1, Params: []*core.Type{core.TInteger}, Lambda: true}
	if !closureMatchesArgs(lam, []core.Value{core.NewInteger(1)}) {
		t.Error("an Integer matches the declared Integer param")
	}
	if closureMatchesArgs(lam, []core.Value{core.NewString("s")}) {
		t.Error("a String does not match the declared Integer param")
	}
}
