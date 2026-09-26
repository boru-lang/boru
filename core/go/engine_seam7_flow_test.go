package core

// Wave-7 coverage, part 3: control-flow (mark/move/loop) resolvers,
// close-paren scanning, implicit-end, and the pending-forward predicate
// probes. Each is exercised by hand-built tape state — these helpers
// read/rewrite the tape directly, so a constructed *Engine reaches their
// edge arms without a full Run(). See design/TEST-SEAMS.10.md.

import (
	"strings"
	"testing"
)

// --- stepMove orphan ------------------------------------------------------

func TestS7StepMoveOrphanMarkGone(t *testing.T) {
	// The mark id is registered in e.marks but the Mark token is not on
	// the tape (a for-loop controller removed it) → orphan move, removed
	// quietly.
	e := engWithTape(t, []Value{NewMove("m1", "for loop")}, 0)
	e.marks = map[string]bool{"m1": true}
	if err := e.stepMove(e.Tape.At(0)); err != nil {
		t.Fatalf("orphan move should be removed quietly, got %v", err)
	}
	if e.Tape.Len() != 0 {
		t.Errorf("orphan move not removed: len=%d", e.Tape.Len())
	}
}

func TestS7StepMoveMarkNotFoundErrors(t *testing.T) {
	// The mark id is NOT registered → move_error.
	e := engWithTape(t, []Value{NewMove("nope", "for loop")}, 0)
	if err := e.stepMove(e.Tape.At(0)); err == nil {
		t.Fatal("move to an unknown mark must error")
	}
}

// --- stepMoveCont next-iteration (nil marks map) --------------------------

func TestS7StepMoveContMoreIterationsNilMarks(t *testing.T) {
	r := covRegistry(t, nil)
	cont := &ForCont{
		Registry: r,
		IterName: "i",
		Current:  0,
		End:      3,
		Step:     1,
		Body:     []Value{NewInteger(1)},
	}
	move := NewMoveCont("m1", "for loop", cont)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMark("m1"), move}, StackHeadroom)
	e.marks = nil // force the lazy-init arm
	if err := e.stepMoveCont(0, 1, MoveInfo{To: "m1", Cont: cont}); err != nil {
		t.Fatalf("stepMoveCont: %v", err)
	}
	if e.marks == nil {
		t.Error("marks map should have been initialised for the next iteration")
	}
}

// --- trace-note gating (only built when a trace hook is installed) --------

