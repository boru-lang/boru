package modules

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	_ "github.com/boru-lang/boru/eng/go" // the compiled runtime's closure bridge
	"github.com/boru-lang/boru/lang/go/native"
)

// cover_merge511_macro_test.go pins the macro fn-dispatch natives' arms
// (main's #510) the corpus never reaches — the merged ADR-008 gate on the
// reverse-order NUR run's merge of main's #511 — each with the verdict the
// arm exists to give.

// A compiled closure the bridge cannot describe (met outside any VM run) is
// no usable fn value: emit and mini raise their own contract errors.
func TestMacroFnDispatchUnbridgedClosure(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: &compiler.Program{}, Unit: 0, InShape: compiler.ClosureInValue, Ident: core.NewFnIdentity()})
	data := native.NewMap(native.NewOrderedMap())
	if _, err := emitFnDispatchHandler([]native.Value{cl, data}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "not a usable function value") {
		t.Errorf("emit over an unbridged closure: %v", err)
	}
	if _, err := miniFnDispatchHandler([]native.Value{cl, native.NewString("ab")}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "not a usable function value") {
		t.Errorf("mini over an unbridged closure: %v", err)
	}
}

// mini's value form: written options reach the transducer, and a FILTER-
// shaped fn (a third input) yields its partial rather than running.
func TestMiniFnDispatchOptionsAndFilter(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mk := func(params ...native.FnParam) native.Value {
		return native.NewFunction(native.FnDefInfo{Signatures: []native.Signature{{
			Params: params, Returns: []*native.Type{native.TAny}, BarrierPos: -1,
			Impl: core.Boru([]native.Value{native.NewWord("opts")}),
		}}})
	}
	src := native.FnParam{Name: "src", Type: native.TString}
	opts := native.FnParam{Name: "opts", Type: native.TMap}
	om := native.NewOrderedMap()
	om.Set("k", native.NewInteger(1))
	res, err := miniFnDispatchHandler([]native.Value{mk(src, opts), native.NewString("ab"), native.NewMap(om)}, nil, nil, r)
	if err != nil || len(res) != 1 || !strings.Contains(res[0].String(), "k") {
		t.Errorf("the written options reach the transducer: %v / %v", res, err)
	}
	res, err = miniFnDispatchHandler([]native.Value{mk(src, opts, native.FnParam{Name: "x", Type: native.TAny}), native.NewString("ab")}, nil, nil, r)
	if err != nil || len(res) != 1 || !res[0].Parent.ConformsTo(native.TFunction) {
		t.Errorf("a filter-shaped fn yields its partial: %v / %v", res, err)
	}
}

// closureFnView over a bridged closure with no declared return: the bridge's
// signature takes the anonymous lambda's single `Any`, as the interpreter's
// lambda carries it.
func TestClosureFnViewDefaultsTheReturn(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Invoker = func(*core.Registry, core.Value, []core.Value) ([]core.Value, error) { return nil, nil }
	p := &compiler.Program{Fns: []compiler.CompiledFn{{Name: "p$body", NParams: 2, NArgs: 2, NLocals: 2,
		Params: []*core.Type{core.TString, core.TMap}, Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: []core.SrcPos{{}}}}}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 0, InShape: compiler.ClosureInValue, Ident: core.NewFnIdentity()})
	bv, ok := closureFnView(r, cl)
	fd, isFn := bv.Data.(native.FnDefInfo)
	if !ok || !isFn || len(fd.Signatures) == 0 {
		t.Fatalf("the closure bridges: %v %v", bv, ok)
	}
	if rets := fd.Signatures[0].Returns; len(rets) != 1 || rets[0] != native.TAny {
		t.Errorf("no declared return reads as the lambda's single Any, got %v", rets)
	}
}
