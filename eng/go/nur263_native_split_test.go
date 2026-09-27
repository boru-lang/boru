package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestNativeSplitRaiseIsTheInterpretersPlan pins the no-match arm of a
// committed CALL_NATIVE the pass matched optimistically (SigRef.Split,
// NUR263). The program passes the body as a compiled closure; the arm puts
// the token list the interpreter's tape holds back in its slot, then plans
// the operands as that tape. `0 zzfold [add] 's'` raises the interpreter's
// no-match over the written pair — whatever sits in the closure slot — and
// a window the plan fills, or a body slot out of range, leaves the
// handler's own error standing.
func TestNativeSplitRaiseIsTheInterpretersPlan(t *testing.T) {
	r := newTestRegistry(t)
	impl := core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(1)}, nil
	})
	r.Register("zzfold",
		core.Signature{Args: []*core.Type{core.TList, core.TMap}, NoEvalArgs: map[int]bool{0: true}, BarrierPos: 2, Impl: impl},
		core.Signature{Args: []*core.Type{core.TList, core.TList, core.TAny}, NoEvalArgs: map[int]bool{0: true}, BarrierPos: 3, Impl: impl},
	)
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	body := core.NewList([]core.Value{core.NewWord("add")})
	body.Quoted = true
	closure, s, zero := core.NewInteger(99), core.NewString("s"), core.NewInteger(0)
	sp := &compiler.NativeSplit{NFwd: 2, BodyAt: 0, Body: body}
	err := nativeSplitRaise(r, "zzfold", sp, []core.Value{closure, s, zero}, []core.SrcPos{{Row: 1, Col: 3}}, 0)
	be, ok := err.(*core.BoruError)
	if !ok || be.Code != "signature_error" || !strings.Contains(err.Error(), "[word(add)]") || !strings.Contains(err.Error(), "'s'") || be.Row != 1 || be.Col != 3 {
		t.Fatalf("0 zzfold [add] 's': the interpreter's no-match over [add] and 's', at the op; got %v", err)
	}
	// The operands the handler saw are not rewritten.
	args := []core.Value{closure, s, zero}
	_ = nativeSplitRaise(r, "zzfold", sp, args, nil, 0)
	if args[0].String() != closure.String() {
		t.Error("the closure slot is replaced in a copy, not in the caller's operands")
	}
	// A window the plan fills: the handler's own error stands.
	if err := nativeSplitRaise(r, "zzfold", sp, []core.Value{closure, core.NewList([]core.Value{core.NewInteger(1)}), zero}, nil, 0); err != nil {
		t.Errorf("0 zzfold [add] [1] finds the 3-operand form: no raise, got %v", err)
	}
	for _, at := range []int{-1, 3} {
		bad := &compiler.NativeSplit{NFwd: 2, BodyAt: at, Body: body}
		if err := nativeSplitRaise(r, "zzfold", bad, []core.Value{closure, s, zero}, nil, 0); err != nil {
			t.Errorf("a body slot at %d is out of range: no raise, got %v", at, err)
		}
	}
}

// TestNativeSplitRaisePlansTheSurround pins NUR283: the arm plans the whole
// tape the interpreter's plan could reach — the constants beneath the stack
// operands and the source tokens after the written ones (NativeSplit's
// Beneath and After) — so `7 0 zzfold [add] 's'` raises the no-match over
// that tape, and a plan that TAKES a token after the operands (`zzfold [add]
// [1] 5`, the 3-operand form collecting the 5) is a dispatch the program
// never assembled: a designed defer, never the handler's bare error.
func TestNativeSplitRaisePlansTheSurround(t *testing.T) {
	r := newTestRegistry(t)
	impl := core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(1)}, nil
	})
	r.Register("zzfold",
		core.Signature{Args: []*core.Type{core.TList, core.TMap}, NoEvalArgs: map[int]bool{0: true}, BarrierPos: 2, Impl: impl},
		core.Signature{Args: []*core.Type{core.TList, core.TList, core.TAny}, NoEvalArgs: map[int]bool{0: true}, BarrierPos: 3, Impl: impl},
	)
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	body := core.NewList([]core.Value{core.NewWord("add")})
	body.Quoted = true
	closure, s, zero := core.NewInteger(99), core.NewString("s"), core.NewInteger(0)
	sp := &compiler.NativeSplit{NFwd: 2, BodyAt: 0, Body: body, Beneath: []core.Value{core.NewInteger(7)}, After: []core.Value{core.NewWord("zzfold")}}
	err := nativeSplitRaise(r, "zzfold", sp, []core.Value{closure, s, zero}, []core.SrcPos{{Row: 1, Col: 5}}, 0)
	if be, ok := err.(*core.BoruError); !ok || be.Code != "signature_error" || !strings.Contains(err.Error(), "[word(add)]") {
		t.Fatalf("7 0 zzfold [add] 's' zzfold: the interpreter's no-match over its tape; got %v", err)
	}
	// The 3-operand form collects the literal after the written pair.
	one := core.NewList([]core.Value{core.NewInteger(1)})
	reach := &compiler.NativeSplit{NFwd: 2, BodyAt: 0, Body: body, After: []core.Value{core.NewInteger(5)}}
	err = nativeSplitRaise(r, "zzfold", reach, []core.Value{closure, one}, nil, 0)
	if be, ok := err.(*core.BoruError); !ok || !be.VMDefer || !strings.Contains(err.Error(), "NUR283") {
		t.Errorf("zzfold [add] [1] 5: the plan takes the 5, a designed defer; got %v", err)
	}
	// Over its own operands the same plan is the dispatch the program made.
	if err := nativeSplitRaise(r, "zzfold", &compiler.NativeSplit{NFwd: 3, BodyAt: 0, Body: body, Beneath: []core.Value{core.NewInteger(7)}},
		[]core.Value{closure, one, zero}, nil, 0); err != nil {
		t.Errorf("7 zzfold [add] [1] 0: the plan fills the operands, no raise; got %v", err)
	}
}
