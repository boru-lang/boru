package core

import "testing"

// The plan cache must answer exactly what PlanMatch answers. pmPlan holds
// it to that over a battery of dispatches — the miss that records a plan,
// then the hit that replays it — and the rest pins each branch of the key
// and the store.

func pmImpl(n int64) SigImpl {
	return Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return []Value{NewInteger(n)}, nil
	})
}

// pmReg is a registry with the words the plan tests dispatch.
func pmReg(t *testing.T) *Registry {
	t.Helper()
	r := w8reg(t)
	r.Register("pmadd",
		Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1, Returns: []*Type{TInteger}, Impl: pmImpl(1)},
		Signature{Args: []*Type{TString, TString}, BarrierPos: -1, Returns: []*Type{TInteger}, Impl: pmImpl(2)},
	)
	r.Register("pmstk", Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 0, Returns: []*Type{TInteger}, Impl: pmImpl(3)})
	r.Register("pmany", Signature{Args: []*Type{TAny}, BarrierPos: -1, Returns: []*Type{TInteger}, Impl: pmImpl(4)})
	r.Register("pmq",
		Signature{Args: []*Type{TAtom, TAny}, QuoteArgs: map[int]bool{0: true}, BarrierPos: -1, Returns: []*Type{TInteger}, Impl: pmImpl(5)},
		Signature{Args: []*Type{TString, TAny}, BarrierPos: -1, Returns: []*Type{TInteger}, Impl: pmImpl(6)},
	)
	// lte's shape: a dependent-type constructor overload with a type-literal
	// slot, and a value overload.
	r.Register("pmdep",
		Signature{Args: []*Type{TScalar, TScalar}, TypeArgs: map[int]bool{1: true}, BarrierPos: -1, Returns: []*Type{TScalar}, Impl: pmImpl(8)},
		Signature{Args: []*Type{TAny, TAny}, BarrierPos: -1, Returns: []*Type{TBoolean}, Impl: pmImpl(9)},
	)
	r.Register("pmpat", Signature{Args: []*Type{TInteger}, Patterns: map[int]Value{0: NewInteger(0)}, BarrierPos: -1, Returns: []*Type{TInteger}, Impl: pmImpl(7)})
	InstallFnDef(r, "pmfn", FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru([]Value{NewWord("n")}),
	}}})
	r.Defs.Push("pmx", NewInteger(3))
	r.Defs.PushType("PmT", TInteger, NewTypeLiteral(TInteger))
	return r
}

// pmAt is an engine over toks with the pointer at index i.
func pmAt(r *Registry, toks []Value, i int) *Engine {
	e := NewTop(r)
	e.Tape = NewTape(toks, StackHeadroom)
	e.Pointer = i
	return e
}

// pmPlan matches the word at the pointer afresh and twice through the plan
// cache, failing unless all three answers agree, and reports whether the
// second pass answered from a recorded plan.
func pmPlan(t *testing.T, e *Engine, w WordInfo) (sig *Signature, positions []int, specAt int, recorded bool) {
	t.Helper()
	fn := e.Registry.Lookup(w.Name)
	if fn == nil {
		t.Fatalf("%s is not a word", w.Name)
	}
	resolved := e.EffectiveResolved()
	inside := e.isInsidePendingForward()
	sig, positions, specAt = PlanMatch(patternDispatch{e}, e.Tape, e.Registry, fn, w, resolved, e.Pointer, inside, false, false)
	for pass := 0; pass < 2; pass++ {
		s, p, sp := planDispatch(patternDispatch{e}, e.Tape, e.Registry, fn, w, resolved, e.Pointer, inside)
		if s != sig || sp != specAt || (p == nil) != (positions == nil) || !equalInts(p, positions) {
			t.Fatalf("%s pass %d: plan cache answered (%v, %v, %d), PlanMatch (%v, %v, %d)", w.Name, pass, s != nil, p, sp, sig != nil, positions, specAt)
		}
	}
	if set := planSetFor(fn); set != nil {
		if key, ok := planKeyFor(set, patternDispatch{e}, e.Tape, w, resolved, e.Pointer, inside); ok {
			_, recorded = set.find(&key)
		}
	}
	return sig, positions, specAt, recorded
}

func pmWord(name string) Value { return NewWord(name) }

func pmVal(name string) Value { return NewWordRef(name) }

