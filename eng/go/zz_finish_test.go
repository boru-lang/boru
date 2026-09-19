package eng

import (
	"errors"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

func TestW8NamedFnValueCompileMode(t *testing.T) {
	// A non-anonymous body-bearing fn VALUE reached as a call while the
	// bytecode emitter is active routes through check.SpliceFnValueCheckResult.
	r := covRegistry(t, nil)
	fnv := namedFnVal("w8nf",
		[]core.FnParam{{Name: "a", Type: core.TInteger}},
		[]*core.Type{core.TInteger},
		parenBody(core.NewWord("cadd"), core.NewWord("a"), core.NewWord("a")),
	)
	if _, reason := compileTokens(t, r, []core.Value{fnv, core.NewInteger(4)}); reason != "" {
		// Not required to compile cleanly; the point is exercising the arm.
		t.Logf("compile reason: %s", reason)
	}
}

func TestCompiledNamedFnValueCall(t *testing.T) {
	// A named body-bearing fn value CALLED while the emitter is active
	// routes through check.SpliceFnValueCheckResult. If the program compiles it
	// must agree with the interpreter; a refusal is also sound.
	tokens := func() []core.Value {
		fnv := namedFnVal("vdbl",
			[]core.FnParam{{Name: "x", Type: core.TInteger}},
			[]*core.Type{core.TInteger},
			parenBody(core.NewWord("cadd"), core.NewWord("x"), core.NewWord("x")),
		)
		return []core.Value{fnv, core.NewInteger(16)}
	}
	ri := covRegistry(t, nil)
	iOut, iErr := core.NewTop(ri).Run(tokens())
	if iErr != nil {
		t.Fatalf("interpreted: %v", iErr)
	}
	if got := renderAll(iOut); got != "32" {
		t.Fatalf("interpreted = %q, want 32", got)
	}
	rc := covRegistry(t, nil)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Logf("compile refused (%s) — interpreter owns the case", reason)
		return
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled: %v", cErr)
	}
	if got := renderAll(cOut); got != "32" {
		t.Errorf("compiled = %q, want 32", got)
	}
}

func TestCompiledAnonFnValueCall(t *testing.T) {
	// Anonymous lambda dispatch under the emitter: the ReturnsFn attached
	// by compileFnDef runs AnalyseFnBody for the return type.
	tokens := func() []core.Value {
		fnv := anonFnVal(
			[]core.FnParam{{Name: "a", Type: core.TInteger}, {Name: "b", Type: core.TInteger}},
			nil,
			parenBody(core.NewWord("cmul"), core.NewWord("a"), core.NewWord("b")),
		)
		return []core.Value{fnv, core.NewInteger(6), core.NewInteger(9)}
	}
	ri := covRegistry(t, nil)
	iOut, iErr := core.NewTop(ri).Run(tokens())
	if iErr != nil {
		t.Fatalf("interpreted: %v", iErr)
	}
	if got := renderAll(iOut); got != "54" {
		t.Fatalf("interpreted = %q, want 54", got)
	}
	rc := covRegistry(t, nil)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Logf("compile refused (%s) — interpreter owns the case", reason)
		return
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled: %v", cErr)
	}
	if got := renderAll(cOut); got != "54" {
		t.Errorf("compiled = %q, want 54", got)
	}
}

func TestCompiledComputedListArg(t *testing.T) {
	// A computed list consumed at the top frame records OpMakeList; the
	// compiled program must agree with the interpreter.
	got := runDifferential(t, nil, func() []core.Value {
		lv := core.NewList([]core.Value{core.NewWord("cadd"), core.NewInteger(2), core.NewInteger(3), core.NewEnd(), core.NewInteger(9)})
		lv.Eval = true
		return []core.Value{core.NewWord("clen"), lv}
	})
	if got != "2" {
		t.Errorf("computed list len = %q", got)
	}
}

