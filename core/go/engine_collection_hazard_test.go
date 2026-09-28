package core

import "testing"

// hazardEmit is the inactive recorder plus the collection-hazard note
// (NUR121): it records what noteCollectionHazards marks and answers the
// paren classifier's CollectionHazard read from the same set.
type hazardEmit struct {
	EmitRecorder
	active  bool
	marked  map[string]bool
	leadOK  bool
	applyOK bool
}

func newHazardEmit() *hazardEmit {
	return &hazardEmit{EmitRecorder: TheInactiveEmit, active: true, marked: map[string]bool{}, leadOK: true, applyOK: true}
}

func (h *hazardEmit) Active() bool                   { return h.active }
func (h *hazardEmit) NoteCollectionHazard(id string) { h.marked[id] = true }
func (h *hazardEmit) CollectionHazard(id string) bool {
	return h.marked[id]
}
func (h *hazardEmit) DynApplyLeadEligible(Value) bool { return h.leadOK }
func (h *hazardEmit) RecordDynApply(args []Value, fn, out Value, pos SrcPos) (int, bool) {
	return len(args), h.applyOK
}

func hazardEngine(t *testing.T, es EmitRecorder) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	r.Check.Emit = es
	return NewTop(r)
}

// TestNoteCollectionHazardsScope pins the scan's SCOPE: an unapplied
// fn-typed value below the lowest stack-collected index is marked; an open
// paren between them seals it off; a frame's resolved-argument prefix
// (FrameOpenInfo.ArgSpan) and the run's StartAt prefix are never marked
// (arguments are inert); a quoted fn and a plain value are not fn leads.
func TestNoteCollectionHazardsScope(t *testing.T) {
	fn := NewCarrier(TFunction)
	dyn := NewDynamicCarrier(TAny)
	quoted := NewCarrier(TFunction)
	quoted.Quoted = true
	five := NewInteger(5)

	t.Run("marks the lead below the collected value", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		// [fn dyn 5 ^word]: the word stack-collects 5 (index 2); both fn-shaped
		// values below it are marked.
		e.Tape = NewTape([]Value{fn, dyn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if !es.marked[fn.ID] || !es.marked[dyn.ID] {
			t.Errorf("both fn-shaped values below the collected index must be marked: %v", es.marked)
		}
	})
	t.Run("an open paren seals the scope", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, NewOpenParen(), five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("a lead outside the paren scope must not be marked: %v", es.marked)
		}
	})
	t.Run("a frame's argument prefix is inert", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		// FrameOpen(ArgSpan 1) fn 5 ^word: fn is the spliced unnamed argument.
		e.Tape = NewTape([]Value{NewFrameOpenSpan(&FnFrameMeta{Name: "f"}, 1), fn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("a frame-prefix argument must not be marked: %v", es.marked)
		}
	})
	t.Run("the run's StartAt prefix is inert", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 2
		e.inertPrefix = 1
		e.noteCollectionHazards(nil, []int{1})
		if len(es.marked) != 0 {
			t.Errorf("a StartAt-prefix argument must not be marked: %v", es.marked)
		}
	})
	t.Run("quoted and plain values are not leads", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{quoted, NewInteger(1), five, NewWord("add")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("nothing to mark: %v", es.marked)
		}
	})
	t.Run("forward-only collection marks nothing", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, NewWord("add"), five}, StackHeadroom)
		e.Pointer = 1
		e.noteCollectionHazards(nil, nil)
		e.noteCollectionHazards(nil, []int{2})
		if len(es.marked) != 0 {
			t.Errorf("a forward-collected index consumes nothing below the word: %v", es.marked)
		}
	})
	t.Run("an inactive recorder marks nothing", func(t *testing.T) {
		es := newHazardEmit()
		es.active = false
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, five, NewWord("add")}, StackHeadroom)
		e.Pointer = 2
		e.noteCollectionHazards(nil, []int{1})
		if len(es.marked) != 0 {
			t.Errorf("inactive: %v", es.marked)
		}
	})
	t.Run("a strip-input hop is transparent", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.Tape = NewTape([]Value{fn, five, NewWord("error")}, StackHeadroom)
		e.Pointer = 2
		strip := &Signature{Callable: &CallableSpec{StripsUnconsumedInput: true}}
		e.noteCollectionHazards(strip, []int{1})
		if len(es.marked) != 0 {
			t.Errorf("a strip-input hop passes the region through and marks nothing: %v", es.marked)
		}
		plain := &Signature{Callable: &CallableSpec{}}
		e.noteCollectionHazards(plain, []int{1})
		if !es.marked[fn.ID] {
			t.Errorf("an ordinary collection past the lead marks it: %v", es.marked)
		}
	})
	t.Run("a full-stack scope floor bounds the scan", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		other := NewCarrier(TFunction)
		e.Tape = NewTape([]Value{other, fn, five, NewWord("depth")}, StackHeadroom)
		e.Pointer = 3
		e.noteCollectionHazardsBelow(1, 3)
		if es.marked[other.ID] || !es.marked[fn.ID] {
			t.Errorf("only values at or above the floor are in scope: %v", es.marked)
		}
	})
}

