package eng

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// FnAnalysisKey carries a per-registry scope id (design/module-fn-
// checkstate-ownership.1.md §5a) so that a module sub-registry's fn cannot
// alias a same-named, same-positioned parent fn once a check pass is shared
// across registries. These tests pin both halves of that contract:
// identical scope → identical key (memoisation / recursion detection
// intact), distinct scope → distinct key (the §4 cross-registry collision
// cannot occur).

// sameKeyInputs returns a fixed (name, args, captures, body) tuple so the
// only variable across calls is the scope id.
func sameKeyInputs() (string, []core.Value, []core.CapturedBinding, []core.Value) {
	name := "decide"
	args := []core.Value{core.NewCarrier(core.TWord), core.NewCarrier(core.TMap)}
	body := []core.Value{core.NewWord("apply-op"), core.NewInteger(1)}
	return name, args, nil, body
}

func TestFnAnalysisKeyScopeDisambiguates(t *testing.T) {
	name, args, caps, body := sameKeyInputs()

	// Positive: same scope id + identical inputs → identical key. This is
	// what keeps memoisation and in-flight recursion detection working —
	// the same construction re-analysed must hit the same memo slot.
	k1 := check.FnAnalysisKey(7, name, args, caps, body)
	k1again := check.FnAnalysisKey(7, name, args, caps, body)
	if k1 != k1again {
		t.Fatalf("same scope + same inputs must yield the same key:\n  %q\n  %q", k1, k1again)
	}

	// Negative: a DIFFERENT scope id with otherwise-identical inputs (same
	// name, arg types, body, and source position) must yield a DIFFERENT
	// key. Without the scope discriminator these collide — exactly the
	// parent-vs-module aliasing that broke the prior attempt (.0 §4).
	k2 := check.FnAnalysisKey(8, name, args, caps, body)
	if k1 == k2 {
		t.Fatalf("distinct scope ids must not collide; both produced %q", k1)
	}
}

func TestFnAnalysisKeyScopeOnlyDifference(t *testing.T) {
	// Guard that the scope id is the ONLY thing separating the two keys —
	// stripping the leading "<scope>#" prefix must make them equal, proving
	// the disambiguation is the prefix and nothing about the inputs drifted.
	name, args, caps, body := sameKeyInputs()
	k7 := check.FnAnalysisKey(7, name, args, caps, body)
	k8 := check.FnAnalysisKey(8, name, args, caps, body)

	strip := func(s string) string {
		for i := 0; i < len(s); i++ {
			if s[i] == '#' {
				return s[i:]
			}
		}
		return s
	}
	if strip(k7) != strip(k8) {
		t.Fatalf("only the scope prefix should differ:\n  %q\n  %q", k7, k8)
	}
}

func TestNewRegistryScopeIDsDistinct(t *testing.T) {
	r1, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r2, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// Each freshly constructed registry (e.g. a parent and a module's
	// sub-registry built via DefaultRegistry → NewRegistry) gets a distinct
	// scope id, so their fn analyses cannot alias.
	if r1.AnalysisScopeID() == r2.AnalysisScopeID() {
		t.Fatalf("distinct registries must have distinct scope ids; both %d", r1.AnalysisScopeID())
	}
	// A nil registry is scope 0 (no panic) — the documented contract.
	var rnil *core.Registry
	if rnil.AnalysisScopeID() != 0 {
		t.Fatalf("nil registry scope id must be 0, got %d", rnil.AnalysisScopeID())
	}
}

// A root-node carrier (None / Any / Never) has a nil Parent because it IS its
// own lattice node. FnAnalysisKey and the captures loop dereferenced the arg's
// Parent unguarded, so a `none` fold-seed promoted to a dynamic carrier (tst's
// `none entries [… tst-insert] fold`) panicked the whole check pass. Assert the
// key builds without panicking and still discriminates the None arg.
func TestFnAnalysisKeyNilParentArg(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FnAnalysisKey panicked on a nil-Parent (None root) arg: %v", r)
		}
	}()
	noneArm := core.NewDynamicCarrierValue(core.NewTypeLiteral(core.TNone)) // Parent == nil
	if noneArm.Parent() != nil {
		t.Fatalf("test fixture invalid: expected a nil-Parent None carrier, got Parent=%v", noneArm.Parent())
	}
	args := []core.Value{noneArm, core.NewCarrier(core.TMap)}
	caps := []core.CapturedBinding{{Name: "seed", Value: noneArm}}
	body := []core.Value{core.NewWord("build")}

	kNone := check.FnAnalysisKey(1, "ins", args, caps, body)
	if kNone == "" {
		t.Fatalf("FnAnalysisKey returned an empty key")
	}
	// The None arg must still discriminate: a Map-typed first arg yields a
	// different key, proving the nil-Parent fallback names the type (not a
	// blanket constant).
	kMap := check.FnAnalysisKey(1, "ins", []core.Value{core.NewCarrier(core.TMap), core.NewCarrier(core.TMap)}, nil, body)
	if kNone == kMap {
		t.Fatalf("nil-Parent None arg must key distinctly from a Map arg; both %q", kNone)
	}
}
