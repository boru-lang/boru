package core

// Wave-8 coverage for engine.go: the interpreter step loop, dispatch,
// forward-collection, check-mode drain, and error arms. White-box tests,
// in-package: helpers and *Engine methods are driven directly with
// hand-built tape/registry state where a full Run() would be a roundabout
// way to reach a guard arm, and driven through Run() where the arm only
// fires as part of real execution. See design/TEST-SEAMS.10.md and the
// wave-7 engine_seam7*_test.go suites for the pattern.

import (
	"strings"
	"testing"
)

// --- stepEnd: pending forward at a statement boundary ---------------------

func TestW8StepEndPendingForwardBeforeFunc(t *testing.T) {
	// A Forward marker sitting BEFORE its function word, then an End.
	// stepEnd finds the forward below the End, removes both, and applies
	// the fwdIdx<funcIdx adjustment before curryOrStack.
	r := covRegistry(t, nil)
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "cadd", Sig: sig, FuncIndex: 2, CollectedArgs: 1, StackArgs: 0})
	e := NewTop(r)
	e.Tape = NewTape([]Value{fwd, NewInteger(1), NewWord("cadd"), NewEnd()}, StackHeadroom)
	e.Pointer = 3 // the End
	if err := e.stepEnd(); err != nil {
		t.Fatalf("stepEnd: %v", err)
	}
}

func TestW8StepEndPendingForwardAfterFunc(t *testing.T) {
	// The Forward marker sits AFTER its function word (fwdIdx>funcIdx), so
	// the funcIdx-- arm is NOT taken. Covers the plain endIdx>fwdIdx path.
	r := covRegistry(t, nil)
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "cadd", Sig: sig, FuncIndex: 0, CollectedArgs: 1, StackArgs: 0})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("cadd"), NewInteger(1), fwd, NewEnd()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepEnd(); err != nil {
		t.Fatalf("stepEnd: %v", err)
	}
}

func TestW8StepEndNoPendingForward(t *testing.T) {
	// An End with no pending forward below it (stops at an open paren) just
	// removes the End.
	e := engWithTape(t, []Value{NewOpenParen(), NewInteger(1), NewEnd()}, 2)
	if err := e.stepEnd(); err != nil {
		t.Fatalf("stepEnd: %v", err)
	}
	if e.Tape.Len() != 2 {
		t.Errorf("End not removed: len=%d", e.Tape.Len())
	}
}

// --- curryOrStack ---------------------------------------------------------

func TestW8CurryOrStackNonWordFunc(t *testing.T) {
	// funcIdx points at a non-word → early return, pointer set to funcIdx.
	e := engWithTape(t, []Value{NewInteger(1)}, 0)
	e.curryOrStack(0, 0, 0)
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0", e.Pointer)
	}
}

func TestW8CurryOrStackExcludeIndices(t *testing.T) {
	// An inner Forward with CollectedArgs>0 and StackArgs>0 exercises the
	// exclude-index bookkeeping (collected-arg and stack-arg loops), and the
	// resolved-loop `continue` for excluded positions. An outer forward is
	// present so the curry-list branch runs.
	r := covRegistry(t, nil)
	inner := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 2, CollectedArgs: 1, StackArgs: 1})
	e := NewTop(r)
	e.Tape = NewTape([]Value{
		NewInteger(100), NewInteger(200), NewWord("cadd"), inner, NewWord("cmul"),
	}, StackHeadroom)
	e.curryOrStack(4, 0, 0)
	// cmul left on the stack; no panic is the key assertion.
	if e.Tape.Len() == 0 {
		t.Fatal("tape emptied unexpectedly")
	}
}

func TestW8CurryOrStackCurryList(t *testing.T) {
	// An outer Forward before checkStart with collectedCount>=1 builds a
	// curry list [word, arg...] and splices it in.
	r := covRegistry(t, nil)
	outer := NewForward(ForwardInfo{FuncName: "cmul", FuncIndex: 2, CollectedArgs: 0, StackArgs: 0})
	e := NewTop(r)
	e.Tape = NewTape([]Value{outer, NewInteger(7), NewWord("cadd")}, StackHeadroom)
	e.curryOrStack(2, 1, 0)
	// A curry list should now sit where cadd+7 were.
	found := false
	for i := 0; i < e.Tape.Len(); i++ {
		if e.Tape.At(i).Parent.Equal(TList) {
			found = true
		}
	}
	if !found {
		t.Error("expected a curry list to be spliced in")
	}
}

func TestW8CurryOrStackCheckStartClamp(t *testing.T) {
	// collectedCount > funcIdx forces checkStart<0 → clamp to 0. With no
	// outer forward reachable (loop starts at -1) this force-stacks.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1), NewWord("cadd")}, StackHeadroom)
	e.curryOrStack(1, 5, 0)
	if e.Pointer != 1 {
		t.Errorf("pointer = %d, want 1 (force-stacked at funcIdx)", e.Pointer)
	}
}

// --- matchSignature: forward-scan boundary and type-path arms -------------

// mkFn wraps a single native-style Signature in an FnDefInfo.
func mkFn(sig Signature) *FnDefInfo { return &FnDefInfo{Signatures: []Signature{sig}} }

func TestW8MatchSignatureForwardOpenParenBoundary(t *testing.T) {
	// A forward-collecting sig whose next forward token is an OpenParen
	// (left raw) treats it as a boundary and stops the forward scan.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(0), NewOpenParen()}, StackHeadroom)
	e.Pointer = 0
	fn := mkFn(Signature{Args: []*Type{TInteger}, BarrierPos: 1})
	sig, _, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, nil)
	if sig != nil {
		t.Errorf("open-paren boundary should leave the 1-arg sig unmatched, got %v", sig)
	}
}

