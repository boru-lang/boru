package core

import (
	"slices"
	"testing"
)

// TestWithRenderedPrefix pins the failed window's widening to the stack
// prefix its report renders (NUR351): a prefix value the pass holds as a
// carrier widens the stack-only window to the whole rendered prefix, so the
// failure is the runtime rematch's and its report names the run's value.
func TestWithRenderedPrefix(t *testing.T) {
	carrier, zero := withID(NewDynamicCarrier(TAny)), withID(NewInteger(0))
	build := func(tape ...Value) (*Engine, *FnDefInfo) {
		r := covRegistry(t, func(r *Registry) {
			r.Register("w", Signature{Args: []*Type{TMap}, BarrierPos: -1, Impl: Go(nilHandler)})
		})
		e := NewTop(r)
		e.Tape = NewTape(tape, StackHeadroom)
		for i, v := range tape {
			if IsWord(v) {
				e.Pointer = i
			}
		}
		return e, r.Lookup("w")
	}
	e, fn := build(carrier, zero, NewWord("w"))
	if got := e.withRenderedPrefix([]int{1}, fn); !slices.Equal(got, []int{0, 1}) {
		t.Errorf("`f end 0 w`: the carrier beneath joins the window, got %v", got)
	}
	cases := []struct {
		name   string
		tape   []Value
		window []int
	}{
		{"a window past the word", []Value{carrier, NewWord("w"), zero}, []int{2}},
		{"an operand written after the word", []Value{carrier, zero, NewWord("w"), withID(NewInteger(5))}, []int{1}},
		{"a window holding the whole prefix", []Value{carrier, zero, NewWord("w")}, []int{0, 1}},
		{"a concrete prefix", []Value{withID(NewInteger(7)), zero, NewWord("w")}, []int{1}},
		{"a window that is not the prefix's top", []Value{carrier, zero, NewWord("w")}, []int{0}},
	}
	for _, c := range cases {
		e, fn := build(c.tape...)
		if got := e.withRenderedPrefix(c.window, fn); !slices.Equal(got, c.window) {
			t.Errorf("%s: the window stands, got %v", c.name, got)
		}
	}
}

// TestForwardValueFits pins the interpreter's forward-scan fit over a run
// value (NUR357): a form or quoted slot captures any token, an Any slot
// takes any value, a typed slot a value of its type, and a bare type node
// is refused at a concrete-payload slot unless the slot takes types.
func TestForwardValueFits(t *testing.T) {
	mapSig := &Signature{Args: []*Type{TMap}, BarrierPos: -1}
	NormalizeSig(mapSig)
	anySig := &Signature{Args: []*Type{TAny}, BarrierPos: -1}
	NormalizeSig(anySig)
	intLit := NewTypeLiteral(TInteger)
	cases := []struct {
		name string
		sig  *Signature
		k    int
		v    Value
		want bool
	}{
		{"no signature", nil, 0, NewInteger(0), false},
		{"a position past the signature", mapSig, 1, NewInteger(0), false},
		{"a negative position", mapSig, -1, NewInteger(0), false},
		{"a Map at the Map slot", mapSig, 0, NewMap(NewOrderedMap()), true},
		{"an Integer at the Map slot", mapSig, 0, NewInteger(0), false},
		{"anything at an Any slot", anySig, 0, NewInteger(0), true},
		{"a type node at a concrete slot", &Signature{Args: []*Type{TInteger}}, 0, intLit, false},
		{"a type node at a type slot", &Signature{Args: []*Type{TInteger}, TypeArgs: map[int]bool{0: true}}, 0, intLit, true},
		{"a form slot", &Signature{Args: []*Type{TMap}, FormArgs: map[int]bool{0: true}}, 0, NewInteger(0), true},
		{"a quoted slot", &Signature{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}}, 0, NewInteger(0), true},
	}
	for _, c := range cases {
		if c.sig != nil && len(c.sig.Params) == 0 {
			NormalizeSig(c.sig)
		}
		if got := ForwardValueFits(c.sig, c.k, c.v); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	fits := []ForwardFit{{Sig: mapSig, Idx: 0}, {Sig: anySig, Idx: 0}}
	if !ForwardFitsAll(nil, NewInteger(0)) || !ForwardFitsAll(fits, NewMap(NewOrderedMap())) || ForwardFitsAll(fits, NewInteger(0)) {
		t.Error("every fit must hold: none, both, one missed")
	}
}

