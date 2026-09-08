package check

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fn_read_arrival_test.go pins the thirty-fifth increment's read model
// (tryShapedFnReadArrival): the WORD DISPATCH of a def-bound computed fn
// whose arity its producing word claimed (`def k (FnUtil.const 7)  (k 99)`)
// is modelled where the interpreter dispatches it — at the read, over the
// wrapper's whole arity of evaluation-fixed tokens inside the statement —
// as one guarded OpCallDynMethod, and every other window REFUSES rather
// than declining to the residual classifier.

// fraFix arms a pass with the recorder double, mints the fn-typed carrier the
// check pass substitutes for the read, notes the read under its name and
// claims the wrapper's arity.
func fraFix(t *testing.T, arity int) (*core.Registry, *zzmsRecorder, core.Value, func()) {
	t.Helper()
	r := zzmsReg(t)
	done := r.Check.Begin()
	rec := newZZMSRecorder()
	r.Check.Emit = rec
	carrier := core.NewCarrier(core.TFunction)
	rec.defReads = map[string]string{carrier.ID: "k"}
	r.Check.NoteFnShape(carrier, arity)
	return r, rec, carrier, done
}

func TestShapedFnReadArrivalSuccess(t *testing.T) {
	r, rec, carrier, done := fraFix(t, 1)
	defer done()
	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(99), core.NewInteger(2), core.NewEnd()})
	if !tryMemberFnArrivalDispatch(e, 0) {
		t.Fatal("the read model must consume the wrapper's arity window")
	}
	if len(rec.dynCalls) != 1 {
		t.Fatalf("exactly one RecordDynMethod event, got %d", len(rec.dynCalls))
	}
	call := rec.dynCalls[0]
	if call.word != "k" || call.fn.ID != carrier.ID {
		t.Errorf("event = %q over %q, want the def name over the read carrier", call.word, call.fn.ID)
	}
	if len(call.args) != 1 || len(call.outs) != 1 {
		t.Fatalf("recorded arity = %d args / %d outs, want 1/1", len(call.args), len(call.outs))
	}
	if n, _ := call.args[0].AsConcreteInteger(); n != 99 {
		t.Errorf("the window's token is the argument, got %v", call.args[0])
	}
	if e.Tape.Len() != 3 {
		t.Fatalf("the splice claims the ARITY window only, tape len = %d", e.Tape.Len())
	}
	if out := e.Tape.At(0); !out.Carrier || !out.Dynamic {
		t.Errorf("the modelled result is one dynamic value, got %v", out)
	}
	if n, _ := e.Tape.At(1).AsConcreteInteger(); n != 2 {
		t.Error("the token past the arity stays on the tape (`(k 1 2)` is `7 2`)")
	}
	if len(rec.reasons) != 0 {
		t.Errorf("a consumed window refuses nothing, got %v", rec.reasons)
	}
}

func TestShapedFnReadArrivalZeroArity(t *testing.T) {
	r, rec, carrier, done := fraFix(t, 0)
	defer done()
	e := zzmsEngine(r, []core.Value{carrier, core.NewEnd()})
	if !tryMemberFnArrivalDispatch(e, 0) {
		t.Fatal("a 0-param wrapper applies on its bare read (`(p)` over a partial of a unary fn)")
	}
	if len(rec.dynCalls) != 1 || len(rec.dynCalls[0].args) != 0 {
		t.Fatalf("an empty-window arity-0 event, got %v", rec.dynCalls)
	}
	if e.Tape.Len() != 2 {
		t.Errorf("the carrier alone is replaced, tape len = %d", e.Tape.Len())
	}
}