func TestCompiledInterpString(t *testing.T) {
	// An interp string whose hole is a const expression folds; one with a
	// dispatch records OpInterp (or refuses) — parity either way.
	got := runDifferential(t, nil, func() []core.Value {
		return []core.Value{core.NewInterpString([]core.InterpPart{
			{Lit: "v="},
			{Expr: []core.Value{core.NewWord("cadd"), core.NewInteger(20), core.NewInteger(22)}},
		})}
	})
	if !strings.Contains(got, "v=42") {
		t.Errorf("interp = %q", got)
	}
}

// A unit whose body is [PUSH_LOCAL 0, RET] returns its single param verbatim.
func TestRunUnitBindsParam(t *testing.T) {
	p := &compiler.Program{
		Fns: []compiler.CompiledFn{{
			Name:    "echo",
			NParams: 1,
			NLocals: 1,
			Code:    []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug:   []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &compiler.CompiledFnRef{Prog: p, Unit: 0}
	out, err := RunUnit(ref, runUnitReg(t), []core.Value{core.NewInteger(7)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("result = %v, want 7", out[0])
	}
}

// A capture fills the trailing local slot: NParams=2, NCaptures=1, body returns
// local 1 (the capture), so the per-call arg lands in slot 0 and the capture in
// slot 1.
func TestRunUnitBindsCapture(t *testing.T) {
	p := &compiler.Program{
		Fns: []compiler.CompiledFn{{
			Name:      "grabCapture",
			NParams:   2,
			NCaptures: 1,
			NLocals:   2,
			Code:      []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 1}, {Op: compiler.OpRet, Arg: 0}},
			Debug:     []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &compiler.CompiledFnRef{Prog: p, Unit: 0, Captures: []core.Value{core.NewInteger(99)}}
	out, err := RunUnit(ref, runUnitReg(t), []core.Value{core.NewInteger(7)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 99 {
		t.Fatalf("result = %v, want the captured 99", out[0])
	}
}

// A unit index outside Prog.Fns is rejected (never indexed).
func TestRunUnitUnitOutOfRange(t *testing.T) {
	p := &compiler.Program{Fns: []compiler.CompiledFn{{Name: "only", NParams: 0, NLocals: 0,
		Code: []compiler.Instr{{Op: compiler.OpRet, Arg: 0}}, Debug: []core.SrcPos{{}}}}}
	for _, unit := range []int{-1, 5} {
		ref := &compiler.CompiledFnRef{Prog: p, Unit: unit}
		if _, err := RunUnit(ref, runUnitReg(t), nil); err == nil ||
			!strings.Contains(err.Error(), "out of range") {
			t.Fatalf("RunUnit(unit=%d) err = %v, want out of range", unit, err)
		}
	}
}

// A nil ref and a ref with a nil program both report "nil unit reference"
// (the invoke seam treats a nil ref as "no runnable unit").
func TestRunUnitNilRef(t *testing.T) {
	for _, ref := range []*compiler.CompiledFnRef{nil, {Prog: nil, Unit: 0}} {
		if _, err := RunUnit(ref, runUnitReg(t), nil); err == nil ||
			!strings.Contains(err.Error(), "nil unit reference") {
			t.Fatalf("RunUnit(%v) err = %v, want nil unit reference", ref, err)
		}
	}
}

func TestCompiledBranchConstCondValue(t *testing.T) {
	// A const (non-event) condition value reaches lowerBranch's default
	// pushOperand arm: cif over a literal-true word condition.
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewBoolean(true),
			codeBody(core.NewInteger(1)),
			codeBody(core.NewInteger(2)),
		}
	})
	if got != "1" {
		t.Errorf("const-cond value = %q", got)
	}
}

// --- paren-bounded dynamic apply ------------------------------------------------------

// swapTailArgs — the tail-call bracket swap: with a live frame it replaces
// the entry above the frame's base; at activation root it truncates to the
// run floor; outside DynEnv it is a no-op.
func TestSwapTailArgsBranches(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	vc := &vmContext{p: &compiler.Program{DynEnv: true}, r: r, argsFloor: 0}
	nl := []core.Value{core.NewInteger(7)}
	// Root tail swap: floor-truncate then push.
	_ = r.Args.Push(core.NewList([]core.Value{core.NewInteger(1)}))
	vc.swapTailArgs(nil, nl, 1)
	if r.Args.Depth() != 1 {
		t.Errorf("root tail swap: want depth 1, got %d", r.Args.Depth())
	}
	// Framed tail swap: replace the top entry above the frame's base.
	frames := []vmFrame{{argsBase: 1}}
	_ = r.Args.Push(core.NewList([]core.Value{core.NewInteger(2)}))
	vc.swapTailArgs(frames, nl, 1)
	if r.Args.Depth() != 2 {
		t.Errorf("framed tail swap: want depth 2, got %d", r.Args.Depth())
	}
	top, ok, _ := r.Args.Top()
	if !ok {
		t.Fatalf("framed tail swap: no top")
	}
	if lst, lerr := core.AsList(top); lerr != nil || lst.Len() != 1 {
		t.Errorf("framed tail swap: top must be the new callee's args [7], got %v", top)
	} else if n, nerr := core.AsInteger(lst.Slice()[0]); nerr != nil || n != 7 {
		t.Errorf("framed tail swap: want [7], got %v", top)
	}
	// Non-DynEnv: all three helpers no-op.
	off := &vmContext{p: &compiler.Program{}, r: r}
	d := r.Args.Depth()
	off.swapTailArgs(frames, nl, 1)
	off.pushFrameArgs(nl, 1)
	off.retFrameArgs(&vmFrame{argsBase: 0})
	if r.Args.Depth() != d {
		t.Errorf("non-DynEnv helpers must not touch the args stack")
	}
}

// The RET contract's NUnnamed arms (both frame shapes): deficit errors, an
// over-allowance surplus errors after the trim, and the frameless trim caps
// at NUnnamed.
func TestCheckReturnContractNUnnamedArms(t *testing.T) {
	r := seam7Reg(t)
	fn := &compiler.CompiledFn{Name: "z9f", Returns: []*core.Type{core.TInteger}, NUnnamed: 1}

	// hasFrame deficit: 0 produced above the base, 1 declared.
	if _, err := checkReturnContract(r, fn, []core.Value{core.NewInteger(9)}, 1, true, core.SrcPos{}); err == nil {
		t.Error("frame deficit must error")
	} else if !strings.Contains(err.Error(), "z9f") {
		t.Errorf("deficit error %q should name the fn", err)
	}

	// hasFrame surplus beyond the unnamed allowance: 3 produced, 1 declared,
	// NUnnamed 1 → 1 extra tolerated, the second errors.
	stack := []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}
	if _, err := checkReturnContract(r, fn, stack, 0, true, core.SrcPos{}); err == nil {
		t.Error("frame surplus beyond NUnnamed must error")
	}

	// hasFrame surplus within the allowance: trimmed from the frame bottom.
	stack = []core.Value{core.NewInteger(1), core.NewInteger(2)}
	out, err := checkReturnContract(r, fn, stack, 0, true, core.SrcPos{})
	if err != nil {
		t.Fatalf("frame trim: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("frame trim left %v, want the single declared return", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 2 {
		t.Errorf("frame trim kept %v, want the TOP value 2", out[0])
	}

	// Frameless trim: extra bottoms discarded up to NUnnamed.
	out, err = checkReturnContract(r, fn, []core.Value{core.NewInteger(1), core.NewInteger(2)}, 0, false, core.SrcPos{})
	if err != nil {
		t.Fatalf("frameless trim: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("frameless trim left %v, want one value", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 2 {
		t.Errorf("frameless trim kept %v, want the TOP value 2", out[0])
	}

	// Frameless trim CAP: extra 2 but NUnnamed 1 — only one discards, the
	// remaining surplus stays (the residual absorbs it).
	out, err = checkReturnContract(r, fn, []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}, 0, false, core.SrcPos{})
	if err != nil {
		t.Fatalf("frameless capped trim: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("frameless capped trim left %v, want 2 values (cap at NUnnamed)", out)
	}
}

// A runtime user-poly window that matches NO baked arm defers to the
// interpreter (matchUserPoly's no-match, reached through OpCallUserPoly),
// and a poly ref whose recorded impl no longer matches the registered
// signature defers with the drift diagnostic.
func TestSeam7CallUserPolyRuntimeNoMatchAndDrift(t *testing.T) {
	r := seam7Reg(t)
	core.InstallFnDef(r, "z9poly", core.FnDefInfo{
		Signatures: []core.Signature{{
			Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
			Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
			Impl: core.Boru([]core.Value{core.NewWord("n")}),
		}},
	})
	fd := r.Lookup("z9poly")
	p := &compiler.Program{
		Consts: []core.Value{core.NewBoolean(true)}, // matches no arm (Integer only)
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallUserPoly, Arg: 0}},
		Debug:  []core.SrcPos{{}, {}},
		UserPolys: []compiler.UserPolyRef{{
			Word: "z9poly", Arity: 1,
			SigIdx: []int{0}, Units: []int{0}, Impls: []core.SigImpl{fd.Signatures[0].Impl},
		}},
		Fns: []compiler.CompiledFn{{
			Name: "z9poly", NParams: 1, NLocals: 1, Returns: []*core.Type{core.TAny},
			Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet}},
			Debug: []core.SrcPos{{}, {}},
		}},
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "CALL_USER_POLY no match for z9poly")

	// Drift: the recorded impl is not the registered signature's impl.
	vc := seam7VC(r)
	drift := &compiler.UserPolyRef{
		Word: "z9poly", Arity: 1,
		SigIdx: []int{0}, Units: []int{0}, Impls: []core.SigImpl{core.Boru([]core.Value{core.NewWord("other")})},
	}
	_, _, derr := vc.matchUserPoly(drift, []core.Value{core.NewInteger(1)}, seam7Dbg, 0)
	wantInternal(t, derr, "CALL_USER_POLY signature drift at z9poly")
}

// TestPolyNoMatchRaiseDeclines pins every arm that keeps the sound defer: no
// recorded spec, an unresolvable word, a signature table whose length drifted
// since the record, a NARROWER-arity overload now live, and a recorded index
// outside the window (per tuple).
func TestPolyNoMatchRaiseDeclines(t *testing.T) {
	r := pnmRegistry(t, []core.Signature{pnmSig(-1, core.TInteger, core.TString)})
	vc := seam7VC(r)
	fn := r.Lookup("pnmw")
	if fn == nil {
		t.Fatal("pnmw not registered")
	}
	window := []core.Value{core.NewBoolean(true), core.NewBoolean(false)}
	if err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("a nil spec must keep the defer, got %v", err)
	}
	spec := &core.PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 1}
	if err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, nil, window, seam7Dbg, 0); err != nil {
		t.Errorf("a nil fn must keep the defer, got %v", err)
	}
	drift := &core.PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 2}
	if err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: drift}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("a table-length drift must keep the defer, got %v", err)
	}
	// A narrower-arity overload could match a runtime collection this raise
	// cannot model — the record-time screen excluded it, so its presence at
	// run time is drift.
	r2 := pnmRegistry(t, []core.Signature{pnmSig(-1, core.TInteger, core.TString), pnmSig(-1, core.TInteger)})
	fn2 := r2.Lookup("pnmw")
	narrow := &core.PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 2}
	if err := seam7VC(r2).polyNoMatchRaise(r2, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: narrow}, fn2, window, seam7Dbg, 0); err != nil {
		t.Errorf("a narrower-arity live overload must keep the defer, got %v", err)
	}
	oobW := &core.PolyNoMatchSpec{Written: []int{2}, StackTuple: []int{0}, NSigs: 1}
	if err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: oobW}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("an out-of-window Written index must keep the defer, got %v", err)
	}
	oobS := &core.PolyNoMatchSpec{Written: []int{0}, StackTuple: []int{-1}, NSigs: 1}
	if err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: oobS}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("an out-of-window StackTuple index must keep the defer, got %v", err)
	}
}