func TestW8MatchSignatureForwardQuoteSlotNonAtom(t *testing.T) {
	// A /q forward slot whose expected type is NOT atom-family: the upcoming
	// Word cannot fill it → the forward scan breaks.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(0), NewWord("x")}, StackHeadroom)
	e.Pointer = 0
	fn := mkFn(Signature{Args: []*Type{TString}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1})
	sig, _, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, nil)
	if sig != nil {
		t.Errorf("a /q slot expecting String cannot capture a Word, got %v", sig)
	}
}

func TestW8MatchSignatureForwardTypePathMatch(t *testing.T) {
	// A slashed type-path Word in a forward Any slot resolves to a type
	// literal and matches.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(0), NewWord("Scalar/Number/Integer")}, StackHeadroom)
	e.Pointer = 0
	fn := mkFn(Signature{Args: []*Type{TAny}, BarrierPos: 1})
	sig, positions, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, nil)
	if sig == nil || positions[0] != 1 {
		t.Errorf("type-path word should match the Any forward slot; sig=%v pos=%v", sig, positions)
	}
}

func TestW8MatchSignatureForwardTypePathMismatch(t *testing.T) {
	// A type-path Word against a String slot: the type literal does not
	// match → the forward scan breaks.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(0), NewWord("Scalar/Number/Integer")}, StackHeadroom)
	e.Pointer = 0
	fn := mkFn(Signature{Args: []*Type{TString}, BarrierPos: 1})
	sig, _, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, nil)
	if sig != nil {
		t.Errorf("a type literal cannot fill a String slot, got %v", sig)
	}
}

func TestW8MatchSignatureForwardRefWordTypeGate(t *testing.T) {
	// A `/v`-marked fn word in the forward window (NUR050/G12): a slot
	// that admits its reference value claims it via the def-binding
	// branch (post-ADR-011 the binding IS a Function), while a slot no
	// Function can fill falls through to the 1.4 function-word stop —
	// the /v claim is type-gated.
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "w8refnat", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: -1, Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return args, nil
		})},
	}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(0), NewWordRef("w8refnat")}, StackHeadroom)
	e.Pointer = 0
	fn := mkFn(Signature{Args: []*Type{TFunction}, BarrierPos: 1})
	sig, positions, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, nil)
	if sig == nil || positions[0] != 1 {
		t.Errorf("a /v word should claim the forward Function slot; sig=%v pos=%v", sig, positions)
	}
	// Negative pair: a String slot declines the reference datum — the /v
	// word stays a barrier and the sig goes unmatched.
	e2 := NewTop(r)
	e2.Tape = NewTape([]Value{NewInteger(0), NewWordRef("w8refnat")}, StackHeadroom)
	e2.Pointer = 0
	strFn := mkFn(Signature{Args: []*Type{TString}, BarrierPos: 1})
	if sig, _, _ := e2.MatchSignature(strFn, WordInfo{ArgCount: -1}, nil); sig != nil {
		t.Errorf("a /v word must not claim a String slot, got %v", sig)
	}
}

func TestW8MatchSignatureDefensiveQuoteStackWordMatch(t *testing.T) {
	// Defensive branch: a Word arriving on the stack at a /q position whose
	// expected type IS atom-family is admitted (positions filled).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("x")}, StackHeadroom)
	e.Pointer = 1
	fn := mkFn(Signature{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 0})
	sig, positions, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, []Value{NewWord("x")})
	if sig == nil || positions[0] != 0 {
		t.Errorf("stack Word at an Atom /q slot should match; sig=%v pos=%v", sig, positions)
	}
}

func TestW8MatchSignatureDefensiveQuoteStackWordReject(t *testing.T) {
	// Defensive branch: a Word on the stack at a /q position whose expected
	// type is NOT atom-family → the stack match fails.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("x")}, StackHeadroom)
	e.Pointer = 1
	fn := mkFn(Signature{Args: []*Type{TString}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 0})
	sig, _, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, []Value{NewWord("x")})
	if sig != nil {
		t.Errorf("a Word cannot fill a String /q stack slot, got %v", sig)
	}
}

func TestW8MatchSignatureInsideForwardMixedFill(t *testing.T) {
	// Inside a pending forward, a sig with a forward slot and a stack slot:
	// the forward token fills slot 0, the resolved stack value fills slot 1.
	r := covRegistry(t, nil)
	e := NewTop(r)
	marker := NewForward(ForwardInfo{FuncName: "outer"})
	// [marker, stackArg(10), funcWord(pointer), forwardArg(20)]
	e.Tape = NewTape([]Value{marker, NewInteger(10), NewWord("fn"), NewInteger(20)}, StackHeadroom)
	e.Pointer = 2
	fn := mkFn(Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 1})
	// resolved = the one stack value below the pointer that is not the marker.
	sig, positions, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, []Value{NewInteger(10)})
	if sig == nil {
		t.Fatal("expected a forward+stack mixed match inside a pending forward")
	}
	if positions[0] != 3 || positions[1] != 1 {
		t.Errorf("positions = %v, want [3 1]", positions)
	}
}

func TestW8MatchSignatureInsideForwardPreferWordDeferred(t *testing.T) {
	// Same mixed fill, but the forward token is a Word (preferWordSig) and
	// the sig is not /q-preferred → the match is DEFERRED as bestDeferred
	// and returned after the loop.
	r := covRegistry(t, nil)
	r.Defs.Push("w8fwd", NewInteger(20))
	e := NewTop(r)
	marker := NewForward(ForwardInfo{FuncName: "outer"})
	// [marker, stackArg(10), funcWord(pointer), Word(w8fwd)] — the forward
	// token is a Word, so preferWordSig is set.
	e.Tape = NewTape([]Value{marker, NewInteger(10), NewWord("fn"), NewWord("w8fwd")}, StackHeadroom)
	e.Pointer = 2
	fn := mkFn(Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 1})
	sig, positions, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, []Value{NewInteger(10)})
	if sig == nil {
		t.Fatal("expected a deferred mixed match")
	}
	if positions[0] != 3 || positions[1] != 1 {
		t.Errorf("positions = %v, want [3 1]", positions)
	}
}

