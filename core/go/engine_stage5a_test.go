package core

// Stage-5a coverage for engine.go (blocks below line 5000): error paths,
// check/compile-mode arms reached through the CheckBraid / AnalysisImpl /
// EmitRecorder seams, the forward pre-scan's prune/boundary arms, and the
// smaller step-loop hooks. White-box, in-package: Engine methods are driven
// directly over hand-built tapes (the engine_seam8_test.go pattern) and via
// Run() where the arm only fires as part of real execution.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// s5aEng builds a top-level engine over the given registry with a
// hand-built tape.
func s5aEng(t *testing.T, r *Registry, vals []Value, pointer int) *Engine {
	t.Helper()
	e := NewTop(r)
	e.Tape = NewTape(vals, StackHeadroom)
	e.Pointer = pointer
	return e
}

// s5aEmit is an EmitRecorder stub: TheInactiveEmit with a few overrides.
type s5aEmit struct {
	EmitRecorder
	active bool
	armed  bool
	fold   []Value
	foldOK bool
}

func (s *s5aEmit) Active() bool { return s.active }
func (s *s5aEmit) Armed() bool  { return s.armed }
func (s *s5aEmit) FoldFullStack(string, []Value, []Value) ([]Value, bool) {
	return s.fold, s.foldOK
}

// s5aPolicy is a hand-rolled WordChecker + ModuleCallChecker.
type s5aPolicy struct{ denyWord, denyModule string }

func (p s5aPolicy) CheckWord(name string) error {
	if name == p.denyWord {
		return fmt.Errorf("s5a policy denies word %s", name)
	}
	return nil
}

func (p s5aPolicy) CheckModuleCall(module, export string) error {
	if module == p.denyModule {
		return fmt.Errorf("s5a policy denies module %s.%s", module, export)
	}
	return nil
}

// s5aCheckOn arms plain check mode on r for the test's duration.
func s5aCheckOn(t *testing.T, r *Registry) {
	t.Helper()
	done := r.Check.Begin()
	t.Cleanup(done)
}

// --- rematchWritten / polyReachBound --------------------------------------

func TestS5ARematchWrittenForwardStops(t *testing.T) {
	// A Word right after the pointer stops the forward walk; the stack
	// prefix (empty) yields nothing.
	e := engWithTape(t, []Value{NewWord("w"), NewWord("x")}, 0)
	if got := e.rematchWritten(); len(got) != 0 {
		t.Errorf("rematchWritten = %v, want empty", got)
	}
	// A non-concrete, non-carrier token (zero Value) stops the walk too.
	e2 := engWithTape(t, []Value{NewWord("w"), {}}, 0)
	if got := e2.rematchWritten(); len(got) != 0 {
		t.Errorf("rematchWritten(zero token) = %v, want empty", got)
	}
}

func TestS5ARematchWrittenStackFallback(t *testing.T) {
	// Nothing after the pointer: the stack prefix is collected top-first.
	e := engWithTape(t, []Value{NewInteger(1), NewInteger(2), NewWord("w")}, 2)
	got := e.rematchWritten()
	if renderAll(got) != "2 | 1" {
		t.Errorf("rematchWritten = %s, want 2 | 1", renderAll(got))
	}
	// The stack walk stops at an OpenParen boundary.
	e2 := engWithTape(t, []Value{NewOpenParen(), NewInteger(3), NewWord("w")}, 2)
	got2 := e2.rematchWritten()
	if renderAll(got2) != "3" {
		t.Errorf("rematchWritten = %s, want 3", renderAll(got2))
	}
}

func TestS5APolyReachBoundUnboundableToken(t *testing.T) {
	// A zero Value in the forward window is neither a boundary, a marker,
	// a word, nor a concrete/carrier value: the bound is untrustworthy.
	e := engWithTape(t, []Value{NewWord("w"), {}}, 0)
	if n, ok := e.polyReachBound(); ok || n != 0 {
		t.Errorf("polyReachBound = (%d,%v), want (0,false)", n, ok)
	}
}

// --- ReorderHintFor structured-sig skip -----------------------------------

func TestS5AReorderHintSkipsPatternSig(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{{
		Args:     []*Type{TInteger, TString},
		Patterns: map[int]Value{0: NewInteger(5)},
	}}}
	written := []Value{NewString("a"), NewInteger(1)}
	if got := ReorderHintFor("w", fn, written); got != "" {
		t.Errorf("ReorderHintFor = %q, want empty (pattern sig skipped)", got)
	}
}

// --- sigError / undefinedWordError void-group paths -----------------------

func TestS5ASigErrorVoidGroup(t *testing.T) {
	e := engWithTape(t, []Value{NewWord("vw")}, 0)
	e.voidGroups = []string{"vw"}
	ae := e.sigError("vw", nil, SrcPos{Row: 1, Col: 1})
	if ae == nil || ae.Code != "no_value_error" {
		t.Fatalf("sigError = %v, want no_value_error", ae)
	}
}

func TestS5AUndefinedWordErrorVoidGroupHint(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.voidGroups = []string{"nm"}
	ae := e.undefinedWordError("nm", SrcPos{})
	if ae == nil || len(ae.Suggestions) == 0 {
		t.Fatalf("undefinedWordError without void-group suggestion: %v", ae)
	}
	if !strings.Contains(ae.Suggestions[0].Message, "produced no value") {
		t.Errorf("suggestion = %q", ae.Suggestions[0].Message)
	}
}

// --- stampResultPos / stampErrPos -----------------------------------------

func TestS5AStampResultPos(t *testing.T) {
	positioned := NewInteger(1)
	positioned.pos = &SrcPos{Row: 9, Col: 1}
	fnVal := NewFunction(FnDefInfo{Name: "f"})
	vals := []Value{positioned, fnVal}
	pos := SrcPos{Row: 3, Col: 2}
	stampResultPos(vals, &pos)
	if vals[0].Pos().Row != 9 {
		t.Errorf("positioned value restamped: %v", vals[0].Pos())
	}
	if vals[1].Pos().Row != 3 {
		t.Errorf("function value not stamped: %v", vals[1].Pos())
	}
}

func TestS5AStampErrPos(t *testing.T) {
	r := covRegistry(t, nil)
	r.BaseFile = "s5a.boru"
	tok := NewInteger(1)
	tok.pos = &SrcPos{Row: 2, Col: 3, Src: "tok"}
	e := NewTop(r)
	e.SetSource("line1\nline2")
	e.Tape = NewTape([]Value{tok}, StackHeadroom)
	e.Pointer = 0

	// Non-BoruError: passed through untouched.
	plain := errors.New("plain")
	if got := e.stampErrPos(plain); got != plain {
		t.Errorf("stampErrPos(plain) = %v", got)
	}

	ae := &BoruError{Code: "x_error", Detail: "d"}
	_ = e.stampErrPos(ae)
	if ae.Row != 2 || ae.Col != 3 {
		t.Errorf("pos not stamped: row=%d col=%d", ae.Row, ae.Col)
	}
	if ae.Src != "tok" {
		t.Errorf("Src = %q, want tok", ae.Src)
	}
	if ae.FullSource == "" {
		t.Error("FullSource not attached")
	}
	if ae.File != "s5a.boru" {
		t.Errorf("File = %q, want s5a.boru", ae.File)
	}
}

// --- stripTapeAscriptions / validateReturnTypes ---------------------------

func TestS5AStripTapeAscriptions(t *testing.T) {
	v := NewInteger(5)
	v.SetAscribed(TInteger)
	e := engWithTape(t, []Value{v}, 0)
	e.stripTapeAscriptions(0, 1)
	if e.Tape.At(0).AscribedType() != nil {
		t.Error("ascription not stripped")
	}
}

