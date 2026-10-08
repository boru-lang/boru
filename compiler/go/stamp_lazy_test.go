package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func lazyStampReg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// A fn value whose one sig has the given body; Registry nil (a main-program
// value with no foreign home).
func lazyFnValue(body ...core.Value) (core.FnDefInfo, *core.Signature) {
	fd := core.FnDefInfo{Name: "lz", Signatures: []core.Signature{{
		Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru(body),
	}}}
	return fd, &fd.Signatures[0]
}

// The lazy stamp's gates, in order: a Go-backed sig has nothing to stamp; a
// disarmed registry stamps nothing and leaves the slot untouched (a later
// armed application may still stamp); an ineligible body — a flow sentinel,
// a registry-mutating word — is declined and REMEMBERED on the value.
func TestLazyStampFnSigGates(t *testing.T) {
	r := lazyStampReg(t)
	goSig := &core.Signature{Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, nil
	})}
	if ref := LazyStampFnSig(r, core.FnDefInfo{Signatures: []core.Signature{*goSig}}, goSig, core.SrcPos{}); ref != nil {
		t.Error("a Go-backed sig has no body to stamp")
	}
	fd, sig := lazyFnValue(core.NewWord("n"))
	if ref := LazyStampFnSig(r, fd, sig, core.SrcPos{}); ref != nil {
		t.Error("a disarmed registry stamps nothing")
	}
	if sig.Impl.(*core.BoruImpl).Compiled() != nil {
		t.Error("a disarmed attempt must leave the slot untouched — an armed application may still stamp")
	}
	r.EnableRuntimeStamping()
	defer r.DisableRuntimeStamping()
	hazard, hsig := lazyFnValue(core.NewWord("def"), core.NewWord("T"), core.NewInteger(1), core.NewWord("n"))
	if ref := LazyStampFnSig(r, hazard, hsig, core.SrcPos{}); ref != nil {
		t.Error("a registry-mutating body is never stamped lazily")
	}
	if slot := hsig.Impl.(*core.BoruImpl).Compiled(); slot == nil {
		t.Error("the hazard decline must be remembered on the value")
	} else if _, isRef := slot.(*CompiledFnRef); isRef {
		t.Error("the decline marker is not a ref")
	}
	if CompiledRef(hsig) != nil {
		t.Error("CompiledRef reads a decline marker as no ref")
	}
	// A sig whose impl is not in the value's own table (a foreign copy) declines
	// and is remembered too.
	stray := &core.Signature{Impl: core.Boru([]core.Value{core.NewWord("n")})}
	if ref := LazyStampFnSig(r, fd, stray, core.SrcPos{}); ref != nil || stray.Impl.(*core.BoruImpl).Compiled() == nil {
		t.Error("a sig outside the value's own table declines, remembered")
	}
}

// A body the detached compile declines is compiled ONCE: the second
// application finds the decline on the value and the stamp ledger does not
// grow. An unknown word in the body is the decline (undefined_word in the
// body analysis) on a bare registry.
func TestLazyStampFnSigRemembersDecline(t *testing.T) {
	r := lazyStampReg(t)
	r.EnableRuntimeStamping()
	defer r.DisableRuntimeStamping()
	fd, sig := lazyFnValue(core.NewWord("no-such-word"))
	if ref := LazyStampFnSig(r, fd, sig, core.SrcPos{}); ref != nil {
		t.Fatal("a body with an unknown word must decline")
	}
	first := len(r.StampEvents())
	if first == 0 {
		t.Fatal("the decline is recorded in the stamp ledger")
	}
	if ref := LazyStampFnSig(r, fd, sig, core.SrcPos{}); ref != nil {
		t.Fatal("a remembered decline stays declined")
	}
	if got := len(r.StampEvents()); got != first {
		t.Fatalf("the second application must not compile again: ledger %d -> %d", first, got)
	}
}

// A body that compiles is stamped ONCE and the ref is memoised on the value's
// shared impl: the second application returns the same ref without a compile.
func TestLazyStampFnSigMemoisesRef(t *testing.T) {
	r := lazyStampReg(t)
	r.EnableRuntimeStamping()
	defer r.DisableRuntimeStamping()
	fd, sig := lazyFnValue(core.NewWord("n"))
	ref := LazyStampFnSig(r, fd, sig, core.SrcPos{})
	if ref == nil || ref.Prog == nil {
		t.Fatalf("the body compiles: ref=%v", ref)
	}
	ledger := len(r.StampEvents())
	if again := LazyStampFnSig(r, fd, sig, core.SrcPos{}); again != ref {
		t.Fatal("the second application must find the same ref")
	}
	if got := len(r.StampEvents()); got != ledger {
		t.Fatalf("the second application must not compile again: ledger %d -> %d", ledger, got)
	}
	if CompiledRef(sig) != ref {
		t.Fatal("CompiledRef reads the memoised ref")
	}
}
