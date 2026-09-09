package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fakeTailRec overrides the tail-apply query on the inactive recorder.
type fakeTailRec struct {
	core.EmitRecorder
	n  int
	ok bool
}

func (f fakeTailRec) UnitTailApply(int) (int, bool) { return f.n, f.ok }

// TestCollapseTailApplyArms pins the call-site collapse (the twenty-ninth
// increment): no tail apply leaves the residual alone; a residual that is
// not exactly the window (shorter, or wider — a value beneath the applied
// result is the interpreter's count error), or one whose top is neither a
// fn-typed carrier nor a fn value, is left alone; the window collapses to
// one gradual result.
func TestCollapseTailApplyArms(t *testing.T) {
	fnCarrier := core.NewCarrier(core.TFunction)
	one, two := core.NewInteger(1), core.NewInteger(2)
	if got := collapseTailApply(fakeTailRec{EmitRecorder: core.TheInactiveEmit}, 0, []core.Value{two, fnCarrier}); len(got) != 2 {
		t.Error("no tail apply: the residual stays")
	}
	rec := fakeTailRec{EmitRecorder: core.TheInactiveEmit, n: 1, ok: true}
	if got := collapseTailApply(rec, 0, []core.Value{fnCarrier}); len(got) != 1 || !core.IsFnTypedCarrier(got[0]) {
		t.Error("a residual shorter than the window stays")
	}
	if got := collapseTailApply(rec, 0, []core.Value{one, two, fnCarrier}); len(got) != 3 {
		t.Error("a residual wider than the window stays (a value beneath the result)")
	}
	if got := collapseTailApply(rec, 0, []core.Value{one, two}); len(got) != 2 {
		t.Error("a residual whose top is data stays")
	}
	stk := []core.Value{two, fnCarrier}
	got := collapseTailApply(rec, 0, stk)
	if len(got) != 1 || !got[0].Carrier || !got[0].Dynamic || got[0].Parent != core.TAny {
		t.Errorf("the window [2 f] collapses to one gradual result: %v", got)
	}
	if len(stk) != 2 {
		t.Error("the caller's residual must not be mutated")
	}
	fnVal := core.NewFunction(core.FnDefInfo{Anonymous: true})
	if got := collapseTailApply(rec, 0, []core.Value{one, fnVal}); len(got) != 1 || !got[0].Dynamic {
		t.Error("a fn VALUE on top collapses too")
	}
}

// TestCollapseElidedTailApplyArms pins the plain check pass's twin: nothing
// for a tuple contract, an empty body, a one-value residual, a body whose
// last token is not the `apply` word, or a residual whose top is not a
// fn-typed carrier; the whole residual to one gradual result otherwise.
func TestCollapseElidedTailApplyArms(t *testing.T) {
	apply := core.NewWord("apply")
	fnCarrier := core.NewCarrier(core.TFunction)
	one := core.NewInteger(1)
	stk := []core.Value{one, fnCarrier}
	if got := collapseElidedTailApply([]core.Value{one, apply}, stk, false); len(got) != 2 {
		t.Error("a tuple contract keeps its residual")
	}
	if got := collapseElidedTailApply(nil, stk, true); len(got) != 2 {
		t.Error("an empty body: nothing to read")
	}
	if got := collapseElidedTailApply([]core.Value{apply}, []core.Value{fnCarrier}, true); len(got) != 1 || !core.IsFnTypedCarrier(got[0]) {
		t.Error("a one-value residual stays")
	}
	if got := collapseElidedTailApply([]core.Value{one, core.NewWord("add")}, stk, true); len(got) != 2 {
		t.Error("a body ending in another word stays")
	}
	if got := collapseElidedTailApply([]core.Value{one, apply}, []core.Value{one, one}, true); len(got) != 2 {
		t.Error("a residual whose top is data stays")
	}
	got := collapseElidedTailApply([]core.Value{one, apply}, stk, true)
	if len(got) != 1 || !got[0].Carrier || !got[0].Dynamic || got[0].Parent != core.TAny {
		t.Errorf("the residual collapses to one gradual result: %v", got)
	}
	if len(stk) != 2 {
		t.Error("the caller's residual must not be mutated")
	}
}
