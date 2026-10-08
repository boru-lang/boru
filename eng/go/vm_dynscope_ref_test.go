package eng

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_dynscope_ref_test.go pins OpLookupDynScopeRef (NUR334), the `/v` read's
// twin of OpLookupDynScope: the interpreter's stepWordVal at run time. It
// pushes what core.ResolveRef answers — a plain value, a fn binding as the
// Function value, a class — where OpLookupDynScope defers on the last two,
// keeps the lookup's defer on an active token, and a miss of a live-read
// name raises the read's undefined_word.
func TestLookupDynScopeRefArms(t *testing.T) {
	ref := func(name string) *compiler.Program {
		return dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScopeRef, Arg: 0}}, core.NewString(name))
	}
	// A plain value: the binding itself.
	r := seam7Reg(t)
	r.Defs.Push("dry", core.NewInteger(7))
	out, err := RunProgram(ref("dry"), r)
	if err != nil || len(out) != 1 {
		t.Fatalf("a value read: %v %v", out, err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Errorf("read %v, want 7", out[0])
	}

	// A fn binding: pushed as the Function value, never dispatched and never
	// deferred (OpLookupDynScope defers on the same binding).
	r2 := seam7Reg(t)
	r2.Defs.Push("drf", core.NewFunction(core.FnDefInfo{Name: "drf", Registry: r2,
		Signatures: []core.Signature{{Returns: []*core.Type{core.TAny}, BarrierPos: -1}}}))
	out, err = RunProgram(ref("drf"), r2)
	if err != nil || len(out) != 1 {
		t.Fatalf("a fn binding's /v read pushes it: %v %v", out, err)
	}
	if fd, isFn := out[0].Data.(core.FnDefInfo); !isFn || fd.Name != "drf" || !out[0].Parent.Equal(core.TFunction) {
		t.Errorf("pushed %v, want the Function value of drf", out[0])
	}

	// A class binding is data to the value spelling too.
	r4 := seam7Reg(t)
	r4.Defs.Push("drc", core.NewValueRaw(core.TClass, &core.ClassTypeInfo{Name: "Class/C", Fields: core.NewOrderedMap()}))
	out, err = RunProgram(ref("drc"), r4)
	if err != nil || len(out) != 1 {
		t.Fatalf("a class binding's /v read pushes it: %v %v", out, err)
	}
	if _, isClass := out[0].Data.(*core.ClassTypeInfo); !isClass {
		t.Errorf("pushed %v, want the class", out[0])
	}

	// An active token keeps the lookup's defer: where the interpreter's tape
	// steps the marker later (a fn's return re-steps it) is not the read's
	// to decide.
	r3 := seam7Reg(t)
	r3.Defs.Push("drs", core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)})))
	_, err = RunProgram(ref("drs"), r3)
	wantInternal(t, err, "dynamic-scope read of an active token `drs`")

	// A miss of a live-read name is the read's undefined_word, at the read.
	readAt := core.SrcPos{Row: 1, Col: 9}
	live := ref("drm")
	live.Debug = []core.SrcPos{readAt}
	live.LiveReadNames = map[string]bool{"drm": true}
	_, err = RunProgram(live, seam7Reg(t))
	var ae *core.BoruError
	if !errors.As(err, &ae) || ae.Code != "undefined_word" || ae.Row != readAt.Row || ae.Col != readAt.Col {
		t.Fatalf("a live-read name's miss raises undefined_word at the read: %v", err)
	}
	// Outside the live-read names a miss keeps the lookup's defer.
	_, err = RunProgram(ref("drm"), seam7Reg(t))
	wantInternal(t, err, "dynamic-scope read miss for `drm`")

	if got := compiler.OpLookupDynScopeRef.String(); got != "LOOKUP_DYN_SCOPE_REF" {
		t.Errorf("the op's name: %q", got)
	}
}
