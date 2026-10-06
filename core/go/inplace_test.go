package core

import (
	"strings"
	"testing"
)

// Unit tests for in-place compilation's kernel pieces (inplace.go): the cell
// kinds, the switch's parsing, the tape's nop pass and counters, and the
// engine helpers whose branches the corpus differential cannot steer into
// (lang/go/inplace_test.go drives the mechanism end to end). Positive and
// negative cases are paired.

func TestParseInPlaceMode(t *testing.T) {
	for in, want := range map[string]InPlaceMode{
		"1": InPlaceOn, "on": InPlaceOn, "true": InPlaceOn,
		"verify": InPlaceVerify,
		"":       InPlaceOff, "0": InPlaceOff, "off": InPlaceOff, "yes": InPlaceOff,
	} {
		if got := ParseInPlaceMode(in); got != want {
			t.Errorf("ParseInPlaceMode(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestInPlaceCellKinds(t *testing.T) {
	call := Value{Parent: TInternal, Data: newCallInfo("add", nil, 2, 0)}
	if !IsNop(nopCell) || IsCall(nopCell) {
		t.Error("the nop cell must read as a nop and not as a call")
	}
	if !IsCall(call) || IsNop(call) {
		t.Error("a call cell must read as a call and not as a nop")
	}
	// Neither kind is mistaken for an ordinary value or another marker.
	for _, v := range []Value{NewInteger(1), NewWord("add"), NewForward(ForwardInfo{FuncName: "f"}), NewOpenParen(), {Parent: TInternal}} {
		if IsNop(v) || IsCall(v) {
			t.Errorf("%v must be neither a nop nor a call", v)
		}
		if _, ok := AsCall(v); ok {
			t.Errorf("AsCall(%v) must decline", v)
		}
	}
	// Both are engine markers, never values.
	if !isEngineMarker(nopCell) || !isEngineMarker(call) || IsRecordableLiteral(nopCell) || IsRecordableLiteral(call) {
		t.Error("nop and call cells are engine markers, never recordable literals")
	}
	if nopCell.Data.IsTypeContent(&nopCell) || call.Data.IsTypeContent(&call) {
		t.Error("neither cell carries type content")
	}
}

func TestCallInfoOperandStorage(t *testing.T) {
	for n := 1; n <= 3; n++ {
		ci := newCallInfo("f", nil, n, 4)
		if cap(ci.Args()) != n || len(ci.Args()) != 0 || ci.Name() != "f" || ci.Sig() != nil {
			t.Errorf("a %d-operand record uses its sized storage: cap %d len %d", n, cap(ci.Args()), len(ci.Args()))
		}
		if !ci.match.InPlace || ci.match.SpanLo != 4 {
			t.Errorf("a %d-operand record's match is the call cell's", n)
		}
	}
	big := newCallInfo("g", nil, 5, 0)
	if cap(big.Args()) != 5 {
		t.Errorf("a 5-operand record allocates its storage: cap %d", cap(big.Args()))
	}
	if none := newCallInfo("h", nil, 0, 0); none.Args() != nil {
		t.Error("a nullary record carries no operand slice, as the legacy match does")
	}
}

func TestInPlaceCellRendering(t *testing.T) {
	ci := newCallInfo("add", nil, 2, 0)
	ci.match.Args = append(ci.match.Args, NewInteger(2), NewInteger(1))
	call := Value{Parent: TInternal, Data: ci}
	if got := nopCell.String(); got != "nop" {
		t.Errorf("nop renders %q", got)
	}
	if got := call.String(); got != "call(add [2 1])" {
		t.Errorf("call renders %q", got)
	}
	if got := TraceVisibleLen(TraceColorize(nopCell)); got != 1 {
		t.Errorf("a traced nop is one visible character, got %d", got)
	}
	if got := TraceColorize(call); !strings.Contains(got, "add") || !strings.Contains(got, "⟨") {
		t.Errorf("a traced call shows the word in brackets: %q", got)
	}
}

func tapeOf(vals ...Value) *Tape { return NewTape(vals, StackHeadroom) }

func TestTapeDropNopsIn(t *testing.T) {
	fwd := NewForward(ForwardInfo{FuncName: "f", ExpectedArgs: 1})
	tp := tapeOf(NewInteger(1), nopCell, fwd, nopCell, nopCell, NewInteger(2), nopCell)
	if got := tp.DropNopsIn(-5, 99); got != 4 {
		t.Fatalf("dropped %d nops, want 4 (the range is clamped to the tape)", got)
	}
	if tp.Len() != 3 || !IsForward(tp.At(1)) {
		t.Fatalf("survivors out of order: %v", tp.Snapshot())
	}
	if a, _ := AsInteger(tp.At(0)); a != 1 {
		t.Errorf("first survivor %v", tp.At(0))
	}
	if b, _ := AsInteger(tp.At(2)); b != 2 {
		t.Errorf("last survivor %v", tp.At(2))
	}
	assertForwardCount(t, tp, "after a nop pass that moved a forward")
	// A range with no nops changes nothing.
	if got := tp.DropNopsIn(0, tp.Len()); got != 0 || tp.Len() != 3 {
		t.Errorf("a nop-free range dropped %d", got)
	}
	// Only the range is touched.
	tp = tapeOf(nopCell, NewInteger(1), nopCell, nopCell)
	if got := tp.DropNopsIn(1, 3); got != 1 || tp.Len() != 3 || !IsNop(tp.At(0)) || !IsNop(tp.At(2)) {
		t.Errorf("a ranged pass dropped %d and left %v", got, tp.Snapshot())
	}
}

func TestTapeStats(t *testing.T) {
	tp := tapeOf(NewInteger(1), NewInteger(2), NewInteger(3))
	tp.Set(0, NewInteger(9))
	tp.Insert(0, NewInteger(8))
	tp.Remove(3)
	tp.Splice(0, 1, NewInteger(7))
	st := tp.Stats()
	if st.Sets != 1 || st.Inserts != 1 || st.Removes != 1 || st.Splices != 1 {
		t.Errorf("edit counts %+v", st)
	}
	if st.Moved == 0 {
		t.Error("the gap moved across cells and the count shows none")
	}
	if (NewTape(nil, 0)).Stats() != (TapeStats{}) {
		t.Error("a fresh tape counts nothing")
	}
}

// inPlaceEngine is an engine over tape on a fresh registry, for the helpers.
func inPlaceEngine(t *testing.T, vals ...Value) *Engine {
	t.Helper()
	e := New(newTestRegistry(t))
	e.Tape = tapeOf(vals...)
	return e
}

func TestInPlaceOperandWalks(t *testing.T) {
	e := inPlaceEngine(t, NewInteger(5), NewWord("f"), NewForward(ForwardInfo{FuncName: "f"}), NewInteger(1), nopCell, NewInteger(2))
	if got := e.inPlaceOperands(2, e.Tape.Len(), 2); len(got) != 2 || got[0] != 3 || got[1] != 5 {
		t.Errorf("operands after the marker, nops skipped: %v", got)
	}
	if got := e.inPlaceOperands(2, e.Tape.Len(), 0); got != nil {
		t.Errorf("no operands claimed reads none: %v", got)
	}
	if got := e.claimedStack(1, 1); len(got) != 1 || got[0] != 0 {
		t.Errorf("the value-stack operand below the word: %v", got)
	}
	if got := e.claimedStack(1, 0); got != nil {
		t.Errorf("no stack operands claimed reads none: %v", got)
	}
	if got := e.prevNonNop(5); got != 3 {
		t.Errorf("the left neighbour past a nop is 3, got %d", got)
	}
	e.Tape = tapeOf(nopCell, NewInteger(1))
	if got := e.prevNonNop(1); got != -1 {
		t.Errorf("nothing but nops below: %d", got)
	}
	// A cell that is no in-place forward normalises to itself.
	if got := e.normalizeInPlaceForward(1, NormCommit); got != 1 {
		t.Errorf("a value cell normalised to %d", got)
	}
}

func TestInPlaceCompaction(t *testing.T) {
	// A statement stream's nops go; the pointer follows the cells it is on.
	e := inPlaceEngine(t, nopCell, NewInteger(1), nopCell, NewInteger(2))
	e.Pointer, e.nopPending, e.sawNop = 3, nopCompactEvery+1, true
	e.compactNops()
	if e.Tape.Len() != 2 || e.Pointer != 1 || e.nopPending != 0 {
		t.Errorf("compaction left %v, pointer %d, pending %d", e.Tape.Snapshot(), e.Pointer, e.nopPending)
	}
	// Above an open paren only: the cells below it are another scope's.
	e = inPlaceEngine(t, nopCell, NewOpenParen(), nopCell, NewInteger(2))
	e.Pointer = 3
	e.compactNops()
	if e.Tape.Len() != 3 || !IsNop(e.Tape.At(0)) || e.Pointer != 2 {
		t.Errorf("compaction crossed the open paren: %v", e.Tape.Snapshot())
	}
	// Declined while a forward is pending in the range (its FuncIndex would move).
	e = inPlaceEngine(t, nopCell, NewForward(ForwardInfo{FuncName: "f"}), nopCell, NewInteger(2))
	e.Pointer = 3
	e.compactNops()
	if e.Tape.Len() != 4 {
		t.Errorf("compaction ran under a pending forward: %v", e.Tape.Snapshot())
	}
	// Declined while a call cell stands at the pointer: its span is live.
	call := Value{Parent: TInternal, Data: newCallInfo("f", nil, 1, 0)}
	e = inPlaceEngine(t, nopCell, NewInteger(1), nopCell, call)
	e.Pointer, e.nopPending = 3, nopCompactEvery+1
	e.compactNops()
	if e.Tape.Len() != 4 || e.nopPending != nopCompactEvery+1 {
		t.Errorf("compaction ran under a call cell: %v, pending %d", e.Tape.Snapshot(), e.nopPending)
	}
	// A group's own pass: only on a lane that wrote nops.
	e = inPlaceEngine(t, NewOpenParen(), nopCell, NewInteger(1), NewCloseParen())
	e.Pointer = 3
	if got := e.compactGroup(0, 3); got != 3 || e.Tape.Len() != 4 {
		t.Errorf("the legacy lane must not pay for a nop pass: close %d", got)
	}
	e.sawNop = true
	if got := e.compactGroup(0, 3); got != 2 || e.Tape.Len() != 3 || e.Pointer != 2 {
		t.Errorf("the group pass: close %d, pointer %d, %v", got, e.Pointer, e.Tape.Snapshot())
	}
	e.dropAllNops()
	e.Tape = tapeOf(nopCell, NewInteger(1))
	e.dropAllNops()
	if e.Tape.Len() != 1 {
		t.Errorf("the run's end drops every nop: %v", e.Tape.Snapshot())
	}
}

func TestInPlaceVerifyBookkeeping(t *testing.T) {
	TakeInPlaceDisagreements()
	sigA := &Signature{Args: []*Type{TInteger}}
	sigB := &Signature{Args: []*Type{TString}}
	e := inPlaceEngine(t, NewInteger(1), NewWord("f"))
	// Off the verify lane nothing is armed.
	e.armVerify(sigA, 1, 1, false)
	if e.verifySig != nil {
		t.Fatal("armed off the verify lane")
	}
	e.Registry.InPlace = InPlaceVerify
	e.armVerify(sigA, 1, 1, false)
	if e.verifySig != sigA || !e.verifyHeld {
		t.Fatalf("armed %v, held %v", e.verifySig, e.verifyHeld)
	}
	// A re-plan somewhere else is not the armed one's.
	e.Pointer = 0
	e.checkVerify("f", sigB)
	if e.verifySig != nil || len(TakeInPlaceDisagreements()) != 0 {
		t.Error("a re-plan at another word must disarm without a finding")
	}
	// The same signature agrees; another disagrees, and no re-plan at all too.
	verified := InPlaceStats.Verified.Load()
	e.Pointer = 1
	e.armVerify(sigA, 1, 1, false)
	e.checkVerify("f", sigA)
	if InPlaceStats.Verified.Load() != verified+1 {
		t.Error("an agreeing re-plan is counted verified")
	}
	e.armVerify(sigA, 1, 1, true)
	e.checkVerify("f", sigB)
	e.armVerify(sigA, 1, 1, false)
	e.checkVerify("f", nil)
	ds := TakeInPlaceDisagreements()
	if len(ds) != 2 || ds[0].Replanned == "" || !ds[0].Commit || ds[1].Replanned != "" || !ds[0].PlanHeld {
		t.Errorf("findings %+v", ds)
	}
	// Disarmed: a later re-plan records nothing.
	e.checkVerify("f", sigB)
	if len(TakeInPlaceDisagreements()) != 0 {
		t.Error("an unarmed re-plan recorded a finding")
	}
}

// An in-place def forward keeps its typed-name map AFTER the marker; the
// §7.2 hint probe reads it there (the legacy layout keeps it before the word).
func TestInPlaceTypedBindingContext(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("MyShape", NewCarrier(TFnUndef))
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("f", NewWord("MyShape"))
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 0, InPlace: true})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("def"), fwd, nopCell, NewMap(m)}, StackHeadroom)
	e.Pointer = 4
	if !e.IsFnShapeTypedBindingContext() {
		t.Error("an in-place def's typed-name map after its marker is the fn-shape context")
	}
	// Nothing collected after the marker: no map to read.
	empty := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 0, InPlace: true})
	e.Tape = NewTape([]Value{NewWord("def"), empty}, StackHeadroom)
	e.Pointer = 2
	if e.IsFnShapeTypedBindingContext() {
		t.Error("an in-place def with no operand after its marker has no typed-name map")
	}
}

// A failed dispatch's diagnostic reads the stack prefix; under a pending
// in-place forward that prefix must read as the legacy one does — empty, the
// marker being the cell below the pointer there.
func TestInPlaceDiagPrefix(t *testing.T) {
	inPlace := NewForward(ForwardInfo{FuncName: "f", FuncIndex: 0, CollectedArgs: 1, InPlace: true})
	e := engWithTape(t, []Value{NewWord("f"), inPlace, NewInteger(1)}, 3)
	if got := e.diagPrefix(); got != nil {
		t.Errorf("an in-place forward's operands are not a diagnostic's stack: %v", got)
	}
	legacy := NewForward(ForwardInfo{FuncName: "f", FuncIndex: 1, CollectedArgs: 1})
	e = engWithTape(t, []Value{NewInteger(1), NewWord("f"), legacy}, 3)
	if got := e.diagPrefix(); len(got) != 3 {
		t.Errorf("a legacy forward keeps the plain prefix: %v", got)
	}
	e = engWithTape(t, []Value{NewInteger(1), nopCell}, 2)
	if got := ReorderCandidates(e.diagPrefix()); len(got) != 1 {
		t.Errorf("a nop below the pointer is no operand and no stop: %v", got)
	}
}
