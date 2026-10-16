package core

// Stage-5b coverage: engine.go blocks with start line >= 5000 —
// execFnDefLiteral tails, ExecFnDefSigStackMatch check-value arms,
// execFnDefSig cross-registry analysis, forward-scan helpers
// (dispatchModAt, tagReachCollapsedFn, expandScanSugar,
// reachCallHeadBarrier, ReachFnWouldClaim), policyGateWord,
// strandedForwardError, mark/move + flow-ctrl
// resolvers, stepCloseParen's check-mode fn-value boundary
// (recordParenLeadingApply / parenLeadFnApplyIdx /
// recordParenLeadFnApply), the RecorderSkipper hook,
// EffectiveResolved scratch reuse, MatchSignature arms, and the
// compile-mode dispatch-trap recorder (TryRecordUnmatchedDispatchTrap,
// TryRecordRecoveredUserFn).

import (
	"errors"
	"strings"
	"testing"
)

// s5bSkipRec implements Recorder + RecorderSkipper.
type s5bSkipRec struct{ lits, calls, skipped int }

func (s *s5bSkipRec) OnPushLit(Value)         { s.lits++ }
func (s *s5bSkipRec) OnCall(string, int, int) { s.calls++ }
func (s *s5bSkipRec) Skip(n int)              { s.skipped += n }

// s5bWordPolicy is a WordChecker stub for policyGateWord.
type s5bWordPolicy struct{ err error }

func (p s5bWordPolicy) CheckWord(string) error { return p.err }

// registerCraise registers a 0-arg native that always raises.
func registerCraise(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "craise",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, errors.New("craise boom")
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
}

// --- execFnDefLiteral tails -------------------------------------------------

func TestS5BReachGroupDeferral(t *testing.T) {
	// A named arg-taking fn value alone inside a REACH-LOWERED group
	// defers dispatch: the pointer simply advances (line 5052).
	r := covRegistry(t, nil)
	e := NewTop(r)
	op := NewOpenParen()
	SetReachGroup(&op, true)
	fnv := namedFnVal("nm",
		[]FnParam{{Name: "n", Type: TInteger}}, []*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))
	e.Tape = NewTape([]Value{op, fnv, NewCloseParen()}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefLiteral(1); err != nil {
		t.Fatalf("execFnDefLiteral: %v", err)
	}
	if e.Pointer != 2 {
		t.Errorf("deferral must advance the pointer, got %d", e.Pointer)
	}
}

func TestS5BFnValueForwardArgsError(t *testing.T) {
	// resolveForwardArgs propagates a paren-group evaluation error
	// (line 5137).
	r := covRegistry(t, registerCraise)
	e := NewTop(r)
	fnv := anonFnVal(
		[]FnParam{{Name: "n", Type: TInteger}}, []*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))
	e.Tape = NewTape([]Value{fnv, NewOpenParen(), NewWord("craise"), NewCloseParen()}, StackHeadroom)
	e.Pointer = 0
	err := e.execFnDefLiteral(0)
	if err == nil || !strings.Contains(err.Error(), "craise boom") {
		t.Fatalf("want the paren-group error, got %v", err)
	}
}

func TestS5BFnValueFallbackSigIgnored(t *testing.T) {
	// A registry-installed fn reached as a bare VALUE with no args:
	// matchSignature answers with the synthetic Fallback sig, which the
	// fn-value path discards (line 5161) — the value stays data.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewFunction(FnDefInfo{Name: "ctwice"})}, StackHeadroom)
	e.Pointer = 0
	if err := e.execFnDefLiteral(0); err != nil {
		t.Fatalf("execFnDefLiteral: %v", err)
	}
	if e.Pointer != 1 {
		t.Errorf("fallback-matched value must stay data, pointer got %d", e.Pointer)
	}
}

