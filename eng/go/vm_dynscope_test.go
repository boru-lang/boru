package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_dynscope_test.go pins the dynamic-scope opcode pair (OpBindDynScope /
// OpLookupDynScope): the registry install + frame-exit cleanup discipline,
// the lookup's read path, and every runtime deferral arm (miss, dispatching
// binding, active token), plus the error-path unwind that keeps a failed run
// from leaking bindings into the registry.

func dsProgram(fns []compiler.CompiledFn, main []compiler.Instr, consts ...core.Value) *compiler.Program {
	dbg := make([]core.SrcPos, len(main))
	return &compiler.Program{Code: main, Debug: dbg, Consts: consts, Fns: fns}
}

func TestBindDynScopeInstallAndRetCleanup(t *testing.T) {
	r := seam7Reg(t)
	fn := compiler.CompiledFn{
		Name: "dsf", Returns: []*core.Type{core.TAny},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},    // 5
			{Op: compiler.OpBindDynScope, Arg: 1}, // install dsx=5
			{Op: compiler.OpLookupDynScope, Arg: 1},
			{Op: compiler.OpRet},
		},
		Debug: make([]core.SrcPos, 4),
	}
	p := dsProgram([]compiler.CompiledFn{fn},
		[]compiler.Instr{{Op: compiler.OpCallUser, Arg: 0}},
		core.NewInteger(5), core.NewString("dsx"))
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("bind+lookup: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual %v, want the looked-up value", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 5 {
		t.Errorf("lookup read %v, want the frame's binding 5", out[0])
	}
	if _, ok := r.Defs.Top("dsx"); ok {
		t.Error("RET must truncate the frame's dynamic-scope binding")
	}
}

// TestBindDynScopeClosureValue: a compiled CLOSURE value bound by a frame's
// OpBindDynScope is PUSHED onto the def stack (the installer's carrier guard
// installs nothing for it — NUR192's false `undefined word: a5`), so the
// data-position lookup reads it back, and the RET's trail truncation pops
// it with the frame.
func TestBindDynScopeClosureValue(t *testing.T) {
	r := seam7Reg(t)
	fn := compiler.CompiledFn{
		Name: "dscl", Returns: []*core.Type{core.TAny},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},    // the closure value
			{Op: compiler.OpBindDynScope, Arg: 1}, // bind dsc
			{Op: compiler.OpLookupDynScopeData, Arg: 1},
			{Op: compiler.OpRet},
		},
		Debug: make([]core.SrcPos, 4),
	}
	body := compiler.CompiledFn{Name: "dscl$body", Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: make([]core.SrcPos, 1)}
	p := dsProgram([]compiler.CompiledFn{fn, body},
		[]compiler.Instr{{Op: compiler.OpCallUser, Arg: 0}})
	closure := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 1, Render: "fn ()"})
	p.Consts = []core.Value{closure, core.NewString("dsc")}
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("bind+lookup of a closure: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual %v, want the looked-up closure", out)
	}
	if cl, ok := out[0].Data.(core.ClosurePayload); !ok || cl.Unit != 1 {
		t.Errorf("the lookup must read the bound closure back, got %v", out[0])
	}
	if _, ok := r.Defs.Top("dsc"); ok {
		t.Error("RET must pop the frame's closure binding with the trail")
	}
}

func TestBindDynScopeErrorUnwind(t *testing.T) {
	r := seam7Reg(t)
	fn := compiler.CompiledFn{
		Name: "dsboom", Returns: []*core.Type{core.TAny},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},
			{Op: compiler.OpBindDynScope, Arg: 1},
			{Op: compiler.OpTrap, Arg: 0},
		},
		Debug: make([]core.SrcPos, 3),
	}
	p := dsProgram([]compiler.CompiledFn{fn},
		[]compiler.Instr{{Op: compiler.OpCallUser, Arg: 0}},
		core.NewInteger(9), core.NewString("dserr"))
	p.Traps = []compiler.TrapSpec{{Code: "value_error", Detail: "boom", Word: "dsboom"}}
	if _, err := RunProgram(p, r); err == nil {
		t.Fatal("trap must error")
	}
	if _, ok := r.Defs.Top("dserr"); ok {
		t.Error("an error unwind must restore the registry's dynamic-scope bindings")
	}
}

func TestLookupDynScopeDeferralArms(t *testing.T) {
	// Miss: no live binding.
	r := seam7Reg(t)
	p := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScope, Arg: 0}}, core.NewString("nosuch"))
	_, err := RunProgram(p, r)
	wantInternal(t, err, "dynamic-scope read miss for `nosuch`")

	// A dispatching binding (FnDefInfo): the interpreter would dispatch, not
	// substitute — defer.
	r2 := seam7Reg(t)
	r2.Defs.Push("dsfn", core.NewFunction(core.FnDefInfo{Name: "dsfn", Registry: r2,
		Signatures: []core.Signature{{Returns: []*core.Type{core.TAny}, BarrierPos: -1}}}))
	p2 := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScope, Arg: 0}}, core.NewString("dsfn"))
	_, err = RunProgram(p2, r2)
	wantInternal(t, err, "dynamic-scope read of a dispatching binding `dsfn`")

	// An active token (a splice marker): steps on the tape, never data — defer.
	r3 := seam7Reg(t)
	r3.Defs.Push("dssp", core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)})))
	p3 := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScope, Arg: 0}}, core.NewString("dssp"))
	_, err = RunProgram(p3, r3)
	wantInternal(t, err, "dynamic-scope read of an active token `dssp`")
}

