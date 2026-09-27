package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_arm_residue_test.go pins the VM arms the merged ADR-008 profile
// (2026-09-27) found no suite reaching. Each test asserts what its arm DOES —
// the residual it leaves, the error it surfaces, the rung it takes — so a
// regression in the arm fails here rather than merely lowering a percentage.
// Where the arm's shape is a runtime VALUE the corpus cannot produce on
// demand, the value is built by hand, the discipline vm_seam7_test.go
// documents.

// raiserProg is a program whose unit 0 takes nparams args and raises through
// a trap — a hosted body that fails.
func raiserProg(nparams int, detail string) *compiler.Program {
	return &compiler.Program{
		Traps: []compiler.TrapSpec{{Code: "bad_input", Detail: detail, Word: "boom"}},
		Fns: []compiler.CompiledFn{{
			Name: "raiser", NParams: nparams, NLocals: nparams, NArgs: nparams,
			Code:  []compiler.Instr{{Op: compiler.OpTrap, Arg: 0}},
			Debug: []core.SrcPos{{Row: 1, Col: 1}},
		}},
	}
}

// oneArgConstProg is a program whose unit 0 takes one arg and answers n —
// distinguishable from the arg.
func oneArgConstProg(n int64) *compiler.Program {
	return &compiler.Program{
		Consts: []core.Value{core.NewInteger(n)},
		Fns: []compiler.CompiledFn{{
			Name: "argConst", NParams: 1, NLocals: 1, NArgs: 1,
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
}

var oneIntParam = []core.FnParam{{Name: "n", Type: core.TInteger}}

// armEntries counts the interpreter entries a registry reports while armed.
func armEntries(t *testing.T, r *core.Registry) *entryCollector {
	t.Helper()
	c := &entryCollector{}
	t.Cleanup(r.ArmInterpEntryHook(c.add))
	return c
}

// --- the compiled runtime's LazyStamp ------------------------------------

// A nil signature carries no unit to stamp, so the VM runtime declines it —
// the same answer core's inactive default gives (core compiled_runtime_test),
// rather than a nil dereference inside the stamp.
func TestLazyStampNilSigDeclines(t *testing.T) {
	if (vmCompiledRuntime{}).LazyStamp(seam7Reg(t), core.FnDefInfo{Name: "f"}, nil, core.SrcPos{}) {
		t.Error("a nil signature must report no unit")
	}
}

// --- callDynamic -----------------------------------------------------------

// A fn-VALUE closure the window does not match stays data on both lanes, as
// the window was written: LEADING leaves `[fn arg]`, TRAILING — the arg
// beneath, the fn on top — undoes the rotation the lowering made for the
// apply, leaving `[arg fn]` (NUR159's probe answered `[fn arg]` without the
// trailing arm).
func TestCallDynamicClosureNoMatchKeepsWrittenWindow(t *testing.T) {
	prog := func(op compiler.Opcode) *compiler.Program {
		return &compiler.Program{
			Consts: []core.Value{core.NewString("s")},
			Code: []compiler.Instr{
				{Op: compiler.OpPushClosure, Arg: 0}, // the fn value at the base
				{Op: compiler.OpPushConst, Arg: 0},   // its arg, a String, above
				{Op: op, Arg: 1},
			},
			Debug: make([]core.SrcPos, 3),
			Fns: []compiler.CompiledFn{{
				Name: "fnval$body", Lambda: true, NArgs: 1, NParams: 1, NLocals: 1,
				Params: []*core.Type{core.TInteger}, // `[z:Integer] => …`: a String no-matches
				Code:   []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
				Debug:  make([]core.SrcPos, 2),
			}},
		}
	}
	for _, tc := range []struct {
		name   string
		op     compiler.Opcode
		fnSlot int
	}{
		{"leading", compiler.OpCallDynamic, 0},
		{"trailing", compiler.OpCallDynamicTrailing, 1},
	} {
		got, err := RunProgram(prog(tc.op), seam7Reg(t))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != 2 {
			t.Fatalf("%s: residual %v, want the two values untouched", tc.name, got)
		}
		if !compiler.IsCompiledClosure(got[tc.fnSlot]) {
			t.Errorf("%s: residual %v, want the closure at %d", tc.name, got, tc.fnSlot)
		}
		if s, _ := core.AsString(got[1-tc.fnSlot]); s != "s" {
			t.Errorf("%s: residual %v, want the arg at %d", tc.name, got, 1-tc.fnSlot)
		}
	}
}

// A MODIFIER WRAPPER over a fn value whose unit is DETACHED (a module fn's
// own stamp, its Program not the running one) resolves to what it wraps and
// hosts that unit nested on both the dynamic apply and the shaped method
// apply: the unit's 42 comes back and the census records no interpreter
// entry, which the island (the rung below) would be.
func TestWrapperOverDetachedUnitHostsNested(t *testing.T) {
	inner := dynApplyFn(oneIntParam, oneArgConstProg(42), 0)
	for _, wrap := range []struct {
		name string
		mk   func(core.Value) (core.Value, bool)
	}{
		{"usurp", core.UsurpFunction},
		{"forward-args", core.ForceForwardFunction},
	} {
		w, ok := wrap.mk(inner)
		if !ok {
			t.Fatalf("%s: the wrapper did not build", wrap.name)
		}
		vc, r := foreignVC(t, oneConstProg(1))
		entries := armEntries(t, r)

		got, ent, err := vc.callDynamic(r, 1, false, []core.Value{w, core.NewInteger(5)}, seam7Dbg, 0)
		if err != nil || ent != nil || len(got) != 1 {
			t.Fatalf("%s: callDynamic: %v %v %v", wrap.name, got, ent, err)
		}
		if n, _ := got[0].AsConcreteInteger(); n != 42 {
			t.Errorf("%s: callDynamic answered %v, want the detached unit's 42", wrap.name, got[0])
		}

		got, ent, err = vc.callDynMethod(r, &compiler.DynMethodSpec{Word: "m", NArgs: 1, NOut: 1}, []core.Value{core.NewInteger(5), w}, seam7Dbg, 0)
		if err != nil || ent != nil || len(got) != 1 {
			t.Fatalf("%s: callDynMethod: %v %v %v", wrap.name, got, ent, err)
		}
		if n, _ := got[0].AsConcreteInteger(); n != 42 {
			t.Errorf("%s: callDynMethod answered %v, want the detached unit's 42", wrap.name, got[0])
		}
		if len(entries.entries) != 0 {
			t.Errorf("%s: the hosted unit must not enter the interpreter, got %v", wrap.name, entries.entries)
		}
	}
}

// --- callDynMethod: a raise from the applied member surfaces as-is ---------

// The detached-unit rung, bare and under a wrapper, and the compiled-closure
// rung under a wrapper: each raise is the member's own error, surfaced at the
// method apply (the interpreter raises the same, prior effects included).
func TestCallDynMethodMemberRaises(t *testing.T) {
	spec := &compiler.DynMethodSpec{Word: "m", NArgs: 1, NOut: 1}
	raiser := dynApplyFn(oneIntParam, raiserProg(1, "the method raised"), 0)
	wrapped, _ := core.ForceForwardFunction(raiser)
	for _, tc := range []struct {
		name string
		fn   core.Value
	}{{"detached unit", raiser}, {"wrapped detached unit", wrapped}} {
		vc, r := foreignVC(t, oneConstProg(1))
		_, _, err := vc.callDynMethod(r, spec, []core.Value{core.NewInteger(5), tc.fn}, seam7Dbg, 0)
		wantErr(t, err, "the method raised")
	}

	// A wrapper over a COMPILED CLOSURE applies the closure itself; its unit
	// declares Integer and returns a String, so the closure's own return
	// contract raises.
	p := &compiler.Program{
		Consts: []core.Value{core.NewString("x")},
		Fns: []compiler.CompiledFn{{
			Name: "badret", Lambda: true, NArgs: 1, NParams: 1, NLocals: 1,
			Params:  []*core.Type{core.TInteger},
			Returns: []*core.Type{core.TInteger},
			Code:    []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug:   make([]core.SrcPos, 2),
		}},
	}
	r := seam7Reg(t)
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	// The modifier words build a closure's wrapper from its bridged shape,
	// which exists only while a run's invoker is installed (closureShape).
	r.Invoker = func(*core.Registry, core.Value, []core.Value) ([]core.Value, error) { return nil, nil }
	w, ok := core.ForceForwardClosure(r, compiler.NewClosure(p, 0, nil))
	r.Invoker = nil
	if !ok {
		t.Fatal("the closure wrapper did not build")
	}
	_, _, err := vc.callDynMethod(r, spec, []core.Value{core.NewInteger(5), w}, seam7Dbg, 0)
	wantErr(t, err, "expected Integer")
}

// --- callDynApply ----------------------------------------------------------

// A 0-ARG closure under the `apply` word fires over nothing (applyHandler's
// MarkApplied); when its body raises, the raise is the result — surfaced at
// the apply, not a stack.
func TestCallDynApplyZeroArgClosureRaises(t *testing.T) {
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(5), core.NewString("x")},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},   // the window's one value
			{Op: compiler.OpPushClosure, Arg: 0}, // a 0-arg closure on top
			{Op: compiler.OpCallDynApplyTop, Arg: 1},
		},
		Debug: make([]core.SrcPos, 3),
		Fns: []compiler.CompiledFn{{
			Name: "bad0", NParams: 0, NLocals: 0, Returns: []*core.Type{core.TInteger},
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpRet}},
			Debug: make([]core.SrcPos, 2),
		}},
	}
	_, err := RunProgram(p, seam7Reg(t))
	wantErr(t, err, "expected Integer")
}