func TestS5AValidateReturnTypesDynamicSkip(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	e := s5aEng(t, r, nil, 0)
	rc := ReturnCheckInfo{FuncName: "f", Returns: []*Type{TInteger}}
	got := NewDynamicCarrier(TAny)
	if err := e.validateReturnTypes(rc, []Value{got}, 0); err != nil {
		t.Fatalf("dynamic residual must pass in check mode: %v", err)
	}
}

func TestS5AValidateReturnTypesPatternReject(t *testing.T) {
	e := engWithTape(t, nil, 0)
	pat := NewInteger(5)
	rc := ReturnCheckInfo{FuncName: "f", Returns: []*Type{TAny}, ReturnPatterns: []*Value{&pat}}
	err := e.validateReturnTypes(rc, []Value{NewInteger(7)}, 0)
	if err == nil {
		t.Fatal("pattern mismatch must error")
	}
}

// --- traceSigStr ----------------------------------------------------------

func TestS5ATraceSigStr(t *testing.T) {
	sig := Signature{Args: []*Type{TInteger, TString}}
	got := traceSigStr("w", &sig)
	if !strings.Contains(got, "w(") || !strings.Contains(got, "Integer") {
		t.Errorf("traceSigStr = %q", got)
	}
}

// --- Run: panic guard, check budget, quoted ParenExpr, halt, DefCleanup ---

func TestS5ARunPanicRecovers(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "s5aboom", Signatures: []Signature{{
			Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
				panic("s5a kaboom")
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}}})
	})
	_, err := NewTop(r).Run([]Value{NewWord("s5aboom")})
	ae, ok := err.(*BoruError)
	if !ok || ae.Code != "internal_error" {
		t.Fatalf("Run after panic = %v, want internal_error", err)
	}
}

func TestS5ARunCheckStepBudget(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	r.Check.StepBudget = 0
	out, err := NewTop(r).Run([]Value{NewInteger(1)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = out
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "step_budget_exceeded" {
			found = true
		}
	}
	if !found {
		t.Error("no step_budget_exceeded diagnostic")
	}
}

func TestS5ARunQuotedParenExpr(t *testing.T) {
	r := covRegistry(t, nil)
	pe := NewParenExpr([]Value{NewInteger(1)})
	pe.Quoted = true
	out, err := NewTop(r).Run([]Value{pe})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 1 || !IsParenExpr(out[0]) {
		t.Errorf("residual = %v, want the quoted ParenExpr", out)
	}
}

func TestS5ARunHaltOnUndefinedEntry(t *testing.T) {
	r := covRegistry(t, nil)
	_, err := NewTop(r).Run([]Value{{}})
	ae, ok := err.(*BoruError)
	if !ok || ae.Code != "halt" {
		t.Fatalf("Run(zero value) = %v, want halt", err)
	}
}

func TestS5ARunDefCleanupResidualError(t *testing.T) {
	r := covRegistry(t, nil)
	bad := NewEvalList([]Value{NewWord("s5a-missing")})
	dc := NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true})
	_, err := NewTop(r).Run([]Value{bad, dc})
	if err == nil || !strings.Contains(err.Error(), "s5a-missing") {
		t.Fatalf("Run = %v, want undefined_word for s5a-missing", err)
	}
}

func TestS5ARunFlowBreakInLoop(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "s5abrk", Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				reg.FlowCtrl = FlowBreak
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}}})
	})
	cont := &ForCont{Registry: r, IterName: "s5ai", Results: []Value{NewInteger(42)}}
	prog := []Value{
		NewMark("s5aL"),
		NewWord("s5abrk"),
		NewMoveCont("s5aL", "for loop", cont),
	}
	out, err := NewTop(r).Run(prog)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if renderAll(out) != "42" {
		t.Errorf("residual = %s, want 42", renderAll(out))
	}
}

// --- Run: orphaned gen spec ----------------------------------------------

func TestS5ARunOrphanGenRuntime(t *testing.T) {
	r := covRegistry(t, nil)
	if err := r.SetPendingGen(&GenSpecInfo{}); err != nil {
		t.Fatal(err)
	}
	_, err := NewTop(r).Run(nil)
	ae, ok := err.(*BoruError)
	if !ok || ae.Code != "gen_without_constructor" {
		t.Fatalf("Run = %v, want gen_without_constructor", err)
	}
}

func TestS5ARunOrphanGenCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	if err := r.SetPendingGen(&GenSpecInfo{}); err != nil {
		t.Fatal(err)
	}
	_, err := NewTop(r).Run(nil)
	if err != nil {
		t.Fatalf("check mode must be lenient: %v", err)
	}
	if !r.Check.SuppressedRuntimeError {
		t.Error("SuppressedRuntimeError not latched")
	}
}

// --- rawParenForward / rawFormForward / pendingForwardWantsRawParen -------

func TestS5ARawCaptureProbes(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{{
		Args:      []*Type{TAny},
		RawParens: map[int]bool{0: true},
	}}}
	if !rawParenForward(fn, 0) {
		t.Error("rawParenForward = false, want true")
	}
	fn2 := &FnDefInfo{Signatures: []Signature{{
		Args:     []*Type{TAny},
		FormArgs: map[int]bool{0: true},
	}}}
	if !rawFormForward(fn2, 0) {
		t.Error("rawFormForward = false, want true")
	}
}

func TestS5APendingForwardWantsRawParen(t *testing.T) {
	sig := &Signature{Args: []*Type{TAny}, RawParens: map[int]bool{0: true}, BarrierPos: 1}
	fwd := NewForward(ForwardInfo{FuncName: "w", Sig: sig, ExpectedArgs: 1})
	e := engWithTape(t, []Value{fwd, NewInteger(1)}, 1)
	if !e.pendingForwardWantsRawParen() {
		t.Error("pendingForwardWantsRawParen = false, want true")
	}
}

// --- keyword-slot probes --------------------------------------------------

func TestS5ASigsHaveKeywordSlot(t *testing.T) {
	kw := &Signature{
		Args:      []*Type{TAtom},
		QuoteArgs: map[int]bool{0: true},
		Patterns:  map[int]Value{0: NewAtom("s5akw")},
	}
	if !sigsHaveKeywordSlot([]ViableSig{{Sig: kw, Barrier: 1}}) {
		t.Error("sigsHaveKeywordSlot = false, want true")
	}
}

func TestS5APruneKeywordViable(t *testing.T) {
	kw := &Signature{
		Args:      []*Type{TAtom},
		QuoteArgs: map[int]bool{0: true},
		Patterns:  map[int]Value{0: NewAtom("s5akw")},
	}
	viable := []ViableSig{{Sig: kw, Barrier: 1}}
	// The matching keyword word keeps the sig.
	kept := pruneKeywordViable(viable, 0, NewWord("s5akw"))
	if len(kept) != 1 {
		t.Errorf("matching keyword pruned: %d", len(kept))
	}
	// A different word drops it.
	viable = []ViableSig{{Sig: kw, Barrier: 1}}
	kept = pruneKeywordViable(viable, 0, NewWord("other"))
	if len(kept) != 0 {
		t.Errorf("mismatching keyword kept: %d", len(kept))
	}
	// A non-word token drops it too.
	viable = []ViableSig{{Sig: kw, Barrier: 1}}
	kept = pruneKeywordViable(viable, 0, NewInteger(3))
	if len(kept) != 0 {
		t.Errorf("non-word token kept: %d", len(kept))
	}
}

// --- resolveForwardArgs arms ----------------------------------------------

func TestS5AForwardScanPatternPrune(t *testing.T) {
	// A concrete forward token whose TYPE matches but whose PATTERN
	// definitely rejects prunes the sig (the else-if arm).
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{{
		Args:       []*Type{TInteger},
		Patterns:   map[int]Value{0: NewInteger(5)},
		BarrierPos: 1,
	}}}
	e := s5aEng(t, r, []Value{NewWord("s5aw"), NewInteger(7)}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
}

