package core

// Stage-5b coverage: engine.go blocks with start line >= 5000 —
// execFnDefLiteral tails, ExecFnDefSigStackMatch check-value arms,
// execFnDefSig cross-registry analysis, forward-scan helpers
// (dispatchModAt, tagReachCollapsedFn, expandScanSugar,
// reachCallHeadBarrier, ReachFnWouldClaim), policyGateWord,
// strandedForwardError, unwindFrameTailOnError, mark/move + flow-ctrl
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

// s5bEmit is an EmitRecorder stub: TheInactiveEmit with the handful of
// methods the stage-5b paths consult overridden and tallied.
type s5bEmit struct {
	EmitRecorder
	activeOn         bool
	memberRead       bool
	dynMethodOK      bool
	dynApplyOK       bool
	dynApplyConsumed int
	leadEligible     bool
	trapOK           bool
	rematchOK        bool

	uncompilable []string
	trailing     []string
	dynMethods   int
	dynApplies   int
	trapErrs     []*BoruError
	rematches    int
	// pending is the one value id the stub reports as an `apply`-word
	// application in flight (ApplyPending); lastOut is the out carrier the
	// last RecordDynApply received.
	pending string
	lastOut Value
}

func newS5BEmit() *s5bEmit { return &s5bEmit{EmitRecorder: TheInactiveEmit, activeOn: true} }

func (s *s5bEmit) Active() bool                   { return s.activeOn }
func (s *s5bEmit) MarkUncompilable(reason string) { s.uncompilable = append(s.uncompilable, reason) }
func (s *s5bEmit) MemberFnRead(string) bool       { return s.memberRead }
func (s *s5bEmit) RecordDynMethod(fn Value, args, outs []Value, word string, pos SrcPos) bool {
	s.dynMethods++
	return s.dynMethodOK
}
func (s *s5bEmit) ApplyPending(id string) bool { return id != "" && id == s.pending }
func (s *s5bEmit) RecordDynApply(args []Value, fn, out Value, pos SrcPos) (int, bool) {
	s.dynApplies++
	s.lastOut = out
	if s.dynApplyConsumed > 0 {
		return s.dynApplyConsumed, s.dynApplyOK
	}
	return len(args), s.dynApplyOK
}
func (s *s5bEmit) DynApplyLeadEligible(Value) bool { return s.leadEligible }
func (s *s5bEmit) RegisterTrailingApply(id string, arity int) {
	s.trailing = append(s.trailing, id)
}
func (s *s5bEmit) RecordTrapErr(ae *BoruError, pos SrcPos) bool {
	s.trapErrs = append(s.trapErrs, ae)
	return s.trapOK
}
func (s *s5bEmit) RecordDispatchRematchValues(word string, vals []Value, off, n int, pos SrcPos) bool {
	s.rematches++
	return s.rematchOK
}

func installS5BEmit(t *testing.T, r *Registry, es EmitRecorder) {
	t.Helper()
	prev := r.Check.Emit
	r.Check.Emit = es
	t.Cleanup(func() { r.Check.Emit = prev })
}

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
	op.ReachGroup = true
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