// The VM's module-poly re-match arm: a module poly word resolves its
// overload at run time, and the gate runs AFTER the match so it applies
// to the overload that actually dispatches.
func TestModuleGateVMPolyRematchArm(t *testing.T) {
	impl := core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{args[0]}, nil
	})
	r := moduleGateReg(t, "boru:zz", "denied")
	sub, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("sub registry: %v", err)
	}
	sub.ModuleRef = "boru:zz"
	sub.Register("zz-poly", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1, Impl: impl,
		ModuleCall: &core.ModuleCallID{Module: "boru:zz", Export: "denied"},
	})

	p := &compiler.Program{
		Code:     []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNativePoly, Arg: 0}},
		Consts:   []core.Value{core.NewInteger(1)},
		PolyRefs: []compiler.PolyRef{{Word: "zz-poly", Arity: 1, Reg: sub}},
	}
	if _, err := RunProgram(p, r); err == nil || !strings.Contains(err.Error(), "module export denied") {
		t.Fatalf("the re-matched module overload must gate, got %v", err)
	}

	// Negative twin: the same program under a checker denying a
	// different export dispatches.
	if _, err := RunProgram(p, moduleGateReg(t, "boru:zz", "other")); err != nil &&
		strings.Contains(err.Error(), "zz-policy") {
		t.Fatalf("an unruled export must pass the gate, got %v", err)
	}
}

