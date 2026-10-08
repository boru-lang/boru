package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_live_deopt_test.go pins OpDeoptIfFn's LIVE-READ arm (DeoptSpec.Live,
// compiler kept_live_deopt.go): the value tested is the registry binding
// itself, read before the statement's first op. A binding the compiled
// statement takes is a no-op; one it cannot take (no binding, a fn a bare
// read dispatches, an active token, a value off the compiled type) hands the
// body from the statement's token to the interpreter over the whole frame
// region; a root island's residual is the program's, a unit's is screened;
// the island's error is stamped; the defensive arms raise.
func TestLiveDeoptArms(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	vc := seam7VC(r)
	inc := core.NewFunction(*r.Lookup("cinc"))
	five := core.NewInteger(5)
	body := []core.Value{core.NewInteger(0), core.NewWord("j"), core.NewWordRef("s"), core.NewWord("nope")}
	spec := compiler.DeoptSpec{Name: "j", Slot: -1, Depth: -1, Token: 1, RetPC: 9, Live: true, Model: core.TInteger}

	// A binding of the compiled type: untouched, not fired.
	core.InstallDef(r, "j", five)
	st, fired, err := vc.deoptIfFn(r, body, false, &spec, 0, []core.Value{five}, nil, seam7Dbg, 0)
	if err != nil || fired || len(st) != 1 {
		t.Errorf("a binding the statement takes: no-op, got %v %v %v", st, fired, err)
	}
	core.UninstallDef(r, "j")

	// A fn a bare read dispatches: the island runs `j` over the region [5].
	core.InstallDef(r, "j", inc)
	st, fired, err = vc.deoptIfFn(r, body[:2], false, &spec, 0, []core.Value{five}, nil, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 1 {
		t.Fatalf("a dispatching binding: the island's residual replaces the region, got %v %v %v", st, fired, err)
	}
	if n, _ := core.AsInteger(st[0]); n != 6 {
		t.Errorf("the word collected the frame's 5: %v", st[0])
	}
	core.UninstallDef(r, "j")

	// No binding: the interpreter's own error, stamped.
	miss := spec
	miss.Name, miss.Token = "nope", 3
	if _, _, err := vc.deoptIfFn(r, body, false, &miss, 0, nil, nil, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "undefined word: nope") {
		t.Errorf("an unbound name is the interpreter's undefined_word: %v", err)
	}

	// A `/v` read of a splice: the value spelling delivers it as data. At the
	// root the residual is the program's answer; a unit's goes back to a
	// compiled caller that would keep it as data, so it is screened.
	splice := core.NewSplice(core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)}))
	core.InstallDef(r, "s", splice)
	ref := compiler.DeoptSpec{Name: "s", Slot: -1, Depth: -1, Token: 2, RetPC: 9, Live: true, Ref: true, Model: core.TInteger}
	st, fired, err = vc.deoptIfFn(r, body[:3], true, &ref, 0, nil, nil, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 1 || !core.IsSplice(st[0]) {
		t.Errorf("a root island leaves the splice as the program does: %v %v %v", st, fired, err)
	}
	if _, _, err := vc.deoptIfFn(r, body[:3], false, &ref, 0, nil, nil, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "tape-coupled deopt result") {
		t.Errorf("a unit's splice result is screened: %v", err)
	}
	core.UninstallDef(r, "s")

	// Defensive arms: a bad table entry, a bad prefix slot.
	for _, bad := range []compiler.DeoptSpec{
		{Name: "nope", Live: true, Token: -1, RetPC: 9},
		{Name: "nope", Live: true, Token: 1, RetPC: -1},
		{Name: "nope", Live: true, Token: 1, RetPC: 9, Prefix: []int{3}},
	} {
		if _, _, err := vc.deoptIfFn(r, body, false, &bad, 0, nil, nil, seam7Dbg, 0); err == nil || strings.Contains(err.Error(), "undefined word") {
			t.Errorf("%+v: a bad table entry raises before any island, got %v", bad, err)
		}
	}
}
