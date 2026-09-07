package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_deopt_test.go pins OpDeoptIfFn's runtime arms (NUR123's per-read
// deopt): plain data is a no-op; a fn at the guarded slot hands the body
// tokens from the statement's token to the interpreter over the frame's
// region, whose residual replaces it; a value living on the stack (Depth)
// is dropped from the island's prefix; an island error is stamped; the
// defensive arms raise.
func TestDeoptIfFnArms(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	vc := seam7VC(r)
	inc := core.NewFunction(*r.Lookup("cinc"))
	five := core.NewInteger(5)
	at := func(col int) core.SrcPos { return core.SrcPos{Row: 1, Col: col} }
	tok := func(name string, col int) core.Value {
		w := core.NewWord(name)
		w.SetPos(at(col))
		return w
	}
	// (The seam registry carries no `def`: a body-local def's teardown is
	// pinned by the lang parity rows, `j  def y 1`.)
	body := []core.Value{core.NewInteger(0), tok("j", 9)}
	fn := &compiler.CompiledFn{Body: body}
	core.InstallDef(r, "j", inc)
	defer core.UninstallDef(r, "j")

	// Plain data at the slot: untouched, not fired.
	spec := compiler.DeoptSpec{Name: "j", Slot: 0, Depth: -1, Token: 1, RetPC: 9}
	st, fired, err := vc.deoptIfFn(r, fn, &spec, 0, []core.Value{five}, []core.Value{five}, seam7Dbg, 0)
	if err != nil || fired || len(st) != 1 {
		t.Errorf("data: no-op, got %v %v %v", st, fired, err)
	}
	// A fn at the slot: the island runs `j` over the frame region [5] — j
	// collects 5 → 6.
	st, fired, err = vc.deoptIfFn(r, fn, &spec, 0, []core.Value{five}, []core.Value{inc}, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 1 {
		t.Fatalf("fn: the island's residual replaces the region, got %v %v %v", st, fired, err)
	}
	if n, _ := core.AsInteger(st[0]); n != 6 {
		t.Errorf("the word collected the frame's 5: %v", st[0])
	}
	// A value living on the stack (Depth 0 = the top): dropped from the
	// prefix — the interpreter's frame never held it.
	depthSpec := compiler.DeoptSpec{Name: "j", Slot: -1, Depth: 0, Token: 1, RetPC: 9}
	st, fired, err = vc.deoptIfFn(r, fn, &depthSpec, 1, []core.Value{core.NewInteger(0), five, inc}, nil, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 2 {
		t.Fatalf("stack-resident fn: got %v %v %v", st, fired, err)
	}
	if n, _ := core.AsInteger(st[1]); n != 6 {
		t.Errorf("the prefix below the frame base and the dropped entry: %v", st)
	}
	// The island's error is the interpreter's own, stamped: j over an
	// empty frame region has no argument to collect.
	_, _, err = vc.deoptIfFn(r, fn, &spec, 0, nil, []core.Value{inc}, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "cannot call `j`") {
		t.Errorf("j over no argument is the interpreter's no-match: %v", err)
	}
	// Defensive arms: a bad slot, an underflowing depth, a bad table entry.
	if _, _, err := vc.deoptIfFn(r, fn, &compiler.DeoptSpec{Slot: 3}, 0, nil, []core.Value{inc}, seam7Dbg, 0); err == nil {
		t.Error("a slot beyond the frame raises")
	}
	if _, _, err := vc.deoptIfFn(r, fn, &compiler.DeoptSpec{Slot: -1, Depth: 2}, 0, []core.Value{inc}, nil, seam7Dbg, 0); err == nil {
		t.Error("a depth below the frame base raises")
	}
	if _, _, err := vc.deoptIfFn(r, fn, &compiler.DeoptSpec{Slot: 0, Depth: -1, Token: 9, RetPC: 1}, 0, nil, []core.Value{inc}, seam7Dbg, 0); err == nil {
		t.Error("a token beyond the body raises")
	}
}

// TestReStepIfFnArms pins the RE-STEP arm of OpDeoptIfFn (DeoptSpec.Results,
// NUR124): plain results are a no-op; a fn among the results hands them as
// TOKENS — a fn value stepped at the pointer dispatches over the region
// beneath — followed by the body from the resume token; an unnamed param the
// unit has not pushed (Prefix) is seated beneath the region; the island's
// error is the interpreter's; the defensive arms raise.
func TestReStepIfFnArms(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	vc := seam7VC(r)
	inc := core.NewFunction(*r.Lookup("cinc"))
	five := core.NewInteger(5)
	body := []core.Value{core.NewInteger(0), core.NewWord("j")}
	fn := &compiler.CompiledFn{Body: body}
	core.InstallDef(r, "j", inc)
	defer core.UninstallDef(r, "j")

	// Plain results on top: untouched, not fired.
	spec := compiler.DeoptSpec{Slot: -1, Depth: -1, Results: 1, Token: 2, RetPC: 9}
	st, fired, err := vc.deoptIfFn(r, fn, &spec, 0, []core.Value{five, five}, nil, seam7Dbg, 0)
	if err != nil || fired || len(st) != 2 {
		t.Errorf("data: no-op, got %v %v %v", st, fired, err)
	}
	// A fn on top over the region [5], resuming at the body's end: the fn
	// re-steps as a token and collects the 5 beneath it → 6.
	st, fired, err = vc.deoptIfFn(r, fn, &spec, 0, []core.Value{five, inc}, nil, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 1 {
		t.Fatalf("fn: the island's residual replaces the region, got %v %v %v", st, fired, err)
	}
	if n, _ := core.AsInteger(st[0]); n != 6 {
		t.Errorf("the re-stepped fn collected the region's 5: %v", st[0])
	}
	// The region is above a frame base: entries below it stay.
	st, fired, err = vc.deoptIfFn(r, fn, &spec, 1, []core.Value{core.NewInteger(0), five, inc}, nil, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 2 {
		t.Fatalf("frame base: got %v %v %v", st, fired, err)
	}
	// An unpushed unnamed input rides as the prefix beneath the region.
	pre := compiler.DeoptSpec{Slot: -1, Depth: -1, Prefix: []int{0}, Results: 1, Token: 2, RetPC: 9}
	st, fired, err = vc.deoptIfFn(r, fn, &pre, 0, []core.Value{inc}, []core.Value{five}, seam7Dbg, 0)
	if err != nil || !fired || len(st) != 1 {
		t.Fatalf("prefix: got %v %v %v", st, fired, err)
	}
	if n, _ := core.AsInteger(st[0]); n != 6 {
		t.Errorf("the prefix seated the frame's input: %v", st[0])
	}
	// The island's error is the interpreter's own: resuming at the body's
	// `j` (Token 1), the re-stepped fn value has a token after it and
	// nothing to collect — the value path's uncalled_function.
	tail := compiler.DeoptSpec{Slot: -1, Depth: -1, Results: 1, Token: 1, RetPC: 9}
	if _, _, err := vc.deoptIfFn(r, fn, &tail, 0, []core.Value{inc}, nil, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "uncalled_function") {
		t.Errorf("the island's no-match is the interpreter's: %v", err)
	}
	// Defensive arms: a region shorter than the results, a token beyond
	// the body, a prefix slot beyond the frame.
	if _, _, err := vc.deoptIfFn(r, fn, &compiler.DeoptSpec{Slot: -1, Depth: -1, Results: 2, Token: 2, RetPC: 9}, 1, []core.Value{inc}, nil, seam7Dbg, 0); err == nil {
		t.Error("results below the frame base raise")
	}
	if _, _, err := vc.deoptIfFn(r, fn, &compiler.DeoptSpec{Slot: -1, Depth: -1, Results: 1, Token: 3, RetPC: 9}, 0, []core.Value{inc}, nil, seam7Dbg, 0); err == nil {
		t.Error("a token beyond the body raises")
	}
	if _, _, err := vc.deoptIfFn(r, fn, &compiler.DeoptSpec{Slot: -1, Depth: -1, Prefix: []int{3}, Results: 1, Token: 2, RetPC: 9}, 0, []core.Value{inc}, nil, seam7Dbg, 0); err == nil {
		t.Error("a prefix slot beyond the frame raises")
	}
}
