package eng

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_seam7_test.go drives the bytecode VM's defensive error arms and a few
// otherwise-uncovered normal branches. Most of these guards are unreachable
// through a correctly compiled program (they exist to turn a compiler bug into
// a clean internal_error → interpreter fallback, never a Go panic), so they are
// exercised here two ways:
//
//   - hand-built malformed Programs fed to RunProgram (the run() dispatch arms),
//   - direct calls to the in-package vmContext helpers (the extracted methods).
//
// Both are legitimate: feeding the VM malformed bytecode simulates exactly the
// compiler-bug shape each guard defends against, and the VM's contract is to
// return an internal_error BoruError (RunCompiled then re-runs the interpreter).

func seam7Reg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// wantInternal asserts err is a non-nil *BoruError carrying the internal_error
// taxonomy whose message contains sub.
func wantInternal(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want internal_error containing %q, got nil", sub)
	}
	var ae *core.BoruError
	if errors.As(err, &ae) {
		if ae.Code != "internal_error" {
			t.Fatalf("want internal_error, got %q (%v)", ae.Code, err)
		}
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("error %q does not contain %q", err.Error(), sub)
	}
}

// wantErr asserts err is non-nil and its message contains sub (any taxonomy).
func wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q, got nil", sub)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("error %q does not contain %q", err.Error(), sub)
	}
}

var seam7Dbg = []core.SrcPos{{}}

func seam7VC(r *core.Registry) *vmContext {
	return &vmContext{r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
}

// --- nil program / tapeCoupled / screenResults ---------------------------

func TestSeam7NilProgram(t *testing.T) {
	_, err := RunProgram(nil, seam7Reg(t))
	wantErr(t, err, "nil program")
}

func TestSeam7TapeCoupledDetectsTokens(t *testing.T) {
	// Each tape-coupled token flavour trips tapeCoupled (vm.go:124-131).
	toks := []core.Value{
		core.NewWord("w"),
		core.NewMark("m"),
		core.NewMove("to", "why"),
		core.NewForward(core.ForwardInfo{}),
		core.NewOpenParen(),
		core.NewSplice(core.NewInteger(1)),
	}
	for i := range toks {
		if !tapeCoupled([]core.Value{toks[i]}) {
			t.Errorf("token %d (%s) not detected as tape-coupled", i, toks[i].String())
		}
	}
	if tapeCoupled([]core.Value{core.NewInteger(1), core.NewString("x")}) {
		t.Error("plain data reported tape-coupled")
	}
}

func TestSeam7ScreenResultsRejectsToken(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	err := vc.screenResults([]core.Value{core.NewWord("w")}, "poke", seam7Dbg, 0)
	wantInternal(t, err, "tape-coupled poke")
	if err := vc.screenResults([]core.Value{core.NewInteger(1)}, "poke", seam7Dbg, 0); err != nil {
		t.Errorf("clean results screened as tape-coupled: %v", err)
	}
}

// --- run() dispatch error arms via malformed programs --------------------

func runMalformed(t *testing.T, p *compiler.Program) error {
	t.Helper()
	if p.Debug == nil {
		p.Debug = make([]core.SrcPos, len(p.Code))
	}
	_, err := RunProgram(p, seam7Reg(t))
	return err
}

func TestSeam7RunUnderflowArms(t *testing.T) {
	cases := []struct {
		name string
		p    *compiler.Program
		sub  string
	}{
		{"store-local", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpStoreLocal, Arg: 0}}, NumLocals: 1}, "STORE_LOCAL stack underflow"},
		{"drop", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpDrop}}}, "DROP stack underflow"},
		{"make-list", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpMakeList, Arg: 2}}}, "MAKE_LIST stack underflow"},
		{"make-map", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpMakeMap, Arg: 0}}, MakeMaps: []compiler.MakeMapSpec{{Keys: []string{"a"}}}}, "MAKE_MAP stack underflow"},
		{"interp", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpInterp, Arg: 0}}, Interps: []compiler.InterpSpec{{NHoles: 1, Segs: []compiler.InterpSeg{{Hole: true}}}}}, "INTERP stack underflow"},
		{"interp-xml", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpInterpXml, Arg: 0}}, XmlInterps: []compiler.XmlInterpSpec{{NHoles: 1, Tmpl: core.XmlTmpl{Tag: "p"}}}}, "INTERP_XML stack underflow"},
		{"push-closure", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpPushClosure, Arg: 0}}, Fns: []compiler.CompiledFn{{Name: "c", NCaptures: 2}}}, "PUSH_CLOSURE capture underflow"},
		{"for-setup", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpForSetup, Arg: 0}}}, "FOR_SETUP underflow"},
		{"for-next", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpForNext, Arg: 0}}}, "FOR_NEXT without a loop"},
		{"jmpiffalse", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpJmpIfFalse, Arg: 5}}}, "JMP_IF_FALSE underflow"},
		{"bind-typed", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpBindTyped, Arg: 0}}, TypedBinds: []core.TypedBindSpec{{Kind: core.TypedBindDepScalar, Name: "x"}}}, "BIND_TYPED stack underflow"},
		{"call-native-poly", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpCallNativePoly, Arg: 0}}, PolyRefs: []compiler.PolyRef{{Word: "p", Arity: 2}}}, "CALL_NATIVE_POLY underflow"},
		{"drop-to-mark", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpDropToMark}}}, "DROP_TO_MARK with no open mark"},
		{"pop-mark", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpPopMark}}}, "POP_MARK with no open mark"},
		{"unknown", &compiler.Program{Code: []compiler.Instr{{Op: compiler.Opcode(250)}}}, "unknown opcode"},
		{"flow-break-noloop", &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpFlowBreak}}}, "flow signal with no enclosing loop"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantInternal(t, runMalformed(t, c.p), c.sub)
		})
	}
}