func TestS5BForeignWrapperSigSelection(t *testing.T) {
	// The module-wrapper own-sig pairing loop: a body-less native-ref sig
	// is skipped (line 5270), a wrong-arity body-bearing sig is skipped
	// (line 5276), and a Module + trace combination stamps the traceNote
	// (line 5296).
	sub := covRegistry(t, nil)
	mainSig := Signature{
		Params:     []FnParam{{Name: "n", Type: TInteger}},
		Returns:    []*Type{TInteger},
		Impl:       Boru(parenBody(NewWord("cmul"), NewWord("n"), NewInteger(7))),
		BarrierPos: BarrierAllForward,
	}
	InstallFnDef(sub, "t7", FnDefInfo{Signatures: []Signature{mainSig}})

	main := covRegistry(t, nil)
	nativeRef := Signature{
		Args: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}),
		Returns: []*Type{TInteger}, BarrierPos: -1,
	}
	twoParam := Signature{
		Params:     []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		Returns:    []*Type{TInteger},
		Impl:       Boru(parenBody(NewWord("cadd"), NewWord("a"), NewWord("b"))),
		BarrierPos: BarrierAllForward,
	}
	fnv := NewFunction(FnDefInfo{
		Name:       "t7",
		Registry:   sub,
		Module:     "test:mod",
		Signatures: []Signature{nativeRef, twoParam, mainSig},
	})
	e := NewTop(main)
	traced := 0
	e.SetTrace(func(step, pointer int, stack []Value, note string) { traced++ })
	out, err := e.Run([]Value{fnv, NewInteger(6)})
	if err != nil {
		t.Fatalf("foreign wrapper run: %v", err)
	}
	if got := renderAll(out); got != "42" {
		t.Errorf("got %q, want 42", got)
	}
	if traced == 0 {
		t.Error("trace callback never fired")
	}
}

// --- ExecFnDefSigStackMatch check-value arms --------------------------------

func TestS5BStackMatchMapPatternDecline(t *testing.T) {
	// A named-param map Pattern that fails OpenUnifyMap declines the sig
	// (line 5511) and the named-fn no-match error is raised.
	r := covRegistry(t, nil)
	pm := NewOrderedMap()
	pm.Set("k", NewInteger(1))
	pat := NewMap(pm)
	vm := NewOrderedMap()
	vm.Set("k", NewInteger(2))
	val := NewMap(vm)
	fnDef := FnDefInfo{Name: "fm", Signatures: []Signature{{
		Params:     []FnParam{{Name: "m", Type: TMap, Pattern: &pat}},
		Impl:       Boru([]Value{NewInteger(1)}),
		BarrierPos: BarrierAllForward,
	}}}
	e := NewTop(r)
	e.Tape = NewTape([]Value{val, NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	err := e.ExecFnDefSigStackMatch(1, &fnDef, []Value{val})
	if err == nil || !strings.Contains(err.Error(), "matched no signature") {
		t.Fatalf("want uncalled_function error, got %v", err)
	}
}

func TestS5BStackMatchAnalysisWreckage(t *testing.T) {
	// In analysis mode a failed named-fn value dispatch marks the value
	// FailedDispatch (line 5622) and, at the uncaught top level, records
	// an uncalled_function diagnostic (line 5631). The position borrows
	// the nearest argument's span when the value has none (line 5612).
	r := covRegistry(t, nil)
	r.Check.Mode = true
	t.Cleanup(func() { r.Check.Mode = false })

	prevTop := AnalysisImpl.AtUncaughtTopLevel
	AnalysisImpl.AtUncaughtTopLevel = func(*Registry) bool { return true }
	t.Cleanup(func() { AnalysisImpl.AtUncaughtTopLevel = prevTop })
	// The unique-diagnostic dedupe is no longer a slot to stub — it moved
	// into core with the carrier lattice (CheckAddUnique, check_state.go),
	// so the finding lands in the registry's own diagnostic list.
	diags := func() []CheckDiagnostic { return r.Check.Diagnostics }

	fnDef := FnDefInfo{Name: "fw", Signatures: []Signature{{
		Params:     []FnParam{{Name: "n", Type: TInteger}},
		Impl:       Boru([]Value{NewInteger(1)}),
		BarrierPos: BarrierAllForward,
	}}}
	arg := NewString("nope")
	arg.SetPos(SrcPos{Row: 5, Col: 2})
	e := NewTop(r)
	e.Tape = NewTape([]Value{arg, NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, &fnDef, []Value{arg}); err != nil {
		t.Fatalf("analysis mode must not raise: %v", err)
	}
	if !IsFailedDispatch(addrOf(e.Tape.At(1))) {
		t.Error("value must be marked FailedDispatch")
	}
	if got := diags(); len(got) != 1 || got[0].Code != "uncalled_function" || got[0].Row != 5 {
		t.Errorf("diagnostic = %+v", got)
	}
}

func TestS5BExecFnDefSigForeignAnalysisError(t *testing.T) {
	// Cross-registry execFnDefSig in analysis mode takes CallBoruNamed
	// (line 5824) and propagates the body's error (line 5841).
	sub := covRegistry(t, registerCraise)
	main := covRegistry(t, nil)
	main.Check.Mode = true
	t.Cleanup(func() { main.Check.Mode = false })
	sig := &FnSig{
		Params:     []FnParam{{Name: "x", Type: TInteger}},
		Impl:       Boru(parenBody(NewWord("craise"))),
		BarrierPos: BarrierAllForward,
	}
	e := NewTop(main)
	e.Tape = NewTape([]Value{NewInteger(1), NewFunction(FnDefInfo{Name: "xfn", Registry: sub})}, StackHeadroom)
	e.Pointer = 1
	err := e.execFnDefSig(1, sig, []Value{NewInteger(1)}, sub, false)
	if err == nil || !strings.Contains(err.Error(), "craise boom") {
		t.Fatalf("want the body error, got %v", err)
	}
}

// --- forward-scan helpers ---------------------------------------------------

func TestS5BDispatchModAt(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewDispatchMod(DispatchModInfo{Val: true}), NewInteger(1)}, StackHeadroom)
	if !e.dispatchModAt(0) {
		t.Error("DM marker at 0 must report true")
	}
	if e.dispatchModAt(1) {
		t.Error("a plain value is no DM marker")
	}
	if e.dispatchModAt(9) {
		t.Error("out-of-range index must report false")
	}
}

func TestS5BTagReachCollapsedFn(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	fnv := namedFnVal("nm", []FnParam{{Name: "n", Type: TInteger}}, nil,
		parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))
	e.Tape = NewTape([]Value{fnv}, StackHeadroom)
	e.tagReachCollapsedFn(0, 2, true)
	if !IsReachGroup(addrOf(e.Tape.At(0))) {
		t.Error("named fn from a reach group must be tagged")
	}
}