// TestInvokeCallbackInternalErrorFallsBack pins PR #243 comment #3: a callback
// fires AFTER the enclosing RunProgram returned (serve-raw handling a connection,
// a spawned process), so there is no outer RunCompiled to catch a VM bailout and
// re-run on the interpreter. InvokeCallback therefore applies that discipline
// itself — when the stamped unit raises an internal_error (here a CALL_DYNAMIC
// underflow, the vmErrAt soundness-bailout class), it falls back to CallBoru over
// the sig's boru body instead of leaking the raw internal_error to the peer. The
// body `[42]` is NOT run: the unit ran and failed, and its failure is the
// answer. It used to be retried through CallBoru so the interpreter produced
// 42 and the soundness bug left no mark.
func TestInvokeCallbackInternalErrorPropagates(t *testing.T) {
	// A unit that raises internal_error when run: CALL_DYNAMIC over an empty
	// stack underflows → vmErrAt(internal_error).
	p := &compiler.Program{Fns: []compiler.CompiledFn{{
		Name: "boom", NParams: 0, NLocals: 0,
		Code:  []compiler.Instr{{Op: compiler.OpCallDynamic, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
		Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
	}}}
	ref := &compiler.CompiledFnRef{Prog: p, Unit: 0}
	// Confirm the unit really does raise internal_error on its own.
	if _, err := RunUnit(ref, runUnitReg(t), nil); !core.IsInternalErr(err) {
		t.Fatalf("RunUnit err = %v, want an internal_error to drive the fallback", err)
	}
	// The sig carries the stamped ref AND a boru body CallBoru can run to 42.
	sig := &core.Signature{Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(42)}, ref)}
	out, err := core.InvokeCallback(runUnitReg(t), sig, nil, nil)
	if !core.IsInternalErr(err) {
		t.Fatalf("InvokeCallback must propagate the unit's internal error, got out=%v err=%v", out, err)
	}
}

