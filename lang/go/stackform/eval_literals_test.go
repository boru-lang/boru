package stackform

import (
	"fmt"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestEvalLiteralsOnlyMatchesTheEngine: a form of plain literal pushes alone
// answers without an engine run, and the answer is exactly the engine's
// replay of the same form — value, identity and Quoted mark alike — for a
// scalar, an identified list, a quoted list and a map; any other form (a call,
// a Function push) replays on the engine as before.
func TestEvalLiteralsOnlyMatchesTheEngine(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	lst := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	core.SetID(&lst, "L_shrink")
	qlst := core.NewList([]core.Value{core.NewInteger(3)})
	core.SetQuoted(&qlst, true)
	om := core.NewOrderedMap()
	om.Set("a", core.NewInteger(1))
	for _, v := range []core.Value{core.NewInteger(7), core.NewString("b"), lst, qlst, core.NewMap(om)} {
		form := &StackForm{Ops: []Op{PushLit{V: v}}}
		lits, only := plainLiterals(form)
		if !only || len(lits) != 1 {
			t.Fatalf("%v: a plain literal push is answered directly", v)
		}
		got, err := Eval(reg, form)
		if err != nil || len(got) != 1 {
			t.Fatalf("%v: %v %v", v, got, err)
		}
		engine, err := core.NewTop(reg).Run(Flatten(form))
		if err != nil || len(engine) != 1 {
			t.Fatalf("%v: the engine's replay %v %v", v, engine, err)
		}
		if core.Canon(got) != core.Canon(engine) || core.IDOf(&got[0]) != core.IDOf(&engine[0]) || core.IsQuoted(&got[0]) != core.IsQuoted(&engine[0]) {
			t.Errorf("%v: the direct answer %v (id %q quoted %v) is not the engine's %v (id %q quoted %v)", v, got, core.IDOf(&got[0]), core.IsQuoted(&got[0]), engine, core.IDOf(&engine[0]), core.IsQuoted(&engine[0]))
		}
	}
	// Two pushes are two values, in order.
	two := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}, PushLit{V: core.NewInteger(2)}}}
	if got, err := Eval(reg, two); err != nil || fmt.Sprint(got) != "[1 2]" {
		t.Errorf("two plain pushes: %v %v", got, err)
	}
	// Not plain: a call (the engine replays it — an unbound word raises), a
	// Function push (the engine's dispatch, stamped Quoted for the replay), a
	// nested quote, an empty or nil form.
	fn := core.NewFunction(core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}}}})
	for _, form := range []*StackForm{
		{Ops: []Op{PushLit{V: core.NewInteger(1)}, Call{Name: "no-such-word", Arity: 1}}},
		{Ops: []Op{PushLit{V: fn}}},
		{Ops: []Op{Quote{Body: &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}}}}}},
		{},
		nil,
	} {
		if _, only := plainLiterals(form); only {
			t.Errorf("%v: not a form of plain literals", form)
		}
	}
	if _, err := Eval(reg, &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}, Call{Name: "no-such-word", Arity: 1}}}); err == nil {
		t.Error("a call replays on the engine, which raises on the unbound word")
	}
	if !plainLiteral(core.NewInteger(1)) || plainLiteral(core.Value{}) || plainLiteral(fn) {
		t.Error("plain: a typed scalar; not plain: an untyped value, a Function")
	}
	// The literals the engine STEPS rather than pushes are not plain either.
	for _, v := range []core.Value{
		core.NewParenExpr([]core.Value{core.NewInteger(1)}),
		core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)})),
		core.NewSugar(core.SugarInfo{}),
		core.NewReach(core.ReachInfo{}),
	} {
		if plainLiteral(v) {
			t.Errorf("%v: a stepped literal is not plain", v)
		}
	}
}