func TestSeam7CallNativeUnderflow(t *testing.T) {
	r := seam7Reg(t)
	sig := core.Signature{Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: -1,
		Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return []core.Value{core.NewInteger(0)}, nil
		})}
	p := &compiler.Program{
		Code:  []compiler.Instr{{Op: compiler.OpCallNative, Arg: 0}},
		Debug: []core.SrcPos{{}},
		Sigs:  []compiler.SigRef{{Word: "twoarg", Sig: &sig}},
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "CALL_NATIVE underflow at twoarg")
}

func TestSeam7CallUserUnderflow(t *testing.T) {
	p := &compiler.Program{
		Code:  []compiler.Instr{{Op: compiler.OpCallUser, Arg: 0}},
		Debug: []core.SrcPos{{}},
		Fns:   []compiler.CompiledFn{{Name: "f", NParams: 2, NLocals: 2}},
	}
	wantInternal(t, runMalformed(t, p), "CALL_USER underflow at f")
}

func TestSeam7BackwardJumpNotToForNext(t *testing.T) {
	// A backward JMP whose target is not a FOR_NEXT is rejected.
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(1)},
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpJmp, Arg: 0}},
		Debug:  []core.SrcPos{{}, {}},
	}
	wantInternal(t, runMalformed(t, p), "backward jump not to a FOR_NEXT")
}

func TestSeam7BackwardConditionalJump(t *testing.T) {
	// A JMP_IF_FALSE whose (taken) target is <= pc is rejected.
	p := &compiler.Program{
		Consts: []core.Value{core.NewBoolean(false)},
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpJmpIfFalse, Arg: 0}},
		Debug:  []core.SrcPos{{}, {}},
	}
	wantInternal(t, runMalformed(t, p), "backward conditional jump")
}

func TestSeam7UnresolvableType(t *testing.T) {
	p := &compiler.Program{
		Code:  []compiler.Instr{{Op: compiler.OpPushType, Arg: 0}},
		Debug: []core.SrcPos{{}},
		Types: []compiler.TypeRef{{Name: "Bogus", ID: "no-such-id"}},
	}
	wantInternal(t, runMalformed(t, p), "unresolvable type operand Bogus")
}

func TestSeam7CodeUnitEndedWithoutRet(t *testing.T) {
	// main CALL_USERs into a unit that runs off the end with a live frame.
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(7)},
		Code:   []compiler.Instr{{Op: compiler.OpCallUser, Arg: 0}},
		Debug:  []core.SrcPos{{}},
		Fns: []compiler.CompiledFn{{
			Name: "noret", NParams: 0, NLocals: 0,
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}},
			Debug: []core.SrcPos{{}},
		}},
	}
	wantInternal(t, runMalformed(t, p), "code unit ended without RET")
}

// --- opForSetup range errors (direct) ------------------------------------

func TestSeam7ForSetupRangeErrors(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// Non-integer range triple.
	_, _, err := vc.opForSetup([]core.Value{core.NewString("a"), core.NewString("b"), core.NewString("c")}, nil, 0, nil, -1, 0, seam7Dbg)
	wantErr(t, err, "range must be concrete Integers")
	// Zero step (stack top→ start, then end, then step).
	_, _, err = vc.opForSetup([]core.Value{core.NewInteger(0), core.NewInteger(5), core.NewInteger(1)}, nil, 0, nil, -1, 0, seam7Dbg)
	wantErr(t, err, "step cannot be zero")
	// Underflow.
	_, _, err = vc.opForSetup([]core.Value{core.NewInteger(1)}, nil, 0, nil, -1, 0, seam7Dbg)
	wantInternal(t, err, "FOR_SETUP underflow")
}