func TestS5AForwardScanResolvedPatternPrune(t *testing.T) {
	// A def-bound word's binding prunes through the PATTERN-ONLY prune.
	r := covRegistry(t, nil)
	InstallDef(r, "s5abound", NewInteger(7))
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{{
		Args:       []*Type{TInteger},
		Patterns:   map[int]Value{0: NewInteger(5)},
		BarrierPos: 1,
	}}}
	e := s5aEng(t, r, []Value{NewWord("s5aw"), NewWord("s5abound")}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
}

func TestS5AForwardScanKeywordPrune(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{{
		Args:       []*Type{TAtom},
		QuoteArgs:  map[int]bool{0: true},
		Patterns:   map[int]Value{0: NewAtom("s5akw")},
		BarrierPos: 1,
	}}}
	e := s5aEng(t, r, []Value{NewWord("s5aw"), NewInteger(3)}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
}

func TestS5AForwardScanParenNotConsumed(t *testing.T) {
	// After the leading Integer prunes the 2-arg overload, the paren at
	// position 1 is consumed by no viable sig and stays raw.
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 1},
		{Args: []*Type{TString, TAny}, BarrierPos: 2},
	}}
	e := s5aEng(t, r, []Value{NewWord("s5aw"), NewInteger(1), NewOpenParen(), NewCloseParen()}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if !IsOpenParen(e.Tape.At(2)) {
		t.Error("unconsumed paren was evaluated")
	}
}

func TestS5AForwardScanInterpStringNotConsumed(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 1},
		{Args: []*Type{TString, TAny}, BarrierPos: 2},
	}}
	is := NewInterpString([]InterpPart{{Lit: "hi"}})
	e := s5aEng(t, r, []Value{NewWord("s5aw"), NewInteger(1), is}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if !IsInterpString(e.Tape.At(2)) {
		t.Error("unconsumed interp string was evaluated")
	}
}

func TestS5AForwardScanInterpStringEval(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TString}, BarrierPos: 1},
	}}
	is := NewInterpString([]InterpPart{{Lit: "hi"}})
	e := s5aEng(t, r, []Value{NewWord("s5aw"), is}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if s, err := AsString(e.Tape.At(1)); err != nil || s != "hi" {
		t.Errorf("interp string not evaluated in place: %v", e.Tape.At(1))
	}
}

func TestS5AForwardScanXmlNotConsumed(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 1},
		{Args: []*Type{TString, TAny}, BarrierPos: 2},
	}}
	xi := NewXmlInterp(XmlTmpl{Tag: "p"})
	e := s5aEng(t, r, []Value{NewWord("s5aw"), NewInteger(1), xi}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if !IsXmlInterp(e.Tape.At(2)) {
		t.Error("unconsumed xml interp was evaluated")
	}
}

func TestS5AForwardScanXmlEval(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: 1},
	}}
	xi := NewXmlInterp(XmlTmpl{Tag: "p"})
	e := s5aEng(t, r, []Value{NewWord("s5aw"), xi}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if IsXmlInterp(e.Tape.At(1)) {
		t.Error("xml interp not evaluated in place")
	}
}

func TestS5AForwardScanQuotedParenExprSkips(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: 1},
	}}
	pe := NewParenExpr([]Value{NewInteger(1)})
	pe.Quoted = true
	e := s5aEng(t, r, []Value{NewWord("s5aw"), pe}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if !IsParenExpr(e.Tape.At(1)) {
		t.Error("quoted ParenExpr was expanded")
	}
}

func TestS5AForwardScanInertReachSkips(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: 1},
	}}
	reach := NewReach(ReachInfo{Eval: false})
	e := s5aEng(t, r, []Value{NewWord("s5aw"), reach}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if !IsReach(e.Tape.At(1)) {
		t.Error("inert reach was expanded")
	}
}

func TestS5AForwardScanSpliceWordRewrite(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "s5asp", NewSplice(NewList([]Value{NewInteger(1), NewInteger(2)})))
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TInteger, TInteger}, BarrierPos: 2},
	}}
	e := s5aEng(t, r, []Value{NewWord("s5aw"), NewWord("s5asp")}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	// The splice-bound word expanded as (w): its two values sit on the tape.
	if e.Tape.Len() < 3 {
		t.Errorf("splice group did not expand: len=%d", e.Tape.Len())
	}
}

func TestS5AForwardScanSugarExpansion(t *testing.T) {
	r := covRegistry(t, nil)
	r.BindSugarWord(SugarStackArgs, "s5aunbound")
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: 1},
	}}
	marker := NewSugar(SugarInfo{Kind: SugarStackArgs})
	e := s5aEng(t, r, []Value{NewWord("s5aw"), marker, NewInteger(4)}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if IsSugar(e.Tape.At(1)) {
		t.Error("sugar marker not expanded")
	}
}

func TestS5AForwardScanSugarHeadError(t *testing.T) {
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1},
	}}
	marker := NewSugar(SugarInfo{Kind: SugarAngle, Name: "Box", HeadErr: "s5a bad head"})
	e := s5aEng(t, r, []Value{NewWord("s5aw"), marker}, 0)
	err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1})
	if err == nil || !strings.Contains(err.Error(), "s5a bad head") {
		t.Fatalf("resolveForwardArgs = %v, want head error", err)
	}
}

func TestS5AForwardScanReachCallHeadToken(t *testing.T) {
	// A tagged reach-collapsed named fn already in the window that would
	// claim its next token stops the scan.
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: 1},
	}}
	callee := NewFunction(FnDefInfo{Name: "s5acallee", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 1},
	}})
	callee.ReachGroup = true
	e := s5aEng(t, r, []Value{NewWord("s5aw"), callee, NewInteger(5)}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
}

func TestS5AForwardScanReachCallHeadAfterCollapse(t *testing.T) {
	// A reach-lowered group collapsing to a named fn that would claim the
	// following token stops the scan at the collapsed result.
	r := covRegistry(t, nil)
	fn := &FnDefInfo{Name: "s5aw", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: 1},
	}}
	callee := NewFunction(FnDefInfo{Name: "s5acallee", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 1},
	}})
	open := NewOpenParen()
	open.ReachGroup = true
	e := s5aEng(t, r, []Value{
		NewWord("s5aw"), open, callee, NewCloseParen(), NewInteger(5),
	}, 0)
	if err := e.resolveForwardArgs(fn, WordInfo{Name: "s5aw", ArgCount: -1}); err != nil {
		t.Fatalf("resolveForwardArgs: %v", err)
	}
	if !isFnDefValue(e.Tape.At(1)) {
		t.Errorf("group did not collapse to the fn value: %v", e.Tape.At(1))
	}
}

// --- StaticForwardType ----------------------------------------------------

func TestS5AStaticForwardTypeCarrierBoundary(t *testing.T) {
	e := engWithTape(t, nil, 0)
	if _, kind := e.StaticForwardType(NewCarrier(TInteger)); kind != FwdBoundary {
		t.Errorf("carrier classified %d, want FwdBoundary", kind)
	}
}

// --- evalParenGroupAt arms ------------------------------------------------

func TestS5AEvalParenGroupGuardFacts(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	boolCarrier := NewCarrier(TBoolean)
	toks := []Value{
		NewOpenParen(), NewOpenParen(), NewEnd(), boolCarrier,
		NewCloseParen(), NewCloseParen(),
	}
	e := s5aEng(t, r, toks, 0)
	if err := e.evalParenGroupAt(0); err != nil {
		t.Fatalf("evalParenGroupAt: %v", err)
	}
	res := e.Tape.At(0)
	if _, ok := res.Data.(GuardFactInfo); !ok {
		t.Errorf("guard fact not attached: %T", res.Data)
	}
}

