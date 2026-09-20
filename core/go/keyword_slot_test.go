package core

// KEYWORD slots: a /q signature position carrying a concrete Atom
// pattern admits exactly one literal word, matched binding-agnostically
// by token NAME (patternsOk) — the mechanism behind def's
// `[name/q fn/q sigs]` form. These tests pin the matcher branch and the
// plan-scan capture gate (capturesForwardToken) directly, mirroring the
// white-box style of engine_seam7_barrier_test.go.

import "testing"

// kwSig builds a def-shaped keyword signature: [Atom/q, Atom/q=lit, List].
func kwSig(lit string) *Signature {
	return &Signature{
		Args:       []*Type{TAtom, TAtom, TList},
		QuoteArgs:  map[int]bool{0: true, 1: true},
		Patterns:   map[int]Value{1: NewAtom(lit)},
		BarrierPos: -1,
	}
}

// --- patternsOk keyword branch ---------------------------------------------

// patternsTape lays out [name-word, tok] and reports patternsOk with both
// positions forward-matched.
func kwPatternsOk(t *testing.T, sig *Signature, tok Value) bool {
	t.Helper()
	tape := NewTape([]Value{NewWord("g"), tok}, StackHeadroom)
	return patternsOk(sig, []int{0, 1}, tape, 2, nil)
}

func TestKeywordSlotMatchesLiteralWord(t *testing.T) {
	// The raw token NAME matches — even though `fn`-like words are
	// normally Defs-bound to a dispatching value, /q capture is
	// binding-agnostic and the pattern must never see the binding.
	if !kwPatternsOk(t, kwSig("fn"), NewWord("fn")) {
		t.Error("keyword slot must match the literal word token by name")
	}
}

func TestKeywordSlotMatchesAtom(t *testing.T) {
	// A stack/arrival position may already hold the converted Atom.
	if !kwPatternsOk(t, kwSig("fn"), NewAtom("fn")) {
		t.Error("keyword slot must match an Atom of the pattern's name")
	}
}

func TestKeywordSlotRejectsOtherWord(t *testing.T) {
	if kwPatternsOk(t, kwSig("fn"), NewWord("gen")) {
		t.Error("keyword slot must reject a word with a different name")
	}
}

func TestKeywordSlotRejectsNonWord(t *testing.T) {
	// A value (a pre-evaluated group's Function, a literal) can never
	// satisfy a keyword slot.
	if kwPatternsOk(t, kwSig("fn"), NewInteger(7)) {
		t.Error("keyword slot must reject a non-word, non-atom value")
	}
}

// --- capturesForwardToken ----------------------------------------------------

func kwFnDef(lit string) *FnDefInfo {
	return &FnDefInfo{Signatures: []Signature{*kwSig(lit)}}
}

func TestCapturesForwardTokenKeywordMatch(t *testing.T) {
	if !capturesForwardToken(kwFnDef("fn"), 1, NewWord("fn")) {
		t.Error("the matching keyword token is captured, not a barrier")
	}
}

func TestCapturesForwardTokenKeywordMiss(t *testing.T) {
	if capturesForwardToken(kwFnDef("fn"), 1, NewWord("add")) {
		t.Error("a non-matching word keeps its barrier treatment")
	}
}

func TestCapturesForwardTokenUnpatternedQuote(t *testing.T) {
	// Position 0 is a plain /q slot — captures ANY word, as before.
	if !capturesForwardToken(kwFnDef("fn"), 0, NewWord("anything")) {
		t.Error("an unpatterned /q slot captures unconditionally")
	}
}

func TestCapturesForwardTokenRawSlot(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{{
		Args:       []*Type{TAny},
		RawParens:  map[int]bool{0: true},
		BarrierPos: -1,
	}}}
	if !capturesForwardToken(fn, 0, NewWord("w")) {
		t.Error("raw/form/type slots capture unconditionally")
	}
}

func TestCapturesForwardTokenNoCapture(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{{
		Args:       []*Type{TInteger},
		BarrierPos: -1,
	}}}
	if capturesForwardToken(fn, 0, NewWord("w")) {
		t.Error("a plain typed slot never captures a word structurally")
	}
}

// --- DispatchSig -------------------------------------------------------------

func TestDispatchSigNoImplementation(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A match-only shape (no Impl) must decline loudly, not panic.
	if _, err := DispatchSig(&Signature{Args: []*Type{TAny}}, nil, r); err == nil {
		t.Fatal("a signature with no run implementation must error")
	}
}

// --- scanMacroOperands paren folding -----------------------------------------

func macroFnDef(arity int) *FnDefInfo {
	params := make([]FnParam, arity)
	for i := range params {
		params[i] = FnParam{Type: TAny}
	}
	return &FnDefInfo{Name: "m", Macro: true,
		Signatures: []Signature{{Params: params}}}
}