// --- callDynFrame ----------------------------------------------------------

// A LONE fn token over the frame's prefix collects its one parameter from
// the prefix top; a raise inside the hosted unit is the call's own error.
func TestCallDynFrameLoneTokenRaises(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	fv := fnValueWithRef(&compiler.CompiledFnRef{Prog: raiserProg(1, "the lone token raised"), Unit: 0}, oneIntParam, nil)
	_, _, err := vc.callDynFrame(r, 1, 0, []core.Value{core.NewInteger(5), fv}, seam7Dbg, 0, nil)
	wantErr(t, err, "the lone token raised")
}

// --- reStepLanding / landingFire -------------------------------------------

// landingFire's last rung: a NAMED 0-arg fn value the VM can neither apply
// natively (its name is no registered word) nor enter (no compiled unit)
// islands, and the interpreter's re-step FIRES it — a name always calls.
func TestReStepLandingFireIslands(t *testing.T) {
	r := seam7Reg(t)
	fn := core.NewFunction(core.FnDefInfo{
		Name: "zz-landing-unregistered",
		Signatures: []core.Signature{{
			BarrierPos: -1,
			Returns:    []*core.Type{core.TInteger},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(13)}, nil
			}),
		}},
	})
	if vmNativeApplicable(r, fn.Data.(core.FnDefInfo)) {
		t.Fatal("fixture reads as natively applicable — it would take the native rung, not the island")
	}
	entries := armEntries(t, r)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{core.NewString("below"), fn}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil {
		t.Fatalf("island landing: %v %v", ent, err)
	}
	if len(got) != 2 {
		t.Fatalf("landing residual = %v, want the value beneath and the fired result", got)
	}
	if n, _ := got[1].AsConcreteInteger(); n != 13 {
		t.Errorf("landing = %v, want the fired 13 over the value beneath", got)
	}
	island := false
	for _, e := range entries.entries {
		island = island || e.Seam == "vm:island"
	}
	if !island {
		t.Errorf("the landing must island (the last rung), entries %v", entries.entries)
	}
}