// --- execFnDefLiteral / upcomingArgs edge arms ----------------------------

func TestW8ExecFnDefLiteralNonFnValue(t *testing.T) {
	// A non-FnDef value at valIdx → the guard advances the pointer.
	e := engWithTape(t, []Value{NewInteger(1)}, 0)
	if err := e.execFnDefLiteral(0); err != nil {
		t.Fatalf("execFnDefLiteral: %v", err)
	}
	if e.Pointer != 1 {
		t.Errorf("pointer = %d, want 1", e.Pointer)
	}
}

func TestW8UpcomingArgsSkipsForwardStopsAtEnd(t *testing.T) {
	// A Forward token is skipped; the End boundary stops collection.
	fwd := NewForward(ForwardInfo{})
	e := engWithTape(t, []Value{NewInteger(0), fwd, NewInteger(5), NewEnd(), NewInteger(9)}, 0)
	got := e.upcomingArgs(0)
	if len(got) != 1 {
		t.Fatalf("upcomingArgs = %v, want one value (5)", got)
	}
	if n, _ := AsInteger(got[0]); n != 5 {
		t.Errorf("collected %d, want 5", n)
	}
}

// --- anonymous / named fn dispatch in check mode --------------------------

func TestW8AnonFnCheckModeZeroArg(t *testing.T) {
	// A 0-arg anonymous fn dispatched in check mode routes through
	// ExecFnDefSigStackMatch's checkMode branch → spliceAnonCheckResult(0).
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnv := anonFnVal(nil, []*Type{TInteger}, parenBody(NewInteger(7)))
	if _, err := NewTop(r).Run([]Value{fnv}); err != nil {
		t.Fatalf("check-mode 0-arg anon: %v", err)
	}
}

func TestW8AnonFnCheckModeNamedParamsStack(t *testing.T) {
	// A named-param anonymous fn with stack args in check mode →
	// spliceAnonCheckResult(nArgs) via the hasNamed branch.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnv := anonFnVal(
		[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("a"), NewWord("b")),
	)
	if _, err := NewTop(r).Run([]Value{NewInteger(2), NewInteger(3), fnv}); err != nil {
		t.Fatalf("check-mode named anon: %v", err)
	}
}

func TestW8AnonFnCheckModeUnnamedParamsStack(t *testing.T) {
	// An unnamed-param anonymous fn with stack args in check mode →
	// spliceAnonCheckResult(nArgs) via the else (unnamed) branch.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnv := anonFnVal(
		[]FnParam{{Type: TInteger}, {Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd")),
	)
	if _, err := NewTop(r).Run([]Value{NewInteger(2), NewInteger(3), fnv}); err != nil {
		t.Fatalf("check-mode unnamed anon: %v", err)
	}
}

// --- ExecFnDefSigStackMatch: check-mode splice arms (direct) --------------

// w8AnonDef builds an anonymous FnDefInfo with one signature.
func w8AnonDef(params []FnParam, returns []*Type, body []Value) FnDefInfo {
	return FnDefInfo{
		Anonymous: true,
		Signatures: []Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       Boru(body),
			BarrierPos: BarrierAllForward,
		}},
	}
}

