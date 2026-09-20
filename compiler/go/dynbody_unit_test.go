package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The DynEnv args-bracket primitives and tryRecordDynBody's decline guards —
// white-box coverage of the branches end-to-end programs cannot reach
// (defensive shapes, tail-swap variants).
func TestArgsStackDepthTruncate(t *testing.T) {
	var nilStack *core.ArgsStack
	if nilStack.Depth() != 0 {
		t.Errorf("nil Depth: want 0")
	}
	nilStack.Truncate(0) // no-op, no panic
	as := core.NewArgsStack()
	_ = as.Push(core.NewInteger(1))
	_ = as.Push(core.NewInteger(2))
	if as.Depth() != 2 {
		t.Errorf("Depth: want 2, got %d", as.Depth())
	}
	as.Truncate(5) // above depth: no-op
	if as.Depth() != 2 {
		t.Errorf("Truncate above depth must no-op")
	}
	as.Truncate(1)
	if as.Depth() != 1 {
		t.Errorf("Truncate: want 1, got %d", as.Depth())
	}
	as.Truncate(-1) // negative: no-op
	if as.Depth() != 1 {
		t.Errorf("negative Truncate must no-op")
	}
}

// tryRecordClosure's gradual-ambiguity gate: a multi-overload Callable word
// WITHOUT a poly re-match (no CompileDynBody) and WITHOUT the
// CrossCollectionTokenShape robustness flag must OWN the dispatch with an
// uncompilable mark — the checker's single committed overload may not be the
// one the runtime value needs, and there is no runtime re-match to correct
// it. Every shipping Callable word now carries one of the two escapes (do:
// CompileDynBody poly re-match; each/fold/scan: CrossCollectionTokenShape),
// so the arm is pinned white-box against a synthetic word. The positive twin
// (a CompileDynBody sig DECLINES here so the dyn-body poly takes the funnel)
// is pinned end-to-end by lang's TestDynBodyVariadicAndSpliceShapes gradual
// row.
func TestGradualAmbiguityMarksUncompilable(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	h := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	r.Register("grady",
		core.Signature{Args: []*core.Type{core.TList, core.TList}, NoEvalArgs: map[int]bool{0: true}, Impl: core.Go(h), Returns: []*core.Type{core.TList}, BarrierPos: -1},
		core.Signature{Args: []*core.Type{core.TList, core.TMap}, NoEvalArgs: map[int]bool{0: true}, Impl: core.Go(h), Returns: []*core.Type{core.TMap}, BarrierPos: -1},
	)
	done := r.Check.BeginCompilePass()
	defer done()
	sig := &core.Signature{
		Args:       []*core.Type{core.TList, core.TList},
		NoEvalArgs: map[int]bool{0: true},
		Callable: &core.CallableSpec{BodyPos: 0, BodyOut: 1, Inputs: func(_ []core.Value) []core.Value {
			return []core.Value{core.NewCarrier(core.TAny)}
		}},
	}
	body := core.NewList([]core.Value{core.NewInteger(1)})
	data := core.NewCarrier(core.TAny)
	data.Dynamic = true
	outs := []core.Value{core.NewCarrier(core.TList)}
	if !tryRecordClosure(r, "grady", sig, []core.Value{body, data}, outs, core.SrcPos{}) {
		t.Fatalf("gradual ambiguity without a poly re-match must be OWNED (true)")
	}
	es, _ := r.Check.Recorder().(*EmitState)
	if es == nil || es.Compilable || es.Reason == "" {
		t.Errorf("gradual ambiguity must mark uncompilable with a named reason; got es=%v", es)
	}
}

// tryRecordDynBody decline guards: a Callable whose BodyPos exceeds the args
// (a malformed dispatch shape) and an unresolvable operand both decline,
// leaving the ordinary compile failure path.
func TestTryRecordDynBodyDeclines(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	done := r.Check.BeginCompilePass()
	defer done()
	sig := &core.Signature{Callable: &core.CallableSpec{BodyPos: 2}, CompileEffect: core.CompileDynBody}
	outs := []core.Value{core.NewCarrier(core.TAny)}
	if tryRecordDynBody(r, "do", sig, []core.Value{core.NewInteger(1)}, outs, core.SrcPos{}) {
		t.Errorf("BodyPos beyond args must decline")
	}
	// An operand with no compiled home (an unregistered carrier) declines.
	sig2 := &core.Signature{Callable: &core.CallableSpec{BodyPos: 0}, CompileEffect: core.CompileDynBody}
	ghost := core.NewCarrier(core.TList)
	ghost.Dynamic = true
	if tryRecordDynBody(r, "do", sig2, []core.Value{ghost}, outs, core.SrcPos{}) {
		t.Errorf("unresolvable operand must decline")
	}
}