func TestS5BCheckFnValueSpliceArms(t *testing.T) {
	// A non-anonymous body-bearing fn VALUE dispatched while the emitter
	// is active routes through SpliceFnValueCheckResult: the 0-arg arm
	// (line 5482), the named-params arm (5533), the unnamed-params arm
	// (5571).
	run := func(t *testing.T, fnDef FnDefInfo, tape []Value, valIdx int, resolved []Value) int {
		t.Helper()
		r := covRegistry(t, nil)
		r.Check.Mode = true
		t.Cleanup(func() { r.Check.Mode = false })
		installS5BEmit(t, r, newS5BEmit())
		called := 0
		prev := CheckBraid.SpliceFnValueCheckResult
		CheckBraid.SpliceFnValueCheckResult = func(e *Engine, valIdx, nArgs int, fnDef FnDefInfo, sig *FnSig, args []Value) error {
			called++
			return nil
		}
		t.Cleanup(func() { CheckBraid.SpliceFnValueCheckResult = prev })
		e := NewTop(r)
		e.Tape = NewTape(tape, StackHeadroom)
		e.Pointer = valIdx
		if err := e.ExecFnDefSigStackMatch(valIdx, fnDef, resolved); err != nil {
			t.Fatalf("ExecFnDefSigStackMatch: %v", err)
		}
		return called
	}

	zeroArg := FnDefInfo{Name: "f0", Signatures: []Signature{{
		Impl: Boru([]Value{NewInteger(1)}), BarrierPos: BarrierAllForward,
	}}}
	if got := run(t, zeroArg, []Value{NewFunction(zeroArg)}, 0, nil); got != 1 {
		t.Errorf("0-arg splice arm: called %d times", got)
	}

	named := FnDefInfo{Name: "f1", Signatures: []Signature{{
		Params:     []FnParam{{Name: "n", Type: TInteger}},
		Impl:       Boru(parenBody(NewWord("cadd"), NewWord("n"), NewWord("n"))),
		BarrierPos: BarrierAllForward,
	}}}
	arg := NewInteger(4)
	if got := run(t, named, []Value{arg, NewFunction(named)}, 1, []Value{arg}); got != 1 {
		t.Errorf("named-params splice arm: called %d times", got)
	}

	unnamed := FnDefInfo{Name: "f2", Signatures: []Signature{{
		Params:     []FnParam{{Type: TInteger}},
		Impl:       Boru([]Value{NewInteger(9)}),
		BarrierPos: BarrierAllForward,
	}}}
	if got := run(t, unnamed, []Value{arg, NewFunction(unnamed)}, 1, []Value{arg}); got != 1 {
		t.Errorf("unnamed-params splice arm: called %d times", got)
	}
}

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
	err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{val})
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
	if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{arg}); err != nil {
		t.Fatalf("analysis mode must not raise: %v", err)
	}
	if !e.Tape.At(1).FailedDispatch {
		t.Error("value must be marked FailedDispatch")
	}
	if got := diags(); len(got) != 1 || got[0].Code != "uncalled_function" || got[0].Row != 5 {
		t.Errorf("diagnostic = %+v", got)
	}
}

func TestS5BStackMatchDefiniteTrapMirrors(t *testing.T) {
	// The forty-ninth increment's arm: on a COMPILING pass, over operands the
	// runtime match examines unchanged, the failed named-fn dispatch bakes the
	// interpreter's own error into a terminal trap and the diagnostic becomes a
	// RuntimeMirror — the pipeline compiles past it instead of refusing.
	//
	// The twin below is the same program on a PLAIN check pass, where there is
	// no trap to be exact about and the finding stays model-undermining.
	for _, tc := range []struct {
		name          string
		compiling     bool
		trapOK        bool
		arg           Value
		wantMirror    bool
		wantTrapCalls int
	}{
		{"compiling, definite", true, true, NewString("nope"), true, 1},
		{"the trap declines", true, false, NewString("nope"), false, 1},
		{"a plain check never traps", false, true, NewString("nope"), false, 0},
		{"an inexact operand", true, true, NewDynamicCarrier(TString), false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := covRegistry(t, nil)
			r.Check.Mode = true
			r.Check.Compiling = tc.compiling
			t.Cleanup(func() { r.Check.Mode, r.Check.Compiling = false, false })

			prevTop := AnalysisImpl.AtUncaughtTopLevel
			AnalysisImpl.AtUncaughtTopLevel = func(*Registry) bool { return true }
			t.Cleanup(func() { AnalysisImpl.AtUncaughtTopLevel = prevTop })

			es := newS5BEmit()
			es.trapOK = tc.trapOK
			installS5BEmit(t, r, es)

			fnDef := FnDefInfo{Name: "fw", Signatures: []Signature{{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Impl:       Boru([]Value{NewInteger(1)}),
				BarrierPos: BarrierAllForward,
			}}}
			arg := tc.arg
			arg.SetPos(SrcPos{Row: 5, Col: 2})
			e := NewTop(r)
			e.Tape = NewTape([]Value{arg, NewFunction(fnDef)}, StackHeadroom)
			e.Pointer = 1
			if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{arg}); err != nil {
				t.Fatalf("analysis mode must not raise: %v", err)
			}
			got := r.Check.Diagnostics
			if len(got) != 1 || got[0].Code != "uncalled_function" {
				t.Fatalf("diagnostic = %+v", got)
			}
			if got[0].RuntimeMirror != tc.wantMirror {
				t.Errorf("RuntimeMirror = %v, want %v", got[0].RuntimeMirror, tc.wantMirror)
			}
			if len(es.trapErrs) != tc.wantTrapCalls {
				t.Fatalf("RecordTrapErr calls = %d, want %d", len(es.trapErrs), tc.wantTrapCalls)
			}
			// The trap carries the error the INTERPRETER raises here, hint and
			// all — that identity is the whole licence for the mirror.
			if tc.wantTrapCalls > 0 {
				ae := es.trapErrs[0]
				if ae == nil || ae.Code != "uncalled_function" ||
					!strings.Contains(ae.Detail, "call to 'fw' matched no signature") ||
					!strings.Contains(ae.Hint, "fw/v to push the function as a value") {
					t.Errorf("trapped error = %+v", ae)
				}
			}
		})
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
	if !e.Tape.At(0).ReachGroup {
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
	if w, werr := AsWord(e.Tape.At(0)); werr != nil || w.Name != "cadd" {
		t.Errorf("expansion must splice the bound word, got %v", e.Tape.At(0))
	}
}

