package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// zz_cover_merge519_keptdefs_test.go pins the kept-defs latch's live-read
// settlement (kept_defs.go keptReadSeatedLive / flushKeptRead, NUR282) and
// the parking-lambda token proof's shape screen (kept_defs_scan.go
// anonLambdaArity) directly. The whole-program halves are in lang/go
// (zz_cover_merge519_keptdefs_test.go); the fn-value and type-literal arms
// of the settlement are dialled here because every program that reaches
// them today declines first at lowering ("dynamic-scope def `t` of unknown
// provenance" — a type or fn value def'd in the unit), so no program row can
// show what the arm itself decides.

// m519PendingRead arms the latch and leaves v's read pending, as
// noteKeptDefsRead does for a read that may be seated live.
func m519PendingRead(t *testing.T, v core.Value, name string) *EmitState {
	t.Helper()
	es := NewEmitState()
	es.latchKeptDefs("do")
	es.pendingKeptRead, es.pendingKeptName = v.ID, name
	return es
}

// A read that may hold a fn value stays an OBSERVER when seated live: a fn
// is interned as a const the carrier would not stop, so the latch declines
// (NUR210) and the value is left as it was. Three shapes reach the arm: an
// FnDefInfo payload, a value with no Parent (the Any type literal) and a
// Function-typed carrier.
func TestMerge519KeptReadSeatedLiveFnValueObserves(t *testing.T) {
	fnv := zeroArgFn()
	anyLit := core.NewTypeLiteral(core.TAny)
	fnCarrier := core.NewCarrier(core.TFunction)
	for label, v := range map[string]core.Value{"fn value": fnv, "Any literal": anyLit, "Function carrier": fnCarrier} {
		v.ID = "m519-" + strings.ReplaceAll(label, " ", "-")
		if label == "Any literal" && v.Parent != nil {
			t.Fatalf("the Any type literal must have no Parent for this row, got %v", v.Parent)
		}
		es := m519PendingRead(t, v, "t")
		before := v
		es.keptReadSeatedLive(&v)
		if es.pendingKeptRead != "" || es.pendingKeptName != "" {
			t.Errorf("%s: the pending read is settled", label)
		}
		if !strings.Contains(es.armReadCompileFailure, "the read of `t`") || !strings.Contains(es.armReadCompileFailure, "(NUR210)") {
			t.Errorf("%s: a fn-valued live read is an observer and poisons the latch, got %q", label, es.armReadCompileFailure)
		}
		if v.Carrier != before.Carrier || v.Parent != before.Parent || v.ID != before.ID {
			t.Errorf("%s: the observer arm leaves the value alone, got %+v", label, v)
		}
	}
}

// A type VALUE seated live becomes a Type carrier (core.ValueCarrier,
// NUR323) — never a carrier of the node's Parent (Integer's is Number) — and
// observes nothing: the latch is not poisoned.
func TestMerge519KeptReadSeatedLiveTypeValue(t *testing.T) {
	v := core.NewTypeLiteral(core.TInteger)
	v.ID = "m519-type"
	v.SetPos(core.SrcPos{Row: 3, Col: 7})
	es := m519PendingRead(t, v, "t")
	es.keptReadSeatedLive(&v)
	if es.armReadCompileFailure != "" {
		t.Fatalf("a type value seated live observes nothing stale, got %q", es.armReadCompileFailure)
	}
	if !v.Carrier || v.Parent == nil || !v.Parent.Equal(core.TType) {
		t.Errorf("the type value becomes a Type carrier, got carrier=%v parent=%v", v.Carrier, v.Parent)
	}
	if p := v.Pos(); p.Row != 3 || p.Col != 7 {
		t.Errorf("the read keeps its position, got %v", p)
	}
	if es.pendingKeptRead != "" {
		t.Error("the pending read is settled")
	}
	// A plain value becomes a carrier of its Parent.
	w := core.NewInteger(5)
	w.ID = "m519-int"
	es = m519PendingRead(t, w, "t")
	es.keptReadSeatedLive(&w)
	if es.armReadCompileFailure != "" || !w.Carrier || !w.Parent.Equal(core.TInteger) {
		t.Errorf("a plain value becomes a carrier of its type: carrier=%v parent=%v poison=%q", w.Carrier, w.Parent, es.armReadCompileFailure)
	}
	// A value that is not the pending read is untouched.
	x := core.NewInteger(6)
	x.ID = "m519-other"
	es = m519PendingRead(t, w, "t")
	es.keptReadSeatedLive(&x)
	if x.Carrier || es.pendingKeptRead != w.ID {
		t.Error("another value's seat settles nothing")
	}
}