// The tag-only rung (a Function value with no callable payload) islands too;
// the island is an interpreter run, and whatever it raises — here the step
// budget the registry allows it, one step for a one-token window that needs
// two — surfaces stamped at the landing.
func TestReStepLandingIslandRaises(t *testing.T) {
	r := seam7Reg(t)
	r.StepLimit = 1
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	_, _, err := vc.reStepLanding(r, 0, 0, []core.Value{{Parent: core.TFunction}}, []core.SrcPos{{Row: 4, Col: 2}}, 0, compiler.LandingWord{})
	wantErr(t, err, "evaluation_limit")
	if ae, ok := err.(*core.BoruError); !ok || ae.Row != 4 {
		t.Errorf("the island's raise must be stamped at the landing, got %v", err)
	}
}

// --- callDynTrailTop -------------------------------------------------------

// Under a seated NAME a closure the window does not match raises the word
// dispatch's no-match, described over the closure's own declared contract
// (closureSigView) — but only a closure whose unit DECLARES one can be
// judged. A token body's unit records none, so there is nothing to raise
// over: it applies positionally, as it always did.
func TestCallDynTrailTopNamedHeadClosureContract(t *testing.T) {
	p := &compiler.Program{Fns: []compiler.CompiledFn{{
		Name: "each$body", NArgs: 1, NParams: 1, NLocals: 1, // no Params: no contract
		Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
		Debug: make([]core.SrcPos, 2),
	}, {
		Name: "fnval$body", Lambda: true, NArgs: 1, NParams: 1, NLocals: 1,
		Params: []*core.Type{core.TInteger},
		Code:   []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
		Debug:  make([]core.SrcPos, 2),
	}}}
	r := seam7Reg(t)
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	head := compiler.DynApplyHead{Name: "a5", NWritten: 1}
	got, _, err := vc.callDynTrailTop(r, 1, []core.Value{core.NewString("s"), compiler.NewClosure(p, 0, nil)}, seam7Dbg, 0, head)
	if err != nil || len(got) != 1 {
		t.Fatalf("an uncontracted closure applies: %v %v", got, err)
	}
	if s, _ := core.AsString(got[0]); s != "s" {
		t.Errorf("the body answers its input, got %v", got)
	}
	_, _, err = vc.callDynTrailTop(r, 1, []core.Value{core.NewString("s"), compiler.NewClosure(p, 1, nil)}, seam7Dbg, 0, head)
	wantErr(t, err, "cannot call `a5` — no signature matches the arguments")
	wantErr(t, err, "candidate `a5 (Integer)`") // the closure's own declared contract
}

