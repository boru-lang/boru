package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// Merged-coverage wave 519: the VM arms (vm.go, vm_list_restep.go,
// vm_spec_guard.go) no suite reached. Each is reached here at its seam —
// the hand-built program or the in-package helper the arm lives in — and
// the behaviour it owes is asserted: a designed defer stays a loud VM defer,
// an island's raise propagates unchanged, a guard that does not hold
// refuses.

// TestMerge519PolyDynBodyPlain pins the poly re-match's plain-run check
// (PolyRef.DynBodyPlain, NUR213): a computed body's run that holds only data
// seats, and one holding a value the interpreter's tape would dispatch (a
// fn value) is the loud vm:dyn-body-plain defer, never seated as data.
func TestMerge519PolyDynBodyPlain(t *testing.T) {
	prog := &compiler.Program{
		Consts:   []core.Value{core.NewInteger(4)},
		Code:     []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNativePoly, Arg: 0}},
		Debug:    make([]core.SrcPos, 2),
		PolyRefs: []compiler.PolyRef{{Word: "zz-plain-poly", Arity: 1, NOut: 1, DynBodyPlain: true}},
	}
	fnVal := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	reg := func(out core.Value) *core.Registry {
		r := seam7Reg(t)
		r.Register("zz-plain-poly", core.Signature{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{out}, nil
			}),
		})
		if err := r.Err(); err != nil {
			t.Fatalf("registration: %v", err)
		}
		return r
	}
	res, err := RunProgram(prog, reg(core.NewInteger(7)))
	if err != nil || len(res) != 1 || res[0].String() != "7" {
		t.Fatalf("a plain run seats: got %v / %v, want [7]", res, err)
	}
	_, err = RunProgram(prog, reg(fnVal))
	if err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "where the run is seated as data") {
		t.Fatalf("a fn value in the run: want the loud dyn-body-plain defer, got %v", err)
	}
}

// TestMerge519LandingDataRestarts pins reStepLanding's data rung under a
// paren apply that takes the value as its method (LandingWord.Restart with
// SkipTo, NUR242): data is no method, so the statement's own island answers
// what the paren places and the run resumes at RetPC.
func TestMerge519LandingDataRestarts(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	lword := compiler.LandingWord{Restart: true, SkipTo: 3, Root: true, RetPC: 4, Island: []core.Value{core.NewInteger(9)}}
	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{core.NewInteger(3)}, seam7Dbg, 0, lword)
	if err != nil || ent == nil || !ent.jump || ent.jumpPC != 4 || len(got) != 1 || got[0].String() != "9" {
		t.Fatalf("data under a restarting landing runs the statement island: got %v %+v %v", got, ent, err)
	}
	// Without the paren apply's skip the data stays where it landed.
	got, ent, err = vc.reStepLanding(r, 0, 0, []core.Value{core.NewInteger(3)}, seam7Dbg, 0, compiler.LandingWord{Restart: true})
	if err != nil || ent != nil || len(got) != 1 || got[0].String() != "3" {
		t.Fatalf("data with no skip stays data: got %v %+v %v", got, ent, err)
	}
}