// TestS7TraceNotesGated pins the F1 optimization: every e.traceNote builder
// on the mark/move/loop/if resolvers is wrapped in `if e.trace != nil`, so
// the (fmt.Sprintf / concat) string is built ONLY when a trace hook is
// installed. The positive arms assert the note IS produced under trace; the
// negative arm asserts stepMark leaves the note EMPTY with tracing off (the
// whole point — no dead string on the hot path).
func TestS7TraceNotesGated(t *testing.T) {
	noop := func(int, int, []Value, string) {}

	// stepMark → "mark <id>"
	{
		e := NewTop(covRegistry(t, nil))
		e.SetTrace(noop)
		e.Tape = NewTape([]Value{NewMark("m1")}, StackHeadroom)
		e.stepMark(e.Tape.At(0))
		if e.traceNote != "mark m1" {
			t.Errorf("stepMark note = %q, want %q", e.traceNote, "mark m1")
		}
	}

	// negative: stepMark with NO trace hook leaves the note empty.
	{
		e := NewTop(covRegistry(t, nil))
		e.Tape = NewTape([]Value{NewMark("m1")}, StackHeadroom)
		e.stepMark(e.Tape.At(0))
		if e.traceNote != "" {
			t.Errorf("stepMark built a note with tracing off: %q", e.traceNote)
		}
	}

	// stepMove orphan → "move orphan <id>"
	{
		e := engWithTape(t, []Value{NewMove("m1", "for loop")}, 0)
		e.SetTrace(noop)
		e.marks = map[string]bool{"m1": true}
		if err := e.stepMove(e.Tape.At(0)); err != nil {
			t.Fatalf("orphan move: %v", err)
		}
		if e.traceNote != "move orphan m1" {
			t.Errorf("orphan note = %q", e.traceNote)
		}
	}

	// stepMove replaying a mark body → "move→mark <id>"
	{
		e := NewTop(covRegistry(t, nil))
		e.SetTrace(noop)
		e.Tape = NewTape([]Value{NewMark("m2", NewInteger(7)), NewMove("m2", "r")}, StackHeadroom)
		e.marks = map[string]bool{"m2": true}
		e.Pointer = 1
		if err := e.stepMove(e.Tape.At(1)); err != nil {
			t.Fatalf("move→mark: %v", err)
		}
		if e.traceNote != "move→mark m2" {
			t.Errorf("move→mark note = %q", e.traceNote)
		}
	}

	// stepMoveCont next iteration → "for next <id> i=<n>"
	{
		r := covRegistry(t, nil)
		cont := &ForCont{Registry: r, IterName: "i", Current: 0, End: 3, Step: 1, Body: []Value{NewInteger(1)}}
		e := NewTop(r)
		e.SetTrace(noop)
		e.Tape = NewTape([]Value{NewMark("m1"), NewMoveCont("m1", "for loop", cont)}, StackHeadroom)
		e.marks = map[string]bool{"m1": true}
		if err := e.stepMoveCont(0, 1, MoveInfo{To: "m1", Cont: cont}); err != nil {
			t.Fatalf("for next: %v", err)
		}
		if !strings.HasPrefix(e.traceNote, "for next ") {
			t.Errorf("for-next note = %q", e.traceNote)
		}
	}

	// stepMoveCont final iteration → "for done"
	{
		r := covRegistry(t, nil)
		cont := &ForCont{Registry: r, IterName: "i", Current: 3, End: 3, Step: 1, Body: []Value{NewInteger(1)}}
		e := NewTop(r)
		e.SetTrace(noop)
		e.Tape = NewTape([]Value{NewMark("m1"), NewMoveCont("m1", "for loop", cont)}, StackHeadroom)
		e.marks = map[string]bool{"m1": true}
		if err := e.stepMoveCont(0, 1, MoveInfo{To: "m1", Cont: cont}); err != nil {
			t.Fatalf("for done: %v", err)
		}
		if e.traceNote != "for done" {
			t.Errorf("for-done note = %q", e.traceNote)
		}
	}

	// stepMoveIf → "if <bool>"
	{
		ifc := &IfCont{Then: []Value{NewInteger(1)}, Else: []Value{NewInteger(2)}}
		e := NewTop(covRegistry(t, nil))
		e.SetTrace(noop)
		e.Tape = NewTape([]Value{NewMark("m1"), NewBoolean(true), NewMoveIf("m1", "if", ifc)}, StackHeadroom)
		e.marks = map[string]bool{"m1": true}
		e.Pointer = 2
		if err := e.stepMove(e.Tape.At(2)); err != nil {
			t.Fatalf("if: %v", err)
		}
		if e.traceNote != "if true" {
			t.Errorf("if note = %q", e.traceNote)
		}
	}
}

// --- effectiveResolved scratch reuse (F4) ---------------------------------

// TestEffectiveResolvedScratchReuse pins the F4 optimization: effectiveResolved
// reuses one engine-owned result buffer and one exclusion set across calls
// instead of allocating per dispatch. It asserts the reuse is correct — a
// shorter window never leaks the previous (longer) result, and a stale
// exclusion set from a prior forward-bearing call is not applied to a later
// forward-free one.
func TestEffectiveResolvedScratchReuse(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1), NewInteger(2), NewInteger(3)}, 0)

	// Full window: all three are resolved.
	e.Pointer = 3
	if got := e.EffectiveResolved(); len(got) != 3 {
		t.Fatalf("want 3 resolved, got %d: %s", len(got), renderAll(got))
	}

	// Reuse must not leak the longer previous result: a shorter window
	// returns exactly its own entries (buffer reset to [:0]).
	e.Pointer = 1
	got := e.EffectiveResolved()
	iv, _ := AsInteger(got[0])
	if len(got) != 1 || iv != 1 {
		t.Fatalf("reuse leaked stale data: %s", renderAll(got))
	}

	// An OpenParen bounds the scan: entries below the paren are excluded.
	e2 := engWithTape(t, []Value{NewInteger(9), NewOpenParen(), NewInteger(1), NewInteger(2)}, 0)
	e2.Pointer = 4
	if got := e2.EffectiveResolved(); len(got) != 2 {
		t.Fatalf("paren-bounded window: want 2, got %d", len(got))
	}

	// A Forward in the window excludes its function-word slot.
	sig := &Signature{Args: []*Type{TInteger}, BarrierPos: -1}
	e3 := engWithTape(t, []Value{
		NewInteger(5),
		NewForward(ForwardInfo{FuncName: "cadd", Sig: sig, FuncIndex: 2, CollectedArgs: 0}),
		NewWord("cadd"),
	}, 0)
	e3.Pointer = 3
	got = e3.EffectiveResolved()
	iv, _ = AsInteger(got[0])
	if len(got) != 1 || iv != 5 {
		t.Fatalf("forward-exclude: want [5], got %s", renderAll(got))
	}

	// A later forward-FREE call on the same engine must NOT apply the stale
	// exclusion set from the call above (the reused map is only consulted
	// when this call itself saw a forward).
	e3.Tape = NewTape([]Value{NewInteger(7), NewInteger(8)}, StackHeadroom)
	e3.Pointer = 2
	if got := e3.EffectiveResolved(); len(got) != 2 {
		t.Fatalf("stale-exclusion leak: want 2, got %d", len(got))
	}
}

