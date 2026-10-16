package core

import (
	"testing"
)

// Stage-5 coverage for module_gate.go: the StampModuleCallGates
// stamping walk (export-map sigs, sub-registry stored targets,
// trivial-delegation targets, the skip arms) plus the StampedModuleCall
// stamp reader. Mirrors the lang call site
// (lang/go/native/native_module_module.go: StampModuleCallGates(
// desc.Exports, desc.Ref)) with hand-built export maps.

func modgateStage5Fn(name string, sigs []Signature) Value {
	return NewFunction(FnDefInfo{Name: name, Signatures: sigs})
}

func TestModuleGateStage5StampModuleCallGates(t *testing.T) {
	sub, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Stored dispatch targets in the module sub-registry: the fn's own
	// name and the trivial-delegation inner native. A non-FnDefInfo
	// entry stacked under a target name is skipped.
	sub.Defs.Push("zz-inner", modgateStage5Fn("zz-inner", []Signature{{
		Params: []FnParam{{Name: "x", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("x")}), BarrierPos: BarrierAllForward,
	}}))
	sub.Defs.Push("zz-target", NewInteger(9))
	sub.Defs.Push("zz-target", modgateStage5Fn("zz-target", []Signature{{
		Args: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(1)}), BarrierPos: BarrierAllForward,
	}}))

	preStamp := &ModuleCallID{Module: "boru:zz-pre", Export: "kept"}
	wrapper := FnDefInfo{
		Name:     "zz-inner",
		Registry: sub,
		Signatures: []Signature{
			// Trivial delegation: single-word body, unnamed params.
			{Args: []*Type{TInteger}, Impl: Boru([]Value{NewWord("zz-target")}), BarrierPos: BarrierAllForward},
			// Ordinary sig — stamped but no delegation target.
			{Params: []FnParam{{Name: "x", Type: TInteger}}, Impl: Boru([]Value{NewWord("x"), NewWord("x")}), BarrierPos: BarrierAllForward},
			// Fallback: never stamped.
			{Fallback: true, Impl: Boru(nil)},
			// Already stamped: keeps its first identity (idempotence).
			{Impl: Boru(nil), ModuleCall: preStamp},
		},
	}
	om := NewOrderedMap()
	om.Set("wrapped", NewFunction(wrapper))
	om.Set("const", NewInteger(1)) // data export: not a call, skipped
	om.Set("ext", NewFunction(NewWordExtension("zz-owner", "zz-base", []Signature{{
		Args: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(2)}), BarrierPos: BarrierAllForward,
	}})))
	// A fn export with no sub-registry: own sigs stamped, then done.
	om.Set("noreg", NewFunction(FnDefInfo{Signatures: []Signature{{
		Impl: Boru([]Value{NewInteger(3)}), BarrierPos: BarrierAllForward,
	}}}))
	exports := map[string]*OrderedMap{"Pkg": om, "Nil": nil}

	// An empty moduleRef (inline `module […]`) stamps nothing.
	StampModuleCallGates(exports, "")
	if v, _ := om.Get("wrapped"); v.Data().(*FnDefInfo).Signatures[0].ModuleCall != nil {
		t.Fatal("an empty moduleRef must stamp nothing")
	}

	StampModuleCallGates(exports, "boru:zz-mod")

	v, _ := om.Get("wrapped")
	fd := v.Data().(*FnDefInfo)
	id := fd.Signatures[0].ModuleCall
	if id == nil || id.Module != "boru:zz-mod" || id.Export != "wrapped" {
		t.Fatalf("wrapper sig0 stamp = %+v", id)
	}
	if fd.Signatures[1].ModuleCall != id {
		t.Errorf("wrapper sig1 stamp = %+v, want the shared id", fd.Signatures[1].ModuleCall)
	}
	if fd.Signatures[2].ModuleCall != nil {
		t.Errorf("fallback sig must stay unstamped, got %+v", fd.Signatures[2].ModuleCall)
	}
	if fd.Signatures[3].ModuleCall != preStamp {
		t.Errorf("pre-stamped sig must keep its identity, got %+v", fd.Signatures[3].ModuleCall)
	}
	if sub.ModuleRef != "boru:zz-mod" {
		t.Errorf("sub-registry ModuleRef = %q", sub.ModuleRef)
	}

	// Stored sigs under both target names carry the stamp.
	for _, name := range []string{"zz-inner", "zz-target"} {
		stamped := false
		for _, entry := range sub.Defs.Stack(name) {
			if sfd, ok := entry.Data().(*FnDefInfo); ok {
				for i := range sfd.Signatures {
					if sid := sfd.Signatures[i].ModuleCall; sid != nil &&
						sid.Module == "boru:zz-mod" && sid.Export == "wrapped" {
						stamped = true
					}
				}
			}
		}
		if !stamped {
			t.Errorf("stored sigs for %q must be stamped", name)
		}
	}

	// Data and word-extension exports stay untouched.
	if ev, _ := om.Get("ext"); ev.Data().(*FnDefInfo).Signatures[0].ModuleCall != nil {
		t.Error("a word-extension clone must not be stamped")
	}
	if nv, _ := om.Get("noreg"); nv.Data().(*FnDefInfo).Signatures[0].ModuleCall == nil {
		t.Error("a registry-less fn export must still stamp its own sigs")
	}

	// Idempotence: a re-stamp under a different ref keeps the first id.
	StampModuleCallGates(exports, "boru:zz-other")
	v, _ = om.Get("wrapped")
	if got := v.Data().(*FnDefInfo).Signatures[0].ModuleCall; got != id {
		t.Errorf("re-stamp must keep the first identity, got %+v", got)
	}
	if sub.ModuleRef != "boru:zz-mod" {
		t.Errorf("sub-registry ModuleRef must keep its first ref, got %q", sub.ModuleRef)
	}
}

func TestModuleGateStage5StampedModuleCallReader(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// An unstamped fn binding walks its sigs and yields nothing.
	r.Defs.Push("zz-plain", modgateStage5Fn("zz-plain", []Signature{{
		Impl: Boru(nil), BarrierPos: BarrierAllForward,
	}}))
	if id := StampedModuleCall(r, "zz-plain"); id != nil {
		t.Errorf("an unstamped binding has no stamp, got %+v", id)
	}
	// A stamped sig is found and returned.
	want := &ModuleCallID{Module: "boru:zz-m", Export: "usage"}
	r.Defs.Push("zz-stamped", modgateStage5Fn("zz-stamped", []Signature{
		{Impl: Boru(nil), BarrierPos: BarrierAllForward},
		{Impl: Boru(nil), BarrierPos: BarrierAllForward, ModuleCall: want},
	}))
	if id := StampedModuleCall(r, "zz-stamped"); id != want {
		t.Errorf("stamp reader = %+v, want %+v", id, want)
	}
}