// TestInvokeCallbackBusyRegistryFallsBack covers the no-VM-path branch of
// invokeCompiledUnit: a stamped ref whose registry is mid-run (canHostVM false)
// with no nestedRunner installed cannot run on the VM, so invokeCompiledUnit
// reports ran=false and InvokeCallback owns the call on the interpreter. The
// CallBoru body `[7]` proves the interpreter ran.
func TestInvokeCallbackBusyRegistryFallsBack(t *testing.T) {
	r := runUnitReg(t)
	r.VmRunning = 1 // canHostVM() false; nestedRunner stays nil
	p := &compiler.Program{Fns: []compiler.CompiledFn{{
		Name: "x", NParams: 0, NLocals: 0,
		Code: []compiler.Instr{{Op: compiler.OpRet, Arg: 0}}, Debug: []core.SrcPos{{Row: 1, Col: 1}},
	}}}
	ref := &compiler.CompiledFnRef{Prog: p, Unit: 0}
	// Direct: no VM path applies, so ran=false.
	if _, _, ran := invokeCompiledUnit(r, ref, nil); ran {
		t.Fatal("a busy registry with no nestedRunner must report ran=false")
	}
	// Via InvokeCallback: it falls through to CallBoru over the body.
	sig := &core.Signature{Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(7)}, ref)}
	out, err := core.InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback should have fallen back to CallBoru, got err: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("result = %v, want 7 from the interpreter fallback", out[0])
	}
}

