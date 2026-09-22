package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// memberIsFnValued recognises a container member that holds a fn: a concrete
// fn value, or a fn-typed carrier (a produced closure a map literal stored —
// the container-member calls, 2026-09-22); anything else is data.
func TestMemberIsFnValued(t *testing.T) {
	if !memberIsFnValued(core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "inc"}}) {
		t.Error("a concrete fn value is fn-valued")
	}
	if !memberIsFnValued(core.NewCarrier(core.TFunction)) {
		t.Error("a fn-typed carrier is fn-valued")
	}
	if memberIsFnValued(core.NewInteger(5)) || memberIsFnValued(core.NewCarrier(core.TInteger)) {
		t.Error("data, concrete or carried, is not fn-valued")
	}
	// The read tag follows: a concrete map whose member is a fn-typed
	// carrier tags the read, and pinpoints the carrier as the member.
	m := core.NewOrderedMap()
	m.Set("b", core.NewCarrier(core.TFunction))
	args := []core.Value{core.NewAtom("b"), core.NewMap(m)}
	if !readsFnMember(args) {
		t.Error("a fn-typed carrier member tags the read")
	}
	if mv, ok := readFnMemberValue(args); !ok || !core.IsFnTypedCarrier(mv) {
		t.Errorf("the pinpointed member is the carrier, got %#v (%v)", mv, ok)
	}
}