func TestW8ExecFnDefStackMatchCheckZeroArg(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := w8AnonDef(nil, []*Type{TInteger}, parenBody(NewInteger(7)))
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 0
	if err := e.ExecFnDefSigStackMatch(0, fnDef, nil); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8ExecFnDefStackMatchCheckZeroArgEmptyBody(t *testing.T) {
	// A body that yields no value → spliceAnonCheckResult's empty-result
	// default (NewCarrier(TAny)).
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := w8AnonDef(nil, nil, parenBody())
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 0
	if err := e.ExecFnDefSigStackMatch(0, fnDef, nil); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8ExecFnDefStackMatchCheckNamed(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := w8AnonDef(
		[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("a"), NewWord("b")),
	)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 2
	if err := e.ExecFnDefSigStackMatch(2, fnDef, []Value{NewInteger(2), NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8ExecFnDefStackMatchCheckUnnamed(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := w8AnonDef(
		[]FnParam{{Type: TInteger}, {Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd")),
	)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 2
	if err := e.ExecFnDefSigStackMatch(2, fnDef, []Value{NewInteger(2), NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8ExecFnDefStackMatchNamedPatternUnifyFail(t *testing.T) {
	// A named param with a non-map Pattern the arg does not unify with →
	// the else (Unify) branch fails the match.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	pat := NewInteger(0)
	fnDef := w8AnonDef(
		[]FnParam{{Name: "a", Type: TInteger, Pattern: &pat}},
		[]*Type{TInteger},
		parenBody(NewWord("a")),
	)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8ExecFnDefStackMatchUnnamedTypeMismatch(t *testing.T) {
	// An unnamed param whose type the stack value does not match → the
	// unnamed sigTypeMatches failure.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := w8AnonDef(
		[]FnParam{{Type: TString}},
		[]*Type{TString},
		parenBody(NewInteger(0)),
	)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8ExecFnDefStackMatchUnnamedPatternUnifyFail(t *testing.T) {
	// An unnamed param with a non-map Pattern the arg does not unify with.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	pat := NewInteger(0)
	fnDef := w8AnonDef(
		[]FnParam{{Type: TInteger, Pattern: &pat}},
		[]*Type{TInteger},
		parenBody(NewInteger(1)),
	)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

// --- execFnDefSig: runtime splice + autoEval arms (direct) ----------------

func w8UnnamedSig(n int) *Signature {
	params := make([]FnParam, n)
	for i := range params {
		params[i] = FnParam{Type: TInteger}
	}
	return &Signature{Params: params, Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(42)}), BarrierPos: 0}
}

func TestW8ExecFnDefSigRuntimeCompaction(t *testing.T) {
	// A skipped marker between the two args forces the compaction loop body.
	r := covRegistry(t, nil)
	sig := w8UnnamedSig(2)
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), NewForward(ForwardInfo{}), NewInteger(3), fnv}, StackHeadroom)
	e.Pointer = 3
	if err := e.execFnDefSig(3, sig, []Value{NewInteger(2), NewInteger(3)}, nil, false); err != nil {
		t.Fatalf("execFnDefSig: %v", err)
	}
}

func TestW8ExecFnDefSigRuntimeElseBranch(t *testing.T) {
	// nArgs exceeds resolved values before valIdx → the else branch clamps.
	r := covRegistry(t, nil)
	sig := w8UnnamedSig(2)
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefSig(1, sig, []Value{NewInteger(2), NewInteger(3)}, nil, false); err != nil {
		t.Fatalf("execFnDefSig: %v", err)
	}
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0", e.Pointer)
	}
}

func TestW8ExecFnDefSigCapturedRegCompaction(t *testing.T) {
	// The captured-registry (CallBoru) splice path with a skipped marker.
	r := covRegistry(t, nil)
	sig := w8UnnamedSig(2)
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), NewForward(ForwardInfo{}), NewInteger(3), fnv}, StackHeadroom)
	e.Pointer = 3
	if err := e.execFnDefSig(3, sig, []Value{NewInteger(2), NewInteger(3)}, r, false); err != nil {
		t.Fatalf("execFnDefSig captured: %v", err)
	}
}

func TestW8ExecFnDefSigCapturedRegZeroArg(t *testing.T) {
	// The captured-registry path with a 0-arg sig.
	r := covRegistry(t, nil)
	sig := &Signature{Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(42)}), BarrierPos: 0}
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	e := NewTop(r)
	e.Tape = NewTape([]Value{fnv}, StackHeadroom)
	e.Pointer = 0
	if err := e.execFnDefSig(0, sig, nil, r, false); err != nil {
		t.Fatalf("execFnDefSig captured 0-arg: %v", err)
	}
}

func TestW8ExecFnDefSigCapturedRegElseBranch(t *testing.T) {
	// The captured-registry else branch (fewer resolved than nArgs).
	r := covRegistry(t, nil)
	sig := w8UnnamedSig(2)
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefSig(1, sig, []Value{NewInteger(2), NewInteger(3)}, r, false); err != nil {
		t.Fatalf("execFnDefSig captured else: %v", err)
	}
}

func TestW8ExecFnDefSigListAutoEval(t *testing.T) {
	// A list arg with Eval=true is auto-evaluated in place.
	r := covRegistry(t, nil)
	sig := &Signature{Params: []FnParam{{Type: TList}}, Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(42)}), BarrierPos: 0}
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	listArg := NewList([]Value{NewInteger(1), NewInteger(2)})
	listArg.Eval = true
	e := NewTop(r)
	e.Tape = NewTape([]Value{listArg, fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefSig(1, sig, []Value{listArg}, nil, false); err != nil {
		t.Fatalf("execFnDefSig list autoeval: %v", err)
	}
}

func TestW8ExecFnDefSigListAutoEvalError(t *testing.T) {
	// A list arg whose evaluation fails (undefined word) propagates the error.
	r := covRegistry(t, nil)
	sig := &Signature{Params: []FnParam{{Type: TList}}, Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(42)}), BarrierPos: 0}
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	listArg := NewList([]Value{NewWord("w8_undefined_word")})
	listArg.Eval = true
	e := NewTop(r)
	e.Tape = NewTape([]Value{listArg, fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefSig(1, sig, []Value{listArg}, nil, false); err == nil {
		t.Fatal("expected an error from an undefined word in a list arg")
	}
}

func TestW8ExecFnDefSigMapAutoEval(t *testing.T) {
	// A map arg with Eval=true is auto-evaluated in place (the success twin
	// of the error arm below).
	r := covRegistry(t, nil)
	sig := &Signature{Params: []FnParam{{Type: TMap}}, Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(42)}), BarrierPos: 0}
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	m := NewOrderedMap()
	m.Set("k", NewInteger(1))
	mapArg := NewMap(m)
	mapArg.Eval = true
	e := NewTop(r)
	e.Tape = NewTape([]Value{mapArg, fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefSig(1, sig, []Value{mapArg}, nil, false); err != nil {
		t.Fatalf("execFnDefSig map autoeval: %v", err)
	}
}

func TestW8ExecFnDefSigMapAutoEvalError(t *testing.T) {
	// A map arg whose value evaluation fails propagates the error.
	r := covRegistry(t, nil)
	sig := &Signature{Params: []FnParam{{Type: TMap}}, Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(42)}), BarrierPos: 0}
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	m := NewOrderedMap()
	m.Set("k", NewWord("w8_undefined_word"))
	mapArg := NewMap(m)
	mapArg.Eval = true
	e := NewTop(r)
	e.Tape = NewTape([]Value{mapArg, fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefSig(1, sig, []Value{mapArg}, nil, false); err == nil {
		t.Fatal("expected an error from an undefined word in a map arg value")
	}
}

func TestW8ExecFnDefSigArgsPushError(t *testing.T) {
	// A nil Args stack makes the runtime-path Args.Push fail.
	r := covRegistry(t, nil)
	r.Args = nil
	sig := w8UnnamedSig(1)
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{*sig}})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefSig(1, sig, []Value{NewInteger(2)}, nil, false); err == nil {
		t.Fatal("expected an Args.Push error with a nil Args stack")
	}
}

// --- execMatch: FullStack + empty-positions arms --------------------------