func TestS5BReachCallHeadBarrier(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	fd := FnDefInfo{Name: "g", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: 1}}}
	tok := NewFunction(fd)
	tok.ReachGroup = true

	// A Function-conforming viable slot exempts the fn (line 6206).
	fnSlot := []ViableSig{{Sig: &Signature{Args: []*Type{TFunction}}, Barrier: 1}}
	e.Tape = NewTape([]Value{tok, NewInteger(5)}, StackHeadroom)
	if e.reachCallHeadBarrier(tok, fnSlot, 0, 0) {
		t.Error("a Function slot takes the fn as data, no barrier")
	}

	// No exemption: the claim test decides (line 6210).
	if !e.reachCallHeadBarrier(tok, nil, 0, 0) {
		t.Error("a claiming fn must be a call-head barrier")
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

func TestS5BUnwindFrameTailStopsAtForeignWord(t *testing.T) {
	// The undef-pair replay stops at the first non-undef token
	// (line 6735).
	r := covRegistry(t, nil)
	InstallDef(r, "x", NewInteger(1))
	e := NewTop(r)
	e.Tape = NewTape([]Value{
		NewInteger(0), // marker slot (content irrelevant)
		NewWord("__pa"),
		NewWordModified("undef", -1, false, true),
		NewWord("x"),
		NewWord("cadd"),
		NewWord("cadd"),
	}, StackHeadroom)
	e.unwindFrameTailOnError(DefCleanupInfo{Registry: r, SkipCleanup: true}, 0)
	if _, ok := r.Defs.Top("x"); ok {
		t.Error("the undef pair must uninstall x")
	}
}

// --- mark/move + flow control ----------------------------------------------

func TestS5BStepMoveContCollectsResults(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	id := NextMarkID()
	cont := &ForCont{Registry: r, IterName: "i", Current: 0, End: 1, Step: 1, Body: []Value{NewInteger(9)}}
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
	cont := &ForCont{Registry: r, IterName: "i", Results: []Value{NewInteger(7)}}
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
	cont := &ForCont{Registry: r, IterName: "i"}
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

func TestS5BCloseParenTrailingApplyRecorded(t *testing.T) {
	// The trailing fn-value apply records an event and collapses the
	// window (lines 7471-7497); parenLeadFnApplyIdx declines on an
	// fn-valued last (line 7192).
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.dynApplyOK = true
	installS5BEmit(t, r, es)
	e := NewTop(r)
	fnv := namedFnVal("cmp", []FnParam{{Name: "n", Type: TInteger}}, nil,
		parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(3), fnv, NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if es.dynApplies != 1 {
		t.Errorf("RecordDynApply calls = %d", es.dynApplies)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("window must collapse to the out carrier, tape len %d", e.Tape.Len())
	}
}

func TestS5BCloseParenTrailingApplyDeclined(t *testing.T) {
	// A declined trailing apply registers the residual instead
	// (line 7498).
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.dynApplyOK = false
	installS5BEmit(t, r, es)
	e := NewTop(r)
	fnv := namedFnVal("cmp", []FnParam{{Name: "n", Type: TInteger}}, nil,
		parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(3), fnv, NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if len(es.trailing) != 1 {
		t.Errorf("RegisterTrailingApply calls = %d", len(es.trailing))
	}
}

func TestS5BCloseParenLeadFnApply(t *testing.T) {
	// A leading one-arg fn-carrier apply is classified (lines 7195-7199)
	// and recorded (lines 7216-7226, 7501-7506).
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.dynApplyOK = true
	es.leadEligible = true
	installS5BEmit(t, r, es)
	e := NewTop(r)
	lead := NewCarrier(TFunction)
	e.Tape = NewTape([]Value{NewOpenParen(), lead, NewInteger(5), NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if es.dynApplies != 1 {
		t.Errorf("RecordDynApply calls = %d", es.dynApplies)
	}
	if e.Tape.Len() != 1 {
		t.Errorf("lead apply must collapse the window, tape len %d", e.Tape.Len())
	}
}

func TestS5BCloseParenLeadingDynamicApply(t *testing.T) {
	// A leading DYNAMIC value with a member-read provenance records a
	// dyn-method event (lines 7146-7166, 7507-7510); the lead-fn probe
	// breaks on the dynamic first value (line 7201).
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.memberRead = true
	es.dynMethodOK = true
	installS5BEmit(t, r, es)
	e := NewTop(r)
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	e.Tape = NewTape([]Value{NewOpenParen(), dyn, NewInteger(7), NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if es.dynMethods != 1 {
		t.Errorf("RecordDynMethod calls = %d", es.dynMethods)
	}
	if e.Tape.Len() != 1 {
		t.Errorf("dyn apply must collapse the window, tape len %d", e.Tape.Len())
	}
}

func TestS5BCloseParenPendingGradualLead(t *testing.T) {
	// A DYNAMIC last value the apply word holds pending (`(x r apply)` over
	// a gradual r, the twenty-seventh increment) is the paren's TRAILING
	// apply, recorded through RecordDynApply with a GRADUAL out carrier
	// (the lead's result is unknown), and the window collapses to it.
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.dynApplyOK = true
	lead := NewCarrier(TAny)
	lead.Dynamic = true
	lead.ID = "lead-id"
	es.pending = lead.ID
	installS5BEmit(t, r, es)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(7), lead, NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if es.dynApplies != 1 || !es.lastOut.Dynamic || !es.lastOut.Carrier {
		t.Errorf("the pending gradual lead records a trailing apply with a gradual out: calls=%d out=%+v", es.dynApplies, es.lastOut)
	}
	if e.Tape.Len() != 1 {
		t.Errorf("the window collapses to the out carrier, tape len %d", e.Tape.Len())
	}
	// The same window with NOTHING pending is no trailing apply: a dynamic
	// last value is not the fn it might be, and the leading value is data,
	// so the paren just opens — no record, both values kept.
	es2 := newS5BEmit()
	es2.dynApplyOK = true
	installS5BEmit(t, r, es2)
	e2 := NewTop(r)
	e2.Tape = NewTape([]Value{NewOpenParen(), NewInteger(7), lead, NewCloseParen()}, StackHeadroom)
	e2.Pointer = 3
	if err := e2.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if es2.dynApplies != 0 || e2.Tape.Len() != 2 {
		t.Errorf("no pending apply: no record, window kept: calls=%d len=%d", es2.dynApplies, e2.Tape.Len())
	}
}

func TestS5BCloseParenLeadingDynamicRefused(t *testing.T) {
	// Without member-read provenance the leading-dynamic window refuses
	// (line 7142).
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.memberRead = false
	installS5BEmit(t, r, es)
	e := NewTop(r)
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	e.Tape = NewTape([]Value{NewOpenParen(), dyn, NewInteger(7), NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
	if len(es.uncompilable) != 1 {
		t.Errorf("MarkUncompilable calls = %d", len(es.uncompilable))
	}
}

func TestS5BParenLeadFnApplyIdxArity(t *testing.T) {
	// Only a two-value window is a lead-fn candidate (line 7188).
	r := covRegistry(t, nil)
	es := newS5BEmit()
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(1), NewCloseParen()}, StackHeadroom)
	if got := e.parenLeadFnApplyIdx(es, 0, 2, 1, 1); got != -1 {
		t.Errorf("one-value window must decline, got %d", got)
	}
}

// TestS5BParenLeadFnApplyIdxGradualArgDeclines pins the classifier's
// ARGUMENT gate — the one that keeps the Church-chain family refused
// (design/HIGHER-ORDER-FUNCTIONS.0.md §5.8). Dropping either clause
// compiles `def app f:Function => [x:Any => [(f x)]]` and its whole
// family, which LOOKS like a graduation and is a miscompile waiting on a
// function-valued argument: the interpreter never applies one — its
// leading collection meets a function word, a barrier that never feeds
// forward collection, and raises — where the trailing model the window
// records binds and applies. There is no runtime repair: the raise is a
// property of WORD dispatch, so an island over the resolved window leaves
// both values inert instead of raising, and the two possible texts (the
// stranded-forward barrier, or the lead's own no-match) are selected by
// collection state the window does not carry.
func TestS5BParenLeadFnApplyIdxGradualArgDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.dynApplyOK = true
	es.leadEligible = true
	e := NewTop(r)
	lead := NewCarrier(TFunction)

	// A GRADUAL argument: its runtime value may be a function.
	gradual := NewCarrier(TAny)
	gradual.Dynamic = true
	e.Tape = NewTape([]Value{NewOpenParen(), lead, gradual, NewCloseParen()}, StackHeadroom)
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != -1 {
		t.Errorf("a gradual argument must decline the lead window, got %d", got)
	}

	// A STATICALLY-known fn argument: the divergence is certain.
	e.Tape = NewTape([]Value{NewOpenParen(), lead, NewCarrier(TFunction), NewCloseParen()}, StackHeadroom)
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != -1 {
		t.Errorf("an fn-valued argument must decline the lead window, got %d", got)
	}

	// The admitted shape, for contrast: a concrete non-fn argument.
	e.Tape = NewTape([]Value{NewOpenParen(), lead, NewInteger(5), NewCloseParen()}, StackHeadroom)
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != 1 {
		t.Errorf("a non-fn argument must admit the lead window, got %d", got)
	}
}

// TestS5BParenLeadFnApplyIdxNestedBodyDeclines pins the classifier's
// NESTING gate (§9f). Inside a branch / loop / quotation body the
// compiled body does not carry the bindings the trailing-event model
// needs, so the window must decline there however eligible it looks:
// `def mkg g:Function => [v:Integer => [(g v)]]  def h (mkg …)
// do [(h 1)]` compiled to an island that raised `undefined word: g` —
// the factory's captured param — where the interpreter answers 8.
//
// Pinned HERE, in core's own suite, deliberately: the merged ADR-008
// profile credits this branch from the cmd / lang / test suites, so
// only `make cover-gate-core` — core measured by core alone — catches
// it going unexercised, and that is the gate CI runs.
func TestS5BParenLeadFnApplyIdxNestedBodyDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	es := newS5BEmit()
	es.dynApplyOK = true
	es.leadEligible = true
	e := NewTop(r)
	lead := NewCarrier(TFunction)
	e.Tape = NewTape([]Value{NewOpenParen(), lead, NewInteger(5), NewCloseParen()}, StackHeadroom)

	// At top level the window is admitted (the contrast arm).
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != 1 {
		t.Fatalf("an unnested window must admit, got %d", got)
	}
	// Inside ANY nested body it declines.
	r.Check.NestedBodyDepth = 1
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != -1 {
		t.Errorf("a nested-body window must decline, got %d", got)
	}
	r.Check.NestedBodyDepth = 0
}

func TestS5BCloseParenSkipperHook(t *testing.T) {
	// Surviving paren content tells a RecorderSkipper to ignore the
	// re-visit (lines 7538-7558).
	r := covRegistry(t, nil)
	e := NewTop(r)
	rec := &s5bSkipRec{}
	e.SetRecorder(rec)
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(1), NewCloseParen()}, StackHeadroom)
	e.Pointer = 2
	if err := e.stepCloseParen(true); err != nil {
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
	if err := e.stepCloseParen(true); err != nil {
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
	err := e.stepCloseParen(true)
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
	if err := e.stepCloseParen(true); err != nil {
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
	// A bare type-name word is refused at a concrete-payload slot
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
	fnTok.ReachGroup = true
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
	if w, werr := AsWord(e.Tape.At(1)); werr != nil || w.Name != "af" {
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

// trapFn builds the standard 2-arg no-match dispatch fixture.
func trapFn() *FnDefInfo {
	return &FnDefInfo{Name: "trapw", Signatures: []Signature{
		{Fallback: true},
		{Args: []*Type{TString, TInteger}, BarrierPos: 2},
	}}
}

// trapEngine prepares a compile-armed registry+engine with the given
// tape, pointer, and fallback window.
func trapEngine(t *testing.T, tape []Value, pointer int, window []int) (*Engine, *s5bEmit) {
	t.Helper()
	r := covRegistry(t, nil)
	r.Check.Compiling = true
	t.Cleanup(func() { r.Check.Compiling = false })
	es := newS5BEmit()
	es.trapOK = true
	es.rematchOK = true
	installS5BEmit(t, r, es)
	prevFP := CheckBraid.CheckModeFallbackPositions
	CheckBraid.CheckModeFallbackPositions = func(e *Engine, n int) []int { return window }
	t.Cleanup(func() { CheckBraid.CheckModeFallbackPositions = prevFP })
	e := NewTop(r)
	e.Tape = NewTape(tape, StackHeadroom)
	e.Pointer = pointer
	return e, es
}

func TestS5BTrapInactiveDeclines(t *testing.T) {
	// No active recorder: the trap declines immediately (line 8593).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hd")}, StackHeadroom)
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("an inactive pass must decline")
	}
}

func TestS5BTrapZeroArgSigDeclines(t *testing.T) {
	// A 0-arg real sig is courtesy-dispatchable: not definite
	// (line 8603).
	e, _ := trapEngine(t, []Value{NewWord("hd")}, 0, nil)
	fn := &FnDefInfo{Name: "z", Signatures: []Signature{{Args: []*Type{}, BarrierPos: 0}}}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "z"}, fn, SrcPos{}) {
		t.Error("a 0-arg overload must decline the trap")
	}
}

func TestS5BTrapDefiniteRecordsSigError(t *testing.T) {
	// Concrete forward operands make the failure definite: the
	// interpreter's sigError is recorded as a trap (line 8757). Word
	// operands resolve exactly as the match saw them (lines 8639-8653).
	e, es := trapEngine(t, []Value{
		NewWord("hd"), NewWord("bnd"), NewWord("true"), NewWord("false"), NewWord("none"),
	}, 0, []int{1, 2, 3, 4})
	InstallDef(e.Registry, "bnd", NewInteger(7))
	fn := &FnDefInfo{Name: "trapw", Signatures: []Signature{
		{Fallback: true},
		{Args: []*Type{TString, TInteger, TString, TInteger}, BarrierPos: 4},
	}}
	if !e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, fn, SrcPos{Row: 1, Col: 1}) {
		t.Fatal("a definite failure must record the trap")
	}
	if len(es.trapErrs) != 1 || es.trapErrs[0].Code != "signature_error" {
		t.Errorf("trap errors = %+v", es.trapErrs)
	}
}

func TestS5BTrapVoidGroupDefinite(t *testing.T) {
	// The void-argument-group override rides the definite trap
	// (line 8754).
	e, es := trapEngine(t, []Value{NewWord("hd"), NewString("a"), NewInteger(1)}, 0, []int{1, 2})
	e.voidGroups = []string{"trapw"}
	if !e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{Row: 1}) {
		t.Fatal("the void-group trap must record")
	}
	if len(es.trapErrs) != 1 || es.trapErrs[0].Code != "no_value_error" {
		t.Errorf("trap errors = %+v", es.trapErrs)
	}
}

func TestS5BTrapOpenParenDeclines(t *testing.T) {
	// An unevaluated paren group inside the window is never definite
	// (line 8631).
	e, _ := trapEngine(t, []Value{NewWord("hd"), NewOpenParen()}, 0, []int{1})
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("a paren in the window must decline")
	}
}

func TestS5BTrapUndefinedOperandDeclines(t *testing.T) {
	// An undefined-word placeholder declines (line 8659).
	und := NewInteger(3)
	und.Undefined = true
	e, _ := trapEngine(t, []Value{NewWord("hd"), und}, 0, []int{1})
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("an undefined operand must decline")
	}
}

func TestS5BTrapDeferredExprDeclines(t *testing.T) {
	// A deferred-expression token (ParenExpr) declines (line 8668).
	pe := NewParenExpr([]Value{NewInteger(1)})
	e, _ := trapEngine(t, []Value{NewWord("hd"), pe}, 0, []int{1})
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("a deferred expression must decline")
	}
}

func TestS5BTrapCarrierRematchRecords(t *testing.T) {
	// A carrier window whose written tuple is a contiguous slice records
	// the runtime rematch (lines 8697-8747).
	carrier := NewCarrier(TInteger)
	e, es := trapEngine(t, []Value{NewWord("hd"), carrier, NewInteger(5)}, 0, []int{1, 2})
	if !e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{Row: 1}) {
		t.Fatal("the carrier rematch must record")
	}
	if es.rematches != 1 {
		t.Errorf("RecordDispatchRematchValues calls = %d", es.rematches)
	}
}

