package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur217_285_test.go pins the recorder helpers NUR217 and NUR285 added: a
// multi-output word's first result bound by a def whose rest a later event
// consumed (restConsumed, restAllConsumed), the claim a shuffled copy
// carries (shuffleSource, shuffledFrom), a root read's consumer and its
// deepest-operand test (rootReadConsumer, readIsDeepestOperand), a
// residual-held capture's deferral (residualAfterRead), a code body's
// captures at the root (rootCapturesBound) and the replay window's data
// lookups (replayWordLookups).

func n217Shuffle(word string, mapping []int, ops ...EmitOperand) EmitEvent {
	sig := &core.Signature{Args: make([]*core.Type, len(ops)), ReturnsFn: core.ReturnsIdentity(mapping...)}
	return EmitEvent{seq: 9, kind: evCall, call: emitCall{word: word, sig: sig, nout: len(mapping), ops: ops}}
}

func TestRestConsumed(t *testing.T) {
	es := NewEmitState()
	dup := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "dup", nout: 2}})
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "typeof", nout: 1, ops: []EmitOperand{EventOperand(dup, 0)}}})
	if es.restConsumed(dup) {
		t.Error("the first result's consumer consumes no rest")
	}
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "drop", ops: []EmitOperand{EventOperand(dup, 1)}}})
	if !es.restConsumed(dup) {
		t.Error("the drop consumed the second result")
	}
	all := []*EmitEvent{{kind: evCall, call: emitCall{ops: []EmitOperand{EventOperand(4, 1)}}}}
	if !restAllConsumed(all, 4, 2) || restAllConsumed(all, 4, 3) {
		t.Error("every result past the first must be some event's operand")
	}
}

func TestShuffleSource(t *testing.T) {
	src := EventOperand(3, 0)
	dup := n217Shuffle("dup", []int{0, 0}, src)
	for idx := 0; idx < 2; idx++ {
		if op, ok := shuffleSource(&dup, idx); !ok || op.kind != opEvent || op.idx != 3 {
			t.Errorf("dup's result %d copies its argument, got %+v %v", idx, op, ok)
		}
	}
	swap := n217Shuffle("swap", []int{0, 1}, ConstOperand(1), src)
	if op, ok := shuffleSource(&swap, 1); !ok || op.kind != opEvent || op.idx != 3 {
		t.Errorf("swap's second result is its second argument, got %+v %v", op, ok)
	}
	fresh := n217Shuffle("dup", nil, src)
	fresh.call.sig.ReturnsFn = func([]core.Value, *core.Registry) []core.Value { return []core.Value{core.NewCarrier(core.TAny)} }
	noSig := dup
	noSig.call.sig = nil
	noFn := n217Shuffle("dup", []int{0, 0}, src)
	noFn.call.sig.ReturnsFn = nil
	other := n217Shuffle("typeof", []int{0}, src)
	for name, c := range map[string]struct {
		ev  *EmitEvent
		idx int
	}{
		"no event":            {nil, 0},
		"not a shuffle":       {&other, 0},
		"no signature":        {&noSig, 0},
		"no returns":          {&noFn, 0},
		"a result past them":  {&dup, 2},
		"a result not a copy": {&fresh, 0},
	} {
		if _, ok := shuffleSource(c.ev, c.idx); ok {
			t.Errorf("%s: no source", name)
		}
	}
}

func TestShuffledFrom(t *testing.T) {
	es := NewEmitState()
	mk := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "mk", nout: 1}})
	dup := n217Shuffle("dup", []int{0, 0}, EventOperand(mk, 0))
	dup.seq = 0
	d := es.appendEvent(dup)
	if seq, ok := es.shuffledFrom(producer{seq: d, idx: 1}); !ok || seq != mk {
		t.Errorf("dup's copy is mk's result: %d %v", seq, ok)
	}
	if _, ok := es.shuffledFrom(producer{seq: mk}); ok {
		t.Error("no shuffle stands between")
	}
	lit := n217Shuffle("dup", []int{0, 0}, ConstOperand(0))
	lit.seq = 0
	l := es.appendEvent(lit)
	if _, ok := es.shuffledFrom(producer{seq: l}); ok {
		t.Error("a copied literal is no event's result")
	}
	swap := n217Shuffle("swap", []int{0, 1}, EventOperand(d, 1), ConstOperand(0))
	swap.seq = 0
	s := es.appendEvent(swap)
	if seq, ok := es.shuffledFrom(producer{seq: s}); !ok || seq != mk {
		t.Errorf("a chain of copies ends at mk: %d %v", seq, ok)
	}
	two := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "pair", nout: 2}})
	second := n217Shuffle("dup", []int{0, 0}, EventOperand(two, 1))
	second.seq = 0
	c := es.appendEvent(second)
	if _, ok := es.shuffledFrom(producer{seq: c}); ok {
		t.Error("a copy of another event's second result names no first result")
	}
}