// TestUserPolyNoMatchAltCoverageScreen pins the subset-coverage screen: the
// alt rides the user-poly no-match defer only when the recorded arm subset
// covers EVERY non-fallback overload of the live table — an uncovered
// same-arity arm could match at run time where the raise claims failure.
func TestUserPolyNoMatchAltCoverageScreen(t *testing.T) {
	r := covRegistry(t, nil)
	core.InstallFnDef(r, "upnm", core.FnDefInfo{
		Signatures: []core.Signature{
			{Params: []core.FnParam{{Name: "a", Type: core.TInteger}}, Returns: []*core.Type{core.TAny},
				BarrierPos: core.BarrierAllForward, Impl: core.Boru([]core.Value{core.NewWord("a")})},
			{Params: []core.FnParam{{Name: "a", Type: core.TString}}, Returns: []*core.Type{core.TAny},
				BarrierPos: core.BarrierAllForward, Impl: core.Boru([]core.Value{core.NewWord("a")})},
		},
	})
	fd := r.Lookup("upnm")
	if fd == nil || len(fd.Signatures) < 2 {
		t.Fatal("upnm not installed")
	}
	vc := &vmContext{r: r, p: &compiler.Program{Fns: []compiler.CompiledFn{
		{Name: "upnm", NParams: 1}, {Name: "upnm", NParams: 1},
	}}}
	window := []core.Value{core.NewBoolean(true)} // matches neither arm
	full := &compiler.UserPolyRef{Word: "upnm", Arity: 1,
		SigIdx: []int{0, 1}, Units: []int{0, 1},
		Impls: []core.SigImpl{fd.Signatures[0].Impl, fd.Signatures[1].Impl}}
	_, _, err := vc.matchUserPoly(full, window, seam7Dbg, 0)
	ae, ok := err.(*core.BoruError)
	if !ok || ae.DeferAlt == nil || ae.DeferAlt.Code != "signature_error" {
		t.Errorf("a covering subset must ride the alt, got %v", err)
	}
	partial := &compiler.UserPolyRef{Word: "upnm", Arity: 1,
		SigIdx: []int{0}, Units: []int{0},
		Impls: []core.SigImpl{fd.Signatures[0].Impl}}
	_, _, err = vc.matchUserPoly(partial, window, seam7Dbg, 0)
	ae, ok = err.(*core.BoruError)
	if !ok || ae.Code != "internal_error" || ae.DeferAlt != nil {
		t.Errorf("an uncovered live arm must keep the plain defer, got %v", err)
	}
}

// A registry already driving a compiled run rejects a second, overlapping run:
// RunUnit shares runProgram's vmRunning CAS guard, so a concurrent callback on
// the SAME registry (rather than a ForkConcurrent clone) is refused rather than
// racing the shared invoker/scopes.
func TestRunUnitRejectsConcurrentRun(t *testing.T) {
	r := runUnitReg(t)
	p := &compiler.Program{Fns: []compiler.CompiledFn{{Name: "x", NParams: 0, NLocals: 0,
		Code: []compiler.Instr{{Op: compiler.OpRet, Arg: 0}}, Debug: []core.SrcPos{{}}}}}
	r.VmRunning = 1 // simulate an in-flight compiled run on this registry
	_, err := RunUnit(&compiler.CompiledFnRef{Prog: p, Unit: 0}, r, nil)
	var ae *core.BoruError
	if err == nil || !errors.As(err, &ae) || ae.Code != "concurrency_error" {
		t.Fatalf("RunUnit during an active run = %v, want concurrency_error", err)
	}
}