func TestS5BReachFnWouldClaimSkipsBarrierZero(t *testing.T) {
	// The claim loop skips a BarrierPos-0 overload (line 6014) before the
	// forward-eligible one claims the concrete probe.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(5)}, StackHeadroom)
	fd := FnDefInfo{Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 0},
		{Args: []*Type{TInteger}, BarrierPos: 1},
	}}
	if !e.ReachFnWouldClaim(NewFunction(fd), 0) {
		t.Error("the forward-eligible overload must claim the Integer probe")
	}
}

func TestS5BExpandScanSugar(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)

	// No viable consumer: the marker is a boundary (lines 6130, 6160).
	if ok, err := e.expandScanSugar(NewSugar(SugarInfo{Kind: SugarAngle}), 0, 0, nil); ok || err != nil {
		t.Errorf("no-consumer marker: ok=%v err=%v", ok, err)
	}

	// A /q viable slot selects the Angle HEAD form (line 6168); its
	// unavailable head surfaces the user's error (line 6178).
	qsig := &Signature{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}}
	viable := []ViableSig{{Sig: qsig, Barrier: 1}}
	tok := NewSugar(SugarInfo{Kind: SugarAngle, Name: "Box", HeadErr: "bad head"})
	if ok, err := e.expandScanSugar(tok, 0, 0, viable); ok || err == nil || !strings.Contains(err.Error(), "bad head") {
		t.Errorf("head-form failure: ok=%v err=%v", ok, err)
	}

	// A bindable marker expands in place (line 6189).
	r.BindSugarWord(SugarStackArgs, "cadd")
	plain := []ViableSig{{Sig: &Signature{Args: []*Type{TInteger}}, Barrier: 1}}
	e.Tape = NewTape([]Value{NewSugar(SugarInfo{Kind: SugarStackArgs})}, StackHeadroom)
	ok, err := e.expandScanSugar(e.Tape.At(0), 0, 0, plain)
	if !ok || err != nil {
		t.Fatalf("expansion: ok=%v err=%v", ok, err)
	}
	if w, werr := AsWord(e.Tape.Ref(0)); werr != nil || w.Name != "cadd" {
		t.Errorf("expansion must splice the bound word, got %v", e.Tape.At(0))
	}
}