// TestMerge519LandingSkipCaptureRaises pins landingSkipCapture's island
// error arm (NUR190): a `/q` capture whose handler raises surfaces that
// raise, and lands nothing.
func TestMerge519LandingSkipCaptureRaises(t *testing.T) {
	r := walkReg(t)
	r.Register("zz-cap-fail", core.Signature{
		Args: []*core.Type{core.TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1,
		Returns: []*core.Type{core.TAtom},
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
			return nil, reg.BoruError("value_error", "zz-cap-fail: boom", "zz-cap-fail")
		}),
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	inner := r.Lookup("zz-cap-fail")
	if inner == nil {
		t.Fatal("zz-cap-fail did not register")
	}
	v := core.NewFunction(core.FnDefInfo{Name: "zz-cap-fail", Registry: r, Signatures: inner.Signatures})
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	w := compiler.LandingWord{Name: "z", Pos: core.SrcPos{Row: 1, Col: 9}, SkipTo: 4, SkipOut: 1}
	got, ent, err := vc.landingSkipCapture(r, v, w, []core.Value{v}, 0, seam7Dbg, 0)
	if got != nil || ent != nil {
		t.Errorf("a raising capture lands nothing, got %v %+v", got, ent)
	}
	wantErr(t, err, "zz-cap-fail: boom")
}

// TestMerge519DynTrailZeroArgLeadRaises pins callDynTrailTop's 0-arg-lead
// island (NUR176): a named lead whose only overload takes no argument fires
// over nothing on the island, and a raise from its body surfaces unchanged
// — the frame binding the island installed is torn down either way.
func TestMerge519DynTrailZeroArgLeadRaises(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	lead := core.NewFunction(core.FnDefInfo{
		Name: "zk", Registry: r,
		Signatures: []core.Signature{{
			Returns: []*core.Type{core.TInteger}, BarrierPos: core.BarrierAllForward,
			Impl: core.Boru([]core.Value{core.NewWord("cfail"), core.NewInteger(1)}),
		}},
	})
	if !core.FnValueOnlyZeroArgSigs(lead.Data.(core.FnDefInfo)) {
		t.Fatal("the lead must take no argument")
	}
	vc := seam7VC(r)
	got, ent, err := vc.callDynTrailTop(r, 1, []core.Value{core.NewInteger(5), lead}, seam7Dbg, 0, compiler.DynApplyHead{Name: "zk"})
	if got != nil || ent != nil {
		t.Errorf("a raising lead lands nothing, got %v %+v", got, ent)
	}
	wantErr(t, err, "cfail: boom")
	if _, bound := r.Defs.Top("zk"); bound {
		t.Error("the island's frame binding of the lead must be torn down")
	}
}

// TestMerge519MixedPlacedFnRefuses pins callDynamicMixed's placed-window
// refusal (NUR312): a placed fn value above a value the window must step
// cannot start the island between them, so the op is the loud
// vm:mixed-placed-fn defer rather than a verbatim island that would re-step
// what the interpreter's delivery held resolved.
func TestMerge519MixedPlacedFnRefuses(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	g := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	list := core.NewList([]core.Value{core.NewInteger(1)})
	window := []core.Value{list, core.NewOpenParen(), g, core.NewCloseParen(), core.NewWord("add"), core.NewInteger(3)}
	got, err := vc.callDynamicMixed(r, len(window), window, seam7Dbg, 0)
	if got != nil || err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "NUR312") {
		t.Fatalf("a placed fn above a stepping value: want the loud defer, got %v / %v", got, err)
	}
}

// TestMerge519MakeListReStepArms pins makeListReStep's arms (NUR295): an
// underflowing window is the internal error it is (through the run loop's
// OpMakeListReStep dispatch), a re-stepping element whose `/q` slot would
// capture a token no word was noted for is the loud vm:list-restep defer,
// and an island whose run raises surfaces the raise.
func TestMerge519MakeListReStepArms(t *testing.T) {
	under := &compiler.Program{
		Code:        []compiler.Instr{{Op: compiler.OpMakeListReStep, Arg: 0}},
		Debug:       make([]core.SrcPos, 1),
		ListReSteps: []compiler.ListReStepSpec{{N: 2, Fn: []int{0}, Island: true}},
	}
	_, err := RunProgram(under, seam7Reg(t))
	wantInternal(t, err, "MAKE_LIST_RESTEP stack underflow")

	r, _, _ := seam7DelegReg(t)
	vc := seam7VC(r)
	quoting := core.NewFunction(core.FnDefInfo{
		Name: "zq", Registry: r,
		Signatures: []core.Signature{{
			Params:  []core.FnParam{{Name: "a", Type: core.TAny, Quote: true}},
			Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
			Impl: core.Boru([]core.Value{core.NewWord("a")}),
		}},
	})
	spec := compiler.ListReStepSpec{N: 2, Fn: []int{0}, Island: true}
	got, err := vc.makeListReStep(r, spec, []core.Value{quoting, core.NewInteger(5)}, seam7Dbg, 0)
	if got != nil || err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "NUR295") {
		t.Fatalf("a /q capture with no noted word: want the loud defer, got %v / %v", got, err)
	}
	// The island runs the elements as the list's own evaluation; a fn whose
	// body raises surfaces that raise.
	got, err = vc.makeListReStep(r, spec, []core.Value{seam7UserFail(r), core.NewInteger(5)}, seam7Dbg, 0)
	if got != nil {
		t.Errorf("a raising island builds no list, got %v", got)
	}
	wantErr(t, err, "cfail: boom")
}

