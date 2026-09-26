package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestCheckDynBodyOne pins the VM half of the runtime-checked single value
// (vm_dyn_body_one.go; compiler dyn_body_one.go): exactly one value the
// interpreter's tape pushes as data seats; any other count, and any value
// the tape dispatches, is the loud vm:dyn-body-one defer.
func TestCheckDynBodyOne(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkDynBodyOne(r, "do", []core.Value{core.NewInteger(5)}, nil, 0); err != nil {
		t.Fatalf("one plain value seats: %v", err)
	}
	fnVal := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	for name, results := range map[string][]core.Value{
		"no value":       nil,
		"two values":     {core.NewInteger(5), core.NewInteger(6)},
		"a fn value":     {fnVal},
		"a class":        {{Data: &core.ClassTypeInfo{}}},
		"a reach":        {core.NewReachFromKeys(core.NewInteger(1), nil)},
		"a dispatch mod": {core.NewDispatchMod(core.DispatchModInfo{})},
	} {
		err := checkDynBodyOne(r, "do", results, nil, 0)
		if err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "over a computed body left") {
			t.Errorf("%s: want the loud dyn-body-one defer, got %v", name, err)
		}
	}
}
