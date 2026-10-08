package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestAnyPendingActiveContainer: a fn-value apply window holding a map or
// list literal the interpreter still evaluates before binding it is not a
// window of plain operands (NUR337) — recordDynApply leaves it unrecorded
// and the body-tail lowering declines it. A window of inert values, and a
// literal with nothing to evaluate, is.
func TestAnyPendingActiveContainer(t *testing.T) {
	pending := core.NewList([]core.Value{core.NewInteger(1), core.NewWord("add"), core.NewInteger(2)})
	pending.Eval = true
	inert := core.NewList([]core.Value{core.NewInteger(1)})
	inert.Eval = true
	if !anyPendingActiveContainer([]core.Value{core.NewInteger(5), pending}) {
		t.Error("a window with a pending `[1 add 2]` must be refused")
	}
	if anyPendingActiveContainer([]core.Value{core.NewInteger(5), inert}) {
		t.Error("a window of inert values must be admitted")
	}
	if anyPendingActiveContainer(nil) {
		t.Error("an empty window holds no pending container")
	}
}