func TestRootReadConsumerMatchesTheResult(t *testing.T) {
	events := []EmitEvent{
		{kind: evCall, call: emitCall{word: "drop", ops: []EmitOperand{EventOperand(2, 1)}}},
		{kind: evCall, call: emitCall{word: "typeof", nout: 1, ops: []EmitOperand{EventOperand(2, 0)}}},
	}
	if ci, direct := rootReadConsumer(events, "j", 2, 0, nil); ci != 1 || !direct {
		t.Errorf("the typeof consumes the read, not the drop: %d %v", ci, direct)
	}
	slotted := []EmitEvent{
		{kind: evCall, call: emitCall{word: "drop", ops: []EmitOperand{localOperand(5)}}},
		{kind: evCall, call: emitCall{word: "typeof", nout: 1, ops: []EmitOperand{localOperand(4)}}},
	}
	if ci, _ := rootReadConsumer(slotted, "j", 2, 0, map[int]int{2: 4}); ci != 1 {
		t.Errorf("a promoted result lives at its base slot plus its index: %d", ci)
	}
}

func TestReadIsDeepestOperand(t *testing.T) {
	read := EventOperand(3, 0)
	call := func(c emitCall) *EmitEvent { return &EmitEvent{kind: evCall, call: c} }
	for name, c := range map[string]struct {
		ev   *EmitEvent
		want bool
	}{
		"a call's deepest":      {call(emitCall{ops: []EmitOperand{ConstOperand(1), read}}), true},
		"a call's top":          {call(emitCall{ops: []EmitOperand{read, ConstOperand(1)}}), false},
		"another result":        {call(emitCall{ops: []EmitOperand{EventOperand(3, 1)}}), false},
		"a mixed window":        {call(emitCall{dynMixed: true, ops: []EmitOperand{read}}), false},
		"a list":                {call(emitCall{makeList: true, ops: []EmitOperand{read}}), false},
		"a dynamic apply":       {call(emitCall{dynApply: 1, ops: []EmitOperand{read}}), false},
		"no operands":           {call(emitCall{}), false},
		"a user call's deepest": {&EmitEvent{kind: evCallUser, uc: emitUserCall{ops: []EmitOperand{read}}}, true},
		"a branch":              {&EmitEvent{kind: evBranch, br: &emitBranch{}}, false},
	} {
		if got := readIsDeepestOperand(c.ev, 3, 0); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

func TestResidualAfterRead(t *testing.T) {
	outs := []EmitOperand{ConstOperand(1), localOperand(2), ConstOperand(3)}
	pushed := &deoptPoint{atPush: true, slot: 2}
	if got := residualAfterRead(outs, pushed, -1); len(got) != 1 || got[0].kind != opConst || got[0].idx != 3 {
		t.Errorf("the entries after the read's own: %+v", got)
	}
	for name, c := range map[string]struct {
		d  *deoptPoint
		ci int
	}{
		"tested before its statement": {&deoptPoint{slot: 2}, -1},
		"a consumed read":             {pushed, 0},
		"a producer's read":           {&deoptPoint{atPush: true, slot: -1}, -1},
		"a slot the residual lacks":   {&deoptPoint{atPush: true, slot: 7}, -1},
	} {
		if got := residualAfterRead(outs, c.d, c.ci); len(got) != len(outs) {
			t.Errorf("%s: the whole residual, got %+v", name, got)
		}
	}
}

func TestRootCapturesBound(t *testing.T) {
	names := map[string]bool{"j": true}
	bind := EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "j"}}
	es := NewEmitState()
	es.appendEvent(bind)
	rec := &fnUnitRec{}
	if !es.rootCapturesBound(rec, names) || !rec.rootCaptures {
		t.Error("a def the root makes itself is the capture's binding")
	}
	armed := NewEmitState()
	armed.appendEvent(EmitEvent{kind: evBranch, br: &emitBranch{then: &EmitFragment{events: []EmitEvent{{kind: evLoop, loop: &emitLoop{body: &EmitFragment{events: []EmitEvent{bind}}}}}}}})
	if armed.rootCapturesBound(&fnUnitRec{}, names) {
		t.Error("a def inside a branch arm the run may skip is not")
	}
	between := NewEmitState()
	between.frames = append(between.frames, []EmitEvent{bind})
	if between.rootCapturesBound(&fnUnitRec{}, names) {
		t.Error("nor is one a frame between the root and the body makes")
	}
	if fragmentBinds(nil, names) || fragmentBinds(&EmitFragment{events: []EmitEvent{{kind: evCall}}}, names) {
		t.Error("no fragment, or no def of the name, binds nothing")
	}
}

func TestReplayWordLookups(t *testing.T) {
	ops := []EmitOperand{dynScopeOperand(1), ConstOperand(2), dynScopeOperand(3)}
	got := replayWordLookups(append([]EmitOperand(nil), ops...), 2, []DynFrameWord{{}, {Name: "j"}})
	if got[0].kind != opDynScope || got[2].kind != opDataScope || got[2].idx != 3 {
		t.Errorf("the window's word read takes the data lookup, the rest keep theirs: %+v", got)
	}
	for name, c := range map[string]struct {
		w     int
		words []DynFrameWord
	}{
		"no replay":                  {0, nil},
		"a table of the wrong size":  {2, []DynFrameWord{{Name: "j"}}},
		"a window past the residual": {4, make([]DynFrameWord, 4)},
	} {
		if out := replayWordLookups(append([]EmitOperand(nil), ops...), c.w, c.words); out[2].kind != opDynScope {
			t.Errorf("%s: nothing re-keyed, got %+v", name, out)
		}
	}
}
