package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur356_remainder_test.go pins the helpers of NUR356's and NUR357's
// remainders over synthetic events: the sure match of a poly over constants
// and literals as written (literalMatchSure), the effect classification of
// a poly (polyEffectful's guard), the word reads a split's report puts back
// (splitWords), a pending literal in a user call's window (callWindowOp) and
// the def-making branch a statement island starts past (quietBranchAt). The
// lang suite drives them end to end (nur356_remainder_test.go,
// nur357_branch_island_test.go).

func nur356Registry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Register("lw", core.Signature{Args: []*core.Type{core.TList}, Returns: []*core.Type{core.TList}})
	r.Register("rw", core.Signature{Args: []*core.Type{core.TList}, NoEvalArgs: map[int]bool{0: true}, Returns: []*core.Type{core.TList}})
	r.Register("tw", core.Signature{Args: []*core.Type{core.TList, core.TType}, Returns: []*core.Type{core.TList}})
	r.Register("iw", core.Signature{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}})
	quiet := core.Signature{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}}
	loud := core.Signature{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, CompileEffect: core.CompileSideEffect}
	r.Register("mixed", quiet, loud)
	r.Register("loud", loud)
	r.Register("raising", core.Signature{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, CompileEffect: core.CompileValueDiverges}, quiet)
	return r
}

func TestLiteralMatchSure(t *testing.T) {
	r := nur356Registry(t)
	es := NewEmitState()
	defer es.BindRegistry(r)()
	lit := deoptLit(core.NewList([]core.Value{deoptTok("x", 7)}), 6)
	lit.Eval = true
	body := []core.Value{deoptTok("lw", 1), lit}
	asm := EmitEvent{seq: 1, kind: evCall, call: emitCall{makeList: true, nout: 1, pos: deoptAt(6)}}
	notAsm := EmitEvent{seq: 1, kind: evCall, call: emitCall{word: "iw", nout: 1, pos: deoptAt(6)}}
	poly := func(word string, ops ...EmitOperand) EmitEvent {
		return EmitEvent{seq: 2, kind: evCall, call: emitCall{word: word, poly: true, nout: 1, ops: ops}}
	}
	sure := func(events []EmitEvent) bool { return es.literalMatchSure(events, len(events)-1, body, nil) }
	es.consts = []core.Value{core.NewInteger(5)}
	ti := es.internType(core.NewTypeLiteral(core.TInteger))
	if !sure([]EmitEvent{asm, poly("lw", EventOperand(1, 0))}) {
		t.Error("a list slot matches the literal as written")
	}
	if sure([]EmitEvent{asm, poly("rw", EventOperand(1, 0))}) {
		t.Error("a code-body slot takes it raw: never evaluated")
	}
	if !sure([]EmitEvent{asm, poly("tw", EventOperand(1, 0), EmitOperand{kind: opType, idx: ti})}) {
		t.Error("a type operand the run pushes as the pass does")
	}
	for name, events := range map[string][]EmitEvent{
		"a committed call":      {asm, {seq: 2, kind: evCall, call: emitCall{word: "lw", sig: &core.Signature{}, ops: []EmitOperand{EventOperand(1, 0)}}}},
		"a dynamic apply":       {asm, {seq: 2, kind: evCall, call: emitCall{word: "lw", poly: true, dynApply: 1, ops: []EmitOperand{EventOperand(1, 0)}}}},
		"an unknown word":       {asm, poly("nope", EventOperand(1, 0))},
		"no literal":            {poly("iw", ConstOperand(0))},
		"a mismatch":            {poly("iw", EventOperand(1, 0)), asm},
		"no assembly":           {notAsm, poly("lw", EventOperand(1, 0))},
		"an operand of no home": {asm, poly("lw", localOperand(0))},
		"an unknown type":       {asm, poly("tw", EventOperand(1, 0), EmitOperand{kind: opType, idx: 9})},
		"a missing assembly":    {poly("lw", EventOperand(7, 0))},
	} {
		if sure(events) {
			t.Errorf("%s: no sure match", name)
		}
	}
	es.runTypes = map[string]bool{core.TInteger.ID: true}
	if _, ok := es.typeOperand(ti, nil); ok {
		t.Error("a node the run installs anew proves nothing")
	}
	es.runTypes = nil
	if typ, ok := es.typeOperand(ti, r); !ok || !typ.Equal(core.TInteger) {
		t.Errorf("the unit's registry first: %v %v", typ, ok)
	}
	foo := r.Types.MintType("Foo", core.TIdeal)
	if typ, ok := es.typeOperand(es.internType(core.NewTypeLiteral(foo)), nil); !ok || !typ.Equal(foo) {
		t.Errorf("a type the program minted: %v %v", typ, ok)
	}
	var none EmitState
	if none.polyWord(&emitCall{word: "lw"}) != nil {
		t.Error("no registry holds no word")
	}
	if !rawSlot(&core.Signature{QuoteArgs: map[int]bool{0: true}}, 0) || rawSlot(&core.Signature{}, 0) {
		t.Error("rawSlot")
	}
}