func TestDispatchPlanAgreesWithPlanMatch(t *testing.T) {
	w := func(name string) WordInfo { return WordInfo{Name: name, ArgCount: -1} }
	cases := []struct {
		name     string
		toks     []Value
		at       int
		w        WordInfo
		wantSig  bool
		recorded bool
	}{
		{"infix", []Value{NewInteger(1), pmWord("pmadd"), NewInteger(2)}, 1, w("pmadd"), true, true},
		{"forward", []Value{pmWord("pmadd"), NewInteger(1), NewInteger(2)}, 0, w("pmadd"), true, true},
		{"strings", []Value{NewString("a"), pmWord("pmadd"), NewString("b")}, 1, w("pmadd"), true, true},
		{"no match", []Value{NewString("a"), pmWord("pmadd"), NewInteger(2)}, 1, w("pmadd"), false, true},
		{"stack only", []Value{NewInteger(1), NewInteger(2), pmWord("pmstk")}, 2, w("pmstk"), true, true},
		{"statement end", []Value{NewInteger(1), pmWord("pmadd"), NewInteger(2), NewEnd()}, 1, w("pmadd"), true, true},
		{"window ends", []Value{NewInteger(1), NewInteger(2), pmWord("pmadd")}, 2, w("pmadd"), true, true},
		{"quoted name", []Value{pmWord("pmq"), pmWord("zq"), NewInteger(5)}, 0, w("pmq"), true, true},
		{"boolean name", []Value{pmWord("pmany"), pmWord("true")}, 0, w("pmany"), true, true},
		{"type name", []Value{pmWord("pmany"), pmWord("Integer")}, 0, w("pmany"), true, true},
		{"value binding", []Value{pmWord("pmany"), pmWord("pmx")}, 0, w("pmany"), true, true},
		{"fn binding", []Value{pmWord("pmany"), pmWord("pmadd")}, 0, w("pmany"), true, true},
		{"fn reference", []Value{pmWord("pmany"), pmVal("pmadd")}, 0, w("pmany"), true, true},
		{"fallback", []Value{pmWord("pmfn")}, 0, w("pmfn"), true, true},
		{"force stack", []Value{NewInteger(1), NewInteger(2), pmWord("pmadd")}, 2, WordInfo{Name: "pmadd", ArgCount: -1, ForceStack: true}, true, true},
		{"force forward", []Value{pmWord("pmadd"), NewInteger(1), NewInteger(2)}, 0, WordInfo{Name: "pmadd", ArgCount: -1, ForceForward: true}, true, true},
		{"arity", []Value{pmWord("pmadd"), NewInteger(1), NewInteger(2)}, 0, WordInfo{Name: "pmadd", ArgCount: 2}, true, true},
		{"type slot value", []Value{NewInteger(5), pmWord("pmdep"), NewInteger(1)}, 1, w("pmdep"), true, true},
		{"type slot name", []Value{pmWord("pmdep"), NewInteger(10), pmWord("Integer")}, 0, w("pmdep"), true, true},
		{"type slot body", []Value{pmWord("pmdep"), NewInteger(10), NewRecordType(NewOrderedMap())}, 0, w("pmdep"), true, true},
		{"type slot constraint", []Value{pmWord("pmdep"), NewInteger(10), NewDepScalar(DepLTE, NewInteger(3))}, 0, w("pmdep"), true, true},
		// Values the key cannot describe by their tag plan afresh.
		{"type literal on the stack", []Value{NewTypeLiteral(TInteger), pmWord("pmany")}, 1, w("pmany"), true, false},
		{"type literal forward", []Value{pmWord("pmany"), NewTypeLiteral(TInteger)}, 0, w("pmany"), true, false},
		{"type binding forward", []Value{pmWord("pmany"), pmWord("PmT")}, 0, w("pmany"), true, false},
		{"pattern word", []Value{pmWord("pmpat"), NewInteger(0)}, 0, w("pmpat"), true, false},
	}
	for _, c := range cases {
		// A registry per case: each word's plan set starts empty, so a case
		// is never left unrecorded by the set the earlier ones filled.
		e := pmAt(pmReg(t), c.toks, c.at)
		sig, _, _, recorded := pmPlan(t, e, c.w)
		if (sig != nil) != c.wantSig {
			t.Errorf("%s: matched = %v, want %v", c.name, sig != nil, c.wantSig)
		}
		if recorded != c.recorded {
			t.Errorf("%s: recorded = %v, want %v", c.name, recorded, c.recorded)
		}
	}
}

// TestDispatchPlanKeyUndescribableTokens: a token whose scan treatment reads
// more than a key holds — a sugar marker, a reach-collapsed value — leaves
// the dispatch to plan afresh, as does a window or stack value that is not
// decided by its tag.
func TestDispatchPlanKeyUndescribableTokens(t *testing.T) {
	r := pmReg(t)
	fn := r.Lookup("pmany")
	set := planSetFor(fn)
	if set == nil {
		t.Fatal("pmany is plannable")
	}
	reach := NewInteger(9)
	SetReachGroup(&reach, true)
	carrier := NewCarrier(TInteger)
	for _, tok := range []Value{NewSugar(SugarInfo{Kind: SugarUsurp}), reach, carrier} {
		e := pmAt(r, []Value{pmWord("pmany"), tok}, 0)
		if _, ok := planKeyFor(set, patternDispatch{e}, e.Tape, WordInfo{Name: "pmany", ArgCount: -1}, nil, 0, false); ok {
			t.Errorf("a key described the undescribable token %v", tok)
		}
	}
	// A pending forward and a following word are part of the key.
	e := pmAt(r, []Value{pmWord("pmany"), pmWord("zz")}, 0)
	a, _ := planKeyFor(set, patternDispatch{e}, e.Tape, WordInfo{Name: "pmany", ArgCount: -1}, nil, 0, false)
	b, _ := planKeyFor(set, patternDispatch{e}, e.Tape, WordInfo{Name: "pmany", ArgCount: -1}, nil, 0, true)
	if a == b || a.mods&planNextIsWord == 0 || b.mods&planInsideForward == 0 {
		t.Errorf("keys %+v / %+v miss the pending-forward or next-word bit", a, b)
	}
}

