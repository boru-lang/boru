package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fn_read_arrival_test.go pins the thirty-fifth increment's read model
// (tryShapedFnReadArrival): the WORD DISPATCH of a def-bound computed fn
// whose arity its producing word claimed (`def k (FnUtil.const 7)  (k 99)`)
// is modelled where the interpreter dispatches it — at the read, over the
// wrapper's whole arity of evaluation-fixed tokens inside the statement —
// as one guarded OpCallDynMethod, and every other window DECLINES rather
// than declining to the residual classifier.

// ─── the PLAIN-check half ────────────────────────────────────────────

// frwFix arms a plain pass (the inactive recorder), binds the carrier under
// `k` in the fn-carrier side table and claims the wrapper's arity.
func frwFix(t *testing.T, arity int) (*core.Registry, core.Value, func()) {
	t.Helper()
	r := zzmsReg(t)
	done := r.Check.Begin()
	carrier := core.NewCarrier(core.TFunction)
	core.NoteCheckFnCarrierBind(r, "k", carrier)
	r.Check.NoteFnShape(carrier, core.FnShape{Arity: arity})
	return r, carrier, done
}

func TestShapedFnReadWindowPlainCheck(t *testing.T) {
	r, carrier, done := frwFix(t, 2)
	defer done()
	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(3), core.NewInteger(5), core.NewInteger(9), core.NewEnd()})
	if !tryDynamicFnValueDispatch(e, 0) {
		t.Fatal("the plain check collapses the claimed window")
	}
	if e.Tape.Len() != 3 {
		t.Fatalf("the arity window collapses to one value, tape len = %d", e.Tape.Len())
	}
	if out := e.Tape.At(0); !out.Carrier() || !core.IsDynamic(&out) {
		t.Errorf("the dispatch's one result is dynamic, got %v", out)
	}
	if n, _ := addrOf(e.Tape.At(1)).AsConcreteInteger(); n != 9 {
		t.Error("the token past the arity stays on the tape")
	}
}

func TestShapedFnReadWindowPlainCheckDeclines(t *testing.T) {
	r, carrier, done := frwFix(t, 1)
	defer done()
	quoted := carrier
	core.SetQuoted(&quoted, true)
	unbound := core.NewCarrier(core.TFunction) // claimed, but not in the side table
	r.Check.NoteFnShape(unbound, core.FnShape{Arity: 1})
	unclaimed := core.NewCarrier(core.TFunction) // bound, no claim
	core.NoteCheckFnCarrierBind(r, "u", unclaimed)
	cases := map[string][]core.Value{
		"quoted":        {quoted, core.NewInteger(1), core.NewEnd()},
		"not def-bound": {unbound, core.NewInteger(1), core.NewEnd()},
		"no claim":      {unclaimed, core.NewInteger(1), core.NewEnd()},
		"short window":  {carrier, core.NewEnd()},
		"non-fixed":     {carrier, core.NewWord("x"), core.NewEnd()},
		"past the end":  {carrier},
	}
	for name, tape := range cases {
		e := zzmsEngine(r, tape)
		if tryDynamicFnValueDispatch(e, 0) || e.Tape.Len() != len(tape) {
			t.Errorf("%s: the plain check must leave the tape as it is", name)
		}
	}
}

// ─── the result shape ────────────────────────────────────────────────