func TestS5AEvalParenGroupMark(t *testing.T) {
	r := covRegistry(t, nil)
	e := s5aEng(t, r, []Value{NewOpenParen(), NewMark("s5am"), NewCloseParen()}, 0)
	if err := e.evalParenGroupAt(0); err != nil {
		t.Fatalf("evalParenGroupAt: %v", err)
	}
}

func TestS5AEvalParenGroupReturnCheckError(t *testing.T) {
	r := covRegistry(t, nil)
	rc := NewReturnCheck(ReturnCheckInfo{FuncName: "f", Returns: []*Type{TString}})
	e := s5aEng(t, r, []Value{NewOpenParen(), rc, NewInteger(1), NewCloseParen()}, 0)
	if err := e.evalParenGroupAt(0); err == nil {
		t.Fatal("return-type mismatch inside the group must error")
	}
}

func TestS5AEvalParenGroupDefCleanupError(t *testing.T) {
	r := covRegistry(t, nil)
	bad := NewEvalList([]Value{NewWord("s5a-missing")})
	dc := NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true})
	e := s5aEng(t, r, []Value{NewOpenParen(), bad, dc, NewCloseParen()}, 0)
	if err := e.evalParenGroupAt(0); err == nil {
		t.Fatal("DefCleanup residual eval error must propagate")
	}
}

func TestS5AEvalParenGroupInterpString(t *testing.T) {
	r := covRegistry(t, nil)
	is := NewInterpString([]InterpPart{{Lit: "hi"}})
	e := s5aEng(t, r, []Value{NewOpenParen(), is, NewCloseParen()}, 0)
	if err := e.evalParenGroupAt(0); err != nil {
		t.Fatalf("evalParenGroupAt: %v", err)
	}
	if s, err := AsString(e.Tape.At(0)); err != nil || s != "hi" {
		t.Errorf("group result = %v, want 'hi'", e.Tape.At(0))
	}
}

func TestS5AEvalParenGroupLiteralError(t *testing.T) {
	r := covRegistry(t, nil)
	failFn := NewFunction(FnDefInfo{Name: "s5afail", Signatures: []Signature{{
		Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return nil, errors.New("s5a lit boom")
		}),
		Returns: []*Type{}, BarrierPos: -1,
	}}})
	e := s5aEng(t, r, []Value{NewOpenParen(), failFn, NewCloseParen()}, 0)
	err := e.evalParenGroupAt(0)
	if err == nil || !strings.Contains(err.Error(), "s5a lit boom") {
		t.Fatalf("evalParenGroupAt = %v, want handler error", err)
	}
}

// --- stepWordUsurp / stepWordVal arms -------------------------------------

func TestS5AUsurpUndefinedCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	e := s5aEng(t, r, []Value{NewWordUsurp("s5a-missing", false)}, 0)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

func TestS5AUsurpNonFnCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "s5aval", NewInteger(5))
	s5aCheckOn(t, r)
	e := s5aEng(t, r, []Value{NewWordUsurp("s5aval", false)}, 0)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "illegal_ref" {
			found = true
		}
	}
	if !found {
		t.Error("no illegal_ref diagnostic")
	}
}

func TestS5AUsurpBarrierCommit(t *testing.T) {
	r := covRegistry(t, nil)
	caddSig := &r.Lookup("cadd").Signatures[0]
	fwd := NewForward(ForwardInfo{
		FuncName: "cadd", ExpectedArgs: 2, CollectedArgs: 2, StackArgs: 0,
		FuncIndex: 2, Sig: caddSig,
	})
	e := s5aEng(t, r, []Value{
		NewInteger(1), NewInteger(2), NewWord("cadd"), fwd, NewWordUsurp("cneg", false),
	}, 4)
	if err := e.stepWord(e.Tape.At(4)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
	if e.Pointer != 2 {
		t.Errorf("pointer = %d, want 2 (the committed word)", e.Pointer)
	}
}

func TestS5ARefUndefinedCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	e := s5aEng(t, r, []Value{NewWordRef("s5a-missing")}, 0)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

// TestS5AValDynamicCarrierCheckMode: with the function-only gate gone,
// `/v` on a dynamic Any-typed binding surfaces THAT carrier unchanged.
// It used to be narrowed to a Function carrier, which was the gate's
// escape hatch for "this might hold a fn at run time"; totality removes
// the need to guess, and the carrier keeps its own (wider) type.
func TestS5AValDynamicCarrierCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	c := NewCarrier(TAny)
	InstallDef(r, "s5adyn", c)
	s5aCheckOn(t, r)
	e := s5aEng(t, r, []Value{NewWordRef("s5adyn")}, 0)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
	got := e.Tape.At(0)
	if !got.Carrier || !got.Parent.Equal(TAny) {
		t.Errorf("/v did not surface the binding's own carrier: %v", got)
	}
}

func TestS5ARefNonFnCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "s5aval", NewInteger(5))
	s5aCheckOn(t, r)
	e := s5aEng(t, r, []Value{NewWordRef("s5aval")}, 0)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