// TestParenLeadFnApplyIdxDeclinesAHazardLead pins the classifier's consumer
// half: a lead the scan marked declines the window (`(g x add 1)` — the
// window's argument is add's result), an unmarked twin admits.
func TestParenLeadFnApplyIdxDeclinesAHazardLead(t *testing.T) {
	es := newHazardEmit()
	e := hazardEngine(t, es)
	lead := NewCarrier(TFunction)
	e.Tape = NewTape([]Value{NewOpenParen(), lead, NewInteger(5), NewCloseParen()}, StackHeadroom)
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != 1 {
		t.Fatalf("an unmarked lead admits the window, got %d", got)
	}
	es.marked[lead.ID] = true
	if got := e.parenLeadFnApplyIdx(es, 0, 3, 2, 2); got != -1 {
		t.Errorf("a hazard-marked lead must decline the window, got %d", got)
	}
}

// TestCollectionHazardStopsAtAStatementEnd pins NUR276: a statement end
// between a candidate and the collected value keeps the candidate unmarked
// — its re-step collects nothing past its own statement's end. The
// collected value answers by its own position, or, def-bound and
// positionless, by EVERY position it was read at; an unknown position
// proves nothing.
func TestCollectionHazardStopsAtAStatementEnd(t *testing.T) {
	at := func(r, c int) SrcPos { return SrcPos{Row: r, Col: c} }
	dyn := WithPosAt(NewDynamicCarrier(TAny), at(1, 1))
	t.Run("a positioned value past the end", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.stmtEnds = []SrcPos{at(1, 5)}
		e.Tape = NewTape([]Value{dyn, WithPosAt(NewInteger(5), at(1, 9)), NewWord("size")}, StackHeadroom)
		e.Pointer = 2
		e.noteCollectionHazards(nil, []int{1})
		if es.marked[dyn.ID] {
			t.Error("a value past the statement end is no argument of the earlier lead")
		}
	})
	t.Run("a positioned value in the lead's statement", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.stmtEnds = []SrcPos{at(1, 5)}
		e.Tape = NewTape([]Value{dyn, WithPosAt(NewInteger(5), at(1, 3)), NewWord("size")}, StackHeadroom)
		e.Pointer = 2
		e.noteCollectionHazards(nil, []int{1})
		if !es.marked[dyn.ID] {
			t.Error("a value before the end is the lead's own argument: marked")
		}
	})
	t.Run("a def-bound value by its reads", func(t *testing.T) {
		es := newHazardEmit()
		e := hazardEngine(t, es)
		e.stmtEnds = []SrcPos{at(1, 5)}
		s := NewString("s")
		s.ID = "bound-s"
		e.Tape = NewTape([]Value{dyn, s, NewWord("size")}, StackHeadroom)
		e.Pointer = 2
		e.noteCollectionHazards(nil, []int{1})
		if !es.marked[dyn.ID] {
			t.Error("no read recorded: nothing is proven, the lead is marked")
		}
		es.marked = map[string]bool{}
		e.noteDefReadPos("bound-s", at(1, 9))
		e.noteDefReadPos("", at(1, 9))
		e.noteDefReadPos("bound-s", SrcPos{})
		e.noteCollectionHazards(nil, []int{1})
		if es.marked[dyn.ID] {
			t.Error("every read past the end: unmarked")
		}
		e.noteDefReadPos("bound-s", at(1, 3))
		e.noteCollectionHazards(nil, []int{1})
		if !es.marked[dyn.ID] {
			t.Error("one read before the end proves nothing: marked")
		}
	})
	t.Run("unknown positions prove nothing", func(t *testing.T) {
		e := hazardEngine(t, newHazardEmit())
		e.stmtEnds = []SrcPos{at(1, 5)}
		if e.stmtEndBetween(SrcPos{}, at(1, 9)) || e.stmtEndBetween(at(1, 1), SrcPos{}) || e.collectedPastStmtEnd(at(1, 1), NewInteger(3)) {
			t.Error("a zero position or a value with no identity proves no crossing")
		}
		if !srcPosBefore(at(1, 9), at(2, 1)) || srcPosBefore(at(2, 1), at(1, 9)) || srcPosBefore(at(1, 1), at(1, 1)) {
			t.Error("srcPosBefore orders by row, then column, strictly")
		}
	})
	t.Run("an analysis pass notes each positioned end", func(t *testing.T) {
		r := covRegistry(t, nil)
		r.Check.Mode = true
		e := NewTop(r)
		if _, err := e.Run([]Value{NewInteger(1), WithPosAt(NewEnd(), at(1, 3)), NewInteger(2), NewEnd()}); err != nil {
			t.Fatal(err)
		}
		if len(e.stmtEnds) != 1 || e.stmtEnds[0] != at(1, 3) {
			t.Errorf("the positioned end is noted, the positionless one is not: %v", e.stmtEnds)
		}
	})
}