func TestScanMacroOperandsFoldsNestedGroup(t *testing.T) {
	// (1 (2) 3): the fold tracks nesting depth and captures the whole
	// balanced span as ONE ParenExpr operand.
	e := engWithTape(t, []Value{
		NewWord("m"), NewOpenParen(), NewInteger(1),
		NewOpenParen(), NewInteger(2), NewCloseParen(),
		NewInteger(3), NewCloseParen(),
	}, 0)
	ops, last, err := e.scanMacroOperands(macroFnDef(1), 1, 0)
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if len(ops) != 1 || !IsParenExpr(ops[0]) {
		t.Fatalf("want one ParenExpr operand, got %v", ops)
	}
	if last != 7 {
		t.Errorf("operand span must end at the close paren, got %d", last)
	}
	toks, _ := AsParenExpr(ops[0])
	// 1 ( 2 ) 3 — the nested markers ride inside; the outer pair is
	// consumed by the fold.
	if len(toks) != 5 {
		t.Errorf("inner tokens preserved incl. nested markers, got %d", len(toks))
	}
}

func TestScanMacroOperandsUnbalancedGroupIsBoundary(t *testing.T) {
	// An unbalanced open paren cannot be an operand form — it is a
	// structural boundary, so the macro is short an operand.
	e := engWithTape(t, []Value{
		NewWord("m"), NewOpenParen(), NewInteger(1),
	}, 0)
	if _, _, err := e.scanMacroOperands(macroFnDef(1), 1, 0); err == nil {
		t.Fatal("unbalanced group must leave the macro short of operands")
	}
}

// --- matchSignature: infix-form-inside-forward with a mismatched stack arg ---

// TestMatchSignatureInsideForwardStackMismatch drives the arm where a
// word dispatching INSIDE a pending forward has forward-collected some
// args (fwd>0) but a remaining stack operand mismatches the sig type,
// so the mixed forward+stack split is rejected (canStack=false). A
// Forward marker parked above the pointer makes isInsidePendingForward
// true; the forward token fills sig[0]; the mismatched resolved value
// fails sig[1].
func TestMatchSignatureInsideForwardStackMismatch(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	// tape[0] = a parked Forward (an outer word still collecting), so
	// isInsidePendingForward() is true at pointer 1. tape[2] is the
	// forward arg that fills sig[0].
	e.Tape = NewTape([]Value{
		fwdMarker("outer", 0, 1, 0),
		NewWord("inner"),
		NewInteger(1),
	}, StackHeadroom)
	e.Pointer = 1
	fn := mkFn(Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 2})
	// resolved carries one stack value of the WRONG type for sig[1].
	sig, _, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, []Value{NewBoolean(true)})
	if sig != nil {
		t.Errorf("a mismatched stack operand must reject the swap split, got %v", sig)
	}
}

// TestMatchSignatureInsideForwardPatternReject drives the pattern GATE of
// that same infix-form-inside-forward arm: the remaining stack operand
// type-matches (canStack=true), so the split is filled — but the sig
// carries a value Pattern that the forward-collected position VIOLATES,
// so patternsOk rejects the selection exactly like the full-forward and
// normal-stack returns. Under the shipped strict barrier the enclosing
// pending forward strands before a bare word can dispatch into it, so
// this gate is only reachable by a direct dispatch call; the white-box
// call pins it. sig[0] carries a literal-0 Pattern; the forward token is
// 3, which fails it while the Integer stack operand satisfies sig[1].
func TestMatchSignatureInsideForwardPatternReject(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	// tape[0] is a resolved stack Integer (fills sig[1] from the stack via
	// resolvedIdx); tape[1] is the parked outer Forward so
	// isInsidePendingForward() is true at pointer 2; tape[3] is the
	// forward token that fills sig[0] and violates its literal-0 pattern.
	e.Tape = NewTape([]Value{
		NewInteger(5),
		fwdMarker("outer", 0, 1, 0),
		NewWord("inner"),
		NewInteger(3),
	}, StackHeadroom)
	e.Pointer = 2
	fn := mkFn(Signature{
		Args:       []*Type{TInteger, TInteger},
		Patterns:   map[int]Value{0: NewInteger(0)},
		BarrierPos: 2,
	})
	// resolved carries one stack value of the RIGHT type for sig[1], so
	// canStack holds and the split is filled — only the pattern rejects.
	sig, _, _ := e.MatchSignature(fn, WordInfo{ArgCount: -1}, []Value{NewInteger(5)})
	if sig != nil {
		t.Errorf("a pattern-violating forward operand must reject the swap split, got %v", sig)
	}
}

// The positive twin (canStack=true, pattern satisfied → fill the
// remaining slots from the stack) is exercised by ordinary infix-form
// dispatch in the spec suite (e.g. `5 sub 2 add 1`); it needs the real
// resolvedIdx tape mapping a hand-built matchSignature call cannot supply.