func TestS5ARefArrivesAtPendingForward(t *testing.T) {
	r := covRegistry(t, nil)
	caddSig := &r.Lookup("cadd").Signatures[0]
	fwd := NewForward(ForwardInfo{
		FuncName: "cadd", ExpectedArgs: 2, CollectedArgs: 0, StackArgs: 0,
		FuncIndex: 0, Sig: caddSig,
	})
	e := s5aEng(t, r, []Value{NewWord("cadd"), fwd, NewWordRef("cneg")}, 2)
	if err := e.stepWord(e.Tape.At(2)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

func TestS5AHasPendingForwardCollectingParenBarrier(t *testing.T) {
	e := engWithTape(t, []Value{NewOpenParen(), NewInteger(1)}, 1)
	if e.hasPendingForwardCollecting() {
		t.Error("paren barrier must hide the outer forward")
	}
}

// --- stepWord arms: form capture, type bodies, def substitution -----------

func TestS5AStepWordFormArgCapture(t *testing.T) {
	r := covRegistry(t, nil)
	sig := &Signature{Args: []*Type{TAny}, FormArgs: map[int]bool{0: true}, BarrierPos: 1}
	fwd := NewForward(ForwardInfo{FuncName: "s5ahost", ExpectedArgs: 1, FuncIndex: 0, Sig: sig})
	e := s5aEng(t, r, []Value{NewWord("s5ahost"), fwd, NewWord("s5araw")}, 2)
	if err := e.stepWord(e.Tape.At(2)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

func TestS5AStepWordTypeBodyPlain(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.PushType("S5aTy", TInteger, NewTypeLiteral(TInteger))
	out, err := NewTop(r).Run([]Value{NewWord("S5aTy")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual = %v", out)
	}
}

func TestS5AStepWordTypeBodyFnShape(t *testing.T) {
	// A type name denotes its lattice NODE (the Stage 2 flip of
	// design/legacy/TYPE-REPRESENTATION.1.ignore), so a predicate-type name pushes
	// the bare node — inert by nature, no Quoted mark needed (the body
	// push this replaced had to be Quoted so the fn would not dispatch).
	r := covRegistry(t, nil)
	pred := NewFunction(FnDefInfo{Name: "S5aPred", Signatures: []Signature{{
		Args: []*Type{TAny}, BarrierPos: -1,
	}}})
	r.Defs.PushType("S5aPred", TFunction, pred)
	out, err := NewTop(r).Run([]Value{NewWord("S5aPred")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 1 || !IsBareTypeNode(out[0]) || !out[0].Equal(TFunction) {
		t.Errorf("a type name must denote its node: %v", out)
	}
}

func TestS5AStepWordDefReadCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "s5anum", NewInteger(5))
	s5aCheckOn(t, r)
	e := s5aEng(t, r, []Value{NewWord("s5anum")}, 0)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

func TestS5AStepWordSplicePendingForward(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "s5asp", NewSplice(NewList([]Value{NewInteger(1), NewInteger(2)})))
	caddSig := &r.Lookup("cadd").Signatures[0]
	fwd := NewForward(ForwardInfo{
		FuncName: "cadd", ExpectedArgs: 2, CollectedArgs: 0, StackArgs: 0,
		FuncIndex: 0, Sig: caddSig,
	})
	e := s5aEng(t, r, []Value{NewWord("cadd"), fwd, NewWord("s5asp")}, 2)
	if err := e.stepWord(e.Tape.At(2)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
	// The splice-bound word was wrapped as (w) and expanded to markers.
	if !IsOpenParen(e.Tape.At(2)) {
		t.Errorf("splice word not wrapped as a group: %v", e.Tape.At(2))
	}
}

func TestS5AStepWordReservedLiterals(t *testing.T) {
	r := w8reg(t)
	out, err := NewTop(r).Run([]Value{
		NewWord("true"), NewWord("false"), NewWord("none"), NewWord("null"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("residual = %v", out)
	}
	if b, err := AsBoolean(out[0]); err != nil || !b {
		t.Errorf("out[0] = %v, want true", out[0])
	}
}

func TestS5AStepWordUndefinedCheckMode(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	out, err := NewTop(r).Run([]Value{NewWord("s5a-nope")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 1 || !out[0].Undefined {
		t.Errorf("residual = %v, want one Undefined atom", out)
	}
}

// --- check-mode fallback retry / surface-shape hook -----------------------

func s5aFallbackReg(t *testing.T) *Registry {
	t.Helper()
	return covRegistry(t, func(r *Registry) {
		h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return nil, nil
		}
		r.RegisterNativeFunc(NativeFunc{Name: "s5afb", Signatures: []Signature{
			{Args: []*Type{TInteger}, Impl: Go(h), Returns: []*Type{TInteger}, BarrierPos: -1},
			{Fallback: true, Impl: Go(h), Returns: []*Type{}, BarrierPos: 0},
		}})
	})
}

func TestS5ACheckModeFallbackRetry(t *testing.T) {
	r := s5aFallbackReg(t)
	s5aCheckOn(t, r)
	e := s5aEng(t, r, []Value{NewString("x"), NewWord("s5afb")}, 1)
	if err := e.stepWord(e.Tape.At(1)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

func TestS5ACheckModeSurfaceShapeHandles(t *testing.T) {
	r := s5aFallbackReg(t)
	s5aCheckOn(t, r)
	saved := CheckBraid.CheckModeSurfaceShape
	CheckBraid.CheckModeSurfaceShape = func(*Engine, WordInfo, SrcPos) (bool, error) {
		return true, nil
	}
	defer func() { CheckBraid.CheckModeSurfaceShape = saved }()
	e := s5aEng(t, r, []Value{NewString("x"), NewWord("s5afb")}, 1)
	if err := e.stepWord(e.Tape.At(1)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

// --- trace notes / recorder / drift hook ----------------------------------

func TestS5ATraceNotesOnDispatch(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.SetTrace(func(int, int, []Value, string) {})
	out, err := e.Run([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if renderAll(out) != "3" {
		t.Errorf("out = %s", renderAll(out))
	}
}

func TestS5ARecorderOnDispatch(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	rec := &w4CountRecorder{}
	e.SetRecorder(rec)
	if _, err := e.Run([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rec.calls == 0 {
		t.Error("OnCall never fired")
	}
	if rec.lits == 0 {
		t.Error("OnPushLit never fired for forward-collected args")
	}
}

func TestS5ADriftWindowRecorderHook(t *testing.T) {
	saved := DriftWindowRecorder
	DriftWindowRecorder = func(*Engine, WordInfo, *Signature, []int) bool { return true }
	defer func() { DriftWindowRecorder = saved }()
	r := covRegistry(t, nil)
	e := s5aEng(t, r, []Value{
		NewInteger(1), NewInteger(2), NewWordModified("cadd", -1, true, false),
	}, 2)
	if err := e.stepWord(e.Tape.At(2)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
}

// --- dynShuffleConsumerAt -------------------------------------------------

func TestS5ADynShuffleUnknownWord(t *testing.T) {
	e := engWithTape(t, []Value{NewWord("s5azz")}, 0)
	if e.dynShuffleConsumerAt(0) {
		t.Error("unknown word must not be a shuffle consumer")
	}
}

// --- policy gates ---------------------------------------------------------

func TestS5APolicyGateWordDenied(t *testing.T) {
	r := covRegistry(t, nil)
	if err := r.Capabilities.Set(CapPolicy, s5aPolicy{denyWord: "cadd"}); err != nil {
		t.Fatal(err)
	}
	e := s5aEng(t, r, []Value{NewWord("cadd")}, 0)
	err := e.stepWord(e.Tape.At(0))
	if err == nil || !strings.Contains(err.Error(), "s5a policy denies word") {
		t.Fatalf("stepWord = %v, want policy denial", err)
	}
}

func TestS5APolicyGateModuleCallDenied(t *testing.T) {
	r := covRegistry(t, nil)
	if err := r.Capabilities.Set(CapPolicy, s5aPolicy{denyModule: "s5amod"}); err != nil {
		t.Fatal(err)
	}
	h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return nil, nil
	}
	sig := &Signature{Impl: Go(h), ModuleCall: &ModuleCallID{Module: "s5amod", Export: "x"}}
	e := s5aEng(t, r, []Value{NewWord("s5amx")}, 0)
	err := e.execMatch(&MatchResult{Sig: sig, Name: "s5amx"})
	if err == nil || !strings.Contains(err.Error(), "s5a policy denies module") {
		t.Fatalf("execMatch = %v, want module denial", err)
	}
}

// --- execMatch arg-processing arms ----------------------------------------

func TestS5AExecMatchMapEvalError(t *testing.T) {
	r := covRegistry(t, nil)
	om := NewOrderedMap()
	om.Set("a", NewParenExpr([]Value{NewWord("s5a-missing")}))
	mv := NewMap(om)
	mv.Eval = true
	h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return nil, nil
	}
	sig := &Signature{Args: []*Type{TMap}, Impl: Go(h)}
	e := s5aEng(t, r, []Value{mv, NewWord("s5awm")}, 1)
	err := e.execMatch(&MatchResult{
		Sig: sig, Name: "s5awm", Args: []Value{mv}, Positions: []int{0},
	})
	if err == nil || !strings.Contains(err.Error(), "s5a-missing") {
		t.Fatalf("execMatch = %v, want map-eval error", err)
	}
}

func TestS5AExecMatchListEvalError(t *testing.T) {
	r := covRegistry(t, nil)
	lv := NewEvalList([]Value{NewWord("s5a-missing")})
	h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return nil, nil
	}
	sig := &Signature{Args: []*Type{TList}, Impl: Go(h)}
	e := s5aEng(t, r, []Value{lv, NewWord("s5awl")}, 1)
	err := e.execMatch(&MatchResult{
		Sig: sig, Name: "s5awl", Args: []Value{lv}, Positions: []int{0},
	})
	if err == nil || !strings.Contains(err.Error(), "s5a-missing") {
		t.Fatalf("execMatch = %v, want list-eval error", err)
	}
}

func TestS5AExecMatchNoEvalBodyUseScan(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	body := NewEvalList([]Value{NewWord("s5aused")})
	sig := &Signature{Args: []*Type{TList}, NoEvalArgs: map[int]bool{0: true}}
	e := s5aEng(t, r, []Value{body, NewWord("s5awb")}, 1)
	if err := e.execMatch(&MatchResult{
		Sig: sig, Name: "s5awb", Args: []Value{body}, Positions: []int{0},
	}); err != nil {
		t.Fatalf("execMatch: %v", err)
	}
	if !r.Check.DefsUsed["s5aused"] {
		t.Error("body word not recorded as a use")
	}
}

func TestS5AExecMatchUndefinedArgGradualizes(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	a := NewAtom("s5aund")
	a.Undefined = true
	sig := &Signature{Args: []*Type{TAny}}
	e := s5aEng(t, r, []Value{a, NewWord("s5awu")}, 1)
	if err := e.execMatch(&MatchResult{
		Sig: sig, Name: "s5awu", Args: []Value{a}, Positions: []int{0},
	}); err != nil {
		t.Fatalf("execMatch: %v", err)
	}
}

func TestS5AExecMatchPatternFillDefaults(t *testing.T) {
	r := covRegistry(t, nil)
	h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return nil, nil
	}
	sig := &Signature{
		Args: []*Type{TInteger}, Patterns: map[int]Value{0: NewInteger(5)},
		Impl: Go(h),
	}
	e := s5aEng(t, r, []Value{NewInteger(5), NewWord("s5apat")}, 1)
	if err := e.execMatch(&MatchResult{
		Sig: sig, Name: "s5apat", Args: []Value{NewInteger(5)}, Positions: []int{0},
	}); err != nil {
		t.Fatalf("execMatch: %v", err)
	}
}

// --- FullStack: check-mode fold + runtime error ---------------------------

func TestS5AFullStackCheckFold(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return nil, nil
		}
		cfn := func(args []Value, stack []Value, reg *Registry) []Value {
			return []Value{NewCarrier(TInteger)}
		}
		r.RegisterNativeFunc(NativeFunc{Name: "s5afs", Signatures: []Signature{{
			Args: []*Type{}, Impl: Go(h, FullStack(), CheckFullStack(cfn)),
			Returns: []*Type{TAny}, BarrierPos: 0,
		}}})
	})
	s5aCheckOn(t, r)
	stub := &s5aEmit{EmitRecorder: TheInactiveEmit, active: true, armed: true,
		fold: []Value{NewInteger(9)}, foldOK: true}
	saved := r.Check.Emit
	r.Check.Emit = stub
	defer func() { r.Check.Emit = saved }()

	e := s5aEng(t, r, []Value{NewInteger(1), NewWord("s5afs")}, 1)
	if err := e.stepWord(e.Tape.At(1)); err != nil {
		t.Fatalf("stepWord: %v", err)
	}
	if e.Tape.Len() != 1 {
		t.Fatalf("tape = %d tokens, want the folded value only", e.Tape.Len())
	}
	if n, err := AsInteger(e.Tape.At(0)); err != nil || n != 9 {
		t.Errorf("folded value = %v, want 9", e.Tape.At(0))
	}
}

func TestS5AFullStackRuntimeError(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return nil, errors.New("s5a full boom")
		}
		r.RegisterNativeFunc(NativeFunc{Name: "s5afse", Signatures: []Signature{{
			Args: []*Type{}, Impl: Go(h, FullStack()),
			Returns: []*Type{TAny}, BarrierPos: 0,
		}}})
	})
	_, err := NewTop(r).Run([]Value{NewInteger(1), NewWord("s5afse")})
	if err == nil || !strings.Contains(err.Error(), "s5a full boom") {
		t.Fatalf("Run = %v, want handler error", err)
	}
}

// --- TCO: shell elision (values below the tail call) ----------------------

func TestS5ATCOShellElideValuesBelow(t *testing.T) {
	r := covRegistry(t, nil)
	InstallFnDef(r, "s5avb", FnDefInfo{
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "n", Type: TInteger}},
			Returns: []*Type{TInteger, TInteger},
			Impl: Boru([]Value{
				NewOpenParen(), NewInteger(7),
				NewOpenParen(), NewWord("ctwice"), NewWord("n"), NewCloseParen(),
				NewCloseParen(),
			}),
			BarrierPos: BarrierAllForward,
		}},
	})
	before := r.TCO.Elided
	out, err := NewTop(r).Run([]Value{NewWord("s5avb"), NewInteger(3)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if renderAll(out) != "7 | 6" {
		t.Errorf("out = %s, want 7 | 6", renderAll(out))
	}
	if r.TCO.Elided <= before {
		t.Errorf("Elided = %d (before %d): shell elision did not fire", r.TCO.Elided, before)
	}
}

// --- ParkResult -----------------------------------------------------------

func TestS5AParkResultAdvances(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		h := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return []Value{NewInteger(7)}, nil
		}
		r.RegisterNativeFunc(NativeFunc{Name: "s5apark", Signatures: []Signature{{
			Impl: Go(h, Park()), Returns: []*Type{TInteger}, BarrierPos: -1,
		}}})
	})
	out, err := NewTop(r).Run([]Value{NewWord("s5apark")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if renderAll(out) != "7" {
		t.Errorf("out = %s, want 7", renderAll(out))
	}
}

// --- spliceMatchResults / resolved-index helpers --------------------------

func TestS5ASpliceMatchResultsCompactsSurvivor(t *testing.T) {
	e := engWithTape(t, []Value{
		NewInteger(1), NewAtom("x"), NewInteger(2), NewWord("cadd"),
	}, 3)
	if err := e.spliceMatchResults(nil, []int{0, 2}, 2, []Value{NewInteger(9)}); err != nil {
		t.Fatalf("spliceMatchResults: %v", err)
	}
	if e.Tape.Len() != 2 {
		t.Fatalf("tape len = %d, want 2", e.Tape.Len())
	}
	if a, err := AsAtom(e.Tape.At(0)); err != nil || a != "x" {
		t.Errorf("survivor not compacted: %v", e.Tape.At(0))
	}
}

func TestS5AResolvedIndicesStopAtParen(t *testing.T) {
	e := engWithTape(t, []Value{NewOpenParen(), NewInteger(1), NewWord("w")}, 2)
	got := e.ResolvedIndicesBefore(5)
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("indices = %v, want [1]", got)
	}
}

func TestS5AResolvedStackBeforeFrom(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1), NewInteger(2), NewWord("w")}, 2)
	got := e.resolvedStackBeforeFrom(0, nil)
	if renderAll(got) != "1 | 2" {
		t.Errorf("stack = %s, want 1 | 2", renderAll(got))
	}
}

// --- spliceIsData / stepLiteral expansion / splice recording --------------

func TestS5ASpliceIsData(t *testing.T) {
	if !spliceIsData(SpliceInfo{Data: NewList([]Value{NewInteger(1), NewInteger(2)})}) {
		t.Error("plain data splice reported code-bearing")
	}
	if spliceIsData(SpliceInfo{Data: NewList([]Value{NewWord("add")})}) {
		t.Error("code-bearing splice reported data")
	}
}

func TestS5AStepLiteralExpandsParenExpr(t *testing.T) {
	e := engWithTape(t, []Value{NewParenExpr([]Value{NewInteger(1)})}, 0)
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
	if e.Tape.Len() != 3 || !IsOpenParen(e.Tape.At(0)) {
		t.Errorf("ParenExpr not expanded: len=%d", e.Tape.Len())
	}
}

func TestS5AStepLiteralSpliceDynRecording(t *testing.T) {
	r := covRegistry(t, nil)
	stub := &s5aEmit{EmitRecorder: TheInactiveEmit, active: true, armed: true}
	saved := r.Check.Emit
	r.Check.Emit = stub
	defer func() { r.Check.Emit = saved }()
	sp := NewSplice(NewDynamicCarrier(TAny))
	e := s5aEng(t, r, []Value{sp}, 0)
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
	if IsSplice(e.Tape.At(0)) {
		t.Error("splice marker not expanded")
	}
}

// --- stepLiteral: check-mode dispatch hooks -------------------------------

func TestS5AStepLiteralCheckHooks(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)

	savedShaped := CheckBraid.TryShapedMethodDispatch
	CheckBraid.TryShapedMethodDispatch = func(*Engine, int) bool { return true }
	e := s5aEng(t, r, []Value{NewInteger(1)}, 0)
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral (shaped): %v", err)
	}
	CheckBraid.TryShapedMethodDispatch = savedShaped

	savedMember := CheckBraid.TryMemberFnArrivalDispatch
	CheckBraid.TryMemberFnArrivalDispatch = func(*Engine, int) bool { return true }
	e2 := s5aEng(t, r, []Value{NewInteger(1)}, 0)
	if err := e2.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral (member): %v", err)
	}
	CheckBraid.TryMemberFnArrivalDispatch = savedMember

	savedDyn := CheckBraid.TryDynamicFnValueDispatch
	CheckBraid.TryDynamicFnValueDispatch = func(*Engine, int) bool { return true }
	e3 := s5aEng(t, r, []Value{NewInteger(1)}, 0)
	if err := e3.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral (dyn): %v", err)
	}
	CheckBraid.TryDynamicFnValueDispatch = savedDyn
}