// --- vmMark error arms (direct) ------------------------------------------

func TestSeam7VmMarkErrors(t *testing.T) {
	if _, _, err := vmMark(compiler.OpDropToMark, nil, []core.Value{core.NewInteger(1)}, seam7Dbg, 0); err == nil {
		t.Error("DROP_TO_MARK with no mark did not error")
	} else {
		wantInternal(t, err, "DROP_TO_MARK with no open mark")
	}
	// mark above current depth.
	if _, _, err := vmMark(compiler.OpDropToMark, []int{5}, []core.Value{core.NewInteger(1)}, seam7Dbg, 0); err == nil {
		t.Error("DROP_TO_MARK above depth did not error")
	} else {
		wantInternal(t, err, "DROP_TO_MARK above current depth")
	}
	if _, _, err := vmMark(compiler.OpPopMark, nil, nil, seam7Dbg, 0); err == nil {
		t.Error("POP_MARK with no mark did not error")
	} else {
		wantInternal(t, err, "POP_MARK with no open mark")
	}
}

// --- checkParamContract / checkNativeParamContract / checkReturnContract --

func TestSeam7CheckParamContractNilParam(t *testing.T) {
	r := seam7Reg(t)
	// A nil param slot is a guaranteed pass (the continue arm).
	fn := &compiler.CompiledFn{Name: "f", Params: []*core.Type{nil}}
	if err := checkParamContract(r, fn, []core.Value{core.NewInteger(1)}); err != nil {
		t.Errorf("nil param should pass, got %v", err)
	}
}

func TestSeam7CheckNativeParamContractArms(t *testing.T) {
	r := seam7Reg(t)
	// break arm: more args than the sig's TotalArgs.
	oneArg := core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: -1}
	if err := checkNativeParamContract(r, &compiler.SigRef{Word: "one", Sig: &oneArg}, []core.Value{core.NewInteger(1), core.NewInteger(2)}); err != nil {
		t.Errorf("extra args past sig arity should be ignored, got %v", err)
	}
	// Any slot: guaranteed pass.
	anySig := core.Signature{Args: []*core.Type{core.TAny}, BarrierPos: -1}
	if err := checkNativeParamContract(r, &compiler.SigRef{Word: "any", Sig: &anySig}, []core.Value{core.NewString("x")}); err != nil {
		t.Errorf("Any slot should pass, got %v", err)
	}
	// QuoteArgs slot: skipped (literal atom key bound as data).
	quoteSig := core.Signature{Args: []*core.Type{core.TInteger}, QuoteArgs: map[int]bool{0: true}, BarrierPos: -1}
	if err := checkNativeParamContract(r, &compiler.SigRef{Word: "q", Sig: &quoteSig}, []core.Value{core.NewAtom("k")}); err != nil {
		t.Errorf("QuoteArgs slot should be skipped, got %v", err)
	}
	// Mismatch raises the byte-identical signature_error.
	intSig := core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: -1}
	err := checkNativeParamContract(r, &compiler.SigRef{Word: "iw", Sig: &intSig}, []core.Value{core.NewString("x")})
	wantErr(t, err, "cannot call `iw`")
}

func TestSeam7CheckReturnContractUnderflow(t *testing.T) {
	r := seam7Reg(t)
	fn := &compiler.CompiledFn{Name: "g", Returns: []*core.Type{core.TInteger}}
	// Re-entrant (no frame) with too few values on the stack.
	_, err := checkReturnContract(r, fn, nil, 0, false, core.SrcPos{})
	wantErr(t, err, "g")
	if err == nil {
		t.Fatal("underflow return count did not error")
	}
}

// --- vmMakeMap / vmInterp underflow (direct) -----------------------------

func TestSeam7VmMakeMapInterpUnderflow(t *testing.T) {
	p := &compiler.Program{
		MakeMaps: []compiler.MakeMapSpec{{Keys: []string{"a", "b"}}},
		Interps:  []compiler.InterpSpec{{NHoles: 2, Segs: []compiler.InterpSeg{{Hole: true}, {Hole: true}}}},
	}
	_, err := vmMakeMap(p, nil, 0, seam7Dbg, 0)
	wantInternal(t, err, "MAKE_MAP stack underflow")
	_, err = vmInterp(p, nil, 0, seam7Dbg, 0)
	wantInternal(t, err, "INTERP stack underflow")
}

// --- callPoly / matchUserPoly error arms (direct) ------------------------

