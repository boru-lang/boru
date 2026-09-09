package eng

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	"github.com/boru-lang/boru/core/go"
)

// store_name_test.go pins the def-site closure rename (the twenty-ninth
// increment): the StoreNames lookup by emission target, and the rename of
// a stored ClosurePayload — its diagnostics name (RetName) and render.

func TestStoreNameAt(t *testing.T) {
	if _, ok := storeNameAt(nil, -1, 0); ok {
		t.Fatal("a nil program names nothing")
	}
	p := &compiler.Program{StoreNames: map[int]string{3: "h", 4: ""}, Fns: []compiler.CompiledFn{{StoreNames: map[int]string{1: "k"}}}}
	if name, ok := storeNameAt(p, -1, 3); !ok || name != "h" {
		t.Errorf("the main code's table: %q %v", name, ok)
	}
	if _, ok := storeNameAt(p, -1, 4); ok {
		t.Error("an empty name is no name")
	}
	if _, ok := storeNameAt(p, -1, 9); ok {
		t.Error("an unseated pc names nothing")
	}
	if name, ok := storeNameAt(p, 0, 1); !ok || name != "k" {
		t.Errorf("a unit's table: %q %v", name, ok)
	}
	if _, ok := storeNameAt(p, 7, 1); ok {
		t.Error("a unit beyond the program names nothing")
	}
}

func TestNameStoredClosure(t *testing.T) {
	r := seam7Reg(t)
	p := &compiler.Program{Fns: []compiler.CompiledFn{
		{Name: "", NParams: 1, NArgs: 1, Params: []*core.Type{core.TInteger}, Render: "fn (Integer)"},
	}}
	vc := &vmContext{p: p, r: r}
	// A non-closure is left alone.
	if v := vc.nameStoredClosure(core.NewInteger(5), "h"); !core.ValuesEqual(v, core.NewInteger(5)) {
		t.Error("a plain value is not renamed")
	}
	// A closure already named takes the new binding's name, as installDef
	// renames whatever it binds (the thirty-first increment); one already
	// so named is untouched.
	named := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 0, RetName: "g", Render: "fn g(Integer)"})
	if got := vc.nameStoredClosure(named, "h").Data.(core.ClosurePayload); got.RetName != "h" || !strings.Contains(got.Render, "fn h(") {
		t.Errorf("a binding renames a named closure: RetName=%q Render=%q", got.RetName, got.Render)
	}
	if v := vc.nameStoredClosure(named, "g"); v.Data.(core.ClosurePayload).Render != "fn g(Integer)" {
		t.Error("the same name is a no-op")
	}
	// A closure over a known unit takes the name in RetName and Render.
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 0, Render: "fn (Integer)"})
	got := vc.nameStoredClosure(cl, "h").Data.(core.ClosurePayload)
	if got.RetName != "h" || !strings.Contains(got.Render, "fn h(") {
		t.Errorf("renamed: RetName=%q Render=%q", got.RetName, got.Render)
	}
	// A closure whose payload names no unit (beyond the program) takes the
	// name and keeps its render.
	lost := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 5, Render: "fn (Integer)"})
	got = vc.nameStoredClosure(lost, "h").Data.(core.ClosurePayload)
	if got.RetName != "h" || got.Render != "fn (Integer)" {
		t.Errorf("no unit: RetName=%q Render=%q", got.RetName, got.Render)
	}
	// A unit whose params the bridge cannot describe keeps its render too.
	p2 := &compiler.Program{Fns: []compiler.CompiledFn{{NParams: 2, NArgs: 2, Params: []*core.Type{core.TInteger}, Render: "fn (Integer, Integer)"}}}
	vc2 := &vmContext{p: p2, r: r}
	odd := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p2, Unit: 0, Render: "fn (Integer, Integer)"})
	got = vc2.nameStoredClosure(odd, "h").Data.(core.ClosurePayload)
	if got.RetName != "h" || got.Render != "fn (Integer, Integer)" {
		t.Errorf("undescribable unit: RetName=%q Render=%q", got.RetName, got.Render)
	}
}