func TestS5BReachCallHeadBarrier(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	fd := FnDefInfo{Name: "g", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	tok := NewFunction(fd)
	SetReachGroup(&tok, true)

	// The claim test decides, whatever slot is open (NUR078 retired the
	// Function-slot exemption): a claiming fn is a call head.
	e.Tape = NewTape([]Value{tok, NewInteger(5)}, StackHeadroom)
	if !e.reachCallHeadBarrier(tok, 0) {
		t.Error("a claiming fn must be a call-head barrier")
	}
	// A /v-quoted fn is data, never a call head.
	quoted := tok
	SetQuoted(&quoted, true)
	if e.reachCallHeadBarrier(quoted, 0) {
		t.Error("a quoted fn is data, no barrier")
	}
}

func TestS5BPolicyGateWord(t *testing.T) {
	r := covRegistry(t, nil)
	denied := errors.New("word denied")
	if err := r.Capabilities.Set(CapPolicy, s5bWordPolicy{err: denied}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	e := NewTop(r)
	if err := e.policyGateWord("cadd"); !errors.Is(err, denied) {
		t.Errorf("policy gate must consult the checker, got %v", err)
	}
}

func TestS5BStrandedForwardBarrierReceiverNote(t *testing.T) {
	// A boundary word with a stack-barrier slot appends the sequential
	// spelling note (line 6528).
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "recvw",
			Signatures: []Signature{{
				Args: []*Type{TInteger, TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{args[0]}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: 1,
			}},
		})
	})
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "cadd", ExpectedArgs: 2, CollectedArgs: 1})
	e.Tape = NewTape([]Value{fwd}, StackHeadroom)
	e.Pointer = 1
	err := e.strandedForwardError("recvw")
	if err == nil || !strings.Contains(err.Error(), "seals off") {
		t.Fatalf("want the barrier-receiver note, got %v", err)
	}
}

// --- mark/move + flow control ----------------------------------------------

func TestS5BStepMoveContCollectsResults(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	id := NextMarkID()
	cont := &Loop{Registry: r, IterName: "i", Current: 0, End: 1, Step: 1, Body: []Value{NewInteger(9)}}
	move := NewMoveCont(id, "for", cont)
	e.Tape = NewTape([]Value{NewMark(id, NewInteger(9)), NewInteger(9), move}, StackHeadroom)
	e.Pointer = 2
	e.marks = map[string]bool{id: true}
	if err := e.stepMove(move); err != nil {
		t.Fatalf("stepMove: %v", err)
	}
	if len(cont.Results) != 1 {
		t.Errorf("iteration output must be collected, got %d", len(cont.Results))
	}
	if got := renderAll(e.Tape.Snapshot()); got != "9" {
		t.Errorf("final tape = %q", got)
	}
}

func TestS5BStepMoveIfNoConditionValue(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	id := NextMarkID()
	move := NewMoveIf(id, "if", &IfCont{Then: []Value{NewInteger(1)}})
	e.Tape = NewTape([]Value{NewMark(id), move}, StackHeadroom)
	e.Pointer = 1
	e.marks = map[string]bool{id: true}
	err := e.stepMove(move)
	if err == nil || !strings.Contains(err.Error(), "condition produced no value") {
		t.Fatalf("want the no-value error, got %v", err)
	}
}

func TestS5BStepMoveIfElseBranch(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	id := NextMarkID()
	move := NewMoveIf(id, "if", &IfCont{Then: []Value{NewInteger(1)}, Else: []Value{NewInteger(2)}})
	e.Tape = NewTape([]Value{NewMark(id), NewBoolean(false), move}, StackHeadroom)
	e.Pointer = 2
	e.marks = map[string]bool{id: true}
	if err := e.stepMove(move); err != nil {
		t.Fatalf("stepMove: %v", err)
	}
	if got := renderAll(e.Tape.Snapshot()); got != "2" {
		t.Errorf("else branch must splice, tape = %q", got)
	}
}

