package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fakePendingRec overrides the two seam methods the produced-closure apply
// record site uses, so its arms are testable without a real EmitState
// (compiler is above this module).
type fakePendingRec struct {
	core.EmitRecorder
	fn      core.Value
	has     bool
	applyOK bool
	window  []core.Value
	out     core.Value
}

func (f *fakePendingRec) PendingClosureApply([]core.Value) (core.Value, bool) { return f.fn, f.has }
func (f *fakePendingRec) RecordDynApply(args []core.Value, _, out core.Value, _ core.SrcPos) (int, bool) {
	f.window, f.out = args, out
	return len(args), f.applyOK
}

// TestRecordPendingClosureApplyArms pins the twenty-eighth increment's
// record-site guards: the out-count and arg-count gates, the pending
// lookup, the recorder's decline, the freshened carrier (parent, Dynamic,
// a nil parent as Any), the fn-VALUE out kept under a fresh id, and the
// window reversal (signature order in, tape order out).
func TestRecordPendingClosureApplyArms(t *testing.T) {
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true})
	rec := &fakePendingRec{EmitRecorder: core.TheInactiveEmit, fn: fn, has: true, applyOK: true}
	args := []core.Value{core.NewInteger(5), core.NewInteger(6)}
	carrierOut := []core.Value{core.NewCarrier(core.TInteger)}

	if _, ok := recordPendingClosureApply(rec, nil, args, nil, core.SrcPos{}); ok {
		t.Error("a 0-out call must decline")
	}
	if _, ok := recordPendingClosureApply(rec, nil, args, []core.Value{core.NewInteger(1), core.NewInteger(2)}, core.SrcPos{}); ok {
		t.Error("a 2-out call must decline")
	}
	if _, ok := recordPendingClosureApply(rec, nil, nil, carrierOut, core.SrcPos{}); ok {
		t.Error("a 0-arg call must decline")
	}
	rec.has = false
	if _, ok := recordPendingClosureApply(rec, nil, args, carrierOut, core.SrcPos{}); ok {
		t.Error("no pending entry: must decline")
	}
	rec.has, rec.applyOK = true, false
	if _, ok := recordPendingClosureApply(rec, nil, args, carrierOut, core.SrcPos{}); ok {
		t.Error("the recorder's decline must propagate")
	}
	rec.applyOK = true
	fresh, ok := recordPendingClosureApply(rec, nil, args, carrierOut, core.SrcPos{})
	if !ok || !fresh.Carrier || fresh.Parent != core.TInteger || fresh.ID == carrierOut[0].ID || fresh.Dynamic {
		t.Errorf("a carrier out must freshen to a strict carrier of its type: %+v", fresh)
	}
	if len(rec.window) != 2 || !core.ValuesEqual(rec.window[0], args[1]) || !core.ValuesEqual(rec.window[1], args[0]) {
		t.Errorf("the window must be the args in tape order (sig[0] on top): %v", rec.window)
	}
	if rec.out.ID != fresh.ID {
		t.Error("the recorder must see the freshened out")
	}
	dyn := core.NewCarrier(core.TAny)
	dyn.Dynamic = true
	if fresh, ok := recordPendingClosureApply(rec, nil, args, []core.Value{dyn}, core.SrcPos{}); !ok || !fresh.Dynamic {
		t.Error("a gradual out stays gradual")
	}
	if fresh, ok := recordPendingClosureApply(rec, nil, args, []core.Value{{}}, core.SrcPos{}); !ok || fresh.Parent != core.TAny {
		t.Error("an out with no parent freshens as Any")
	}
	lam := core.NewFunction(core.FnDefInfo{Anonymous: true})
	fresh, ok = recordPendingClosureApply(rec, nil, args, []core.Value{lam}, core.SrcPos{})
	if _, isFn := fresh.Data.(core.FnDefInfo); !ok || !isFn || fresh.ID == lam.ID {
		t.Error("a fn-value out keeps its payload under a fresh id")
	}
}

// fakeOrderRec is fakePendingRec with the name fallback's seam observable.
type fakeOrderRec struct {
	fakePendingRec
	named bool
}

func (f *fakeOrderRec) RecordDynApplyName(string, []core.Value, core.Value, core.Value, core.SrcPos) bool {
	f.named = true
	return true
}

// TestRecordUserCallOrApplyPendingFirst pins the record site's order (the
// thirty-first increment): a re-step dispatch that is both a pending
// closure apply and a name the fallback could re-dispatch by (`3 p/v
// apply` over a def-bound produced closure) takes the pending route, and
// the name fallback only when no entry is pending.
func TestRecordUserCallOrApplyPendingFirst(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := []core.Value{core.NewInteger(1)}
	top := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{Impl: &core.BoruImpl{Body: body}}}})
	r.Defs.Push("p", top)
	captures := []core.CapturedBinding{{Name: "k", Value: core.NewCarrier(core.TInteger)}}
	args := []core.Value{core.NewInteger(3)}
	outs := []core.Value{core.NewCarrier(core.TInteger)}
	rec := &fakeOrderRec{fakePendingRec: fakePendingRec{EmitRecorder: core.TheInactiveEmit, fn: top, has: true, applyOK: true}}
	got := recordUserCallOrApply(rec, r, "p", captures, body, 0, args, outs)
	if rec.named || len(rec.window) != 1 || got[0].ID == outs[0].ID {
		t.Errorf("the pending route runs first: named=%v window=%v", rec.named, rec.window)
	}
	rec.has, rec.window = false, nil
	got = recordUserCallOrApply(rec, r, "p", captures, body, 0, args, outs)
	if !rec.named || rec.window != nil || got[0].ID == outs[0].ID {
		t.Errorf("with no pending entry the name fallback takes the dispatch: named=%v window=%v", rec.named, rec.window)
	}
}
