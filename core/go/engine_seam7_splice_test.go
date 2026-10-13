package core

// Wave-7 coverage, part 5: splice / hint / rearrange helpers reached with
// hand-built engine state. See design/TEST-SEAMS.10.md.

import (
	"strings"
	"testing"
)

// fnShapeCtxEngine returns an engine positioned in a fn-shape typed-binding
// context (IsFnShapeTypedBindingContext() == true).
func fnShapeCtxEngine(t *testing.T) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	r.Defs.Push("MyShape", NewCarrier(TFnUndef))
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("f", NewWord("MyShape"))
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 1})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMap(m), fwd, NewInteger(0)}, StackHeadroom)
	e.Pointer = 3
	if !e.IsFnShapeTypedBindingContext() {
		t.Fatal("setup: expected fn-shape typed-binding context")
	}
	return e
}

// --- maybeAddFnShapeHint ---------------------------------------------------

func TestS7MaybeAddFnShapeHintNil(t *testing.T) {
	e := &Engine{}
	if got := e.maybeAddFnShapeHint(nil); got != nil {
		t.Errorf("nil err → nil, got %v", got)
	}
}

func TestS7MaybeAddFnShapeHintNonSigError(t *testing.T) {
	e := &Engine{}
	orig := &BoruError{Code: "other_error", Detail: "x"}
	if got := e.maybeAddFnShapeHint(orig); got != error(orig) {
		t.Error("a non-signature error must pass through unchanged")
	}
}

func TestS7MaybeAddFnShapeHintSetsSuggestion(t *testing.T) {
	e := fnShapeCtxEngine(t)
	err := &BoruError{Code: "signature_error", Src: "double"}
	out := e.maybeAddFnShapeHint(err).(*BoruError)
	if len(out.Suggestions) != 1 || !strings.Contains(out.Suggestions[0].Message, "double/q") {
		t.Errorf("suggestions = %v, want the /q suggestion", out.Suggestions)
	}
}

func TestS7MaybeAddFnShapeHintAppendsSuggestion(t *testing.T) {
	e := fnShapeCtxEngine(t)
	// Existing suggestions are kept; the /q one is appended after them.
	err := &BoruError{Code: "signature_error", Src: "double",
		Suggestions: []DiagSuggestion{{Message: "prior"}}}
	out := e.maybeAddFnShapeHint(err).(*BoruError)
	if len(out.Suggestions) != 2 || out.Suggestions[0].Message != "prior" ||
		!strings.Contains(out.Suggestions[1].Message, "double/q") {
		t.Errorf("suggestions = %v, want prior + /q suggestion", out.Suggestions)
	}
}

// --- rearrangeForForward ---------------------------------------------------

func TestS7RearrangeForForwardZeroTotal(t *testing.T) {
	e := engWithTape(t, []Value{NewWord("x")}, 0)
	e.rearrangeForForward(0, 0) // total == 0 → early return, no panic
}

// --- spliceMatchResults fallback branch ------------------------------------

func TestS7SpliceMatchResultsFallback(t *testing.T) {
	// len(sortedIndices) != n and n != 0 → contiguous fallback splice.
	e := engWithTape(t, []Value{NewInteger(1), NewInteger(2), NewInteger(3), NewWord("w")}, 3)
	if at := e.spliceMatchResults(&MatchResult{}, nil, 2, []Value{NewInteger(99)}); at != 1 {
		t.Fatalf("spliceMatchResults placed the results at %d, want 1", at)
	}
	// argStart = pointer(3)-n(2) = 1; [1..3] replaced by [99].
	if e.Pointer != 1 {
		t.Errorf("pointer = %d, want 1", e.Pointer)
	}
}

func TestS7SpliceMatchResultsFallbackClampsArgStart(t *testing.T) {
	// pointer < n → argStart clamps to 0.
	e := engWithTape(t, []Value{NewInteger(1), NewWord("w")}, 1)
	if at := e.spliceMatchResults(&MatchResult{}, nil, 5, []Value{NewInteger(7)}); at != 0 {
		t.Fatalf("spliceMatchResults placed the results at %d, want 0", at)
	}
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0 (clamped)", e.Pointer)
	}
}
