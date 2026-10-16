package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// moduleResidualStable is the residual-position stability gate for a
// module-family read (an import-bound namespace, a Module descriptor bound
// by def): the live binding must still hold the very instance the read saw,
// compared by the pointer eq/deq compare (NUR031). Driven directly because
// the end-to-end rows (lang/go/module_value_read_test.go) can only reach the
// stable and the rebound arms, not the nameless or nil-registry ones.
func TestModuleResidualStable(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc := core.NewModuleInstance(core.ModuleDesc{Ref: "boru:x"})
	other := core.NewModuleInstance(core.ModuleDesc{Ref: "boru:y"})
	ns := core.WithModuleNS(core.NewMap(core.NewOrderedMap()), "X", desc)
	ns.SetDynFrom("X")
	r.Defs.Push("X", ns)
	r.Defs.Push("m", desc)

	var nilES *EmitState
	if nilES.moduleResidualStable(ns) {
		t.Error("a nil recorder has no registry to read")
	}
	es := NewEmitState()
	if es.moduleResidualStable(ns) {
		t.Error("a recorder without a registry cannot vouch for a binding")
	}
	es.reg = r

	if !es.moduleResidualStable(ns) {
		t.Error("a namespace whose binding still holds its instance is stable")
	}
	nameless := core.WithModuleNS(core.NewMap(core.NewOrderedMap()), "X", desc)
	if es.moduleResidualStable(nameless) {
		t.Error("a read with no name (no DynFrom tag, no def-read entry) cannot be re-read")
	}
	// The def-read table is the second name channel.
	core.SetID(&nameless, "v-ns")
	es.defReads = map[string]string{"v-ns": "X"}
	if es.moduleResidualStable(nameless) {
		t.Error("a DIFFERENT namespace instance under the same name is not the read's")
	}
	same := ns
	core.SetID(&same, "v-ns")
	if !es.moduleResidualStable(same) {
		t.Error("the def-read name resolves the same instance")
	}

	// A descriptor bound by def: pointer identity of the boxed *ModuleDesc.
	d := desc
	d.SetDynFrom("m")
	if !es.moduleResidualStable(d) {
		t.Error("a descriptor whose binding still boxes the same *ModuleDesc is stable")
	}
	o := other
	o.SetDynFrom("m")
	if es.moduleResidualStable(o) {
		t.Error("a different descriptor under the same name is a rebind")
	}

	// The binding moved on: a scalar, or gone.
	r.Defs.Push("m", core.NewInteger(5))
	if es.moduleResidualStable(d) {
		t.Error("a binding rebound to a scalar no longer holds the instance")
	}
	r.Defs.Delete("X")
	if es.moduleResidualStable(ns) {
		t.Error("an undef'd binding cannot be re-read")
	}
}

// The paren-placement probes read the check state's ID sets and must decline
// on a nil recorder, a recorder without a registry, and an identity-less
// value — the last was reached end to end by the `$module` descriptor until
// it started taking its event's identity; pinned directly now.
func TestParenPlacementProbesDeclineWithoutIdentity(t *testing.T) {
	var nilES *EmitState
	v := core.NewInteger(1)
	core.SetID(&v, "v-1")
	if nilES.parenReSteppedFn(v) || nilES.parenPlacedMemberFn(v) {
		t.Error("a nil recorder places nothing")
	}
	es := NewEmitState()
	if es.parenReSteppedFn(v) || es.parenPlacedMemberFn(v) {
		t.Error("a recorder without a registry has no check state to read")
	}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.reg = r
	if es.parenReSteppedFn(core.Value{}) || es.parenPlacedMemberFn(core.Value{}) {
		t.Error("an identity-less value is never in an ID set")
	}
}
