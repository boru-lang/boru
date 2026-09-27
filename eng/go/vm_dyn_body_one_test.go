package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
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

// TestCheckDynBodyPlain pins the VM half of the plain-run check
// (checkDynBodyPlain; compiler lowerCall's DynBodyPlain, NUR213): a run of
// any count seats when none of its values is one the interpreter's tape
// dispatches, and a fn value, class, reach or modifier anywhere in it is the
// loud vm:dyn-body-plain defer.
func TestCheckDynBodyPlain(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for name, results := range map[string][]core.Value{
		"no value":     nil,
		"one value":    {core.NewInteger(5)},
		"two values":   {core.NewInteger(5), core.NewString("x")},
		"a plain list": {core.NewList([]core.Value{core.NewInteger(1)})},
	} {
		if err := checkDynBodyPlain(r, "do", results, nil, 0); err != nil {
			t.Errorf("%s: a plain run seats, got %v", name, err)
		}
	}
	fnVal := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	for name, results := range map[string][]core.Value{
		"a fn value last":  {core.NewInteger(5), fnVal},
		"a fn value first": {fnVal, core.NewInteger(5)},
		"a class":          {{Data: &core.ClassTypeInfo{}}},
		"a reach":          {core.NewReachFromKeys(core.NewInteger(1), nil)},
	} {
		err := checkDynBodyPlain(r, "do", results, nil, 0)
		if err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "where the run is seated as data") {
			t.Errorf("%s: want the loud dyn-body-plain defer, got %v", name, err)
		}
	}
}

// TestHeadNamedContract pins NUR275's frame naming (vm_dyn_apply.go): a fn
// value applied under a named head reports its contract under the head's
// name, as the interpreter's word dispatch of the binding does; no contract,
// an unnamed head or the same name keeps the frame's own.
func TestHeadNamedContract(t *testing.T) {
	h := &compiler.CompiledFn{Name: "h"}
	if got := headNamedContract(nil, compiler.DynApplyHead{Name: "g"}); got != nil {
		t.Errorf("no contract stays none, got %v", got)
	}
	if got := headNamedContract(h, compiler.DynApplyHead{}); got != h {
		t.Error("an unnamed head keeps the value's contract")
	}
	if got := headNamedContract(h, compiler.DynApplyHead{Name: "h"}); got != h {
		t.Error("the same name keeps the contract as it is")
	}
	if got := headNamedContract(h, compiler.DynApplyHead{Name: "g"}); got == h || got.Name != "g" || h.Name != "h" {
		t.Errorf("a named head renames a copy: got %+v, original %q", got, h.Name)
	}
}