// --- stepLiteral: reach-collapsed fn arrival gate -------------------------

func TestS5AReachFnDispatchModMarksData(t *testing.T) {
	r := covRegistry(t, nil)
	hostSig := &Signature{Args: []*Type{TAny}, BarrierPos: 1}
	fwd := NewForward(ForwardInfo{FuncName: "s5ahost", ExpectedArgs: 1, FuncIndex: 0, Sig: hostSig})
	fnVal := NewFunction(FnDefInfo{Name: "s5acallee", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: 1},
	}})
	fnVal.ReachGroup = true
	dm := NewDispatchMod(DispatchModInfo{Val: true})
	e := s5aEng(t, r, []Value{NewWord("s5ahost"), fwd, fnVal, dm}, 2)
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
}

func TestS5AReachFnZeroArgPropertyCall(t *testing.T) {
	r := covRegistry(t, nil)
	hostSig := &Signature{Args: []*Type{TAny}, BarrierPos: 1}
	fwd := NewForward(ForwardInfo{FuncName: "s5ahost", ExpectedArgs: 1, FuncIndex: 0, Sig: hostSig})
	h0 := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return []Value{NewInteger(1)}, nil
	}
	fnVal := NewFunction(FnDefInfo{Name: "s5aprop", Signatures: []Signature{
		{Impl: Go(h0), Returns: []*Type{TInteger}, BarrierPos: -1},
	}})
	fnVal.ReachGroup = true
	e := s5aEng(t, r, []Value{NewWord("s5ahost"), fwd, fnVal}, 2)
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
}