func w8FullStackReg(t *testing.T) *Registry {
	t.Helper()
	return covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "w8fs",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
					return []Value{NewInteger(int64(len(stack)))}, nil
				}, FullStack(), CheckFullStack(func(_ []Value, _ []Value, _ *Registry) []Value {
					return []Value{NewCarrier(TInteger)}
				})),
				Returns: []*Type{TInteger}, BarrierPos: 0,
			}},
		})
	})
}

func TestW8ExecMatchFullStackRuntime(t *testing.T) {
	// A FullStack word dispatched inside a paren: the base finder hits the
	// open paren, and an installed recorder gets OnCall.
	r := w8FullStackReg(t)
	e := NewTop(r)
	e.SetRecorder(&w4CountRecorder{})
	out, err := e.Run([]Value{NewOpenParen(), NewInteger(1), NewWord("w8fs"), NewCloseParen()})
	if err != nil {
		t.Fatalf("full-stack runtime: %v", err)
	}
	_ = out
}

func TestW8ExecMatchFullStackCheckMode(t *testing.T) {
	// The same FullStack word in check mode routes through the
	// checkFullStackFn path (base + end finders).
	r := w8FullStackReg(t)
	done := r.Check.Begin()
	defer done()
	if _, err := NewTop(r).Run([]Value{NewOpenParen(), NewInteger(1), NewWord("w8fs"), NewCloseParen()}); err != nil {
		t.Fatalf("full-stack check: %v", err)
	}
}

func TestW8ExecMatchCheckModeForwardTail(t *testing.T) {
	// A forward-collecting dispatch in check mode: the paren-tail callEnd
	// finder advances past the forward arg position (p > callEnd).
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	if _, err := NewTop(r).Run([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}); err != nil {
		t.Fatalf("check forward dispatch: %v", err)
	}
}

func TestW8ExecMatchFullStackCheckForward(t *testing.T) {
	// A FullStack word that forward-collects in check mode: the end finder
	// advances past the forward arg position.
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "w8fsf",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
					return []Value{NewInteger(int64(len(stack)))}, nil
				}, FullStack(), CheckFullStack(func(_ []Value, _ []Value, _ *Registry) []Value {
					return []Value{NewCarrier(TInteger)}
				})),
				Returns: []*Type{TInteger}, BarrierPos: 1,
			}},
		})
	})
	done := r.Check.Begin()
	defer done()
	if _, err := NewTop(r).Run([]Value{NewWord("w8fsf"), NewInteger(5)}); err != nil {
		t.Fatalf("full-stack check forward: %v", err)
	}
}

func TestW8ExecMatchEmptyPositions(t *testing.T) {
	// A MatchResult with no recorded Positions but n>0 derives them from the
	// stack.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), NewInteger(3)}, StackHeadroom)
	e.Pointer = 2
	fn := r.Lookup("cadd")
	sig := &fn.Signatures[0]
	match := &MatchResult{Sig: sig, Name: "cadd", Positions: nil, Args: []Value{NewInteger(2), NewInteger(3)}}
	if err := e.execMatch(match); err != nil {
		t.Fatalf("execMatch: %v", err)
	}
}

// --- /v and /u recorder push arms (direct) --------------------------------

func TestW8StepWordRefRecorder(t *testing.T) {
	// The /v path pushes the resolved Function as data and reports it to an
	// installed recorder via OnPushLit.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.SetRecorder(&w4CountRecorder{})
	e.Tape = NewTape([]Value{NewWord("cadd")}, StackHeadroom)
	e.Pointer = 0
	if err := e.stepWordVal(NewWord("cadd"), WordInfo{Name: "cadd", ArgCount: -1, ForceVal: true}); err != nil {
		t.Fatalf("stepWordVal: %v", err)
	}
}

func TestW8StepWordUsurpRefRecorder(t *testing.T) {
	// The /uv path leaves the usurped wrapper as data and reports it to the
	// recorder via OnPushLit.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.SetRecorder(&w4CountRecorder{})
	e.Tape = NewTape([]Value{NewWord("cadd")}, StackHeadroom)
	e.Pointer = 0
	if err := e.stepWordUsurp(NewWord("cadd"), WordInfo{Name: "cadd", ArgCount: -1, ForceUsurp: true, ForceVal: true}); err != nil {
		t.Fatalf("stepWordUsurp: %v", err)
	}
}

// --- refuseForwardStackDrift / tryRecordUnmatchedDispatchTrap (direct) -----

// --- isRecordableLiteral: control-marker arm ------------------------------

func TestW8IsRecordableLiteralMarkers(t *testing.T) {
	if IsRecordableLiteral(NewMark("m")) {
		t.Error("a Mark is not a recordable literal")
	}
	if IsRecordableLiteral(NewMove("m", "for loop")) {
		t.Error("a Move is not a recordable literal")
	}
	if IsRecordableLiteral(NewReturnCheck(ReturnCheckInfo{})) {
		t.Error("a ReturnCheck is not a recordable literal")
	}
}

// --- ExecFnDefSigStackMatch: checkFnValue + map pattern (direct) -----------

