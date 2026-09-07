package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// closure_bridge_test.go pins the VM's side of the closure VALUE bridge
// (vmCompiledRuntime.ClosureAsFnDef, NUR124's payload axis): what declines
// (not a closure, no running VM, no program, a bad unit, a unit with no
// param contract) and what the bridged fn carries (one signature over the
// unit's params, the source lambda's Anonymous flag, a handler that applies
// the closure through the registry's invoker).
func TestClosureAsFnDefArms(t *testing.T) {
	rt := vmCompiledRuntime{}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	prog := &compiler.Program{Fns: []compiler.CompiledFn{
		{Name: "fnval$body", NArgs: 1, Params: []*core.Type{core.TInteger}, Lambda: true},
		{Name: "each$body", NArgs: 1},
	}}
	closure := compiler.NewClosure(prog, 0, nil)

	if v, ok := rt.ClosureAsFnDef(r, core.NewInteger(1)); ok || !v.Parent.Equal(core.TInteger) {
		t.Error("not a closure: declined with the value")
	}
	if _, ok := rt.ClosureAsFnDef(r, closure); ok {
		t.Error("no running VM (no invoker): declined")
	}
	if _, ok := rt.ClosureAsFnDef(nil, closure); ok {
		t.Error("no registry: declined")
	}
	var seen []core.Value
	r.Invoker = func(_ *core.Registry, body core.Value, inputs []core.Value) ([]core.Value, error) {
		seen = inputs
		n, _ := core.AsInteger(inputs[0])
		return []core.Value{core.NewInteger(n * 3)}, nil
	}
	if _, ok := rt.ClosureAsFnDef(r, compiler.NewClosure(nil, 0, nil)); ok {
		t.Error("no program identity: declined")
	}
	if _, ok := rt.ClosureAsFnDef(r, compiler.NewClosure(prog, 5, nil)); ok {
		t.Error("a unit beyond the program: declined")
	}
	if _, ok := rt.ClosureAsFnDef(r, compiler.NewClosure(prog, 1, nil)); ok {
		t.Error("a unit with no param contract (a token body): declined")
	}
	fnv, ok := rt.ClosureAsFnDef(r, closure)
	if !ok {
		t.Fatal("a lambda unit with a contract bridges")
	}
	fd, isFn := fnv.Data.(core.FnDefInfo)
	if !isFn || len(fd.Signatures) != 1 || !fd.Anonymous || fd.Signatures[0].TotalArgs() != 1 {
		t.Fatalf("one signature over the unit's param, Anonymous as the lambda: %+v", fd)
	}
	out, err := fd.Signatures[0].DispatchHandler()([]core.Value{core.NewInteger(5)}, nil, nil, r)
	if err != nil || len(out) != 1 || len(seen) != 1 {
		t.Fatalf("the handler applies the closure through the invoker: %v %v", out, err)
	}
	if n, _ := core.AsInteger(out[0]); n != 15 {
		t.Errorf("15: %v", out)
	}
	// A named (non-lambda) unit bridges without the flag: a 0-arg one fires
	// where the interpreter's named fn value fires.
	prog.Fns[0].Lambda = false
	fnv, _ = rt.ClosureAsFnDef(r, closure)
	if fd := fnv.Data.(core.FnDefInfo); fd.Anonymous {
		t.Error("a named unit's bridge is not anonymous")
	}
}