func TestPolyEffectfulGuard(t *testing.T) {
	r := nur356Registry(t)
	es := NewEmitState()
	defer es.BindRegistry(r)()
	for _, tc := range []struct {
		word          string
		effect, guard bool
	}{
		{"iw", false, false},
		{"loud", true, false},
		{"mixed", false, true},
		{"raising", true, false},
		{"nope", true, false},
	} {
		c := &emitCall{word: tc.word, poly: true, nout: 1}
		if got := es.polyEffectful(c); got != tc.effect || c.quietGuard != tc.guard {
			t.Errorf("%s: effect %v guard %v, want %v %v", tc.word, got, c.quietGuard, tc.effect, tc.guard)
		}
	}
	ev := EmitEvent{kind: evCall, call: emitCall{word: "mixed", poly: true, nout: 1}}
	if es.mayEffect(&ev, effectScope{seen: map[int]bool{}, binds: anyBind}) || !ev.call.quietGuard {
		t.Error("a mixed poly is judged quiet and guarded")
	}
	committed := EmitEvent{kind: evCall, call: emitCall{word: "loud", nout: 1, sig: &r.Lookup("loud").Signatures[0]}}
	if !es.mayEffect(&committed, effectScope{seen: map[int]bool{}, binds: anyBind}) {
		t.Error("a committed effectful native has an effect")
	}
}

func TestSplitWordsAndWindowLiteral(t *testing.T) {
	es := NewEmitState()
	word := deoptTok("each", 1)
	lRead := deoptTok("l", 9)
	body := []core.Value{word, deoptLit(core.NewInteger(3), 6), lRead}
	lw := &lowerer{landingBody: body}
	sp := &PolySplit{NFwd: 2}
	got := lw.splitWords(sp, deoptAt(1), []core.SrcPos{deoptAt(6), deoptAt(9)})
	if got == sp || len(got.Words) != 1 || got.Words[1].Pos() != deoptAt(9) {
		t.Errorf("the word read at 9 is put back: %v", got)
	}
	first := deoptTok("k", 12)
	body2 := []core.Value{word, first, lRead}
	lw2 := &lowerer{landingBody: body2}
	merged := lw2.splitWords(&PolySplit{NFwd: 2, Words: map[int]core.Value{0: first}}, deoptAt(1), []core.SrcPos{deoptAt(12), deoptAt(9)})
	if len(merged.Words) != 2 {
		t.Errorf("the layout's own word stays beside the read: %v", merged.Words)
	}
	known := &PolySplit{NFwd: 2, Words: map[int]core.Value{1: lRead}}
	if lw.splitWords(known, deoptAt(1), []core.SrcPos{deoptAt(6), deoptAt(9)}) != known {
		t.Error("a word the layout already names adds nothing")
	}
	if lw.splitWords(nil, deoptAt(1), nil) != nil || lw.splitWords(sp, deoptAt(1), nil) != sp {
		t.Error("no split, or no positions: unchanged")
	}
	if lw.splitWords(sp, deoptAt(4), []core.SrcPos{deoptAt(9)}) != sp {
		t.Error("no token at the word: unchanged")
	}
	if _, _, ok := siblingTokens(nil, deoptAt(1)); ok {
		t.Error("an empty body holds no token")
	}
	a := deoptLit(core.NewInteger(1), 2)
	a.ID = "A"
	es.readPos = map[string]core.SrcPos{"A": deoptAt(9)}
	if ps := es.writtenPositions([]core.Value{a, deoptLit(core.NewInteger(2), 4)}, 2); ps[0] != deoptAt(9) || ps[1] != deoptAt(4) {
		t.Errorf("a read's position, else its own: %v", ps)
	}
	lit := deoptLit(core.NewList([]core.Value{deoptTok("x", 7)}), 6)
	lit.Eval = true
	if op, ok := es.callWindowOp(lit, false, nil); !ok || op.kind != WinValue {
		t.Errorf("a pending literal rides as written: %v %v", op, ok)
	}
}

