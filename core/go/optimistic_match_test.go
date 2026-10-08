package core

import (
	"slices"
	"testing"
)

// optimisticSig is fold's three-operand shape: a List body, a Map
// collection, an Any seed.
func optimisticSig() *Signature {
	s := &Signature{Args: []*Type{TList, TMap, TAny}, BarrierPos: 2}
	NormalizeSig(s)
	return s
}

// optimisticEngine is a compiling pass over `0 fold [add] (mk)` as the
// matched dispatch leaves it — rearrangeForForward has laid the operands out
// beneath the word, signature position i sitting i+1 below it — with the
// declared-Any result of (mk) at the Map slot.
func optimisticEngine(t *testing.T) (*Engine, *MatchResult, []int) {
	t.Helper()
	zero, dyn, body := withID(NewInteger(0)), withID(NewDynamicCarrier(TAny)), withID(NewList([]Value{NewWord("add")}))
	e := layoutEngine(t, []Value{zero, body, dyn, NewWord("fold")}, 3)
	e.Registry.Check.Mode, e.Registry.Check.Compiling = true, true
	t.Cleanup(func() { e.Registry.Check.Mode, e.Registry.Check.Compiling = false, false })
	// The forward phase collected [add] then (mk): the rearrangement puts the
	// first-collected on top and records the split for the word.
	e.rearrangeForForward(1, 2)
	return e, &MatchResult{Sig: optimisticSig(), Args: []Value{body, dyn, zero}, Name: "fold"}, []int{2, 1, 0}
}

// TestRearrangeForForwardRecordsTheSplit pins the record NUR264's window
// reads: the word the rearrangement laid out, and how many of its operands
// were written after it. The record is the word's only at its pointer and
// position; a dispatch that collected nothing forward has no split.
func TestRearrangeForForwardRecordsTheSplit(t *testing.T) {
	e, _, _ := optimisticEngine(t)
	if got := e.Tape.At(2); !got.Parent.Equal(TList) {
		t.Fatalf("the first-collected operand sits on top, got %v", got)
	}
	if e.forwardSplit() != 2 {
		t.Errorf("two operands were written after the word, got %d", e.forwardSplit())
	}
	e.Pointer = 2
	if e.forwardSplit() != 0 {
		t.Error("another pointer is another dispatch: no split")
	}
	e.Pointer = 3
	w := e.Tape.At(3)
	w.SetPos(SrcPos{Row: 1, Col: 9})
	e.Tape.Set(3, w)
	if e.forwardSplit() != 0 {
		t.Error("another word at the pointer is another dispatch: no split")
	}
	e.Pointer = 9
	if e.forwardSplit() != 0 {
		t.Error("a pointer past the tape has no split")
	}
}