func TestShapedFnReadArrivalRefusals(t *testing.T) {
	cases := []struct {
		name   string
		arity  int
		tape   func(c core.Value) []core.Value
		dynOK  bool
		reason string
	}{
		{"window past the tape end", 1, func(c core.Value) []core.Value { return []core.Value{c} }, true, "ends short of the wrapper's arity"},
		{"statement end inside the window", 2, func(c core.Value) []core.Value {
			return []core.Value{c, core.NewInteger(3), core.NewEnd(), core.NewInteger(5)}
		}, true, "ends short of the wrapper's arity"},
		{"a word inside the window", 1, func(c core.Value) []core.Value {
			return []core.Value{c, core.NewWord("x"), core.NewEnd()}
		}, true, "not an evaluation-fixed value"},
		{"an operand with no compiled home", 1, func(c core.Value) []core.Value {
			return []core.Value{c, core.NewInteger(99), core.NewEnd()}
		}, false, "no compiled home"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, rec, carrier, done := fraFix(t, c.arity)
			defer done()
			rec.dynOK = c.dynOK
			tape := c.tape(carrier)
			e := zzmsEngine(r, tape)
			if tryMemberFnArrivalDispatch(e, 0) {
				t.Error("the model must not consume this window")
			}
			if e.Tape.Len() != len(tape) {
				t.Error("a refused read must leave the tape untouched")
			}
			if len(rec.reasons) != 1 || !strings.Contains(rec.reasons[0], c.reason) || !strings.Contains(rec.reasons[0], "`k`") {
				t.Errorf("refusal = %v, want one naming `k` with %q", rec.reasons, c.reason)
			}
		})
	}
}

// Shapes that are not the model's decline SILENTLY — no refusal, nothing
// recorded — and the sibling member model declines them too.
func TestShapedFnReadArrivalDeclines(t *testing.T) {
	r, rec, carrier, done := fraFix(t, 1)
	defer done()
	quoted := carrier
	quoted.Quoted = true
	noID := carrier
	noID.ID = ""
	unread := core.NewCarrier(core.TFunction) // claimed, but no read noted
	r.Check.NoteFnShape(unread, 1)
	unclaimed := core.NewCarrier(core.TFunction) // read, but no claim
	rec.defReads[unclaimed.ID] = "u"
	notFn := core.NewCarrier(core.TInteger) // read and claimed, not a fn carrier
	rec.defReads[notFn.ID] = "n"
	r.Check.NoteFnShape(notFn, 1)
	for _, v := range []core.Value{quoted, noID, unread, unclaimed, notFn} {
		tape := []core.Value{v, core.NewInteger(99), core.NewEnd()}
		e := zzmsEngine(r, tape)
		if tryMemberFnArrivalDispatch(e, 0) {
			t.Errorf("%v: not the model's shape", v)
		}
		if e.Tape.Len() != len(tape) || len(rec.reasons) != 0 || len(rec.dynCalls) != 0 {
			t.Errorf("%v: a silent decline leaves everything untouched (reasons %v, calls %d)", v, rec.reasons, len(rec.dynCalls))
		}
	}
}

// ─── the PLAIN-check half ────────────────────────────────────────────

// frwFix arms a plain pass (the inactive recorder), binds the carrier under
// `k` in the fn-carrier side table and claims the wrapper's arity.
func frwFix(t *testing.T, arity int) (*core.Registry, core.Value, func()) {
	t.Helper()
	r := zzmsReg(t)
	done := r.Check.Begin()
	carrier := core.NewCarrier(core.TFunction)
	core.NoteCheckFnCarrierBind(r, "k", carrier)
	r.Check.NoteFnShape(carrier, arity)
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
	if out := e.Tape.At(0); !out.Carrier || !out.Dynamic {
		t.Errorf("the dispatch's one result is dynamic, got %v", out)
	}
	if n, _ := e.Tape.At(1).AsConcreteInteger(); n != 9 {
		t.Error("the token past the arity stays on the tape")
	}
}

func TestShapedFnReadWindowPlainCheckDeclines(t *testing.T) {
	r, carrier, done := frwFix(t, 1)
	defer done()
	quoted := carrier
	quoted.Quoted = true
	unbound := core.NewCarrier(core.TFunction) // claimed, but not in the side table
	r.Check.NoteFnShape(unbound, 1)
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