func TestSeam7CallPolyUnderflow(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.callPoly(&compiler.PolyRef{Word: "p", Arity: 2}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_NATIVE_POLY underflow at p")
}

func TestSeam7MatchUserPolyArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// underflow
	_, _, err := vc.matchUserPoly(&compiler.UserPolyRef{Word: "u", Arity: 2}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_USER_POLY underflow at u")
	// unresolved fn (arity 0, so no underflow; Lookup misses)
	_, _, err = vc.matchUserPoly(&compiler.UserPolyRef{Word: "nosuchfn", Arity: 0}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_USER_POLY unresolved fn nosuchfn")
}

// --- callDynamic family error arms (direct) ------------------------------

func TestSeam7CallDynamicArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// leading underflow
	_, _, err := vc.callDynamic(vc.r, 2, false, []core.Value{core.NewInteger(1)}, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC underflow")
	// trailing with arity != 1
	_, _, err = vc.callDynamic(vc.r, 2, true, []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC_TRAILING with arity != 1")
	// non-callable leading: value + args stay put.
	got, _, err := vc.callDynamic(vc.r, 1, false, []core.Value{core.NewInteger(9), core.NewInteger(5)}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("non-callable leading: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("non-callable leading residual len=%d, want 2", len(got))
	}
}

func TestSeam7CallDynTrailTopArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, _, err := vc.callDynTrailTop(vc.r, 1, nil, seam7Dbg, 0, compiler.DynApplyHead{})
	wantInternal(t, err, "CALL_DYN_TRAIL_TOP underflow")
	// non-callable: [args, fn] left untouched.
	got, _, err := vc.callDynTrailTop(vc.r, 1, []core.Value{core.NewInteger(5), core.NewInteger(9)}, seam7Dbg, 0, compiler.DynApplyHead{})
	if err != nil {
		t.Fatalf("trail-top non-callable: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("trail-top non-callable residual len=%d, want 2", len(got))
	}
}

func TestSeam7CallDynApplyTopArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, _, err := vc.callDynApplyTop(vc.r, 1, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYN_APPLY_TOP underflow")
	// A value that is no fn at all raises the interpreter's own `apply`
	// no-match over the top two stack values (the twenty-seventh increment:
	// a gradual lead can be data at run time); a Function-typed value with
	// a payload that is neither an FnDefInfo nor a closure raises
	// applyHandler's own error.
	_, _, err = vc.callDynApplyTop(vc.r, 1, []core.Value{core.NewInteger(5), core.NewInteger(9)}, seam7Dbg, 0)
	wantErr(t, err, "cannot call `apply`")
	wantErr(t, err, "the arguments were 9 (an Integer) and 5 (an Integer)")
	odd := core.Value{Parent: core.TFunction, Data: core.IntPayload{N: 1}}
	_, _, err = vc.callDynApplyTop(vc.r, 1, []core.Value{core.NewInteger(5), odd}, seam7Dbg, 0)
	wantErr(t, err, "carries no FnDefInfo")
}

func TestSeam7CallDynMethodArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, _, err := vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "m", NArgs: 1, NOut: 1}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYN_METHOD underflow at m")
	// non-appliable value on top: shape claim failed → defer.
	_, _, err = vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "m", NArgs: 1, NOut: 1}, []core.Value{core.NewInteger(5), core.NewInteger(9)}, seam7Dbg, 0)
	wantInternal(t, err, "is not an appliable function at run time")
}