func TestS5BHandleFlowCtrlBreak(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	id := NextMarkID()
	cont := &Loop{Registry: r, IterName: "i", Results: []Value{NewInteger(7)}}
	e.Tape = NewTape([]Value{NewMark(id), NewInteger(5), NewMoveCont(id, "for", cont)}, StackHeadroom)
	e.Pointer = 0
	e.marks = map[string]bool{id: true}
	r.FlowCtrl = FlowBreak
	if !e.handleFlowCtrl() {
		t.Fatal("break inside a loop must be handled")
	}
	if r.FlowCtrl != FlowNone {
		t.Error("handled signal must clear FlowCtrl")
	}
	if got := renderAll(e.Tape.Snapshot()); got != "7" {
		t.Errorf("break must splice the accumulated results, tape = %q", got)
	}
}

func TestS5BHandleFlowCtrlContinue(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	id := NextMarkID()
	cont := &Loop{Registry: r, IterName: "i"}
	e.Tape = NewTape([]Value{NewMark(id), NewInteger(5), NewMoveCont(id, "for", cont)}, StackHeadroom)
	e.Pointer = 0
	e.marks = map[string]bool{id: true}
	r.FlowCtrl = FlowContinue
	if !e.handleFlowCtrl() {
		t.Fatal("continue inside a loop must be handled")
	}
	if r.FlowCtrl != FlowNone {
		t.Error("handled signal must clear FlowCtrl")
	}
	if e.Pointer != 1 || e.Tape.Len() != 2 {
		t.Errorf("partial results must be discarded: ptr=%d len=%d", e.Pointer, e.Tape.Len())
	}
}

func TestS5BExitWithFlowCtrl(t *testing.T) {
	// Top-level: break outside a loop errors (line 6970).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape(nil, StackHeadroom)
	r.FlowCtrl = FlowBreak
	if _, err := e.exitWithFlowCtrl(); err == nil || !strings.Contains(err.Error(), "outside loop") {
		t.Fatalf("want break-outside-loop, got %v", err)
	}

	// VM island: frames unwind and nothing is returned (line 6975).
	r2 := covRegistry(t, nil)
	e2 := NewTop(r2)
	e2.IsTop = false
	e2.FlowUnwind = true
	e2.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)
	out, err := e2.exitWithFlowCtrl()
	if err != nil || out != nil {
		t.Fatalf("island exit must return nothing: out=%v err=%v", out, err)
	}
}

// --- stepCloseParen check-mode fn-value boundary ----------------------------

func TestS5BCloseParenSkipperHook(t *testing.T) {
	// Surviving paren content tells a RecorderSkipper to ignore the
	// re-visit (lines 7538-7558).
	r := covRegistry(t, nil)
	e := NewTop(r)
	rec := &s5bSkipRec{}
	e.SetRecorder(rec)
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(1), NewCloseParen()}, StackHeadroom)
	e.Pointer = 2
	if err := e.stepCloseParen(true, false); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if rec.skipped != 1 {
		t.Errorf("Skip total = %d, want 1", rec.skipped)
	}
}

func TestS5BCloseParenVoidGroupStopsAtOpenParen(t *testing.T) {
	// The void-group consumer scan stops at an enclosing OpenParen
	// (line 7363).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewOpenParen(), NewOpenParen(), NewCloseParen()}, StackHeadroom)
	e.Pointer = 2
	if err := e.stepCloseParen(true, false); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if len(e.voidGroups) != 0 {
		t.Errorf("no consumer must be recorded past the paren, got %v", e.voidGroups)
	}
}

func TestS5BCloseParenReturnCountOverflow(t *testing.T) {
	// Extra results beyond the unnamed-arg allowance raise the return
	// count error (line 7410).
	r := covRegistry(t, nil)
	e := NewTop(r)
	rc := NewReturnCheck(ReturnCheckInfo{FuncName: "f", Returns: []*Type{TInteger}})
	e.Tape = NewTape([]Value{NewOpenParen(), rc, NewInteger(5), NewInteger(7), NewCloseParen()}, StackHeadroom)
	e.Pointer = 4
	err := e.stepCloseParen(true, false)
	if err == nil || !strings.Contains(err.Error(), "return") {
		t.Fatalf("want a return-count error, got %v", err)
	}
}

