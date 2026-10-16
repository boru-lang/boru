package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// frame_name_test.go pins nameFrameFns (the twenty-sixth increment, NUR122's
// class): a fn VALUE bound for a NAMED param or capture takes the binding's
// name, as the interpreter's frame binding gives it (installDef constructs
// the binding's fn under its name); an unnamed slot, a non-fn value, a value
// already so named and a module wrapper (a foreign Registry) are left alone;
// a compiled closure is named too (the thirtieth increment). Since no-copy
// phase 3 the name is the SLOT's (core.SetFnName): the payload is shared,
// never copied to carry it, and every reader asks core.FnNameOf.
func TestNameFrameFns(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	other, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fn := &compiler.CompiledFn{NParams: 5, NLocals: 6, LocalNames: []string{"g", "", "h", "m", "k", "t"}}
	lam := core.NewFunction(core.FnDefInfo{Anonymous: true})
	named := core.NewFunction(core.FnDefInfo{Name: "h"})
	// A fn whose home is ANOTHER module's registry — a module wrapper.
	foreign := core.NewFunction(core.FnDefInfo{Name: "sqrt", Registry: other})
	closure := compiler.NewClosure(&compiler.Program{}, 0, nil)
	// A fn at home in r — every boru-bodied fn carries the registry that
	// minted it — is named like the homeless lambda above; the home is not
	// what makes a wrapper foreign, the OTHER module is.
	home := core.NewFunction(core.FnDefInfo{Name: "inc", Registry: r})
	locals := []core.Value{lam, lam, named, foreign, home, closure}
	nameFrameFns(r, fn, locals)
	if fd := locals[0].Data().(*core.FnDefInfo); core.FnNameOf(&locals[0]) != "g" || !fd.Anonymous || fd != lam.Data().(*core.FnDefInfo) || fd.Name != "" {
		t.Errorf("a lambda bound for param g is named g in its slot, its payload shared and unrenamed: %q %+v", core.FnNameOf(&locals[0]), *fd)
	}
	if core.FnNameOf(&locals[1]) != "" {
		t.Errorf("an unnamed slot names nothing: %q", core.FnNameOf(&locals[1]))
	}
	if core.FnNameOf(&locals[2]) != "h" || core.HolderFnName(&locals[2]) != "" {
		t.Errorf("a value already so named is untouched: %q", core.HolderFnName(&locals[2]))
	}
	if core.FnNameOf(&locals[3]) != "sqrt" {
		t.Errorf("a module wrapper keeps its name (installDef's rebinding path is not mirrored): %q", core.FnNameOf(&locals[3]))
	}
	if fd := locals[4].Data().(*core.FnDefInfo); core.FnNameOf(&locals[4]) != "k" || fd.Registry != r || fd.Name != "inc" {
		t.Errorf("a fn at home in r is named for its param, its home and payload kept: %q %+v", core.FnNameOf(&locals[4]), *fd)
	}
	if cl, ok := closureView(&locals[5]); !ok || cl.RetName != "" {
		t.Errorf("a compiled closure in a CAPTURE slot (past NParams) is left alone: %v", locals[5])
	}
	// A compiled closure bound for a named PARAM takes the name (the
	// thirtieth increment) in its slot, the payload kept — and renders under
	// it when its unit can be described; this payload names no unit.
	named2 := []core.Value{closure}
	nameFrameFns(r, fn, named2)
	if cl, ok := closureView(&named2[0]); !ok || cl.RetName != "g" || cl.Render != "" {
		t.Errorf("a compiled closure bound for param g is named g: %v", named2[0])
	}
	if cl := named2[0].Data().(*core.ClosurePayload); cl.RetName != "" {
		t.Errorf("the closure's payload is not renamed: %q", cl.RetName)
	}
	// A closure ALREADY named — a def-bound closure handed to a Function
	// param — takes the param's name too, as installDef renames whatever it
	// binds (the thirty-first increment); one already so named is untouched.
	prenamed := []core.Value{core.NewValueRaw(core.TFunction, &core.ClosurePayload{Prog: &compiler.Program{}, Unit: 0, RetName: "p"})}
	nameFrameFns(r, fn, prenamed)
	if cl, _ := closureView(&prenamed[0]); cl.RetName != "g" {
		t.Errorf("a named closure bound for param g is renamed g: %v", prenamed[0])
	}
	same := []core.Value{core.NewValueRaw(core.TFunction, &core.ClosurePayload{RetName: "g", Render: "kept"})}
	nameFrameFns(r, fn, same)
	if cl := same[0].Data().(*core.ClosurePayload); cl.Render != "kept" || core.HolderFnName(&same[0]) != "" {
		t.Errorf("a closure already so named is untouched: %v", same[0])
	}
	// A slot past the frame, a data value: nothing to do, nothing to panic on.
	short := []core.Value{core.NewInteger(1)}
	nameFrameFns(r, fn, short)
	if n, _ := core.AsInteger(short[0]); n != 1 {
		t.Errorf("data untouched: %v", short)
	}
	// bindUnitLocals names through the same helper.
	bound := bindUnitLocals(r, fn, []core.Value{lam, lam, named, foreign}, []core.Value{closure})
	if core.FnNameOf(&bound[0]) != "g" {
		t.Errorf("bindUnitLocals names the frame's fns: %q", core.FnNameOf(&bound[0]))
	}
}
