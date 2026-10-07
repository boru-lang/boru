package check

// The live-depth oracle (test/go/langspec/bind_ledger_live_oracle_test.go)
// caught two ways the pass's registry drifted from its own ledger, and both
// fixes live in this module. These pin them directly:
//
//   - a narrowing push is an ANALYSIS artifact (the same runtime value under
//     a tighter static bound) and must not outlive the pass — left in place
//     it shadowed the real binding for every later Run on the same instance
//     (module-io.tsv:223 — a later read of `l` answered the leaked dynamic
//     carrier instead of the bound Lock);
//   - AnalyseLoopBody's final joined pushes ARE the post-loop environment
//     the pass leaves, so the bind ledger records them, exactly as
//     InstallJoinedDefs records a branch join.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A narrowing whose entry is still on top at pass end is popped: the pass
// leaves the binding it found, not its own refinement of it.
func TestNarrowingPushPoppedAtPassEnd(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()

	v := core.NewDynamicCarrier(core.TAny)
	v.SetDynFrom("npq")
	r.Defs.Push("npq", v)

	sig := &core.Signature{Args: []*core.Type{core.TList}, BarrierPos: 1}
	narrowDynamicUses(r, "", sig, []core.Value{v})
	if r.Defs.Depth("npq") != 2 {
		t.Fatalf("the narrowing must push during the pass: depth = %d, want 2", r.Defs.Depth("npq"))
	}

	done()
	if r.Defs.Depth("npq") != 1 {
		t.Fatalf("the narrowing must be popped at pass end: depth = %d, want 1", r.Defs.Depth("npq"))
	}
	got, _ := r.Defs.Top("npq")
	if got.Parent != nil && got.Parent.Equal(core.TList) {
		t.Error("pass end must restore the ORIGINAL binding, not keep the narrowed one")
	}
}

// A narrowing buried under a later real binding is left alone: popping by
// name would remove the real binding, and removing the buried entry would
// shift every ledger depth recorded above it. The later binding here is
// deliberately a carrier VALUE-EQUAL to the narrowing (Codex P1, PR #418):
// value equality alone would pop the real binding and re-expose the
// narrowing beneath — the depth half of the guard is what rules it out.
func TestNarrowingPopSkipsBuriedEntry(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()

	v := core.NewDynamicCarrier(core.TAny)
	v.SetDynFrom("nbq")
	r.Defs.Push("nbq", v)
	sig := &core.Signature{Args: []*core.Type{core.TList}, BarrierPos: 1}
	narrowDynamicUses(r, "", sig, []core.Value{v})

	// A later check-mode rebind lands on top of the narrowing, binding a
	// carrier ValuesEqual cannot tell apart from the narrowing's own value.
	narrowedTop, _ := r.Defs.Top("nbq")
	r.Defs.Push("nbq", narrowedTop)
	depth := r.Defs.Depth("nbq")

	done()
	if r.Defs.Depth("nbq") != depth {
		t.Fatalf("a buried narrowing must be left alone even under a value-equal rebind: depth = %d, want %d",
			r.Defs.Depth("nbq"), depth)
	}
}

// A loop body is a BLOCK (design/IMMUTABLE-DEF.1.md §2.1, phase 2): a def
// inside it ends with the iteration. Nothing of the body's is installed
// after the loop — no joined rebind of the enclosing binding, no fresh
// post-loop binding — and the ledger notes no post-loop install either
// (before the rule this test asserted the joined pushes at their live
// depths, 2 and 1).
func TestAnalyseLoopBodyLedgersJoinedInstalls(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterPushq(r)
	done := r.Check.Begin()
	defer done()

	r.Defs.Push("ljaccq", core.NewCarrier(core.TInteger))
	body := core.NewList([]core.Value{
		core.NewWord("pushq"), core.NewWord("ljaccq"), core.NewString("s"),
		core.NewWord("pushq"), core.NewWord("ljfreshq"), core.NewInteger(7),
	})
	AnalyseLoopBody(r, body, nil, nil, false)

	if r.Defs.Depth("ljaccq") != 1 {
		t.Errorf("the enclosing binding must stand alone after the loop: depth %d, want 1", r.Defs.Depth("ljaccq"))
	}
	if acc, _ := r.Defs.Top("ljaccq"); !acc.Parent.Equal(core.TInteger) {
		t.Errorf("post-loop bound = %v, want the enclosing Integer, unjoined", acc.Parent)
	}
	if r.Defs.Has("ljfreshq") {
		t.Error("a body-local binding must not be installed after the loop")
	}
	for _, tr := range r.Check.BindLedger {
		if tr.Kind == core.BindDef && (tr.Name == "ljaccq" || tr.Name == "ljfreshq") {
			t.Errorf("no post-loop install may be ledgered, got %+v", tr)
		}
	}
}

// The loop join applies the same narrowing-only skip as the branch join
// (core.InstallJoinedDefs): a body add whose ID equals the pre-loop binding's
// is the pass's own refinement — no join push, no carried slot, no ledger
// entry (Codex P2, PR #418).
func TestAnalyseLoopBodySkipsNarrowingOnlyAdds(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()

	pre := core.NewDynamicCarrier(core.TAny)
	pre.SetDynFrom("lnq")
	r.Defs.Push("lnq", pre)

	// `refq` stands in for narrowDynamicUses firing inside the body: it
	// pushes a SAME-ID refinement of the enclosing binding, exactly the
	// stack shape a typed-slot consumption leaves. Driving the push
	// directly keeps the test about the JOIN's skip arm rather than about
	// the narrowing's own trigger conditions (which live in their own
	// tests above).
	refined := core.NewDynamicCarrierValue(core.NewCarrier(core.TMap))
	refined.ID = pre.ID
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "refq",
		Signatures: []core.Signature{{
			Returns: []*core.Type{core.TInteger},
			Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
				r.Defs.Push("lnq", refined)
				return []core.Value{core.NewInteger(1)}, nil
			}, core.RunInCheck()),
		}},
	})
	base := len(r.Check.BindLedger)

	body := core.NewList([]core.Value{core.NewWord("refq")})
	AnalyseLoopBody(r, body, nil, nil, false)

	if d := r.Defs.Depth("lnq"); d != 1 {
		t.Fatalf("a narrowing-only body add must not be re-pushed post-loop: depth = %d, want 1", d)
	}
	for _, tr := range r.Check.BindLedger[base:] {
		if tr.Name == "lnq" {
			t.Fatalf("a narrowing-only body add must not be ledgered: %+v", tr)
		}
	}
}