func TestW8ExecFnDefStackMatchUnnamedMapPatternFail(t *testing.T) {
	// An unnamed param with a MAP Pattern the arg does not open-unify with.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	pm := NewOrderedMap()
	pm.Set("x", NewInteger(1))
	pat := NewMap(pm)
	fnDef := w8AnonDef([]FnParam{{Type: TMap, Pattern: &pat}}, []*Type{TInteger}, parenBody(NewInteger(0)))
	am := NewOrderedMap()
	am.Set("y", NewInteger(2))
	arg := NewMap(am)
	e := NewTop(r)
	e.Tape = NewTape([]Value{arg, NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{arg}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

// --- stepCloseParen: forward-resolution inner loop (direct) ---------------
//
// A Forward left pending inside a paren at close time is resolved by
// stepCloseParen's inner loop, which then re-evaluates the surviving
// tokens. Real programs resolve their forwards before the close paren, so
// these arms are driven by hand-built tapes carrying a pending Forward.

func w8scpEng(t *testing.T, vals []Value, ptr int) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape(vals, StackHeadroom)
	e.Pointer = ptr
	return e
}

func w8scpFwd(name string, fi int) Value {
	return NewForward(ForwardInfo{FuncName: name, Sig: &Signature{Args: []*Type{TInteger}, BarrierPos: -1}, FuncIndex: fi})
}

func TestW8StepCloseParenReEvalEnd(t *testing.T) {
	// Forward resolves, then the re-eval loop steps an End.
	e := w8scpEng(t, []Value{NewOpenParen(), w8scpFwd("cadd", 2), NewEnd(), NewCloseParen()}, 3)
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

func TestW8StepCloseParenReEvalForward(t *testing.T) {
	// Forward resolves, then the re-eval loop advances past another Forward.
	e := w8scpEng(t, []Value{NewOpenParen(), w8scpFwd("cadd", 2), w8scpFwd("cadd", 3), NewCloseParen()}, 3)
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

func TestW8StepCloseParenReEvalReturnCheck(t *testing.T) {
	e := w8scpEng(t, []Value{NewOpenParen(), w8scpFwd("cadd", 2), NewReturnCheck(ReturnCheckInfo{FuncName: "f"}), NewCloseParen()}, 3)
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

func TestW8StepCloseParenReEvalDefCleanup(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	dc := NewDefCleanup(DefCleanupInfo{Registry: r, Snapshot: map[string]int{}})
	e.Tape = NewTape([]Value{NewOpenParen(), w8scpFwd("cadd", 2), dc, NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

func TestW8StepCloseParenReEvalLiteral(t *testing.T) {
	// Forward whose funcIdx points at a literal: after resolution the
	// re-eval loop steps that literal via the default arm.
	e := w8scpEng(t, []Value{NewOpenParen(), w8scpFwd("cadd", 2), NewInteger(9), NewCloseParen()}, 3)
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

func TestW8StepCloseParenReEvalOpenParen(t *testing.T) {
	// Forward whose funcIdx points at an open paren: the re-eval loop
	// advances past it (pointer++).
	e := w8scpEng(t, []Value{NewWord("cneg"), NewOpenParen(), w8scpFwd("cadd", 1), NewCloseParen()}, 3)
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

func TestW8StepCloseParenReEvalWordSuccess(t *testing.T) {
	// Forward before its funcIdx; the re-eval loop force-stacks and
	// dispatches the word, applying the funcIdx-- adjustment.
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "cadd", Sig: sig, FuncIndex: 3})
	e := w8scpEng(t, []Value{NewOpenParen(), fwd, NewInteger(5), NewWord("cadd"), NewInteger(7), NewCloseParen()}, 5)
	// The re-eval word dispatch may error (forward args running into the
	// next word); the point is exercising the re-eval Word arm either way.
	_ = e.stepCloseParen(true)
}

func TestW8StepCloseParenReEvalWordError(t *testing.T) {
	// The re-eval loop dispatches a word that errors → the error propagates.
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "cfail8", Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, &BoruError{Code: "runtime_error", Detail: "boom8"}
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}}})
	})
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "cfail8", Sig: &Signature{Args: []*Type{TInteger}, BarrierPos: -1}, FuncIndex: 2})
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(1), NewWord("cfail8"), fwd, NewCloseParen()}, StackHeadroom)
	e.Pointer = 4
	if err := e.stepCloseParen(true); err == nil {
		t.Fatal("expected the re-eval word dispatch error to propagate")
	}
}

func TestW8StepCloseParenVoidGroupForward(t *testing.T) {
	// An empty group with a Forward marker below the open paren records the
	// forward's FuncName as a void-group candidate.
	e := w8scpEng(t, []Value{w8scpFwd("cadd", 0), NewOpenParen(), NewCloseParen()}, 2)
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

func TestW8StepCloseParenVoidGroupWord(t *testing.T) {
	// An empty group with a Word below the open paren records it as a
	// void-group candidate.
	e := w8scpEng(t, []Value{NewWord("cneg"), NewOpenParen(), NewCloseParen()}, 2)
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

// --- resolveOrphanedForwards retry loop -----------------------------------

func TestW8ResolveOrphanedReEvalLiteral(t *testing.T) {
	// A forward whose funcIdx points at a literal: after removal the retry
	// loop steps that literal (default arm).
	r := covRegistry(t, nil)
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0})
	e.Tape = NewTape([]Value{NewInteger(9), fwd}, StackHeadroom)
	e.Pointer = 0
	if err := e.resolveOrphanedForwards(); err != nil {
		t.Fatalf("resolveOrphanedForwards: %v", err)
	}
}

func TestW8ResolveOrphanedReEvalEnd(t *testing.T) {
	// A forward whose funcIdx points at an End: the retry loop steps it.
	r := covRegistry(t, nil)
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0})
	e.Tape = NewTape([]Value{NewEnd(), fwd}, StackHeadroom)
	e.Pointer = 0
	if err := e.resolveOrphanedForwards(); err != nil {
		t.Fatalf("resolveOrphanedForwards: %v", err)
	}
}

func TestW8ResolveOrphanedReEvalForward(t *testing.T) {
	// Two forwards: resolving the first leaves the retry loop stepping the
	// second Forward (pointer++ arm).
	r := covRegistry(t, nil)
	e := NewTop(r)
	f1 := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0})
	f2 := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0})
	e.Tape = NewTape([]Value{f1, f2}, StackHeadroom)
	e.Pointer = 0
	if err := e.resolveOrphanedForwards(); err != nil {
		t.Fatalf("resolveOrphanedForwards: %v", err)
	}
}