// TestDispatchPlanSetAdmission pins which signature lists plan from memory.
func TestDispatchPlanSetAdmission(t *testing.T) {
	r := pmReg(t)
	patWord := r.Lookup("pmpat")
	if planSetFor(patWord) != nil {
		t.Error("a word with a pattern slot planned from memory")
	}
	// The verdict is kept with the list: the next dispatch reads it back.
	if planSetFor(patWord) != nil {
		t.Error("a word with a pattern slot planned from memory on its second dispatch")
	}
	if planSetFor(&FnDefInfo{Signatures: r.Lookup("pmadd").Signatures}) != nil {
		t.Error("a payload with no plan store planned from memory")
	}
	// A copy of an aggregate carrying another list starts its own set.
	agg := r.Lookup("pmadd")
	first := planSetFor(agg)
	other := *agg
	other.Signatures = append([]Signature(nil), agg.Signatures...)
	if s := planSetFor(&other); s == nil || s == first || !sameList(s.sigs, other.Signatures) {
		t.Error("a rebuilt signature list reused the old list's plans")
	}
	wide := make([]*Type, planArgsMax+1)
	for i := range wide {
		wide[i] = TInteger
	}
	pat := NewInteger(1)
	minted := r.Types.MintType("PmMinted", TInteger)
	for _, c := range []struct {
		name string
		sig  Signature
	}{
		{"wide", Signature{Args: wide, BarrierPos: 0}},
		{"pattern", Signature{Params: []FnParam{{Type: TInteger, Pattern: &pat}}}},
		{"minted type slot", Signature{Params: []FnParam{{Type: minted}}, TypeArgs: map[int]bool{0: true}}},
		{"untyped", Signature{Params: []FnParam{{Name: "x"}}}},
		{"minted", Signature{Params: []FnParam{{Type: minted}}}},
		{"options", Signature{Params: []FnParam{{Type: TOptions}}}},
		{"content", Signature{Params: []FnParam{{Type: TType}}}},
	} {
		if s := newPlanSet([]Signature{c.sig}); s.maxArgs >= 0 {
			t.Errorf("%s signature admitted to planning", c.name)
		}
	}
	if s := newPlanSet([]Signature{{Fallback: true}, {Args: []*Type{TInteger, TAny}, BarrierPos: 1}}); s.maxArgs != 2 || s.maxBarrier != 1 {
		t.Errorf("bounds = (%d args, barrier %d), want (2, 1)", s.maxArgs, s.maxBarrier)
	}
}

// TestDispatchPlanRecordBounds: a full set records nothing more, and an
// answer apply would not reproduce is never recorded.
func TestDispatchPlanRecordBounds(t *testing.T) {
	r := pmReg(t)
	fn := r.Lookup("pmadd")
	set := planSetFor(fn)
	e := pmAt(r, []Value{NewInteger(1), pmWord("pmadd"), NewInteger(2)}, 1)
	sig := &fn.Signatures[0]
	// Positions apply cannot rebuild: the forward slot is not the next token.
	recordPlan(fn, set, planKey{argCount: 7}, sig, []int{5, 0}, -1, e.Tape, 1)
	if s := planSetFor(fn); len(s.plans) != 0 {
		t.Error("an answer of another shape was recorded")
	}
	for i := 0; i < planSetMax+2; i++ {
		cur := planSetFor(fn)
		recordPlan(fn, cur, planKey{argCount: 100 + i}, nil, nil, -1, e.Tape, 1)
	}
	if s := planSetFor(fn); len(s.plans) != planSetMax {
		t.Errorf("set holds %d plans, want the first %d", len(s.plans), planSetMax)
	}
	if !equalInts(nil, nil) || equalInts([]int{1}, []int{1, 2}) || equalInts([]int{1, 2}, []int{1, 3}) || !equalInts([]int{4}, []int{4}) {
		t.Error("equalInts")
	}
}

// TestDispatchPlanRunsAgree: a program run twice — the second run answering
// from the plans the first recorded — computes what the first computed.
func TestDispatchPlanRunsAgree(t *testing.T) {
	r := pmReg(t)
	prog := []Value{
		NewInteger(1), pmWord("pmadd"), NewInteger(2), NewEnd(),
		pmWord("pmadd"), NewString("a"), NewString("b"), NewEnd(),
		NewInteger(3), NewInteger(4), pmWord("pmstk"), NewEnd(),
		pmWord("pmany"), pmWord("pmx"),
	}
	var runs [2]string
	for i := range runs {
		out, err := NewTop(r).Run(prog)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		runs[i] = renderAll(out)
	}
	if runs[0] != runs[1] || runs[0] != "1 | 2 | 3 | 4" {
		t.Errorf("runs = %q / %q, want both 1 | 2 | 3 | 4", runs[0], runs[1])
	}
}