// A pending read nothing seated live before the next recorded event is an
// observer (flushKeptRead): the latch declines, naming the read, and the
// pending slot clears so a second flush does nothing.
func TestMerge519FlushKeptRead(t *testing.T) {
	es := NewEmitState()
	es.flushKeptRead()
	if es.armReadCompileFailure != "" {
		t.Fatal("nothing pending: the flush poisons nothing")
	}
	es.latchKeptDefs("each")
	es.pendingKeptRead, es.pendingKeptName = "m519-x", "x"
	es.flushKeptRead()
	if es.pendingKeptRead != "" || es.pendingKeptName != "" {
		t.Error("the flush settles the pending read")
	}
	if !strings.HasPrefix(es.armReadCompileFailure, "each: ") || !strings.Contains(es.armReadCompileFailure, "the read of `x`") {
		t.Errorf("an unseated pending read is an observer, got %q", es.armReadCompileFailure)
	}
	// The next event flushes too (keptDefsEvent), and the first poison wins.
	first := es.armReadCompileFailure
	es.pendingKeptRead, es.pendingKeptName = "m519-y", "y"
	es.keptDefsEvent(&EmitEvent{kind: evCall, call: emitCall{word: "add"}})
	if es.pendingKeptRead != "" || es.armReadCompileFailure != first {
		t.Errorf("the event flushes the pending read and the first poison stands, got %q", es.armReadCompileFailure)
	}
}

// anonLambdaArity's shape screen: a three-item paren that is not the arrow
// fold, and an arrow fold whose params or body is no concrete list, build no
// anonymous fn value.
func TestMerge519AnonLambdaArityShapes(t *testing.T) {
	lambda := core.NewSugar(core.SugarInfo{Kind: core.SugarLambda})
	params := core.NewList([]core.Value{core.NewWord("a"), core.NewWord("b")})
	body := core.NewList([]core.Value{core.NewWord("a")})
	if n, ok := anonLambdaArity(core.NewParenExpr([]core.Value{params, lambda, body})); !ok || n != 2 {
		t.Fatalf("([a b] => [a]) is a two-parameter lambda, got %d %v", n, ok)
	}
	nested := core.NewParenExpr([]core.Value{core.NewParenExpr([]core.Value{params, lambda, body})})
	if n, ok := anonLambdaArity(nested); !ok || n != 2 {
		t.Errorf("single-item paren nesting is seen through, got %d %v", n, ok)
	}
	for label, tk := range map[string]core.Value{
		"(1 add 2)":        core.NewParenExpr([]core.Value{core.NewInteger(1), core.NewWord("add"), core.NewInteger(2)}),
		"a sugar not =>":   core.NewParenExpr([]core.Value{params, core.NewSugar(core.SugarInfo{Kind: core.SugarUsurp}), body}),
		"(a => [a])":       core.NewParenExpr([]core.Value{core.NewWord("a"), lambda, body}),
		"([a b] => a)":     core.NewParenExpr([]core.Value{params, lambda, core.NewWord("a")}),
		"(carrier => [a])": core.NewParenExpr([]core.Value{core.NewCarrier(core.TList), lambda, body}),
		"a bare word":      core.NewWord("f"),
		"a two-item paren": core.NewParenExpr([]core.Value{params, lambda}),
	} {
		if n, ok := anonLambdaArity(tk); ok || n != 0 {
			t.Errorf("%s builds no anonymous fn value, got %d %v", label, n, ok)
		}
	}
}