func TestSeam7CallDynamicMixedUnderflow(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.callDynamicMixed(vc.r, 2, []core.Value{core.NewInteger(1)}, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC_MIXED underflow")
	_, err = vc.callDynamicMixed(vc.r, 0, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC_MIXED underflow")
}

// --- isDelegationFnDef / tryNativeFnApply (direct) -----------------------

func TestSeam7IsDelegationFnDefEmpty(t *testing.T) {
	if core.IsDelegationFnDef(core.FnDefInfo{}) {
		t.Error("sig-less FnDefInfo reported as delegation")
	}
}

func TestSeam7TryNativeFnApplyNoSigs(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// No registered inner and no own signatures → not applied (done=false).
	_, done, err := vc.tryNativeFnApply(core.NewFunction(core.FnDefInfo{Name: "unregistered-xyz"}), nil)
	if done || err != nil {
		t.Errorf("no-sig fn: done=%v err=%v, want done=false err=nil", done, err)
	}
	// Own signatures present but no runtime match → still not applied.
	fd := core.FnDefInfo{Name: "unregistered-xyz", Signatures: []core.Signature{{Args: []*core.Type{core.TInteger}, BarrierPos: -1}}}
	_, done, err = vc.tryNativeFnApply(core.NewFunction(fd), []core.Value{core.NewString("x")})
	if done || err != nil {
		t.Errorf("own-sig no-match: done=%v err=%v, want done=false err=nil", done, err)
	}
}

// --- runFallback error arms (direct) -------------------------------------

func TestSeam7RunFallbackArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.runFallback(vc.r, &core.FallbackSpan{NIn: 2, Desc: "d"}, nil, seam7Dbg, 0)
	wantInternal(t, err, "FALLBACK underflow at d")
	// NIn > 1 with enough stack: the lowerer never threads >1, so it is refused.
	_, err = vc.runFallback(vc.r, &core.FallbackSpan{NIn: 2, Desc: "d"}, []core.Value{core.NewInteger(1), core.NewInteger(2)}, seam7Dbg, 0)
	wantInternal(t, err, "FALLBACK threads >1 input at d")
}

// --- flowSignal no-loop (direct) -----------------------------------------

func TestSeam7FlowSignalNoLoop(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, _, _, _, _, _, err := vc.flowSignal(compiler.OpFlowBreak, nil, nil, nil, nil, 0, -1, seam7Dbg)
	wantInternal(t, err, "flow signal with no enclosing loop")
}

// --- closure fn-value apply branches -------------------------------------

// seam7IdentityClosureProg builds a program that pushes an argument, an
// identity closure ON TOP of it, then applies the given trailing-fn opcode
// (OpCallDynTrailTop / OpCallDynApplyTop / OpCallDynMethod), driving the
// closure-invoke branch of the corresponding apply method.
func seam7IdentityClosureProg(applyOp compiler.Opcode) *compiler.Program {
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(5)},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},   // arg 5
			{Op: compiler.OpPushClosure, Arg: 0}, // identity closure on top
			{Op: applyOp, Arg: 1},                // apply the closure to its 1 arg
		},
		Debug: []core.SrcPos{{}, {}, {}},
		Fns: []compiler.CompiledFn{{
			Name: "id", NParams: 1, NLocals: 1, Returns: []*core.Type{core.TAny},
			Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet}},
			Debug: []core.SrcPos{{}, {}},
		}},
	}
	if applyOp == compiler.OpCallDynMethod {
		p.DynMethods = []compiler.DynMethodSpec{{Word: "m", NArgs: 1, NOut: 1}}
		p.Code[2] = compiler.Instr{Op: compiler.OpCallDynMethod, Arg: 0}
	}
	return p
}

func TestSeam7ClosureApplyBranches(t *testing.T) {
	for _, op := range []compiler.Opcode{compiler.OpCallDynTrailTop, compiler.OpCallDynApplyTop, compiler.OpCallDynMethod} {
		p := seam7IdentityClosureProg(op)
		got, err := RunProgram(p, seam7Reg(t))
		if err != nil {
			t.Fatalf("%v closure apply: %v", op, err)
		}
		if len(got) != 1 {
			t.Fatalf("%v closure apply residual len=%d, want 1", op, len(got))
		}
		if n, _ := got[0].AsConcreteInteger(); n != 5 {
			t.Errorf("%v identity closure returned %d, want 5", op, n)
		}
	}
}

// --- run()-level CALL_DYN error propagation arms -------------------------

func TestSeam7RunCallDynErrArms(t *testing.T) {
	// OpCallDynTrailTop / OpCallDynApplyTop with an empty stack: the method
	// underflows and run() propagates the error (vm.go run-loop err arms).
	for _, op := range []compiler.Opcode{compiler.OpCallDynTrailTop, compiler.OpCallDynApplyTop} {
		p := &compiler.Program{Code: []compiler.Instr{{Op: op, Arg: 1}}, Debug: []core.SrcPos{{}}}
		_, err := RunProgram(p, seam7Reg(t))
		if err == nil {
			t.Fatalf("%v empty-stack apply did not error", op)
		}
		wantInternal(t, err, "underflow")
	}
}

// --- delegation fn-value apply branches ----------------------------------

// seam7DelegReg registers a `cinc` (Integer→Integer, n+1) native and a
// `cfail` (always errors) native, and returns delegation fn VALUES wrapping
// each — a trivial-delegation FnDefInfo whose only sig body is `[Word(name)]`.
func seam7DelegReg(t *testing.T) (r *core.Registry, inc, fail core.Value) {
	t.Helper()
	r = seam7Reg(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cinc",
		Signatures: []core.Signature{{
			Args:    []*core.Type{core.TInteger},
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				n, _ := core.AsInteger(a[0])
				return []core.Value{core.NewInteger(n + 1)}, nil
			}),
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cfail",
		Signatures: []core.Signature{{
			Args:    []*core.Type{core.TInteger},
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
				return nil, r.BoruError("value_error", "cfail: boom", "cfail")
			}),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	deleg := func(name string) core.Value {
		return core.NewFunction(core.FnDefInfo{
			Name: name, Registry: r,
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
				Impl: core.Boru([]core.Value{core.NewWord(name)}),
			}},
		})
	}
	return r, deleg("cinc"), deleg("cfail")
}