// TestPolyNoMatchRaiseBuildsDiagnostic pins the raise arms: the rebuilt
// written tuple renders in the notes, a FALLBACK sig is skipped by the arity
// screen, and the two-probe reorder cascade fires — the primary probe over
// the written tuple, then the secondary over the stack tuple when the
// primary found nothing (a permuted stack pair behind a single forward
// token).
func TestPolyNoMatchRaiseBuildsDiagnostic(t *testing.T) {
	r := pnmRegistry(t, []core.Signature{pnmSig(-1, core.TInteger, core.TString), {Fallback: true}})
	vc := seam7VC(r)
	fn := r.Lookup("pnmw")
	window := []core.Value{core.NewBoolean(true), core.NewString("s")}
	spec := &core.PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 2, Pos: core.SrcPos{Row: 3, Col: 7}}
	err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, fn, window, seam7Dbg, 0)
	if err == nil {
		t.Fatal("a valid spec must raise")
	}
	ae, ok := err.(*core.BoruError)
	if !ok || ae.Code != "signature_error" {
		t.Fatalf("raise = %v, want signature_error", err)
	}
	if ae.Row != 3 || ae.Col != 7 {
		t.Errorf("raise anchored at %d:%d, want the spec's 3:7", ae.Row, ae.Col)
	}
	if msg := err.Error(); !strings.Contains(msg, "the arguments were true (a Boolean) and 's' (a ProperString)") {
		t.Errorf("the rebuilt written tuple must render in the notes:\n%v", msg)
	}

	// Primary reorder probe: the written tuple permutes the declared order.
	permuted := []core.Value{core.NewString("s"), core.NewInteger(1)}
	err = vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, fn, permuted, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "did you swap the arguments?") {
		t.Errorf("the primary reorder probe must fire over the written tuple:\n%v", err)
	}

	// Secondary probe: the written tuple is ONE unclaimed forward token (a
	// 1-tuple no 2-arg sig probes), the stack tuple permutes the declared
	// pair — sigError's fallback probe, rebuilt from the same window.
	sec := &core.PolyNoMatchSpec{Written: []int{0}, StackTuple: []int{0, 1}, NSigs: 2, Pos: core.SrcPos{Row: 1, Col: 1}}
	err = vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: sec}, fn, permuted, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "did you swap the arguments?") {
		t.Errorf("the secondary reorder probe must fire over the stack tuple:\n%v", err)
	}
}

// RunProgram must refuse to start a compiled run while an INTERPRETER run is
// already in flight on the same registry — the cross-engine race the vmRunning
// CAS alone cannot see (it only guards compiled-vs-compiled). The interpRunDepth
// counter Engine.Run maintains is what makes this detectable. Paired
// positive/negative: refused while a run is active, allowed once it ends.
func TestRunProgramRejectsConcurrentInterpreterRun(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	p := &compiler.Program{} // empty program: runs off the end, returns an empty residual

	// NEGATIVE — an interpreter run is in flight (depth > 0): refuse with a
	// concurrency_error, and the vmRunning flag must be released on that return
	// (so the positive case below can still acquire it).
	r.EnterInterpRun()
	if _, err := RunProgram(p, r); err == nil {
		r.ExitInterpRun()
		t.Fatal("RunProgram succeeded while an interpreter run was active; want concurrency_error")
	} else {
		var ae *core.BoruError
		if !errors.As(err, &ae) || ae.Code != "concurrency_error" {
			r.ExitInterpRun()
			t.Fatalf("wrong error while interpreter active: got %v, want concurrency_error", err)
		}
	}
	r.ExitInterpRun()

	// POSITIVE — no interpreter run in flight: the compiled run proceeds (so the
	// negative case is not vacuously "always refuses").
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("RunProgram refused on an idle registry: %v", err)
	}
}

func TestCompiledUserPolyReMatch(t *testing.T) {
	// A dynamic operand to a 2-overload user fn with identical returns:
	// every arm bakes and the VM re-matches at entry (OpCallUserPoly).
	runTolerant(t, registerUserPoly2, func() []core.Value {
		return []core.Value{
			core.NewWord("upoly2"),
			core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
		}
	}, "8")
}