// fitWord registers w's three overloads — (Integer, Integer), (Map) and a
// stack-only (String) — and returns the engine over `m <dyn> w` as the
// forward collection leaves it: the written dynamic operand laid out beneath
// the word, the split recorded for it, the Map beneath.
func fitEngine(t *testing.T, beneath ...Value) (*Engine, *MatchResult, *FnDefInfo) {
	t.Helper()
	r := covRegistry(t, func(r *Registry) {
		r.Register("w",
			Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1, Impl: Go(nilHandler)},
			Signature{Args: []*Type{TMap}, BarrierPos: -1, Impl: Go(nilHandler)},
			Signature{Args: []*Type{TString}, BarrierPos: 0, Impl: Go(nilHandler)},
		)
	})
	r.Check.Emit = layoutRecorder{}
	dyn := withID(NewDynamicCarrier(TAny))
	tape := append(append([]Value{}, beneath...), dyn, NewWord("w"))
	e := NewTop(r)
	e.Tape = NewTape(tape, StackHeadroom)
	e.Pointer = len(tape) - 1
	e.fwdSplitAt, e.fwdSplitN, e.fwdSplitPos = e.Pointer, 1, e.Tape.At(e.Pointer).Pos()
	fn := r.Lookup("w")
	var mapSig *Signature
	for i := range fn.Signatures {
		if fn.Signatures[i].TotalArgs() == 1 && fn.Signatures[i].BarrierPos != 0 {
			mapSig = &fn.Signatures[i]
		}
	}
	return e, &MatchResult{Sig: mapSig, Args: []Value{dyn}, Name: "w"}, fn
}

func nilHandler([]Value, map[string]Value, []Value, *Registry) ([]Value, error) { return nil, nil }

// TestForwardFitsListsTheViableCandidates pins the fits a dispatch
// publishes for a gradual operand it collected forward (NUR357): the
// candidates whose collection, stopped at the operand, could match over the
// stack beneath — a stack-only overload never reaches the operand, and one
// needing more values than the stack holds fails whatever the run holds.
func TestForwardFitsListsTheViableCandidates(t *testing.T) {
	e, match, _ := fitEngine(t, withID(NewMap(NewOrderedMap())))
	fits := e.forwardFits(match)
	if len(fits) != 1 || len(fits[0]) != 1 || fits[0][0].Sig != match.Sig || fits[0][0].Idx != 0 {
		t.Fatalf("`{} w y`: the Map overload alone, got %+v", fits)
	}
	restore := e.publishForwardFits(match)
	if got := e.Registry.Check.FitsFor(match.Args); len(got[0]) != 1 {
		t.Errorf("published for the dispatch's own operands, got %+v", got)
	}
	if e.Registry.Check.FitsFor([]Value{match.Args[0]}) != nil || e.Registry.Check.FitsFor(nil) != nil {
		t.Error("another operand slice reads nothing")
	}
	restore()
	if e.Registry.Check.CurFits != nil {
		t.Error("the restore unpublishes the fits")
	}

	// Nothing beneath: no candidate can take the stack, but the operand is
	// still listed — its call's island serves the no-match.
	e, match, _ = fitEngine(t)
	if fits := e.forwardFits(match); fits == nil || len(fits[0]) != 0 {
		t.Errorf("`w y` over nothing: listed, with no fit; got %+v", fits)
	}
	// A value beneath the Map slot refuses: no alternative there either.
	e, match, _ = fitEngine(t, withID(NewInteger(3)))
	if fits := e.forwardFits(match); len(fits[0]) != 0 {
		t.Errorf("`3 w y`: 3 misses Map, got %+v", fits)
	}
	// Two values beneath: the (Integer, Integer) overload's collection is
	// viable over a gradual pair.
	e, match, _ = fitEngine(t, withID(NewDynamicCarrier(TAny)), withID(NewDynamicCarrier(TAny)))
	if fits := e.forwardFits(match); len(fits[0]) != 2 {
		t.Errorf("two gradual values beneath: both one-slot-short candidates, got %+v", fits)
	}
	// A region's seat beneath holds any count: every reaching candidate.
	reg := withID(NewCarrier(TList))
	e, match, _ = fitEngine(t, reg)
	e.Registry.Check.Emit = &regionRecorder{region: map[string]bool{reg.ID: true}}
	if fits := e.forwardFits(match); len(fits[0]) != 2 {
		t.Errorf("a region beneath: unbounded, got %+v", fits)
	}

	declines := []struct {
		name string
		prep func(*Engine, *MatchResult) *MatchResult
	}{
		{"no split", func(e *Engine, m *MatchResult) *MatchResult { e.fwdSplitN = 0; return m }},
		{"no signature", func(_ *Engine, m *MatchResult) *MatchResult { return &MatchResult{Args: m.Args, Name: "w"} }},
		{"a split wider than the operands", func(e *Engine, m *MatchResult) *MatchResult { e.fwdSplitN = 2; return m }},
		{"not a word", func(e *Engine, m *MatchResult) *MatchResult {
			e.Tape.Set(e.Pointer, NewInteger(1))
			e.fwdSplitPos = e.Tape.At(e.Pointer).Pos()
			return m
		}},
		{"no such word", func(_ *Engine, m *MatchResult) *MatchResult {
			return &MatchResult{Sig: m.Sig, Args: m.Args, Name: "nope"}
		}},
		{"a concrete operand", func(_ *Engine, m *MatchResult) *MatchResult {
			return &MatchResult{Sig: m.Sig, Args: []Value{withID(NewMap(NewOrderedMap()))}, Name: "w"}
		}},
		{"a proven carrier", func(_ *Engine, m *MatchResult) *MatchResult {
			return &MatchResult{Sig: m.Sig, Args: []Value{withID(NewCarrier(TMap))}, Name: "w"}
		}},
	}
	for _, c := range declines {
		e, match, _ := fitEngine(t, withID(NewMap(NewOrderedMap())))
		if fits := e.forwardFits(c.prep(e, match)); fits != nil {
			t.Errorf("%s: no fits, got %+v", c.name, fits)
		}
	}
	// The matched candidate is Chosen; a matched copy (a user fn's frame
	// sig) names none of fn's own, and every candidate may be the pass's.
	e, match, _ = fitEngine(t, withID(NewMap(NewOrderedMap())))
	if fits := e.forwardFits(match); !fits[0][0].Chosen {
		t.Errorf("the Map overload is the pass's: %+v", fits)
	}
	cp := *match.Sig
	match.Sig = &cp
	if fits := e.forwardFits(match); len(fits[0]) != 1 || !fits[0][0].Chosen {
		t.Errorf("a copy: every candidate chosen, got %+v", fits)
	}
	// A module word's own registry names it (MatchResult.Reg).
	e, match, _ = fitEngine(t, withID(NewMap(NewOrderedMap())))
	match.Reg = e.Registry
	if fits := e.forwardFits(match); len(fits[0]) != 1 {
		t.Errorf("the match's registry: got %+v", fits)
	}
	// A plain pass publishes nothing, and neither does a dispatch with no
	// fits; both restores are no-ops.
	e, match, _ = fitEngine(t, withID(NewMap(NewOrderedMap())))
	e.Registry.Check.Emit = nil
	e.publishForwardFits(match)()
	e.Registry.Check.Emit = layoutRecorder{}
	e.fwdSplitN = 0
	e.publishForwardFits(match)()
	if e.Registry.Check.CurFits != nil {
		t.Error("nothing published")
	}
}