func TestQuietBranchAt(t *testing.T) {
	arm := func(col int) core.Value {
		v := deoptLit(core.NewList([]core.Value{deoptTok("def", col+1)}), col)
		v.Eval = true
		return v
	}
	body := []core.Value{deoptTok("if", 1), deoptTok("c", 4), arm(6), arm(20), deoptTok("keys", 30)}
	br := &emitBranch{pos: deoptAt(4), hasElse: true, then: &EmitFragment{}, els: &EmitFragment{}}
	tree := map[int]treeEvent{1: {ev: &EmitEvent{kind: evBranch, seq: 1, br: br}}}
	if got := quietBranchAt(tree, body, 0); got != 4 {
		t.Errorf("past the else arm: %d", got)
	}
	if got := fitStartBefore(tree, body, 0, deoptAt(30)); got != 4 {
		t.Errorf("fitStartBefore: %d", got)
	}
	if got := fitStartBefore(tree, body, 0, deoptAt(6)); got != 0 {
		t.Errorf("a stop inside the branch starts at it: %d", got)
	}
	if got := defsBefore(tree, body, 0, deoptAt(30)); got != 4 {
		t.Errorf("defsBefore: %d", got)
	}
	if left, bound := defLeftovers(tree, body, 0, 4); left != nil || bound != nil {
		t.Errorf("a branch leaves no def leftover: %v %v", left, bound)
	}
	br.hasElse = false
	if got := quietBranchAt(tree, body, 0); got != 3 {
		t.Errorf("a 2-operand if: %d", got)
	}
	br.then = &EmitFragment{residualN: 1}
	if quietBranchAt(tree, body, 0) != -1 {
		t.Error("an arm that leaves a value")
	}
	br.then = &EmitFragment{}
	br.pending = core.PendingResidue{Left: true}
	if quietBranchAt(tree, body, 0) != -1 {
		t.Error("a pending literal")
	}
	br.pending = core.PendingResidue{}
	br.pos = deoptAt(99)
	if quietBranchAt(tree, body, 0) != -1 {
		t.Error("no branch at the token")
	}
	for name, tok := range map[string]int{"not an if": 1, "past the end": 3, "before the start": -1} {
		if quietBranchAt(tree, body, tok) != -1 {
			t.Errorf("%s", name)
		}
	}
	mod := deoptTok("if", 1)
	w, _ := core.AsWord(mod)
	w.ForceStack = true
	if unmodifiedWordInfo(w) {
		t.Error("a modified word")
	}
	if leadingDef(tree, treeDefs(tree), []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}, 0) != -1 {
		t.Error("a statement opening with a value")
	}
	if leadingDef(tree, treeDefs(tree), body[:2], 0) != -1 {
		t.Error("too short")
	}
}