// --- SPLICE_DYN ------------------------------------------------------------

// Arg 1 is the recorder's claim that the spread is the ONE fn operand of the
// apply that follows. A payload that spreads to any other count would lay N
// values out as one, so the run defers — loudly, and to the bail census.
func TestSpliceDynClaimedSingleDefersOnMany(t *testing.T) {
	r := seam7Reg(t)
	var sites []string
	t.Cleanup(r.ArmRuntimeBailHook(func(b core.BailEvent) { sites = append(sites, b.Site) }))
	p := &compiler.Program{
		Consts: []core.Value{core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})},
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpSpliceDyn, Arg: 1}},
		Debug:  make([]core.SrcPos, 2),
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "splice of a multi-value payload where the program applies one value")
	if len(sites) != 1 || sites[0] != "vm:splice-active-payload" {
		t.Errorf("the defer must reach the bail census once, got %v", sites)
	}
}

// --- CALL_NATIVE_POLY ------------------------------------------------------

// A poly re-match's handler runs bodies, so a break can escape it; with no
// loop open anywhere the flow signal has nowhere to land and the run fails
// rather than dropping it (the interpreter's canonical `break outside loop`).
func TestCallNativePolyEscapedFlowWithoutLoop(t *testing.T) {
	r := seam7Reg(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zz-poly-break",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				reg.FlowCtrl = core.FlowBreak
				return nil, nil
			}),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	p := &compiler.Program{
		Consts:   []core.Value{core.NewInteger(1)},
		Code:     []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNativePoly, Arg: 0}},
		Debug:    make([]core.SrcPos, 2),
		PolyRefs: []compiler.PolyRef{{Word: "zz-poly-break", Arity: 1}},
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "flow signal with no enclosing loop")
	if r.FlowCtrl != core.FlowNone {
		t.Error("the escaped signal must be consumed, not left on the registry")
	}
}

// --- vmDeferAlt ------------------------------------------------------------