func TestS5BCloseParenDiscardsUnnamedArgs(t *testing.T) {
	// Unconsumed unnamed args at the bottom of the scope are discarded
	// (line 7427).
	r := covRegistry(t, nil)
	e := NewTop(r)
	rc := NewReturnCheck(ReturnCheckInfo{FuncName: "f", Returns: []*Type{TInteger}, UnnamedCount: 1})
	e.Tape = NewTape([]Value{NewOpenParen(), rc, NewInteger(5), NewInteger(7), NewCloseParen()}, StackHeadroom)
	e.Pointer = 4
	if err := e.stepCloseParen(true, false); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if got := renderAll(e.Tape.Snapshot()); got != "7" {
		t.Errorf("unnamed arg must be discarded, tape = %q", got)
	}
}

func TestS5BEffectiveResolvedScratchReuse(t *testing.T) {
	// The second call reuses the exclusion scratch map (line 7604).
	r := covRegistry(t, nil)
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0, CollectedArgs: 0, StackArgs: 0})
	e.Tape = NewTape([]Value{fwd, NewInteger(1)}, StackHeadroom)
	e.Pointer = 2
	first := len(e.EffectiveResolved())
	second := len(e.EffectiveResolved())
	if first != 1 || second != 1 {
		t.Errorf("resolved lens = %d, %d; want 1, 1", first, second)
	}
}

// --- MatchSignature arms ----------------------------------------------------

func TestS5BMatchSignatureMixedCarrierSplit(t *testing.T) {
	// A mixed gradual carrier rejected by the specific overload
	// (line 8371) followed by a forward-collecting selection flags the
	// ambiguous split (lines 7911, 7916).
	r := covRegistry(t, nil)
	r.Check.Mode = true
	t.Cleanup(func() { r.Check.Mode = false })
	prevMixed := AnalysisImpl.MixedConform
	AnalysisImpl.MixedConform = func(Value, *Type) bool { return true }
	t.Cleanup(func() { AnalysisImpl.MixedConform = prevMixed })

	carrier := NewCarrier(TBoolean)
	e := NewTop(r)
	e.Tape = NewTape([]Value{carrier, NewWord("hd"), NewString("s")}, StackHeadroom)
	e.Pointer = 1
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 0},
		{Args: []*Type{TString}, BarrierPos: 1},
	}}
	sig, positions, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, []Value{carrier})
	if sig == nil || len(positions) != 1 || positions[0] != 2 {
		t.Fatalf("forward overload must be selected, sig=%v positions=%v", sig, positions)
	}
	if !r.Check.AmbiguousGradualSplit {
		t.Error("the ambiguous gradual split must be flagged")
	}
}

func TestS5BMatchSignatureArgCountFilter(t *testing.T) {
	// A /N arity filter skips mismatched sigs in the main loop
	// (line 7974) and the fallback loop (line 8414).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hd")}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: 2}, nil); sig != nil {
		t.Errorf("arity filter must reject, got %v", sig)
	}
}

func TestS5BMatchSignatureFormArgs(t *testing.T) {
	// A FormArgs slot captures any raw form (line 8042).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("rawop")}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "m", Signatures: []Signature{{
		Args: []*Type{TAny}, FormArgs: map[int]bool{0: true}, BarrierPos: 1,
	}}}
	sig, positions, _ := e.MatchSignature(fn, WordInfo{Name: "m", ArgCount: -1}, nil)
	if sig == nil || len(positions) != 1 || positions[0] != 1 {
		t.Fatalf("form slot must capture the word, sig=%v positions=%v", sig, positions)
	}
}

func TestS5BMatchSignatureSimpleDefMismatch(t *testing.T) {
	// A simple (non-fn) def binding that misses the slot type is a
	// boundary (line 8099).
	r := covRegistry(t, nil)
	InstallDef(r, "dw", NewString("s"))
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("dw")}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("mismatched def binding must not match, got %v", sig)
	}
}

func TestS5BMatchSignatureNamedTypeMismatch(t *testing.T) {
	// A predicate-type binding (an FnDefInfo body under a type def)
	// consults TopTypeBody (line 8110) and breaks when the type value
	// does not fit the slot (line 8117).
	r := covRegistry(t, nil)
	r.Defs.PushType("PredT", TInteger, NewFunction(FnDefInfo{Name: "pred"}))
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("PredT")}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("type-name value must not fit an Integer slot, got %v", sig)
	}
}