// TestStackBeneathIsTheInterpretersStack pins the stack a stopped
// collection draws from: the stack operands, then what lies beneath them,
// top first — or no proof, where the operands are not laid out beneath the
// word or a region's seat holds any count.
func TestStackBeneathIsTheInterpretersStack(t *testing.T) {
	a, b := withID(NewInteger(1)), withID(NewInteger(2))
	e, match, _ := fitEngine(t, a, b)
	stack, unbounded := e.stackBeneath(match, 1)
	if unbounded || len(stack) != 2 || stack[0].ID != b.ID || stack[1].ID != a.ID {
		t.Errorf("`1 2 w y`: [2 1], got %v %v", stack, unbounded)
	}
	e, match, _ = fitEngine(t, a, NewMark("m"))
	two := &MatchResult{Sig: match.Sig, Args: []Value{match.Args[0], a}, Name: "w"}
	if _, unbounded := e.stackBeneath(two, 1); !unbounded {
		t.Error("operands not where the layout puts them: no proof")
	}
	e, match, _ = fitEngine(t)
	if _, unbounded := e.stackBeneath(&MatchResult{Args: []Value{match.Args[0], a, b}}, 1); !unbounded {
		t.Error("more operands than values beneath: no proof")
	}
	sig := &Signature{Args: []*Type{TInteger, TInteger}}
	NormalizeSig(sig)
	if altViable(sig, 0, []Value{NewInteger(1)}, false) || !altViable(sig, 0, []Value{NewInteger(1)}, true) ||
		altViable(sig, 0, []Value{NewString("s"), NewInteger(1)}, false) || !altViable(sig, 1, []Value{NewInteger(1)}, false) {
		t.Error("viable: enough values, or any count; none refusing its slot")
	}
	if !fitProven(sig, 0, NewCarrier(TInteger)) || fitProven(sig, 0, NewDynamicCarrier(TInteger)) || fitProven(sig, 0, NewCarrier(TAny)) {
		t.Error("proven: a strict carrier of the slot's type only")
	}
	q := &Signature{Args: []*Type{TAtom, TAny}, QuoteArgs: map[int]bool{0: true}}
	NormalizeSig(q)
	if !fitProven(q, 0, NewCarrier(TAny)) || !fitProven(q, 1, NewCarrier(TAny)) {
		t.Error("a quoted slot and an Any slot take any value")
	}
}