// --- resolveConsumedInertTypeShape / resolveInertTypeShape ----------------

func TestS5AResolveInertTypeShape(t *testing.T) {
	r := covRegistry(t, nil)
	e := s5aEng(t, r, nil, 0)

	// A typed list whose child word resolves.
	if _, ok := e.resolveInertTypeShape(NewTypedList(NewWord("Integer"))); !ok {
		t.Error("resolvable typed list not resolved")
	}
	// A typed list whose child is already resolved.
	if _, ok := e.resolveInertTypeShape(NewTypedList(NewTypeLiteral(TInteger))); ok {
		t.Error("already-resolved typed list reported changed")
	}
	// A disjunct with word alternatives resolves.
	if _, ok := e.resolveInertTypeShape(NewDisjunct([]Value{NewWord("Integer"), NewWord("String")})); !ok {
		t.Error("resolvable disjunct not resolved")
	}
	// An already-resolved disjunct does not.
	dv := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if _, ok := e.resolveInertTypeShape(dv); ok {
		t.Error("already-resolved disjunct reported changed")
	}
}

func TestS5AResolveConsumedInertTypeShape(t *testing.T) {
	r := covRegistry(t, nil)
	e := s5aEng(t, r, nil, 0)

	// Quoted arg: untouched.
	q := NewTypedList(NewWord("Integer"))
	q.Quoted = true
	m1 := &MatchResult{Sig: &Signature{Args: []*Type{TAny}}, Args: []Value{q}}
	e.resolveConsumedInertTypeShape(m1, 0)
	if !m1.Args[0].Quoted {
		t.Error("quoted arg mutated")
	}

	// NoEvalMapArgs slot: untouched.
	m2 := &MatchResult{
		Sig:  &Signature{Args: []*Type{TAny}, NoEvalMapArgs: map[int]bool{0: true}},
		Args: []Value{NewTypedList(NewWord("Integer"))},
	}
	e.resolveConsumedInertTypeShape(m2, 0)

	// Plain slot: resolved in place.
	m3 := &MatchResult{
		Sig:  &Signature{Args: []*Type{TAny}},
		Args: []Value{NewTypedList(NewWord("Integer"))},
	}
	e.resolveConsumedInertTypeShape(m3, 0)
	ci, err := AsChildType(m3.Args[0])
	if err != nil || IsWord(ci.Child) {
		t.Errorf("typed-list child not resolved: %v", m3.Args[0])
	}
}

func TestS5AAutoEvalStackResolvesInertShape(t *testing.T) {
	r := covRegistry(t, nil)
	e := s5aEng(t, r, []Value{NewTypedList(NewWord("Integer"))}, 0)
	if err := e.autoEvalStack(); err != nil {
		t.Fatalf("autoEvalStack: %v", err)
	}
	ci, err := AsChildType(e.Tape.At(0))
	if err != nil || IsWord(ci.Child) {
		t.Errorf("typed-list child not resolved on the residual stack: %v", e.Tape.At(0))
	}
}

// --- AutoEvalConsumedList / Map and list recording ------------------------

func TestS5AAutoEvalConsumedHelpers(t *testing.T) {
	r := covRegistry(t, nil)
	lv, err := AutoEvalConsumedList(r, NewEvalList([]Value{NewInteger(1)}))
	if err != nil {
		t.Fatalf("AutoEvalConsumedList: %v", err)
	}
	if lst, lerr := AsList(lv); lerr != nil || lst.Len() != 1 {
		t.Errorf("list = %v", lv)
	}
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	mv := NewMap(om)
	if _, err := AutoEvalConsumedMap(r, mv, false); err != nil {
		t.Fatalf("AutoEvalConsumedMap: %v", err)
	}
}

func TestS5AAutoEvalListQuotedInertElem(t *testing.T) {
	r := covRegistry(t, nil)
	tq := NewTypedList(NewWord("Integer"))
	tq.Quoted = true
	lv, err := AutoEvalConsumedList(r, NewEvalList([]Value{tq}))
	if err != nil {
		t.Fatalf("AutoEvalConsumedList: %v", err)
	}
	lst, _ := AsList(lv)
	ci, cerr := AsChildType(lst.Get(0))
	if cerr != nil || IsWord(ci.Child) {
		t.Errorf("quoted typed-list element not resolved: %v", lst.Get(0))
	}
}

func TestS5AAutoEvalListRecording(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	stub := &s5aEmit{EmitRecorder: TheInactiveEmit, active: true, armed: true}
	saved := r.Check.Emit
	r.Check.Emit = stub
	defer func() { r.Check.Emit = saved }()

	// Top-level: RecordMakeList.
	eTop := NewTop(r)
	if _, err := eTop.autoEvalList(NewEvalList([]Value{NewDynamicCarrier(TAny)}), false); err != nil {
		t.Fatalf("autoEvalList top: %v", err)
	}
	// Consumed inner: RecordMakeListInner.
	eSub := New(r)
	if _, err := eSub.autoEvalList(NewEvalList([]Value{NewDynamicCarrier(TAny)}), true); err != nil {
		t.Fatalf("autoEvalList inner: %v", err)
	}
}

// --- interp string / xml interp dynamic arms ------------------------------

func TestS5AInterpStringMultiValueHole(t *testing.T) {
	r := covRegistry(t, nil)
	e := s5aEng(t, r, nil, 0)
	is := NewInterpString([]InterpPart{
		{Lit: "x"},
		{Expr: []Value{NewInteger(1), NewInteger(2)}},
	})
	got, err := e.evalInterpString(is)
	if err != nil {
		t.Fatalf("evalInterpString: %v", err)
	}
	if s, serr := AsString(got); serr != nil || s != "x12" {
		t.Errorf("result = %v, want 'x12'", got)
	}
}