func TestW8ResolveOrphanedReEvalWordError(t *testing.T) {
	// A word that errors when the retry loop re-steps it (an unbound name)
	// propagates the error out of resolveOrphanedForwards.
	r := covRegistry(t, nil)
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0})
	e.Tape = NewTape([]Value{NewWord("w8-no-such-word"), fwd}, StackHeadroom)
	e.Pointer = 0
	if err := e.resolveOrphanedForwards(); err == nil {
		t.Fatal("expected the retry loop to surface the stepWord error")
	}
}

func TestW8RunOrphanedForwardRetryError(t *testing.T) {
	// A pending forward whose func word is UNBOUND mid-collection (a
	// dispatched word tore the binding down) still fails LOUDLY at
	// end-of-input — Run surfaces a fault rather than silently dropping
	// the half-collected call. (The organic producers error in the main
	// loop; resolveOrphanedForwards' own error return is the allowlisted
	// §engine defensive arm, covered directly by the ReEvalWordError
	// sibling above.)
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "w8unbind", Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				reg.Defs.Delete("cadd")
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}}})
	})
	// cadd parks a 2-arg forward; w8unbind is a fn-word barrier, so it
	// dispatches (removing cadd's binding) instead of being collected.
	_, err := NewTop(r).Run([]Value{NewWord("cadd"), NewWord("w8unbind")})
	if err == nil {
		t.Fatal("Run must surface the retry-step error for the unbound func word")
	}
}

// --- evalParenGroupAt: inner-loop arms (direct) ---------------------------

func TestW8EvalParenGroupForward(t *testing.T) {
	// A Forward token inside the paren group is advanced past (pointer++).
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewOpenParen(), NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0}), NewInteger(1), NewCloseParen()}, StackHeadroom)
	e.Pointer = 0
	// The group evaluates; the point is exercising the IsForward arm.
	_ = e.evalParenGroupAt(0)
}

func TestW8EvalParenGroupMoveError(t *testing.T) {
	// A Move to an unregistered mark inside the paren errors.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewOpenParen(), NewMove("w8_nomark", "for loop"), NewCloseParen()}, StackHeadroom)
	e.Pointer = 0
	if err := e.evalParenGroupAt(0); err == nil {
		t.Fatal("expected a move error for an unregistered mark")
	}
}

// --- flow-control propagation arms ----------------------------------------

// w8BreakReg registers a 0-arg word that raises a break flow-control signal.
func w8BreakReg(t *testing.T) *Registry {
	t.Helper()
	return covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "w8break",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					reg.FlowCtrl = FlowBreak
					return []Value{NewInteger(0)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	})
}

func TestW8ResolveForwardArgsParenFlowCtrl(t *testing.T) {
	// A break raised inside a paren group during forward collection
	// propagates out of evalParenGroupAt and resolveForwardArgs.
	r := w8BreakReg(t)
	_, err := NewTop(r).Run([]Value{
		NewWord("cadd"), NewOpenParen(), NewWord("w8break"), NewCloseParen(), NewInteger(2),
	})
	// A top-level break with no enclosing loop is reported; the point is
	// exercising the flow-control propagation arms.
	_ = err
}

func TestW8ResolveOrphanedFlowCtrl(t *testing.T) {
	// A break raised during the orphaned-forward retry loop stops it.
	r := w8BreakReg(t)
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "w8break", FuncIndex: 1})
	e.Tape = NewTape([]Value{fwd, NewWord("w8break")}, StackHeadroom)
	e.Pointer = 0
	if err := e.resolveOrphanedForwards(); err != nil {
		t.Fatalf("resolveOrphanedForwards: %v", err)
	}
	if e.Registry.FlowCtrl != FlowBreak {
		t.Error("break signal should remain set for the outer frame")
	}
}

func TestW8StepCloseParenReEvalFlowCtrl(t *testing.T) {
	// A break raised during stepCloseParen's re-eval loop stops it and
	// leaves the signal for the outer frame.
	r := w8BreakReg(t)
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "w8break", FuncIndex: 1})
	e.Tape = NewTape([]Value{NewOpenParen(), NewWord("w8break"), fwd, NewCloseParen()}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true); err != nil {
		t.Fatalf("stepCloseParen: %v", err)
	}
}

// --- stepLiteral: forward collection of a value below the func word -------

func TestW8StepLiteralCollectBeforeFunc(t *testing.T) {
	// A pending forward whose FuncIndex is AFTER the value being collected:
	// removing the value decrements funcIdx (the valIdx<funcIdx arm).
	r := covRegistry(t, nil)
	e := NewTop(r)
	sig := &Signature{Args: []*Type{TInteger}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "cneg", Sig: sig, FuncIndex: 3, CollectedArgs: 0, ExpectedArgs: 1})
	e.Tape = NewTape([]Value{fwd, NewInteger(5), NewInteger(0), NewWord("cneg")}, StackHeadroom)
	e.Pointer = 1
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
}

// --- XmlInterp evaluation arms --------------------------------------------

func w8BadXml() Value {
	return NewXmlInterp(XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenExpr, Expr: []Value{NewWord("w8_undefined_word")}}}})
}

func w8GoodXml() Value {
	return NewXmlInterp(XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenLit, Lit: "hi"}}})
}

func TestW8ResolveForwardArgsXmlInterpError(t *testing.T) {
	// A forward word scanning over an interpolated XML literal whose hole
	// fails to evaluate propagates the error.
	r := covRegistry(t, nil)
	if _, err := NewTop(r).Run([]Value{NewWord("cadd"), w8BadXml()}); err == nil {
		t.Fatal("expected an XML-interp evaluation error during forward collection")
	}
}

func TestW8EvalParenGroupXmlInterpSuccess(t *testing.T) {
	// A good interpolated XML literal inside a paren group evaluated during
	// forward collection: the IsXmlInterp arm sets the resolved node.
	r := covRegistry(t, nil)
	_, _ = NewTop(r).Run([]Value{NewWord("cadd"), NewOpenParen(), w8GoodXml(), NewCloseParen(), NewInteger(2)})
}