// The alt a defer site builds over its live values takes the defer's
// POSITION when it has none of its own — the interpreter's raise pointed at
// the token — and keeps its own when it has one.
func TestVmDeferAltLendsPosition(t *testing.T) {
	r := covRegistry(t, nil)
	dbg := []core.SrcPos{{Row: 3, Col: 7, Src: "tok"}}
	bare := &core.BoruError{Code: "signature_error", Detail: "d"}
	err := vmDeferAlt(r, dbg, 0, "vm:poly-no-match", "x", bare)
	if ae, ok := err.(*core.BoruError); !ok || ae.DeferAlt != bare {
		t.Fatalf("the alt must ride the defer, got %v", err)
	}
	if bare.Row != 3 || bare.Col != 7 || bare.Src != "tok" {
		t.Errorf("an unpositioned alt takes the defer's position, got %d:%d %q", bare.Row, bare.Col, bare.Src)
	}
	own := &core.BoruError{Code: "signature_error", Detail: "d", Row: 9, Col: 1, Src: "own"}
	_ = vmDeferAlt(r, dbg, 0, "vm:poly-no-match", "x", own)
	if own.Row != 9 || own.Src != "own" {
		t.Errorf("a positioned alt keeps its own position, got %d %q", own.Row, own.Src)
	}
}

// --- dynApplyForeign declines ----------------------------------------------

// A detached ref runs only while it is FRESH: a stale one re-stamps through
// its box, and without one (a compile-time ref) it declines to the island;
// a unit index outside its program is drift and declines too.
func TestDynApplyForeignFreshnessAndDrift(t *testing.T) {
	vc, _ := foreignVC(t, oneConstProg(1))
	stale := map[string]compiler.DepSnapEntry{"dep": {Depth: 99, Gen: -1}} // never matches
	fnOn := func(ref *compiler.CompiledFnRef) core.Value {
		return fnValueWithRef(ref, nil, []*core.Type{core.TAny})
	}

	if _, ran, _ := vc.dynApplyForeign(fnOn(&compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0, DepSnap: stale}), nil, 0); ran {
		t.Error("a stale ref with no re-stamp box must decline")
	}
	fresh := &compiler.CompiledFnRef{Prog: oneConstProg(43), Unit: 0}
	res, ran, err := vc.dynApplyForeign(fnOn(&compiler.CompiledFnRef{
		Prog: oneConstProg(42), Unit: 0, DepSnap: stale, Restamp: &compiler.RestampBox{Cur: fresh},
	}), nil, 0)
	if !ran || err != nil || len(res) != 1 {
		t.Fatalf("a stale ref with a fresh re-stamp runs it: ran=%v err=%v res=%v", ran, err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 43 {
		t.Errorf("the re-stamped unit answers 43, got %v", res[0])
	}
	if _, ran, _ := vc.dynApplyForeign(fnOn(&compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 7}), nil, 0); ran {
		t.Error("a unit index past its program must decline")
	}
}

// --- the fn-value seam's return contract -----------------------------------

// The value's own declared count is enforced over the hosted unit's residual
// with the TOKEN seam's discipline: an EXCESS beyond the unnamed-param
// allowance raises the count error naming the value; the allowance itself
// (the untouched unnamed input at the frame bottom) is not counted.
func TestFnValueSeamExcessReturns(t *testing.T) {
	twoOut := func(nUnnamed int) *compiler.Program {
		return &compiler.Program{
			Consts: []core.Value{core.NewInteger(42)},
			Fns: []compiler.CompiledFn{{
				Name: "two", NUnnamed: nUnnamed,
				Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
				Debug: make([]core.SrcPos, 3),
			}},
		}
	}
	vc, r := foreignVC(t, oneConstProg(1))
	one := []*core.Type{core.TInteger}
	_, err, ran := vc.invokeFnValue(r, fnValueWithRef(&compiler.CompiledFnRef{Prog: twoOut(0), Unit: 0}, nil, one), nil)
	if !ran || err == nil || !strings.Contains(err.Error(), "fv: expected 1 return value(s), got 2") {
		t.Fatalf("an excess return must raise the count error: ran=%v err=%v", ran, err)
	}
	res, err, ran := vc.invokeFnValue(r, fnValueWithRef(&compiler.CompiledFnRef{Prog: twoOut(1), Unit: 0}, nil, one), nil)
	if !ran || err != nil {
		t.Fatalf("one unnamed input is allowed beneath the one declared return: ran=%v err=%v res=%v", ran, err, res)
	}
}