func TestLookupDynScopeReadsTopLevelBinding(t *testing.T) {
	r := seam7Reg(t)
	r.Defs.Push("dsy", core.NewInteger(7))
	p := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScope, Arg: 0}}, core.NewString("dsy"))
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual %v, want one value", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Errorf("lookup read %v, want 7", out[0])
	}
}

// TestLookupDynScopeDataArms pins OpLookupDynScopeData, the DATA-position twin
// of OpLookupDynScope: it PUSHES an FnDefInfo (a parser/fn value read as data)
// where OpLookupDynScope defers, and still defers on a miss, a class binding,
// and an active token.
func TestLookupDynScopeDataArms(t *testing.T) {
	// FnDefInfo binding: OpLookupDynScope DEFERS (name-position dispatch), but
	// the DATA twin PUSHES it — the parselang-fn-dispatch parser operand.
	r := seam7Reg(t)
	fn := core.NewFunction(core.FnDefInfo{Name: "dspfn", Registry: r,
		Signatures: []core.Signature{{Returns: []*core.Type{core.TAny}, BarrierPos: -1}}})
	r.Defs.Push("dspfn", fn)
	p := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScopeData, Arg: 0}}, core.NewString("dspfn"))
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("data read of a fn binding must PUSH, not defer: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual %v, want the pushed fn value", out)
	}
	if _, isFn := out[0].Data.(core.FnDefInfo); !isFn {
		t.Errorf("pushed %v, want the FnDefInfo parser value as data", out[0])
	}

	// Miss: no live binding — defer.
	r2 := seam7Reg(t)
	p2 := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScopeData, Arg: 0}}, core.NewString("dspmiss"))
	_, err = RunProgram(p2, r2)
	wantInternal(t, err, "dynamic-scope data read miss for `dspmiss`")

	// A class binding is not a parser — defer.
	r3 := seam7Reg(t)
	r3.Defs.Push("dspcls", core.NewValueRaw(core.TClass, &core.ClassTypeInfo{Name: "Class/C", Fields: core.NewOrderedMap()}))
	p3 := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScopeData, Arg: 0}}, core.NewString("dspcls"))
	_, err = RunProgram(p3, r3)
	wantInternal(t, err, "dynamic-scope data read of a class binding `dspcls`")

	// An active token (a splice marker) steps on the tape, never data — defer.
	r4 := seam7Reg(t)
	r4.Defs.Push("dspsp", core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)})))
	p4 := dsProgram(nil, []compiler.Instr{{Op: compiler.OpLookupDynScopeData, Arg: 0}}, core.NewString("dspsp"))
	_, err = RunProgram(p4, r4)
	wantInternal(t, err, "dynamic-scope data read of an active token `dspsp`")
}

// A break escaping a callee to an outer loop (flowSignal) tears down the
// escaped frame's dynamic-scope bindings — the interpreter's
// unwindLiveFrames replays the frame's cleanup tail; without the unwind the
// dead frame's install stays readable in Defs (PR #233 review).
func TestFlowSignalUnwindsEscapedFrameDynBinds(t *testing.T) {
	r := seam7Reg(t)
	fn := compiler.CompiledFn{
		Name: "dsbrk", Returns: nil,
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},
			{Op: compiler.OpBindDynScope, Arg: 1}, // install dsfl=5 in this frame
			{Op: compiler.OpFlowBreak},            // escape to the caller's loop
		},
		Debug: make([]core.SrcPos, 3),
	}
	main := []compiler.Instr{
		{Op: compiler.OpPushConst, Arg: 2}, // step 1
		{Op: compiler.OpPushConst, Arg: 3}, // end 3
		{Op: compiler.OpPushConst, Arg: 4}, // start 0
		{Op: compiler.OpForSetup, Arg: 0},  // iterator slot 0
		{Op: compiler.OpForNext, Arg: 7},   // exit -> 7
		{Op: compiler.OpCallUser, Arg: 0},  // per-iteration call that breaks
		{Op: compiler.OpJmp, Arg: 4},       // back-edge
	}
	p := dsProgram([]compiler.CompiledFn{fn}, main,
		core.NewInteger(5), core.NewString("dsfl"), core.NewInteger(1), core.NewInteger(3), core.NewInteger(0))
	p.NumLocals = 1
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("break-out-of-callee run: %v", err)
	}
	if _, ok := r.Defs.Top("dsfl"); ok {
		t.Error("the escaped frame's dynamic-scope binding must tear down with the frame")
	}
}
