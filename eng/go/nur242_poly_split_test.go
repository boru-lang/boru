package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestPolySplitRaiseIsTheInterpretersPlan pins the poly no-match arm over a
// recorded layout (PolyRef.Split, NUR242), on a word shaped like fold: a
// 2-operand and a 3-operand overload. The window lists the operands in
// signature order, the written ones first. `0 zzfold [add] 's'` is two
// written and one beneath: the interpreter's plan fills neither overload
// and its report names the two written values. The same values with one
// written, and the 3-operand window that fits, find a plan and leave the
// caller's path; so does a window the host cannot plan, a missing layout
// or an out-of-range count.
func TestPolySplitRaiseIsTheInterpretersPlan(t *testing.T) {
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
	fn := r.Lookup("zzfold")
	body := core.NewList([]core.Value{core.NewWord("add")})
	body.Quoted = true
	s, zero := core.NewString("s"), core.NewInteger(0)
	pr := &compiler.PolyRef{Word: "zzfold", Arity: 3, NOut: 1, Split: &compiler.PolySplit{NFwd: 2}}
	err := polySplitRaise(r, pr, fn, []core.Value{body, s, zero}, nil, nil, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "signature_error") || !strings.Contains(err.Error(), "'s'") {
		t.Fatalf("0 zzfold [add] 's' is the interpreter's no-match over the written pair, got %v", err)
	}
	// A plan that fits: `0 zzfold [add] [1 2]` takes the 3-operand form.
	if err := polySplitRaise(r, pr, fn, []core.Value{body, core.NewList([]core.Value{core.NewInteger(1)}), zero}, nil, nil, nil, 0); err != nil {
		t.Errorf("a window the plan fills keeps the caller's path, got %v", err)
	}
	// A written template string needs an evaluation: no plan.
	tpl := core.NewInterpString([]core.InterpPart{{Lit: "a"}, {Expr: []core.Value{core.NewInteger(1)}}})
	if err := polySplitRaise(r, pr, fn, []core.Value{body, tpl, zero}, nil, nil, nil, 0); err != nil {
		t.Errorf("an unplannable window keeps the caller's path, got %v", err)
	}
	for _, c := range []struct {
		name string
		pr   *compiler.PolyRef
		fn   *core.FnDefInfo
	}{
		{"no layout", &compiler.PolyRef{Word: "zzfold", Arity: 3}, fn},
		{"no binding", pr, nil},
		{"a negative count", &compiler.PolyRef{Word: "zzfold", Arity: 3, Split: &compiler.PolySplit{NFwd: -1}}, fn},
		{"a count past the window", &compiler.PolyRef{Word: "zzfold", Arity: 3, Split: &compiler.PolySplit{NFwd: 4}}, fn},
	} {
		if err := polySplitRaise(r, c.pr, c.fn, []core.Value{body, s, zero}, nil, nil, nil, 0); err != nil {
			t.Errorf("%s: no raise, got %v", c.name, err)
		}
	}
	// The raise is anchored where the op's debug entry says, as the
	// interpreter's is at the word.
	err = polySplitRaise(r, pr, fn, []core.Value{body, s, zero}, nil, nil, []core.SrcPos{{Row: 3, Col: 7}}, 0)
	if be, ok := err.(*core.BoruError); !ok || be.Row != 3 || be.Col != 7 {
		t.Errorf("the raise sits at the op's position, got %v", err)
	}
}