func TestS5BTrapCarrierWrittenTooWide(t *testing.T) {
	// A written tuple wider than the window cannot be rebuilt
	// (line 8722).
	carrier := NewCarrier(TInteger)
	e, _ := trapEngine(t, []Value{NewWord("hd"), carrier, NewInteger(5)}, 0, []int{1})
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("a too-wide written tuple must decline")
	}
}

func TestS5BTrapCarrierWrittenOffsetMiss(t *testing.T) {
	// A written tuple absent from the window (by ID) declines
	// (line 8738).
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	t.Cleanup(done)
	r.Check.Compiling = true
	es := newS5BEmit()
	es.rematchOK = true
	installS5BEmit(t, r, es)
	window := []int{0}
	prevFP := CheckBraid.CheckModeFallbackPositions
	CheckBraid.CheckModeFallbackPositions = func(e *Engine, n int) []int { return window }
	t.Cleanup(func() { CheckBraid.CheckModeFallbackPositions = prevFP })

	carrierA := NewCarrier(TInteger)
	carrierB := NewCarrier(TInteger)
	if carrierA.ID == carrierB.ID {
		t.Fatal("pass-armed carriers must carry distinct IDs")
	}
	e := NewTop(r)
	e.Tape = NewTape([]Value{carrierA, NewWord("hd"), carrierB}, StackHeadroom)
	e.Pointer = 1
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("an offset miss must decline")
	}
}

func TestS5BTrapCarrierVoidGroupDeclines(t *testing.T) {
	// The tape-state diagnostic layers gate the rematch: a void-group
	// record declines (line 8741).
	carrier := NewCarrier(TInteger)
	e, _ := trapEngine(t, []Value{NewWord("hd"), carrier, NewInteger(5)}, 0, []int{1, 2})
	e.voidGroups = []string{"trapw"}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("a void-group record must decline the rematch")
	}
}

func TestS5BTrapCarrierReorderHintDeclines(t *testing.T) {
	// The stack reorder probe declines the rematch (line 8744).
	carrier := NewCarrier(TInteger)
	e, _ := trapEngine(t, []Value{
		NewString("a"), NewInteger(1), NewWord("hd"), carrier, NewInteger(5),
	}, 2, []int{3, 4})
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("a reorder-explained failure must decline the rematch")
	}
}

func TestS5BArgTypeSummaryPlainValue(t *testing.T) {
	// The default arm renders the plain Parent leaf (line 8774).
	if got := ArgTypeSummary([]Value{NewInteger(1)}); got != "Integer" {
		t.Errorf("ArgTypeSummary = %q", got)
	}
}
