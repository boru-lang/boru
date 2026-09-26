package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNUR265HandlerRunsOverItsArity pins the compile-time handler run
// (concreteHandlerEval): a window shorter than the signature never reaches
// the handler, and the fold over it declines. A recovery's assumed `gt`
// over one operand — `filter (mk) [gt 1]`'s auto-evaluated list — reached a
// handler that indexes its second argument, and the check pass panicked.
// The full window runs as before.
func TestNUR265HandlerRunsOverItsArity(t *testing.T) {
	r := newTestRegistry(t)
	calls := 0
	sig := &core.Signature{
		Args:          []*core.Type{core.TInteger, core.TInteger},
		CompileEffect: core.CompileScalarFold,
		Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			calls++
			return []core.Value{args[1]}, nil
		}),
	}
	one := []core.Value{core.NewInteger(1)}
	if _, ok := concreteHandlerEval(r, sig, one); ok || calls != 0 {
		t.Errorf("a short window never reaches the handler: ok=%v calls=%d", ok, calls)
	}
	if _, ok := tryFoldScalarConst(r, sig, one); ok || calls != 0 {
		t.Errorf("the fold over a short window declines: ok=%v calls=%d", ok, calls)
	}
	v, ok := concreteHandlerEval(r, sig, []core.Value{core.NewInteger(1), core.NewInteger(2)})
	if n, err := core.AsInteger(v); !ok || err != nil || n != 2 || calls != 1 {
		t.Errorf("the full window runs the handler: %v %v calls=%d", v, ok, calls)
	}
}