func TestS5AInterpStringDynamicHole(t *testing.T) {
	r := covRegistry(t, nil)
	savedVCC := AnalysisImpl.ValueCarriesCarrier
	AnalysisImpl.ValueCarriesCarrier = func(v Value) bool { return v.Carrier }
	defer func() { AnalysisImpl.ValueCarriesCarrier = savedVCC }()
	stub := &s5aEmit{EmitRecorder: TheInactiveEmit, active: true, armed: true}
	saved := r.Check.Emit
	r.Check.Emit = stub
	defer func() { r.Check.Emit = saved }()

	e := s5aEng(t, r, nil, 0)
	is := NewInterpString([]InterpPart{{Expr: []Value{NewCarrier(TInteger)}}})
	got, err := e.evalInterpString(is)
	if err != nil {
		t.Fatalf("evalInterpString: %v", err)
	}
	if !got.Carrier || !got.Parent.Equal(TString) {
		t.Errorf("dynamic interp = %v, want String carrier", got)
	}
}

func TestS5AXmlInterpDynamic(t *testing.T) {
	r := covRegistry(t, nil)
	savedVCC := AnalysisImpl.ValueCarriesCarrier
	AnalysisImpl.ValueCarriesCarrier = func(v Value) bool { return v.Carrier }
	defer func() { AnalysisImpl.ValueCarriesCarrier = savedVCC }()
	stub := &s5aEmit{EmitRecorder: TheInactiveEmit, active: true, armed: true}
	saved := r.Check.Emit
	r.Check.Emit = stub
	defer func() { r.Check.Emit = saved }()

	child := XmlTmpl{Tag: "q", Cren: []XmlCren{
		{Kind: XmlCrenExpr, Expr: []Value{NewCarrier(TInteger)}},
		{Kind: XmlCrenExpr, Expr: []Value{NewInteger(1), NewInteger(2)}},
	}}
	tmpl := XmlTmpl{
		Tag: "p",
		Attr: []XmlAttrTmpl{{
			Name: "a", Parts: []InterpPart{{Expr: []Value{NewCarrier(TInteger)}}},
		}},
		Cren: []XmlCren{{Kind: XmlCrenChild, Child: &child}},
	}
	e := s5aEng(t, r, nil, 0)
	got, err := e.EvalXmlInterp(NewXmlInterp(tmpl))
	if err != nil {
		t.Fatalf("EvalXmlInterp: %v", err)
	}
	if !got.Carrier || !got.Parent.Equal(TXml) {
		t.Errorf("dynamic xml interp = %v, want Xml carrier", got)
	}
}

// --- AutoEvalMap arms -----------------------------------------------------

func TestS5AAutoEvalMapVarietyOfValues(t *testing.T) {
	r := covRegistry(t, nil)
	om := NewOrderedMap()
	om.Implicit = true
	om.Set("t", NewTypedList(NewWord("Integer")))
	om.Set("s", NewSplice(NewList([]Value{NewInteger(1), NewInteger(2)})))
	asc := NewInteger(5)
	asc.SetAscribed(TInteger)
	om.Set("a", asc)
	mv := NewMap(om)

	e := s5aEng(t, r, nil, 0)
	got, err := e.AutoEvalMap(mv, false, false)
	if err != nil {
		t.Fatalf("AutoEvalMap: %v", err)
	}
	gm, _ := AsMap(got)
	if tv, ok := gm.Get("t"); !ok || IsWord(func() Value { ci, _ := AsChildType(tv); return ci.Child }()) {
		t.Errorf("typed-list value not resolved: %v", tv)
	}
	if sv, ok := gm.Get("s"); !ok || !sv.Parent.Equal(TList) {
		t.Errorf("multi-result value not wrapped in a list: %v", sv)
	}
	if av, ok := gm.Get("a"); !ok || av.AscribedType() != nil {
		t.Errorf("ascription not stripped from map value: %v", av)
	}
}

func TestS5AAutoEvalMapConstFold(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	savedCE := CheckBraid.ConcreteEvalOnce
	CheckBraid.ConcreteEvalOnce = func(*Engine, []Value) (Value, bool) {
		return NewInteger(7), true
	}
	defer func() { CheckBraid.ConcreteEvalOnce = savedCE }()

	om := NewOrderedMap()
	om.Set("p", NewParenExpr([]Value{NewInteger(3)}))
	om.Set("b", NewInteger(3))
	mv := NewMap(om)
	e := s5aEng(t, r, nil, 0)
	got, err := e.AutoEvalMap(mv, false, false)
	if err != nil {
		t.Fatalf("AutoEvalMap: %v", err)
	}
	gm, _ := AsMap(got)
	pv, _ := gm.Get("p")
	if n, nerr := AsInteger(pv); nerr != nil || n != 7 {
		t.Errorf("paren value not folded: %v", pv)
	}
	bv, _ := gm.Get("b")
	if n, nerr := AsInteger(bv); nerr != nil || n != 7 {
		t.Errorf("bare value not folded: %v", bv)
	}
}

func TestS5AAutoEvalMapConsumedRecording(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	stub := &s5aEmit{EmitRecorder: TheInactiveEmit, active: true, armed: true}
	saved := r.Check.Emit
	r.Check.Emit = stub
	defer func() { r.Check.Emit = saved }()

	om := NewOrderedMap()
	om.Set("l", NewEvalList([]Value{NewDynamicCarrier(TAny)}))
	mv := NewMap(om)
	e := s5aEng(t, r, nil, 0)
	if _, err := e.AutoEvalMap(mv, false, true); err != nil {
		t.Fatalf("AutoEvalMap: %v", err)
	}
}

// --- exprHasEffect / constFoldContainerVal --------------------------------

func TestS5AExprHasEffect(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "s5amod", NewModuleInstance(ModuleDesc{ID: "s5am"}))
	e := s5aEng(t, r, nil, 0)

	if !e.exprHasEffect([]Value{NewParenExpr([]Value{NewWord("print")}), NewInteger(1)}) {
		t.Error("print inside a paren not detected")
	}
	if !e.exprHasEffect([]Value{NewWord("s5amod"), NewInteger(1)}) {
		t.Error("module-bound word not detected")
	}
	if !e.exprHasEffect([]Value{NewList([]Value{NewWord("print")})}) {
		t.Error("print inside a list not detected")
	}
	om := NewOrderedMap()
	om.Set("k", NewWord("print"))
	if !e.exprHasEffect([]Value{NewMap(om)}) {
		t.Error("print inside a map not detected")
	}
	if e.exprHasEffect([]Value{NewInteger(1)}) {
		t.Error("pure expression flagged")
	}
	// The fold declines an effectful expression outright.
	if _, ok := e.constFoldContainerVal([]Value{NewWord("print")}); ok {
		t.Error("constFoldContainerVal folded an effectful expression")
	}
}

// --- execFnDefLiteral: reach tag cleared ----------------------------------

func TestS5AExecFnDefLiteralClearsReachTag(t *testing.T) {
	r := covRegistry(t, nil)
	h0 := func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return []Value{NewInteger(1)}, nil
	}
	fnVal := NewFunction(FnDefInfo{Name: "s5afn0", Signatures: []Signature{
		{Impl: Go(h0), Returns: []*Type{TInteger}, BarrierPos: -1},
	}})
	fnVal.ReachGroup = true
	e := s5aEng(t, r, []Value{fnVal}, 0)
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
	for i := 0; i < e.Tape.Len(); i++ {
		if e.Tape.At(i).ReachGroup {
			t.Errorf("ReachGroup tag survived at %d", i)
		}
	}
}
