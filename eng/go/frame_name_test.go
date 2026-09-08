package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// frame_name_test.go pins nameFrameFns (the twenty-sixth increment, NUR122's
// class): a fn VALUE bound for a NAMED param or capture takes the binding's
// name, as the interpreter's frame binding gives it (installDef's
// `fnDef.Name = name`); an unnamed slot, a non-fn value, a value already so
// named and a module wrapper (a foreign Registry) are left alone; a compiled closure is named too (the thirtieth increment). The rest of this comment predates that:
// left alone.
func TestNameFrameFns(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fn := &compiler.CompiledFn{NParams: 4, NLocals: 5, LocalNames: []string{"g", "", "h", "m", "t"}}
	lam := core.NewFunction(core.FnDefInfo{Anonymous: true})
	named := core.NewFunction(core.FnDefInfo{Name: "h"})
	foreign := core.NewFunction(core.FnDefInfo{Name: "sqrt", Registry: r})
	closure := compiler.NewClosure(&compiler.Program{}, 0, nil)
	locals := []core.Value{lam, lam, named, foreign, closure}
	nameFrameFns(fn, locals)
	if fd := locals[0].Data.(core.FnDefInfo); fd.Name != "g" || !fd.Anonymous {
		t.Errorf("a lambda bound for param g is named g, its other fields kept: %+v", fd)
	}
	if fd := locals[1].Data.(core.FnDefInfo); fd.Name != "" {
		t.Errorf("an unnamed slot names nothing: %+v", fd)
	}
	if fd := locals[2].Data.(core.FnDefInfo); fd.Name != "h" {
		t.Errorf("a value already so named is untouched: %+v", fd)
	}
	if fd := locals[3].Data.(core.FnDefInfo); fd.Name != "sqrt" {
		t.Errorf("a module wrapper keeps its name (installDef's rebinding path is not mirrored): %+v", fd)
	}
	if cl, ok := locals[4].Data.(core.ClosurePayload); !ok || cl.RetName != "" {
		t.Errorf("a compiled closure in a CAPTURE slot (past NParams) is left alone: %v", locals[4])
	}
	// A compiled closure bound for a named PARAM takes the name (the
	// thirtieth increment), its payload kept — the render stays when the
	// payload names no unit.
	named2 := []core.Value{closure}
	nameFrameFns(fn, named2)
	if cl, ok := named2[0].Data.(core.ClosurePayload); !ok || cl.RetName != "g" || cl.Render != "" {
		t.Errorf("a compiled closure bound for param g is named g: %v", named2[0])
	}
	// A closure ALREADY named — a def-bound closure handed to a Function
	// param — takes the param's name too, as installDef renames whatever it
	// binds (the thirty-first increment); one already so named is untouched.
	prenamed := []core.Value{core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: &compiler.Program{}, Unit: 0, RetName: "p"})}
	nameFrameFns(fn, prenamed)
	if cl := prenamed[0].Data.(core.ClosurePayload); cl.RetName != "g" {
		t.Errorf("a named closure bound for param g is renamed g: %v", prenamed[0])
	}
	same := []core.Value{core.NewValueRaw(core.TFunction, core.ClosurePayload{RetName: "g", Render: "kept"})}
	nameFrameFns(fn, same)
	if cl := same[0].Data.(core.ClosurePayload); cl.Render != "kept" {
		t.Errorf("a closure already so named is untouched: %v", same[0])
	}
	// A slot past the frame, a data value: nothing to do, nothing to panic on.
	short := []core.Value{core.NewInteger(1)}
	nameFrameFns(fn, short)
	if n, _ := core.AsInteger(short[0]); n != 1 {
		t.Errorf("data untouched: %v", short)
	}
	// bindUnitLocals names through the same helper.
	bound := bindUnitLocals(fn, []core.Value{lam, lam, named, foreign}, []core.Value{closure})
	if fd := bound[0].Data.(core.FnDefInfo); fd.Name != "g" {
		t.Errorf("bindUnitLocals names the frame's fns: %+v", fd)
	}
}