// TestOptimisticOuterWindow pins NUR264's published window: the stack run
// beneath the word top down, then the written operands in written order,
// with the render tuple in source order — and the dispatches it declines.
func TestOptimisticOuterWindow(t *testing.T) {
	e, match, indices := optimisticEngine(t)
	o := e.optimisticOuter(match, indices)
	if o == nil {
		t.Fatal("an Any operand at a Map slot is an optimistic match")
	}
	if o.Word != "fold" || o.NFwd != 2 || len(o.Vals) != 3 || o.Vals[0].ID != match.Args[2].ID ||
		o.Vals[1].ID != match.Args[0].ID || o.Vals[2].ID != match.Args[1].ID {
		t.Errorf("window: the stack's 0, then [add] and the carrier as written; got %+v", o)
	}
	if len(o.Written) != 3 || o.Written[0] != 0 || o.Written[1] != 1 || o.Written[2] != 2 {
		t.Errorf("render tuple in source order, got %v", o.Written)
	}
	// Every operand from the stack: the window is the stack run top down,
	// rendered bottom up.
	e.fwdSplitN = 0
	if o := e.optimisticOuter(match, indices); o == nil || o.NFwd != 0 || o.Written[0] != 2 || o.Written[2] != 0 {
		t.Errorf("all from the stack: got %+v", o)
	}
	e.fwdSplitN = 2
	// A pointer past the tape keeps a positionless window.
	e.Pointer = 9
	if o := e.optimisticOuter(match, indices); o == nil || o.NFwd != 0 || o.Pos != (SrcPos{}) {
		t.Errorf("past the tape: no split, no position; got %+v", o)
	}
	e.Pointer = 3

	concrete := &MatchResult{Sig: match.Sig, Args: []Value{match.Args[0], NewMap(NewOrderedMap()), match.Args[2]}, Name: "fold"}
	declines := []struct {
		name  string
		match *MatchResult
		idx   []int
		prep  func()
	}{
		{"a plain pass", match, indices, func() { e.Registry.Check.Compiling = false }},
		{"an outer one published", match, indices, func() { e.Registry.Check.OptimisticOuter = &OuterMatch{} }},
		{"no signature", &MatchResult{Args: match.Args, Name: "fold"}, indices, nil},
		{"no name", &MatchResult{Sig: match.Sig, Args: match.Args}, indices, nil},
		{"positions short", match, indices[:2], nil},
		{"every operand conforms", concrete, indices, nil},
		{"a split wider than the window", match, indices, func() { e.fwdSplitN = 4 }},
	}
	for _, c := range declines {
		if c.prep != nil {
			c.prep()
		}
		if o := e.optimisticOuter(c.match, c.idx); o != nil {
			t.Errorf("%s: no window, got %+v", c.name, o)
		}
		e.Registry.Check.Compiling, e.Registry.Check.OptimisticOuter, e.fwdSplitN = true, nil, 2
	}
}