// TestMerge519FnValueQuotesClosure pins fnValueQuotes over a compiled
// closure: the unit's declared params are read (closureSigParams), and a
// compiled unit's params carry no `/q` flag, so the closure does not quote.
func TestMerge519FnValueQuotesClosure(t *testing.T) {
	p := &compiler.Program{Fns: []compiler.CompiledFn{{Name: "c", NParams: 1, NArgs: 1, Params: []*core.Type{core.TInteger}}}}
	if fnValueQuotes(compiler.NewClosure(p, 0, nil)) {
		t.Error("a compiled closure's params do not quote")
	}
	// A closure payload with no program behind it reads no params.
	if fnValueQuotes(core.Value{Parent: core.TFunction, Data: core.ClosurePayload{}}) {
		t.Error("a closure with no program does not quote")
	}
}

// TestMerge519SpecGuardListShape pins specGuardsHold's list-shape guard: a
// List of the shape's exact length and tags holds it; another length or
// another tag fails the guard.
func TestMerge519SpecGuardListShape(t *testing.T) {
	shape, ok := core.ListShapeOf(core.NewList([]core.Value{core.NewInteger(1), core.NewString("a")}), 8)
	if !ok || !core.IsListShapeGuard(shape) {
		t.Fatalf("list shape: %v %v", shape, ok)
	}
	guards := []compiler.SpecGuard{{Param: 0, Fn: shape}}
	if !specGuardsHold(guards, []core.Value{core.NewList([]core.Value{core.NewInteger(7), core.NewString("z")})}) {
		t.Error("the same shape holds")
	}
	if specGuardsHold(guards, []core.Value{core.NewList([]core.Value{core.NewString("z"), core.NewInteger(7)})}) {
		t.Error("another tag order fails the guard")
	}
	if specGuardsHold(guards, []core.Value{core.NewList([]core.Value{core.NewInteger(7)})}) {
		t.Error("another length fails the guard")
	}
}

// TestMerge519FallbackCountIslandRaises pins OpFallback's count-island arm
// (Program.FallbackCounts, NUR301): a checked island whose run the single
// seat does not hold hands it to the statement's count island, and a
// malformed count island is the compiler's fault, raised as the internal
// error it is rather than run.
func TestMerge519FallbackCountIslandRaises(t *testing.T) {
	prog := func(is *compiler.StmtIsland) *compiler.Program {
		return &compiler.Program{
			Code:           []compiler.Instr{{Op: compiler.OpFallback, Arg: 0}},
			Debug:          make([]core.SrcPos, 1),
			Fallbacks:      []core.FallbackSpan{{Tokens: []core.Value{core.NewInteger(1), core.NewInteger(2)}, CheckOne: true, Desc: "zz-strip"}},
			FallbackCounts: map[int]*compiler.StmtIsland{0: is},
		}
	}
	res, err := RunProgram(prog(&compiler.StmtIsland{Island: []core.Value{core.NewInteger(9)}, RetPC: 1, Root: true}), seam7Reg(t))
	if err != nil || len(res) != 1 || res[0].String() != "9" {
		t.Fatalf("a well-formed count island runs the statement: got %v / %v", res, err)
	}
	_, err = RunProgram(prog(&compiler.StmtIsland{RetPC: 1}), seam7Reg(t))
	wantInternal(t, err, "bad statement-island entry")
}

// TestMerge519FlowBreakLoopResultDefers pins flowSignal's break over a loop
// whose result re-steps (loopExitReStep, NUR314): a fn value left as the
// loop's result that the compiled loop cannot hand the interpreter where
// the loop stood (its results do not begin at its frame's base) is the loud
// vm:loop-result-restep defer, never a silent data landing.
func TestMerge519FlowBreakLoopResultDefers(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpRet}}}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	g := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	stack := []core.Value{core.NewInteger(1), g}
	loops := []vmLoop{{unit: -1, iterBase: 2, base: 1, frameBase: 0, exitPC: 0}}
	_, _, _, _, _, _, err := vc.flowSignal(compiler.OpFlowBreak, nil, loops, nil, stack, 0, -1, seam7Dbg)
	if err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "NUR314") {
		t.Fatalf("a re-stepping loop result the loop cannot hand over: want the loud defer, got %v", err)
	}
}