func TestSeam7DelegationApplySuccess(t *testing.T) {
	r, inc, _ := seam7DelegReg(t)
	if !core.IsDelegationFnDef(inc.Data.(core.FnDefInfo)) {
		t.Fatal("cinc wrapper is not recognised as a delegation fn")
	}
	vc := seam7VC(r)
	// callDynTrailTop / callDynApplyTop: fn ON TOP of its arg → [arg, fn].
	trailTop := func(reg *core.Registry, n int, stack []core.Value, dbg []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
		return vc.callDynTrailTop(reg, n, stack, dbg, pc, compiler.DynApplyHead{})
	}
	for _, apply := range []func(*core.Registry, int, []core.Value, []core.SrcPos, int) ([]core.Value, *dynEnter, error){trailTop, vc.callDynApplyTop} {
		got, _, err := apply(vc.r, 1, []core.Value{core.NewInteger(5), inc}, seam7Dbg, 0)
		if err != nil {
			t.Fatalf("delegation apply: %v", err)
		}
		if n, _ := got[0].AsConcreteInteger(); n != 6 {
			t.Errorf("delegation cinc(5) = %d, want 6", n)
		}
	}
	// callDynamic: LEADING fn → [fn, arg].
	got, _, err := vc.callDynamic(vc.r, 1, false, []core.Value{inc, core.NewInteger(5)}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("leading delegation apply: %v", err)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 6 {
		t.Errorf("leading delegation cinc(5) = %d, want 6", n)
	}
	// callDynMethod: fn ON TOP, shape claim {NArgs:1, NOut:1}.
	got, _, err = vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "cinc", NArgs: 1, NOut: 1}, []core.Value{core.NewInteger(5), inc}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("method delegation apply: %v", err)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 6 {
		t.Errorf("method delegation cinc(5) = %d, want 6", n)
	}
}

func TestSeam7DelegationApplyError(t *testing.T) {
	r, _, fail := seam7DelegReg(t)
	vc := seam7VC(r)
	// Each apply path must surface the inner handler's error (delegation err arm).
	_, _, err := vc.callDynTrailTop(vc.r, 1, []core.Value{core.NewInteger(5), fail}, seam7Dbg, 0, compiler.DynApplyHead{})
	wantErr(t, err, "cfail: boom")
	_, _, err = vc.callDynApplyTop(vc.r, 1, []core.Value{core.NewInteger(5), fail}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, _, err = vc.callDynamic(vc.r, 1, false, []core.Value{fail, core.NewInteger(5)}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, _, err = vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "cfail", NArgs: 1, NOut: 1}, []core.Value{core.NewInteger(5), fail}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
}

// --- closure-invoke error arms -------------------------------------------

// seam7ErrClosureProg builds a program whose closure declares a return type
// its body violates, so invokeClosure surfaces the return-type error and each
// apply method takes its closure-invoke ERROR arm.
func seam7ErrClosureProg(applyOp compiler.Opcode) *compiler.Program {
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(5), core.NewString("x")},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},
			{Op: compiler.OpPushClosure, Arg: 0},
			{Op: applyOp, Arg: 1},
		},
		Debug: []core.SrcPos{{}, {}, {}},
		Fns: []compiler.CompiledFn{{
			Name: "bad", NParams: 1, NLocals: 1, Returns: []*core.Type{core.TInteger},
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpRet}}, // returns a String
			Debug: []core.SrcPos{{}, {}},
		}},
	}
	if applyOp == compiler.OpCallDynMethod {
		p.DynMethods = []compiler.DynMethodSpec{{Word: "m", NArgs: 1, NOut: 1}}
		p.Code[2] = compiler.Instr{Op: compiler.OpCallDynMethod, Arg: 0}
	}
	return p
}

func TestSeam7ClosureApplyErrorArms(t *testing.T) {
	for _, op := range []compiler.Opcode{compiler.OpCallDynTrailTop, compiler.OpCallDynApplyTop, compiler.OpCallDynMethod} {
		_, err := RunProgram(seam7ErrClosureProg(op), seam7Reg(t))
		if err == nil {
			t.Fatalf("%v erroring closure did not surface an error", op)
		}
	}
	// callDynamic's leading closure error arm: [closure, arg], apply leading.
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(5), core.NewString("x")},
		Code: []compiler.Instr{
			{Op: compiler.OpPushClosure, Arg: 0}, // closure at the base
			{Op: compiler.OpPushConst, Arg: 0},   // its arg on top
			{Op: compiler.OpCallDynamic, Arg: 1},
		},
		Debug: []core.SrcPos{{}, {}, {}},
		Fns: []compiler.CompiledFn{{
			Name: "bad", NParams: 1, NLocals: 1, Returns: []*core.Type{core.TInteger},
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpRet}},
			Debug: []core.SrcPos{{}, {}},
		}},
	}
	if _, err := RunProgram(p, seam7Reg(t)); err == nil {
		t.Fatal("leading closure error arm did not surface an error")
	}
}

