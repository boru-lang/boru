package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Unit coverage for the stamping half of the per-export module policy
// gate (NUR045). The gate's dispatch arms are exercised end-to-end from
// lang (policy_module_gate_test.go); these pin the stamper's own
// branches, several of which production never reaches today but which
// the contract promises: an id-less inline module stamps nothing, a nil
// namespace is skipped, and a wrapper without a sub-registry stops
// after its own signatures.
func TestStampModuleCallGatesIDLessModuleStampsNothing(t *testing.T) {
	fd := core.FnDefInfo{Name: "w", Signatures: []core.Signature{{BarrierPos: core.BarrierAllForward}}}
	om := core.NewOrderedMap()
	om.Set("w", core.NewFunction(fd))

	// An inline `module […]` has no stable policy-addressable id, so it
	// is stamped with "" and must come back untouched.
	core.StampModuleCallGates(map[string]*core.OrderedMap{"M": om}, "")

	got, _ := om.Get("w")
	if sigs := got.Data.(*core.FnDefInfo).Signatures; sigs[0].ModuleCall != nil {
		t.Errorf("an empty moduleRef must stamp nothing, got %+v", sigs[0].ModuleCall)
	}
}

func TestStampModuleCallGatesSkipsNilNamespace(t *testing.T) {
	fd := core.FnDefInfo{Name: "w", Signatures: []core.Signature{{BarrierPos: core.BarrierAllForward}}}
	live := core.NewOrderedMap()
	live.Set("w", core.NewFunction(fd))

	// A nil namespace beside a live one must be skipped, not panic, and
	// the live one must still be stamped.
	core.StampModuleCallGates(map[string]*core.OrderedMap{"Nil": nil, "M": live}, "boru:x")

	got, _ := live.Get("w")
	id := got.Data.(*core.FnDefInfo).Signatures[0].ModuleCall
	if id == nil || id.Module != "boru:x" || id.Export != "w" {
		t.Errorf("live namespace should be stamped boru:x/w, got %+v", id)
	}
}

func TestStampModuleCallGatesWrapperWithoutSubRegistry(t *testing.T) {
	// A wrapper carrying no Registry has nothing beyond its own sigs to
	// stamp — the stamper must stop there rather than dereference nil.
	fd := core.FnDefInfo{Name: "w", Signatures: []core.Signature{{BarrierPos: core.BarrierAllForward}}, Registry: nil}
	om := core.NewOrderedMap()
	om.Set("w", core.NewFunction(fd))

	core.StampModuleCallGates(map[string]*core.OrderedMap{"M": om}, "boru:y")

	got, _ := om.Get("w")
	id := got.Data.(*core.FnDefInfo).Signatures[0].ModuleCall
	if id == nil || id.Module != "boru:y" {
		t.Fatalf("own signatures must still be stamped, got %+v", id)
	}
}

func TestStampModuleCallGatesSkipsNonFunctionExports(t *testing.T) {
	// Constants and type literals are data reads, not calls: nothing to
	// stamp, and no panic on the non-FnDefInfo payload.
	om := core.NewOrderedMap()
	om.Set("k", core.NewInteger(1))
	core.StampModuleCallGates(map[string]*core.OrderedMap{"M": om}, "boru:z")
	got, _ := om.Get("k")
	if n, err := core.AsInteger(got); err != nil || n != 1 {
		t.Errorf("a constant export must be left alone, got %v (%v)", n, err)
	}
}

func TestLookupModuleCallCheckerNilRegistry(t *testing.T) {
	// The lookup is called from gate helpers that may run without a
	// registry (check-mode probes, bare sub-engines).
	if mc := core.LookupModuleCallChecker(nil); mc != nil {
		t.Errorf("a nil registry has no checker, got %v", mc)
	}
}
