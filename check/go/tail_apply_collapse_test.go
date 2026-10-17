package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

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
	if got := collapseElidedTailApply([]core.Value{apply}, []core.Value{fnCarrier}, true); len(got) != 1 || !core.IsFnTypedCarrier(&got[0]) {
		t.Error("a one-value residual stays")
	}
	if got := collapseElidedTailApply([]core.Value{one, core.NewWord("add")}, stk, true); len(got) != 2 {
		t.Error("a body ending in another word stays")
	}
	if got := collapseElidedTailApply([]core.Value{one, apply}, []core.Value{one, one}, true); len(got) != 2 {
		t.Error("a residual whose top is data stays")
	}
	got := collapseElidedTailApply([]core.Value{one, apply}, stk, true)
	if len(got) != 1 || !got[0].Carrier() || !core.IsDynamic(&got[0]) || got[0].Parent() != core.TAny {
		t.Errorf("the residual collapses to one gradual result: %v", got)
	}
	if len(stk) != 2 {
		t.Error("the caller's residual must not be mutated")
	}
}