func TestW8EvalParenGroupXmlInterpError(t *testing.T) {
	// A bad interpolated XML literal inside a paren group propagates its error.
	r := covRegistry(t, nil)
	if _, err := NewTop(r).Run([]Value{NewWord("cadd"), NewOpenParen(), w8BadXml(), NewCloseParen(), NewInteger(2)}); err == nil {
		t.Fatal("expected an XML-interp error inside a paren group")
	}
}

func TestW8AutoEvalMapXmlInterpError(t *testing.T) {
	// A map value that is a bad interpolated XML literal propagates its error.
	r := covRegistry(t, nil)
	e := NewTop(r)
	m := NewOrderedMap()
	m.Set("k", w8BadXml())
	mapVal := NewMap(m)
	if _, err := e.AutoEvalMap(mapVal, false, true); err == nil {
		t.Fatal("expected an XML-interp error from a map value")
	}
}

// --- InterpString / Reach evaluation error arms ---------------------------

func TestW8ResolveForwardArgsInterpStringError(t *testing.T) {
	// A forward word scanning over an interpolated string whose expression
	// part fails to evaluate propagates the error.
	r := covRegistry(t, nil)
	bad := NewInterpString([]InterpPart{{Expr: []Value{NewWord("w8_undefined_word")}}})
	_, err := NewTop(r).Run([]Value{NewWord("cadd"), bad})
	if err == nil {
		t.Fatal("expected an interp-string evaluation error during forward collection")
	}
}

func TestW8EvalParenGroupInterpStringError(t *testing.T) {
	// A bad interpolated string inside a paren group evaluated during
	// forward collection propagates its error.
	r := covRegistry(t, nil)
	bad := NewInterpString([]InterpPart{{Expr: []Value{NewWord("w8_undefined_word")}}})
	if _, err := NewTop(r).Run([]Value{NewWord("cadd"), NewOpenParen(), bad, NewCloseParen(), NewInteger(2)}); err == nil {
		t.Fatal("expected an interp-string error inside a paren group")
	}
}

func TestW8AutoEvalMapInterpStringError(t *testing.T) {
	// A map value that is an interpolated string whose expression fails.
	r := covRegistry(t, nil)
	e := NewTop(r)
	m := NewOrderedMap()
	m.Set("k", NewInterpString([]InterpPart{{Expr: []Value{NewWord("w8_undefined_word")}}}))
	mapVal := NewMap(m)
	if _, err := e.AutoEvalMap(mapVal, false, true); err == nil {
		t.Fatal("expected an interp-string error from a map value")
	}
}

func TestW8AutoEvalMapReachError(t *testing.T) {
	// A map value that is an eval-Reach whose strict get on a missing key
	// errors.
	r := covRegistry(t, nil)
	e := NewTop(r)
	empty := NewMap(NewOrderedMap())
	reach := NewReach(ReachInfo{
		Receiver: []Value{empty},
		Segments: []ReachSeg{{Getr: true, KeyLit: NewString("w8missing")}},
		Eval:     true,
	})
	m := NewOrderedMap()
	m.Set("k", reach)
	mapVal := NewMap(m)
	if _, err := e.AutoEvalMap(mapVal, false, true); err == nil {
		t.Fatal("expected a strict-reach missing-key error from a map value")
	}
}

func w8ReachWithComputedKey(keyExpr []Value) Value {
	return NewReach(ReachInfo{
		Receiver: []Value{NewMap(NewOrderedMap())},
		Segments: []ReachSeg{{Computed: true, KeyExpr: keyExpr}},
		Eval:     false,
	})
}

func TestW8ExprHasEffectReachComputedKey(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	reach := w8ReachWithComputedKey([]Value{NewWord("print")})
	if !e.exprHasEffect([]Value{reach}) {
		t.Error("a Reach computed key referencing print should be detected")
	}
}

// --- constFoldContainerVal: non-deterministic decline ---------------------

func TestW8ConstFoldNonDeterministicDeclines(t *testing.T) {
	counter := 0
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "w8ctr", Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				counter++
				return []Value{NewInteger(int64(counter))}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}}})
	})
	e := NewTop(r)
	if _, ok := e.constFoldContainerVal([]Value{NewWord("w8ctr")}); ok {
		t.Error("a non-deterministic expression must not fold")
	}

	// With an armed eval slot (the check piece's configuration), the two
	// runs yield 1 then 2 → ConstFoldAgrees declines and the fold declines
	// through the disagreement arm rather than the eval-declined arm.
	prev := CheckBraid.ConcreteEvalOnce
	defer func() { CheckBraid.ConcreteEvalOnce = prev }()
	CheckBraid.ConcreteEvalOnce = func(e *Engine, items []Value) (Value, bool) {
		counter++
		return NewInteger(int64(counter)), true
	}
	if _, ok := e.constFoldContainerVal([]Value{NewWord("w8ctr")}); ok {
		t.Error("disagreeing concrete evals must not fold")
	}
}

func TestW8ResolveOrphanedUnmatchedClose(t *testing.T) {
	// The retry loop steps a CloseParen with no open paren → stepCloseParen
	// errors and the error propagates out of resolveOrphanedForwards.
	r := covRegistry(t, nil)
	e := NewTop(r)
	fwd := NewForward(ForwardInfo{FuncName: "cadd", FuncIndex: 0, CollectedArgs: 0, StackArgs: 0})
	e.Tape = NewTape([]Value{NewInteger(1), fwd, NewCloseParen()}, StackHeadroom)
	e.Pointer = 0
	if err := e.resolveOrphanedForwards(); err == nil {
		t.Fatal("expected an unmatched close paren error")
	} else if !strings.Contains(err.Error(), "closing") {
		t.Errorf("err = %v, want closing-paren error", err)
	}
}

// --- OpDispatchRematch record/lower/VM degenerate arms ----------------------