// --- handleLoopBreak / handleLoopContinue with missing mark ---------------

func TestS7HandleLoopBreakMissingMark(t *testing.T) {
	r := covRegistry(t, nil)
	cont := &ForCont{Registry: r, IterName: "i"}
	// A MoveCont with no matching Mark on the tape → markIdx<0 → skip.
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMoveCont("gone", "for loop", cont)}, StackHeadroom)
	e.marks = map[string]bool{"gone": true}
	if e.handleLoopBreak() {
		t.Error("break with no mark on tape should not resolve")
	}
}

func TestS7HandleLoopContinueMissingMark(t *testing.T) {
	r := covRegistry(t, nil)
	cont := &ForCont{Registry: r, IterName: "i"}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMoveCont("gone", "for loop", cont)}, StackHeadroom)
	e.marks = map[string]bool{"gone": true}
	if e.handleLoopContinue() {
		t.Error("continue with no mark on tape should not resolve")
	}
}

// --- cleanMarks -----------------------------------------------------------

func TestS7CleanMarks(t *testing.T) {
	e := engWithTape(t, []Value{NewMark("a"), NewInteger(1), NewMove("a", "r")}, 0)
	e.cleanMarks()
	if e.Tape.Len() != 1 {
		t.Errorf("cleanMarks left %d values, want 1 (the Integer)", e.Tape.Len())
	}
	if e.marks != nil {
		t.Error("marks map should be cleared")
	}
}

// --- findCloseParenAfter --------------------------------------------------

func TestS7FindCloseParenAfter(t *testing.T) {
	// Nested open with only one close → the outer group is unterminated.
	e := engWithTape(t, []Value{NewOpenParen(), NewOpenParen(), NewCloseParen()}, 0)
	if got := e.findCloseParenAfter(0); got != -1 {
		t.Errorf("findCloseParenAfter = %d, want -1 (unterminated outer)", got)
	}
	// Balanced: the matching close is found.
	e2 := engWithTape(t, []Value{NewOpenParen(), NewInteger(1), NewCloseParen()}, 0)
	if got := e2.findCloseParenAfter(0); got != 2 {
		t.Errorf("findCloseParenAfter = %d, want 2", got)
	}
}

// --- implicitEnd ----------------------------------------------------------

func TestS7ImplicitEndForwardBeforeFunc(t *testing.T) {
	// Forward marker sits BEFORE its function index → the funcIdx-- arm.
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "cadd", Sig: sig, FuncIndex: 2, CollectedArgs: 0})
	e := engWithTape(t, []Value{fwd, NewInteger(1), NewWord("cadd")}, 0)
	if err := e.implicitEnd(0); err != nil {
		t.Fatalf("implicitEnd: %v", err)
	}
}

// --- pending-forward predicates (break arms) ------------------------------

func fwdFull(sig *Signature) Value {
	return NewForward(ForwardInfo{Sig: sig, CollectedArgs: sig.TotalArgs(), FuncIndex: 0})
}

func TestS7PendingForwardPredicatesFullyCollected(t *testing.T) {
	// A pending forward whose slots are all filled (CollectedArgs ==
	// TotalArgs) hits the break in each probe → all report false.
	sig := &Signature{Params: []FnParam{{Type: TInteger}}, BarrierPos: -1}
	e := engWithTape(t, []Value{fwdFull(sig), NewInteger(0)}, 1)
	if e.hasPendingForwardQuoteArg() {
		t.Error("fully-collected forward → no pending quote slot")
	}
	if e.hasPendingForwardFormArg() {
		t.Error("fully-collected forward → no pending form slot")
	}
}