func TestS5BMatchSignatureBooleanLiterals(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)

	// `true` matches a Boolean slot (line 8137).
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("true")}, StackHeadroom)
	e.Pointer = 0
	boolFn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TBoolean}, BarrierPos: 1}}}
	sig, positions, _ := e.MatchSignature(boolFn, WordInfo{Name: "w", ArgCount: -1}, nil)
	if sig == nil || len(positions) != 1 {
		t.Fatalf("true must fill a Boolean slot, sig=%v", sig)
	}

	// `false` at an Integer slot is a boundary (line 8143).
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("false")}, StackHeadroom)
	e.Pointer = 0
	intFn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(intFn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("false must not fill an Integer slot, got %v", sig)
	}
}

func TestS5BMatchSignatureTypeNameRejected(t *testing.T) {
	// A bare type-name word is declined at a concrete-payload slot
	// (line 8153).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("Integer")}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("type literal must be rejected at a concrete slot, got %v", sig)
	}
}

func TestS5BMatchSignatureUndefinedWordAtom(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)

	// An undefined word resolves to Atom and fills an Atom slot
	// (line 8165).
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("zzz")}, StackHeadroom)
	e.Pointer = 0
	atomFn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TAtom}, BarrierPos: 1}}}
	sig, positions, _ := e.MatchSignature(atomFn, WordInfo{Name: "w", ArgCount: -1}, nil)
	if sig == nil || len(positions) != 1 {
		t.Fatalf("undefined word must fill an Atom slot, sig=%v", sig)
	}

	// The same word at an Integer slot is a boundary (line 8171).
	e.Tape = NewTape([]Value{NewWord("hd"), NewWord("zzz")}, StackHeadroom)
	e.Pointer = 0
	intFn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(intFn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("undefined word must not fill an Integer slot, got %v", sig)
	}
}

func TestS5BMatchSignatureReachFnBoundary(t *testing.T) {
	// A reach-collapsed named fn that would claim its next token stops
	// the forward scan (line 8188).
	r := covRegistry(t, nil)
	e := NewTop(r)
	fnTok := NewFunction(FnDefInfo{Name: "g", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}})
	SetReachGroup(&fnTok, true)
	e.Tape = NewTape([]Value{NewWord("hd"), fnTok, NewInteger(5)}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("call-head fn must stop the scan, got %v", sig)
	}
}

func TestS5BMatchSignatureQuoteCarrierGuards(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	carrier := NewCarrier(TInteger)

	// Forward /q slot: a non-atom carrier is a boundary (line 8205).
	e.Tape = NewTape([]Value{NewWord("hd"), carrier}, StackHeadroom)
	e.Pointer = 0
	fwdFn := &FnDefInfo{Name: "q", Signatures: []Signature{{
		Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1,
	}}}
	if sig, _, _ := e.MatchSignature(fwdFn, WordInfo{Name: "q", ArgCount: -1}, nil); sig != nil {
		t.Errorf("carrier must not fill a forward /q slot, got %v", sig)
	}

	// Stack /q slot: the same guard (line 8361).
	e.Tape = NewTape([]Value{carrier, NewWord("hd")}, StackHeadroom)
	e.Pointer = 1
	stkFn := &FnDefInfo{Name: "q", Signatures: []Signature{{
		Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 0,
	}}}
	if sig, _, _ := e.MatchSignature(stkFn, WordInfo{Name: "q", ArgCount: -1}, []Value{carrier}); sig != nil {
		t.Errorf("carrier must not fill a stack /q slot, got %v", sig)
	}
}

func TestS5BMatchSignatureSugarArms(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)

	// An Angle marker is a plain boundary in the per-sig scan
	// (line 8226).
	e.Tape = NewTape([]Value{NewWord("hd"), NewSugar(SugarInfo{Kind: SugarAngle, Name: "Box"})}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("angle marker must be a boundary, got %v", sig)
	}

	// A bindable marker expands in place mid-scan (line 8233).
	r.BindSugarWord(SugarForceArity, "af")
	e.Tape = NewTape([]Value{NewWord("hd"), NewSugar(SugarInfo{Kind: SugarForceArity, N: 2})}, StackHeadroom)
	e.Pointer = 0
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("expanded marker still misses the Integer slot, got %v", sig)
	}
	if w, werr := AsWord(e.Tape.Ref(1)); werr != nil || w.Name != "af" {
		t.Errorf("the marker must expand on the tape, got %v", e.Tape.At(1))
	}
}