// --- island-run error arms -----------------------------------------------

// seam7UserFail returns an appliable, non-delegation, non-closure user-fn
// VALUE whose body calls the erroring `cfail` native — so applying it forces
// the island sub-engine to error, exercising each apply method's island error
// arm. (A named param disqualifies it from the trivial-delegation fast path.)
func seam7UserFail(r *core.Registry) core.Value {
	return core.NewFunction(core.FnDefInfo{
		Name: "cuserfail", Registry: r,
		Signatures: []core.Signature{{
			Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
			Returns: []*core.Type{core.TInteger}, BarrierPos: core.BarrierAllForward,
			Impl: core.Boru([]core.Value{core.NewWord("cfail"), core.NewWord("n")}),
		}},
	})
}

func TestSeam7IslandApplyErrorArms(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	fn := seam7UserFail(r)
	if core.IsDelegationFnDef(fn.Data.(core.FnDefInfo)) {
		t.Fatal("user fail fn should NOT be a trivial delegation")
	}
	vc := seam7VC(r)
	_, _, err := vc.callDynTrailTop(vc.r, 1, []core.Value{core.NewInteger(5), fn}, seam7Dbg, 0, compiler.DynApplyHead{})
	wantErr(t, err, "cfail: boom")
	_, _, err = vc.callDynApplyTop(vc.r, 1, []core.Value{core.NewInteger(5), fn}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, _, err = vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "cuserfail", NArgs: 1, NOut: 1}, []core.Value{core.NewInteger(5), fn}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	// callDynamicMixed islands its window verbatim — a window that calls cfail
	// errors through the island (the mixed island error arm).
	_, err = vc.callDynamicMixed(vc.r, 2, []core.Value{core.NewWord("cfail"), core.NewInteger(5)}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
}

// TestSeam7MatchUserPolyUnitShapeMismatch drives the unit-shape guard: a
// recorded arm whose Impl/arity still match the live table but whose compiled
// unit index is out of range (a compile/run drift) is refused.
func TestSeam7MatchUserPolyUnitShapeMismatch(t *testing.T) {
	r := seam7Reg(t)
	core.InstallFnDef(r, "cpoly", core.FnDefInfo{
		Signatures: []core.Signature{{
			Params:  []core.FnParam{{Name: "n", Type: core.TAny}},
			Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
			Impl: core.Boru([]core.Value{core.NewWord("n")}),
		}},
	})
	fd := r.Lookup("cpoly")
	if fd == nil || len(fd.Signatures) == 0 {
		t.Fatal("cpoly not installed")
	}
	n := fd.Signatures[0].TotalArgs()
	vc := &vmContext{r: r, p: &compiler.Program{}} // no Fns → any unit index is out of range
	pr := &compiler.UserPolyRef{
		Word: "cpoly", Arity: n,
		SigIdx: []int{0}, Units: []int{999}, Impls: []core.SigImpl{fd.Signatures[0].Impl},
	}
	window := make([]core.Value, n)
	for i := range window {
		window[i] = core.NewInteger(1)
	}
	_, _, err := vc.matchUserPoly(pr, window, seam7Dbg, 0)
	wantInternal(t, err, "CALL_USER_POLY unit shape mismatch at cpoly")
}

// TestSeam7CallUserPolyParamContract drives OpCallUserPoly where the arm
// matches at the signature level (an Any param) but the compiled fn's concrete
// param type rejects the runtime value — the param-contract guard the checker
// installs for gradual poly dispatch.
func TestSeam7CallUserPolyParamContract(t *testing.T) {
	r := seam7Reg(t)
	core.InstallFnDef(r, "cpoly2", core.FnDefInfo{
		Signatures: []core.Signature{{
			Params:  []core.FnParam{{Name: "n", Type: core.TAny}},
			Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
			Impl: core.Boru([]core.Value{core.NewWord("n")}),
		}},
	})
	fd := r.Lookup("cpoly2")
	n := fd.Signatures[0].TotalArgs()
	p := &compiler.Program{
		Consts: []core.Value{core.NewString("x")}, // a String flows into a concrete Integer param
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallUserPoly, Arg: 0}},
		Debug:  []core.SrcPos{{}, {}},
		UserPolys: []compiler.UserPolyRef{{
			Word: "cpoly2", Arity: n,
			SigIdx: []int{0}, Units: []int{0}, Impls: []core.SigImpl{fd.Signatures[0].Impl},
		}},
		Fns: []compiler.CompiledFn{{
			Name: "cpoly2", NParams: n, NLocals: n, Params: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny},
			Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet}},
			Debug: []core.SrcPos{{}, {}},
		}},
	}
	_, err := RunProgram(p, r)
	wantErr(t, err, "cannot call `cpoly2`")
}

