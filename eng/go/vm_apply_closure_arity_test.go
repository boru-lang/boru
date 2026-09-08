package eng

import (
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	"github.com/boru-lang/boru/core/go"
)

// TestCallDynApplyClosureArity pins the apply op's closure arm (the
// twenty-eighth increment): the arity it compares with the window is the
// unit's PARAM slots alone — NParams counts the trailing capture slots too,
// and reading it whole sent every capturing closure to the interpreter's
// re-step island — so a closure of ANOTHER arity, and only that, takes the
// re-step, which parks the closure over the value it cannot bind exactly
// as the interpreter's applyHandler does. No compiling program reaches this
// arm today (the check engine's own re-step matches the closure's arity
// before it records the apply, and a closure of another arity refuses at
// the record site), so the arm is pinned here at the seam.
func TestCallDynApplyClosureArity(t *testing.T) {
	r := seam7Reg(t)
	prog := &compiler.Program{Fns: []compiler.CompiledFn{{
		Name: "cl", NParams: 3, NCaptures: 1, NArgs: 2, NLocals: 3,
		Params: []*core.Type{core.TInteger, core.TInteger},
	}}}
	vc := &vmContext{p: prog, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: prog, Unit: 0, Captures: []core.Value{core.NewInteger(1)}, Ident: core.NewFnIdentity()})
	// Two param slots over a one-wide window: the re-step, which leaves the
	// closure parked above the value (the interpreter's under-application).
	got, ent, err := vc.callDynApply(r, 1, []core.Value{core.NewInteger(4), cl}, seam7Dbg, 0, false)
	if err != nil || ent != nil || len(got) != 2 {
		t.Fatalf("a closure of another arity takes the re-step and parks: %v %v %v", got, ent, err)
	}
	if n, _ := core.AsInteger(got[0]); n != 4 || !got[1].Parent.Equal(core.TFunction) {
		t.Errorf("the value stays beneath the parked closure: %v", got)
	}
	// The event form commits exactly one result: the parked pair defers.
	_, _, err = vc.callDynApply(r, 1, []core.Value{core.NewInteger(4), cl}, seam7Dbg, 0, true)
	wantInternal(t, err, "netted 2 value(s)")
}