// TestOptimisticLayoutIsReplayableOrNothing pins NUR263's layout and
// NUR283's surround: the dispatch's operands beneath the word in signature
// order, their written count from the rearrangement's record, and a
// boundary on both sides with what lies between it and the operands riding
// along when a rebuilt tape holds it as the pass's does — or nothing.
func TestOptimisticLayoutIsReplayableOrNothing(t *testing.T) {
	e, match, indices := optimisticEngine(t)
	restore := e.publishOptimisticLayout(match, indices)
	if l := e.Registry.Check.LayoutFor(match.Args); l == nil || l.NFwd != 2 {
		t.Fatalf("0 fold [add] (mk): two written, one beneath; got %+v", l)
	}
	restore()
	if e.Registry.Check.CurLayout != nil {
		t.Error("the restore unpublishes the layout")
	}
	// A declined layout publishes nothing and its restore is a no-op.
	e.Registry.Check.Compiling = false
	e.Registry.Check.Emit = nil
	restore = e.publishOptimisticLayout(match, indices)
	if e.Registry.Check.CurLayout != nil {
		t.Error("no recording pass: no layout")
	}
	restore()
	e.Registry.Check.Emit = layoutRecorder{}

	type prep func(e *Engine, m *MatchResult) (*MatchResult, []int)
	word := func(w WordInfo) prep {
		return func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.Tape.Set(3, NewValueRaw(TWord, w))
			return m, []int{2, 1, 0}
		}
	}
	declines := []struct {
		name string
		prep prep
	}{
		{"no operands", func(_ *Engine, m *MatchResult) (*MatchResult, []int) {
			return &MatchResult{Sig: m.Sig, Name: "fold"}, nil
		}},
		{"every operand conforms", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			mp := withID(NewMap(NewOrderedMap()))
			e.Tape.Set(1, mp)
			return &MatchResult{Sig: m.Sig, Args: []Value{m.Args[0], mp, m.Args[2]}, Name: "fold"}, []int{2, 1, 0}
		}},
		{"a pointer past the tape", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.Pointer = 9
			return m, []int{2, 1, 0}
		}},
		{"not a word", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.Tape.Set(3, NewInteger(7))
			return m, []int{2, 1, 0}
		}},
		{"a written count", word(WordInfo{Name: "fold", ArgCount: 3})},
		{"a forced stack read with no split", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.Tape.Set(3, NewValueRaw(TWord, WordInfo{Name: "fold", ArgCount: -1, ForceStack: true}))
			e.fwdSplitN = 0
			return m, []int{2, 1, 0}
		}},
		{"a forced forward read", word(WordInfo{Name: "fold", ArgCount: -1, ForceForward: true})},
		{"a split wider than the window", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.fwdSplitN = 4
			return m, []int{2, 1, 0}
		}},
		{"a void group", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.voidGroups = []string{"fold"}
			return m, []int{2, 1, 0}
		}},
		{"a gap", func(_ *Engine, m *MatchResult) (*MatchResult, []int) { return m, []int{2, 1, 1} }},
		{"not the tape's value", func(_ *Engine, m *MatchResult) (*MatchResult, []int) {
			return &MatchResult{Sig: m.Sig, Args: []Value{m.Args[0], withID(NewDynamicCarrier(TAny)), m.Args[2]}, Name: "fold"}, []int{2, 1, 0}
		}},
		{"a carrier beneath with no identity", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			v := NewDynamicCarrier(TAny)
			v.ID = ""
			e.Tape.Insert(0, v)
			e.Pointer, e.fwdSplitAt = 4, 4
			return m, []int{3, 2, 1}
		}},
		{"a word beneath", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.Tape.Insert(0, withID(NewWord("x")))
			e.Pointer, e.fwdSplitAt = 4, 4
			return m, []int{3, 2, 1}
		}},
		{"a group after", func(e *Engine, m *MatchResult) (*MatchResult, []int) {
			e.Tape.Insert(4, NewOpenParen())
			return m, []int{2, 1, 0}
		}},
	}
	for _, c := range declines {
		e, m, _ := optimisticEngine(t)
		m, idx := c.prep(e, m)
		if l := e.optimisticLayout(m, idx); l != nil {
			t.Errorf("%s: no layout, got %+v", c.name, l)
		}
	}
	// A GRADUAL operand whose bound conforms publishes the layout too
	// (gradualMatch): the poly record re-matches the live value, which may
	// still match nothing — the run raises the interpreter's report there.
	eg, mg, _ := optimisticEngine(t)
	gm := withID(NewDynamicCarrier(TMap))
	eg.Tape.Set(1, gm)
	gmatch := &MatchResult{Sig: mg.Sig, Args: []Value{mg.Args[0], gm, mg.Args[2]}, Name: "fold"}
	if optimisticMatch(gmatch) {
		t.Fatal("a dynamic(Map) at the Map slot conforms: no optimistic match")
	}
	if l := eg.optimisticLayout(gmatch, []int{2, 1, 0}); l == nil || l.NFwd != 2 {
		t.Errorf("a conforming gradual operand still publishes the layout, got %+v", l)
	}
	// The group and statement boundaries close the two sides.
	e, m, _ := optimisticEngine(t)
	e.Tape.Insert(0, NewOpenParen())
	e.Tape.Insert(5, NewEnd())
	e.Pointer, e.fwdSplitAt = 4, 4
	if l := e.optimisticLayout(m, []int{3, 2, 1}); l == nil || l.NFwd != 2 || len(l.Beneath)+len(l.After) != 0 {
		t.Errorf("(0 fold [add] (mk)) end: exact; got %+v", l)
	}
	// A carrier beneath rides as a LIVE entry: the run's stack holds its
	// value there, which the record places (NUR351).
	e, m, _ = optimisticEngine(t)
	e.Tape.Insert(0, withID(NewInteger(7)))
	e.Tape.Insert(1, withID(NewDynamicCarrier(TAny)))
	e.Pointer, e.fwdSplitAt = 5, 5
	if l := e.optimisticLayout(m, []int{4, 3, 2}); l == nil || len(l.Beneath) != 2 || !slices.Equal(l.Live, []int{1}) {
		t.Errorf("7 (mk) 0 fold [add] (mk): the carrier beneath is live; got %+v", l)
	}
	// A constant beneath and a literal after ride with it (NUR283).
	e, m, _ = optimisticEngine(t)
	e.Tape.Insert(0, withID(NewInteger(7)))
	e.Tape.Insert(5, withID(NewInteger(5)))
	e.Pointer, e.fwdSplitAt = 4, 4
	if l := e.optimisticLayout(m, []int{3, 2, 1}); l == nil || len(l.Beneath) != 1 || len(l.After) != 1 {
		t.Errorf("7 0 fold [add] (mk) 5: 7 beneath, 5 after; got %+v", l)
	}
}