func TestSeam7CallNativeHandlerTokenScreened(t *testing.T) {
	r := seam7Reg(t)
	sig := core.Signature{
		Args: nil, BarrierPos: -1,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return []core.Value{core.NewWord("leak")}, nil // a tape-coupled token must never reach the stack
		}),
	}
	p := &compiler.Program{
		Code:  []compiler.Instr{{Op: compiler.OpCallNative, Arg: 0}},
		Debug: []core.SrcPos{{}},
		Sigs:  []compiler.SigRef{{Word: "cretword", Sig: &sig}},
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "tape-coupled handler result at cretword")
}

// TestSeam7CallDynApplyOneArms pins the EVENT form of the apply-word op
// (OpCallDynApplyOne, the twenty-seventh increment) arm by arm: a fn of
// the window's arity commits its one result; a 0-arg anonymous fn is
// MARKED applied and fires, and the extra value it leaves beside the
// receiver DEFERS the run (the model committed one result); a lens on top
// defers; a compiled closure of another arity takes the interpreter's apply
// re-step, and one whose payload names no unit is invoked as it is.
func TestSeam7CallDynApplyOneArms(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	one := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1,
		Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			n, _ := core.AsInteger(a[0])
			return []core.Value{core.NewInteger(n * 3)}, nil
		}),
	}}})
	core.NormalizeSig(&one.Data.(core.FnDefInfo).Signatures[0])
	got, ent, err := vc.callDynApply(r, 1, []core.Value{core.NewInteger(5), one}, seam7Dbg, 0, true)
	if err != nil || ent != nil || len(got) != 1 {
		t.Fatalf("a 1-arg fn over its one arg commits one result: %v %v %v", got, ent, err)
	}
	if n, _ := core.AsInteger(got[0]); n != 15 {
		t.Errorf("15: %v", got)
	}
	zero := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		BarrierPos: 0,
		Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return []core.Value{core.NewInteger(42)}, nil
		}),
	}}})
	core.NormalizeSig(&zero.Data.(core.FnDefInfo).Signatures[0])
	// The tail form (one=false) keeps whatever the re-step leaves: the 0-arg
	// fn fires and its result lands ABOVE the untouched receiver, exactly
	// the interpreter's [4 42].
	got, _, err = vc.callDynApply(r, 1, []core.Value{core.NewInteger(4), zero}, seam7Dbg, 0, false)
	if err != nil || len(got) != 2 {
		t.Fatalf("the tail form keeps the re-step's residual: %v %v", got, err)
	}
	if a, _ := core.AsInteger(got[0]); a != 4 {
		t.Errorf("the receiver stays beneath the 0-arg fn's result: %v", got)
	}
	if b, _ := core.AsInteger(got[1]); b != 42 {
		t.Errorf("the 0-arg fn fired (MarkApplied): %v", got)
	}
	// The event form defers the same state.
	_, _, err = vc.callDynApply(r, 1, []core.Value{core.NewInteger(4), zero}, seam7Dbg, 0, true)
	wantInternal(t, err, "netted 2 value(s)")
	// A lens on top defers.
	_, _, err = vc.callDynApply(r, 1, []core.Value{core.NewInteger(4), core.NewValueRaw(core.TReach, core.NonePayload{})}, seam7Dbg, 0, true)
	wantInternal(t, err, "apply over a lens value")
	// closureUnit: a payload naming no program or a unit beyond it names no
	// unit (such a closure is invoked as it is, and the invoker answers).
	if _, known := vc.closureUnit(core.ClosurePayload{Prog: nil, Unit: 0}); known {
		t.Error("no program: no unit")
	}
	if _, known := vc.closureUnit(core.ClosurePayload{Prog: &compiler.Program{}, Unit: 0}); known {
		t.Error("a unit beyond the program: no unit")
	}
	// The resolved re-step surfaces the island's own error.
	bad := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1,
		Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, fmt.Errorf("boom")
		}),
	}}})
	core.NormalizeSig(&bad.Data.(core.FnDefInfo).Signatures[0])
	if _, err := vc.applyReStep(nil, bad, []core.Value{core.NewInteger(1)}, seam7Dbg, 0); err == nil {
		t.Error("the island's error rides out of the re-step")
	}
}