func TestS5BMatchSignatureTypeLiteralArms(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	lit := NewTypeLiteral(TString)

	// Forward literal type-literal rejection (line 8239).
	e.Tape = NewTape([]Value{NewWord("hd"), lit}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TString}, BarrierPos: 1}}}
	if sig, _, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("type literal must be rejected forward, got %v", sig)
	}

	// Stack-phase rejection (line 8378).
	e.Tape = NewTape([]Value{lit, NewWord("hd")}, StackHeadroom)
	e.Pointer = 1
	stkFn := &FnDefInfo{Name: "w", Signatures: []Signature{{Args: []*Type{TString}, BarrierPos: 0}}}
	if sig, _, _ := e.MatchSignature(stkFn, WordInfo{Name: "w", ArgCount: -1}, []Value{lit}); sig != nil {
		t.Errorf("type literal must be rejected on the stack, got %v", sig)
	}
}

func TestS5BMatchSignaturePatternRejections(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)

	// Forward-matched position failing its scalar pattern (line 8261).
	e.Tape = NewTape([]Value{NewWord("hd"), NewInteger(5)}, StackHeadroom)
	e.Pointer = 0
	fwdFn := &FnDefInfo{Name: "w", Signatures: []Signature{{
		Args: []*Type{TInteger}, Patterns: map[int]Value{0: NewInteger(0)}, BarrierPos: 1,
	}}}
	if sig, _, _ := e.MatchSignature(fwdFn, WordInfo{Name: "w", ArgCount: -1}, nil); sig != nil {
		t.Errorf("pattern must reject the forward fill, got %v", sig)
	}

	// Stack-matched position failing the same pattern (line 8392).
	five := NewInteger(5)
	e.Tape = NewTape([]Value{five, NewWord("hd")}, StackHeadroom)
	e.Pointer = 1
	stkFn := &FnDefInfo{Name: "w", Signatures: []Signature{{
		Args: []*Type{TInteger}, Patterns: map[int]Value{0: NewInteger(0)}, BarrierPos: 0,
	}}}
	if sig, _, _ := e.MatchSignature(stkFn, WordInfo{Name: "w", ArgCount: -1}, []Value{five}); sig != nil {
		t.Errorf("pattern must reject the stack fill, got %v", sig)
	}
}

// --- recovery + trap recorders ----------------------------------------------

func TestS5BSingleOverloadRecoverable(t *testing.T) {
	sole := Signature{
		Params: []FnParam{{Name: "x", Type: TInteger}},
		Impl:   Boru([]Value{NewInteger(1)}),
		ReturnsFn: func(args []Value, r *Registry) []Value {
			return []Value{NewInteger(9)}
		},
		BarrierPos: BarrierAllForward,
	}
	fn := &FnDefInfo{Name: "u", Signatures: []Signature{{Fallback: true}, sole}}
	if !SingleOverloadRecoverable(&fn.Signatures[1], fn) {
		t.Fatal("a sole body-bearing overload must be recoverable")
	}

	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(3)}, StackHeadroom)
	e.Pointer = 1
	spliced := 0
	prev := CheckBraid.SpliceCheckResults
	CheckBraid.SpliceCheckResults = func(e *Engine, positions []int, results []Value) { spliced++ }
	t.Cleanup(func() { CheckBraid.SpliceCheckResults = prev })
	if !e.TryRecordRecoveredUserFn(&fn.Signatures[1], fn, []Value{NewInteger(3)}, 0, []int{0}) {
		t.Fatal("the guarded recovery must record")
	}
	if spliced != 1 {
		t.Errorf("SpliceCheckResults calls = %d", spliced)
	}
	if e.TryRecordRecoveredUserFn(&fn.Signatures[1], nil, nil, 0, nil) {
		t.Error("a nil fn must decline")
	}
}

func TestS5BArgTypeSummaryPlainValue(t *testing.T) {
	// The default arm renders the plain Parent leaf (line 8774).
	if got := ArgTypeSummary([]Value{NewInteger(1)}); got != "Integer" {
		t.Errorf("ArgTypeSummary = %q", got)
	}
}

// trapEngine prepares a compile-armed registry+engine with the given
// tape, pointer, and fallback window.