// TestExecMatchPublishesTheOptimisticRecords pins where the two records ride:
// execMatch publishes the outer match around the dispatch and the layout
// around its own record — the analysis's carrier results see both — and
// unpublishes them when it returns.
func TestExecMatchPublishesTheOptimisticRecords(t *testing.T) {
	e, match, indices := optimisticEngine(t)
	match.Positions = indices
	saved := AnalysisImpl.CarrierResults
	t.Cleanup(func() { AnalysisImpl.CarrierResults = saved })
	var outer *OuterMatch
	var layout *DispatchLayout
	AnalysisImpl.CarrierResults = func(r *Registry, _ string, _ *Signature, args []Value, _ SrcPos, _ *Registry, _ bool) []Value {
		outer, layout = r.Check.OptimisticOuter, r.Check.LayoutFor(args)
		return nil
	}
	if err := e.execMatch(match); err != nil {
		t.Fatal(err)
	}
	if outer == nil || outer.Word != "fold" || layout == nil || layout.NFwd != 2 {
		t.Errorf("the record sees the outer match and the layout; got %+v, %+v", outer, layout)
	}
	if e.Registry.Check.OptimisticOuter != nil || e.Registry.Check.CurLayout != nil {
		t.Error("both are unpublished when the dispatch returns")
	}
}

// regionRecorder answers RegionResult for a chosen set of ids; everything
// else is the inactive no-op.
type regionRecorder struct {
	inactiveEmit
	region map[string]bool
}

func (m *regionRecorder) RegionResult(id string) bool { return m.region[id] }

// TestOptimisticOuterOverARegionSeat pins NUR340: an operand that is a
// runtime-counted region's modelled seat — a loop's `[:Integer]` for the
// one Integer `for 1 [1]` leaves — makes the match optimistic even where
// every operand's static type conforms, so a trap met under it is the
// word's rematch. An operand the recorder does not vouch for, or one with no
// id, leaves a conforming match unpublished.
func TestOptimisticOuterOverARegionSeat(t *testing.T) {
	e, match, indices := optimisticEngine(t)
	seat := withID(NewCarrier(TMap))
	concrete := &MatchResult{Sig: match.Sig, Args: []Value{match.Args[0], seat, match.Args[2]}, Name: "fold"}
	rec := &regionRecorder{region: map[string]bool{seat.ID: true}}
	e.Registry.Check.Emit = rec
	t.Cleanup(func() { e.Registry.Check.Emit = nil })
	if o := e.optimisticOuter(concrete, indices); o == nil || o.Word != "fold" || o.Vals[2].ID != seat.ID {
		t.Fatalf("a region seat at a conforming slot is an optimistic match; got %+v", o)
	}
	rec.region = map[string]bool{}
	if o := e.optimisticOuter(concrete, indices); o != nil {
		t.Errorf("a conforming match over no region seat publishes nothing, got %+v", o)
	}
	anon := &MatchResult{Sig: match.Sig, Args: []Value{match.Args[0], NewCarrier(TMap), match.Args[2]}, Name: "fold"}
	rec.region = map[string]bool{"": true}
	if o := e.optimisticOuter(anon, indices); o != nil {
		t.Errorf("an operand with no id is no region seat, got %+v", o)
	}
}
